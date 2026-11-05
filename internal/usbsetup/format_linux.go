//go:build linux

package usbsetup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/fat32"
	"github.com/diskfs/go-diskfs/partition/mbr"
)

func formatPlatform(ctx context.Context, d Drive, p *Package, config string) error {
	if !regexp.MustCompile(`^/dev/sd[a-z]+$`).MatchString(d.Device) {
		return errors.New("unsupported USB device path")
	}
	// No mkfs/parted installation is needed: diskfs writes FAT32 and MBR directly.
	mounts := append([]string{}, d.Mounts...)
	sort.Slice(mounts, func(i, j int) bool { return len(mounts[i]) > len(mounts[j]) })
	for _, mount := range mounts {
		if _, e := Run(ctx, "/usr/bin/umount", []string{"--", mount}, ""); e != nil {
			return e
		}
	}
	dd, e := diskfs.Open(d.Device, diskfs.WithOpenMode(diskfs.ReadWriteExclusive))
	if e != nil {
		return e
	}
	defer dd.Close()
	fresh, e := findDrive(ctx, d.ID)
	if e != nil {
		return e
	}
	if fresh.Fingerprint != d.Fingerprint || dd.Size != int64(d.Size) {
		return errors.New("USB changed before formatting")
	}
	return formatDisk(dd, p, config)
}

// Also exercised against ordinary image files: tests never format attached hardware.
func formatDisk(dd *disk.Disk, p *Package, config string) error {
	if dd.LogicalBlocksize != 512 || dd.Size < 64<<20 || dd.Size > 2<<40 {
		return errors.New("unsupported disk geometry")
	}
	sectors := uint32(dd.Size/512 - 2048)
	table := &mbr.Table{LogicalSectorSize: 512, PhysicalSectorSize: 512, Partitions: []*mbr.Partition{{Index: 1, Type: mbr.Fat32LBA, Start: 2048, Size: sectors}}}
	raw, e := dd.Backend.Sys()
	if e != nil {
		return e
	}
	// Clear old GPT headers at both ends before installing the new MBR.
	zero := make([]byte, 1<<20)
	if _, e = raw.WriteAt(zero, 0); e != nil {
		return e
	}
	if _, e = raw.WriteAt(zero, dd.Size-int64(len(zero))); e != nil {
		return e
	}
	if e = raw.Sync(); e != nil {
		return e
	}
	if e := dd.Partition(table); e != nil {
		return e
	}
	fs, e := dd.CreateFilesystem(disk.FilesystemSpec{Partition: 1, FSType: filesystem.TypeFat32, VolumeLabel: "JBLTOOLKIT"})
	if e != nil {
		return e
	}
	defer fs.Close()
	names := make([]string, 0, len(p.Files))
	for name := range p.Files {
		if name != entry {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	names = append(names, entry)
	// Absolute FAT paths distinguish AutoScript/AutoScript from its parent name.
	for _, name := range names {
		if dir := path.Dir(name); dir != "." {
			if e = fs.Mkdir("/" + dir); e != nil {
				return e
			}
		}
		data := p.Files[name]
		if name == configPath {
			data = []byte(config)
		}
		file, err := fs.OpenFile("/"+name, os.O_CREATE|os.O_RDWR|os.O_EXCL)
		if err != nil {
			return err
		}
		_, err = file.Write(data)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		// Flush before publishing AutoScript, as well as after each bounded file.
		raw, err := dd.Backend.Sys()
		if err != nil {
			return err
		}
		if err = raw.Sync(); err != nil {
			return err
		}
		check, err := fs.OpenFile("/"+name, os.O_RDONLY)
		if err != nil {
			return err
		}
		err = verifyFATFile(raw, check, data)
		closeErr = check.Close()
		if err != nil {
			return fmt.Errorf("formatted USB readback mismatch: %s: %w", strings.TrimSpace(name), err)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if e = fs.Close(); e != nil {
		return e
	}
	return raw.Sync()
}

// diskfs v1.8 can read cluster padding past EOF and discard backend read errors.
// Check the directory size, then read exactly the file's bytes from its disk ranges.
func verifyFATFile(raw io.ReaderAt, file filesystem.File, want []byte) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != int64(len(want)) {
		return fmt.Errorf("size %d, expected %d", info.Size(), len(want))
	}
	fat, ok := file.(*fat32.File)
	if !ok {
		return errors.New("expected FAT32 file")
	}
	ranges, err := fat.GetDiskRanges()
	if err != nil {
		return err
	}
	got := make([]byte, len(want))
	read := 0
	for _, r := range ranges {
		if read == len(got) {
			break
		}
		count := min(uint64(len(got)-read), r.Length)
		if count == 0 {
			continue
		}
		n, err := raw.ReadAt(got[read:read+int(count)], int64(r.Offset)+(1<<20))
		if err != nil {
			return err
		}
		if n != int(count) {
			return io.ErrUnexpectedEOF
		}
		read += n
	}
	if read != len(got) {
		return io.ErrUnexpectedEOF
	}
	if !bytes.Equal(got, want) {
		return errors.New("file contents differ")
	}
	return nil
}
