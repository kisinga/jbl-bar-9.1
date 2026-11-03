package main

import (
	"errors"
	"os"
	"strings"
)

// The installer probe reads a bounded native snapshot. It never starts the
// server, automation, discovery, helper executables or any control action.
type installCheck struct {
	Schema      int      `json:"schema"`
	Supported   bool     `json:"supported"`
	BootID      string   `json:"boot_id,omitempty"`
	Filesystems []string `json:"kernel_filesystems"`
	Power       string   `json:"power"`
	Mode        string   `json:"mode"`
	Volume      *uint32  `json:"volume,omitempty"`
	Muted       *bool    `json:"muted,omitempty"`
	Source      *uint32  `json:"source,omitempty"`
	Bluetooth   bool     `json:"bluetooth_helper"`
	FrontEQ     bool     `json:"front_eq_helper"`
	Error       string   `json:"error,omitempty"`
}

func inspectInstallation(requireAwake bool) (installCheck, error) {
	r := installCheck{Schema: 1, Power: "Unknown", Mode: "Unknown",
		Bluetooth: bluetoothPayload != "", FrontEQ: eqParserPayload != ""}
	if b, err := os.ReadFile("/proc/filesystems"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 1 {
				r.Filesystems = append(r.Filesystems, fields[0])
			}
		}
	}
	if b, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil {
		r.BootID = strings.TrimSpace(string(b))
	}
	d := &device{}
	if err := d.identify(); err != nil {
		return r, err
	}
	if hash, err := hashFile("/bin/cli"); err != nil || hash != cliHash {
		return r, errors.New("unsupported native CLI")
	}
	power, ep := d.read(14191893, 1)
	system, es := d.read(12962676, 4)
	mode, em := d.read(13378532, 4)
	volume, ev := d.configByte(50)
	mute, eu := d.configByte(155)
	source, ei := d.configByte(365)
	if ep != nil || es != nil || em != nil || ev != nil || eu != nil || ei != nil || !d.same() {
		return r, errors.New("native state unreadable or firmware process changed")
	}
	if power == 1 && system == 0 {
		r.Power = "On"
	} else if power == 0 || system == 1 || system == 2 || system == 3 {
		r.Power = "Standby"
	}
	if mode == 0 {
		r.Mode = "Standard"
	} else if mode == 3 {
		r.Mode = "Smart"
	}
	if r.Power == "Unknown" || r.Mode == "Unknown" || volume > 32 || mute > 1 {
		return r, errors.New("unexpected native state; installation not qualified")
	}
	muted := mute != 0
	r.Volume, r.Muted, r.Source = &volume, &muted, &source
	for _, group := range []struct {
		enabled bool
		files   map[string]string
	}{{r.Bluetooth, bluetoothLibraries}, {r.FrontEQ, eqParserDependencies}} {
		if !group.enabled {
			continue
		}
		for path, expected := range group.files {
			if actual, err := hashFile(path); err != nil || actual != expected {
				return r, errors.New("bundled native helper has an unsupported library")
			}
		}
	}
	r.Supported = true
	if requireAwake && r.Power != "On" {
		return r, errors.New("turn the soundbar fully on before installation")
	}
	return r, nil
}
