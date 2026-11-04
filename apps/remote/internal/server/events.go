package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// One device reader serves every visible browser; slow clients retain only the latest state.
type Feed[S State] struct {
	mu           sync.Mutex
	clients      map[chan S]bool
	lastRevision uint64
	latest       S
	hasLatest    bool
	held         bool
	control      Control
}

func (f *Feed[S]) Subscribe() (chan S, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.clients == nil {
		f.clients = map[chan S]bool{}
	}
	c := make(chan S, 1)
	f.clients[c] = true
	return c, func() { f.mu.Lock(); delete(f.clients, c); f.mu.Unlock() }
}
func (f *Feed[S]) Publish(s S) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s.StateRevision() < f.lastRevision {
		return
	}
	f.latest, f.hasLatest = s, true
	f.broadcastLocked(s)
}

// broadcastLocked sends s, with the current control, to every client.
func (f *Feed[S]) broadcastLocked(s S) {
	if s.StateRevision() < f.lastRevision {
		return
	}
	f.lastRevision = s.StateRevision()
	s = f.withControlLocked(s)
	for c := range f.clients {
		select {
		case c <- s:
		default:
			select {
			case <-c:
			default:
			}
			select {
			case c <- s:
			default:
			}
		}
	}
}
func (f *Feed[S]) Run(ctx context.Context, snapshot func() S) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var previous []byte
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.mu.Lock()
			watching := len(f.clients) > 0
			f.mu.Unlock()
			if !watching {
				previous = nil
				continue
			}
			s := snapshot()
			b, _ := json.Marshal(s.ChangeState())
			if string(b) != string(previous) {
				f.Publish(s)
				previous = b
			}
		}
	}
}
func (r *Server[S]) stream(w http.ResponseWriter, q *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	c, unsubscribe := r.feed.Subscribe()
	defer unsubscribe()
	send := func(s S) bool {
		b, e := json.Marshal(s.PublicState(r.csrf))
		if e != nil {
			return false
		}
		http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, e = fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
		return e == nil
	}
	if !send(r.snapshot()) {
		return
	}
	var done <-chan struct{}
	if r.done != nil {
		done = r.done()
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-q.Context().Done():
			return
		case <-done:
			return
		case s := <-c:
			if !send(s) {
				return
			}
		case <-heartbeat.C:
			http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, e := fmt.Fprint(w, ": keepalive\n\n"); e != nil {
				return
			}
			flusher.Flush()
		}
	}
}
