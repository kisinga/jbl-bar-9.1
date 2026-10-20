package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var eqParserPayload string
var eqParserBridge struct {
	sync.Once
	directory, path string
}
var eqParserDependencies = map[string]string{
	"/usr/lib/libhdsp.so":           eqHDSPHash,
	"/system/bdplib/ld-linux.so.3":  "49343df4311cb08d47dce9705eebcd06b666509d797ea018ef7c3142cb1f8af9",
	"/system/bdplib/libc.so.6":      "2f3952341c498325cc9423acadcf7c2ad94ee9196af071c661ffd16057552dc0",
	"/system/bdplib/libdl.so.2":     "a90018084bd508ce2a1a8d4a581cb3caebb0dfcd357f60b9e16a137850a064c6",
	"/system/bdplib/libgcc_s.so.1":  "8cefc5883637998effb2b60cf89115692fcfea32845892e4e3b3570b036190d1",
	"/system/bdplib/libm.so.6":      "8a5316e5d611f775819f77509014278d06335b03d8569cf2ff9dafd2e1e104c5",
	"/system/bdplib/libstdc++.so.6": "59a59e75b7ad46d45b239411ada16818b56f48803c6e25195cdcd266ed86a67d",
}

const eqWatchPath = "/mnt/ubi_boot/temp.json"
const eqRecoveryPath = "/var/JBLRemote/EQ_RECOVERY.json"
const eqBackupPath = "/var/JBLRemote/eq-baseline.bin"

func eqParserPath() string {
	eqParserBridge.Do(func() {
		if eqParserPayload == "" {
			return
		}
		b, e := base64.StdEncoding.DecodeString(eqParserPayload)
		if e != nil || len(b) < 52 || len(b) > 65536 || !bytes.Equal(b[:7], []byte{127, 'E', 'L', 'F', 1, 1, 1}) || b[18] != 40 || b[19] != 0 {
			return
		}
		for path, want := range eqParserDependencies {
			if got, e := hashFile(path); e != nil || got != want {
				return
			}
		}
		dir, e := os.MkdirTemp("/tmp", "jbl-eq-parser-")
		if e != nil {
			return
		}
		path := filepath.Join(dir, "eq-parser")
		if os.WriteFile(path, b, 0700) != nil {
			os.RemoveAll(dir)
			return
		}
		eqParserBridge.directory, eqParserBridge.path = dir, path
	})
	return eqParserBridge.path
}
func closeEQParser() {
	if eqParserBridge.directory != "" {
		_ = os.RemoveAll(eqParserBridge.directory)
	}
}

