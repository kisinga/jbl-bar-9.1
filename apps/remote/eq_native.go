package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const eqSpeakerBase = uint32(0xd4bd0c + 0x16a8)
const eqHPPElement = uint32(0x840000)
const eqHPPHash = "e9f525a1771b4376921402d016ff54d6c6b3dc2d0d5f29ec0e6e352d40b0f982"
const eqHDSPHash = "1dcb71b2cd28df3fd8d36a16cd8cf71e47344ecb063cfab9efc55fcd94104258"
const eqPPCHash = "98d5b8fdee338842a2355867af1af6159214584b9dd1b9ac876db73d68279378"

// This adapter only reads the pinned active front-speaker chain and restores
// its native preset. It never writes DSP memory or accepts arbitrary commands.
// Callers hold d.mu across any EQ transaction, including watched-file changes.
type eqNativeState struct {
	Block                            []byte
	Mode, Cache, Element, SampleRate uint32
	outer, instance, hppContext      uint32
}
type eqMemoryMap struct {
	low, high uint32
	writable  bool
	offset    uint64
	path      string
}

func eqParseMaps(data string) ([]eqMemoryMap, error) {
	var result []eqMemoryMap
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			return nil, errors.New("invalid audio memory map")
		}
		bounds := strings.Split(f[0], "-")
		if len(bounds) != 2 {
			return nil, errors.New("invalid audio memory map")
		}
		lo, e1 := strconv.ParseUint(bounds[0], 16, 32)
		hi, e2 := strconv.ParseUint(bounds[1], 16, 32)
		off, e3 := strconv.ParseUint(f[2], 16, 64)
		if e1 != nil || e2 != nil || e3 != nil || lo >= hi || len(f[1]) != 4 {
			return nil, errors.New("invalid audio memory map")
		}
		if f[1][0] != 'r' {
			continue
		}
		path := ""
		if len(f) >= 6 {
			path = strings.Join(f[5:], " ")
		}
		result = append(result, eqMemoryMap{uint32(lo), uint32(hi), f[1][1] == 'w', off, path})
	}
	if len(result) == 0 {
		return nil, errors.New("audio mappings unavailable")
	}
	return result, nil
}
func eqMapped(maps []eqMemoryMap, a uint32, n int, write bool) bool {
	if n <= 0 || uint64(a)+uint64(n) > uint64(^uint32(0)) {
		return false
	}
	for _, m := range maps {
		if a >= m.low && uint64(a)+uint64(n) <= uint64(m.high) && (!write || m.writable) {
			return true
		}
	}
	return false
}
func eqLibraryBase(maps []eqMemoryMap, path string) (uint32, error) {
	var base uint32
	count := 0
	for _, m := range maps {
		if m.path == path && m.offset == 0 {
			base = m.low
			count++
		}
	}
	if count != 1 {
		return 0, errors.New("unsupported audio library mapping")
	}
	return base, nil
}

type eqNativeRead func(uint32, int) ([]byte, error)

