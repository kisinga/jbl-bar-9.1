// A small, firmware-pinned LAN remote. It exposes fixed buttons, never a shell.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

//go:embed www/*
var assets embed.FS

const firmwareHash = "86ec83472a10dd4278285123f8ecbfb96bdd97444dd98c3a56d5d87441c90b63"
const cliHash = "8910fb852d2f994a519615fede404fbb036e3a8f5264cc69cdff57038af3c78f"

type config struct {
	Listen      string `json:"listen"`
	AllowedCIDR string `json:"allowed_cidr"`
}
type bluetoothState struct {
	PeerName     string `json:"peer_name,omitempty"`
	Selected     bool   `json:"selected"`
	Name         string `json:"name,omitempty"`
	Connectable  bool   `json:"connectable"`
	Discoverable bool   `json:"discoverable"`
	Connected    *bool  `json:"connected,omitempty"`
	Audio        string `json:"audio,omitempty"`
}
type state struct {
	Revision    uint64          `json:"revision"`
	Sound       *soundState     `json:"sound,omitempty"`
	Streaming   *streamState    `json:"streaming,omitempty"`
	SourceState *uint32         `json:"source_state,omitempty"`
	WiFi        wifiState       `json:"wifi"`
	Bluetooth   *bluetoothState `json:"bluetooth,omitempty"`
	Available   bool            `json:"available"`
	Error       string          `json:"error,omitempty"`
	Power       string          `json:"power"`
	Mode        string          `json:"mode"`
	Volume      *uint32         `json:"volume,omitempty"`
	Muted       *bool           `json:"muted,omitempty"`
	Source      *uint32         `json:"source,omitempty"`
	Input       string          `json:"input"`
	Uptime      int64           `json:"uptime"`
	Watcher     string          `json:"watcher"`
	CSRF        string          `json:"csrf"`
}
type device struct {
	mu         sync.Mutex
	pid        int
	start      string
	revision   uint64
	cliChecked bool
}
type remote struct {
	cfg      config
	network  *net.IPNet
	csrf     string
	device   *device
	feed     stateFeed
	stop     <-chan struct{}
	actionMu sync.Mutex
}

func hashFile(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func processStart(pid int) (string, error) {
	b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if e != nil {
		return "", e
	}
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return "", errors.New("invalid process stat")
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 20 {
		return "", errors.New("short process stat")
	}
	return f[19], nil
}
func (d *device) identify() error {
	entries, e := os.ReadDir("/proc")
	if e != nil {
		return e
	}
	pid := 0
	for _, entry := range entries {
		p, e := strconv.Atoi(entry.Name())
		if e != nil {
			continue
		}
		b, e := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if e == nil && strings.TrimSpace(string(b)) == "bdpprog" {
			if pid != 0 {
				return errors.New("multiple soundbar processes")
			}
			pid = p
		}
	}
	if pid == 0 {
		return errors.New("soundbar is starting")
	}
	start, e := processStart(pid)
	if e != nil {
		return e
	}
	if d.pid != pid || d.start != start {
		h, e := hashFile(fmt.Sprintf("/proc/%d/exe", pid))
		if e != nil {
			return e
		}
		if h != firmwareHash {
			return errors.New("unsupported firmware; controls disabled")
		}
		d.pid = pid
		d.start = start
	}
	return nil
}
func (d *device) same() bool { s, e := processStart(d.pid); return e == nil && s == d.start }
func (d *device) read(address int64, width int) (uint32, error) {
	f, e := os.Open(fmt.Sprintf("/proc/%d/mem", d.pid))
	if e != nil {
		return 0, e
	}
	defer f.Close()
	b := make([]byte, width)
	if _, e = f.ReadAt(b, address); e != nil {
		return 0, e
	}
	switch width {
	case 1:
		return uint32(b[0]), nil
	case 2:
		return uint32(binary.LittleEndian.Uint16(b)), nil
	case 4:
		return binary.LittleEndian.Uint32(b), nil
	}
	return 0, errors.New("invalid read width")
}
func (d *device) write(address int64, width int, value uint32) error {
	if !d.same() {
		return errors.New("soundbar restarted")
	}
	if !(address == 15427780 && width == 1 || address == 15427784 && width == 4) {
		return errors.New("write outside console fields")
	}
	f, e := os.OpenFile(fmt.Sprintf("/proc/%d/mem", d.pid), os.O_WRONLY, 0)
	if e != nil {
		return e
	}
	defer f.Close()
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], value)
	if _, e = f.WriteAt(b[:width], address); e != nil {
		return e
	}
	got, e := d.read(address, width)
	if e != nil {
		return e
	}
	if got != value {
		return errors.New("console write verification failed")
	}
	return nil
}
func (d *device) configByte(id uint32) (uint32, error) {
	switch id {
	case 50, 51, 155, 365, 238, 201, 202:
	default:
		return 0, errors.New("unknown setting")
	}
	node := int64(0xf046d0) + int64(id)*0x4c
	t, e := d.read(node+0x28, 4)
	if e != nil {
		return 0, e
	}
	count, e := d.read(node+0x2c, 2)
	if e != nil {
		return 0, e
	}
	enabled, e := d.read(node+0x40, 1)
	if e != nil {
		return 0, e
	}
	if t != 1 || count != 1 || enabled != 1 {
		return 0, errors.New("setting layout changed")
	}
	base, e := d.read(0xd89770, 4)
	if e != nil {
		return 0, e
	}
	offset, e := d.read(node+0x48, 4)
	if e != nil {
		return 0, e
	}
	if base < 0x10000 || offset > 2*1024*1024 || uint64(base)+uint64(offset) > 0xffffffff {
		return 0, errors.New("invalid setting address")
	}
	return d.read(int64(base)+int64(offset), 1)
}

