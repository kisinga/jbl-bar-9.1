package main

import (
	"reflect"
	"testing"
)

func airplayTestState() state {
	generic, product, volume := uint32(14), uint32(7), uint32(6)
	muted, night := false, false
	bass, atmos, rear := uint32(2), uint32(1), uint32(2)
	return state{
		Available: true, Power: "On", Mode: "Standard", Source: &generic,
		SourceState: &product, Volume: &volume, Muted: &muted,
		Sound:     &soundState{Night: &night, Bass: &bass, Atmos: &atmos, Rear: &rear},
		Streaming: &streamState{Service: "AirPlay", Status: "Session active", Active: true},
	}
}

func TestAirPlayTransportUsesVerifiedSDKMapping(t *testing.T) {
	for _, tc := range []struct{ action, code string }{
		{"media_play", "0"}, {"media_stop", "1"}, {"media_pause", "2"},
		{"media_next", "3"}, {"media_previous", "4"},
	} {
		got, ok := airplayCommand(tc.action)
		if !ok || !reflect.DeepEqual(got, []string{"app.misc.ap_control", tc.code}) {
			t.Fatalf("%s: command=%v accepted=%v", tc.action, got, ok)
		}
	}
	for _, action := range []string{"", "play", "power", "volume_up", "media_repeat", "media_play 7", "app.misc.AirPlay2_Exec"} {
		if command, ok := airplayCommand(action); ok || command != nil {
			t.Fatalf("accepted action outside fixed transport set: %q", action)
		}
	}
}

func TestAirPlayCapabilitiesRequireBothSourceGuardsAndSession(t *testing.T) {
	for _, action := range []string{"media_previous", "media_play", "media_pause", "media_next", "media_stop"} {
		if !mediaAllowed(action, airplayTestState()) {
			t.Fatalf("active AirPlay lacks %s", action)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*state)
	}{
		{"unavailable", func(s *state) { s.Available = false }},
		{"standby", func(s *state) { s.Power = "Standby" }},
		{"unknown generic source", func(s *state) { s.Source = nil }},
		{"wrong generic source", func(s *state) { *s.Source = 10 }},
		{"unknown product source", func(s *state) { s.SourceState = nil }},
		{"background AirPlay", func(s *state) { *s.SourceState = 0 }},
		{"no stream state", func(s *state) { s.Streaming = nil }},
		{"other service", func(s *state) { s.Streaming.Service = "Chromecast" }},
		{"inactive session", func(s *state) { s.Streaming.Active = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := airplayTestState()
			tc.change(&s)
			if airplayReady(s) || len(mediaCommands(s)) != 0 {
				t.Fatal("AirPlay commands enabled without a complete active route")
			}
		})
	}
}

func TestAirPlaySessionDoesNotClaimSenderPlayback(t *testing.T) {
	s := airplayTestState()
	for _, status := range []string{"Session active", "Playing", "Paused"} {
		s.Streaming.Status = status
		for _, action := range []string{"media_play", "media_pause"} {
			if mediaObserved(action, s) {
				t.Fatalf("%s inferred from AirPlay session status %q", action, status)
			}
		}
	}
}

func TestAirPlayRequestRequiresPreservedSettings(t *testing.T) {
	before, after := airplayTestState(), airplayTestState()
	if !airplaySettingsPreserved(before, after) {
		t.Fatal("unchanged settings rejected")
	}
	after.Streaming.Active = false
	if !airplaySettingsPreserved(before, after) {
		t.Fatal("ending the AirPlay session rejected despite preserved route/settings")
	}
	for _, tc := range []struct {
		name   string
		change func(*state)
	}{
		{"unavailable", func(s *state) { s.Available = false }},
		{"power", func(s *state) { s.Power = "Standby" }},
		{"generic source", func(s *state) { *s.Source = 10 }},
		{"missing generic source", func(s *state) { s.Source = nil }},
		{"product source", func(s *state) { *s.SourceState = 2 }},
		{"missing product source", func(s *state) { s.SourceState = nil }},
		{"mode", func(s *state) { s.Mode = "Smart" }},
		{"volume", func(s *state) { *s.Volume++ }},
		{"missing volume", func(s *state) { s.Volume = nil }},
		{"mute", func(s *state) { *s.Muted = true }},
		{"missing mute", func(s *state) { s.Muted = nil }},
		{"night", func(s *state) { *s.Sound.Night = true }},
		{"bass", func(s *state) { *s.Sound.Bass++ }},
		{"height", func(s *state) { *s.Sound.Atmos++ }},
		{"rear", func(s *state) { *s.Sound.Rear++ }},
		{"missing sound", func(s *state) { s.Sound = nil }},
		{"missing sound value", func(s *state) { s.Sound.Bass = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after := airplayTestState()
			tc.change(&after)
			if airplaySettingsPreserved(before, after) {
				t.Fatal("changed or lost settings accepted")
			}
		})
	}
	before.Streaming.Active = false
	if airplaySettingsPreserved(before, airplayTestState()) {
		t.Fatal("accepted request begun without an active AirPlay session")
	}
}
