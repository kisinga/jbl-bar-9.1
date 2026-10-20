package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

// Synthetic tuning only: no owner calibration or vendor block is committed.
func syntheticEQ() ([]byte, []byte, []eqEntry) {
	block := make([]byte, eqBlockSize)
	registry := []eqEntry{{Effect: "EQ", Name: "LR EQ", Version: 1, Offset: 0x30, Count: 5}}
	at := 0x100
	for i := 1; i < 22; i++ {
		count := 8
		if i < 4 {
			count = 5
		}
		for {
			overlaps := false
			for _, reserved := range []int{0x9ac, 0x9c0, 0xdac} {
				if at <= reserved && reserved < at+count*16 {
					at = reserved + 4
					overlaps = true
					break
				}
			}
			if !overlaps {
				break
			}
		}
		registry = append(registry, eqEntry{Effect: "EQ", Name: fmt.Sprintf("Synthetic %d", i), Version: 1, Offset: at, Count: count})
		at += count * 16
	}
	for _, entry := range registry {
		for i := 0; i < entry.Count; i++ {
			offset := entry.Offset + i*16
			eqPutFloat(block, offset, 1000)
			eqPutFloat(block, offset+8, 0.707)
		}
	}
	eqPutFloat(block, 0x30, 8000)
	eqPutFloat(block, 0x34, 2)
	eqPutFloat(block, 0x3c, 16)
	eqPutFloat(block, 0x40, 4000)
	eqPutFloat(block, 0x44, 2)
	eqPutFloat(block, 0x4c, 16)
	return block, append([]byte(nil), block...), registry
}

func TestFrontEQNeutralPreservesSyntheticSnapshot(t *testing.T) {
	block, defaults, registry := syntheticEQ()
	before := append([]byte(nil), block...)
	data, expected, err := generateFrontEQ(block, defaults, registry, [5]float64{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, block) || !bytes.Equal(block, before) {
		t.Fatal("neutral changed tuning or input")
	}
	var recipe eqRecipe
	if err = json.Unmarshal(data, &recipe); err != nil {
		t.Fatal(err)
	}
	if len(recipe.Chain) != 22 {
		t.Fatal("recipe omits native EQ groups")
	}
	count := 0
	for _, group := range recipe.Chain {
		count += len(group.Band)
	}
	if count != 164 {
		t.Fatal("recipe omits native EQ bands")
	}
	if bytes.Contains(data, []byte(`"f":`)) || !bytes.Contains(data, []byte(`"fc":`)) {
		t.Fatal("incorrect native frequency key")
	}
}

func TestFrontEQCutChangesOnlyItsSlot(t *testing.T) {
	for band, slot := range []int{2, 3, 4, 1, 0} {
		block, defaults, registry := syntheticEQ()
		gains := [5]float64{}
		gains[band] = -1.5
		data, expected, err := generateFrontEQ(block, defaults, registry, gains)
		if err != nil {
			t.Fatal(err)
		}
		for at := range block {
			if block[at] != expected[at] && (at < 0x30+slot*16 || at >= 0x40+slot*16) {
				t.Fatalf("band%d modified byte0x%x", band, at)
			}
		}
		at := 0x30 + slot*16
		if slot < 2 {
			if eqFloat(expected, at+4) != 0.5 || eqFloat(expected, at+12) != 16 {
				t.Fatal("factory shelf replaced")
			}
		} else if eqFloat(expected, at+4) != -1.5 || eqFloat(expected, at+12) != 11 {
			t.Fatal("bell not activated")
		}
		if len(data) == 0 {
			t.Fatal("missing recipe")
		}
		_, restored, err := generateFrontEQ(block, defaults, registry, [5]float64{})
		if err != nil || !bytes.Equal(restored, block) {
			t.Fatal("neutral did not restore exact original tuple")
		}
	}
}

func TestFrontEQRejectsInvalidLevels(t *testing.T) {
	block, defaults, registry := syntheticEQ()
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0.5, -6.5, -0.1} {
		if _, _, err := generateFrontEQ(block, defaults, registry, [5]float64{bad}); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	for _, good := range []float64{0, -0.5, -6} {
		if _, _, err := generateFrontEQ(block, defaults, registry, [5]float64{good}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFrontEQRejectsUnknownTuningAndMalformedRegistry(t *testing.T) {
	block, defaults, registry := syntheticEQ()
	eqPutFloat(block, 0, 1)
	if _, _, err := generateFrontEQ(block, defaults, registry, [5]float64{}); err == nil || !strings.Contains(err.Error(), "unmapped") {
		t.Fatal("accepted unmapped runtime deviation")
	}
	block, defaults, registry = syntheticEQ()
	for _, mutate := range []func([]eqEntry){
		func(r []eqEntry) { r[0].Offset = 0x40 },
		func(r []eqEntry) { r[1].Name = "LR EQ" },
		func(r []eqEntry) { r[1].Offset = 0x30 },
		func(r []eqEntry) { r[1].Offset = -4 },
		func(r []eqEntry) { r[1].Count = 9 },
		func(r []eqEntry) { r[1].Version = 2 },
		func(r []eqEntry) { r[1].Offset = eqBlockSize - 4 },
	} {
		copyRegistry := append([]eqEntry(nil), registry...)
		mutate(copyRegistry)
		if _, _, err := generateFrontEQ(block, defaults, copyRegistry, [5]float64{}); err == nil {
			t.Fatal("accepted malformed registry")
		}
	}
	if _, _, err := generateFrontEQ(block[:3], defaults, registry, [5]float64{}); err == nil {
		t.Fatal("accepted truncated block")
	}
	if _, _, err := generateFrontEQ(block, defaults, registry[:21], [5]float64{}); err == nil {
		t.Fatal("accepted incomplete registry")
	}
}

func TestFrontEQRejectsUsedSlotsAndChangedShelves(t *testing.T) {
	block, defaults, registry := syntheticEQ()
	eqPutFloat(block, 0x5c, 11)
	if _, _, err := generateFrontEQ(block, defaults, registry, [5]float64{-1}); err == nil {
		t.Fatal("overwrote an occupied filter slot")
	}
	block, defaults, registry = syntheticEQ()
	eqPutFloat(block, 0x30, 7000)
	if _, _, err := generateFrontEQ(block, defaults, registry, [5]float64{0, 0, 0, 0, -1}); err == nil {
		t.Fatal("altered an unrecognized factory shelf")
	}
}

func TestFrontEQMappedGainRecipeAndFiniteGuards(t *testing.T) {
	block, defaults, registry := syntheticEQ()
	eqPutFloat(block, 0x9c0, 1)
	data, _, err := generateFrontEQ(block, defaults, registry, [5]float64{})
	if err != nil {
		t.Fatal(err)
	}
	var recipe eqRecipe
	if err = json.Unmarshal(data, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Chain[0].Name != "Ls/Rs Gain" || len(recipe.Chain[0].Value) != 1 || recipe.Chain[0].Value[0] != 0 {
		t.Fatal("mapped gain omitted or neighboring gains overwritten")
	}
	eqPutFloat(block, 0x9c0, -1)
	if _, _, err = generateFrontEQ(block, defaults, registry, [5]float64{}); err == nil {
		t.Fatal("accepted invalid native amplitude")
	}
	block, defaults, registry = syntheticEQ()
	eqPutFloat(block, 0x30, math.NaN())
	if _, _, err = generateFrontEQ(block, defaults, registry, [5]float64{}); err == nil {
		t.Fatal("accepted nonfinite native tuple")
	}
}
