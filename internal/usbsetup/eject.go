package usbsetup

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Eject runs only after all writes and verification handles have closed.
func Eject(ctx context.Context, selected Drive) error {
	return ejectDrive(ctx, selected, runtime.GOOS, Run, Discover)
}

func ejectDrive(ctx context.Context, selected Drive, platform string, run RunFunc, list func(context.Context) ([]Drive, error)) error {
	drives, err := list(ctx)
	if err != nil {
		return err
	}
	var d Drive
	for _, candidate := range drives {
		if candidate.ID == selected.ID && candidate.Fingerprint == selected.Fingerprint {
			d = candidate
			break
		}
	}
	if d.ID == "" {
		return errors.New("USB changed or was removed before eject")
	}
	switch platform {
	case "linux":
		if !regexp.MustCompile(`^/dev/sd[a-z]+$`).MatchString(d.Device) {
			return errors.New("unsupported USB device path")
		}
		if d.Mount != "" {
			if !regexp.MustCompile(`^/dev/sd[a-z]+1$`).MatchString(d.Partition) || d.Partition != d.Device+"1" {
				return errors.New("USB partition changed before eject")
			}
			if _, err = run(ctx, "udisksctl", []string{"unmount", "--block-device", d.Partition}, ""); err != nil {
				return err
			}
		}
		_, err = run(ctx, "udisksctl", []string{"power-off", "--block-device", d.Device}, "")
		return err
	case "darwin":
		if !regexp.MustCompile(`^/dev/disk[0-9]+$`).MatchString(d.Device) {
			return errors.New("unsupported USB device path")
		}
		_, err = run(ctx, "/usr/sbin/diskutil", []string{"eject", d.Device}, "")
		return err
	case "windows":
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(d.Device) || !regexp.MustCompile(`^[A-Za-z]:\\?$`).MatchString(d.Mount) {
			return errors.New("USB has no ejectable drive letter")
		}
		script := fmt.Sprintf(`$ErrorActionPreference='Stop'; $d=Get-Disk -Number %s; if($d.BusType -ne 'USB' -or $d.IsBoot -or $d.IsSystem -or $d.Size -ne %d -or ([string]$d.SerialNumber).Trim() -ne %s){throw 'USB changed before eject'}; $p=Get-Partition -DiskNumber %s | Where-Object DriveLetter -eq '%s'; if(-not $p){throw 'USB drive letter changed'}; $item=(New-Object -ComObject Shell.Application).Namespace(17).ParseName('%s'); if(-not $item){throw 'USB eject unavailable'}; $item.InvokeVerb('Eject')`, d.Device, d.Size, psQuote(strings.TrimSpace(d.Serial)), d.Device, d.Mount[:1], d.Mount[:2])
		if _, err = run(ctx, "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", script}, ""); err != nil {
			return err
		}
		// Shell eject is asynchronous and has no return status. Confirm removal.
		for {
			drives, err = list(ctx)
			if err != nil {
				return err
			}
			mounted := false
			for _, candidate := range drives {
				if candidate.ID == d.ID && (candidate.Mount != "" || len(candidate.Mounts) > 0) {
					mounted = true
				}
			}
			if !mounted {
				return nil
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("USB eject was not confirmed: %w", ctx.Err())
			case <-time.After(250 * time.Millisecond):
			}
		}
	default:
		return errors.New("automatic eject is unavailable on this platform")
	}
}
