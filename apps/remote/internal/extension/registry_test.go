package extension

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCompositionAndOwnership(t *testing.T) {
	var r Registry[int]
	m := Module[int]{Descriptor: Descriptor{ID: "bar91.eq", Included: true}, Actions: map[string]Handler[int]{
		"bar91.eq.apply": func(context.Context, json.RawMessage) (int, error) { return 42, nil },
	}, Project: func(s int) any { return s + 1 }}
	if err := r.Register(m); err != nil {
		t.Fatal(err)
	}
	delete(m.Actions, "bar91.eq.apply")
	listed := r.List()
	listed[0].Actions[0] = "changed"
	handler, ok := r.Lookup("bar91.eq.apply")
	if !ok {
		t.Fatal("caller changed registered routing")
	}
	if value, err := handler(context.Background(), nil); value != 42 || err != nil {
		t.Fatal(value, err)
	}
	if r.List()[0].Actions[0] != "bar91.eq.apply" || r.State(10)["bar91.eq"] != 11 {
		t.Fatal("metadata or projection changed")
	}
	if err := r.Register(m); err == nil {
		t.Fatal("duplicate ID accepted")
	}
}

func TestWorkersRespectInclusionAndFinishRestoration(t *testing.T) {
	var registry Registry[int]
	started, restored, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	err := registry.Register(Module[int]{Descriptor: Descriptor{ID: "fixture.worker", Included: true}, Run: func(ctx context.Context) { close(started); <-ctx.Done(); <-restored }})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.Register(Module[int]{Descriptor: Descriptor{ID: "fixture.omitted", Included: false}, Run: func(context.Context) { t.Error("omitted worker started") }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { registry.Run(ctx); close(done) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("worker restoration was not joined")
	default:
	}
	close(restored)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not finish")
	}
}

func TestExcludedFeaturesCannotRunOrProject(t *testing.T) {
	var r Registry[int]
	m := Module[int]{Descriptor: Descriptor{ID: "bar91.eq", Included: false}, Actions: map[string]Handler[int]{
		"bar91.eq.nested.apply": func(context.Context, json.RawMessage) (int, error) { t.Fatal("excluded action ran"); return 0, nil },
	}, Project: func(int) any { t.Fatal("excluded projection ran"); return nil }}
	if err := r.Register(m); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Lookup("bar91.eq.nested.apply"); ok || len(r.State(1)) != 0 {
		t.Fatal("excluded module exposed runtime behavior")
	}
	if r.List()[0].Included || len(r.List()[0].Actions) != 1 {
		t.Fatal("availability metadata lost")
	}
	// Nested namespaces cannot claim an action reserved by an excluded module.
	m.Descriptor.ID = "bar91.eq.nested"
	m.Actions = map[string]Handler[int]{"bar91.eq.nested.apply": func(context.Context, json.RawMessage) (int, error) { return 0, nil }}
	if err := r.Register(m); err == nil {
		t.Fatal("invalid namespace collision accepted")
	}
}
