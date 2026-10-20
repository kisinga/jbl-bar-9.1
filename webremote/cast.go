package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// CastSnapshot contains display information only. Transport IDs, network
// identifiers and arbitrary receiver payloads never leave the status client.
type CastSnapshot struct {
	Available     bool     `json:"available"`
	Application   string   `json:"application,omitempty"`
	AppID         string   `json:"app_id,omitempty"`
	Mirroring     bool     `json:"mirroring"`
	PlayerState   string   `json:"player_state,omitempty"`
	StreamType    string   `json:"stream_type,omitempty"`
	Title         string   `json:"title,omitempty"`
	Artist        string   `json:"artist,omitempty"`
	Album         string   `json:"album,omitempty"`
	CurrentTime   *float64 `json:"current_time,omitempty"`
	Duration      *float64 `json:"duration,omitempty"`
	LowShelfGain  *float64 `json:"low_shelf_gain,omitempty"`
	HighShelfGain *float64 `json:"high_shelf_gain,omitempty"`
}

const (
	castFrameLimit = 131072
	castConnection = "urn:x-cast:com.google.cast.tp.connection"
	castHeartbeat  = "urn:x-cast:com.google.cast.tp.heartbeat"
	castReceiver   = "urn:x-cast:com.google.cast.receiver"
	castMedia      = "urn:x-cast:com.google.cast.media"
	castSender     = "jbl-remote-status"
)

type castMessage struct {
	source, destination, namespace, payload string
}

