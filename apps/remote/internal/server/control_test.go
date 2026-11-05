package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"jbl-bar-toolkit/remote/internal/server"
)

// busyState carries the reported control like a model state does.
type busyState struct {
	Revision uint64          `json:"revision"`
	Volume   int             `json:"volume"`
	Control  *server.Control `json:"control,omitempty"`
	CSRF     string          `json:"csrf,omitempty"`
}

func (s busyState) StateRevision() uint64                  { return s.Revision }
func (s busyState) PublicState(token string) any           { s.CSRF = token; return s }
func (s busyState) ChangeState() any                       { s.Revision = 0; s.CSRF = ""; return s }
func (s busyState) WithControl(c server.Control) busyState { s.Control = &c; return s }

// deviceBackend holds its device lock for a whole control, like a native
// backend: a snapshot read during a control would wait for it.
type deviceBackend struct {
	device   sync.Mutex
	revision uint64
	volume   int
	started  chan struct{}
	release  chan struct{}
	fail     bool
}

func (b *deviceBackend) Snapshot() busyState {
	b.device.Lock()
	defer b.device.Unlock()
	b.revision++
	return busyState{Revision: b.revision, Volume: b.volume}
}
func (b *deviceBackend) Execute(_ context.Context, c server.Command) (busyState, error) {
	b.device.Lock()
	defer b.device.Unlock()
	b.started <- struct{}{}
	<-b.release
	b.revision++
	if b.fail {
		return busyState{}, errors.New("native control failed")
	}
	b.volume++
	return busyState{Revision: b.revision, Volume: b.volume}, nil
}
func (b *deviceBackend) Features() server.Features { return server.Features{Schema: 1} }

type busyFixture struct {
	t       *testing.T
	backend *deviceBackend
	app     *server.Server[busyState]
	gate    *sync.Mutex
	http    *httptest.Server
}

func newBusyFixture(t *testing.T) *busyFixture {
	return newBusyFixtureWait(t, 300*time.Millisecond)
}

func newBusyFixtureWait(t *testing.T, wait time.Duration) *busyFixture {
	t.Helper()
	b := &deviceBackend{started: make(chan struct{}, 1), release: make(chan struct{})}
	gate := &sync.Mutex{}
	app, err := server.New[busyState](b, server.Options[busyState]{AllowedCIDR: "127.0.0.0/8", Gate: gate, Feed: &server.Feed[busyState]{}, BackgroundWait: wait, Assets: fstest.MapFS{"index.html": {Data: []byte("fixture")}}})
	if err != nil {
		t.Fatal(err)
	}
	f := &busyFixture{t: t, backend: b, app: app, gate: gate, http: httptest.NewServer(app)}
	t.Cleanup(f.http.Close)
	return f
}

// within fails the test when a read does not answer promptly.
func within[T any](t *testing.T, what string, read func() T) T {
	t.Helper()
	done := make(chan T, 1)
	go func() { done <- read() }()
	select {
	case v := <-done:
		return v
	case <-time.After(2 * time.Second):
		t.Fatalf("%s waited for the running control", what)
	}
	panic("unreachable")
}

func (f *busyFixture) state() busyState {
	q, _ := http.NewRequest("GET", f.http.URL+"/api/state", nil)
	res, err := http.DefaultClient.Do(q)
	if err != nil {
		f.t.Error(err)
		return busyState{}
	}
	defer res.Body.Close()
	var s busyState
	json.NewDecoder(res.Body).Decode(&s)
	return s
}

type reply struct {
	code int
	body map[string]json.RawMessage
}

func (f *busyFixture) action() reply {
	q, _ := http.NewRequest("POST", f.http.URL+"/api/action", strings.NewReader(`{"action":"fixture-slow"}`))
	q.Header.Set("Content-Type", "application/json")
	q.Header.Set("X-CSRF-Token", f.app.Token())
	res, err := http.DefaultClient.Do(q)
	if err != nil {
		f.t.Error(err)
		return reply{}
	}
	defer res.Body.Close()
	r := reply{code: res.StatusCode, body: map[string]json.RawMessage{}}
	json.NewDecoder(res.Body).Decode(&r.body)
	return r
}

// stream opens /api/events and returns its states as they arrive.
func (f *busyFixture) stream() <-chan busyState {
	ctx, cancel := context.WithCancel(context.Background())
	f.t.Cleanup(cancel)
	q, _ := http.NewRequestWithContext(ctx, "GET", f.http.URL+"/api/events", nil)
	states := make(chan busyState, 16)
	go func() {
		res, err := http.DefaultClient.Do(q)
		if err != nil {
			return
		}
		defer res.Body.Close()
		lines := bufio.NewScanner(res.Body)
		for lines.Scan() {
			if data, ok := strings.CutPrefix(lines.Text(), "data: "); ok {
				var s busyState
				json.Unmarshal([]byte(data), &s)
				states <- s
			}
		}
	}()
	return states
}

func next(t *testing.T, states <-chan busyState, want func(busyState) bool, what string) busyState {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case s := <-states:
			if want(s) {
				return s
			}
		case <-deadline:
			t.Fatalf("stream never reported %s", what)
		}
	}
}

func busy(action string) func(busyState) bool {
	return func(s busyState) bool { return s.Control != nil && s.Control.Busy && s.Control.Action == action }
}
func idle(s busyState) bool { return s.Control != nil && !s.Control.Busy }

