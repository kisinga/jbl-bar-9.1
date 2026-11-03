package main

import (
	"errors"
	"math"
)

var bassStockDB = [5]int{-15, -10, -5, -1, 3}

// Derive from the native level every time. Never subtract from the last trim.
func bassPlan(level uint32, cached int32, gain float32, offset float64) (float32, bool, error) {
	if level > 4 || math.IsNaN(offset) || math.IsInf(offset, 0) || offset < -3 || offset > 3 || offset*2 != math.Trunc(offset*2) {
		return 0, false, errors.New("invalid bass trim setting")
	}
	stock := bassStockDB[level]
	if cached != int32(stock) {
		return 0, false, errors.New("native bass update pending or unsupported tuning")
	}
	amplitude := func(db float64) float32 { return float32(math.Pow(10, db/20)) }
	close := func(a, b float32) bool { return math.Abs(float64(a-b)) <= math.Max(1, math.Abs(float64(b)))*0.000001 }
	target := amplitude(float64(stock) + offset)
	if close(gain, target) {
		return target, false, nil
	}
	// Recognize only the supported saved offsets; preserve unknown tuning.
	for cut := 3.0; cut >= -3; cut -= 0.5 {
		if close(gain, amplitude(float64(stock)+cut)) {
			return target, true, nil
		}
	}
	return 0, false, errors.New("unrecognized bass amplitude preserved")
}
