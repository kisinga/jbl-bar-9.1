package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"time"
)

var bassOffsetPath = "/var/JBLRemote/bass-offset.json"

type bassOffsetState struct {
	OffsetDB float64 `json:"offset_db"`
	Status   string  `json:"status"`
	Error    string  `json:"error,omitempty"`
}

type bassController struct {
	loaded, enabled      bool
	db                   float64
	status, problem      string
	stableKey, pausedKey string
	stable               int
}

func validBassOffset(db float64) bool {
	return !math.IsNaN(db) && !math.IsInf(db, 0) && db >= -3 && db <= 3 && db*2 == math.Trunc(db*2)
}

func loadBassOffset(path string) (float64, bool, error) {
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return 0, false, nil
	}
	if e != nil {
		return 0, false, e
	}
	if !info.Mode().IsRegular() || info.Size() > 256 {
		return 0, false, errors.New("invalid bass offset file")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return 0, false, e
	}
	var cfg struct {
		OffsetDB *float64 `json:"offset_db"`
	}
	if json.Unmarshal(raw, &cfg) != nil || cfg.OffsetDB == nil || !validBassOffset(*cfg.OffsetDB) {
		return 0, false, errors.New("invalid saved bass offset")
	}
	return *cfg.OffsetDB, true, nil
}

func saveBassOffset(path string, db float64) error {
	if !validBassOffset(db) {
		return errors.New("bass offset accepts -3 to +3 dB in 0.5 dB steps")
	}
	raw, _ := json.Marshal(struct {
		OffsetDB float64 `json:"offset_db"`
	}{db})
	f, e := os.CreateTemp(filepath.Dir(path), ".bass-offset-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(append(raw, '\n')); e == nil {
		e = f.Sync()
	}
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}

// A missing file leaves the installation entirely untouched. Saving zero opts
// into restoring stock bass on supported routes, including after a restart.
func (d *device) bassStatus() *bassOffsetState {
	c := &d.bass
	if !c.loaded {
		var e error
		c.db, c.enabled, e = loadBassOffset(bassOffsetPath)
		c.loaded, c.status = true, "original"
		if c.enabled {
			c.status = "waiting"
		}
		if e != nil {
			c.status, c.problem = "paused", e.Error()
		}
	}
	return &bassOffsetState{OffsetDB: c.db, Status: c.status, Error: c.problem}
}

func (d *device) bassOffsetAction(db float64) (state, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bassStatus()
	if e := saveBassOffset(bassOffsetPath, db); e != nil {
		return d.snapshotLocked(), e
	}
	c := &d.bass
	c.db, c.enabled, c.status, c.problem = db, true, "waiting", ""
	c.stable, c.stableKey = 0, ""
	// Saving must not clear a write-error pause on the same route.
	return d.snapshotLocked(), nil
}

// Poll under the same locks as UI, EQ, Smart Mode and center operations.
// Three stable samples precede a write. A write/readback failure latches a
// pause for that route; the monitor never repeatedly writes through an error.
func (d *device) bassTick() {
	d.bassStatus()
	c := &d.bass
	if !c.enabled {
		return
	}
	c.tick(func() (bassReading, error) { return bassRead(d) }, func(s bassReading, gain float32) error { return bassWrite(d, s, gain) })
}

func (c *bassController) tick(read func() (bassReading, error), apply func(bassReading, float32) error) {
	sample, e := read()
	if e != nil {
		c.stable, c.stableKey, c.status, c.problem = 0, "", "waiting", e.Error()
		return
	}
	key := bassKey(sample)
	if bassRouteKey(sample) == c.pausedKey {
		c.status, c.problem = "paused", "Bass readback failed on this route; automatic writes paused."
		return
	}
	if key != c.stableKey {
		c.stableKey, c.stable = key, 0
	}
	c.stable++
	if c.stable < 3 {
		c.status, c.problem = "waiting", ""
		return
	}
	target, write, e := bassPlan(sample.Level, sample.Cached, sample.Gain, c.db)
	if e != nil {
		c.status, c.problem = "waiting", e.Error()
		return
	}
	if write {
		if e = apply(sample, target); e != nil {
			c.pausedKey, c.status, c.problem = bassRouteKey(sample), "paused", e.Error()
			return
		}
	}
	c.status, c.problem = "applied", ""
}

func (r *remote) monitorBass(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !r.actionMu.TryLock() {
			continue
		}
		r.device.mu.Lock()
		r.device.bassTick()
		r.device.mu.Unlock()
		r.actionMu.Unlock()
	}
}
