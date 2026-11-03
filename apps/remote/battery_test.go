package main

import "testing"

func TestRearBatteryPercent(t *testing.T) {
	for _, tc := range []struct{ raw, want uint32 }{
		{0, 0}, {1, 1}, {80, 80}, {90, 90}, {99, 99}, {100, 100},
		{101, 1}, {180, 80}, {190, 90}, {199, 99}, {200, 100},
	} {
		got := rearBatteryPercent(tc.raw)
		if got == nil || *got != tc.want {
			t.Errorf("raw %d: got %v, want %d", tc.raw, got, tc.want)
		}
	}
	for _, raw := range []uint32{201, 255, 256} {
		if rearBatteryPercent(raw) != nil {
			t.Errorf("unqualified value %d accepted", raw)
		}
	}
}
