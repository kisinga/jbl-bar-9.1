package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"time"
)

// These requests are internal, fixed commands, not a proxy for Cast payloads.
// Native state must still verify receiver volume/mute: this firmware does not
// consistently implement SET_VOLUME as an idempotent absolute setter.
const castControlSender = "jbl-remote-control"

type castControlRequest struct {
	Action string
	Level  *float64
	Muted  *bool
}

type castControlResult struct {
	Level       *float64
	Muted       *bool
	PlayerState string
}

func (r castControlRequest) validate() error {
	switch r.Action {
	case "volume":
		if r.Level != nil && r.Muted == nil && !math.IsNaN(*r.Level) && !math.IsInf(*r.Level, 0) && *r.Level >= 0 && *r.Level <= 1 {
			return nil
		}
	case "mute":
		if r.Muted != nil && r.Level == nil {
			return nil
		}
	case "play", "pause", "stop", "next", "previous":
		if r.Level == nil && r.Muted == nil {
			return nil
		}
	}
	return errors.New("invalid Cast control request")
}

// Unlike the read-only monitor, mutations require a response to this precise
// request ID. A timeout or rejection is final; never retry or fall back to a key.
func awaitCastControl(conn net.Conn, namespace, source, kind string, requestID int) ([]byte, error) {
	for i := 0; i < 32; i++ {
		m, e := readCastMessage(conn)
		if e != nil {
			return nil, e
		}
		if m.destination != castControlSender && m.destination != "*" {
			continue
		}
		var h struct {
			Type      string `json:"type"`
			RequestID *int   `json:"requestId"`
		}
		if e := json.Unmarshal([]byte(m.payload), &h); e != nil {
			return nil, errors.New("invalid Cast control response")
		}
		if m.namespace == castHeartbeat && h.Type == "PING" {
			if e := sendCastMessageFrom(conn, castControlSender, castHeartbeat, m.source, map[string]string{"type": "PONG"}); e != nil {
				return nil, e
			}
			continue
		}
		if m.source != source {
			continue
		}
		if m.namespace == castConnection && h.Type == "CLOSE" {
			return nil, errors.New("Cast control connection closed")
		}
		if m.namespace != namespace || h.RequestID == nil || *h.RequestID != requestID {
			continue
		}
		if h.Type != kind {
			return nil, fmt.Errorf("Cast control rejected (%s)", castDisplayText(h.Type))
		}
		return []byte(m.payload), nil
	}
	return nil, errors.New("Cast control response limit reached")
}

func castReceiverVolume(b []byte) (castControlResult, error) {
	var r struct {
		Status struct {
			Volume struct {
				Level *float64 `json:"level"`
				Muted *bool    `json:"muted"`
			} `json:"volume"`
		} `json:"status"`
	}
	if e := json.Unmarshal(b, &r); e != nil {
		return castControlResult{}, e
	}
	v := r.Status.Volume
	if v.Level != nil && (math.IsNaN(*v.Level) || math.IsInf(*v.Level, 0) || *v.Level < 0 || *v.Level > 1) {
		return castControlResult{}, errors.New("invalid Cast receiver volume")
	}
	return castControlResult{Level: v.Level, Muted: v.Muted}, nil
}

