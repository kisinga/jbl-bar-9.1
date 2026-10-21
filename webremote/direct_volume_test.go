package main

import (
	"math"
	"testing"
)

func directVolumeTestState() state {
	generic, product, volume := uint32(10), uint32(2), uint32(6)
	muted, night := false, false
	bass, atmos, rear := uint32(2), uint32(1), uint32(2)
	return state{
		Available: true, Power: "On", Mode: "Standard", Source: &generic,
		SourceState: &product, Volume: &volume, Muted: &muted,
		Sound: &soundState{Night: &night, Bass: &bass, Atmos: &atmos, Rear: &rear},
	}
}

func TestDirectVolumeRequiresQualifiedNativeRouteAndRange(t *testing.T) {
	if !castVolumeReady(directVolumeTestState()) {
		t.Fatal("qualified Bluetooth state rejected")
	}
	for _, tc := range []struct {
		name   string
		change func(*state)
	}{
		{"unavailable", func(s *state) { s.Available = false }},
		{"standby", func(s *state) { s.Power = "Standby" }},
		{"missing generic source", func(s *state) { s.Source = nil }},
		{"wrong generic source", func(s *state) { *s.Source = 14 }},
		{"missing product source", func(s *state) { s.SourceState = nil }},
		{"wrong product source", func(s *state) { *s.SourceState = 6 }},
		{"missing volume", func(s *state) { s.Volume = nil }},
		{"missing mute", func(s *state) { s.Muted = nil }},
		{"minimum volume", func(s *state) { *s.Volume = 0 }},
		{"unqualified high volume", func(s *state) { *s.Volume = 31 }},
		{"maximum volume", func(s *state) { *s.Volume = 32 }},
		{"invalid volume", func(s *state) { *s.Volume = math.MaxUint32 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := directVolumeTestState()
			tc.change(&s)
			if castVolumeReady(s) {
				t.Fatal("unqualified native state accepted")
			}
			for _, action := range []string{"volume_up", "volume_down", "mute"} {
				if _, _, _, e := planCastVolume(action, s); e == nil {
					t.Fatalf("%s planned despite invalid state", action)
				}
			}
		})
	}
}

func TestDirectVolumePlansNativeStepsAndMuteSemantics(t *testing.T) {
	for _, tc := range []struct {
		action      string
		start, want uint32
		beforeMuted bool
		wantMuted   bool
		percent     int
	}{
		{"volume_up", 6, 7, false, false, 21},
		{"volume_down", 6, 5, false, false, 15},
		{"volume_up", 6, 7, true, false, 21},
		{"volume_down", 6, 5, true, false, 15},
		{"volume_down", 1, 0, false, false, 0},
		{"volume_up", 30, 31, false, false, 96},
		{"mute", 6, 6, false, true, 0},
		{"mute", 6, 6, true, false, 0},
	} {
		s := directVolumeTestState()
		*s.Volume, *s.Muted = tc.start, tc.beforeMuted
		r, volume, muted, e := planCastVolume(tc.action, s)
		if e != nil || volume != tc.want || muted != tc.wantMuted {
			t.Fatalf("%s from %d muted=%v: planned %d muted=%v, %v", tc.action, tc.start, tc.beforeMuted, volume, muted, e)
		}
		if e := r.validate(); e != nil {
			t.Fatal("planner produced invalid receiver request", e)
		}
		if tc.action == "mute" {
			if r.Action != "mute" || r.Level != nil || r.Muted == nil || *r.Muted != tc.wantMuted {
				t.Fatal("mute request must not include a volume level", r)
			}
		} else if r.Action != "volume" || r.Level == nil || r.Muted != nil || math.Abs(*r.Level-float64(tc.percent)/100) > 1e-9 {
			t.Fatal("volume request must contain only the mapped level", r)
		}
		if *s.Volume != tc.start || *s.Muted != tc.beforeMuted {
			t.Fatal("planning mutated native snapshot")
		}
	}
	for _, action := range []string{"", "volume", "volume_up 2", "SET_VOLUME", "media_play"} {
		if _, _, _, e := planCastVolume(action, directVolumeTestState()); e == nil {
			t.Fatalf("unsupported action accepted: %q", action)
		}
	}
}

func TestDirectVolumePercentagesRoundTripThroughFirmwareScale(t *testing.T) {
	for initial := uint32(1); initial < 31; initial++ {
		for _, action := range []string{"volume_up", "volume_down"} {
			s := directVolumeTestState()
			*s.Volume = initial
			r, target, _, e := planCastVolume(action, s)
			if e != nil || r.Level == nil {
				t.Fatal(initial, action, e)
			}
			percent := int(math.Round(*r.Level * 100))
			// The native receiver rounds percentage up onto a 0..32 scale.
			// Emitting 97..99 would enter its exceptional top-end mapping.
			decoded := uint32((percent*32 + 99) / 100)
			if decoded != target || percent > 96 {
				t.Fatalf("%s from %d: percent=%d decoded=%d target=%d", action, initial, percent, decoded, target)
			}
		}
	}
}

func TestDirectVolumeRequiresPreservedRouteAndOtherSoundSettings(t *testing.T) {
	before, after := directVolumeTestState(), directVolumeTestState()
	*after.Volume, *after.Muted = 7, true
	if !directRoutePreserved(before, after) {
		t.Fatal("requested volume or mute changes must not count as route drift")
	}
	for _, tc := range []struct {
		name   string
		change func(*state)
	}{
		{"unavailable", func(s *state) { s.Available = false }},
		{"power", func(s *state) { s.Power = "Standby" }},
		{"generic source", func(s *state) { *s.Source = 14 }},
		{"missing generic source", func(s *state) { s.Source = nil }},
		{"product source", func(s *state) { *s.SourceState = 7 }},
		{"missing product source", func(s *state) { s.SourceState = nil }},
		{"mode", func(s *state) { s.Mode = "Smart" }},
		{"night", func(s *state) { *s.Sound.Night = true }},
		{"bass", func(s *state) { *s.Sound.Bass++ }},
		{"height", func(s *state) { *s.Sound.Atmos++ }},
		{"rear", func(s *state) { *s.Sound.Rear++ }},
		{"missing sound", func(s *state) { s.Sound = nil }},
		{"missing night", func(s *state) { s.Sound.Night = nil }},
		{"missing bass", func(s *state) { s.Sound.Bass = nil }},
		{"missing height", func(s *state) { s.Sound.Atmos = nil }},
		{"missing rear", func(s *state) { s.Sound.Rear = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after := directVolumeTestState()
			tc.change(&after)
			if directRoutePreserved(before, after) {
				t.Fatal("route or sound setting drift accepted")
			}
			if directRoutePreserved(after, before) {
				t.Fatal("missing or changed initial settings accepted")
			}
		})
	}
}
