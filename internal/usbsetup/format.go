package usbsetup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type formatJob struct{ ID, Identity, Package, Config, Confirm string }

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func psQuote(s string) string    { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// The browser server stays unprivileged. Only this bounded one-shot operation elevates.
func ElevatedPrepare(ctx context.Context, d Drive, pack, config string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	exe, e = filepath.EvalSymlinks(exe)
	if e != nil {
		return e
	}
	dir, e := os.MkdirTemp("", "jbl-usb-prepare-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	data, e := json.Marshal(formatJob{d.ID, d.Fingerprint, pack, config, "ERASE " + d.Device})
	if e != nil {
		return e
	}
	job := filepath.Join(dir, "job.json")
	if e = os.WriteFile(job, data, 0600); e != nil {
		return e
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	args := []string{"--run-job", job, "--job-hash", digest}
	if runtime.GOOS != "windows" && os.Geteuid() == 0 {
		_, e = Run(ctx, exe, args, "")
		return e
	}
	switch runtime.GOOS {
	case "linux":
		helper, e := exec.LookPath("pkexec")
		if e != nil {
			return errors.New("formatting needs a desktop administrator prompt (pkexec); use an already formatted FAT32 USB if unavailable")
		}
		_, e = Run(ctx, helper, append([]string{exe}, args...), "")
		return e
	case "darwin":
		command := shellQuote(exe)
		for _, a := range args {
			command += " " + shellQuote(a)
		}
		_, e = Run(ctx, "/usr/bin/osascript", []string{"-e", "do shell script " + strconv.Quote(command) + " with administrator privileges"}, "")
		return e
	case "windows":
		var parts []string
		for _, a := range args {
			parts = append(parts, strconv.Quote(a))
		}
		script := "$p=Start-Process -FilePath " + psQuote(exe) + " -ArgumentList " + psQuote(strings.Join(parts, " ")) + " -Verb RunAs -Wait -PassThru; exit $p.ExitCode"
		_, e = Run(ctx, "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", script}, "")
		return e
	default:
		return errors.New("formatting is unsupported on this platform")
	}
}

func RunFormatJob(filename, digest string) error {
	f, e := os.Open(filename)
	if e != nil {
		return e
	}
	data, e := io.ReadAll(io.LimitReader(f, 16385))
	f.Close()
	if e != nil {
		return e
	}
	if len(data) > 16384 {
		return errors.New("format request exceeds limit")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return errors.New("format request changed before elevation")
	}
	var job formatJob
	if e = json.Unmarshal(data, &job); e != nil {
		return e
	}
	if _, e = ParseConfig(job.Config); e != nil {
		return e
	}
	p, e := LoadPackage(job.Package)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	d, e := findDrive(ctx, job.ID)
	if e != nil {
		return e
	}
	if d.Fingerprint != job.Identity || job.Confirm != "ERASE "+d.Device || d.ReadOnly || d.Size < 64<<20 || d.Size > 2<<40 {
		return errors.New("drive identity or formatting eligibility changed")
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	for _, mount := range d.Mounts {
		for _, name := range []string{exe, filename, job.Package} {
			abs, e := filepath.EvalSymlinks(name)
			if e != nil {
				return e
			}
			rel, e := filepath.Rel(mount, abs)
			if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return errors.New("run setup and keep its package on the computer, not on the USB being formatted")
			}
		}
	}
	return formatPlatform(ctx, d, p, job.Config)
}

func nativeFormat(ctx context.Context, d Drive, p *Package, config, platform string, run RunFunc, list func(context.Context) ([]Drive, error)) error {
	if platform == "windows" {
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(d.Device) {
			return errors.New("invalid disk identifier")
		}
		size := "-UseMaximumSize"
		if d.Size > 32<<30 {
			size = "-Size 30GB"
		}
		script := fmt.Sprintf(`$ErrorActionPreference='Stop'; $d=Get-Disk -Number %s; if($d.BusType -ne 'USB' -or $d.IsBoot -or $d.IsSystem -or $d.IsReadOnly -or $d.Size -ne %d -or ([string]$d.SerialNumber).Trim() -ne %s){throw 'Disk is not an eligible USB'}; $d | Clear-Disk -RemoveData -RemoveOEM -Confirm:$false; Initialize-Disk -Number %s -PartitionStyle MBR; New-Partition -DiskNumber %s %s -AssignDriveLetter | Format-Volume -FileSystem FAT32 -NewFileSystemLabel JBLTOOLKIT -Confirm:$false | Out-Null`, d.Device, d.Size, psQuote(strings.TrimSpace(d.Serial)), d.Device, d.Device, size)
		if _, e := run(ctx, "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", script}, ""); e != nil {
			return e
		}
	} else if platform == "darwin" {
		if !regexp.MustCompile(`^/dev/disk[0-9]+$`).MatchString(d.Device) {
			return errors.New("invalid disk identifier")
		}
		if _, e := run(ctx, "/usr/sbin/diskutil", []string{"eraseDisk", "FAT32", "JBLTOOLKIT", "MBRFormat", d.Device}, ""); e != nil {
			return e
		}
	} else {
		return errors.New("unknown native formatter")
	}
	for i := 0; i < 30; i++ {
		drives, e := list(ctx)
		if e != nil {
			return e
		}
		for _, v := range drives {
			if v.ID == d.ID && v.Fingerprint == d.Fingerprint && v.Eligible {
				root, e := openDrive(v)
				if e != nil {
					return e
				}
				defer root.Close()
				return p.Apply(root, config)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.New("formatted USB did not remount; reconnect it and retry preparation without formatting")
}
