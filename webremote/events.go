package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// One device reader serves every visible browser; slow clients retain only the latest state.
type stateFeed struct {
	mu           sync.Mutex
	clients      map[chan state]bool
	lastRevision uint64
}

func (f *stateFeed) subscribe() (chan state, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.clients == nil {
		f.clients = map[chan state]bool{}
	}
	c := make(chan state, 1)
	f.clients[c] = true
	return c, func() { f.mu.Lock(); delete(f.clients, c); f.mu.Unlock() }
}
func (f *stateFeed) publish(s state) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s.Revision < f.lastRevision {
		return
	}
	f.lastRevision = s.Revision
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
func (f *stateFeed) run(ctx context.Context, snapshot func() state) {
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
			comparison := s
			comparison.Uptime = 0
			comparison.Revision = 0
			comparison.CSRF = ""
			b, _ := json.Marshal(comparison)
			if string(b) != string(previous) {
				f.publish(s)
				previous = b
			}
		}
	}
}
func (r *remote) stream(w http.ResponseWriter, q *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	c, unsubscribe := r.feed.subscribe()
	defer unsubscribe()
	send := func(s state) bool {
		s.CSRF = r.csrf
		b, e := json.Marshal(s)
		if e != nil {
			return false
		}
		http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, e = fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
		return e == nil
	}
	if !send(r.device.snapshot()) {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-q.Context().Done():
			return
		case <-r.stop:
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