// REA: BT_GAP mode at context+8; the version-pinned context is static.
// 0/2 are discoverable; 1/3 are not. A2DP state is distinct from source selection.
func bluetoothAudio(value uint32) (bool, string, bool) {
	switch value {
	case 0:
		return false, "Disconnected", true
	case 1:
		return true, "Connected", true
	case 2:
		return true, "Paused", true
	case 3:
		return true, "Streaming", true
	}
	return false, "", false
}
func bluetoothName(b []byte) string {
	end := bytes.IndexByte(b, 0)
	if end < 1 || !utf8.Valid(b[:end]) {
		return ""
	}
	for _, r := range string(b[:end]) {
		if r < 32 || r == 127 {
			return ""
		}
	}
	return string(b[:end])
}
func (d *device) bluetooth() *bluetoothState {
	gap, e := d.read(0xc6d250+0x44, 4)
	if e != nil || gap != 0xc6d2c0 {
		return nil
	}
	mode, e := d.read(int64(gap)+8, 4)
	if e != nil || mode > 3 {
		return nil
	}
	s := &bluetoothState{Connectable: mode == 0 || mode == 1, Discoverable: mode == 0 || mode == 2}
	// GAP info's inline local-name buffer is 128 bytes, zero-terminated.
	info, ie := d.read(0xc6d250+0x4c, 4)
	if info == 0xc6d2cc && ie == nil {
		if f, e := os.Open(fmt.Sprintf("/proc/%d/mem", d.pid)); e == nil {
			b := make([]byte, 128)
			if _, e = f.ReadAt(b, int64(gap)+0x34); e == nil {
				s.Name = bluetoothName(b)
			}
			f.Close()
		}
	}
	a2dp, e := d.read(0xcb6450+0x44, 4)
	if e == nil && a2dp == 0xe3757c {
		value, e := d.read(int64(a2dp)+4, 4)
		if e == nil {
			if connected, audio, ok := bluetoothAudio(value); ok {
				s.Connected = &connected
				s.Audio = audio
			}
		}
	}
	if s.Connected != nil && *s.Connected && info == 0xc6d2cc {
		if f, e := os.Open(fmt.Sprintf("/proc/%d/mem", d.pid)); e == nil {
			b := make([]byte, 32)
			if _, e = f.ReadAt(b, int64(info)+0xc8); e == nil {
				address := peerAddress(b)
				name := readPeerName(address)
				again := make([]byte, 32)
				_, e = f.ReadAt(again, int64(info)+0xc8)
				value, se := d.read(0xe3757c+4, 4)
				connected, _, valid := bluetoothAudio(value)
				if e == nil && se == nil && valid && connected && address != "" && peerAddress(again) == address {
					s.PeerName = name
				}
			}
			f.Close()
		}
	}
	return s
}
func (d *device) snapshotLocked() state {
	if e := d.identify(); e != nil {
		d.revision++
		return state{Revision: d.revision, WiFi: readWiFi(), Power: "Starting", Mode: "Unknown", Input: "Unknown", Error: e.Error()}
	}
	return d.snapshotCurrent()
}