type frontEQState struct {
	Available   bool      `json:"available"`
	Enabled     bool      `json:"enabled"`
	Gains       []float64 `json:"gains_db"`
	Frequencies []int     `json:"frequencies_hz"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
}
type eqRecovery struct {
	Owner        string `json:"owner"`
	Target       string `json:"target"`
	BaselineHash string `json:"baseline_sha256"`
	ProcessStart string `json:"process_start"`
	BootID       string `json:"boot_id"`
}
type eqController struct {
	active, needsReset, recoveryChecked bool
	gains                               [5]float64
	baseline, expected, defaults        []byte
	registry                            []eqEntry
	target, directory, problem          string
	native                              eqNativeState
}

func eqDigest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func eqBootID() string {
	b, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if e != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func ownedEQTarget(target string) bool {
	dir := filepath.Dir(target)
	return filepath.Clean(target) == target && filepath.Dir(dir) == "/tmp" && strings.HasPrefix(filepath.Base(dir), "jbl-front-eq-") && len(filepath.Base(dir)) > len("jbl-front-eq-") && filepath.Base(target) == "active.json"
}
func (m *eqController) removeWatch() error {
	info, e := os.Lstat(eqWatchPath)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	target, e := os.Readlink(eqWatchPath)
	if info.Mode()&os.ModeSymlink == 0 || e != nil || m.target == "" || target != m.target || !ownedEQTarget(target) {
		return errors.New("existing tuning file is not owned by this remote")
	}
	return os.Remove(eqWatchPath)
}
func (m *eqController) writeWatch(recipe []byte) error {
	if !ownedEQTarget(m.target) {
		return errors.New("invalid EQ session")
	}
	next := m.target + ".new"
	if e := os.WriteFile(next, recipe, 0600); e != nil {
		return e
	}
	if e := os.Rename(next, m.target); e != nil {
		return e
	}
	info, e := os.Lstat(eqWatchPath)
	if os.IsNotExist(e) {
		return os.Symlink(m.target, eqWatchPath)
	}
	if e != nil {
		return e
	}
	target, e := os.Readlink(eqWatchPath)
	if info.Mode()&os.ModeSymlink == 0 || e != nil || target != m.target {
		return errors.New("existing tuning file preserved")
	}
	link := eqWatchPath + ".jbl-new"
	if e = os.Symlink(m.target, link); e != nil {
		return e
	}
	defer os.Remove(link)
	return os.Rename(link, eqWatchPath)
}
func (m *eqController) runParser(ctx context.Context, mode string, input []byte) ([]byte, error) {
	helper := eqParserPath()
	if helper == "" {
		return nil, errors.New("native EQ parser is unavailable")
	}
	dir, e := os.MkdirTemp("/tmp", "jbl-eq-parse-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "output")
	args := []string{"/usr/lib/libhdsp.so", mode}
	if mode == "parse" {
		if len(input) == 0 || len(input) > 65536 {
			return nil, errors.New("EQ recipe exceeds its bound")
		}
		path := filepath.Join(dir, "input.json")
		if e = os.WriteFile(path, input, 0600); e != nil {
			return nil, e
		}
		args = append(args, path)
	} else if mode != "defaults" && mode != "registry" {
		return nil, errors.New("unknown EQ parser mode")
	}
	args = append(args, output)
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, helper, args...)
	cmd.Env = []string{"PATH=/bin:/usr/bin", "LD_LIBRARY_PATH=/system/bdplib:/usr/lib:/usr/local/lib"}
	if e = cmd.Run(); e != nil {
		return nil, errors.New("native EQ parser did not validate the recipe")
	}
	b, e := os.ReadFile(output)
	if e != nil || len(b) > 65536 {
		return nil, errors.New("invalid native EQ parser output")
	}
	if mode != "registry" && len(b) != eqBlockSize {
		return nil, errors.New("invalid native EQ block")
	}
	return b, nil
}
func (m *eqController) prepare(ctx context.Context, block []byte) error {
	var e error
	if m.defaults == nil {
		defaults, e := m.runParser(ctx, "defaults", nil)
		if e != nil {
			return e
		}
		var b []byte
		b, e = m.runParser(ctx, "registry", nil)
		if e != nil {
			return e
		}
		var registry []eqEntry
		if e = json.Unmarshal(b, &registry); e != nil {
			return errors.New("native EQ registry is unavailable")
		}
		m.defaults, m.registry = defaults, registry
	}
	recipe, e := validateNeutralSnapshot(block, m.defaults, m.registry)
	if e != nil {
		return e
	}
	parsed, e := m.runParser(ctx, "parse", recipe)
	if e != nil {
		return e
	}
	if !bytes.Equal(parsed, block) {
		return errors.New("original tuning cannot be reconstructed exactly")
	}
	return nil
}
func (d *device) frontEQStatus(s state) *frontEQState {
	m := &d.eq
	out := &frontEQState{Enabled: m.active, Gains: append([]float64(nil), m.gains[:]...), Frequencies: []int{125, 500, 2000, 4000, 8000}, Status: "unavailable"}
	if eqParserPayload == "" {
		out.Error = "EQ support is not installed"
		return out
	}
	if !s.Available || s.Power != "On" {
		out.Error = "Turn on the bar and start audio to adjust EQ"
		if m.active {
			out.Status = "reset_required"
		}
		return out
	}
	if eqParserPath() == "" {
		out.Error = "Unsupported EQ libraries"
		return out
	}
	n, e := d.readEQNative()
	if e != nil {
		out.Error = e.Error()
		return out
	}
	out.Available = true
	out.Status = "ready"
	if m.active {
		if n.Mode != 5 || !bytes.Equal(n.Block, m.expected) || n.outer != m.native.outer || n.SampleRate != m.native.SampleRate {
			out.Status = "reset_required"
			out.Error = "Audio route changed; reset EQ before adjusting"
		} else {
			out.Status = "active"
		}
	} else if n.Mode != 0 || n.Cache != 0 {
		out.Available = false
		out.Error = "Restore the native sound preset before using Front EQ"
	}
	if m.needsReset {
		out.Status = "reset_required"
	}
	if m.problem != "" {
		out.Available = false
		out.Error = m.problem
	}
	return out
}
func (d *device) resetFrontEQ(ctx context.Context) (err error) {
	m := &d.eq
	if !m.active && !m.needsReset {
		return nil
	}
	if e := m.removeWatch(); e != nil {
		return e
	}
	// A deleted/missing recipe is not an update. Allow the watcher to consume
	// its notification before restoring the native preset, then verify stability.
	time.Sleep(100 * time.Millisecond)
	n, e := d.readEQNative()
	if e != nil {
		m.needsReset = true
		return e
	}
	muted, e := d.configByte(155)
	if e != nil {
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if e := d.eqMute(c, muted != 0); e != nil {
			err = e
		}
	}()
	if e = d.eqMute(ctx, true); e != nil {
		return e
	}
	if n.Mode != 0 || n.Cache != 0 {
		if e = d.resetEQNative(ctx); e != nil {
			m.needsReset = true
			return e
		}
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		n, e = d.readEQNative()
		if e != nil || n.Mode != 0 || n.Cache != 0 {
			m.needsReset = true
			return errors.New("native EQ preset restoration was not confirmed")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(m.baseline) == eqBlockSize && !bytes.Equal(n.Block, m.baseline) {
		m.needsReset = true
		return errors.New("native preset returned, but tuning differs from the saved baseline")
	}
	m.active, m.needsReset = false, false
	m.gains = [5]float64{}
	m.expected = nil
	m.problem = ""
	_ = os.Remove(eqRecoveryPath)
	_ = os.Remove(eqBackupPath)
	if m.directory != "" {
		_ = os.RemoveAll(m.directory)
	}
	m.target, m.directory = "", ""
	return nil
}
func (d *device) applyFrontEQ(ctx context.Context, gains [5]float64) (err error) {
	m := &d.eq
	if !m.recoveryChecked {
		d.recoverFrontEQ(ctx)
	}
	if m.problem != "" {
		return errors.New(m.problem)
	}
	if gains == [5]float64{} {
		return d.resetFrontEQ(ctx)
	}
	if e := d.identify(); e != nil {
		return e
	}
	before := d.snapshotCurrent()
	if !before.Available || before.Power != "On" || before.Mode != "Standard" {
		return errors.New("Front EQ requires Standard Mode and active audio")
	}
	if controlBusy() || !d.idleKeys() {
		return errors.New("sound controls are busy; retry shortly")
	}
	n, e := d.readEQNative()
	if e != nil {
		return e
	}
	if m.active {
		if n.Mode != 5 || n.outer != m.native.outer || !bytes.Equal(n.Block, m.expected) {
			return errors.New("reset EQ after the audio route or sound settings change")
		}
	} else {
		if n.Mode != 0 || n.Cache != 0 {
			return errors.New("unsupported native EQ mode")
		}
		if _, e = os.Lstat(eqWatchPath); !os.IsNotExist(e) {
			return errors.New("existing tuning file preserved")
		}
		if e = m.prepare(ctx, n.Block); e != nil {
			return e
		}
		m.baseline = append([]byte(nil), n.Block...)
		m.native = n
	}
	recipe, expected, e := generateFrontEQ(m.baseline, m.defaults, m.registry, gains)
	if e != nil {
		return e
	}
	parsed, e := m.runParser(ctx, "parse", recipe)
	if e != nil {
		return e
	}
	if !bytes.Equal(parsed, expected) {
		return errors.New("requested EQ did not pass full native validation")
	}
	if m.directory == "" {
		dir, e := os.MkdirTemp("/tmp", "jbl-front-eq-")
		if e != nil {
			return e
		}
		target := filepath.Join(dir, "active.json")
		record := eqRecovery{Owner: "JBL_FRONT_EQ_REV1", Target: target, BaselineHash: eqDigest(m.baseline), ProcessStart: d.start, BootID: eqBootID()}
		raw, _ := json.Marshal(record)
		if e = eqWriteExclusive(eqBackupPath, m.baseline); e != nil {
			os.RemoveAll(dir)
			return e
		}
		if e = eqWriteExclusive(eqRecoveryPath, raw); e != nil {
			os.Remove(eqBackupPath)
			os.RemoveAll(dir)
			return e
		}
		m.directory, m.target = dir, target
	}
	muted, e := d.configByte(155)
	if e != nil {
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if restore := d.eqMute(c, muted != 0); restore != nil {
			err = restore
		}
	}()
	if e = d.eqMute(ctx, true); e != nil {
		return e
	}
	m.active, m.needsReset = true, true
	defer func() {
		if err != nil {
			c, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			if restore := d.resetFrontEQ(c); restore != nil {
				m.problem = "EQ needs reset: " + restore.Error()
				err = errors.New(m.problem)
			}
		}
	}()
	if e = m.writeWatch(recipe); e != nil {
		return e
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if e = ctx.Err(); e != nil {
			return e
		}
		current, e := d.readEQNative()
		if e == nil && current.Mode == 5 && current.outer == n.outer && current.SampleRate == n.SampleRate && bytes.Equal(current.Block, expected) {
			m.expected = expected
			m.gains = gains
			m.needsReset = false
			m.problem = ""
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("native EQ application was not confirmed")
}
func (d *device) eqAction(ctx context.Context, action string, gains []float64) (state, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e := d.identify(); e != nil {
		return d.snapshotCurrent(), e
	}
	var e error
	if action == "eq_reset" {
		if gains != nil {
			e = errors.New("reset does not accept band values")
		} else {
			e = d.resetFrontEQ(ctx)
		}
	} else if action == "eq_apply" && len(gains) == 5 {
		var bands [5]float64
		copy(bands[:], gains)
		e = d.applyFrontEQ(ctx, bands)
	} else {
		e = errors.New("provide five Front EQ values")
	}
	return d.snapshotCurrent(), e
}
func (d *device) recoverFrontEQ(ctx context.Context) {
	m := &d.eq
	if m.recoveryChecked {
		return
	}
	m.recoveryChecked = true
	b, e := os.ReadFile(eqRecoveryPath)
	if os.IsNotExist(e) {
		return
	}
	if e != nil {
		m.problem = "EQ recovery record cannot be read"
		return
	}
	var record eqRecovery
	if json.Unmarshal(b, &record) != nil || record.Owner != "JBL_FRONT_EQ_REV1" || !ownedEQTarget(record.Target) {
		m.problem = "Unknown EQ recovery record preserved"
		return
	}
	base, e := os.ReadFile(eqBackupPath)
	if e != nil || len(base) != eqBlockSize || eqDigest(base) != record.BaselineHash {
		m.problem = "EQ backup integrity check failed"
		return
	}
	m.target, m.directory = record.Target, filepath.Dir(record.Target)
	m.baseline = base
	m.active, m.needsReset = true, true
	// Start ticks can repeat across boots. Never compare a fresh calibration
	// against a previous boot's snapshot, even if the process start matches.
	if record.ProcessStart != d.start || record.BootID == "" || record.BootID != eqBootID() {
		m.baseline = nil
	}
	if e = d.resetFrontEQ(ctx); e != nil {
		m.problem = "EQ recovery pending: " + e.Error()
	}
}
func (r *remote) monitorFrontEQ(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if eqParserPayload == "" || !r.actionMu.TryLock() {
			continue
		}
		r.device.mu.Lock()
		d := r.device
		if e := d.identify(); e == nil {
			c, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			if !d.eq.recoveryChecked {
				d.recoverFrontEQ(c)
			}
			if d.eq.active {
				n, e := d.readEQNative()
				if e != nil || n.Mode != 5 || n.outer != d.eq.native.outer || !bytes.Equal(n.Block, d.eq.expected) {
					if e = d.resetFrontEQ(c); e != nil {
						d.eq.problem = "EQ reset pending: " + e.Error()
					}
				}
			}
			cancel()
		}
		d.mu.Unlock()
		r.actionMu.Unlock()
	}
}

func eqWriteExclusive(path string, b []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return errors.New("existing EQ recovery files preserved")
	}
	_, e = f.Write(b)
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		_ = os.Remove(path)
	}
	return e
}
