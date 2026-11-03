package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Optional owner-built helper, embedded by tools/build_web.py. The helper uses
// the existing native AVRCP service without replacing its callback subscriber.
var bluetoothPayload string
var bluetoothBridge struct {
	sync.Once
	path      string
	directory string
}

func bluetoothBridgePath() string {
	bluetoothBridge.Do(func() {
		if bluetoothPayload == "" {
			return
		}
		b, e := base64.StdEncoding.DecodeString(bluetoothPayload)
		if e != nil || len(b) > 65536 || len(b) < 52 || !bytes.Equal(b[:7], []byte{127, 'E', 'L', 'F', 1, 1, 1}) || b[18] != 40 || b[19] != 0 {
			return
		}
		for path, want := range bluetoothLibraries {
			if got, e := hashFile(path); e != nil || got != want {
				return
			}
		}
		dir, e := os.MkdirTemp("/tmp", "jbl-avrcp-")
		if e != nil {
			return
		}
		path := filepath.Join(dir, "bt-remote")
		if e = os.WriteFile(path, b, 0700); e != nil {
			os.RemoveAll(dir)
			return
		}
		bluetoothBridge.path, bluetoothBridge.directory = path, dir
	})
	return bluetoothBridge.path
}
func closeBluetoothBridge() {
	if bluetoothBridge.directory != "" {
		_ = os.RemoveAll(bluetoothBridge.directory)
	}
}

type bluetoothRPCResult struct {
	CheckOnly           *bool `json:"check_only"`
	InitResult          *int  `json:"init_result"`
	Connected           bool  `json:"connected"`
	CloseResult         *int  `json:"close_result"`
	PressResult         *int  `json:"press_result"`
	ReleaseResult       *int  `json:"release_result"`
	CallbacksRegistered *bool `json:"callbacks_registered"`
}

func bluetoothRPCSucceeded(b []byte) bool {
	var r bluetoothRPCResult
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if dec.Decode(&r) != nil {
		return false
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return false
	}
	return r.CheckOnly != nil && !*r.CheckOnly && r.Connected && r.CallbacksRegistered != nil && !*r.CallbacksRegistered && r.InitResult != nil && *r.InitResult == 0 && r.CloseResult != nil && *r.CloseResult == 0 && r.PressResult != nil && *r.PressResult == 0 && r.ReleaseResult != nil && *r.ReleaseResult == 0
}
func (d *device) bluetoothAction(ctx context.Context, action string, before state) (state, error) {
	path := bluetoothBridgePath()
	current := d.snapshotCurrent()
	if path == "" || !d.same() || !mediaAllowed(action, current) || current.Bluetooth == nil || before.Bluetooth == nil || before.Bluetooth.peer == "" || current.Bluetooth.peer != before.Bluetooth.peer {
		return current, errors.New("Bluetooth peer or capability changed; refresh before retrying")
	}
	name, ok := strings.CutPrefix(action, "media_")
	switch name {
	case "play", "pause", "stop", "next", "previous":
	default:
		ok = false
	}
	if !ok {
		return current, errors.New("unsupported Bluetooth control")
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, path, name, current.Bluetooth.peer)
	cmd.Env = []string{"PATH=/bin:/usr/bin", "LD_LIBRARY_PATH=/system/bdplib:/system/usr/lib:/system/usr/local/lib"}
	// Native diagnostics stay private and cannot grow an HTTP response.
	var result bytes.Buffer
	cmd.Stdout = &result
	cmd.Stderr = io.Discard
	err := cmd.Run()
	out := d.snapshotCurrent()
	if err != nil || result.Len() > 1024 || !bluetoothRPCSucceeded(result.Bytes()) {
		return out, errors.New("Bluetooth request not confirmed; check playback before retrying")
	}
	if !d.same() || out.SourceState == nil || *out.SourceState != 2 || out.Bluetooth == nil || out.Bluetooth.peer != current.Bluetooth.peer {
		return out, errors.New("Bluetooth source changed during the request; check playback before retrying")
	}
	// RPC acceptance confirms a request, not playback in the sending app. Some
	// computers leave A2DP Streaming even after AVRCP successfully pauses audio.
	return out, nil
}
