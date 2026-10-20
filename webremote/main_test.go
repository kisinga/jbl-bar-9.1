package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
)

func testRemote(t *testing.T) *remote {
	t.Helper()
	h := sha256.Sum256([]byte("fixture-password"))
	r, e := newRemote(config{Listen: ":8080", AllowedCIDR: "192.168.50.0/24", PasswordHash: hex.EncodeToString(h[:])})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func request(r *remote, method, path, body, ip string, auth, csrf bool, origin string) *httptest.ResponseRecorder {
	q := httptest.NewRequest(method, path, strings.NewReader(body))
	q.RemoteAddr = ip + ":12345"
	q.Host = "soundbar:8080"
	if auth {
		q.SetBasicAuth("root", "fixture-password")
	}
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
		auth, csrf                           bool
		want                                 int
	}{
		{"authentication", "GET", "/", "", "192.168.50.2", "", false, false, 401},
		{"network", "GET", "/", "", "10.0.0.2", "", true, false, 403},
		{"page", "GET", "/", "", "192.168.50.2", "", true, false, 200},
		{"unknown_file", "GET", "/config.json", "", "192.168.50.2", "", true, false, 404},
		{"path_traversal", "GET", "/../config.json", "", "192.168.50.2", "", true, false, 404},
		{"csrf", "POST", "/api/action", `{"action":"mute"}`, "192.168.50.2", "", true, false, 403},
		{"origin", "POST", "/api/action", `{"action":"mute"}`, "192.168.50.2", "http://elsewhere.invalid", true, true, 403},
		{"unknown_action", "POST", "/api/action", `{"action":"exec touch /tmp/unwanted"}`, "192.168.50.2", "", true, true, 409},
		{"unknown_field", "POST", "/api/action", `{"action":"mute","command":"echo"}`, "192.168.50.2", "", true, true, 400},
		{"no_mutating_get", "GET", "/api/action?action=mute", "", "192.168.50.2", "", true, false, 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := request(r, c.method, c.path, c.body, c.ip, c.auth, c.csrf, c.origin)
			if w.Code != c.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
func TestNoUnauthenticatedState(t *testing.T) {
	r := testRemote(t)
	w := request(r, "GET", "/api/state", "", "192.168.50.2", false, false, "")
	if w.Code != 401 || strings.Contains(w.Body.String(), r.csrf) {
		t.Fatal("state leaked")
	}
}
func TestAssetsAndUnavailableDevice(t *testing.T) {
	r := testRemote(t)
	for _, p := range []string{"/", "/app.js", "/style.css", "/api/state"} {
		w := request(r, "GET", p, "", "192.168.50.2", true, false, "")
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
