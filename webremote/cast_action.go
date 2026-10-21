package main

import (
	"context"
	"errors"
	"strings"
	"time"
)

func castMediaReady(s state) bool {
	return s.Available && s.Power == "On" && s.Source != nil && *s.Source == 13 &&
		s.SourceState != nil && *s.SourceState == 6 && s.Streaming != nil &&
		s.Streaming.Cast != nil && s.Streaming.Cast.Available && s.Streaming.Cast.HasMedia && !s.Streaming.Cast.Mirroring
}

func castMediaConfirmed(action string, result castControlResult, after state) bool {
	switch action {
	case "media_play":
		return result.PlayerState == "PLAYING" && mediaObserved(action, after)
	case "media_pause":
		return result.PlayerState == "PAUSED" && mediaObserved(action, after)
	case "media_stop":
		return (result.PlayerState == "" || result.PlayerState == "IDLE") && after.Streaming != nil &&
			after.Streaming.Cast != nil && after.Streaming.Cast.Available &&
			(!after.Streaming.Cast.HasMedia || after.Streaming.Cast.PlayerState == "IDLE")
	case "media_next", "media_previous":
		// A correlated receiver response acknowledges navigation, not which song
		// is audible. Some senders omit track metadata entirely.
		return result.PlayerState != "" && castMediaReady(after)
	}
	return false
}

func (d *device) castMediaAction(ctx context.Context, action string, before state) (state, error) {
	if !castMediaReady(before) || !mediaAllowed(action, before) {
		return before, errors.New("Cast playback source or capability changed; refresh before retrying")
	}
	result, e := castControl(ctx, castControlRequest{Action: strings.TrimPrefix(action, "media_")})
	if e != nil {
		return d.snapshotCurrent(), errors.New("Cast request unconfirmed; check playback before retrying")
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if e := ctx.Err(); e != nil {
			return d.snapshotCurrent(), e
		}
		if !d.same() {
			return before, errors.New("soundbar restarted during Cast control")
		}
		after := d.snapshotCurrent()
		if !directRoutePreserved(before, after) || !standardSettingsPreserved(before, after) {
			return after, errors.New("sound settings changed during Cast control; check playback before retrying")
		}
		if d.idleKeys() && castMediaConfirmed(action, result, after) {
			return after, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return d.snapshotCurrent(), errors.New("Cast playback change not confirmed; automatic retry stopped")
}
