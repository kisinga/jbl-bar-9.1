package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMediaCommandsFollowSourceAndCapabilities(t *testing.T) {
	source := uint32(2)
	connected := true
	s := state{Available: true, Power: "On", SourceState: &source, Bluetooth: &bluetoothState{Selected: true, Connected: &connected, MediaReady: true, Audio: "Streaming"}}
	if !mediaAllowed("media_pause", s) || !mediaAllowed("media_play", s) || !mediaAllowed("media_next", s) {
		t.Fatal(mediaCommands(s))
	}
	s.Bluetooth.Audio = "Paused"
	if !mediaAllowed("media_play", s) || !mediaAllowed("media_pause", s) {
		t.Fatal(mediaCommands(s))
	}
	if mediaObserved("media_pause", s) || mediaObserved("media_play", s) {
		t.Fatal("A2DP stream state claimed app playback confirmation")
	}
	s.Bluetooth.MediaReady = false
	if len(mediaCommands(s)) != 0 {
		t.Fatal("A2DP connection alone enabled AVRCP")
	}
	s.Bluetooth.MediaReady = true
	s.Power = "Standby"
	if len(mediaCommands(s)) != 0 {
		t.Fatal("standby media controls enabled")
	}
	s.Power = "On"
	source = 0
	if len(mediaCommands(s)) != 0 {
		t.Fatal("background Bluetooth stole TV controls")
	}
	source = 6
	c := CastSnapshot{Available: true, HasMedia: true, PlayerState: "PLAYING", SupportedCommands: 1 | 64}
	s.Streaming = &streamState{Cast: &c, native: 2}
	if !mediaAllowed("media_pause", s) || !mediaAllowed("media_next", s) || mediaAllowed("media_previous", s) {
		t.Fatal(mediaCommands(s))
	}
	c.Mirroring = true
	if len(mediaCommands(s)) != 0 {
		t.Fatal("mirroring offered sender controls")
	}
	c.Mirroring = false
	s.Streaming.native = 3
	if mediaAllowed("media_pause", s) {
		t.Fatal("Cast receiver state overrode native dispatch guard")
	}
	c.PlayerState = "PAUSED"
	if !mediaAllowed("media_play", s) || !mediaObserved("media_pause", s) {
		t.Fatal(mediaCommands(s))
	}
	c.HasMedia = false
	if len(mediaCommands(s)) != 0 {
		t.Fatal("missing media session accepted")
	}
}

func TestNightModeRequiresChangedReadback(t *testing.T) {
	off, on := false, true
	before := state{Available: true, Sound: &soundState{Night: &off}}
	after := state{Available: true, Sound: &soundState{Night: &on}}
	if !actionObserved("night_toggle", before, after) || actionObserved("night_toggle", before, before) {
		t.Fatal("night toggle claimed without readback")
	}
	after.Sound.Night = nil
	if actionObserved("night_toggle", before, after) {
		t.Fatal("unknown night state accepted")
	}
}

func TestLocalAliasKeepsNetworkAndOriginChecks(t *testing.T) {
	r := testRemote(t)
	for _, c := range []struct {
		host, client, destination string
		want                      int
	}{
		{"jbl.local", "192.168.50.2", "192.168.50.1", 200},
		{"JBL.LOCAL:80", "192.168.50.2", "192.168.50.1", 200},
		{"jbl.local.", "192.168.50.2", "192.168.50.1", 200},
		{"jbl.local.attacker.invalid", "192.168.50.2", "192.168.50.1", 403},
		{"jbl.local", "10.20.30.40", "192.168.50.1", 403},
		{"jbl.local", "192.168.50.2", "10.20.30.40", 403},
		{"jbl.local", "192.168.50.2", "", 403},
	} {
		q := httptest.NewRequest("GET", "http://example.invalid/", nil)
		q.Host = c.host
		q.RemoteAddr = net.JoinHostPort(c.client, "1234")
		if c.destination != "" {
			q = q.WithContext(context.WithValue(q.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP(c.destination), Port: 80}))
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		if w.Code != c.want {
			t.Fatalf("%+v: %d %s", c, w.Code, w.Body.String())
		}
	}
	q := httptest.NewRequest("POST", "http://jbl.local/api/action", strings.NewReader(`{"action":"mute"}`))
	q.Host = "jbl.local"
	q.RemoteAddr = "192.168.50.2:1234"
	q = q.WithContext(context.WithValue(q.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("192.168.50.1"), Port: 80}))
	q.Header.Set("Origin", "http://attacker.invalid")
	q.Header.Set("Content-Type", "application/json")
	q.Header.Set("X-CSRF-Token", r.csrf)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != 403 {
		t.Fatal("alias bypassed same-origin guard")
	}
}
