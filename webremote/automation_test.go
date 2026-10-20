package main

import (
	"os"
	"path/filepath"
	"testing"
)

func readyWake() wakeSample {
	return wakeSample{identity: "native:1", power: 1, system: 0, mode: 3, source: 2, idle: true}
}
func TestWakeTrackerStableOncePerWake(t *testing.T) {
	var w wakeTracker
	s := readyWake()
	for i := 1; i <= 3; i++ {
		_, apply := w.observe(s)
		if apply != (i == 3) {
			t.Fatalf("sample%d: apply=%v", i, apply)
		}
	}
	w.handled = true
	if _, apply := w.observe(s); apply {
		t.Fatal("manual Smart selection was overridden in same wake")
	}
	asleep := s
	asleep.system = 2
	w.observe(asleep)
	for i := 1; i <= 3; i++ {
		_, apply := w.observe(s)
		if apply != (i == 3) {
			t.Fatal("wake did not rearm")
		}
	}
}
func TestWakeTrackerHandlesLateInitializationAndSourceChanges(t *testing.T) {
	var w wakeTracker
	s := readyWake()
	s.mode = 0
	w.observe(s)
	w.observe(s)
	if w.handled {
		t.Fatal("transient Standard before native initialization marked wake complete")
	}
	s.mode = 3
	for i := 1; i <= 2; i++ {
		if _, apply := w.observe(s); apply {
			t.Fatal("mode transition was not stabilized")
		}
	}
	s.source = 3
	if _, apply := w.observe(s); apply {
		t.Fatal("source change reused old stable samples")
	}
	w.observe(s)
	if _, apply := w.observe(s); !apply {
		t.Fatal("stable changed source was never selected")
	}
	w.handled = true
	s.identity = "native:2"
	if _, apply := w.observe(s); apply || w.handled {
		t.Fatal("new process retained prior wake completion")
	}
}
func TestWakeTrackerNeverRetriesFailedGesture(t *testing.T) {
	var w wakeTracker
	s := readyWake()
	w.observe(s)
	w.failed = true
	for i := 0; i < 100; i++ {
		if _, apply := w.observe(s); apply {
			t.Fatal("blind key retry")
		}
	}
	s.mode = 0
	w.observe(s)
	w.observe(s)
	w.observe(s)
	if status, apply := w.observe(s); status != "AUTOMATION_FAILED" || apply {
		t.Fatal("gesture failure was hidden before the next wake")
	}
	s.system = 2
	w.observe(s)
	s.system = 0
	s.mode = 3
	w.observe(s)
	w.observe(s)
	if _, apply := w.observe(s); !apply {
		t.Fatal("new wake did not clear failure")
	}
}
func TestWakeTrackerWaitsIndefinitelyAndNeverActsOnBusyKeys(t *testing.T) {
	var w wakeTracker
	s := readyWake()
	s.system = 2
	for i := 0; i < 10000; i++ {
		if _, apply := w.observe(s); apply {
			t.Fatal("action during standby")
		}
	}
	s.system = 0
	s.idle = false
	for i := 0; i < 10; i++ {
		if _, apply := w.observe(s); apply {
			t.Fatal("action during held keys")
		}
	}
	s.idle = true
	w.observe(s)
	w.observe(s)
	if _, apply := w.observe(s); !apply {
		t.Fatal("late wake timed out")
	}
}
func legacyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join("..", "smart-mode", "JBLInstall", "payload")
	entries, e := os.ReadDir(src)
	if e != nil {
		t.Fatal(e)
	}
	for _, item := range entries {
		b, e := os.ReadFile(filepath.Join(src, item.Name()))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, item.Name()), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	return dir
}
func TestLegacyOwnershipRestoresOnlyItsOwnDisableMarker(t *testing.T) {
	dir := legacyFixture(t)
	p := filepath.Join(dir, "DISABLED")
	if e := claimLegacySmart(dir); e != nil {
		t.Fatal(e)
	}
	if e := claimLegacySmart(dir); e != nil {
		t.Fatal("crash restart could not reclaim owned marker", e)
	}
	if released, e := releaseLegacySmart(dir); e != nil || !released {
		t.Fatal(released, e)
	}
	if _, e := os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("owned disable marker retained")
	}
	if e := os.WriteFile(p, []byte("MANUALLY_DISABLED\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := claimLegacySmart(dir); e == nil {
		t.Fatal("user disabled helper was taken over")
	}
	if released, e := releaseLegacySmart(dir); e != nil || released {
		t.Fatal("user marker was removed", released, e)
	}
}
func TestLegacyOwnershipRejectsTamperingAndSymlinks(t *testing.T) {
	for _, member := range []string{"OWNER.txt", "checksums.txt", "run.sh"} {
		t.Run(member, func(t *testing.T) {
			dir := legacyFixture(t)
			os.WriteFile(filepath.Join(dir, member), []byte("changed"), 0600)
			if e := claimLegacySmart(dir); e == nil {
				t.Fatal("modified helper accepted")
			}
			if _, e := os.Lstat(filepath.Join(dir, "DISABLED")); !os.IsNotExist(e) {
				t.Fatal("marker created on unknown helper")
			}
		})
	}
	dir := legacyFixture(t)
	outside := filepath.Join(t.TempDir(), "marker")
	os.WriteFile(outside, []byte(goSmartMarker), 0600)
	os.Symlink(outside, filepath.Join(dir, "DISABLED"))
	if e := claimLegacySmart(dir); e == nil {
		t.Fatal("symlink marker followed")
	}
	if released, e := releaseLegacySmart(dir); e != nil || released {
		t.Fatal(released, e)
	}
	if b, _ := os.ReadFile(outside); string(b) != goSmartMarker {
		t.Fatal("outside file modified")
	}
	dir = legacyFixture(t)
	run := filepath.Join(dir, "run.sh")
	b, _ := os.ReadFile(run)
	os.WriteFile(outside, b, 0600)
	os.Remove(run)
	os.Symlink(outside, run)
	if e := claimLegacySmart(dir); e == nil {
		t.Fatal("symlink payload accepted")
	}
}
func TestStandardReadbackRejectsVolumeMuteDrift(t *testing.T) {
	vol := uint32(8)
	muted := false
	s := state{Volume: &vol, Muted: &muted}
	if !standardSettingsPreserved(s, s) {
		t.Fatal("unchanged rejected")
	}
	other := uint32(9)
	changed := true
	if standardSettingsPreserved(s, state{Volume: &other, Muted: &muted}) || standardSettingsPreserved(s, state{Volume: &vol, Muted: &changed}) || standardSettingsPreserved(s, state{}) {
		t.Fatal("missing/changed settings accepted")
	}
}

