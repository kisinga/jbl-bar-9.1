package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Audio sync (lip-sync delay), as described in the owner's manual: hold the
// remote's TV key for more than three seconds, release, then press + or -
// within five seconds. The hold shows the audio sync message on the front
// display, which is checked before any + or -. The first press only shows the
// current delay; each following press changes it by 10 ms. The saved delay is config 375, in ms,
// so every step is confirmed by read-back.
//
// The bar stays in audio sync mode for five seconds after the last press and
// would treat + or - as sync changes meanwhile, so the control keeps the
// native lock until that window has closed. If the hold did not open sync
// mode, nothing else is sent; should the first + still reach the volume, that
// is detected, undone and reported before any sync change is attempted.
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

// keyPress is the native key path used by the long-press controls, split out
// so that its ordering can be tested without a soundbar.
type keyPress struct {
	send    func(ctx context.Context, key, duration string) error
	ack     func(ctx context.Context, wait time.Duration) error
	release func(ctx context.Context, limit time.Duration) error
	gap     time.Duration
}

func (d *device) keyPresser() keyPress {
	return keyPress{
		send:    func(ctx context.Context, key, duration string) error { return d.cli(ctx, "mw.iom.se", key, duration) },
		ack:     d.acknowledge,
		release: d.waitKeysReleased,
		gap:     avSyncPressGap,
	}
}

// press sends one key and returns once the firmware has executed it and the
// key is released. Accepting the cli request does not mean the firmware ran
// it, and before the hold starts the keys still look idle, so the existing
// acknowledgement marker must be consumed first; it is queued behind the
// key, hence a wait longer than the hold.
func (k keyPress) press(ctx context.Context, key, duration string) error {
	ms, e := strconv.Atoi(duration)
	if e != nil || ms <= 0 || ms > 10000 {
		return errors.New("invalid key duration")
	}
	if e := k.send(ctx, key, duration); e != nil {
		return e
	}
	if e := k.ack(ctx, time.Duration(ms)*time.Millisecond+3*time.Second); e != nil {
		return fmt.Errorf("key was not acknowledged: %w", e)
	}
	if e := k.release(ctx, 8*time.Second); e != nil {
		return e
	}
	time.Sleep(k.gap)
	return nil
}

func (d *device) avSyncPress(ctx context.Context, key, duration string) error {
	return d.keyPresser().press(ctx, key, duration)
}

// avSyncSteps is the sync-mode part of the audio sync control.
type avSyncSteps struct {
	press    func(ctx context.Context, key, duration string) error
	shown    func(ctx context.Context) bool // audio sync message on the display
	delay    func() (uint32, error)         // saved delay, config 375
	volume   func() (uint32, error)
	cooldown func(last time.Time) // keeps the lock until sync mode has closed
}

// run enters sync mode and steps the delay. Once the hold has been sent, every
// exit, including errors and an expired request, waits for the bar to leave
// sync mode: until then + and - change the delay instead of the volume.
func (a avSyncSteps) run(ctx context.Context, key string, steps, target int, volume uint32) (err error) {
	last := time.Now()
	press := func(k, duration string) error {
		e := a.press(ctx, k, duration)
		last = time.Now()
		return e
	}
	defer func() { a.cooldown(last) }()
	if e := press(avSyncKey, avSyncHold); e != nil {
		return e
	}
	// The hold alone shows the audio sync message (26): confirm sync mode
	// before any + so that a missed hold never reaches the volume.
	if !a.shown(ctx) {
		return errors.New("audio sync mode did not open; nothing else was sent")
	}
	// Display-only press: must change neither the delay nor the volume.
	if e := press("0x6f000", "60"); e != nil {
		return e
	}
	if v, e := a.volume(); e != nil || v != volume {
		if e == nil && v == volume+1 {
			_ = press("0x6f001", "60")
		}
		return errors.New("audio sync mode did not open; volume was restored, nothing changed")
	}
	for i := 0; i < steps; i++ {
		previous, e := a.delay()
		if e != nil {
			return errors.New("audio sync value unavailable")
		}
		if int(previous) == target {
			break
		}
		if e := press(key, "60"); e != nil {
			return e
		}
		now, e := a.delay()
		if e != nil {
			return errors.New("audio sync value unavailable")
		}
		if now == previous {
			return fmt.Errorf("audio sync stopped at %d ms (limit reached)", now)
		}
	}
	return nil
}

// avSyncCooldown waits until sync mode has closed after the last press. It
// deliberately ignores the request context, which may already have expired.
func avSyncCooldown(last time.Time) {
	if wait := time.Until(last.Add(avSyncWindowClosed)); wait > 0 {
		time.Sleep(wait)
	}
}

func (d *device) avSyncAction(ctx context.Context, target int) (state, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e := d.identify(); e != nil {
		return d.snapshotCurrent(), e
	}
	// The EQ restore needs the bar on; report standby instead of its failure.
	if !d.powerOn() {
		return d.snapshotCurrent(), errors.New("turn the bar on first")
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
	sync := avSyncSteps{
		press: d.avSyncPress,
		shown: func(ctx context.Context) bool {
			return d.waitDisplay(ctx, time.Second, func(m, _ uint32) bool { return m == msgAudioSync })
		},
		delay:    func() (uint32, error) { return d.configByte(avSyncConfig) },
		volume:   func() (uint32, error) { return d.configByte(50) },
		cooldown: avSyncCooldown,
	}
	e = d.withEQConsole(ctx, func(ctx context.Context) error { return sync.run(ctx, key, steps, target, volume) })
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
