package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAVSyncPlan(t *testing.T) {
	for _, c := range []struct {
		current, target, steps int
		key                    string
		ok                     bool
	}{
		{0, 20, 2, "0x6f000", true},
		{20, 0, 2, "0x6f001", true},
		{30, 30, 0, "0x6f000", true},
		{0, 50, 5, "0x6f000", true},
		{0, 60, 0, "", false},  // more than avSyncMaxSteps per request
		{0, 15, 0, "", false},  // not a 10 ms step
		{0, -10, 0, "", false}, // negative
		{0, 260, 0, "", false}, // above the one-byte maximum
		{5, 10, 0, "", false},  // unexpected current value
	} {
		key, steps, err := avSyncPlan(c.current, c.target)
		if (err == nil) != c.ok || err == nil && (steps != c.steps || key != c.key) {
			t.Fatalf("plan(%d,%d) = %q %d %v", c.current, c.target, key, steps, err)
		}
	}
}

func TestAVSyncSources(t *testing.T) {
	for source, want := range map[uint32]bool{0: true, 3: true, 2: false, 6: false, 7: false} {
		s := source
		if avSyncSource(&s) != want {
			t.Fatalf("source %d allowed = %v", source, !want)
		}
	}
	if avSyncSource(nil) {
		t.Fatal("unknown source must be refused")
	}
}

func TestSmartObserved(t *testing.T) {
	before := state{Available: true, Mode: "Standard"}
	after := state{Available: true, Mode: "Smart"}
	if !actionObserved("smart", before, after) || actionObserved("smart", before, before) {
		t.Fatal("smart must be observed only when the mode becomes Smart")
	}
}

func TestKeyPressWaitsForAcknowledgementBeforeRelease(t *testing.T) {
	var order []string
	var ackWait time.Duration
	k := keyPress{
		send: func(_ context.Context, key, duration string) error {
			order = append(order, "send "+key+" "+duration)
			return nil
		},
		ack: func(_ context.Context, wait time.Duration) error {
			order = append(order, "ack")
			ackWait = wait
			return nil
		},
		// Before the firmware runs the hold the keys still look idle: a release
		// check that came first would return at once.
		release: func(context.Context, time.Duration) error { order = append(order, "release"); return nil },
	}
	if e := k.press(context.Background(), "0x1f02c", "3500"); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(order, []string{"send 0x1f02c 3500", "ack", "release"}) {
		t.Fatalf("order %v", order)
	}
	if ackWait <= 3500*time.Millisecond {
		t.Fatalf("acknowledgement wait %v does not cover the hold", ackWait)
	}
	order = nil
	k.ack = func(context.Context, time.Duration) error {
		order = append(order, "ack")
		return errors.New("not acknowledged")
	}
	if k.press(context.Background(), "0x6f000", "60") == nil || !reflect.DeepEqual(order, []string{"send 0x6f000 60", "ack"}) {
		t.Fatalf("unacknowledged key must fail before the release check: %v", order)
	}
	order = nil
	if k.press(context.Background(), "0x6f000", "x") == nil || len(order) != 0 {
		t.Fatal("invalid duration must be refused before sending")
	}
}

type fakeSync struct {
	presses  []string
	cooled   int
	last     time.Time
	shown    bool
	delays   []uint32 // successive delay reads; error when exhausted
	volume   uint32
	pressErr map[int]error // error returned by the n-th press (0 = hold)
}

func (f *fakeSync) steps() avSyncSteps {
	return avSyncSteps{
		press: func(ctx context.Context, key, _ string) error {
			n := len(f.presses)
			f.presses = append(f.presses, key)
			f.last = time.Now()
			if e := ctx.Err(); e != nil {
				return e
			}
			return f.pressErr[n]
		},
		shown: func(context.Context) bool { return f.shown },
		delay: func() (uint32, error) {
			if len(f.delays) == 0 {
				return 0, errors.New("read failed")
			}
			v := f.delays[0]
			f.delays = f.delays[1:]
			return v, nil
		},
		volume: func() (uint32, error) { return f.volume, nil },
		cooldown: func(last time.Time) {
			f.cooled++
			if last.Before(f.last) {
				panic("cooldown measured before the last press")
			}
		},
		settle: 10 * time.Millisecond,
	}
}

func TestAVSyncCooldownOnEveryExit(t *testing.T) {
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []struct {
		name    string
		ctx     context.Context
		f       fakeSync
		ok      bool
		presses int
	}{
		{"success", context.Background(), fakeSync{shown: true, volume: 8, delays: []uint32{0, 0, 10, 10, 20}}, true, 3},
		{"hold failed", context.Background(), fakeSync{pressErr: map[int]error{0: errors.New("cli failed")}}, false, 1},
		{"sync mode not shown", context.Background(), fakeSync{volume: 8}, false, 1},
		{"volume moved", context.Background(), fakeSync{shown: true, volume: 9, delays: []uint32{0, 0}}, false, 3},
		{"delay read failed", context.Background(), fakeSync{shown: true, volume: 8, delays: []uint32{0, 0}}, false, 2},
		{"limit reached", context.Background(), fakeSync{shown: true, volume: 8, delays: []uint32{250, 250, 250}}, false, 2},
		{"step press failed", context.Background(), fakeSync{shown: true, volume: 8, delays: []uint32{0, 0}, pressErr: map[int]error{1: errors.New("cli failed")}}, false, 2},
		{"request expired", expired, fakeSync{shown: true, volume: 8}, false, 1},
	} {
		f := c.f
		current, target := 0, 20
		if c.name == "limit reached" {
			current, target = 250, 260
		}
		e := f.steps().run(c.ctx, "0x6f000", 2, current, target, 8)
		if (e == nil) != c.ok || f.cooled != 1 || len(f.presses) != c.presses {
			t.Fatalf("%s: err=%v cooldowns=%d presses=%v", c.name, e, f.cooled, f.presses)
		}
	}
}

