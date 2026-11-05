package buildtool

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"jbl-bar-toolkit/internal/usbsetup"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	return root
}
func TestProfileAndOutputGuards(t *testing.T) {
	root := repoRoot(t)
	b := &Builder{Root: root}
	if _, e := b.checkedProfile(); e != nil {
		t.Fatal(e)
	}
	for _, out := range []string{root, filepath.Dir(root), filepath.Join(root, "dist"), filepath.Join(root, "build/../../escape")} {
		if _, e := b.safeOutput(out); e == nil {
			t.Fatal("unsafe output accepted", out)
		}
	}
	if _, e := b.Payload(context.Background(), filepath.Join(root, "dist/invalid-model"), "bar1300"); e == nil {
		t.Fatal("unsupported model accepted")
	}
	temp := t.TempDir()
	copy := &Builder{Root: temp}
	if e := os.MkdirAll(filepath.Join(temp, "build"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(t.TempDir(), filepath.Join(temp, "build/escape")); e == nil {
		if _, e = copy.safeOutput(filepath.Join(temp, "build/escape/output")); e == nil {
			t.Fatal("output escaped via symlink")
		}
	}
	out := filepath.Join(temp, "build/owned")
	if _, e := copy.emptyOutput(out); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(out, "keep"), []byte("keep"), 0600)
	if _, e := copy.emptyOutput(out); e == nil {
		t.Fatal("existing output accepted")
	}
}
func TestZIPAndManifest(t *testing.T) {
	files := map[string][]byte{"jbl-setup": []byte("exe"), "toolkit-usb.zip": []byte("data")}
	out := filepath.Join(t.TempDir(), "offline.zip")
	if e := writeZip(out, files, "jbl-setup"); e != nil {
		t.Fatal(e)
	}
	if e := writeZip(out, files, "jbl-setup"); e == nil {
		t.Fatal("existing ZIP overwritten")
	}
	z, e := zip.OpenReader(out)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	for _, f := range z.File {
		r, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		data, _ := io.ReadAll(r)
		r.Close()
		if !bytes.Equal(data, files[f.Name]) {
			t.Fatal("bad ZIP content")
		}
		mode := os.FileMode(0644)
		if f.Name == "jbl-setup" {
			mode = 0755
		}
		if f.Mode().Perm() != mode {
			t.Fatal("bad permissions", f.Name, f.Mode())
		}
	}
	if !strings.Contains(string(manifest(files)), fmt.Sprintf("%x  jbl-setup\n", sha256.Sum256(files["jbl-setup"]))) {
		t.Fatal("bad manifest")
	}
}

func TestPublicPayloadAndNativeExecutable(t *testing.T) {
	if testing.Short() {
		t.Skip("source build integration")
	}
	root := repoRoot(t)
	builder, e := New(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(filepath.Join(root, "dist"), 0700)
	work, e := os.MkdirTemp(filepath.Join(root, "dist"), "go builder test-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(work)
	paths, e := builder.Payload(context.Background(), filepath.Join(work, "payload"), "auto")
	if e != nil {
		t.Fatal(e)
	}
	payload, e := usbsetup.LoadPackage(paths[0])
	if e != nil {
		t.Fatal(e)
	}
	if !payload.FrontEQ || !payload.Bluetooth {
		t.Fatal("public setup lost EQ or Bluetooth support")
	}
	if string(payload.Files["JBLToolkit/config.txt"]) != PublicConfig {
		t.Fatal("public configuration changed")
	}
	if e = usbsetup.VerifyRecovery(paths[1], payload); e != nil {
		t.Fatal(e)
	}
	if _, e = builder.Payload(context.Background(), filepath.Join(work, "payload"), "auto"); e == nil {
		t.Fatal("existing payload overwritten")
	}
	binary := filepath.Join(work, usbsetup.BinaryName(runtime.GOOS, runtime.GOARCH))
	if e = builder.Executable(context.Background(), binary, "dev", runtime.GOOS, runtime.GOARCH); e != nil {
		t.Fatal(e)
	}
	output, e := exec.Command(binary, "--version").Output()
	if e != nil || strings.TrimSpace(string(output)) != "dev" {
		t.Fatal("native executable did not run", e, string(output))
	}
	if e = builder.Executable(context.Background(), binary, "dev", runtime.GOOS, runtime.GOARCH); e == nil {
		t.Fatal("existing executable overwritten")
	}
}

func TestNativeHelpersRejectStaleSourceAndBinaries(t *testing.T) {
	root := repoRoot(t)
	b := &Builder{Root: root}
	profile, err := b.checkedProfile()
	if err != nil {
		t.Fatal(err)
	}
	bt, eq, err := b.nativeHelpers(*profile)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(bt, eq) {
		t.Fatal("different helpers expected")
	}
	files := []string{"models/bar91/remote/native/bin/NOTICE.txt", "models/bar91/remote/native/bin/bt-remote", "models/bar91/remote/native/bin/eq-parser", "models/bar91/remote/native/bin/manifest.json", "models/bar91/remote/native/bt_remote.c", "models/bar91/remote/native/eq_parser.c", "tools/build_bt.py", "tools/build_eq.py"}
	for _, changed := range files {
		t.Run(changed, func(t *testing.T) {
			folder := t.TempDir()
			for _, name := range files {
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil {
					t.Fatal(err)
				}
				if name == changed {
					data = append(data, 'x')
				}
				target := filepath.Join(folder, name)
				if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(target, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := (&Builder{Root: folder}).nativeHelpers(*profile); err == nil {
				t.Fatal("changed native input accepted")
			}
		})
	}
	for _, data := range [][]byte{nil, bytes.Repeat([]byte{0}, 65537), bytes.ReplaceAll(eq, []byte("/system/bdplib/ld-linux.so.3"), []byte("/system/bdplib/ld-linux.so.X"))} {
		if err := validateNativeHelper(data); err == nil {
			t.Fatal("unqualified ELF accepted")
		}
	}
}
