package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func testRemote(t *testing.T) *remote {
	t.Helper()
	r, e := newRemote(config{Listen: ":8080", AllowedCIDR: "192.168.50.0/24"})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func request(r *remote, method, path, body, ip string, csrf bool, origin string) *httptest.ResponseRecorder {
	q := httptest.NewRequest(method, path, strings.NewReader(body))
	q.RemoteAddr = ip + ":12345"
	q.Host = "192.168.50.1:8080"
	if csrf {
		q.Header.Set("X-CSRF-Token", r.csrf)
	}
	if method == "POST" {
		q.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		q.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	return w
}
func TestAccessBoundaries(t *testing.T) {
	r := testRemote(t)
	cases := []struct {
		name, method, path, body, ip, origin string
		csrf                                 bool
		want                                 int
	}{
		{"no_login", "GET", "/", "", "192.168.50.2", "", false, 200},
		{"network", "GET", "/", "", "10.0.0.2", "", false, 403},
		{"page", "GET", "/", "", "192.168.50.2", "", false, 200},
		{"unknown_file", "GET", "/config.json", "", "192.168.50.2", "", false, 404},
		{"path_traversal", "GET", "/../config.json", "", "192.168.50.2", "", false, 404},
		{"csrf", "POST", "/api/action", `{"action":"mute"}`, "192.168.50.2", "", false, 403},
		{"origin", "POST", "/api/action", `{"action":"mute"}`, "192.168.50.2", "http://elsewhere.invalid", true, 403},
		{"unknown_action", "POST", "/api/action", `{"action":"exec touch /tmp/unwanted"}`, "192.168.50.2", "", true, 409},
		{"unknown_field", "POST", "/api/action", `{"action":"mute","command":"echo"}`, "192.168.50.2", "", true, 400},
		{"no_mutating_get", "GET", "/api/action?action=mute", "", "192.168.50.2", "", false, 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := request(r, c.method, c.path, c.body, c.ip, c.csrf, c.origin)
			if w.Code != c.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
func TestNetworkBoundaryProtectsState(t *testing.T) {
	r := testRemote(t)
	w := request(r, "GET", "/api/state", "", "10.0.0.2", false, "")
	if w.Code != 403 || strings.Contains(w.Body.String(), r.csrf) {
		t.Fatal("state leaked outside configured LAN")
	}
	w = request(r, "GET", "/api/state", "", "192.168.50.2", false, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), r.csrf) || w.Header().Get("WWW-Authenticate") != "" {
		t.Fatal("LAN access unexpectedly requires login")
	}
}
func TestAssetsAndUnavailableDevice(t *testing.T) {
	r := testRemote(t)
	for _, p := range []string{"/", "/app.js", "/style.css", "/api/state"} {
		w := request(r, "GET", p, "", "192.168.50.2", false, "")
		if w.Code != 200 {
			t.Fatal(p, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing cache protection")
		}
	}
}
func TestPowerNeedsStableOppositeState(t *testing.T) {
	before := state{Available: true, Power: "On"}
	for _, after := range []state{{Power: "Standby"}, {Available: true, Power: "Starting"}, {Available: true, Power: "On"}} {
		if actionObserved("power", before, after) {
			t.Fatal("unconfirmed power transition accepted", after)
		}
	}
	if !actionObserved("power", before, state{Available: true, Power: "Standby"}) {
		t.Fatal("stable power transition rejected")
	}
}

func TestBluetoothReadback(t *testing.T) {
	for value, want := range []string{"Disconnected", "Connected", "Paused", "Streaming"} {
		connected, audio, ok := bluetoothAudio(uint32(value))
		if !ok || audio != want || connected != (value != 0) {
			t.Fatalf("invalid audio state %d", value)
		}
	}
	if _, _, ok := bluetoothAudio(4); ok {
		t.Fatal("unknown Bluetooth state accepted")
	}
	source := uint32(10)
	before := state{Available: true, Source: &source}
	for _, after := range []state{{Available: true, Source: &source}, {Available: true, Source: &source, Bluetooth: &bluetoothState{}}, {Available: true, Source: &source, Bluetooth: &bluetoothState{Selected: true, Discoverable: true}}, {Source: &source, Bluetooth: &bluetoothState{Selected: true, Connectable: true, Discoverable: true}}} {
		if actionObserved("bluetooth_pair", before, after) {
			t.Fatal("pairing inferred without live discoverability")
		}
	}
	if !actionObserved("bluetooth_pair", before, state{Available: true, Source: &source, Bluetooth: &bluetoothState{Selected: true, Connectable: true, Discoverable: true}}) {
		t.Fatal("discoverability rejected")
	}
}

func TestBluetoothName(t *testing.T) {
	for _, b := range [][]byte{[]byte("unterminated"), {0}, {0xff, 0}, []byte("bad\nname\x00")} {
		if bluetoothName(b) != "" {
			t.Fatal("invalid name accepted")
		}
	}
	if bluetoothName([]byte("My speaker\x00ignored")) != "My speaker" {
		t.Fatal("name not bounded at terminator")
	}
	if bluetoothName([]byte("音楽\x00")) != "音楽" {
		t.Fatal("UTF-8 name rejected")
	}
}

func TestRejectRebindingHost(t *testing.T) {
	r := testRemote(t)
	q := httptest.NewRequest("GET", "http://attacker.invalid/api/state", nil)
	q.RemoteAddr = "192.168.50.2:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != 403 || strings.Contains(w.Body.String(), r.csrf) {
		t.Fatal("DNS host accepted for login-free remote")
	}
}

func TestBluetoothSelectionUsesSourceState(t *testing.T) {
	raw := uint32(9)
	if actionObserved("input_bluetooth", state{}, state{Available: true, Source: &raw}) {
		t.Fatal("raw setting51 treated as Bluetooth source")
	}
	raw = 10
	if !actionObserved("input_bluetooth", state{}, state{Available: true, Source: &raw, Bluetooth: &bluetoothState{Selected: true}}) {
		t.Fatal("validated source-state ignored")
	}
}
