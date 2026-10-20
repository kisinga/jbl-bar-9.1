package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCastCodec(t *testing.T) {
	want := castMessage{source: "sender", destination: "receiver-0", namespace: castReceiver, payload: `{"type":"GET_STATUS","requestId":1}`}
	b := encodeCastMessage(want)
	// Unknown protobuf fields must remain forward compatible.
	b = append(b, 0xa0, 0x01, 0x01, 0xa9, 0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0xb5, 0x01, 0, 0, 0, 0)
	got, e := decodeCastMessage(b)
	if e != nil || got != want {
		t.Fatal(got, e)
	}
	var frame bytes.Buffer
	if e := sendCastMessage(&frame, castReceiver, "receiver-0", map[string]any{"type": "GET_STATUS", "requestId": 1}); e != nil {
		t.Fatal(e)
	}
	got, e = readCastMessage(&frame)
	if e != nil || got.source != castSender || got.namespace != castReceiver || !strings.Contains(got.payload, "GET_STATUS") {
		t.Fatal(got, e)
	}
}
func TestCastRejectsMalformedMessages(t *testing.T) {
	good := encodeCastMessage(castMessage{source: "receiver-0", destination: castSender, namespace: castReceiver, payload: `{}`})
	cases := [][]byte{
		nil, {0}, {0x80}, {0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x02},
		{0x12, 0x7f}, {0x0f}, {0x09, 0}, {0x15, 0}, {0x12, 1, 0xff},
		append(append([]byte{}, good...), 40, 1), // Binary payload declaration.
		append(append([]byte{}, good...), 42, 0), // Wrong wire type.
		bytes.Repeat([]byte{0xa0, 1, 1}, 129),
		make([]byte, castFrameLimit+1),
	}
	for i, b := range cases {
		if _, e := decodeCastMessage(b); e == nil {
			t.Fatalf("malformed case %d accepted", i)
		}
	}
	for _, size := range []uint32{0, castFrameLimit + 1, math.MaxUint32} {
		var h [4]byte
		binary.BigEndian.PutUint32(h[:], size)
		if _, e := readCastMessage(bytes.NewReader(h[:])); e == nil {
			t.Fatalf("invalid frame size %d accepted", size)
		}
	}
	var short bytes.Buffer
	_ = binary.Write(&short, binary.BigEndian, uint32(12))
	short.Write([]byte{8, 0})
	if _, e := readCastMessage(&short); e == nil {
		t.Fatal("truncated frame accepted")
	}
}
func TestCastReceiverClassification(t *testing.T) {
	cases := []struct {
		name, input, app string
		mirror           bool
	}{
		{"idle", `{"type":"RECEIVER_STATUS","status":{}}`, "", false},
		{"idle_screen", `{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"idle","displayName":"Backdrop","isIdleScreen":true,"transportId":"private"}]}}`, "", false},
		{"mirror_id", `{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"85CDB22F","displayName":"Chrome Audio Mirroring","transportId":"private"}]}}`, "Chrome Audio Mirroring", true},
		{"mirror_name", `{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"other","displayName":"Screen Mirroring","transportId":"private"}]}}`, "Screen Mirroring", true},
		{"media", `{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"example","displayName":"Music app","transportId":"private"}]}}`, "Music app", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, transport, e := parseCastReceiver([]byte(c.input))
			if e != nil || !s.Available || s.Application != c.app || s.Mirroring != c.mirror || (transport != "") != (c.app != "") {
				t.Fatal(s, transport, e)
			}
			public, _ := json.Marshal(s)
			if bytes.Contains(public, []byte("private")) || bytes.Contains(public, []byte("transport")) {
				t.Fatal("private transport data exposed")
			}
		})
	}
	if _, _, e := parseCastReceiver([]byte(`{"type":"MEDIA_STATUS"}`)); e == nil {
		t.Fatal("wrong message accepted")
	}
}
func TestCastMediaMetadata(t *testing.T) {
	s, _, e := parseCastReceiver([]byte(`{"type":"RECEIVER_STATUS","status":{"userEq":{"low_shelf":{"gain_db":-2.5},"high_shelf":{"gain_db":3}}}}`))
	if e != nil || s.LowShelfGain == nil || *s.LowShelfGain != -2.5 || s.HighShelfGain == nil || *s.HighShelfGain != 3 {
		t.Fatal(s, e)
	}
	e = applyCastMedia(&s, []byte(`{"type":"MEDIA_STATUS","status":[{"playerState":"PAUSED","currentTime":12.5,"media":{"streamType":"BUFFERED","duration":180,"metadata":{"title":"Example song","artist":"Example artist","albumName":"Example album"}}}]}`))
	if e != nil || s.PlayerState != "PAUSED" || s.Title != "Example song" || s.Artist != "Example artist" || s.Album != "Example album" || s.StreamType != "BUFFERED" || s.CurrentTime == nil || *s.CurrentTime != 12.5 || s.Duration == nil || *s.Duration != 180 {
		t.Fatal(s, e)
	}
	s = CastSnapshot{Mirroring: true}
	e = applyCastMedia(&s, []byte(`{"type":"MEDIA_STATUS","status":[{"playerState":"PLAYING","mediaSessionId":0,"supportedMediaCommands":0,"media":{"streamType":"LIVE","contentType":"video/webm"}}]}`))
	if e != nil || !s.Mirroring || s.PlayerState != "PLAYING" || s.StreamType != "LIVE" || s.Duration != nil {
		t.Fatal("mirroring classification was lost", s, e)
	}
	s = CastSnapshot{}
	e = applyCastMedia(&s, []byte(`{"type":"MEDIA_STATUS","status":[{"playerState":"NEW_UNKNOWN_STATE","currentTime":-1,"media":{"streamType":"UNKNOWN","duration":-9}}]}`))
	if e != nil || s.PlayerState != "" || s.StreamType != "" || s.CurrentTime != nil || s.Duration != nil {
		t.Fatal("unverified values exposed", s, e)
	}
	if e := applyCastMedia(&s, []byte(`{"type":"INVALID_REQUEST"}`)); e == nil {
		t.Fatal("invalid media reply accepted")
	}
	if castDisplayText(" hello\nworld\x00 ") != "hello world" || len([]rune(castDisplayText(strings.Repeat("界", 250)))) != 240 {
		t.Fatal("display metadata not bounded")
	}
	for _, n := range []float64{math.NaN(), math.Inf(1)} {
		if castNumber(&n, false) != nil {
			t.Fatal("non-finite number exposed")
		}
	}
}
func writeFakeCast(conn net.Conn, source, namespace, payload string) error {
	b := encodeCastMessage(castMessage{source: source, destination: castSender, namespace: namespace, payload: payload})
	var p bytes.Buffer
	_ = binary.Write(&p, binary.BigEndian, uint32(len(b)))
	p.Write(b)
	_, e := io.Copy(conn, &p)
	return e
}
func TestCastTransportIsReadOnly(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	_ = server.SetDeadline(time.Now().Add(time.Second))
	result := make(chan error, 1)
	go func() {
		var e error
		defer func() { result <- e }()
		for phase := 0; phase < 2; phase++ {
			for _, want := range []string{"CONNECT", "GET_STATUS"} {
				var m castMessage
				m, e = readCastMessage(server)
				if e != nil {
					return
				}
				var msg map[string]any
				e = json.Unmarshal([]byte(m.payload), &msg)
				if e != nil || msg["type"] != want {
					e = io.ErrUnexpectedEOF
					return
				}
			}
			if phase == 0 {
				e = writeFakeCast(server, "receiver-0", castReceiver, `{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"music","displayName":"Music app","transportId":"app-transport","namespaces":[{"name":"urn:x-cast:com.google.cast.media"}]}]}}`)
			} else {
				e = writeFakeCast(server, "app-transport", castMedia, `{"type":"MEDIA_STATUS","status":[{"playerState":"PAUSED","media":{"metadata":{"title":"Fixture track"}}}]}`)
			}
			if e != nil {
				return
			}
		}
	}()
	s, e := readCastConnection(client)
	if e != nil || s.Application != "Music app" || s.PlayerState != "PAUSED" || s.Title != "Fixture track" {
		t.Fatal(s, e)
	}
	if e := <-result; e != nil {
		t.Fatal(e)
	}
}

