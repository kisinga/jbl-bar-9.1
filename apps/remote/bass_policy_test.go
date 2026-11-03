package main

import (
	"math"
	"testing"
)

func TestBassFollowsNativeLevelsWithoutCompounding(t *testing.T) {
	for level, db := range bassStockDB {
		stock := float32(math.Pow(10, float64(db)/20))
		target, write, e := bassPlan(uint32(level), int32(db), stock, -3)
		if e != nil || !write {
			t.Fatalf("level %d: %v", level, e)
		}
		if math.Abs(20*math.Log10(float64(target))-float64(db-3)) > 0.00001 {
			t.Fatal("wrong trim")
		}
		for i := 0; i < 10; i++ {
			next, write, e := bassPlan(uint32(level), int32(db), target, -3)
			if e != nil || write || next != target {
				t.Fatal("trim compounded")
			}
		}
		if _, _, e := bassPlan(uint32(level), int32(db+1), stock, -3); e == nil {
			t.Fatal("accepted stale native cache")
		}
	}
}
func TestBassPreservesUnrecognizedTuning(t *testing.T) {
	for _, gain := range []float32{0, -1, float32(math.NaN()), float32(math.Inf(1)), 0.9} {
		if _, _, e := bassPlan(2, -5, gain, -3); e == nil {
			t.Fatalf("accepted %v", gain)
		}
	}
	for _, offset := range []float64{-3.5, 3.5, -2.4, math.NaN(), math.Inf(1)} {
		if _, _, e := bassPlan(2, -5, 0.56234133, offset); e == nil {
			t.Fatal("accepted offset")
		}
	}
	if _, _, e := bassPlan(5, -5, 0.56234133, -3); e == nil {
		t.Fatal("accepted level")
	}
	if _, write, e := bassPlan(2, -5, float32(math.Pow(10, -7.5/20)), -3); e != nil || !write {
		t.Fatal("failed to adopt the prior midpoint")
	}
}

func TestLiveOffsetChangesAndOriginal(t *testing.T) {
	gain := float32(math.Pow(10, -8.0/20))
	for _, offset := range []float64{-2.5, 3, 0, -3} {
		target, write, e := bassPlan(2, -5, gain, offset)
		if e != nil || !write {
			t.Fatalf("transition to %v failed: %v", offset, e)
		}
		if math.Abs(20*math.Log10(float64(target))-(-5+offset)) > 0.00001 {
			t.Fatal("incorrect amplitude")
		}
		gain = target
	}
}