func TestAVSyncCooldownIgnoresExpiredRequests(t *testing.T) {
	start := time.Now()
	avSyncCooldown(start.Add(-avSyncWindowClosed + 150*time.Millisecond))
	if elapsed := time.Since(start); elapsed < 120*time.Millisecond {
		t.Fatalf("cooldown returned after %v", elapsed)
	}
	start = time.Now()
	avSyncCooldown(start.Add(-avSyncWindowClosed - time.Second))
	if time.Since(start) > 20*time.Millisecond {
		t.Fatal("no wait once sync mode has closed")
	}
}

func TestAVSyncFirstPressChangesTheDelay(t *testing.T) {
	// The hold already shows the delay: one step down from 20 ms is one press.
	f := fakeSync{shown: true, volume: 8, delays: []uint32{20, 20, 10}}
	if e := f.steps().run(context.Background(), "0x6f001", 1, 20, 10, 8); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(f.presses, []string{avSyncKey, "0x6f001"}) {
		t.Fatalf("presses %v", f.presses)
	}
	// A step that reached the volume instead is undone with the opposite key.
	f = fakeSync{shown: true, volume: 7, delays: []uint32{20, 20}}
	if f.steps().run(context.Background(), "0x6f001", 1, 20, 10, 8) == nil || !reflect.DeepEqual(f.presses, []string{avSyncKey, "0x6f001", "0x6f000"}) {
		t.Fatalf("volume step not undone: %v", f.presses)
	}
}

// barSync simulates the bar: + and - change the delay unless the press is in
// the ignored set (numbered from 1 = first step press after the hold).
type barSync struct {
	delay   uint32
	presses int
	ignored map[int]bool
	keys    []string
}

func (b *barSync) steps() avSyncSteps {
	return avSyncSteps{
		press: func(_ context.Context, key, _ string) error {
			b.keys = append(b.keys, key)
			if key == avSyncKey {
				return nil
			}
			b.presses++
			if b.ignored[b.presses] {
				return nil
			}
			if key == "0x6f000" && b.delay < avSyncMaxMS {
				b.delay += avSyncStepMS
			} else if key == "0x6f001" && b.delay > 0 {
				b.delay -= avSyncStepMS
			}
			return nil
		},
		shown:    func(context.Context) bool { return true },
		delay:    func() (uint32, error) { return b.delay, nil },
		volume:   func() (uint32, error) { return 8, nil },
		cooldown: func(time.Time) {},
		settle:   10 * time.Millisecond,
	}
}

func TestAVSyncMultiStepRetriesAnIgnoredPress(t *testing.T) {
	b := barSync{delay: 10, ignored: map[int]bool{2: true}}
	if e := b.steps().run(context.Background(), "0x6f000", 4, 10, 50, 8); e != nil {
		t.Fatal(e)
	}
	if b.delay != 50 || b.presses != 5 {
		t.Fatalf("delay %d after %d presses", b.delay, b.presses)
	}
	b = barSync{delay: 10, ignored: map[int]bool{2: true, 3: true}}
	if e := b.steps().run(context.Background(), "0x6f000", 4, 10, 50, 8); e == nil || b.delay != 20 || b.presses != 3 {
		t.Fatalf("two ignored presses must stop: %v delay %d presses %d", e, b.delay, b.presses)
	}
	b = barSync{delay: 230}
	if e := b.steps().run(context.Background(), "0x6f000", 4, 230, 270, 8); e == nil || b.delay != avSyncMaxMS || b.presses != 3 {
		t.Fatalf("limit must stop without a retry: %v delay %d presses %d", e, b.delay, b.presses)
	}
	b = barSync{delay: 50}
	if e := b.steps().run(context.Background(), "0x6f001", 5, 50, 0, 8); e != nil || b.delay != 0 || b.presses != 5 {
		t.Fatalf("5 steps down: %v delay %d presses %d", e, b.delay, b.presses)
	}
}

func TestAVSyncDisplayMustMatchSavedDelay(t *testing.T) {
	// The display shows 30 ms while config 375 still says 10: do not step.
	b := barSync{delay: 30}
	if e := b.steps().run(context.Background(), "0x6f000", 2, 10, 30, 8); e == nil || b.presses != 0 {
		t.Fatalf("mismatch must stop before any step: %v presses %d", e, b.presses)
	}
}

func TestSyncTextMS(t *testing.T) {
	for text, want := range map[string]int{"  0MS": 0, " 20MS": 20, "250MS": 250, "120MS\x00junk": 120} {
		b := make([]byte, avSyncTextLength)
		copy(b, text)
		if v, ok := syncTextMS(b); !ok || int(v) != want {
			t.Fatalf("%q: %d %v", text, v, ok)
		}
	}
	for _, text := range []string{"", "VOL 8", "15MS", "260MS", "MS", "-10MS"} {
		b := make([]byte, avSyncTextLength)
		copy(b, text)
		if _, ok := syncTextMS(b); ok {
			t.Fatalf("%q accepted", text)
		}
	}
}
