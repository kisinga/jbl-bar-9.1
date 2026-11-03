// Package preview exercises the same server/UI without firmware or hardware.
// A normalized fixture can be replayed read-only while researching a new model.
package preview

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"jbl-bar-toolkit/remote/internal/extension"
	"jbl-bar-toolkit/remote/internal/server"
)

type State map[string]any

func (s State) StateRevision() uint64 { n, _ := s["revision"].(float64); return uint64(n) }
func (s State) PublicState(token string) any {
	copy := s.clone()
	copy["csrf"] = token
	return copy
}
func (s State) ChangeState() any {
	copy := s.clone()
	delete(copy, "csrf")
	delete(copy, "revision")
	delete(copy, "uptime")
	return copy
}
func (s State) clone() State {
	// Snapshot values stay detached from both the caller and the live backend.
	data, _ := json.Marshal(s)
	var copy State
	_ = json.Unmarshal(data, &copy)
	return copy
}

type Backend struct {
	mu       sync.Mutex
	state    State
	readonly bool
}

func New(fixture io.Reader) (*Backend, error) {
	b := &Backend{state: State{"available": true, "power": "On", "mode": "Unknown", "volume": float64(12), "muted": false, "input": "Preview", "revision": float64(1)}}
	if fixture != nil {
		data, err := io.ReadAll(io.LimitReader(fixture, 65537))
		if err != nil {
			return nil, err
		}
		if len(data) > 65536 {
			return nil, errors.New("fixture exceeds 64 KiB")
		}
		var snapshot State
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return nil, err
		}
		if snapshot == nil {
			return nil, errors.New("fixture must be a JSON state object")
		}
		b.state = snapshot
		// Revision is a local stream counter, not a vendor process identifier.
		b.state["revision"] = float64(1)
		delete(b.state, "csrf")
		b.readonly = true
	}
	return b, nil
}
func (b *Backend) Snapshot() State { b.mu.Lock(); defer b.mu.Unlock(); return b.state.clone() }
func (b *Backend) Features() server.Features {
	actions := []string{}
	if !b.readonly {
		actions = []string{"volume_up", "volume_down", "mute", "power"}
	}
	return server.Features{Schema: 1, Model: "preview", Name: "Toolkit preview", Actions: actions, Extensions: []extension.Descriptor{}}
}
func (b *Backend) Execute(ctx context.Context, command server.Command) (State, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return b.state.clone(), err
	}
	if b.readonly {
		return b.state.clone(), errors.New("fixture replay is read-only")
	}
	if len(command.Parameters) != 0 || command.GainsDB != nil || command.Value != nil {
		return b.state.clone(), errors.New("preview actions do not accept parameters")
	}
	switch command.Action {
	case "power":
		if b.state["power"] == "On" {
			b.state["power"] = "Standby"
		} else {
			b.state["power"] = "On"
		}
	case "mute":
		b.state["muted"] = b.state["muted"] != true
	case "volume_up", "volume_down":
		if b.state["power"] != "On" {
			return b.state.clone(), errors.New("turn the preview on first")
		}
		value, _ := b.state["volume"].(float64)
		if command.Action == "volume_up" && value < 32 {
			value++
		}
		if command.Action == "volume_down" && value > 0 {
			value--
		}
		b.state["volume"] = value
	default:
		return b.state.clone(), errors.New("action is unavailable in this preview")
	}
	b.state["revision"] = float64(b.state.StateRevision() + 1)
	return b.state.clone(), nil
}
