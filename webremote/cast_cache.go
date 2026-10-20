package main

import (
	"context"
	"sync"
	"time"
)

type castStatusCache struct {
	mu       sync.Mutex
	state    CastSnapshot
	updated  time.Time
	requests chan struct{}
}

var castStatus = castStatusCache{requests: make(chan struct{}, 1)}

func cachedCastStatus() CastSnapshot {
	select {
	case castStatus.requests <- struct{}{}:
	default:
	}
	castStatus.mu.Lock()
	defer castStatus.mu.Unlock()
	if time.Since(castStatus.updated) > 6*time.Second {
		return CastSnapshot{}
	}
	return castStatus.state
}
func monitorCast(ctx context.Context) {
	var next time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-castStatus.requests:
			if time.Now().Before(next) {
				continue
			}
			next = time.Now().Add(2 * time.Second)
			snapshot, e := readCastSnapshot(ctx)
			if e != nil {
				snapshot = CastSnapshot{}
			}
			castStatus.mu.Lock()
			castStatus.state = snapshot
			castStatus.updated = time.Now()
			castStatus.mu.Unlock()
		}
	}
}
func castStream(native uint32, metadata CastSnapshot) *streamState {
	s := &streamState{Service: "Chromecast", Status: "Selected"}
	// A playing Cast transport may carry silent browser mirroring. Its state
	// alone does not prove that the sender's video or music is playing.
	if native == 2 {
		s.Status = "Stream active"
		s.Active = true
	} else if native == 3 {
		s.Status = "Stream paused"
	}
	if !metadata.Available {
		return s
	}
	s.Cast = &metadata
	if metadata.Application == "" {
		s.Status = "Not casting"
		s.Active = false
		return s
	}
	if metadata.Mirroring {
		s.Status = "Audio mirroring"
		s.Active = true
		s.Detail = "The sending app’s play/pause state isn’t reported by mirrored audio."
		if metadata.PlayerState == "PAUSED" {
			s.Status = "Mirroring paused"
			s.Active = false
		}
		return s
	}
	switch metadata.PlayerState {
	case "PLAYING":
		s.Status = "Playing"
		s.Active = true
	case "PAUSED":
		s.Status = "Paused"
		s.Active = false
	case "BUFFERING":
		s.Status = "Buffering"
		s.Active = false
	case "IDLE":
		s.Status = "Idle"
		s.Active = false
	default:
		s.Status = "Connected"
		s.Active = false
	}
	return s
}
