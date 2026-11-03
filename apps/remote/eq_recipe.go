package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

const eqBlockSize = 3840

type eqEntry struct {
	Effect  string `json:"effect"`
	Name    string `json:"name"`
	Version int    `json:"version"`
	Offset  int    `json:"offset"`
	Count   int    `json:"count"`
}

type eqBand struct {
	Frequency float64 `json:"fc"`
	Gain      float64 `json:"gain"`
	Q         float64 `json:"Q"`
	Type      float64 `json:"type"`
	Enable    bool    `json:"enable"`
}

type eqRecipeEntry struct {
	Effect  string    `json:"effect"`
	Name    string    `json:"name"`
	Version int       `json:"version"`
	Enable  *bool     `json:"enable,omitempty"`
	Band    []eqBand  `json:"band,omitempty"`
	Value   []float64 `json:"value,omitempty"`
}

type eqRecipe struct {
	Chain []eqRecipeEntry `json:"process_chain"`
}

func eqFloat(block []byte, at int) float64 {
	return float64(math.Float32frombits(binary.LittleEndian.Uint32(block[at : at+4])))
}

func eqPutFloat(block []byte, at int, value float64) {
	binary.LittleEndian.PutUint32(block[at:at+4], math.Float32bits(float32(value)))
}

func eqFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

// generateFrontEQ creates a complete snapshot-preserving native recipe. The
// caller MUST parse the neutral recipe through the hash-pinned native parser
// and compare all 3840 output bytes to block before any live application. It
// must likewise compare each requested recipe's native output to expected.
// This function neither applies EQ nor proves mode/routing preservation.
func generateFrontEQ(block, defaults []byte, registry []eqEntry, gains [5]float64) (recipeJSON, expected []byte, err error) {
	if len(block) != eqBlockSize || len(defaults) != eqBlockSize {
		return nil, nil, errors.New("invalid EQ parameter block size")
	}
	for _, gain := range gains {
		if !eqFinite(gain) || gain < -6 || gain > 0 || gain*2 != math.Trunc(gain*2) {
			return nil, nil, errors.New("Front EQ accepts -6 to 0 dB in 0.5 dB steps")
		}
	}
	type gainField struct {
		offset  int
		name    string
		knownDB float64
	}
	fields := []gainField{{0x9ac, "High Speaker Gain", -3}, {0x9c0, "Ls/Rs Gain", 0}, {0xdac, "Gain8", -5}}
	covered := make([]bool, eqBlockSize/4)
	var recipe eqRecipe
	// Only these three non-default, non-EQ fields have established conversions.
	// Any other runtime deviation fails closed rather than reverting to defaults.
	for _, field := range fields {
		covered[field.offset/4] = true
		if binary.LittleEndian.Uint32(block[field.offset:]) == binary.LittleEndian.Uint32(defaults[field.offset:]) {
			continue
		}
		amplitude := eqFloat(block, field.offset)
		if !eqFinite(amplitude) || amplitude <= 0 {
			return nil, nil, errors.New("unsupported native gain value")
		}
		db := 20 * math.Log10(amplitude)
		if math.Abs(db-field.knownDB) < 0.00001 {
			db = field.knownDB
		}
		// Native float32 conversion, not approximate Go arithmetic, must prove the
		// original amplitude is reconstructed. Caller performs that round-trip.
		recipe.Chain = append(recipe.Chain, eqRecipeEntry{Effect: "gains", Name: field.name, Version: 1, Value: []float64{db}})
	}
	names := map[string]bool{}
	eqGroups, bands, front := 0, 0, -1
	enabled := true
	for _, entry := range registry {
		if entry.Effect != "EQ" {
			continue
		}
		if entry.Version != 1 || entry.Count < 1 || entry.Count > 8 || entry.Offset < 0 || entry.Offset%4 != 0 || entry.Offset+entry.Count*16 > eqBlockSize || entry.Name == "" || names[entry.Name] {
			return nil, nil, errors.New("unsupported native EQ registry")
		}
		names[entry.Name] = true
		group := eqRecipeEntry{Effect: "EQ", Name: entry.Name, Version: 1, Enable: &enabled}
		for i := 0; i < entry.Count; i++ {
			at := entry.Offset + i*16
			for word := at / 4; word < at/4+4; word++ {
				if covered[word] {
					return nil, nil, errors.New("overlapping native EQ registry")
				}
				covered[word] = true
			}
			band := eqBand{Frequency: eqFloat(block, at), Gain: eqFloat(block, at+4), Q: eqFloat(block, at+8), Type: eqFloat(block, at+12)}
			if !eqFinite(band.Frequency) || !eqFinite(band.Gain) || !eqFinite(band.Q) || !eqFinite(band.Type) {
				return nil, nil, errors.New("nonfinite native EQ tuple")
			}
			band.Enable = band.Type != 0
			group.Band = append(group.Band, band)
		}
		if entry.Name == "LR EQ" {
			if entry.Offset != 0x30 || entry.Count != 5 {
				return nil, nil, errors.New("unsupported Front EQ layout")
			}
			front = len(recipe.Chain)
		}
		recipe.Chain = append(recipe.Chain, group)
		eqGroups++
		bands += entry.Count
	}
	if eqGroups != 22 || bands != 164 || front < 0 {
		return nil, nil, errors.New("unsupported native EQ registry coverage")
	}
	for at := 0; at < eqBlockSize; at += 4 {
		if !covered[at/4] && binary.LittleEndian.Uint32(block[at:]) != binary.LittleEndian.Uint32(defaults[at:]) {
			return nil, nil, fmt.Errorf("unmapped native tuning field at 0x%x", at)
		}
	}
	expected = append([]byte(nil), block...)
	frequencies := [5]float64{125, 500, 2000, 4000, 8000}
	slots := [5]int{2, 3, 4, 1, 0}
	for i, delta := range gains {
		if delta == 0 {
			continue
		}
		slot := slots[i]
		band := &recipe.Chain[front].Band[slot]
		if slot < 2 {
			if band.Frequency != frequencies[i] || band.Type != 16 {
				return nil, nil, errors.New("factory shelf identity differs")
			}
			band.Gain += delta
		} else {
			if band.Type != 0 {
				return nil, nil, errors.New("Front EQ bell slot already in use")
			}
			band.Frequency = frequencies[i]
			band.Gain = delta
			band.Q = float64(float32(0.707))
			band.Type = 11
			band.Enable = true
		}
		at := 0x30 + slot*16
		eqPutFloat(expected, at, band.Frequency)
		eqPutFloat(expected, at+4, band.Gain)
		eqPutFloat(expected, at+8, band.Q)
		eqPutFloat(expected, at+12, band.Type)
	}
	recipeJSON, err = json.Marshal(recipe)
	if err != nil {
		return nil, nil, err
	}
	return recipeJSON, expected, nil
}

// validateNeutralSnapshot checks that all runtime deviations are mapped and
// returns the neutral recipe. Exact native round-trip verification remains
// mandatory; this name does not imply acoustic or live restoration validation.
func validateNeutralSnapshot(block, defaults []byte, registry []eqEntry) ([]byte, error) {
	recipe, _, err := generateFrontEQ(block, defaults, registry, [5]float64{})
	return recipe, err
}
