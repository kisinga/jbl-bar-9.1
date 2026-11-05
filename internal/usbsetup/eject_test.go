package usbsetup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareEjectAfterVerification(t *testing.T) {
	for _, format := range []bool{false, true} {
		for _, ejectFailure := range []bool{false, true} {
			t.Run(fmtName(format, ejectFailure), func(t *testing.T) {
				s, d := testServer(t)
				written := false
				s.Format = func(context.Context, Drive, string, string) error { written = true; return nil }
				calls := 0
				s.Eject = func(ctx context.Context, got Drive) error {
					calls++
					if got.ID != d.ID {
						t.Fatal("wrong USB")
					}
					if _, ok := ctx.Deadline(); !ok {
						t.Fatal("unbounded eject")
					}
					if format && !written {
						t.Fatal("ejected before format")
					}
					if !format {
						b, err := os.ReadFile(filepath.Join(d.Mount, entry))
						if err != nil || len(b) == 0 {
							t.Fatal("ejected before completed write", err)
						}
					}
					if ejectFailure {
						return errors.New("device busy")
					}
					return nil
				}
				confirm := d.Device
				if format {
					confirm = "ERASE " + confirm
				}
				w := call(s, "POST", "/api/prepare", map[string]any{"ID": d.ID, "Config": testConfig, "Confirm": confirm, "Format": format}, true)
				if w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), `"written":true`) {
					t.Fatal(w.Code, w.Body.String(), calls)
				}
				if ejectFailure && (!strings.Contains(w.Body.String(), `"ejected":false`) || !strings.Contains(w.Body.String(), "device busy")) {
					t.Fatal(w.Body.String())
				}
				if !ejectFailure && !strings.Contains(w.Body.String(), `"ejected":true`) {
					t.Fatal(w.Body.String())
				}
			})
		}
	}
}

func fmtName(format, failure bool) string {
	if format {
		if failure {
			return "format/eject-fails"
		}
		return "format/ejects"
	}
	if failure {
		return "copy/eject-fails"
	}
	return "copy/ejects"
}

func TestFailedWriteDoesNotEject(t *testing.T) {
	s, d := testServer(t)
	s.Eject = func(context.Context, Drive) error { t.Fatal("ejected failed write"); return nil }
	if err := os.WriteFile(filepath.Join(d.Mount, "unrelated.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if w := call(s, "POST", "/api/prepare", map[string]any{"ID": d.ID, "Config": testConfig, "Confirm": d.Device}, true); w.Code != 400 {
		t.Fatal(w.Body.String())
	}
	s.Format = func(context.Context, Drive, string, string) error { return errors.New("verification failed") }
	if w := call(s, "POST", "/api/prepare", map[string]any{"ID": d.ID, "Config": testConfig, "Confirm": "ERASE " + d.Device, "Format": true}, true); w.Code != 400 {
		t.Fatal(w.Body.String())
	}
}

func TestNativeEject(t *testing.T) {
	d := Drive{ID: "usb", Fingerprint: "same", Device: "/dev/sdz", Partition: "/dev/sdz1", Mount: "/media/usb", Size: 128 << 20}
	for _, platform := range []string{"linux", "darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			selected := d
			if platform == "darwin" {
				selected.Device = "/dev/disk9"
			}
			if platform == "windows" {
				selected.Device = "9"
				selected.Mount = `E:\`
			}
			calls := 0
			listCalls := 0
			list := func(context.Context) ([]Drive, error) {
				listCalls++
				if platform == "windows" && listCalls > 1 {
					return nil, nil
				}
				return []Drive{selected}, nil
			}
			run := func(_ context.Context, name string, args []string, _ string) ([]byte, error) {
				calls++
				joined := strings.Join(args, " ")
				if platform == "linux" {
					if calls == 1 && (name != "udisksctl" || joined != "unmount --block-device /dev/sdz1") {
						t.Fatal(name, joined)
					}
					if calls == 2 && joined != "power-off --block-device /dev/sdz" {
						t.Fatal(joined)
					}
				}
				if platform == "darwin" && joined != "eject /dev/disk9" {
					t.Fatal(joined)
				}
				if platform == "windows" && (!strings.Contains(joined, "InvokeVerb('Eject')") || !strings.Contains(joined, "Get-Disk -Number 9")) {
					t.Fatal(joined)
				}
				return nil, nil
			}
			if err := ejectDrive(context.Background(), selected, platform, run, list); err != nil {
				t.Fatal(err)
			}
			expected := 1
			if platform == "linux" {
				expected = 2
			}
			if calls != expected {
				t.Fatal(calls)
			}
			selected.Fingerprint = "replacement"
			original := d
			original.Device = selected.Device
			calls = 0
			if err := ejectDrive(context.Background(), original, platform, run, list); err == nil || calls != 0 {
				t.Fatal("replacement ejected", err)
			}
		})
	}
}
