// Package extension composes optional features into one static executable.
// HTTP access control, serialization and deadlines remain with the host.
package extension

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
)

type Descriptor struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
	// Component selects a bundled UI component with compatible semantics.
	// It is a local identifier, never a script URL or executable path.
	Component    string   `json:"component,omitempty"`
	Included     bool     `json:"included"`
	Experimental bool     `json:"experimental"`
	Reason       string   `json:"reason,omitempty"`
	Actions      []string `json:"actions"`
}

type Handler[S any] func(context.Context, json.RawMessage) (S, error)

type Module[S any] struct {
	Descriptor Descriptor
	Actions    map[string]Handler[S]
	// Project only reads an already captured snapshot; it must not perform I/O.
	Project func(S) any
	// Run is an optional long-lived worker. Native writers must share the model's
	// operation gate with HTTP actions. Return after ctx cancellation, completing
	// any bounded native restoration first. Registration finishes before Run.
	Run func(context.Context)
}

// Registry is assembled before serving requests and then read concurrently.
// It deliberately has no loader, global registration, dependency container or
// separate HTTP routes that could bypass the host's request protections.
type Registry[S any] struct {
	modules []Module[S]
	actions map[string]Handler[S]
	claimed map[string]bool
}

func (r *Registry[S]) Register(m Module[S]) error {
	id := m.Descriptor.ID
	if !strings.Contains(id, ".") || strings.ContainsAny(id, " /\\\t\r\n") || strings.HasSuffix(id, ".") || strings.HasPrefix(id, ".") || strings.Contains(id, "..") {
		return errors.New("extension ID must have a namespace")
	}
	for _, existing := range r.modules {
		if existing.Descriptor.ID == id {
			return errors.New("duplicate extension ID")
		}
	}
	for action, handler := range m.Actions {
		if !strings.HasPrefix(action, id+".") || len(action) == len(id)+1 || handler == nil || r.claimed[action] {
			return errors.New("invalid or duplicate extension action")
		}
	}
	// Copy caller-owned metadata/maps so later edits cannot change live routing.
	m.Descriptor.Actions = make([]string, 0, len(m.Actions))
	copied := make(map[string]Handler[S], len(m.Actions))
	if r.actions == nil {
		r.actions = make(map[string]Handler[S])
		r.claimed = make(map[string]bool)
	}
	for action, handler := range m.Actions {
		r.claimed[action] = true
		m.Descriptor.Actions = append(m.Descriptor.Actions, action)
		copied[action] = handler
		if m.Descriptor.Included {
			r.actions[action] = handler
		}
	}
	sort.Strings(m.Descriptor.Actions)
	m.Actions = copied
	r.modules = append(r.modules, m)
	return nil
}

func (r *Registry[S]) Lookup(action string) (Handler[S], bool) {
	handler, ok := r.actions[action]
	return handler, ok
}

func (r *Registry[S]) List() []Descriptor {
	items := make([]Descriptor, 0, len(r.modules))
	for _, m := range r.modules {
		d := m.Descriptor
		d.Actions = append([]string{}, d.Actions...)
		items = append(items, d)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func (r *Registry[S]) State(snapshot S) map[string]any {
	var values map[string]any
	for _, m := range r.modules {
		if !m.Descriptor.Included || m.Project == nil {
			continue
		}
		if value := m.Project(snapshot); value != nil {
			if values == nil {
				values = make(map[string]any)
			}
			values[m.Descriptor.ID] = value
		}
	}
	return values
}

// Run starts included feature workers and joins them before returning. It owns
// no device policy or independent scheduler; the model supplies guarded workers.
func (r *Registry[S]) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for _, module := range r.modules {
		if !module.Descriptor.Included || module.Run == nil {
			continue
		}
		workers.Add(1)
		go func(work func(context.Context)) { defer workers.Done(); work(ctx) }(module.Run)
	}
	workers.Wait()
}