func castControlConnection(conn net.Conn, request castControlRequest) (castControlResult, error) {
	if e := request.validate(); e != nil {
		return castControlResult{}, e
	}
	connect := map[string]any{"type": "CONNECT", "origin": map[string]any{}}
	if e := sendCastMessageFrom(conn, castControlSender, castConnection, "receiver-0", connect); e != nil {
		return castControlResult{}, e
	}
	if e := sendCastMessageFrom(conn, castControlSender, castReceiver, "receiver-0", map[string]any{"type": "GET_STATUS", "requestId": 1}); e != nil {
		return castControlResult{}, e
	}
	b, e := awaitCastControl(conn, castReceiver, "receiver-0", "RECEIVER_STATUS", 1)
	if e != nil {
		return castControlResult{}, e
	}
	if request.Action == "volume" || request.Action == "mute" {
		current, e := castReceiverVolume(b)
		if e != nil {
			return castControlResult{}, e
		}
		if request.Action == "volume" && current.Level == nil || request.Action == "mute" && current.Muted == nil {
			return castControlResult{}, errors.New("Cast receiver omitted requested volume status")
		}
		// Repeating a receiver volume value can toggle mute on this firmware.
		// Native readback remains the caller's authority; a receiver no-op must
		// never be retried merely because native and receiver state disagree.
		if request.Action == "volume" && math.Abs(*request.Level-*current.Level) <= 1e-6 || request.Action == "mute" && *request.Muted == *current.Muted {
			return current, nil
		}
		volume := map[string]any{}
		if request.Action == "volume" {
			volume["level"] = *request.Level
		} else {
			volume["muted"] = *request.Muted
		}
		if e := sendCastMessageFrom(conn, castControlSender, castReceiver, "receiver-0", map[string]any{"type": "SET_VOLUME", "requestId": 2, "volume": volume}); e != nil {
			return castControlResult{}, e
		}
		b, e = awaitCastControl(conn, castReceiver, "receiver-0", "RECEIVER_STATUS", 2)
		if e != nil {
			return castControlResult{}, e
		}
		result, e := castReceiverVolume(b)
		if e != nil {
			return castControlResult{}, e
		}
		if request.Action == "volume" && result.Level == nil || request.Action == "mute" && result.Muted == nil {
			return castControlResult{}, errors.New("Cast receiver omitted requested volume status")
		}
		return result, nil
	}

	s, transport, e := parseCastReceiver(b)
	if e != nil || s.Mirroring || transport == "" {
		return castControlResult{}, errors.New("Cast application has no controllable media session")
	}
	var receiver castReceiverReply
	if e := json.Unmarshal(b, &receiver); e != nil {
		return castControlResult{}, e
	}
	mediaNamespace := false
	for _, app := range receiver.Status.Applications {
		if app.TransportID == transport {
			for _, ns := range app.Namespaces {
				mediaNamespace = mediaNamespace || ns.Name == castMedia
			}
		}
	}
	if !mediaNamespace {
		return castControlResult{}, errors.New("Cast application does not expose media control")
	}
	if e := sendCastMessageFrom(conn, castControlSender, castConnection, transport, connect); e != nil {
		return castControlResult{}, e
	}
	if e := sendCastMessageFrom(conn, castControlSender, castMedia, transport, map[string]any{"type": "GET_STATUS", "requestId": 2}); e != nil {
		return castControlResult{}, e
	}
	b, e = awaitCastControl(conn, castMedia, transport, "MEDIA_STATUS", 2)
	if e != nil {
		return castControlResult{}, e
	}
	var media castMediaReply
	if e := json.Unmarshal(b, &media); e != nil || len(media.Status) != 1 || media.Status[0].MediaSessionID == nil || *media.Status[0].MediaSessionID < 0 {
		return castControlResult{}, errors.New("Cast media session is unavailable or ambiguous")
	}
	m := media.Status[0]
	payload := map[string]any{"requestId": 3, "mediaSessionId": *m.MediaSessionID}
	switch request.Action {
	case "play":
		if m.PlayerState != "PAUSED" {
			return castControlResult{}, errors.New("Cast media is not paused")
		}
		payload["type"] = "PLAY"
	case "pause":
		if m.PlayerState != "PLAYING" || m.SupportedCommands&1 == 0 {
			return castControlResult{}, errors.New("Cast media cannot pause")
		}
		payload["type"] = "PAUSE"
	case "stop":
		if m.PlayerState != "PLAYING" && m.PlayerState != "PAUSED" && m.PlayerState != "BUFFERING" {
			return castControlResult{}, errors.New("Cast media is not active")
		}
		payload["type"] = "STOP"
	case "next", "previous":
		bit, jump := uint64(64), 1
		if request.Action == "previous" {
			bit, jump = 128, -1
		}
		if m.SupportedCommands&bit == 0 {
			return castControlResult{}, errors.New("Cast media does not support queue navigation")
		}
		// Cast maps queue next/previous to QUEUE_UPDATE with jump +/-1.
		payload["type"], payload["jump"] = "QUEUE_UPDATE", jump
	}
	if e := sendCastMessageFrom(conn, castControlSender, castMedia, transport, payload); e != nil {
		return castControlResult{}, e
	}
	b, e = awaitCastControl(conn, castMedia, transport, "MEDIA_STATUS", 3)
	if e != nil {
		return castControlResult{}, e
	}
	// A matching request ID alone does not identify the media that replied.
	// Stop may remove its session; other commands require one matching session.
	var reply castMediaReply
	if e := json.Unmarshal(b, &reply); e != nil || reply.Status == nil {
		return castControlResult{}, errors.New("Cast control omitted media status")
	}
	if request.Action == "stop" && len(reply.Status) == 0 {
		return castControlResult{}, nil
	}
	if len(reply.Status) != 1 || reply.Status[0].MediaSessionID == nil || *reply.Status[0].MediaSessionID != *m.MediaSessionID {
		return castControlResult{}, errors.New("Cast media session changed during control")
	}
	var result CastSnapshot
	if e := applyCastMedia(&result, b); e != nil {
		return castControlResult{}, e
	}
	if result.PlayerState == "" {
		return castControlResult{}, errors.New("Cast control omitted playback state")
	}
	return castControlResult{PlayerState: result.PlayerState}, nil
}

func castControlTLS(parent context.Context, raw net.Conn, request castControlRequest) (castControlResult, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	defer raw.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stopCancellation()
	if deadline, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(deadline)
	}
	// Only castControl dials production connections, always to fixed loopback.
	conn := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if e := conn.HandshakeContext(ctx); e != nil {
		return castControlResult{}, fmt.Errorf("Cast control TLS: %w", e)
	}
	return castControlConnection(conn, request)
}

func castControl(parent context.Context, request castControlRequest) (castControlResult, error) {
	if e := request.validate(); e != nil {
		return castControlResult{}, e
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	raw, e := (&net.Dialer{Timeout: 400 * time.Millisecond}).DialContext(ctx, "tcp", "127.0.0.1:8009")
	if e != nil {
		return castControlResult{}, fmt.Errorf("Cast receiver unavailable: %w", e)
	}
	return castControlTLS(ctx, raw, request)
}
