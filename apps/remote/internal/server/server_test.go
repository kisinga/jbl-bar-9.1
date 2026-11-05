package server_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"jbl-bar-toolkit/remote/internal/preview"
	"jbl-bar-toolkit/remote/internal/server"
)

func TestForeignBackendUsesSharedSecurityAndAssets(t *testing.T) {
	b, err := preview.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var gate sync.Mutex
	app, err := server.New[preview.State](b, server.Options[preview.State]{AllowedCIDR: "192.168.50.0/24", Gate: &gate, Assets: fstest.MapFS{
		"favicon.svg": {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)}, "index.html": {Data: []byte("preview")}, "app.js": {Data: []byte("app")}, "style.css": {Data: []byte("css")},
		"components/sound.js": {Data: []byte("component")}, "private.json": {Data: []byte("SECRET")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, ip, token, origin string) *httptest.ResponseRecorder {
		q := httptest.NewRequest(method, path, strings.NewReader(body))
		q.RemoteAddr = ip + ":1234"
		q.Host = "192.168.50.1:8080"
		q.Header.Set("Content-Type", "application/json")
		q.Header.Set("X-CSRF-Token", token)
		q.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, q)
		return w
	}
	for _, test := range []struct {
		method, path, body, ip, token, origin string
		status                                int
	}{
		{"GET", "/api/features", "", "10.0.0.1", "", "", 403},
		{"POST", "/api/action", `{"action":"mute"}`, "192.168.50.2", "", "", 403},
		{"POST", "/api/action", `{"action":"mute"}`, "192.168.50.2", app.Token(), "http://attacker.invalid", 403},
		{"POST", "/api/action", `{"action":"mute"} {}`, "192.168.50.2", app.Token(), "", 400},
		{"POST", "/api/action", `{"action":"mute","exec":"id"}`, "192.168.50.2", app.Token(), "", 400},
		{"POST", "/api/action", `{"action":"mute","value":1}`, "192.168.50.2", app.Token(), "", 409},
		{"GET", "/favicon.svg", "", "192.168.50.2", "", "", 200},
		{"GET", "/components/sound.js", "", "192.168.50.2", "", "", 200},
		{"GET", "/components/../private.json", "", "192.168.50.2", "", "", 404},
		{"GET", "/private.json", "", "192.168.50.2", "", "", 404},
	} {
		w := request(test.method, test.path, test.body, test.ip, test.token, test.origin)
		if w.Code != test.status {
			t.Fatalf("%s %s: %d %s", test.method, test.path, w.Code, w.Body.String())
		}
	}
	if w := request("GET", "/favicon.svg", "", "192.168.50.2", "", ""); w.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatal("favicon MIME", w.Header())
	}
	if b.Snapshot()["muted"] != false {
		t.Fatal("rejected command mutated the backend")
	}
	gate.Lock()
	w := request("POST", "/api/action", `{"action":"mute"}`, "192.168.50.2", app.Token(), "")
	gate.Unlock()
	if w.Code != 409 || b.Snapshot()["muted"] != false {
		t.Fatal("runtime gate was bypassed")
	}
	w = request("POST", "/api/action", `{"action":"mute"}`, "192.168.50.2", app.Token(), "")
	if w.Code != 200 || b.Snapshot()["muted"] != true {
		t.Fatal("valid backend command failed", w.Body.String())
	}
	var state map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &state)
	if state["csrf"] != app.Token() {
		t.Fatal("session token missing")
	}
	w = request("GET", "/api/features", "", "192.168.50.2", "", "")
	if !strings.Contains(w.Body.String(), `"model":"preview"`) {
		t.Fatal("model identity was hardcoded")
	}
}

type budgetBackend struct {
	*preview.Backend
	remaining time.Duration
	command   server.Command
}

func (b *budgetBackend) Execute(ctx context.Context, command server.Command) (preview.State, error) {
	deadline, _ := ctx.Deadline()
	b.remaining = time.Until(deadline)
	b.command = command
	return b.Snapshot(), nil
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestCompositionTimeoutAndLegacyValue(t *testing.T) {
	for _, selected := range []time.Duration{60 * time.Second, 18 * time.Second, 100 * time.Second, 0, -time.Second} {
		t.Run(selected.String(), func(t *testing.T) {
			previewBackend, _ := preview.New(nil)
			backend := &budgetBackend{Backend: previewBackend}
			app, err := server.New[preview.State](backend, server.Options[preview.State]{
				AllowedCIDR: "192.168.50.0/24", Assets: fstest.MapFS{"index.html": {Data: []byte("fixture")}},
				ActionTimeout: func(command server.Command) time.Duration {
					if command.Action != "fixture-long" {
						t.Error("policy received a different action")
					}
					return selected
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			q := httptest.NewRequest("POST", "http://192.168.50.1/api/action", strings.NewReader(`{"action":"fixture-long","value":40}`))
			q.RemoteAddr = "192.168.50.2:1234"
			q.Header.Set("Content-Type", "application/json")
			q.Header.Set("X-CSRF-Token", app.Token())
			w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
			app.ServeHTTP(w, q)
			expected := selected
			if expected <= 0 || expected > 60*time.Second {
				expected = 15 * time.Second
			}
			if w.Code != 200 || backend.remaining > expected || backend.remaining < expected-time.Second || backend.command.Value == nil || *backend.command.Value != 40 {
				t.Fatal("composition policy/value lost or exceeded", w.Code, backend.remaining, backend.command)
			}
			if expected > 15*time.Second && time.Until(w.deadline) < expected+9*time.Second {
				t.Fatal("long action reply deadline was not extended")
			}
			if expected == 15*time.Second && !w.deadline.IsZero() {
				t.Fatal("invalid/default policy altered the write deadline")
			}
		})
	}
}

func TestSecondaryNetworkComesFromTheComposition(t *testing.T) {
	ip := net.ParseIP
	_, lan, _ := net.ParseCIDR("192.168.50.0/24")
	secondary := &net.IPNet{IP: ip("10.42.0.1"), Mask: net.CIDRMask(28, 32)}
	if !server.AllowedNetwork(ip("10.42.0.2"), ip("10.42.0.1"), ip("10.42.0.1"), lan, secondary) {
		t.Fatal("server assumed a model-specific hotspot")
	}
	if server.AllowedNetwork(ip("10.42.0.2"), ip("10.42.0.1"), ip("192.168.50.1"), lan, secondary) {
		t.Fatal("secondary network accepted on another interface")
	}
	if server.AllowedNetwork(ip("10.42.0.20"), ip("10.42.0.1"), ip("10.42.0.1"), lan, secondary) {
		t.Fatal("secondary network mask ignored")
	}
}
