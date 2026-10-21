package main

// airplayCommand is deliberately limited to the transport events whose SDK
// mapping was verified in the pinned firmware. The CLI's printed help uses a
// different numbering; do not derive commands from it or accept raw arguments.
func airplayCommand(action string) ([]string, bool) {
	var code string
	switch action {
	case "media_play":
		code = "0"
	case "media_stop":
		code = "1"
	case "media_pause":
		code = "2"
	case "media_next":
		code = "3"
	case "media_previous":
		code = "4"
	default:
		return nil, false
	}
	return []string{"app.misc.ap_control", code}, true
}

func airplayReady(s state) bool {
	// Generic source 51 is checked by the native event consumer. Product source
	// 365 and an active session additionally prevent dispatch to a stale route.
	return s.Available && s.Power == "On" && s.Source != nil && *s.Source == 14 &&
		s.SourceState != nil && *s.SourceState == 7 && s.Streaming != nil &&
		s.Streaming.Service == "AirPlay" && s.Streaming.Active
}

func airplaySettingsPreserved(before, after state) bool {
	// Stop can end the session, so Active is intentionally not a postcondition.
	// No AirPlay status currently distinguishes sender Playing from Paused.
	return airplayReady(before) && after.Available && after.Power == before.Power &&
		after.Source != nil && *after.Source == *before.Source &&
		after.SourceState != nil && *after.SourceState == *before.SourceState &&
		after.Mode == before.Mode && standardSettingsPreserved(before, after) &&
		airplaySoundPreserved(before.Sound, after.Sound)
}

func airplaySoundPreserved(before, after *soundState) bool {
	if before == nil || after == nil {
		return before == nil && after == nil
	}
	return airplayValueEqual(before.Night, after.Night) &&
		airplayValueEqual(before.Bass, after.Bass) &&
		airplayValueEqual(before.Atmos, after.Atmos) &&
		airplayValueEqual(before.Rear, after.Rear)
}

func airplayValueEqual[T comparable](before, after *T) bool {
	if before == nil || after == nil {
		return before == nil && after == nil
	}
	return *before == *after
}
