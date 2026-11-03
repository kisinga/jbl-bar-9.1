// Package server serves any explicitly composed backend. It has no firmware,
// device filesystem or model-specific control dependencies.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"jbl-bar-toolkit/remote/internal/extension"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// State preserves a backend's typed snapshot and v1 JSON shape. PublicState
// returns a copy with the session token; ChangeState omits volatile counters
// that should not cause an event. Neither method may perform device I/O.
type State interface {
	StateRevision() uint64
	PublicState(token string) any
	ChangeState() any
}

// Command keeps gains_db and value for the existing v1 request contracts.
// Model-specific interpretation belongs to the backend.
type Command struct {
	Action     string          `json:"action"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
	GainsDB    []float64       `json:"gains_db,omitempty"`
	Value      *int            `json:"value,omitempty"`
}
type Features struct {
	Schema     int                    `json:"schema"`
	Model      string                 `json:"model"`
	Name       string                 `json:"name"`
	Actions    []string               `json:"actions"`
	Extensions []extension.Descriptor `json:"extensions"`
}
type Backend[S State] interface {
	Snapshot() S
	Execute(context.Context, Command) (S, error)
	Features() Features
}
type Options[S State] struct {
	AllowedCIDR string
	Hostname    string
	Assets      fs.FS
	// Share this gate with background writers. HTTP owns it while Execute runs;
	// Execute must not acquire it again. Reads retain backend-owned locking.
	Gate *sync.Mutex
	Feed *Feed[S]
	// ActionTimeout is optional composition policy, bounded to at most 60s.
	// The default is 15s; the server never interprets model action names.
	ActionTimeout func(Command) time.Duration
	// Hotspot is optional, passive model-specific network discovery.
	Hotspot func() *net.IPNet
	Done    func() <-chan struct{}
}
type Server[S State] struct {
	backend       Backend[S]
	network       *net.IPNet
	hostname      string
	csrf          string
	assets        fs.FS
	publicAssets  map[string]bool
	actionMu      *sync.Mutex
	feed          *Feed[S]
	hotspot       func() *net.IPNet
	done          func() <-chan struct{}
	actionTimeout func(Command) time.Duration
}

func New[S State](backend Backend[S], options Options[S]) (*Server[S], error) {
	if backend == nil || options.Assets == nil {
		return nil, errors.New("backend and web assets are required")
	}
	_, network, err := net.ParseCIDR(options.AllowedCIDR)
	if err != nil {
		return nil, err
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return nil, err
	}
	r := &Server[S]{backend: backend, network: network, hostname: options.Hostname, csrf: hex.EncodeToString(token), assets: options.Assets,
		publicAssets: map[string]bool{"index.html": true, "app.js": true, "style.css": true}, actionMu: options.Gate, feed: options.Feed, hotspot: options.Hotspot, done: options.Done, actionTimeout: options.ActionTimeout}
	if r.actionMu == nil {
		r.actionMu = &sync.Mutex{}
	}
	if r.feed == nil {
		r.feed = &Feed[S]{}
	}
	// Only local, embedded component modules are served. No arbitrary files,
	// dynamic plugin downloads or filesystem paths are accepted from a client.
	component := regexp.MustCompile(`^components/[a-z][a-z0-9-]*\.js$`)
	err = fs.WalkDir(options.Assets, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && component.MatchString(path) {
			r.publicAssets[path] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}
func (r *Server[S]) Token() string           { return r.csrf }
func (r *Server[S]) Run(ctx context.Context) { r.feed.Run(ctx, r.backend.Snapshot) }
func (r *Server[S]) Publish(s S)             { r.feed.Publish(s) }

func (r *Server[S]) ServeHTTP(w http.ResponseWriter, q *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	client, _, e := net.SplitHostPort(q.RemoteAddr)
	if e != nil {
		http.Error(w, "Local network only", 403)
		return
	}
	target := q.Host
	if h, _, e := net.SplitHostPort(target); e == nil {
		target = h
	}
	var destination net.IP
	if address, ok := q.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if h, _, e := net.SplitHostPort(address.String()); e == nil {
			destination = net.ParseIP(h)
		}
	}
	var ap *net.IPNet
	if !r.network.Contains(net.ParseIP(client)) {
		if r.hotspot != nil {
			ap = r.hotspot()
		}
	}
	hostIP := net.ParseIP(target)
	if strings.EqualFold(strings.TrimSuffix(target, "."), r.hostname) {
		hostIP = destination
	}
	if !AllowedNetwork(net.ParseIP(client), hostIP, destination, r.network, ap) {
		http.Error(w, "Open the remote's local address from its trusted network", 403)
		return
	}
	if q.URL.Path == "/api/events" && q.Method == "GET" {
		r.stream(w, q)
		return
	}
	if q.URL.Path == "/api/state" && q.Method == "GET" {
		s := r.backend.Snapshot()
		writeJSON(w, 200, s.PublicState(r.csrf))
		return
	}
	if q.URL.Path == "/api/features" && q.Method == "GET" {
		writeJSON(w, 200, r.backend.Features())
		return
	}
	if q.URL.Path == "/api/action" && q.Method == "POST" {
		origin := q.Header.Get("Origin")
		if origin != "" && origin != "http://"+q.Host {
			http.Error(w, "Origin rejected", 403)
			return
		}
		if q.Header.Get("Content-Type") != "application/json" || subtle.ConstantTimeCompare([]byte(q.Header.Get("X-CSRF-Token")), []byte(r.csrf)) != 1 {
			http.Error(w, "Refresh the page before using controls", 403)
			return
		}
		var request Command
		dec := json.NewDecoder(http.MaxBytesReader(w, q.Body, 256))
		dec.DisallowUnknownFields()
		if e := dec.Decode(&request); e != nil {
			http.Error(w, "Invalid control request", 400)
			return
		}
		if dec.Decode(new(any)) != io.EOF {
			http.Error(w, "Invalid control request", 400)
			return
		}
		if !r.actionMu.TryLock() {
			writeJSON(w, 409, map[string]string{"error": "Another control is running"})
			return
		}
		defer r.actionMu.Unlock()
		// Finish key-release/restore even if a phone closes the page mid-command.
		timeout := 15 * time.Second
		if r.actionTimeout != nil {
			if selected := r.actionTimeout(request); selected > 0 && selected <= 60*time.Second {
				timeout = selected
			}
		}
		if timeout > 15*time.Second {
			// Allow bounded native restoration after a long action expires.
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(timeout + 10*time.Second))
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		s, e := r.backend.Execute(ctx, request)
		if e != nil {
			writeJSON(w, 409, map[string]any{"error": e.Error(), "state": s.PublicState(r.csrf)})
			return
		}
		r.feed.Publish(s)
		writeJSON(w, 200, s.PublicState(r.csrf))
		return
	}
	if q.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	name := strings.TrimPrefix(q.URL.Path, "/")
	if q.URL.Path == "/" {
		name = "index.html"
	}
	if !r.publicAssets[name] {
		http.NotFound(w, q)
		return
	}
	path := name
	b, e := fs.ReadFile(r.assets, path)
	if e != nil {
		http.Error(w, "Page unavailable", 500)
		return
	}
	if strings.HasSuffix(path, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else if strings.HasSuffix(path, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	w.Write(b)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
