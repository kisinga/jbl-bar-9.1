package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func standardWake() wakeSample {
	s := readyWake()
	s.mode = 0
	return s
}

// settleFor feeds the same sample n times and reports the kept mode.
func settleFor(w *wakeTracker, s wakeSample, n int) (uint32, bool) {
	var mode uint32
	kept := false
	for i := 0; i < n; i++ {
		if m, ok := w.settle(s); ok {
			mode, kept = m, true
		}
	}
	return mode, kept
}

func TestSmartMemoryKeepsSmartWithoutAGesture(t *testing.T) {
	var w wakeTracker
	s := readyWake() // Smart, as the bar comes back after wake
	for i := 1; i <= 3; i++ {
		status, apply := w.observeFor(s, true)
		if apply {
			t.Fatalf("sample%d: a gesture was requested while Smart is remembered", i)
		}
		if i == 3 && (status != "SMART_KEPT_THIS_WAKE" || !w.handled) {
			t.Fatalf("wake not completed: %s handled=%v", status, w.handled)
		}
	}
	if status, apply := w.observeFor(s, true); apply || status != "SMART_KEPT_THIS_WAKE" {
		t.Fatal("kept wake replayed")
	}
}

func TestSmartMemoryAppliesStandardWhenRemembered(t *testing.T) {
	var w wakeTracker
	s := readyWake()
	for i := 1; i <= 3; i++ {
		if _, apply := w.observeFor(s, false); apply != (i == 3) {
			t.Fatalf("sample%d: apply=%v", i, apply)
		}
	}
}

func TestSmartMemoryIgnoresTheBarsPostWakeSmart(t *testing.T) {
	var w wakeTracker
	s := readyWake()
	// Before the wake is handled, Smart samples are the bar's own default.
	for i := 0; i < 10; i++ {
		w.observeFor(s, false)
		if _, kept := w.settle(s); kept {
			t.Fatal("post-wake Smart taken for a choice before the wake was handled")
		}
	}
}

func TestSmartMemoryRecordsAChoiceKeptAfterTheWake(t *testing.T) {
	var w wakeTracker
	s := readyWake()
	for i := 0; i < 3; i++ {
		w.observeFor(s, false)
	}
	w.handled = true // Standard applied
	std := standardWake()
	if mode, kept := settleFor(&w, std, settleSamples); !kept || mode != 0 {
		t.Fatalf("Standard after the gesture not settled: %d %v", mode, kept)
	}
	// The owner selects Smart: it must stay settleSamples before being kept.
	if _, kept := settleFor(&w, s, settleSamples-1); kept {
		t.Fatal("a short Smart was remembered")
	}
	if mode, kept := w.settle(s); !kept || mode != 3 {
		t.Fatalf("kept Smart not reported: %d %v", mode, kept)
	}
	if _, kept := settleFor(&w, s, 10); kept {
		t.Fatal("the same kept mode was reported again")
	}
}

func TestSmartMemoryRecordsNothingOnAFailedWakeOrInStandby(t *testing.T) {
	var w wakeTracker
	s := readyWake()
	for i := 0; i < 3; i++ {
		w.observeFor(s, false)
	}
	w.failed = true
	if _, kept := settleFor(&w, s, 10); kept {
		t.Fatal("a failed wake recorded the mode")
	}
	w.failed, w.handled = false, true
	asleep := s
	asleep.power = 0
	if _, kept := settleFor(&w, asleep, 10); kept {
		t.Fatal("mode recorded in standby")
	}
	busy := s
	busy.idle = false
	if _, kept := settleFor(&w, busy, 10); kept {
		t.Fatal("mode recorded while keys were busy")
	}
	other := s
	other.identity = "native:2"
	if _, kept := settleFor(&w, other, 10); kept {
		t.Fatal("mode recorded across a firmware process change")
	}
	// A new wake resets the record: nothing until it is handled again.
	w.observeFor(asleep, false)
	if _, kept := settleFor(&w, s, 10); kept {
		t.Fatal("mode recorded before the new wake was handled")
	}
}

func TestSmartMemoryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "smart-mode.json")
	if got := loadSmartMemory(path); got != "Standard" {
		t.Fatalf("missing memory: %s", got)
	}
	if e := saveSmartMemory(path, "Smart"); e != nil {
		t.Fatal(e)
	}
	if got := loadSmartMemory(path); got != "Smart" {
		t.Fatalf("saved Smart: %s", got)
	}
	if info, e := os.Stat(path); e != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("memory file mode: %v %v", info, e)
	}
	if e := saveSmartMemory(path, "Standard"); e != nil || loadSmartMemory(path) != "Standard" {
		t.Fatal("saved Standard", e)
	}
	if _, e := os.Stat(path + ".new"); !os.IsNotExist(e) {
		t.Fatal("temporary file left behind")
	}
	if saveSmartMemory(path, "Movie") == nil {
		t.Fatal("unknown mode saved")
	}
	os.WriteFile(path, []byte("garbage"), 0600)
	if loadSmartMemory(path) != "Standard" {
		t.Fatal("unknown content not treated as Standard")
	}
	link := filepath.Join(t.TempDir(), "smart-mode.json")
	os.Symlink(filepath.Join(t.TempDir(), "elsewhere"), link+".new")
	if e := saveSmartMemory(link, "Smart"); e != nil {
		t.Fatalf("stale temporary link not replaced: %v", e)
	}
}

func TestSmartMemoryNeedsAutoStandard(t *testing.T) {
	_, e := newRemote(config{Listen: ":8080", AllowedCIDR: "192.168.50.0/24", SmartModeMemory: true})
	if e == nil || !strings.Contains(e.Error(), "requires auto_standard") {
		t.Fatalf("smart_mode_memory accepted without auto_standard: %v", e)
	}
	if _, e = newRemote(config{Listen: ":8080", AllowedCIDR: "192.168.50.0/24", AutoStandard: true, SmartModeMemory: true}); e != nil {
		t.Fatal(e)
	}
}
