package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

func TestCenterCoefficientsMatchQ923(t *testing.T) {
	for i, db := range centerDB {
		want := uint32(math.Round(math.Pow(10, db/20) * (1 << 23)))
		if centerCoefficients[i] != want {
			t.Fatalf("level %d: got %#08x want %#08x", i+1, centerCoefficients[i], want)
		}
	}
	if centerCoefficients[centerDefault] != 0x00800000 {
		t.Fatal("default level must be the firmware's 0 dB coefficient")
	}
}

func TestCenterDescriptorLayout(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	d := centerDescriptor(0x0C, data)
	if d[0] != 0x4C || binary.LittleEndian.Uint16(d[2:]) != 0x0C || binary.LittleEndian.Uint16(d[4:]) != 4 {
		t.Fatalf("address/register/length: % x", d[:8])
	}
	if binary.LittleEndian.Uint32(d[8:]) != uint32(uintptr(unsafe.Pointer(&data[0]))) {
		t.Fatal("data pointer")
	}
	if binary.LittleEndian.Uint32(d[0x0c:]) != 0 || binary.LittleEndian.Uint32(d[0x18:]) != 0x32 ||
		d[0x1c] != 0x63 || d[0x1d] != 0x62 || binary.LittleEndian.Uint32(d[0x20:]) != 1 {
		t.Fatalf("amplifier bus fields: % x", d)
	}
}

type centerStep struct {
	reg  uint16
	data []byte
}

func fakeCenterBus(t *testing.T, fail uint16) *[]centerStep {
	t.Helper()
	var steps []centerStep
	oldOpen, oldWrite := centerOpen, centerWrite
	centerOpen = func() (*os.File, error) { return os.Open(os.DevNull) }
	centerWrite = func(_ *os.File, reg uint16, data []byte) error {
		steps = append(steps, centerStep{reg, append([]byte(nil), data...)})
		if fail != 0 && reg == fail {
			return os.ErrInvalid
		}
		return nil
	}
	t.Cleanup(func() { centerOpen, centerWrite = oldOpen, oldWrite })
	return &steps
}

func TestCenterGainSequence(t *testing.T) {
	steps := fakeCenterBus(t, 0)
	if e := writeCenterGain(4); e != nil {
		t.Fatal(e)
	}
	coeff := []byte{0x01, 0x82, 0x8D, 0xC7} // level 4 = +9.6 dB
	want := []centerStep{{0x00, []byte{0}}, {0x7F, []byte{0x8C}}, {0x00, []byte{0x0B}}, {0x0C, coeff}, {0x10, coeff}, {0x00, []byte{0}}, {0x7F, []byte{0}}, {0x00, []byte{0}}}
	if !reflect.DeepEqual(*steps, want) {
		t.Fatalf("sequence %v", *steps)
	}
	if writeCenterGain(centerLevels) == nil {
		t.Fatal("out-of-range level accepted")
	}
}

func TestCenterGainRestoresPageAfterFailure(t *testing.T) {
	steps := fakeCenterBus(t, centerRegRight)
	if writeCenterGain(0) == nil {
		t.Fatal("failure not reported")
	}
	last := (*steps)[len(*steps)-3:]
	want := []centerStep{{0x00, []byte{0}}, {0x7F, []byte{0}}, {0x00, []byte{0}}}
	if !reflect.DeepEqual(last, want) {
		t.Fatalf("book/page not restored: %v", *steps)
	}
}

func TestCenterLevelPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "center-level")
	if v, e := loadCenterLevel(path); e != nil || v != centerDefault {
		t.Fatalf("missing file: %d %v", v, e)
	}
	if e := saveCenterLevel(path, 4); e != nil {
		t.Fatal(e)
	}
	if b, _ := os.ReadFile(path); string(b) != "4\n" {
		t.Fatalf("saved %q", b)
	}
	if v, e := loadCenterLevel(path); e != nil || v != 4 {
		t.Fatalf("reload: %d %v", v, e)
	}
	if e := saveCenterLevel(path, centerDefault); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("default level must remove the saved file")
	}
	os.WriteFile(path, []byte("6\n"), 0600)
	if v, e := loadCenterLevel(path); e == nil || v != centerDefault {
		t.Fatal("invalid level accepted")
	}
}

