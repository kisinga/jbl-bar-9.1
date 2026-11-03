package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// BusyBox on the supported firmware lacks mv -T. renameat2 with
// RENAME_NOREPLACE publishes the complete store without replacing any existing
// entry or moving the stage inside a directory created by a competing writer.
func publishDirectory(source, target string) error {
	source, target = filepath.Clean(source), filepath.Clean(target)
	if !filepath.IsAbs(source) || !filepath.IsAbs(target) || filepath.Dir(source) != filepath.Dir(target) ||
		!stringsHasStagePrefix(source, target) {
		return errors.New("publication requires adjacent absolute target and target.stage.* paths")
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("publication source must be a real directory")
	}
	var number uintptr
	switch runtime.GOARCH {
	case "arm":
		number = 382
	case "amd64":
		number = 316
	default:
		return errors.New("directory publication is not qualified for this architecture")
	}
	old, err := syscall.BytePtrFromString(source)
	if err != nil {
		return err
	}
	new, err := syscall.BytePtrFromString(target)
	if err != nil {
		return err
	}
	// AT_FDCWD = -100, RENAME_NOREPLACE = 1. Never fall back to overwriting
	// rename or directory-merging mv on an unsupported kernel/filesystem.
	atFDCWD := ^uintptr(99)
	_, _, errno := syscall.Syscall6(number, atFDCWD, uintptr(unsafe.Pointer(old)), atFDCWD,
		uintptr(unsafe.Pointer(new)), 1, 0)
	runtime.KeepAlive(old)
	runtime.KeepAlive(new)
	if errno != 0 {
		return errno
	}
	return nil
}

func stringsHasStagePrefix(source, target string) bool {
	prefix := target + ".stage."
	return len(source) > len(prefix) && source[:len(prefix)] == prefix
}
