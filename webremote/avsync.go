package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Audio sync (lip-sync delay), as described in the owner's manual: hold the
// remote's TV key for more than three seconds, release, then press + or -
// within five seconds. The first press only shows the current delay; each
// following press changes it by 10 ms. The saved delay is config 375, in ms,
// so every step is confirmed by read-back.
//
// The bar stays in audio sync mode for five seconds after the last press and
// would treat + or - as sync changes meanwhile, so the control keeps the
// native lock until that window has closed. If the hold did not open sync
// mode, the first + reaches volume instead: that is detected, undone and
// reported before any sync change is attempted.
const (
	avSyncConfig       = 375
	avSyncStepMS       = 10
	avSyncMaxMS        = 250 // hardware maximum: + stops at 250 ms (qualified 2 Nov 2026)
	avSyncKey          = "0x1f02c"
	avSyncHold         = "3500"
	avSyncMaxSteps     = 8 // per request, to stay well inside the request timeout
	avSyncTimeout      = 30 * time.Second
	avSyncPressGap     = 400 * time.Millisecond
	avSyncWindowClosed = 5500 * time.Millisecond
)

func avSyncSource(source *uint32) bool {
	return source != nil && (*source == 0 || *source == 3)
}

// avSyncPlan validates a target and returns the key and number of presses
// (excluding the first, display-only press) needed from current.
func avSyncPlan(current, target int) (string, int, error) {
	if target < 0 || target > avSyncMaxMS || target%avSyncStepMS != 0 {
		return "", 0, fmt.Errorf("audio sync accepts 0 to %d ms in %d ms steps", avSyncMaxMS, avSyncStepMS)
	}
	if current < 0 || current%avSyncStepMS != 0 {
		return "", 0, errors.New("unexpected current audio sync value")
	}
	steps := (target - current) / avSyncStepMS
	if steps < 0 {
		steps = -steps
	}
	if steps > avSyncMaxSteps {
		return "", 0, fmt.Errorf("change audio sync by at most %d ms per request", avSyncMaxSteps*avSyncStepMS)
	}
	if target < current {
		return "0x6f001", steps, nil
	}
	return "0x6f000", steps, nil
}

func (d *device) waitKeysReleased(ctx context.Context, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for !d.idleKeys() {
		if e := ctx.Err(); e != nil {
			return e
		}
		if !d.same() {
			return errors.New("soundbar restarted")
		}
		if time.Now().After(deadline) {
			return errors.New("keys were not released")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func (d *device) avSyncPress(ctx context.Context, key, duration string) error {
	if e := d.cli(ctx, "mw.iom.se", key, duration); e != nil {
		return e
	}
	if e := d.waitKeysReleased(ctx, 8*time.Second); e != nil {
		return e
	}
	time.Sleep(avSyncPressGap)
	return nil
}

func (d *device) avSyncAction(ctx context.Context, target int) (state, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e := d.identify(); e != nil {
		return d.snapshotCurrent(), e
	}
	if d.eq.active {
		if e := d.resetFrontEQ(ctx); e != nil {
			return d.snapshotCurrent(), e
		}
	}
	before := d.snapshotLocked()
	if !before.Available {
		return before, errors.New(before.Error)
	}
	if before.Power != "On" {
		return before, errors.New("turn the bar on first")
	}
	// Qualified on TV/ARC (0) and HDMI (3), where holding the TV key opens sync
	// mode without changing the input. Bluetooth and streaming are untested.
	if !avSyncSource(before.SourceState) {
		return before, errors.New("select the TV or HDMI input to adjust audio sync")
	}
	if before.Volume == nil || before.Muted == nil {
		return before, errors.New("volume state unavailable")
	}
	raw, e := d.configByte(avSyncConfig)
	if e != nil {
		return before, errors.New("audio sync value unavailable")
	}
	current := int(raw)
	key, steps, e := avSyncPlan(current, target)
	if e != nil {
		return before, e
	}
	if steps == 0 {
		return before, nil
	}
	volume := *before.Volume
	e = d.withEQConsole(ctx, func(ctx context.Context) error {
		if e := d.avSyncPress(ctx, avSyncKey, avSyncHold); e != nil {
			return e
		}
		// Display-only press: must change neither the delay nor the volume.
		if e := d.avSyncPress(ctx, "0x6f000", "60"); e != nil {
			return e
		}
		if v, e := d.configByte(50); e != nil || v != volume {
			if e == nil && v == volume+1 {
				_ = d.avSyncPress(ctx, "0x6f001", "60")
			}
			return errors.New("audio sync mode did not open; volume was restored, nothing changed")
		}
		for i := 0; i < steps; i++ {
			previous, e := d.configByte(avSyncConfig)
			if e != nil {
				return errors.New("audio sync value unavailable")
			}
			if int(previous) == target {
				break
			}
			if e := d.avSyncPress(ctx, key, "60"); e != nil {
				return e
			}
			now, e := d.configByte(avSyncConfig)
			if e != nil {
				return errors.New("audio sync value unavailable")
			}
			if now == previous {
				return fmt.Errorf("audio sync stopped at %d ms (limit reached)", now)
			}
		}
		// Keep the lock until the bar leaves audio sync mode.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(avSyncWindowClosed):
		}
		return nil
	})
	after := d.snapshotCurrent()
	if e != nil {
		return after, e
	}
	if v, err := d.configByte(avSyncConfig); err != nil || int(v) != target {
		return after, errors.New("audio sync change was not confirmed")
	}
	if !standardSettingsPreserved(before, after) {
		return after, errors.New("audio sync changed but volume or mute changed; check sound before retrying")
	}
	if after.SourceState == nil || *after.SourceState != *before.SourceState {
		return after, errors.New("audio sync changed but the input changed; check the source")
	}
	return after, nil
}
