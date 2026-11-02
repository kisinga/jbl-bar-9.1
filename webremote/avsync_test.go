package main

import "testing"

func TestAVSyncPlan(t *testing.T) {
	for _, c := range []struct {
		current, target, steps int
		key                    string
		ok                     bool
	}{
		{0, 20, 2, "0x6f000", true},
		{20, 0, 2, "0x6f001", true},
		{30, 30, 0, "0x6f000", true},
		{0, 80, 8, "0x6f000", true},
		{0, 90, 0, "", false},  // more than avSyncMaxSteps per request
		{0, 15, 0, "", false},  // not a 10 ms step
		{0, -10, 0, "", false}, // negative
		{0, 260, 0, "", false}, // above the one-byte maximum
		{5, 10, 0, "", false},  // unexpected current value
	} {
		key, steps, err := avSyncPlan(c.current, c.target)
		if (err == nil) != c.ok || err == nil && (steps != c.steps || key != c.key) {
			t.Fatalf("plan(%d,%d) = %q %d %v", c.current, c.target, key, steps, err)
		}
	}
}

func TestSmartObserved(t *testing.T) {
	before := state{Available: true, Mode: "Standard"}
	after := state{Available: true, Mode: "Smart"}
	if !actionObserved("smart", before, after) || actionObserved("smart", before, before) {
		t.Fatal("smart must be observed only when the mode becomes Smart")
	}
}
