package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type eqMemoryFixture struct {
	values    map[uint32][]byte
	hpp, hdsp uint32
}

func eqFixture() *eqMemoryFixture {
	f := &eqMemoryFixture{map[uint32][]byte{}, 0x100000, 0x200000}
	for a, v := range map[uint32]uint32{eqSpeakerBase + 0x40: 1, eqSpeakerBase + 0x44: 0, eqSpeakerBase + 0x28: 0x500000, 0x500000 + 0xd8: 1, 0x500000 + 0xdc: 0x510000, 0x520000: 0x530000, 0x530000 + 0x1c: 0x540000, 0x530000 + 0x38: 0, 0x540000: 0x550000, 0x540000 + 0x28: 44100, 0x550000 + 0x1108: 0, 0x100000 + 0xa45c: 0x540000} {
		f.word(a, v)
	}
	f.values[eqSpeakerBase+5] = []byte{2}
	entries := make([]byte, 0x1b4)
	binary.LittleEndian.PutUint32(entries, eqHPPElement)
	binary.LittleEndian.PutUint32(entries[0x14:], 0x520000)
	f.values[0x510000] = entries
	f.values[f.hdsp+0x1df680] = bytes.Repeat([]byte{0x42}, eqBlockSize)
	return f
}
func (f *eqMemoryFixture) word(a, v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	f.values[a] = b
}
func (f *eqMemoryFixture) read(a uint32, n int) ([]byte, error) {
	b, ok := f.values[a]
	if !ok || len(b) != n {
		return nil, errors.New("invalid test read")
	}
	return append([]byte(nil), b...), nil
}
func TestEQNativeStateAndMode5Readback(t *testing.T) {
	for _, mode := range []uint32{0, 1, 5} {
		f := eqFixture()
		f.word(0x550000+0x1108, mode)
		s, e := eqReadState(f.read, f.hpp, f.hdsp)
		if e != nil || s.Mode != mode || s.Cache != 0 || s.Element != eqHPPElement || s.SampleRate != 44100 || len(s.Block) != eqBlockSize {
			t.Fatalf("mode%d: %+v %v", mode, s, e)
		}
	}
}
func TestEQNativeRejectsUnsafeAndChangingRoutes(t *testing.T) {
	cases := map[string]func(*eqMemoryFixture){
		"not playing":            func(f *eqMemoryFixture) { f.word(eqSpeakerBase+0x40, 0) },
		"pending parameter":      func(f *eqMemoryFixture) { f.word(eqSpeakerBase+0x44, 1) },
		"PPC disabled":           func(f *eqMemoryFixture) { f.values[eqSpeakerBase+5] = []byte{0} },
		"unbounded pool":         func(f *eqMemoryFixture) { f.word(0x500000+0xd8, 17) },
		"unvalidated page":       func(f *eqMemoryFixture) { binary.LittleEndian.PutUint32(f.values[0x510000], 0x840001) },
		"no HPP":                 func(f *eqMemoryFixture) { binary.LittleEndian.PutUint32(f.values[0x510000], 0x860000) },
		"different HPP instance": func(f *eqMemoryFixture) { f.word(0x530000+0x1c, 0x540004) },
		"unknown cache":          func(f *eqMemoryFixture) { f.word(0x530000+0x38, 5) },
		"unknown mode":           func(f *eqMemoryFixture) { f.word(0x550000+0x1108, 2) },
		"unvalidated format":     func(f *eqMemoryFixture) { f.word(0x540000+0x28, 96000) },
		"null context":           func(f *eqMemoryFixture) { f.word(0x520000, 0) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := eqFixture()
			mutate(f)
			if _, e := eqReadState(f.read, f.hpp, f.hdsp); e == nil {
				t.Fatal("unsafe native state accepted")
			}
		})
	}
	for _, change := range []string{"block", "handle", "mode"} {
		t.Run("changed "+change, func(t *testing.T) {
			f := eqFixture()
			reads := 0
			read := func(a uint32, n int) ([]byte, error) {
				b, e := f.read(a, n)
				if a == f.hdsp+0x1df680 {
					reads++
					if reads == 2 {
						switch change {
						case "block":
							b[0] ^= 1
						case "handle":
							f.word(eqSpeakerBase+0x28, 0x500004)
						case "mode":
							f.word(0x550000+0x1108, 5)
						}
					}
				}
				return b, e
			}
			if _, e := eqReadState(read, f.hpp, f.hdsp); e == nil {
				t.Fatal("changing route accepted")
			}
		})
	}
}
func TestEQMappingsRequirePinnedReadableWritableBounds(t *testing.T) {
	maps, e := eqParseMaps("00100000-00103000 r-xp 00000000 00:00 1 /system/usr/local/lib/libHPP.so\n00103000-00104000 rw-p 00003000 00:00 1 /system/usr/local/lib/libHPP.so\n00500000-00600000 rw-p 00000000 00:00 0 [heap]")
	if e != nil {
		t.Fatal(e)
	}
	if b, e := eqLibraryBase(maps, "/system/usr/local/lib/libHPP.so"); e != nil || b != 0x100000 {
		t.Fatal(b, e)
	}
	if !eqMapped(maps, 0x500000, 4, true) || eqMapped(maps, 0x100000, 4, true) || eqMapped(maps, 0x5fffff, 4, true) || eqMapped(maps, ^uint32(0)-1, 4, true) || eqMapped(maps, 0x500000, 0, true) {
		t.Fatal("memory boundary guard failed")
	}
	if _, e := eqLibraryBase(append(maps, maps[0]), "/system/usr/local/lib/libHPP.so"); e == nil {
		t.Fatal("duplicate mapping accepted")
	}
	for _, input := range []string{"", "junk", "00100000-100000000 rw-p 0 00:00 0", "00103000-00100000 rw-p 0 00:00 0"} {
		if _, e := eqParseMaps(input); e == nil {
			t.Fatal("invalid map accepted", input)
		}
	}
}
func TestEQResetCommandSequenceHasNoVariableNativeEndpoint(t *testing.T) {
	for _, mode := range []uint32{0, 1} {
		got, e := eqPresetCommands(mode)
		if e != nil || len(got) != 3 || !reflect.DeepEqual(got[0], []string{"mw.aud.se.ppc.infoset", "element", "0x840000", "0x840001", "1"}) || !reflect.DeepEqual(got[2], []string{"mw.aud.se.ppc.infoset", "elend"}) || strings.Join(got[1], " ") != "mw.aud.se.ppc.infoset epara "+string(rune('0'+mode)) {
			t.Fatal(got, e)
		}
	}
	if _, e := eqPresetCommands(5); e == nil {
		t.Fatal("custom mode allowed through reset endpoint")
	}
}
