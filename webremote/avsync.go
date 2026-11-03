package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Audio sync (lip-sync delay), as described in the owner's manual: hold the
// remote's TV key for more than three seconds, release, then press + or -
// within five seconds. The hold opens sync mode and shows the current delay
// (audio sync message on the front display, checked before any + or -); each
// following press changes it by 10 ms. The saved delay is config 375, in ms,
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
	avSyncMaxSteps     = avSyncMaxMS / avSyncStepMS // the whole range in one sync session: no gap between steps
	avSyncTimeout      = 60 * time.Second           // 25 steps take about 30 s including the hold and the close
	avSyncPressGap     = 400 * time.Millisecond
	avSyncWindowClosed = 5500 * time.Millisecond
	avSyncSettle       = 1500 * time.Millisecond
	avSyncSaveWait     = 3 * time.Second
	avSyncText         = vfdContext + 0xee // display text of the delay, e.g. "  20MS"
	avSyncTextLength   = 28
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
	delay    func() (uint32, error)         // delay shown on the display while sync mode is open
	volume   func() (uint32, error)
	cooldown func(last time.Time) // keeps the lock until sync mode has closed
	settle   time.Duration        // how long a step may take to show on the display
}

// run enters sync mode and steps the delay. Once the hold has been sent, every
// exit, including errors and an expired request, waits for the bar to leave
// sync mode: until then + and - change the delay instead of the volume.
//
// Config 375 is only saved when sync mode closes, so steps are confirmed with
// the delay the display shows; the caller checks config 375 afterwards.
func (a avSyncSteps) run(ctx context.Context, key string, steps, current, target int, volume uint32) (err error) {
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
	if shown, e := a.delay(); e != nil || int(shown) != current {
		return errors.New("the displayed audio sync delay does not match the saved one; nothing else was sent")
	}
	settle := a.settle
	if settle <= 0 {
		settle = avSyncSettle
	}
	retried := false
	for done := 0; done < steps; {
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
		// Second safeguard after the display check: no step may reach the
		// volume (the bar leaves sync mode five seconds after a press).
		if v, e := a.volume(); e != nil || v != volume {
			if e == nil && (v == volume+1 || v+1 == volume) {
				undo := "0x6f001"
				if v < volume {
					undo = "0x6f000"
				}
				_ = press(undo, "60")
			}
			return errors.New("a step reached the volume instead of audio sync; volume was restored")
		}
		now, e := a.delay()
		deadline := time.Now().Add(settle)
		for e == nil && now == previous && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			now, e = a.delay()
		}
		if e != nil {
			return errors.New("audio sync value unavailable")
		}
		if now != previous {
			done, retried = done+1, false
			continue
		}
		atLimit := key == "0x6f000" && int(previous) >= avSyncMaxMS || key == "0x6f001" && previous == 0
		if atLimit || retried {
			if atLimit {
				return fmt.Errorf("audio sync stopped at %d ms (limit reached)", now)
			}
			return fmt.Errorf("audio sync stopped at %d ms: the bar ignored two presses in a row", now)
		}
		// One press was not taken (the bar can drop a press that follows the
		// previous one closely): try it once more.
		retried = true
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
		delay:    d.shownSyncDelay,
		volume:   func() (uint32, error) { return d.configByte(50) },
		cooldown: avSyncCooldown,
	}
	e = d.withEQConsole(ctx, func(ctx context.Context) error { return sync.run(ctx, key, steps, current, target, volume) })
	after := d.snapshotCurrent()
	if e != nil {
		return after, e
	}
	// Sync mode has closed (cooldown); the bar saves the delay at that point.
	saved := func() bool { v, err := d.configByte(avSyncConfig); return err == nil && int(v) == target }
	deadline := time.Now().Add(avSyncSaveWait)
	for !saved() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if !saved() {
		return d.snapshotCurrent(), errors.New("audio sync change was not saved by the bar")
	}
	if !standardSettingsPreserved(before, after) {
		return after, errors.New("audio sync changed but volume or mute changed; check sound before retrying")
	}
	if after.SourceState == nil || *after.SourceState != *before.SourceState {
		return after, errors.New("audio sync changed but the input changed; check the source")
	}
	return after, nil
}

// syncTextMS parses the audio sync display text ("  0MS", "120MS").
func syncTextMS(b []byte) (uint32, bool) {
	text := strings.TrimSpace(audioFormatText(b))
	digits, ok := strings.CutSuffix(text, "MS")
	if !ok {
		return 0, false
	}
	v, e := strconv.Atoi(strings.TrimSpace(digits))
	if e != nil || v < 0 || v > avSyncMaxMS || v%avSyncStepMS != 0 {
		return 0, false
	}
	return uint32(v), true
}

func (d *device) shownSyncDelay() (uint32, error) {
	b, e := d.readBytes(avSyncText, avSyncTextLength)
	if e != nil {
		return 0, e
	}
	v, ok := syncTextMS(b)
	if !ok {
		return 0, errors.New("audio sync display unreadable")
	}
	return v, nil
}
