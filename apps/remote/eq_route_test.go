package main

import (
	"errors"
	"strings"
	"testing"
)

func TestEQHPPModeReadsAnyRoute(t *testing.T) {
	f := eqFixture()
	// The full reader rejects the film page in Smart Mode (cache 2, mode 2),
	// as observed on the bar; the mode reader still reports the mode.
	entry := f.values[0x510000]
	entry[0], entry[1], entry[2] = 0x02, 0x00, 0x84
	f.word(0x530000+0x38, 2)
	f.word(0x550000+0x1108, 2)
	if _, e := eqReadState(f.read, f.hpp, f.hdsp); !errors.Is(e, errEQRoute) {
		t.Fatalf("full reader accepted the unqualified route: %v", e)
	}
	if m, e := eqReadHPPMode(f.read, f.hpp); e != nil || m != 2 {
		t.Fatalf("mode=%d err=%v", m, e)
	}
	f.word(0x550000+0x1108, eqCustomMode)
	if m, e := eqReadHPPMode(f.read, f.hpp); e != nil || m != eqCustomMode {
		t.Fatalf("custom mode=%d err=%v", m, e)
	}
	for name, change := range map[string]func(){
		"no outer":     func() { f.word(f.hpp+0xa45c, 0) },
		"no instance":  func() { f.word(0x540000, 0) },
		"unknown mode": func() { f.word(0x550000+0x1108, 99) },
	} {
		g := *f
		g.values = map[uint32][]byte{}
		for k, v := range f.values {
			g.values[k] = append([]byte(nil), v...)
		}
		saved := f.values
		f.values = g.values
		change()
		if _, e := eqReadHPPMode(f.read, f.hpp); e == nil {
			t.Fatalf("%s accepted", name)
		}
		f.values = saved
	}
}

func TestEQPendingResetDecision(t *testing.T) {
	mode := func(m uint32, e error) func() (uint32, error) { return func() (uint32, error) { return m, e } }
	unreadable := errors.New("native audio mode unavailable")
	cases := []struct {
		name     string
		reset    error
		action   string
		mode     func() (uint32, error)
		proceed  bool
		problem  string
		errorHas string
	}{
		{"reset done", nil, "bass_next", mode(0, nil), true, "", ""},
		{"other failure still refuses", errors.New("audio controls are busy; retry shortly"), "bass_next", mode(0, nil), false, "", "busy"},
		{"custom EQ gone (Smart, film page)", errEQRoute, "bass_next", mode(2, nil), true, eqDeferredProblem, ""},
		{"custom EQ gone, Standard", errEQRoute, "standard", mode(0, nil), true, eqDeferredProblem, ""},
		{"custom EQ active refuses levels", errEQRoute, "bass_next", mode(eqCustomMode, nil), false, eqActiveProblem, "Select Standard"},
		{"custom EQ active allows input", errEQRoute, "input_bluetooth", mode(eqCustomMode, nil), true, eqActiveProblem, ""},
		{"custom EQ active allows Standard", errEQRoute, "standard", mode(eqCustomMode, nil), true, eqActiveProblem, ""},
		{"unreadable mode refuses levels", errEQRoute, "atmos_next", mode(0, unreadable), false, eqActiveProblem, "Reset in Front EQ"},
		{"unreadable mode allows input", errEQRoute, "input_tv", mode(0, unreadable), true, eqActiveProblem, ""},
	}
	for _, c := range cases {
		proceed, problem, err := eqPendingDecision(c.reset, c.action, c.mode)
		if proceed != c.proceed || problem != c.problem || (c.errorHas == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.errorHas)) {
			t.Fatalf("%s: proceed=%v problem=%q err=%v", c.name, proceed, problem, err)
		}
	}
}

func TestEQPendingProblemForTheMonitor(t *testing.T) {
	if got := eqPendingProblem(errEQRoute, func() (uint32, error) { return 2, nil }); got != eqDeferredProblem {
		t.Fatal(got)
	}
	if got := eqPendingProblem(errEQRoute, func() (uint32, error) { return eqCustomMode, nil }); got != eqActiveProblem {
		t.Fatal(got)
	}
	if got := eqPendingProblem(errors.New("x"), nil); got != "EQ reset pending: x" {
		t.Fatal(got)
	}
}

func TestEQResetBeforeKeepsItsExceptions(t *testing.T) {
	// Volume, mute, media and power from standby never wait for an EQ reset.
	for _, action := range []string{"volume_up", "volume_down", "mute", "media_play"} {
		if eqResetBefore(action, true) {
			t.Fatal(action)
		}
	}
	if eqResetBefore("power", false) || !eqResetBefore("power", true) || !eqResetBefore("bass_next", true) || !eqResetBefore("input_hdmi", true) {
		t.Fatal("reset policy changed")
	}
}
