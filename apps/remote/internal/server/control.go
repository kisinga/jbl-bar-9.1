package server

import (
	"context"
	"time"
)

// Control reports the control that holds the action gate. Clients use it to
// wait for a running control instead of sending one that would be refused.
type Control struct {
	Busy   bool   `json:"busy"`
	Action string `json:"action,omitempty"`
	// Since is the Unix time the control started.
	Since int64 `json:"since,omitempty"`
}

// ControlState is implemented by states that carry Control. Other states are
// served unchanged; they still get the cached snapshot during a control.
type ControlState[S any] interface {
	WithControl(Control) S
}

// The feed tracks who holds the gate. While it is held, state reads and new
// event streams get the latest snapshot instead of waiting for the device,
// which the holder is using.
func (f *Feed[S]) begin(action string, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.held = true
	if action != "" {
		f.reportLocked(action, now)
	}
}
func (f *Feed[S]) report(action string, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.held && action != "" && f.control.Action != action {
		f.reportLocked(action, now)
	}
}
func (f *Feed[S]) reportLocked(action string, now time.Time) {
	f.control = Control{Busy: true, Action: action, Since: now.Unix()}
	if f.hasLatest {
		f.broadcastLocked(f.latest)
	}
}

// end releases the tracking. With announce, clients are told the control has
// finished using the latest state; a caller that publishes a fresh state right
// after passes false.
func (f *Feed[S]) end(announce bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reported := f.control.Busy
	f.held, f.control = false, Control{}
	if announce && reported && f.hasLatest {
		f.broadcastLocked(f.latest)
	}
}

// cached returns the latest state while the gate is held.
func (f *Feed[S]) cached() (S, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.held || !f.hasLatest {
		var zero S
		return zero, false
	}
	return f.withControlLocked(f.latest), true
}

// remember keeps a snapshot read outside a control as the latest state.
func (f *Feed[S]) remember(s S) S {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.hasLatest || s.StateRevision() >= f.latest.StateRevision() {
		f.latest, f.hasLatest = s, true
	}
	return f.withControlLocked(s)
}

// Control returns the running control, if any.
func (f *Feed[S]) Control() Control {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.control
}

func (f *Feed[S]) withControlLocked(s S) S {
	if c, ok := any(s).(ControlState[S]); ok {
		return c.WithControl(f.control)
	}
	return s
}

// Hold is the action gate taken by a background writer of the composition.
type Hold[S State] struct {
	server *Server[S]
	done   bool
}

// TryHold takes the action gate without waiting, like an HTTP action. An empty
// action keeps the writer out of the reported control (short monitors); the
// state stays readable from the cache either way.
func (r *Server[S]) TryHold(action string) (*Hold[S], bool) {
	if !r.actionMu.TryLock() {
		return nil, false
	}
	r.feed.begin(action, time.Now())
	return &Hold[S]{server: r}, true
}

// Report names the control once a background writer starts a visible one.
func (h *Hold[S]) Report(action string) { h.server.feed.report(action, time.Now()) }

// Release ends the control and releases the gate. It is idempotent.
func (h *Hold[S]) Release() {
	if h.done {
		return
	}
	h.done = true
	h.server.feed.end(true)
	h.server.actionMu.Unlock()
}

// snapshot reads the state without waiting for a running control.
func (r *Server[S]) snapshot() S {
	if s, ok := r.feed.cached(); ok {
		return s
	}
	return r.feed.remember(r.backend.Snapshot())
}

// Snapshot is the state as served by /api/state.
func (r *Server[S]) Snapshot() S { return r.snapshot() }

// backgroundPoll is how often a control waiting for a background hold retries.
const backgroundPoll = 20 * time.Millisecond

// takeGate takes the action gate for an HTTP control. Monitors hold it for
// a few milliseconds at a time without reporting a control; a control that
// arrives meanwhile waits for them, up to backgroundWait, instead of being
// refused with "Another control is running". A reported control (an HTTP
// action or a named background gesture) is refused at once, as before.
func (r *Server[S]) takeGate(ctx context.Context) bool {
	if r.actionMu.TryLock() {
		return true
	}
	deadline := time.Now().Add(r.backgroundWait)
	// Not reported yet also covers the instant between a holder's TryLock and
	// its begin, and the shutdown cleanup: both end within the wait.
	for !r.feed.Control().Busy && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(backgroundPoll):
		}
		if r.actionMu.TryLock() {
			return true
		}
	}
	return false
}
