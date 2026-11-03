package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBassWriteChangesOnlyAmplitudeWord(t *testing.T) {
	before := bytes.Repeat([]byte{0x42}, eqBlockSize)
	after, e := bassTargetBlock(before, 0.39810717)
	if e != nil {
		t.Fatal(e)
	}
	for i := range before {
		if i < 0xdac || i >= 0xdb0 {
			if before[i] != after[i] {
				t.Fatalf("unrelated byte %x changed", i)
			}
		}
		if before[i] != 0x42 {
			t.Fatal("baseline mutated")
		}
	}
	for _, bad := range []float32{0, -1, float32(math.NaN()), float32(math.Inf(1))} {
		if _, e := bassTargetBlock(before, bad); e == nil {
			t.Fatal("invalid amplitude accepted")
		}
	}
	if _, e := bassTargetBlock(before[:eqBlockSize-1], 1); e == nil {
		t.Fatal("truncated block accepted")
	}
}

func TestBassOffsetAPIBoundaries(t *testing.T) {
	old := bassOffsetPath
	bassOffsetPath = filepath.Join(t.TempDir(), "offset.json")
	t.Cleanup(func() { bassOffsetPath = old })
	r := testRemote(t)
	for _, body := range []string{
		`{"action":"bar91.bass-offset.set"}`,
		`{"action":"bar91.bass-offset.set","parameters":{}}`,
		`{"action":"bar91.bass-offset.set","parameters":{"offset_db":null}}`,
		`{"action":"bar91.bass-offset.set","parameters":{"offset_db":3.5}}`,
		`{"action":"bar91.bass-offset.set","parameters":{"offset_db":0.1}}`,
		`{"action":"bar91.bass-offset.set","parameters":{"offset_db":"-1"}}`,
		`{"action":"bar91.bass-offset.set","parameters":{"offset_db":-1,"gains_db":[0,0,0,0,0]}}`,
		`{"action":"bar91.bass-offset.set","parameters":{"offset_db":-1},"gains_db":[0,0,0,0,0]}`,
		`{"action":"bar91.bass-offset.set","parameters":null}`,
		`{"action":"bass_next","parameters":{"offset_db":-1}}`,
		`{"action":"eq_reset","parameters":{"offset_db":0}}`,
	} {
		w := request(r, "POST", "/api/action", body, "192.168.50.2", true, "")
		if w.Code != 409 {
			t.Fatal(w.Code, body, w.Body.String())
		}
		if _, e := os.Stat(bassOffsetPath); !os.IsNotExist(e) {
			t.Fatal("invalid request saved settings")
		}
	}
	for _, csrf := range []bool{false, true} {
		ip := "192.168.50.2"
		if csrf {
			ip = "10.0.0.2"
		}
		w := request(r, "POST", "/api/action", `{"action":"bar91.bass-offset.set","parameters":{"offset_db":-1}}`, ip, csrf, "")
		if w.Code != 403 {
			t.Fatal("access guard bypassed", w.Code)
		}
	}
	r.actionMu.Lock()
	busy := request(r, "POST", "/api/action", `{"action":"bar91.bass-offset.set","parameters":{"offset_db":0}}`, "192.168.50.2", true, "")
	r.actionMu.Unlock()
	if busy.Code != 409 {
		t.Fatal("bass save bypassed operation gate", busy.Code)
	}
	if _, e := os.Stat(bassOffsetPath); !os.IsNotExist(e) {
		t.Fatal("busy request saved settings")
	}
	if top := request(r, "POST", "/api/action", `{"action":"bar91.bass-offset.set","offset_db":0}`, "192.168.50.2", true, ""); top.Code != 400 {
		t.Fatal("shared command accepted bass field", top.Code)
	}
	w := request(r, "POST", "/api/action", `{"action":"bar91.bass-offset.set","parameters":{"offset_db":0}}`, "192.168.50.2", true, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"bar91.bass-offset":{"offset_db":0,"status":"waiting"}`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if db, enabled, e := loadBassOffset(bassOffsetPath); e != nil || !enabled || db != 0 {
		t.Fatal(db, enabled, e)
	}
}

func TestBassOffsetPersistence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bass-offset.json")
	if db, enabled, e := loadBassOffset(p); e != nil || enabled || db != 0 {
		t.Fatal(db, enabled, e)
	}
	for db := -3.0; db <= 3; db += 0.5 {
		if e := saveBassOffset(p, db); e != nil {
			t.Fatal(e)
		}
		if got, enabled, e := loadBassOffset(p); e != nil || !enabled || got != db {
			t.Fatal(got, enabled, e)
		}
	}
	for _, db := range []float64{-3.5, 3.5, 0.1, math.NaN(), math.Inf(1)} {
		if e := saveBassOffset(p, db); e == nil {
			t.Fatal("invalid offset saved", db)
		}
	}
	for _, raw := range []string{`{}`, `null`, `{"offset_db":null}`, `{"offset_db":-9}`, `{"offset_db":0.1}`, `broken`} {
		if e := os.WriteFile(p, []byte(raw), 0600); e != nil {
			t.Fatal(e)
		}
		if _, enabled, e := loadBassOffset(p); e == nil || enabled {
			t.Fatal("malformed settings enabled", raw)
		}
	}
	if e := saveBassOffset(p, 0); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(p, p+"-link"); e == nil {
		if _, _, e := loadBassOffset(p + "-link"); e == nil {
			t.Fatal("settings symlink followed")
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	for _, entry := range entries {
		if entry.Name()[0] == '.' {
			t.Fatal("temporary file leaked")
		}
	}
}

func TestBassMoviePageDoesNotWidenFrontEQ(t *testing.T) {
	f := eqFixture()
	binary.LittleEndian.PutUint32(f.values[0x510000], 0x840002)
	if _, e := eqReadState(f.read, f.hpp, f.hdsp); e == nil {
		t.Fatal("Front EQ guard widened")
	}
	if s, e := eqReadStateFor(f.read, f.hpp, f.hdsp, true); e != nil || s.Element != 0x840002 {
		t.Fatal(s.Element, e)
	}
	binary.LittleEndian.PutUint32(f.values[0x510000], 0x840001)
	if _, e := eqReadStateFor(f.read, f.hpp, f.hdsp, true); e == nil {
		t.Fatal("unsupported page accepted")
	}
}

func TestBassMonitorStableSamplesAndPause(t *testing.T) {
	c := bassController{db: -3}
	s := bassReading{Level: 2, Cached: -5, Gain: float32(math.Pow(10, -5.0/20)), Native: eqNativeState{Block: make([]byte, eqBlockSize)}}
	read := func() (bassReading, error) { return s, nil }
	calls := 0
	apply := func(_ bassReading, target float32) error { calls++; s.Gain = target; return nil }
	c.tick(read, apply)
	c.tick(read, apply)
	if calls != 0 {
		t.Fatal("wrote before stable")
	}
	c.tick(read, apply)
	if calls != 1 || c.status != "applied" {
		t.Fatal(calls, c.status)
	}
	c.tick(read, apply)
	if calls != 1 {
		t.Fatal("compounded offset")
	}
	// Native level changes require three new stable samples.
	s.Level, s.Cached, s.Gain = 1, -10, float32(math.Pow(10, -10.0/20))
	c.tick(read, apply)
	c.tick(read, apply)
	if calls != 1 {
		t.Fatal("level changed without settling")
	}
	fail := func(_ bassReading, _ float32) error { calls++; return errors.New("readback failed") }
	c.tick(read, fail)
	c.tick(read, fail)
	if calls != 2 || c.status != "paused" {
		t.Fatal("repeated writes after error", calls, c.status)
	}
	c.db = 0
	c.stable = 0
	c.stableKey = ""
	s.Native.Block[0] ^= 1
	s.Level, s.Cached = 2, -5
	c.tick(read, fail)
	if calls != 2 || c.status != "paused" {
		t.Fatal("save, tuning or level change cleared route pause")
	}
	// Unsupported/unready playback breaks sample continuity and applied status.
	c.tick(func() (bassReading, error) { return s, errors.New("standby") }, apply)
	if c.status != "waiting" || c.stable != 0 {
		t.Fatal("stale applied status")
	}
}

func TestBassExtensionDiscoveryAndLifecycle(t *testing.T) {
	r := testRemote(t)
	w := request(r, "GET", "/api/features", "", "192.168.50.2", false, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"component":"bass-offset"`) || !strings.Contains(w.Body.String(), `"bar91.bass-offset.set"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { r.extensions.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("extension workers failed to join")
	}
}
