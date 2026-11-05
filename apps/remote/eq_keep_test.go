package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEQFollowsNativelyOnlyMeasuredControls(t *testing.T) {
	for _, action := range []string{"bass_next", "atmos_next", "rear_next", "night_toggle", "standard", "smart", "input_tv", "input_hdmi", "input_bluetooth"} {
		if !eqFollowsNatively(action) || eqResetBefore(action, true) {
			t.Fatalf("%s still removes the EQ first", action)
		}
	}
	// Not measured with an EQ applied: keep removing it first.
	for _, action := range []string{"power", "bluetooth_pair", "center_next", "calibrate", "sub_pair", "av_sync_set"} {
		if eqFollowsNatively(action) {
			t.Fatalf("%s follows natively without device evidence", action)
		}
	}
	if !eqResetBefore("power", true) {
		t.Fatal("power from on still removes the EQ first")
	}
}

func TestEQFollowDecision(t *testing.T) {
	native := eqNativeState{Mode: 5, outer: 0x100, SampleRate: 48000}
	expected := []byte{1, 2, 3}
	at := func(mode, cache uint32, block []byte) eqNativeState {
		return eqNativeState{Mode: mode, Cache: cache, outer: 0x100, SampleRate: 48000, Block: block}
	}
	cases := []struct {
		name    string
		n       eqNativeState
		err     error
		smart   bool
		changed bool
		want    eqFollow
	}{
		{"expected block", at(5, 0, expected), nil, false, false, eqFollowOK},
		{"recomputed after Bass/Atmos/Rear", at(5, 0, []byte{9}), nil, false, true, eqFollowLevels},
		{"other block, same levels", at(5, 0, []byte{9}), nil, false, false, eqFollowReset},
		{"dropped by a source change", at(0, 0, []byte{7}), nil, false, false, eqFollowDropped},
		{"dropped, levels changed too", at(0, 0, []byte{7}), nil, false, true, eqFollowDropped},
		{"Smart Mode", at(1, 1, []byte{7}), nil, true, false, eqFollowSmart},
		{"unqualified route", eqNativeState{}, errEQRoute, false, false, eqFollowReset},
		{"Smart, unreadable", eqNativeState{}, errEQRoute, true, false, eqFollowSmart},
		{"other audio instance", eqNativeState{Mode: 5, outer: 0x200, SampleRate: 48000, Block: []byte{9}}, nil, false, true, eqFollowReset},
		{"other sample rate", eqNativeState{Mode: 5, outer: 0x100, SampleRate: 44100, Block: []byte{9}}, nil, false, true, eqFollowReset},
		{"unknown mode", at(1, 0, []byte{9}), nil, false, true, eqFollowReset},
	}
	for _, c := range cases {
		if got := eqFollowDecision(c.n, c.err, native, expected, c.smart, c.changed); got != c.want {
			t.Fatalf("%s: got %d want %d", c.name, got, c.want)
		}
	}
	if eqFollowDecision(eqNativeState{}, errors.New("soundbar restarted"), native, expected, false, false) != eqFollowReset {
		t.Fatal("other read errors keep the reset path")
	}
}

func TestKeptEQFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "front-eq.json")
	if _, ok := loadKeptEQ(path); ok {
		t.Fatal("missing file kept an EQ")
	}
	want := [5]float64{0, 0, 0, -6, -6}
	if e := saveKeptEQ(path, want); e != nil {
		t.Fatal(e)
	}
	if got, ok := loadKeptEQ(path); !ok || got != want {
		t.Fatalf("kept %v %v", got, ok)
	}
	if info, e := os.Stat(path); e != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v %v", info, e)
	}
	for _, bad := range [][5]float64{{}, {1, 0, 0, 0, 0}, {-6.5, 0, 0, 0, 0}, {-0.25, 0, 0, 0, 0}} {
		if saveKeptEQ(path, bad) == nil {
			t.Fatalf("invalid EQ kept: %v", bad)
		}
	}
	for _, raw := range []string{`garbage`, `{"gains_db":[0,0,0]}`, `{"gains_db":[0,0,0,0,0]}`, `{"gains_db":[0,0,0,0,3]}`} {
		os.WriteFile(path, []byte(raw), 0600)
		if _, ok := loadKeptEQ(path); ok {
			t.Fatalf("accepted %s", raw)
		}
	}
	if e := clearKeptEQ(path); e != nil {
		t.Fatal(e)
	}
	if e := clearKeptEQ(path); e != nil {
		t.Fatal("clearing twice", e)
	}
	if _, e := os.Stat(path + ".new"); !os.IsNotExist(e) {
		t.Fatal("temporary file left")
	}
}

func TestKeptEQForgetAndKeep(t *testing.T) {
	saved := eqKeepPath
	eqKeepPath = filepath.Join(t.TempDir(), "front-eq.json")
	defer func() { eqKeepPath = saved }()
	d := &device{}
	d.keepFrontEQGains([5]float64{-3, 0, 0, 0, 0})
	if !d.eq.keep {
		t.Fatal("not kept")
	}
	d2 := &device{}
	d2.loadKeptFrontEQ()
	if !d2.eq.keep || d2.eq.kept != [5]float64{-3, 0, 0, 0, 0} {
		t.Fatal("kept EQ not restored after a restart")
	}
	d2.forgetKeptFrontEQ()
	d3 := &device{}
	d3.loadKeptFrontEQ()
	if d3.eq.keep {
		t.Fatal("Reset did not forget the kept EQ")
	}
}

func TestKeptEQStatusShowsTheWaitingEQ(t *testing.T) {
	d := &device{}
	d.eq.keep, d.eq.kept = true, [5]float64{0, 0, 0, -6, -6}
	// Standby: the native reader is not consulted.
	out := d.frontEQStatus(state{Available: true, Power: "Standby"})
	if !out.Kept || out.Status != "waiting" || out.Error != eqKeepWaiting || out.Gains[4] != -6 {
		t.Fatalf("%+v", out)
	}
	d.eq.keep = false
	if out = d.frontEQStatus(state{Available: true, Power: "Standby"}); out.Kept || out.Status == "waiting" {
		t.Fatalf("%+v", out)
	}
}
