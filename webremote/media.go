package main

import (
	"fmt"
	"os"
)

func (d *device) bluetoothMediaReady(peer string) bool {
	return bluetoothBridgePath() != "" && d.bluetoothPeerMediaReady(peer)
}

func (d *device) bluetoothPeerMediaReady(peer string) bool {
	state, e := d.read(0xcb6668+0x44, 4)
	if e != nil || state != 0xe3758a {
		return false
	}
	connected, e := d.read(int64(state)+1, 1)
	if e != nil || connected != 1 {
		return false
	}
	state, e = d.read(0xcb6b40+0x44, 4)
	if e != nil || state != 0xe37590 {
		return false
	}
	info, e := d.read(0xcb6b40+0x4c, 4)
	if e != nil || info != 0xcb6cbc {
		return false
	}
	f, e := os.Open(fmt.Sprintf("/proc/%d/mem", d.pid))
	if e != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 32)
	if _, e = f.ReadAt(b, int64(info)+4); e != nil {
		return false
	}
	return peer != "" && peerAddress(b) == peer
}

func mediaCommands(s state) []string {
	if !s.Available || s.Power != "On" || s.SourceState == nil {
		return nil
	}
	if *s.SourceState == 2 && s.Bluetooth != nil && s.Bluetooth.Selected && s.Bluetooth.Connected != nil && *s.Bluetooth.Connected && s.Bluetooth.MediaReady {
		return []string{"media_previous", "media_play", "media_pause", "media_next", "media_stop"}
	}
	if *s.SourceState != 6 || s.Streaming == nil || s.Streaming.Cast == nil {
		return nil
	}
	c := s.Streaming.Cast
	if !c.Available || c.Mirroring || !c.HasMedia {
		return nil
	}
	var commands []string
	if c.SupportedCommands&64 != 0 {
		commands = append(commands, "media_next")
	}
	if c.SupportedCommands&128 != 0 {
		commands = append(commands, "media_previous")
	}
	if c.PlayerState == "PLAYING" && s.Streaming.native == 2 && c.SupportedCommands&1 != 0 {
		commands = append(commands, "media_pause")
	}
	if c.PlayerState == "PAUSED" && s.Streaming.native == 3 {
		commands = append(commands, "media_play")
	}
	if (c.PlayerState == "PLAYING" && s.Streaming.native == 2) || (c.PlayerState == "PAUSED" && s.Streaming.native == 3) {
		commands = append(commands, "media_stop")
	}
	return commands
}
func mediaAllowed(action string, s state) bool {
	for _, command := range mediaCommands(s) {
		if command == action {
			return true
		}
	}
	return false
}
func mediaObserved(action string, s state) bool {
	if s.SourceState == nil {
		return false
	}
	if *s.SourceState == 6 && s.Streaming != nil && s.Streaming.Cast != nil && !s.Streaming.Cast.Mirroring {
		return action == "media_play" && s.Streaming.Cast.PlayerState == "PLAYING" && s.Streaming.native == 2 || action == "media_pause" && s.Streaming.Cast.PlayerState == "PAUSED" && s.Streaming.native == 3
	}
	return false
}