func TestStateAndNewStreamsStayResponsiveDuringAnAction(t *testing.T) {
	f := newBusyFixture(t)
	if s := f.state(); !idle(s) || s.Volume != 0 {
		t.Fatalf("idle state %+v", s)
	}
	early := f.stream()
	next(t, early, idle, "the idle state")
	result := make(chan reply, 1)
	go func() { result <- f.action() }()
	<-f.backend.started

	s := within(t, "/api/state", f.state)
	if !busy("fixture-slow")(s) || s.Control.Since == 0 || s.Volume != 0 {
		t.Fatalf("state during the control %+v", s)
	}
	next(t, early, busy("fixture-slow"), "the control start")
	late := f.stream()
	within(t, "a new event stream", func() busyState { return next(t, late, busy("fixture-slow"), "the running control to a new stream") })
	refused := within(t, "a second control", f.action)
	if refused.code != 409 || !strings.Contains(string(refused.body["state"]), `"action":"fixture-slow"`) {
		t.Fatalf("a refused control must name the running one: %d %s", refused.code, refused.body["state"])
	}

	close(f.backend.release)
	if r := <-result; r.code != 200 {
		t.Fatalf("control: %d", r.code)
	}
	for _, states := range []<-chan busyState{early, late} {
		if s := next(t, states, idle, "the end of the control"); s.Volume != 1 {
			t.Fatalf("end of control without its result: %+v", s)
		}
	}
	if s := f.state(); !idle(s) || s.Volume != 1 {
		t.Fatalf("state after the control %+v", s)
	}
}

func TestAFailedControlAnnouncesItsEnd(t *testing.T) {
	f := newBusyFixture(t)
	f.state()
	f.backend.fail = true
	states := f.stream()
	next(t, states, idle, "the idle state")
	result := make(chan map[string]json.RawMessage, 1)
	go func() { result <- f.action().body }()
	<-f.backend.started
	next(t, states, busy("fixture-slow"), "the control start")
	close(f.backend.release)
	body := <-result
	if !strings.Contains(string(body["error"]), "native control failed") || !strings.Contains(string(body["state"]), `"busy":false`) {
		t.Fatalf("failure reply %v", body)
	}
	next(t, states, idle, "the end of the failed control")
	if !f.gate.TryLock() {
		t.Fatal("a failed control kept the gate")
	}
	f.gate.Unlock()
	if s := f.state(); !idle(s) {
		t.Fatalf("state after failure %+v", s)
	}
}

func TestBackgroundWritersKeepReadsResponsive(t *testing.T) {
	f := newBusyFixture(t)
	f.state()
	states := f.stream()
	next(t, states, idle, "the idle state")

	// A short monitor holds the gate and the device without being reported.
	hold, ok := f.app.TryHold("")
	if !ok {
		t.Fatal("gate unavailable")
	}
	f.backend.device.Lock()
	if s := within(t, "/api/state during a monitor", f.state); !idle(s) {
		t.Fatalf("a monitor was reported as a control: %+v", s)
	}
	if _, again := f.app.TryHold(""); again {
		t.Fatal("two writers held the gate")
	}
	// A control waits for the monitor (up to the background wait), then is
	// refused if the monitor is still there.
	start := time.Now()
	if r := within(t, "a control during a long monitor", f.action); r.code != 409 {
		t.Fatalf("control during a monitor: %d", r.code)
	}
	if waited := time.Since(start); waited < 200*time.Millisecond {
		t.Fatalf("a control was refused after %v while only a monitor held the gate", waited)
	}
	// A native gesture reports itself once it starts.
	hold.Report("auto_standard")
	next(t, states, busy("auto_standard"), "the background control")
	if s := within(t, "/api/state during a background control", f.state); !busy("auto_standard")(s) {
		t.Fatalf("background control not reported: %+v", s)
	}
	f.backend.device.Unlock()
	hold.Release()
	hold.Release()
	next(t, states, idle, "the end of the background control")
	if !f.gate.TryLock() {
		t.Fatal("released hold kept the gate")
	}
	f.gate.Unlock()
}

// Monitors hold the gate for milliseconds without reporting a control. A
// control sent meanwhile used to be refused with "Another control is
// running" although nothing visible was running; it now waits for them.
func TestAControlWaitsForAShortMonitor(t *testing.T) {
	f := newBusyFixture(t)
	f.state()
	hold, ok := f.app.TryHold("")
	if !ok {
		t.Fatal("gate unavailable")
	}
	time.AfterFunc(60*time.Millisecond, hold.Release)
	result := make(chan reply, 1)
	go func() { result <- f.action() }()
	<-f.backend.started
	close(f.backend.release)
	if r := <-result; r.code != 200 {
		t.Fatalf("control after a short monitor: %d %s", r.code, r.body["error"])
	}
}

// A reported background control (a native gesture) is refused at once, so
// clients wait for it through the reported state instead of the request.
func TestAControlIsRefusedAtOnceDuringAReportedGesture(t *testing.T) {
	f := newBusyFixtureWait(t, 5*time.Second)
	f.state()
	hold, ok := f.app.TryHold("auto_standard")
	if !ok {
		t.Fatal("gate unavailable")
	}
	defer hold.Release()
	start := time.Now()
	r := within(t, "a control during a gesture", f.action)
	if r.code != 409 || !strings.Contains(string(r.body["state"]), `"action":"auto_standard"`) {
		t.Fatalf("control during a gesture: %d %s", r.code, r.body["state"])
	}
	if waited := time.Since(start); waited > time.Second {
		t.Fatalf("refusal took %v", waited)
	}
}

// A negative wait keeps the old immediate refusal.
func TestBackgroundWaitCanBeDisabled(t *testing.T) {
	f := newBusyFixtureWait(t, -1)
	f.state()
	hold, ok := f.app.TryHold("")
	if !ok {
		t.Fatal("gate unavailable")
	}
	defer hold.Release()
	start := time.Now()
	if r := f.action(); r.code != 409 {
		t.Fatalf("control: %d", r.code)
	}
	if waited := time.Since(start); waited > 100*time.Millisecond {
		t.Fatalf("refusal took %v", waited)
	}
}
