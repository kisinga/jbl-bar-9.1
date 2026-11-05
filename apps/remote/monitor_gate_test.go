package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func shortGate(t *testing.T, delay time.Duration, retries int) {
	t.Helper()
	oldDelay, oldRetries := gateRetryDelay, gateRetries
	gateRetryDelay, gateRetries = delay, retries
	t.Cleanup(func() { gateRetryDelay, gateRetries = oldDelay, oldRetries })
}

func TestHoldWaitsForAShortHolder(t *testing.T) {
	shortGate(t, 5*time.Millisecond, 8)
	r := &remote{}
	r.actionMu.Lock()
	time.AfterFunc(12*time.Millisecond, r.actionMu.Unlock)
	release, _, ok := r.hold("")
	if !ok {
		t.Fatal("gate not taken after the holder released it")
	}
	release()
}

func TestHoldSkipsTheTickDuringALongControl(t *testing.T) {
	shortGate(t, 2*time.Millisecond, 3)
	r := &remote{}
	r.actionMu.Lock()
	defer r.actionMu.Unlock()
	start := time.Now()
	if _, _, ok := r.hold(""); ok {
		t.Fatal("gate taken while held")
	}
	if waited := time.Since(start); waited > 200*time.Millisecond {
		t.Fatalf("waited %v; the retries must stay well inside one tick", waited)
	}
}

// Four monitors ticking in phase, as on the bar (automation and bass offset
// every second, center and Front EQ every two). With a bare TryLock one of
// them won every shared tick; each must now run on nearly every tick.
func TestMonitorsInPhaseAllRun(t *testing.T) {
	shortGate(t, 3*time.Millisecond, 8)
	r := &remote{}
	ctx, cancel := context.WithCancel(context.Background())
	periods := []time.Duration{40, 40, 80, 80}
	var ran, ticks [4]atomic.Int64
	var wg sync.WaitGroup
	for i, p := range periods {
		wg.Add(1)
		go func(i int, p time.Duration) {
			defer wg.Done()
			ticker := time.NewTicker(p * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				ticks[i].Add(1)
				release, _, ok := r.hold("")
				if !ok {
					continue
				}
				ran[i].Add(1)
				time.Sleep(time.Millisecond) // a monitor's native reads
				release()
			}
		}(i, p)
	}
	time.Sleep(1600 * time.Millisecond)
	cancel()
	wg.Wait()
	for i := range periods {
		if ticks[i].Load() == 0 || ran[i].Load()*10 < ticks[i].Load()*9 {
			t.Errorf("monitor %d ran %d of %d ticks", i, ran[i].Load(), ticks[i].Load())
		}
	}
}