func eqReadState(read eqNativeRead, hppBase, hdspBase uint32) (eqNativeState, error) {
	var out eqNativeState
	word := func(a uint32) (uint32, error) {
		b, e := read(a, 4)
		if e != nil {
			return 0, e
		}
		if len(b) != 4 {
			return 0, io.ErrUnexpectedEOF
		}
		return binary.LittleEndian.Uint32(b), nil
	}
	fail := func() (eqNativeState, error) { return out, errors.New("native EQ is not ready on this audio route") }
	active, e := word(eqSpeakerBase + 0x40)
	if e != nil || active != 1 {
		return fail()
	}
	pending, e := word(eqSpeakerBase + 0x44)
	if e != nil || pending != 0 {
		return fail()
	}
	enabled, e := read(eqSpeakerBase+5, 1)
	if e != nil || len(enabled) != 1 || enabled[0]&2 == 0 {
		return fail()
	}
	handle, e := word(eqSpeakerBase + 0x28)
	if e != nil || handle == 0 {
		return fail()
	}
	count, e := word(handle + 0xd8)
	if e != nil || count < 1 || count > 16 {
		return fail()
	}
	pool, e := word(handle + 0xdc)
	if e != nil || pool == 0 {
		return fail()
	}
	entries, e := read(pool, int(count)*0x1b4)
	if e != nil || len(entries) != int(count)*0x1b4 {
		return fail()
	}
	found := 0
	for i := uint32(0); i < count; i++ {
		entry := entries[i*0x1b4 : (i+1)*0x1b4]
		id := binary.LittleEndian.Uint32(entry)
		if id>>16 == 0x84 {
			// Other firmware pipeline pages are not part of this first implementation.
			if id != eqHPPElement {
				return fail()
			}
			found++
			module := binary.LittleEndian.Uint32(entry[0x14:])
			if module == 0 {
				return fail()
			}
			out.hppContext, e = word(module)
			if e != nil || out.hppContext == 0 {
				return fail()
			}
		}
	}
	if found != 1 {
		return fail()
	}
	out.Element = eqHPPElement
	out.outer, e = word(hppBase + 0xa45c)
	if e != nil || out.outer == 0 {
		return fail()
	}
	pointer, e := word(out.hppContext + 0x1c)
	if e != nil || pointer != out.outer {
		return fail()
	}
	out.instance, e = word(out.outer)
	if e != nil || out.instance == 0 {
		return fail()
	}
	out.Cache, e = word(out.hppContext + 0x38)
	if e != nil || out.Cache > 1 {
		return fail()
	}
	out.Mode, e = word(out.instance + 0x1108)
	if e != nil || (out.Mode != 0 && out.Mode != 1 && out.Mode != 5) {
		return fail()
	}
	out.SampleRate, e = word(out.outer + 0x28)
	if e != nil || (out.SampleRate != 44100 && out.SampleRate != 48000) {
		return fail()
	}
	out.Block, e = read(hdspBase+0x1df680, eqBlockSize)
	if e != nil || len(out.Block) != eqBlockSize {
		return fail()
	}
	again, e := read(hdspBase+0x1df680, eqBlockSize)
	if e != nil || !bytes.Equal(out.Block, again) {
		return fail()
	}
	// A source or playback format transition can rebuild these objects mid-read.
	for _, pair := range [][2]uint32{{eqSpeakerBase + 0x28, handle}, {eqSpeakerBase + 0x40, 1}, {eqSpeakerBase + 0x44, 0}, {handle + 0xd8, count}, {handle + 0xdc, pool}, {hppBase + 0xa45c, out.outer}, {out.outer, out.instance}, {out.hppContext + 0x1c, out.outer}, {out.hppContext + 0x38, out.Cache}, {out.instance + 0x1108, out.Mode}, {out.outer + 0x28, out.SampleRate}} {
		v, e := word(pair[0])
		if e != nil || v != pair[1] {
			return fail()
		}
	}
	out.Block = append([]byte(nil), out.Block...)
	return out, nil
}

var eqPinCache struct {
	sync.Mutex
	key string
}

func (d *device) eqCheckNativeLibraries(key string, force bool) error {
	eqPinCache.Lock()
	defer eqPinCache.Unlock()
	if !force && key != "" && eqPinCache.key == key {
		return nil
	}
	for path, want := range map[string]string{"/system/usr/local/lib/libHPP.so": eqHPPHash, "/system/usr/lib/libhdsp.so": eqHDSPHash, "/system/usr/lib/ppc/libppc.so": eqPPCHash} {
		if got, e := hashFile(path); e != nil || got != want {
			eqPinCache.key = ""
			return errors.New("unsupported EQ audio libraries")
		}
	}
	eqPinCache.key = key
	return nil
}
func (d *device) readEQNative() (eqNativeState, error) {
	var empty eqNativeState
	if d.pid == 0 {
		return empty, errors.New("soundbar is starting")
	}
	if !d.same() {
		return empty, errors.New("soundbar restarted")
	}
	raw, e := os.ReadFile(fmt.Sprintf("/proc/%d/maps", d.pid))
	if e != nil {
		return empty, errors.New("audio mappings unavailable")
	}
	maps, e := eqParseMaps(string(raw))
	if e != nil {
		return empty, e
	}
	hb, e := eqLibraryBase(maps, "/system/usr/local/lib/libHPP.so")
	if e != nil {
		return empty, e
	}
	db, e := eqLibraryBase(maps, "/system/usr/lib/libhdsp.so")
	if e != nil {
		return empty, e
	}
	pb, e := eqLibraryBase(maps, "/system/usr/lib/ppc/libppc.so")
	if e != nil {
		return empty, e
	}
	key := fmt.Sprintf("%d:%s:%x:%x:%x", d.pid, d.start, hb, db, pb)
	if e = d.eqCheckNativeLibraries(key, false); e != nil {
		return empty, e
	}
	mem, e := os.Open(fmt.Sprintf("/proc/%d/mem", d.pid))
	if e != nil {
		return empty, errors.New("audio readback unavailable")
	}
	defer mem.Close()
	read := func(a uint32, n int) ([]byte, error) {
		if !eqMapped(maps, a, n, true) {
			return nil, errors.New("unexpected audio state pointer")
		}
		b := make([]byte, n)
		_, e := mem.ReadAt(b, int64(a))
		return b, e
	}
	state, e := eqReadState(read, hb, db)
	if e != nil {
		return empty, e
	}
	if !d.same() {
		return empty, errors.New("soundbar restarted")
	}
	return state, nil
}