func castVarint(dst []byte, value uint64) []byte {
	for value >= 128 {
		dst = append(dst, byte(value)|128)
		value >>= 7
	}
	return append(dst, byte(value))
}
func castStringField(dst []byte, field uint64, value string) []byte {
	dst = castVarint(dst, field<<3|2)
	dst = castVarint(dst, uint64(len(value)))
	return append(dst, value...)
}
func encodeCastMessage(m castMessage) []byte {
	b := []byte{8, 0} // CASTV2_1_0.
	b = castStringField(b, 2, m.source)
	b = castStringField(b, 3, m.destination)
	b = castStringField(b, 4, m.namespace)
	b = append(b, 40, 0) // UTF-8 JSON payload.
	return castStringField(b, 6, m.payload)
}
func takeCastVarint(b []byte, pos *int) (uint64, error) {
	var value uint64
	for shift := uint(0); shift < 64; shift += 7 {
		if *pos >= len(b) {
			return 0, io.ErrUnexpectedEOF
		}
		v := b[*pos]
		*pos = *pos + 1
		if shift == 63 && v > 1 {
			return 0, errors.New("invalid Cast varint")
		}
		value |= uint64(v&127) << shift
		if v < 128 {
			return value, nil
		}
	}
	return 0, errors.New("invalid Cast varint")
}
func decodeCastMessage(b []byte) (castMessage, error) {
	var m castMessage
	if len(b) == 0 || len(b) > castFrameLimit {
		return m, errors.New("invalid Cast message size")
	}
	for pos, fields := 0, 0; pos < len(b); fields++ {
		if fields >= 128 {
			return m, errors.New("too many Cast fields")
		}
		key, e := takeCastVarint(b, &pos)
		if e != nil || key>>3 == 0 {
			return m, errors.New("invalid Cast field")
		}
		field, wire := key>>3, key&7
		if (field == 1 || field == 5) && wire != 0 {
			return m, errors.New("invalid Cast numeric field")
		}
		var value []byte
		switch wire {
		case 0:
			v, e := takeCastVarint(b, &pos)
			if e != nil || field == 5 && v != 0 {
				return m, errors.New("invalid Cast payload type")
			}
		case 1, 5:
			n := 8
			if wire == 5 {
				n = 4
			}
			if n > len(b)-pos {
				return m, io.ErrUnexpectedEOF
			}
			pos += n
		case 2:
			n, e := takeCastVarint(b, &pos)
			if e != nil || n > uint64(len(b)-pos) {
				return m, io.ErrUnexpectedEOF
			}
			value = b[pos : pos+int(n)]
			pos += int(n)
		default:
			return m, errors.New("unsupported Cast wire type")
		}
		if field == 2 || field == 3 || field == 4 || field == 6 {
			if wire != 2 || !utf8.Valid(value) {
				return m, errors.New("invalid Cast string")
			}
			switch field {
			case 2:
				m.source = string(value)
			case 3:
				m.destination = string(value)
			case 4:
				m.namespace = string(value)
			case 6:
				m.payload = string(value)
			}
		}
	}
	if m.source == "" || m.namespace == "" || m.payload == "" {
		return m, errors.New("incomplete Cast message")
	}
	return m, nil
}
func readCastMessage(r io.Reader) (castMessage, error) {
	var header [4]byte
	if _, e := io.ReadFull(r, header[:]); e != nil {
		return castMessage{}, e
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > castFrameLimit {
		return castMessage{}, errors.New("invalid Cast frame size")
	}
	b := make([]byte, int(size))
	if _, e := io.ReadFull(r, b); e != nil {
		return castMessage{}, e
	}
	return decodeCastMessage(b)
}
func sendCastMessage(w io.Writer, namespace, destination string, payload any) error {
	j, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	b := encodeCastMessage(castMessage{source: castSender, destination: destination, namespace: namespace, payload: string(j)})
	packet := make([]byte, 4, 4+len(b))
	binary.BigEndian.PutUint32(packet, uint32(len(b)))
	packet = append(packet, b...)
	for len(packet) > 0 {
		n, e := w.Write(packet)
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		packet = packet[n:]
	}
	return nil
}

type castApplication struct {
	AppID       string `json:"appId"`
	DisplayName string `json:"displayName"`
	TransportID string `json:"transportId"`
	Idle        bool   `json:"isIdleScreen"`
	Namespaces  []struct {
		Name string `json:"name"`
	} `json:"namespaces"`
}
type castReceiverReply struct {
	Type   string `json:"type"`
	Status struct {
		Applications []castApplication `json:"applications"`
		UserEQ       struct {
			LowShelf struct {
				Gain *float64 `json:"gain_db"`
			} `json:"low_shelf"`
			HighShelf struct {
				Gain *float64 `json:"gain_db"`
			} `json:"high_shelf"`
		} `json:"userEq"`
	} `json:"status"`
}
type castMediaReply struct {
	Type   string `json:"type"`
	Status []struct {
		PlayerState string   `json:"playerState"`
		CurrentTime *float64 `json:"currentTime"`
		Media       struct {
			StreamType string   `json:"streamType"`
			Duration   *float64 `json:"duration"`
			Metadata   struct {
				Title  string `json:"title"`
				Artist string `json:"artist"`
				Album  string `json:"albumName"`
			} `json:"metadata"`
		} `json:"media"`
	} `json:"status"`
}

func castDisplayText(s string) string {
	if !utf8.ValidString(s) {
		return ""
	}
	r := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
	if len(r) > 240 {
		r = r[:240]
	}
	return strings.TrimSpace(string(r))
}
func castNumber(n *float64, nonnegative bool) *float64 {
	if n == nil || math.IsNaN(*n) || math.IsInf(*n, 0) || nonnegative && *n < 0 {
		return nil
	}
	v := *n
	return &v
}
func parseCastReceiver(b []byte) (CastSnapshot, string, error) {
	var reply castReceiverReply
	if e := json.Unmarshal(b, &reply); e != nil || reply.Type != "RECEIVER_STATUS" {
		return CastSnapshot{}, "", errors.New("invalid Cast receiver status")
	}
	s := CastSnapshot{Available: true, LowShelfGain: castNumber(reply.Status.UserEQ.LowShelf.Gain, false), HighShelfGain: castNumber(reply.Status.UserEQ.HighShelf.Gain, false)}
	var chosen *castApplication
	for i := range reply.Status.Applications {
		a := &reply.Status.Applications[i]
		if a.Idle || a.TransportID == "" || len(a.TransportID) > 256 {
			continue
		}
		if chosen == nil {
			chosen = a
		}
		for _, ns := range a.Namespaces {
			if ns.Name == castMedia {
				chosen = a
				break
			}
		}
	}
	if chosen == nil {
		return s, "", nil
	}
	s.Application, s.AppID = castDisplayText(chosen.DisplayName), castDisplayText(chosen.AppID)
	s.Mirroring = chosen.AppID == "85CDB22F" || strings.Contains(strings.ToLower(chosen.DisplayName), "mirroring")
	return s, chosen.TransportID, nil
}
func applyCastMedia(s *CastSnapshot, b []byte) error {
	var reply castMediaReply
	if e := json.Unmarshal(b, &reply); e != nil || reply.Type != "MEDIA_STATUS" {
		return errors.New("invalid Cast media status")
	}
	if len(reply.Status) == 0 {
		return nil
	}
	m := reply.Status[0]
	switch m.PlayerState {
	case "IDLE", "PLAYING", "PAUSED", "BUFFERING":
		s.PlayerState = m.PlayerState
	}
	switch m.Media.StreamType {
	case "NONE", "BUFFERED", "LIVE":
		s.StreamType = m.Media.StreamType
	}
	s.CurrentTime, s.Duration = castNumber(m.CurrentTime, true), castNumber(m.Media.Duration, true)
	s.Title, s.Artist, s.Album = castDisplayText(m.Media.Metadata.Title), castDisplayText(m.Media.Metadata.Artist), castDisplayText(m.Media.Metadata.Album)
	return nil
}
func awaitCastStatus(conn net.Conn, namespace, kind, source string) ([]byte, error) {
	for i := 0; i < 16; i++ {
		m, e := readCastMessage(conn)
		if e != nil {
			return nil, e
		}
		if m.destination != castSender && m.destination != "*" {
			continue
		}
		var header struct {
			Type string `json:"type"`
		}
		if e := json.Unmarshal([]byte(m.payload), &header); e != nil {
			return nil, e
		}
		if m.namespace == castHeartbeat && header.Type == "PING" {
			if e := sendCastMessage(conn, castHeartbeat, m.source, map[string]string{"type": "PONG"}); e != nil {
				return nil, e
			}
			continue
		}
		if m.namespace == namespace && m.source == source && header.Type == kind {
			return []byte(m.payload), nil
		}
		if m.namespace == castConnection && header.Type == "CLOSE" && m.source == source {
			return nil, errors.New("Cast status connection closed")
		}
	}
	return nil, errors.New("Cast status response limit reached")
}
func readCastConnection(conn net.Conn) (CastSnapshot, error) {
	connect := map[string]any{"type": "CONNECT", "origin": map[string]any{}}
	status := map[string]any{"type": "GET_STATUS", "requestId": 1}
	if e := sendCastMessage(conn, castConnection, "receiver-0", connect); e != nil {
		return CastSnapshot{}, e
	}
	if e := sendCastMessage(conn, castReceiver, "receiver-0", status); e != nil {
		return CastSnapshot{}, e
	}
	b, e := awaitCastStatus(conn, castReceiver, "RECEIVER_STATUS", "receiver-0")
	if e != nil {
		return CastSnapshot{}, e
	}
	s, transport, e := parseCastReceiver(b)
	if e != nil || transport == "" {
		return s, e
	}
	// A receiver can expose an app without a media session. Preserve its name
	// and mirroring classification when the optional media query is unavailable.
	if e := sendCastMessage(conn, castConnection, transport, connect); e != nil {
		return s, nil
	}
	status["requestId"] = 2
	if e := sendCastMessage(conn, castMedia, transport, status); e != nil {
		return s, nil
	}
	b, e = awaitCastStatus(conn, castMedia, "MEDIA_STATUS", transport)
	if e == nil {
		_ = applyCastMedia(&s, b)
	}
	return s, nil
}

// readCastSnapshot queries only the soundbar's own Cast receiver. Cache it in a
// background monitor; never block a button request on this optional service.
func readCastSnapshot(parent context.Context) (CastSnapshot, error) {
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()
	raw, e := (&net.Dialer{Timeout: 400 * time.Millisecond}).DialContext(ctx, "tcp", "127.0.0.1:8009")
	if e != nil {
		return CastSnapshot{}, fmt.Errorf("Cast receiver unavailable: %w", e)
	}
	defer raw.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stopCancellation()
	if deadline, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(deadline)
	}
	// Cast devices use their own certificate chain. Verification is skipped
	// solely for this fixed loopback endpoint; callers cannot choose a host.
	conn := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if e = conn.HandshakeContext(ctx); e != nil {
		return CastSnapshot{}, fmt.Errorf("Cast status TLS: %w", e)
	}
	return readCastConnection(conn)
}