func TestCenterReapplyAfterWake(t *testing.T) {
	now := time.Unix(1000, 0)
	c := centerController{level: centerDefault, loaded: true}
	if c.centerDue(true, "s1", now) {
		t.Fatal("default level must never write")
	}
	c = centerController{level: 4, loaded: true}
	if c.centerDue(true, "s1", now) || c.centerDue(true, "s1", now.Add(3*time.Second)) {
		t.Fatal("applied before the wake settle delay")
	}
	if !c.centerDue(true, "s1", now.Add(4*time.Second)) {
		t.Fatal("not applied after wake")
	}
	c.passes++
	if c.centerDue(true, "s1", now.Add(10*time.Second)) || !c.centerDue(true, "s1", now.Add(15*time.Second)) {
		t.Fatal("confirmation pass timing")
	}
	c.passes++
	if c.centerDue(true, "s1", now.Add(60*time.Second)) {
		t.Fatal("writes must stop after the confirmation pass")
	}
	if c.centerDue(false, "s1", now.Add(61*time.Second)) {
		t.Fatal("must not write in standby")
	}
	wake := now.Add(70 * time.Second)
	c.centerDue(true, "s1", wake)
	if !c.centerDue(true, "s1", wake.Add(4*time.Second)) {
		t.Fatal("not re-applied after the next wake")
	}
	c.passes = 2
	restart := wake.Add(100 * time.Second)
	c.centerDue(true, "s2", restart)
	if !c.centerDue(true, "s2", restart.Add(4*time.Second)) {
		t.Fatal("not re-applied after a firmware restart")
	}
}

func TestCenterChangeSoonAfterWakeIsReapplied(t *testing.T) {
	wake := time.Unix(2000, 0)
	// Level chosen 2 s after a wake the monitor already saw (at level 0).
	c := centerController{level: centerDefault, loaded: true}
	if c.centerDue(true, "s1", wake) {
		t.Fatal("default level must never write")
	}
	c.level = 3
	c.markApplied("s1", wake.Add(2*time.Second))
	if !c.centerDue(true, "s1", wake.Add(4*time.Second)) {
		t.Fatal("new level not re-applied by the settle pass")
	}
	c.passes++
	if !c.centerDue(true, "s1", wake.Add(15*time.Second)) {
		t.Fatal("new level not re-applied by the confirmation pass")
	}
	// Level chosen before the monitor noticed the wake: passes start from the change.
	c = centerController{level: 2, loaded: true}
	change := wake.Add(time.Second)
	c.markApplied("s1", change)
	if c.centerDue(true, "s1", change.Add(3*time.Second)) || !c.centerDue(true, "s1", change.Add(4*time.Second)) {
		t.Fatal("passes must start from an unobserved wake")
	}
	// Long after the wake, the written level is final.
	c = centerController{level: 1, loaded: true}
	c.centerDue(true, "s1", wake)
	c.passes = 2
	c.level = 4
	c.markApplied("s1", wake.Add(time.Minute))
	if c.centerDue(true, "s1", wake.Add(2*time.Minute)) {
		t.Fatal("no extra writes long after the wake")
	}
	// A change between the two passes keeps the confirmation pass.
	c = centerController{level: 1, loaded: true}
	c.centerDue(true, "s1", wake)
	c.passes = 1
	c.level = 5
	c.markApplied("s1", wake.Add(8*time.Second))
	if !c.centerDue(true, "s1", wake.Add(15*time.Second)) {
		t.Fatal("confirmation pass lost after a change between passes")
	}
}
