package main

import (
	"context"
	"errors"
	"time"
)

// Limit the receiver adapter to the product route being qualified. Other
// inputs and custom EQ retain their existing native control path.
func castVolumeReady(s state) bool {
	return s.Available && s.Power == "On" && s.SourceState != nil && *s.SourceState == 2 &&
		s.Source != nil && *s.Source == 10 && s.Volume != nil && *s.Volume > 0 && *s.Volume < 31 && s.Muted != nil
}

func directRoutePreserved(before, after state) bool {
	return before.Available && after.Available && before.Power == "On" && after.Power == "On" &&
		before.Source != nil && after.Source != nil && *before.Source == *after.Source &&
		before.SourceState != nil && after.SourceState != nil && *before.SourceState == *after.SourceState &&
		before.Mode == after.Mode && airplaySoundPreserved(before.Sound, after.Sound)
}

// Keep request planning pure so every caller applies the same qualified-route
// and native range guards before any receiver command can be emitted.
func planCastVolume(action string, before state) (castControlRequest, uint32, bool, error) {
	if !castVolumeReady(before) {
		return castControlRequest{}, 0, false, errors.New("direct volume route is unavailable")
	}
	wantVolume, wantMute := *before.Volume, false
	var request castControlRequest
	switch action {
	case "volume_up":
		wantVolume++
		level := float64(wantVolume*100/32) / 100
		request = castControlRequest{Action: "volume", Level: &level}
	case "volume_down":
		wantVolume--
		level := float64(wantVolume*100/32) / 100
		request = castControlRequest{Action: "volume", Level: &level}
	case "mute":
		wantMute = !*before.Muted
		request = castControlRequest{Action: "mute", Muted: &wantMute}
	default:
		return castControlRequest{}, 0, false, errors.New("unsupported direct volume control")
	}
	return request, wantVolume, wantMute, nil
}

func (d *device) castVolumeAction(ctx context.Context, action string, before state) (state, error) {
	request, wantVolume, wantMute, e := planCastVolume(action, before)
	if e != nil {
		return before, e
	}
	if _, e := castControl(ctx, request); e != nil {
		return d.snapshotCurrent(), errors.New("direct volume request unconfirmed; check sound before retrying")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if e := ctx.Err(); e != nil {
			return d.snapshotCurrent(), e
		}
		if !d.same() {
			return before, errors.New("soundbar restarted during direct volume control")
		}
		volume, e1 := d.configByte(50)
		mute, e2 := d.configByte(155)
		if e1 == nil && e2 == nil && volume == wantVolume && (mute != 0) == wantMute && d.idleKeys() {
			after := d.snapshotCurrent()
			if !directRoutePreserved(before, after) || after.Volume == nil || *after.Volume != wantVolume || after.Muted == nil || *after.Muted != wantMute {
				return after, errors.New("sound settings changed during direct volume control; check sound before retrying")
			}
			return after, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return d.snapshotCurrent(), errors.New("direct volume change was not confirmed; automatic retry stopped")
}
