package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"
)

// Front display state (VFDMG in bdpprog 23.06.41.80, static addresses).
//
// vfdState+0x1c is the temporary message (level 2, 117 = none, each message
// lasts five seconds); vfdState+0x10 is the overlay used by pairing modes
// (18 = none, 17 = subwoofer pairing). vfdContext+0x10a holds the text of the
// last audio stream message (for example "DOLBY SURROUND DD+").
// Qualified on 2 Nov 2026 by watching the values during the gestures below.
const (
	vfdState          = 0xc65494
	vfdContext        = 0xd8a088
	vfdMessageNone    = 117
	vfdOverlayNone    = 18
	vfdOverlaySubPair = 17
	vfdAudioStream    = vfdContext + 0x10a
	vfdAudioLength    = 64
	// Holds of six seconds plus key release and confirmation.
	setupTimeout = 18 * time.Second
)

// Level-2 message numbers used by the setup controls.
const (
	msgSubNotConnected = 2
	msgSubConnected    = 3
	msgAudioSync       = 26
	msgAudioStream     = 94
	msgCalibration1    = 58
	msgCalibration2    = 59
	msgCountdownLast   = 64 // 60..64 = countdown 1..5
	msgCalCancelled    = 65
	msgCalibrating     = 66
	msgCalDone1        = 67
	msgCalDone2        = 68
	msgCalRetry        = 69
	msgReconnectSub    = 77
)

type setupState struct {
	running bool
	// Calibration is the current calibration display, empty when idle.
	Calibration string `json:"calibration,omitempty"`
	// SubPairing is true while the bar waits for the subwoofer.
	SubPairing bool `json:"sub_pairing"`
	// Subwoofer is the last subwoofer message while it is shown.
	Subwoofer string `json:"subwoofer,omitempty"`
}

func calibrationPhase(message uint32) string {
	switch {
	case message == msgCalibration1:
		return "Step 1 starting"
	case message == msgCalibration2:
		return "Step 2 starting"
	case message >= 60 && message <= msgCountdownLast:
		return "Starting in " + string(rune('0'+message-59))
	case message == msgCalibrating:
		return "Measuring"
	case message == msgCalDone1:
		return "Step 1 done"
	case message == msgCalDone2:
		return "Step 2 done"
	case message == msgCalRetry:
		return "Failed, retry"
	case message == msgCalCancelled:
		return "Cancelled"
	}
	return ""
}

// calibrationRunning reports the messages during which the bar is measuring
// or about to; native keys would cancel it.
func calibrationRunning(message uint32) bool {
	return message >= msgCalibration1 && message <= msgCalibrating
}

func subwooferMessage(message uint32) string {
	switch message {
	case msgSubConnected:
		return "Connected"
	case msgSubNotConnected:
		return "Not connected"
	case msgReconnectSub:
		return "Reconnecting"
	}
	return ""
}

func audioFormatText(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	for _, c := range b {
		if c < 32 || c > 126 {
			return ""
		}
	}
	return strings.Join(strings.Fields(string(b)), " ")
}

func (d *device) displayMessages() (message, overlay uint32, err error) {
	if message, err = d.read(vfdState+0x1c, 4); err != nil {
		return
	}
	if overlay, err = d.read(vfdState+0x10, 4); err != nil {
		return
	}
	if message > vfdMessageNone || overlay > vfdOverlayNone {
		err = errors.New("display state layout changed")
	}
	return
}

func (d *device) setup() *setupState {
	message, overlay, e := d.displayMessages()
	if e != nil {
		return nil
	}
	return &setupState{running: calibrationRunning(message), Calibration: calibrationPhase(message), SubPairing: overlay == vfdOverlaySubPair, Subwoofer: subwooferMessage(message)}
}

// formatTracker ties the last audio stream text to the source it was shown
// on. The bar only shows a stream format on TV/ARC and HDMI; the text stays
// in memory afterwards, so a Bluetooth or streaming source would otherwise
// keep reporting the previous TV format.
type formatTracker struct {
	text   string
	source uint32
	known  bool
}

func (f *formatTracker) observe(text string, shown bool, source *uint32) string {
	if source == nil {
		return ""
	}
	// A new text, or the stream message on the display (the same format can
	// be shown again after a source change), ties the format to this source.
	if !f.known || text != f.text || shown {
		f.text, f.source, f.known = text, *source, true
	}
	if (*source != 0 && *source != 3) || f.source != *source {
		return ""
	}
	return f.text
}