// withEQConsole reuses the remote's two existing console fields. The native
// parser's ACK confirms request completion before their original values return.
func (d *device) withEQConsole(ctx context.Context, fn func(context.Context) error) (err error) {
	if !d.same() || !d.idleKeys() || controlBusy() {
		return errors.New("audio controls are busy; retry shortly")
	}
	if got, e := hashFile("/bin/cli"); e != nil || got != cliHash {
		return errors.New("unsupported local control client")
	}
	input, e := d.read(15427780, 1)
	if e != nil {
		return e
	}
	gate, e := d.read(15427784, 4)
	if e != nil {
		return e
	}
	if gate != 2 || (input != 0 && input != 1) {
		return errors.New("local console is in use; retry shortly")
	}
	defer func() {
		a := d.write(15427784, 4, gate)
		b := d.write(15427780, 1, input)
		if a != nil || b != nil {
			err = errors.New("console restoration failed; stop controls and collect logs")
		}
	}()
	if e = d.write(15427780, 1, 1); e != nil {
		return e
	}
	if e = d.write(15427784, 4, 0); e != nil {
		return e
	}
	if e = d.cli(ctx, ""); e != nil {
		return e
	}
	if e = d.acknowledge(ctx, 3*time.Second); e != nil {
		return e
	}
	return fn(ctx)
}
func eqPresetCommands(mode uint32) ([][]string, error) {
	if mode > 1 {
		return nil, errors.New("invalid native EQ reset mode")
	}
	return [][]string{{"mw.aud.se.ppc.infoset", "element", "0x840000", "0x840001", "1"}, {"mw.aud.se.ppc.infoset", "epara", strconv.FormatUint(uint64(mode), 10)}, {"mw.aud.se.ppc.infoset", "elend"}}, nil
}
func (d *device) resetEQNative(ctx context.Context) error {
	if e := d.eqCheckNativeLibraries("", true); e != nil {
		return e
	}
	before, e := d.readEQNative()
	if e != nil {
		return e
	}
	current := d.snapshotCurrent()
	if !current.Available || current.Power != "On" || current.Mode != "Standard" {
		return errors.New("turn on the bar in Standard mode before resetting EQ")
	}
	return d.withEQConsole(ctx, func(ctx context.Context) error {
		now, e := d.readEQNative()
		if e != nil {
			return e
		}
		if now.outer != before.outer || now.hppContext != before.hppContext || now.instance != before.instance {
			return errors.New("audio route changed; retry EQ reset")
		}
		send := func(c context.Context, mode uint32) error {
			commands, _ := eqPresetCommands(mode)
			for _, args := range commands {
				if e := d.cli(c, args...); e != nil {
					return e
				}
			}
			return d.acknowledge(c, 3*time.Second)
		}
		// The watched-file setter leaves HPP's cached mode unchanged. A lone mode0
		// request would be discarded. Always pass through1, then restore0 even when
		// the browser disconnects after the first command.
		firstErr := send(ctx, 1)
		restore, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		defer cancel()
		if firstErr != nil {
			// A transport failure can leave the native three-stage setter in
			// state1/2. End or cancel that pending transaction before reset0.
			if e = d.cli(restore, "mw.aud.se.ppc.infoset", "elend"); e != nil {
				return errors.New("native EQ parameter staging could not be cleared")
			}
			if e = d.acknowledge(restore, 3*time.Second); e != nil {
				return errors.New("native EQ parameter staging could not be cleared")
			}
		}
		if e = send(restore, 0); e != nil {
			return errors.New("native EQ reset was not confirmed")
		}
		after, e := d.readEQNative()
		if e != nil {
			return e
		}
		if after.Mode != 0 || after.Cache != 0 || after.outer != before.outer || after.hppContext != before.hppContext {
			return errors.New("native EQ preset restoration failed")
		}
		return firstErr
	})
}

// eqMute is idempotent and verifies the persistent mute flag. The manager holds
// d.mu and restores the original mute state after each tuning transaction.
func (d *device) eqMute(ctx context.Context, want bool) error {
	current, e := d.configByte(155)
	if e != nil || current > 1 {
		return errors.New("mute state unavailable")
	}
	if (current != 0) == want {
		return nil
	}
	return d.withEQConsole(ctx, func(ctx context.Context) error {
		if e := d.cli(ctx, "mw.iom.se", "0x6f002", "60"); e != nil {
			return e
		}
		if e := d.acknowledge(ctx, 3*time.Second); e != nil {
			return e
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if e := ctx.Err(); e != nil {
				return e
			}
			if !d.same() {
				return errors.New("audio process changed during mute adjustment")
			}
			// Match the normal remote's post-ACK settling interval. The IOM
			// event can be queued even when the console marker already exists.
			time.Sleep(100 * time.Millisecond)
			v, e := d.configByte(155)
			// The flag changes on key-down. Do not start the next transaction
			// until key-up: a fast EQ update can otherwise leave unmute busy.
			if e == nil && v <= 1 && (v != 0) == want && d.idleKeys() {
				return nil
			}
		}
		return errors.New("mute change was not confirmed")
	})
}