func TestWakeTrackerReadGapDoesNotReplayGesture(t *testing.T) {
	for _, failed := range []bool{false, true} {
		var w wakeTracker
		s := readyWake()
		w.observe(s)
		w.failed = failed
		w.handled = !failed
		w.failureReason = "preserved failure"
		w.observe(wakeSample{})
		for i := 0; i < 10; i++ {
			if _, apply := w.observe(s); apply {
				t.Fatal("read gap replayed a handled/failed wake")
			}
		}
		if failed && w.failureReason != "preserved failure" {
			t.Fatal("failure reason lost across read gap")
		}
	}
}

func TestAutomationOwnerLockRejectsDuplicatesAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner")
	first, e := lockSmartAutomation(path)
	if e != nil {
		t.Fatal(e)
	}
	if second, e := lockSmartAutomation(path); e == nil {
		second.Close()
		t.Fatal("second automation owner accepted")
	}
	first.Close()
	next, e := lockSmartAutomation(path)
	if e != nil {
		t.Fatal("lock survived owner close", e)
	}
	next.Close()
	link := filepath.Join(t.TempDir(), "symlink")
	os.Symlink(path, link)
	if f, e := lockSmartAutomation(link); e == nil {
		f.Close()
		t.Fatal("symlink lock followed")
	}
}
