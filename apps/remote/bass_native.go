package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
)

// Callers hold both actionMu and d.mu. Firmware controls are external to these
// locks: repeated route/block checks reduce, but cannot eliminate, that race.
type bassReading struct {
	Native        eqNativeState
	Level, Source uint32
	Cached        int32
	Gain          float32
	PID           int
	Start         string
}

func bassRead(d *device) (bassReading, error) {
	var s bassReading
	if e := d.identify(); e != nil {
		return s, e
	}
	for _, p := range []struct {
		a int64
		n int
		v uint32
	}{{12962676, 4, 0}, {14191893, 1, 1}, {13378532, 4, 0}} {
		v, e := d.read(p.a, p.n)
		if e != nil || v != p.v {
			return s, errors.New("WAITING_FOR_STANDARD_PLAYBACK")
		}
	}
	message, _, displayErr := d.displayMessages()
	if displayErr != nil || calibrationRunning(message) {
		return s, errors.New("WAITING_FOR_CALIBRATION")
	}
	// The Front EQ owns the same native block while it is applied; the page
	// says so instead of a generic wait.
	if d.eq.active || d.eq.needsReset {
		return s, errors.New("WAITING_FOR_FRONT_EQ")
	}
	if !d.idleKeys() || controlBusy() {
		return s, errors.New("WAITING_FOR_NATIVE_CONTROLS")
	}
	n, e := d.readAudioNative(true)
	if e != nil {
		return s, errors.New("WAITING_FOR_AUDIO_ROUTE")
	}
	if n.Mode != 0 || n.Cache != 0 || (n.Element != 0x840000 && n.Element != 0x840002) {
		return s, errors.New("WAITING_FOR_SUPPORTED_DSP_MODE")
	}
	level, e := d.configByte(238)
	if e != nil || level > 4 {
		return s, errors.New("invalid native bass level")
	}
	source, e := d.configByte(365)
	if e != nil || source > 7 {
		return s, errors.New("invalid source")
	}
	cached, e := d.read(int64(n.hppContext)+0x40, 4)
	if e != nil {
		return s, e
	}
	if !d.same() {
		return s, errors.New("audio process changed")
	}
	s = bassReading{Native: n, Level: level, Source: source, Cached: int32(cached), Gain: float32(eqFloat(n.Block, 0xdac)), PID: d.pid, Start: d.start}
	return s, nil
}

func bassRouteKey(s bassReading) string {
	return fmt.Sprintf("%d:%s:%x:%x:%x:%x:%d:%d", s.PID, s.Start, s.Native.outer, s.Native.instance, s.Native.hppContext, s.Native.Element, s.Native.SampleRate, s.Source)
}

func bassKey(s bassReading) string {
	block := append([]byte(nil), s.Native.Block...)
	clear(block[0xdac:0xdb0])
	return fmt.Sprintf("%d:%s:%x:%x:%x:%x:%d:%d:%d:%d:%d:%s", s.PID, s.Start, s.Native.outer, s.Native.instance, s.Native.hppContext, s.Native.Element, s.Native.SampleRate, s.Level, s.Source, s.Cached, s.Native.Mode, eqDigest(block))
}

func bassWrite(d *device, before bassReading, target float32) error {
	now, e := bassRead(d)
	if e != nil {
		return e
	}
	if bassKey(now) != bassKey(before) || !bytes.Equal(now.Native.Block, before.Native.Block) {
		return errors.New("audio changed before bass write")
	}
	data, e := os.ReadFile(fmt.Sprintf("/proc/%d/maps", d.pid))
	if e != nil {
		return e
	}
	maps, e := eqParseMaps(string(data))
	if e != nil {
		return e
	}
	base, e := eqLibraryBase(maps, "/system/usr/lib/libhdsp.so")
	if e != nil {
		return e
	}
	address := base + 0x1df680 + 0xdac
	if !eqMapped(maps, address, 4, true) {
		return errors.New("bass word not in pinned writable mapping")
	}
	expected, e := bassTargetBlock(before.Native.Block, target)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(fmt.Sprintf("/proc/%d/mem", d.pid), os.O_RDWR, 0)
	if e != nil {
		return e
	}
	defer f.Close()
	// Last identity/readback check before the single aligned data-word write.
	check, e := bassRead(d)
	if e != nil {
		return e
	}
	if bassKey(check) != bassKey(before) || !bytes.Equal(check.Native.Block, before.Native.Block) {
		return errors.New("audio changed before write")
	}
	n, e := f.WriteAt(expected[0xdac:0xdb0], int64(address))
	if e != nil || n != 4 {
		return fmt.Errorf("bass write failed: %d, %v", n, e)
	}
	after, e := bassRead(d)
	if e != nil {
		return e
	}
	if bassKey(after) != bassKey(before) || !bytes.Equal(after.Native.Block, expected) {
		return errors.New("bass readback changed unexpectedly; automatic writes paused")
	}
	return nil
}

func bassTargetBlock(block []byte, target float32) ([]byte, error) {
	if len(block) != eqBlockSize || target <= 0 || math.IsNaN(float64(target)) || math.IsInf(float64(target), 0) {
		return nil, errors.New("invalid bass amplitude block")
	}
	expected := append([]byte(nil), block...)
	binary.LittleEndian.PutUint32(expected[0xdac:0xdb0], math.Float32bits(target))
	return expected, nil
}
