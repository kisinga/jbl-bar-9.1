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

var bluetoothLibraries = map[string]string{
	"/system/bdplib/ld-linux.so.3":               "49343df4311cb08d47dce9705eebcd06b666509d797ea018ef7c3142cb1f8af9",
	"/system/bdplib/libc.so.6":                   "2f3952341c498325cc9423acadcf7c2ad94ee9196af071c661ffd16057552dc0",
	"/system/bdplib/libdl.so.2":                  "a90018084bd508ce2a1a8d4a581cb3caebb0dfcd357f60b9e16a137850a064c6",
	"/system/bdplib/libgcc_s.so.1":               "8cefc5883637998effb2b60cf89115692fcfea32845892e4e3b3570b036190d1",
	"/system/bdplib/libipcrpc.so":                "49e4207e59115e4da6ea9e916e1a0981495c606cff331fd34bd85ad0d2b2dea9",
	"/system/bdplib/libm.so.6":                   "8a5316e5d611f775819f77509014278d06335b03d8569cf2ff9dafd2e1e104c5",
	"/system/bdplib/libmtk_bt_ipcrpc_struct.so":  "3e2373e0ceba593dee8109910ad96bcc3482e15fe258c9b57090d6a01a9c6f52",
	"/system/bdplib/libmtk_bt_service_client.so": "a54bf7a2f20b3cee2cb01d8ad33768e50d6f2b5a851d9db6515d1a0de527e414",
	"/system/bdplib/libpthread.so.0":             "a2d3c817beb9869435dec5ab14893f9073829711c1ba56d6bd83882342b0d7e1",
	"/system/bdplib/libresolv.so.2":              "78497b7d61abbccf94b75a33eb4285bfed0cbd4ab4461730fd356a33ca853e1a",
	"/system/bdplib/librt.so.1":                  "7113c86d037bcbf31cad3feff582a5d53918d4ff50e984a2945e7c6124257c95",
	"/system/bdplib/libstdc++.so.6":              "59a59e75b7ad46d45b239411ada16818b56f48803c6e25195cdcd266ed86a67d",
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