// During an action keep the original PID/start time; never restore saved
// console fields into a replacement firmware process.
func (d *device) snapshotCurrent() state {
	d.revision++
	s := state{Revision: d.revision, Power: "Starting", Mode: "Unknown", Input: "Unknown"}
	if b, e := os.ReadFile("/proc/uptime"); e == nil {
		v, _ := strconv.ParseFloat(strings.Fields(string(b))[0], 64)
		s.Uptime = int64(v)
	}
	if b, e := os.ReadFile("/var/JBLStandard/WATCH_STATUS.txt"); e == nil {
		s.Watcher = strings.TrimSpace(string(b))
	}
	p, ep := d.read(14191893, 1)
	system, es := d.read(12962676, 4)
	mode, em := d.read(13378532, 4)
	if ep != nil || es != nil || em != nil {
		s.Error = "Soundbar state is temporarily unavailable"
		return s
	}
	s.Available = true
	s.Bluetooth = d.bluetooth()
	sound := d.sound()
	s.Sound = &sound
	if v, e := d.configByte(365); e == nil {
		s.SourceState = &v
		if s.Bluetooth != nil {
			s.Bluetooth.Selected = v == 2
		}
		s.Input = sourceName(v)
		s.Streaming = d.streaming(v)
	}
	s.WiFi = readWiFi()
	if p == 1 && system == 0 {
		s.Power = "On"
	} else if system == 1 || system == 2 || system == 3 || p == 0 {
		s.Power = "Standby"
	}
	if mode == 0 {
		s.Mode = "Standard"
	} else if mode == 3 {
		s.Mode = "Smart"
	}
	if v, e := d.configByte(50); e == nil {
		s.Volume = &v
	}
	if v, e := d.configByte(155); e == nil {
		b := v != 0
		s.Muted = &b
	}
	if v, e := d.configByte(51); e == nil {
		s.Source = &v
	}
	if s.Bluetooth != nil && s.Bluetooth.Selected {
		s.Input = "Bluetooth"
	}
	if !d.same() {
		s.Available = false
		s.Error = "Soundbar restarted; refreshing"
	}
	return s
}
func (d *device) snapshot() state { d.mu.Lock(); defer d.mu.Unlock(); return d.snapshotLocked() }
func (d *device) cli(ctx context.Context, args ...string) error {
	if !d.same() {
		return errors.New("soundbar restarted")
	}
	if len(args) > 0 && args[0] == "mw.iom.se" && controlBusy() {
		return errors.New("Smart Mode automation is busy; retry shortly")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/cli", args...)
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "LD_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	output, e := cmd.CombinedOutput()
	if e != nil {
		return fmt.Errorf("local control request failed: %w (%s)", e, strings.TrimSpace(string(output)))
	}
	return nil
}
func (d *device) idleKeys() bool {
	for _, item := range [][2]int64{{13822224, 1}, {14200904, 4}, {14200898, 1}} {
		v, e := d.read(item[0], int(item[1]))
		if e != nil || v != 0 {
			return false
		}
	}
	return true
}
func (d *device) acknowledge(ctx context.Context, wait time.Duration) error {
	dir, e := os.MkdirTemp("/tmp", "jbl-web-ack-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	marker := filepath.Join(dir, "done")
	// Native cli has no completion response. A fixed marker command confirms
	// that the firmware parser consumed the preceding request before restoring.
	if e = d.cli(ctx, "exec", "/bin/busybox", "touch", marker); e != nil {
		return e
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if e := ctx.Err(); e != nil {
			return e
		}
		if _, e = os.Stat(marker); e == nil {
			return nil
		}
		if !d.same() {
			return errors.New("soundbar restarted")
		}
		time.Sleep(25 * time.Millisecond)
	}
	return errors.New("local control was not acknowledged")
}
func actionObserved(action string, before, after state) bool {
	if !after.Available {
		return false
	}
	switch action {
	case "volume_up", "volume_down":
		return before.Volume != nil && after.Volume != nil && *before.Volume != *after.Volume
	case "mute":
		return before.Muted != nil && after.Muted != nil && *before.Muted != *after.Muted
	case "power":
		return before.Power == "On" && after.Power == "Standby" || before.Power == "Standby" && after.Power == "On"
	case "input_tv":
		return after.SourceState != nil && *after.SourceState == 0
	case "input_hdmi":
		return after.SourceState != nil && *after.SourceState == 3
	case "input_bluetooth":
		return after.Bluetooth != nil && after.Bluetooth.Selected
	case "bluetooth_pair":
		return after.Bluetooth != nil && after.Bluetooth.Selected && after.Bluetooth.Discoverable && after.Bluetooth.Connectable
	case "bass_next", "atmos_next", "rear_next":
		return soundObserved(action, before.Sound, after.Sound)
	case "standard":
		return after.Mode == "Standard"
	}
	return false
}
func controlBusy() bool {
	if _, e := os.Stat("/tmp/jbl-standard-hook-active"); e == nil {
		return true
	}
	b, e := os.ReadFile("/var/JBLStandard/WATCH_STATUS.txt")
	return e == nil && (strings.Contains(string(b), "APPLYING") || strings.Contains(string(b), "STABILIZING"))
}
func (d *device) action(ctx context.Context, action string) (out state, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	keys := map[string]string{"volume_up": "0x6f000", "volume_down": "0x6f001", "mute": "0x6f002", "power": "0x8f000", "input_tv": "0x6f029", "input_hdmi": "0x6f01e", "input_bluetooth": "0x6f027", "bluetooth_pair": "0x6f027", "bass_next": "0x1f030", "atmos_next": "0x1f02f", "rear_next": "0x1f031"}
	_, simple := keys[action]
	if !simple && action != "standard" {
		return out, errors.New("unknown control")
	}
	before := d.snapshotLocked()
	if !before.Available {
		return before, errors.New(before.Error)
	}
	if strings.HasSuffix(action, "_next") {
		if _, _, ok := soundValue(action, before.Sound); !ok {
			return before, errors.New("sound setting unavailable")
		}
		if action == "rear_next" && (before.Sound.RearDocked == nil || *before.Sound.RearDocked) {
			return before, errors.New("undock both rear speakers first")
		}
	}
	if action == "bluetooth_pair" {
		if before.Bluetooth == nil || !before.Bluetooth.Selected {
			return before, errors.New("select Bluetooth first; pairing status must be available")
		}
		if before.Bluetooth.Discoverable && before.Bluetooth.Connectable {
			return before, nil
		}
	}
	if action == "standard" && before.Mode == "Standard" {
		return before, nil
	}
	if action != "power" && before.Power != "On" {
		return before, errors.New("turn the bar on first")
	}
	if action == "power" && before.Power != "On" && before.Power != "Standby" {
		return before, errors.New("soundbar is starting; wait before using power")
	}
	if _, e := os.Stat("/tmp/jbl-standard-hook-active"); e == nil {
		return before, errors.New("Smart Mode automation is busy; try again shortly")
	}
	if strings.Contains(before.Watcher, "APPLYING") || strings.Contains(before.Watcher, "STABILIZING") {
		return before, errors.New("sound settings are settling; try again shortly")
	}
	if !d.cliChecked {
		h, e := hashFile("/bin/cli")
		if e != nil || h != cliHash {
			return before, errors.New("unsupported local control client")
		}
		d.cliChecked = true
	}
	if !d.idleKeys() {
		return before, errors.New("remote control is busy; release its buttons and retry")
	}
	input, e := d.read(15427780, 1)
	if e != nil {
		return before, e
	}
	gate, e := d.read(15427784, 4)
	if e != nil {
		return before, e
	}
	if !(gate == 2 && (input == 0 || input == 1)) {
		return before, errors.New("local console is in use; retry shortly")
	}
	if controlBusy() {
		return before, errors.New("Smart Mode automation is busy; retry shortly")
	}
	// Restore both fields even if a command fails or the request disconnects.
	defer func() {
		a := d.write(15427784, 4, gate)
		b := d.write(15427780, 1, input)
		if a != nil || b != nil {
			err = errors.New("console restoration failed; stop controls and collect logs")
		}
	}()
	if e = d.write(15427780, 1, 1); e != nil {
		return before, e
	}
	if e = d.write(15427784, 4, 0); e != nil {
		return before, e
	}
	if e = d.cli(ctx, ""); e != nil {
		return before, e
	}
	if e = d.acknowledge(ctx, 3*time.Second); e != nil {
		return before, fmt.Errorf("console readiness: %w", e)
	}
	if action == "standard" {
		if before.Mode != "Smart" {
			return before, errors.New("unknown sound mode")
		}
		if e = d.cli(ctx, "mw.iom.se", "0x6f002", "3500"); e != nil {
			return before, e
		}
		deadline := time.Now().Add(5 * time.Second)
		ready := false
		for time.Now().Before(deadline) {
			if e := ctx.Err(); e != nil {
				return before, e
			}
			combo, _ := d.read(14200904, 4)
			repeat, _ := d.read(14200898, 1)
			if combo == 1 && repeat == 1 {
				ready = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !ready {
			return before, errors.New("mute hold was not confirmed")
		}
		if e = d.cli(ctx, "mw.iom.se", "0x6f000", "50"); e != nil {
			return before, e
		}
	} else {
		duration := "60"
		// Misc repeat callback: 400 ms initial delay, 200 ms repeats, 25 to pair.
		if action == "bluetooth_pair" {
			duration = "6000"
		}
		if e = d.cli(ctx, "mw.iom.se", keys[action], duration); e != nil {
			return before, e
		}
	}
	ackWait := 3 * time.Second
	if action == "bluetooth_pair" {
		ackWait = 9 * time.Second
	}
	if e = d.acknowledge(ctx, ackWait); e != nil {
		return before, fmt.Errorf("control completion: %w", e)
	}
	wait := 3 * time.Second
	if action == "bluetooth_pair" {
		wait = 9 * time.Second
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if e := ctx.Err(); e != nil {
			return before, e
		}
		if !d.same() {
			return before, errors.New("soundbar restarted during the request")
		}
		time.Sleep(100 * time.Millisecond)
		if d.idleKeys() {
			out = d.snapshotCurrent()
			if actionObserved(action, before, out) {
				return out, nil
			}
		}
	}
	return d.snapshotCurrent(), errors.New("change not confirmed (or volume limit reached); refresh before retrying")
}
func newRemote(cfg config) (*remote, error) {
	_, network, e := net.ParseCIDR(cfg.AllowedCIDR)
	if e != nil {
		return nil, e
	}
	r := &remote{cfg: cfg, network: network, device: &device{}}
	token := make([]byte, 32)
	if _, e = rand.Read(token); e != nil {
		return nil, e
	}
	r.csrf = hex.EncodeToString(token)
	return r, nil
}
func (r *remote) ServeHTTP(w http.ResponseWriter, q *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	client, _, e := net.SplitHostPort(q.RemoteAddr)
	if e != nil {
		http.Error(w, "Local network only", 403)
		return
	}
	target := q.Host
	if h, _, e := net.SplitHostPort(target); e == nil {
		target = h
	}
	var destination net.IP
	if address, ok := q.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if h, _, e := net.SplitHostPort(address.String()); e == nil {
			destination = net.ParseIP(h)
		}
	}
	var ap net.IP
	if !r.network.Contains(net.ParseIP(client)) {
		ap = hotspotAddress()
	}
	if !allowedNetwork(net.ParseIP(client), net.ParseIP(target), destination, r.network, ap) {
		http.Error(w, "Open the bar's local IP address from its trusted network", 403)
		return
	}
	if q.URL.Path == "/api/events" && q.Method == "GET" {
		r.stream(w, q)
		return
	}
	if q.URL.Path == "/api/state" && q.Method == "GET" {
		s := r.device.snapshot()
		s.CSRF = r.csrf
		writeJSON(w, 200, s)
		return
	}
	if q.URL.Path == "/api/action" && q.Method == "POST" {
		origin := q.Header.Get("Origin")
		if origin != "" && origin != "http://"+q.Host {
			http.Error(w, "Origin rejected", 403)
			return
		}
		if q.Header.Get("Content-Type") != "application/json" || subtle.ConstantTimeCompare([]byte(q.Header.Get("X-CSRF-Token")), []byte(r.csrf)) != 1 {
			http.Error(w, "Refresh the page before using controls", 403)
			return
		}
		var request struct {
			Action string `json:"action"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, q.Body, 256))
		dec.DisallowUnknownFields()
		if e := dec.Decode(&request); e != nil {
			http.Error(w, "Invalid control request", 400)
			return
		}
		if !r.actionMu.TryLock() {
			writeJSON(w, 409, map[string]string{"error": "Another control is running"})
			return
		}
		defer r.actionMu.Unlock()
		// Finish key-release/restore even if a phone closes the page mid-command.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s, e := r.device.action(ctx, request.Action)
		s.CSRF = r.csrf
		if e != nil {
			writeJSON(w, 409, map[string]any{"error": e.Error(), "state": s})
			return
		}
		r.feed.publish(s)
		writeJSON(w, 200, s)
		return
	}
	if q.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	path := map[string]string{"/": "www/index.html", "/app.js": "www/app.js", "/style.css": "www/style.css"}[q.URL.Path]
	if path == "" {
		http.NotFound(w, q)
		return
	}
	b, e := assets.ReadFile(path)
	if e != nil {
		http.Error(w, "Page unavailable", 500)
		return
	}
	if strings.HasSuffix(path, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else if strings.HasSuffix(path, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	w.Write(b)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func main() {
	path := flag.String("config", "/var/JBLRemote/config.json", "private configuration file")
	flag.Parse()
	b, e := os.ReadFile(*path)
	if e != nil {
		log.Fatal(e)
	}
	var cfg config
	if e = json.Unmarshal(b, &cfg); e != nil {
		log.Fatal(e)
	}
	r, e := newRemote(cfg)
	if e != nil {
		log.Fatal(e)
	}
	server := &http.Server{Addr: cfg.Listen, Handler: r, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	r.stop = ctx.Done()
	go r.feed.run(ctx, r.device.snapshot)
	go monitorWiFi(ctx)
	go monitorCast(ctx)
	stopped := make(chan struct{})
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 17*time.Second)
		defer cancel()
		server.Shutdown(c)
		close(stopped)
	}()
	addresses := []string{cfg.Listen}
	if cfg.Listen == ":80" {
		addresses = append(addresses, ":8080")
	} else if cfg.Listen == ":8080" {
		addresses = append(addresses, ":80")
	}
	var listeners []net.Listener
	for _, address := range addresses {
		listener, e := net.Listen("tcp", address)
		if e != nil {
			log.Printf("Listener unavailable on %s: %v", address, e)
			continue
		}
		listeners = append(listeners, listener)
		log.Printf("JBL remote listening on %s", address)
	}
	if len(listeners) == 0 {
		log.Fatal("No remote listener available")
	}
	finished := make(chan error, len(listeners))
	for _, listener := range listeners {
		go func(l net.Listener) { finished <- server.Serve(l) }(listener)
	}
	for range listeners {
		if e := <-finished; e != nil && e != http.ErrServerClosed {
			log.Printf("Listener stopped: %v", e)
		}
	}
	if ctx.Err() != nil {
		<-stopped
	}
}