func TestCastHeartbeatAndResponseSource(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	_ = server.SetDeadline(time.Now().Add(time.Second))
	result := make(chan error, 1)
	go func() {
		e := writeFakeCast(server, "receiver-0", castHeartbeat, `{"type":"PING"}`)
		if e == nil {
			var m castMessage
			m, e = readCastMessage(server)
			if e == nil && (m.namespace != castHeartbeat || m.destination != "receiver-0" || m.payload != `{"type":"PONG"}`) {
				e = io.ErrUnexpectedEOF
			}
		}
		if e == nil {
			e = writeFakeCast(server, "unrelated-app", castMedia, `{"type":"MEDIA_STATUS","status":[{"playerState":"PLAYING"}]}`)
		}
		if e == nil {
			e = writeFakeCast(server, "wanted-app", castMedia, `{"type":"MEDIA_STATUS","status":[{"playerState":"PAUSED"}]}`)
		}
		result <- e
	}()
	b, e := awaitCastStatus(client, castMedia, "MEDIA_STATUS", "wanted-app")
	if e != nil || !bytes.Contains(b, []byte("PAUSED")) {
		t.Fatal(string(b), e)
	}
	if e := <-result; e != nil {
		t.Fatal(e)
	}
}
func TestCastKeepsReceiverWhenMediaUnavailable(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	_ = server.SetDeadline(time.Now().Add(time.Second))
	result := make(chan error, 1)
	go func() {
		defer server.Close()
		for i := 0; i < 2; i++ {
			if _, e := readCastMessage(server); e != nil {
				result <- e
				return
			}
		}
		e := writeFakeCast(server, "receiver-0", castReceiver, `{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"85CDB22F","displayName":"Chrome Audio Mirroring","transportId":"private"}]}}`)
		if e == nil {
			for i := 0; i < 2; i++ {
				if _, e = readCastMessage(server); e != nil {
					break
				}
			}
		}
		result <- e
	}()
	s, e := readCastConnection(client)
	if e != nil || !s.Available || !s.Mirroring || s.PlayerState != "" {
		t.Fatal("optional media failure discarded known receiver", s, e)
	}
	if e := <-result; e != nil {
		t.Fatal(e)
	}
}

func TestMirroringDoesNotClaimSenderPlayback(t *testing.T) {
	mirror := CastSnapshot{Available: true, Application: "Chrome Audio Mirroring", Mirroring: true, PlayerState: "PLAYING"}
	s := castStream(2, mirror)
	if s.Status != "Audio mirroring" || s.Detail == "" || s.Cast == nil {
		t.Fatal("mirroring presented as ordinary playback", s)
	}
	normal := CastSnapshot{Available: true, Application: "Audio app", PlayerState: "PAUSED"}
	if castStream(2, normal).Status != "Paused" {
		t.Fatal("receiver media state did not override transport")
	}
	if castStream(2, CastSnapshot{}).Status == "Playing" {
		t.Fatal("unknown application inferred as playing")
	}
	if castStream(2, CastSnapshot{Available: true}).Status != "Not casting" {
		t.Fatal("missing receiver application ignored")
	}
}
