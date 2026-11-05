//go:build linux

package usbsetup

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
)

func TestFAT32ImageFormattingAndReadback(t *testing.T) {
	for _, size := range []int64{128 << 20, 64 << 30} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			testFAT32Image(t, size)
		})
	}
}

func testFAT32Image(t *testing.T, size int64) {
	name := filepath.Join(t.TempDir(), "usb.img")
	f, e := os.Create(name)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Truncate(size); e != nil {
		t.Fatal(e)
	}
	f.Close()
	dd, e := diskfs.Open(name, diskfs.WithOpenMode(diskfs.ReadWriteExclusive))
	if e != nil {
		t.Fatal(e)
	}
	p, e := LoadPackage(fixtureZip(t, fixtureFiles()))
	if e != nil {
		t.Fatal(e)
	}
	if e = formatDisk(dd, p, testConfig); e != nil {
		dd.Close()
		t.Fatal(e)
	}

	dd.Close()
	if checker, e := exec.LookPath("fsck.fat"); e == nil && size == 128<<20 {
		source, e := os.Open(name)
		if e != nil {
			t.Fatal(e)
		}
		defer source.Close()
		source.Seek(1<<20, io.SeekStart)
		part, e := os.Create(filepath.Join(t.TempDir(), "partition.img"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = io.Copy(part, source); e != nil {
			t.Fatal(e)
		}
		part.Close()
		if out, e := exec.Command(checker, "-n", part.Name()).CombinedOutput(); e != nil {
			t.Fatalf("independent FAT32 validation: %v\n%s", e, out)
		}
	}
	dd, e = diskfs.Open(name, diskfs.WithOpenMode(diskfs.ReadOnly))
	if e != nil {
		t.Fatal(e)
	}
	defer dd.Close()
	table, e := dd.GetPartitionTable()
	if e != nil || table.Type() != "mbr" {
		t.Fatal(table, e)
	}
	fs, e := dd.GetFilesystem(1)
	if e != nil {
		t.Fatal(e)
	}
	defer fs.Close()
	for n, want := range p.Files {
		file, e := fs.OpenFile("/"+n, os.O_RDONLY)
		if e != nil {
			t.Fatal(n, e)
		}
		info, e := file.Stat()
		if e != nil || info.Size() != int64(len(want)) {
			t.Fatalf("size %s: %v", n, e)
		}
		got := make([]byte, len(want))
		_, e = io.ReadFull(file, got)
		file.Close()
		if e != nil || !bytes.Equal(got, want) {
			t.Fatal("readback", n, e)
		}
	}
}

type failingReadback struct{}

func (failingReadback) ReadAt([]byte, int64) (int, error) {
	return 0, errors.New("readback I/O failed")
}

func TestFATReadbackRejectsDiskErrorsAndSizeMismatch(t *testing.T) {
	name := filepath.Join(t.TempDir(), "usb.img")
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(128 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	dd, err := diskfs.Open(name, diskfs.WithOpenMode(diskfs.ReadWriteExclusive))
	if err != nil {
		t.Fatal(err)
	}
	defer dd.Close()
	p, err := LoadPackage(fixtureZip(t, fixtureFiles()))
	if err != nil {
		t.Fatal(err)
	}
	if err = formatDisk(dd, p, testConfig); err != nil {
		t.Fatal(err)
	}
	fs, err := dd.GetFilesystem(1)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	file, err := fs.OpenFile("/JBLToolkit/SHA256SUMS", os.O_RDONLY)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	want := p.Files["JBLToolkit/SHA256SUMS"]
	if err = verifyFATFile(failingReadback{}, file, want); err == nil || !strings.Contains(err.Error(), "I/O failed") {
		t.Fatal("disk read failure ignored", err)
	}
	raw, err := dd.Backend.Sys()
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyFATFile(raw, file, want[:len(want)-1]); err == nil {
		t.Fatal("file size mismatch ignored")
	}
	corrupt := bytes.Clone(want)
	corrupt[0] ^= 1
	if err = verifyFATFile(raw, file, corrupt); err == nil {
		t.Fatal("changed bytes ignored")
	}
}
