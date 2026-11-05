package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBar91BackgroundWritersReportThroughTheSharedServer(t *testing.T) {
	r := testRemote(t)
	if w := request(r, "GET", "/api/state", "", "192.168.50.2", false, ""); !strings.Contains(w.Body.String(), `"control":{"busy":false}`) {
		t.Fatal("idle state does not report the control", w.Body.String())
	}
	release, report, ok := r.hold("")
	if !ok {
		t.Fatal("gate unavailable")
	}
	// The writer holds the device too; reads use the cached state.
	r.device.mu.Lock()
	done := make(chan string, 1)
	go func() { done <- request(r, "GET", "/api/state", "", "192.168.50.2", false, "").Body.String() }()
	select {
	case body := <-done:
		if !strings.Contains(body, `"control":{"busy":false}`) {
			t.Fatal("a short monitor was reported as a control", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("state read waited for a background writer")
	}
	if w := request(r, "POST", "/api/action", `{"action":"mute"}`, "192.168.50.2", true, ""); w.Code != 409 || !strings.Contains(w.Body.String(), "Another control") {
		t.Fatal("control ran during a background writer", w.Code)
	}
	report("auto_standard")
	if w := request(r, "GET", "/api/state", "", "192.168.50.2", false, ""); !strings.Contains(w.Body.String(), `"action":"auto_standard"`) {
		t.Fatal("automatic Standard Mode not reported", w.Body.String())
	}
	r.device.mu.Unlock()
	release()
	if w := request(r, "GET", "/api/state", "", "192.168.50.2", false, ""); !strings.Contains(w.Body.String(), `"control":{"busy":false}`) {
		t.Fatal("control still reported after release", w.Body.String())
	}
}

// Every native background writer takes the gate through hold, so state reads
// keep using the cached state while it runs (the bass offset monitor, added
// later, is covered the same way).
func TestBar91WritersTakeTheGateThroughHold(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "backend_bar91.go" {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "actionMu.TryLock()") {
			t.Fatalf("%s takes the action gate directly; use r.hold", name)
		}
	}
}