func (d *device) audioFormat(source *uint32) string {
	b, e := d.readBytes(vfdAudioStream, vfdAudioLength)
	if e != nil {
		return ""
	}
	message, _, _ := d.displayMessages()
	return d.format.observe(audioFormatText(b), message == msgAudioStream, source)
}

// calibrating is used to refuse every other control while the bar measures.
func (d *device) calibrating() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.identify() != nil {
		return false
	}
	message, _, e := d.displayMessages()
	return e == nil && calibrationRunning(message)
}

func (d *device) waitDisplay(ctx context.Context, limit time.Duration, match func(message, overlay uint32) bool) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil || !d.same() {
			return false
		}
		if message, overlay, e := d.displayMessages(); e == nil && match(message, overlay) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func (d *device) setupPrepare(ctx context.Context) (state, error) {
	if e := d.identify(); e != nil {
		return d.snapshotCurrent(), e
	}
	// The EQ restore needs the bar on; report standby instead of its failure.
	if !d.powerOn() {
		return d.snapshotCurrent(), errors.New("turn the bar on first")
	}
	if e := d.resetFrontEQBefore(ctx, "setup"); e != nil {
		return d.snapshotCurrent(), e
	}
	before := d.snapshotLocked()
	if !before.Available {
		return before, errors.New(before.Error)
	}
	if before.Power != "On" {
		return before, errors.New("turn the bar on first")
	}
	if before.Setup == nil {
		return before, errors.New("display state unavailable")
	}
	return before, nil
}

// calibrateAction holds HDMI for six seconds, as the manual describes for
// wireless surround calibration. The bar chooses step 1 or step 2 and runs it
// on its own (about 35 s); the request returns once the step has started and
// the state then reports its progress.
func (d *device) calibrateAction(ctx context.Context) (state, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	before, e := d.setupPrepare(ctx)
	if e != nil {
		return before, e
	}
	if before.Sound == nil || before.Sound.RearDocked == nil || *before.Sound.RearDocked {
		return before, errors.New("undock both rear speakers first")
	}
	if before.Setup.running {
		return before, errors.New("calibration is already running")
	}
	if before.Setup.SubPairing {
		return before, errors.New("subwoofer pairing is running")
	}
	started := false
	e = d.withEQConsole(ctx, func(ctx context.Context) error {
		if e := d.avSyncPress(ctx, "0x5f03e", "6000"); e != nil {
			return e
		}
		started = d.waitDisplay(ctx, 3*time.Second, func(message, _ uint32) bool {
			return message == msgCalibration1 || message == msgCalibration2 || (message >= 60 && message <= msgCalibrating)
		})
		return nil
	})
	after := d.snapshotCurrent()
	if e != nil {
		return after, e
	}
	if !started {
		return after, errors.New("calibration did not start; nothing else was sent")
	}
	return after, nil
}

// subPairAction holds BASS for six seconds, then presses -, as the manual
// describes. The bar then waits up to three minutes for the subwoofer, whose
// own button must be held until it flashes white quickly.
func (d *device) subPairAction(ctx context.Context) (state, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	before, e := d.setupPrepare(ctx)
	if e != nil {
		return before, e
	}
	if before.Setup.SubPairing {
		return before, nil
	}
	if before.Setup.running {
		return before, errors.New("calibration is running")
	}
	if before.Volume == nil || before.Muted == nil || before.Sound == nil || before.Sound.Bass == nil {
		return before, errors.New("volume or bass state unavailable")
	}
	volume := *before.Volume
	pairing := false
	e = d.withEQConsole(ctx, func(ctx context.Context) error {
		if e := d.avSyncPress(ctx, "0x1f030", "6000"); e != nil {
			return e
		}
		if e := d.avSyncPress(ctx, "0x6f001", "60"); e != nil {
			return e
		}
		pairing = d.waitDisplay(ctx, 3*time.Second, func(_, overlay uint32) bool { return overlay == vfdOverlaySubPair })
		if !pairing {
			// The hold was not taken as a pairing request: - lowered the volume.
			if v, e := d.configByte(50); e == nil && volume > 0 && v == volume-1 {
				return d.avSyncPress(ctx, "0x6f000", "60")
			}
		}
		return nil
	})
	after := d.snapshotCurrent()
	if e != nil {
		return after, e
	}
	if !pairing {
		return after, errors.New("subwoofer pairing did not start; volume was restored")
	}
	if after.Sound == nil || after.Sound.Bass == nil || *after.Sound.Bass != *before.Sound.Bass {
		return after, errors.New("pairing started but the bass level changed; check it")
	}
	if !standardSettingsPreserved(before, after) {
		return after, errors.New("pairing started but volume or mute changed; check sound")
	}
	return after, nil
}
