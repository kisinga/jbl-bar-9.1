package main

import (
	"context"
	"encoding/json"
	"jbl-bar-toolkit/remote/internal/extension"
	"strings"
	"testing"
)

type fixtureBackend struct{ calls int }

func (b *fixtureBackend) snapshot() state { return state{Available: true, Power: "On"} }
func (b *fixtureBackend) action(context.Context, string) (state, error) {
	b.calls++
	return b.snapshot(), nil
}

func TestExtensionsShareHTTPGuardsAndSerialization(t *testing.T) {
	r := testRemote(t)
	backend := &fixtureBackend{}
	r.backend = backend
	calls := 0
	if err := r.extensions.Register(extension.Module[state]{
		Descriptor: extension.Descriptor{ID: "test.feature", Included: true},
		Actions: map[string]extension.Handler[state]{"test.feature.run": func(ctx context.Context, data json.RawMessage) (state, error) {
			calls++
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("action has no deadline")
			}
			return backend.snapshot(), nil
		}}, Project: func(s state) any { return s.Power },
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		ip     string
		csrf   bool
		origin string
	}{
		{"10.0.0.2", true, ""}, {"192.168.50.2", false, ""}, {"192.168.50.2", true, "http://attacker.invalid"},
	} {
		if w := request(r, "POST", "/api/action", `{"action":"test.feature.run"}`, c.ip, c.csrf, c.origin); w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	r.actionMu.Lock()
	w := request(r, "POST", "/api/action", `{"action":"test.feature.run"}`, "192.168.50.2", true, "")
	r.actionMu.Unlock()
	if w.Code != 409 || calls != 0 {
		t.Fatal("busy or rejected extension executed")
	}
	w = request(r, "POST", "/api/action", `{"action":"test.feature.run","parameters":{}}`, "192.168.50.2", true, "")
	if w.Code != 200 || calls != 1 || backend.calls != 0 || !strings.Contains(w.Body.String(), `"test.feature":"On"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(r, "GET", "/api/state", "", "192.168.50.2", false, "")
	if !strings.Contains(w.Body.String(), `"test.feature":"On"`) {
		t.Fatal("backend snapshot lacks feature state")
	}
	if w = request(r, "GET", "/api/features", "", "10.0.0.2", false, ""); w.Code != 403 {
		t.Fatal("feature discovery leaked")
	}
	if w = request(r, "GET", "/api/features", "", "192.168.50.2", false, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"test.feature.run"`) {
		t.Fatal("feature discovery missing")
	}
}

func TestExtensionParametersAreTyped(t *testing.T) {
	for _, data := range []string{`null`, `[]`, `{"command":"id"}`, `{} {}`, `{"gains_db":["wrong"]}`} {
		var target struct {
			GainsDB []float64 `json:"gains_db"`
		}
		if extensionParameters(json.RawMessage(data), &target) == nil {
			t.Fatal("accepted", data)
		}
	}
	var target struct {
		GainsDB []float64 `json:"gains_db"`
	}
	if extensionParameters(json.RawMessage(`{"gains_db":[1,2]}`), &target) != nil || len(target.GainsDB) != 2 {
		t.Fatal("valid typed parameters rejected")
	}
	r := testRemote(t)
	w := request(r, "POST", "/api/action", `{"action":"mute","parameters":{}}`, "192.168.50.2", true, "")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "included extension") {
		t.Fatal("core action accepted extension parameters")
	}
}
