package main

import "testing"

func castActionState() state {
	s := airplayTestState()
	*s.Source, *s.SourceState = 13, 6
	s.Streaming = &streamState{Service: "Chromecast", native: 2, Cast: &CastSnapshot{Available: true, HasMedia: true, PlayerState: "PLAYING", SupportedCommands: 1 | 64 | 128}}
	return s
}

func TestCastMediaRouteRejectsBackgroundAndMirroring(t *testing.T) {
	if !castMediaReady(castActionState()) {
		t.Fatal("qualified route rejected")
	}
	for _, change := range []func(*state){
		func(s *state) { s.Available = false }, func(s *state) { s.Power = "Standby" },
		func(s *state) { s.Source = nil }, func(s *state) { *s.Source = 10 },
		func(s *state) { s.SourceState = nil }, func(s *state) { *s.SourceState = 2 },
		func(s *state) { s.Streaming = nil }, func(s *state) { s.Streaming.Cast = nil },
		func(s *state) { s.Streaming.Cast.Available = false },
		func(s *state) { s.Streaming.Cast.HasMedia = false },
		func(s *state) { s.Streaming.Cast.Mirroring = true },
	} {
		s := castActionState()
		change(&s)
		if castMediaReady(s) {
			t.Fatal("incomplete route enabled Cast mutation")
		}
	}
}

func TestCastMediaConfirmationRequiresReceiverAndNative(t *testing.T) {
	s := castActionState()
	if !castMediaConfirmed("media_play", castControlResult{PlayerState: "PLAYING"}, s) {
		t.Fatal("matching play rejected")
	}
	if castMediaConfirmed("media_pause", castControlResult{PlayerState: "PAUSED"}, s) {
		t.Fatal("receiver acknowledgment alone proved pause")
	}
	s.Streaming.Cast.PlayerState = "PAUSED"
	if castMediaConfirmed("media_pause", castControlResult{PlayerState: "PAUSED"}, s) {
		t.Fatal("native play overridden by receiver pause")
	}
	s.Streaming.native = 3
	if !castMediaConfirmed("media_pause", castControlResult{PlayerState: "PAUSED"}, s) {
		t.Fatal("matching pause rejected")
	}
	if castMediaConfirmed("media_pause", castControlResult{PlayerState: "PLAYING"}, s) {
		t.Fatal("unmatched mutation response accepted")
	}
	if castMediaConfirmed("media_stop", castControlResult{PlayerState: "IDLE"}, s) {
		t.Fatal("active session proved stop")
	}
	s.Streaming.Cast.HasMedia = false
	s.Streaming.Cast.PlayerState = ""
	if !castMediaConfirmed("media_stop", castControlResult{}, s) {
		t.Fatal("empty media session did not confirm stop")
	}
	if castMediaConfirmed("media_next", castControlResult{PlayerState: "PLAYING"}, s) {
		t.Fatal("missing session proved next")
	}
	s = castActionState()
	if !castMediaConfirmed("media_next", castControlResult{PlayerState: "PLAYING"}, s) {
		t.Fatal("accepted navigation response rejected")
	}
	if castMediaConfirmed("media_next", castControlResult{}, s) {
		t.Fatal("empty response proved next")
	}
	if castMediaConfirmed("media_unknown", castControlResult{PlayerState: "PLAYING"}, s) {
		t.Fatal("unknown action accepted")
	}
}

func TestCastStopCannotBeConfirmedByAnotherRoute(t *testing.T) {
	for _, change := range []func(*state){
		func(s *state) { s.Available = false },
		func(s *state) { s.Power = "Standby" },
		func(s *state) { s.Source = nil },
		func(s *state) { *s.Source = 10 },
		func(s *state) { s.SourceState = nil },
		func(s *state) { *s.SourceState = 2 },
		func(s *state) { s.Streaming = nil },
		func(s *state) { s.Streaming.Cast = nil },
		func(s *state) { s.Streaming.Cast.Available = false },
		func(s *state) { s.Streaming.Cast.Mirroring = true },
	} {
		s := castActionState()
		s.Streaming.Cast.HasMedia = false
		s.Streaming.Cast.PlayerState = "IDLE"
		change(&s)
		if castMediaConfirmed("media_stop", castControlResult{}, s) {
			t.Fatal("unavailable, changed or mirrored route confirmed Stop")
		}
	}
}
