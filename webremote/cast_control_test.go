package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

const castControlReceiverFixture = `{"type":"RECEIVER_STATUS","requestId":1,"status":{"volume":{"level":0.2,"muted":false},"applications":[{"appId":"fixture","displayName":"Fixture music","transportId":"fixture-transport","namespaces":[{"name":"urn:x-cast:com.google.cast.media"}]}]}}`

func writeFakeCastControl(conn net.Conn, source, namespace, payload string) error {
	return sendCastMessageFrom(conn, source, namespace, castControlSender, json.RawMessage(payload))
}

func fakeCastControlRead(conn net.Conn, namespace, destination, kind string, requestID int) (map[string]any, error) {
	m, e := readCastMessage(conn)
	if e != nil {
		return nil, e
	}
	var payload map[string]any
	if e := json.Unmarshal([]byte(m.payload), &payload); e != nil {
		return nil, e
	}
	if m.source != castControlSender || m.namespace != namespace || m.destination != destination || payload["type"] != kind {
		return nil, fmt.Errorf("unexpected message %+v", m)
	}
	if requestID > 0 && payload["requestId"] != float64(requestID) {
		return nil, fmt.Errorf("unexpected request ID: %v", payload["requestId"])
	}
	return payload, nil
}

func fakeCastControlReceiver(conn net.Conn, fixture string) error {
	if _, e := fakeCastControlRead(conn, castConnection, "receiver-0", "CONNECT", 0); e != nil {
		return e
	}
	if _, e := fakeCastControlRead(conn, castReceiver, "receiver-0", "GET_STATUS", 1); e != nil {
		return e
	}
	return writeFakeCastControl(conn, "receiver-0", castReceiver, fixture)
}

func castControlPipe(t *testing.T, request castControlRequest, serverFunc func(net.Conn) error) (castControlResult, error) {
	t.Helper()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	_ = server.SetDeadline(time.Now().Add(2 * time.Second))
	done := make(chan error, 1)
	go func() {
		defer server.Close()
		done <- serverFunc(server)
	}()
	result, e := castControlConnection(client, request)
	if serverError := <-done; serverError != nil {
		t.Fatal("fake receiver:", serverError)
	}
	return result, e
}

func TestCastControlReceiverVolumeAndMute(t *testing.T) {
	level, muted := 0.31, true
	for _, request := range []castControlRequest{{Action: "volume", Level: &level}, {Action: "mute", Muted: &muted}} {
		t.Run(request.Action, func(t *testing.T) {
			result, e := castControlPipe(t, request, func(conn net.Conn) error {
				if e := fakeCastControlReceiver(conn, castControlReceiverFixture); e != nil {
					return e
				}
				p, e := fakeCastControlRead(conn, castReceiver, "receiver-0", "SET_VOLUME", 2)
				if e != nil {
					return e
				}
				v, ok := p["volume"].(map[string]any)
				if !ok || len(v) != 1 || len(p) != 3 {
					return errors.New("mutation must contain only requested volume field")
				}
				if request.Action == "volume" && v["level"] != level || request.Action == "mute" && v["muted"] != muted {
					return errors.New("wrong volume field value")
				}
				// A status from an unrelated request must never acknowledge mutation.
				if e := writeFakeCastControl(conn, "receiver-0", castReceiver, `{"type":"RECEIVER_STATUS","requestId":1,"status":{"volume":{"level":0.9,"muted":false}}}`); e != nil {
					return e
				}
				return writeFakeCastControl(conn, "receiver-0", castReceiver, `{"type":"RECEIVER_STATUS","requestId":2,"status":{"volume":{"level":0.31,"muted":true}}}`)
			})
			if e != nil || result.Level == nil || *result.Level != level || result.Muted == nil || !*result.Muted {
				t.Fatal(result, e)
			}
		})
	}
}

func TestCastControlReceiverNoOpAndMissingStatusNeverMutate(t *testing.T) {
	level, closeLevel, changedLevel, muted := 0.2, 0.2000005, 0.4, false
	for _, tc := range []struct {
		name    string
		request castControlRequest
		fixture string
		wantErr bool
	}{
		{"same_volume", castControlRequest{Action: "volume", Level: &level}, castControlReceiverFixture, false},
		{"volume_rounding", castControlRequest{Action: "volume", Level: &closeLevel}, castControlReceiverFixture, false},
		{"same_mute", castControlRequest{Action: "mute", Muted: &muted}, castControlReceiverFixture, false},
		{"missing_volume", castControlRequest{Action: "volume", Level: &changedLevel}, `{"type":"RECEIVER_STATUS","requestId":1,"status":{"volume":{"muted":false}}}`, true},
		{"missing_mute", castControlRequest{Action: "mute", Muted: &muted}, `{"type":"RECEIVER_STATUS","requestId":1,"status":{"volume":{"level":0.2}}}`, true},
		{"invalid_volume", castControlRequest{Action: "volume", Level: &changedLevel}, `{"type":"RECEIVER_STATUS","requestId":1,"status":{"volume":{"level":1.5,"muted":false}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, e := castControlPipe(t, tc.request, func(conn net.Conn) error {
				if e := fakeCastControlReceiver(conn, tc.fixture); e != nil {
					return e
				}
				// The caller remains connected while this check runs. Any extra
				// protocol byte proves an accidental mutation or retry occurred.
				_ = conn.SetReadDeadline(time.Now().Add(40 * time.Millisecond))
				var b [1]byte
				n, e := conn.Read(b[:])
				if n != 0 {
					return errors.New("no-op or invalid status emitted a command")
				}
				if timeout, ok := e.(net.Error); !ok || !timeout.Timeout() {
					return fmt.Errorf("unexpected no-command read: %v", e)
				}
				return nil
			})
			if (e != nil) != tc.wantErr {
				t.Fatal(result, e)
			}
			if !tc.wantErr && (tc.request.Action == "volume" && (result.Level == nil || *result.Level != level) || tc.request.Action == "mute" && (result.Muted == nil || *result.Muted)) {
				t.Fatal("no-op must return current receiver values", result)
			}
		})
	}
}

func fakeCastControlMedia(conn net.Conn, player string, capabilities int) error {
	if e := fakeCastControlReceiver(conn, castControlReceiverFixture); e != nil {
		return e
	}
	if _, e := fakeCastControlRead(conn, castConnection, "fixture-transport", "CONNECT", 0); e != nil {
		return e
	}
	if _, e := fakeCastControlRead(conn, castMedia, "fixture-transport", "GET_STATUS", 2); e != nil {
		return e
	}
	return writeFakeCastControl(conn, "fixture-transport", castMedia, fmt.Sprintf(`{"type":"MEDIA_STATUS","requestId":2,"status":[{"mediaSessionId":17,"playerState":%q,"supportedMediaCommands":%d}]}`, player, capabilities))
}

func TestCastControlMediaCommands(t *testing.T) {
	for _, tc := range []struct {
		action, initial, kind string
		capabilities, jump    int
	}{
		{"play", "PAUSED", "PLAY", 0, 0},
		{"pause", "PLAYING", "PAUSE", 1, 0},
		{"stop", "BUFFERING", "STOP", 0, 0},
		{"next", "PLAYING", "QUEUE_UPDATE", 64, 1},
		{"previous", "PAUSED", "QUEUE_UPDATE", 128, -1},
	} {
		t.Run(tc.action, func(t *testing.T) {
			result, e := castControlPipe(t, castControlRequest{Action: tc.action}, func(conn net.Conn) error {
				if e := fakeCastControlMedia(conn, tc.initial, tc.capabilities); e != nil {
					return e
				}
				p, e := fakeCastControlRead(conn, castMedia, "fixture-transport", tc.kind, 3)
				if e != nil {
					return e
				}
				if p["mediaSessionId"] != float64(17) || tc.jump != 0 && p["jump"] != float64(tc.jump) {
					return errors.New("wrong session or jump")
				}
				return writeFakeCastControl(conn, "fixture-transport", castMedia, `{"type":"MEDIA_STATUS","requestId":3,"status":[{"mediaSessionId":17,"playerState":"PAUSED"}]}`)
			})
			if e != nil || result.PlayerState != "PAUSED" {
				t.Fatal(result, e)
			}
		})
	}
}

func TestCastControlRejectsUnrelatedOrIncompleteMediaReplies(t *testing.T) {
	for _, tc := range []struct {
		name, action, fields string
		wantErr              bool
	}{
		{"matching", "pause", `,"status":[{"mediaSessionId":17,"playerState":"PAUSED"}]`, false},
		{"different session", "pause", `,"status":[{"mediaSessionId":18,"playerState":"PAUSED"}]`, true},
		{"missing session", "pause", `,"status":[{"playerState":"PAUSED"}]`, true},
		{"multiple sessions", "pause", `,"status":[{"mediaSessionId":17,"playerState":"PAUSED"},{"mediaSessionId":18,"playerState":"PLAYING"}]`, true},
		{"missing playback", "pause", `,"status":[{"mediaSessionId":17}]`, true},
		{"unknown playback", "pause", `,"status":[{"mediaSessionId":17,"playerState":"UNKNOWN"}]`, true},
		{"empty pause", "pause", `,"status":[]`, true},
		{"empty stop", "stop", `,"status":[]`, false},
		{"missing stop status", "stop", ``, true},
		{"null stop status", "stop", `,"status":null`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, e := castControlPipe(t, castControlRequest{Action: tc.action}, func(conn net.Conn) error {
				if e := fakeCastControlMedia(conn, "PLAYING", 1); e != nil {
					return e
				}
				if _, e := fakeCastControlRead(conn, castMedia, "fixture-transport", strings.ToUpper(tc.action), 3); e != nil {
					return e
				}
				return writeFakeCastControl(conn, "fixture-transport", castMedia, `{"type":"MEDIA_STATUS","requestId":3`+tc.fields+`}`)
			})
			if (e != nil) != tc.wantErr {
				t.Fatalf("reply accepted=%v, want error=%v: %v", e == nil, tc.wantErr, e)
			}
		})
	}
}

func TestCastControlRejectsUnsupportedMediaBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		action, state string
		capabilities  int
	}{
		{"pause", "PLAYING", 0}, {"pause", "PAUSED", 1},
		{"play", "PLAYING", 1}, {"stop", "IDLE", 0},
		{"next", "PLAYING", 128}, {"previous", "PLAYING", 64},
	} {
		t.Run(tc.action+tc.state+fmt.Sprint(tc.capabilities), func(t *testing.T) {
			_, e := castControlPipe(t, castControlRequest{Action: tc.action}, func(conn net.Conn) error {
				return fakeCastControlMedia(conn, tc.state, tc.capabilities)
			})
			if e == nil {
				t.Fatal("unsupported command accepted")
			}
		})
	}
	for _, fixture := range []string{
		`{"type":"RECEIVER_STATUS","requestId":1,"status":{}}`,
		strings.Replace(castControlReceiverFixture, `"appId":"fixture"`, `"appId":"85CDB22F"`, 1),
		strings.Replace(castControlReceiverFixture, castMedia, "urn:other", 1),
	} {
		_, e := castControlPipe(t, castControlRequest{Action: "play"}, func(conn net.Conn) error {
			return fakeCastControlReceiver(conn, fixture)
		})
		if e == nil {
			t.Fatal("uncontrollable receiver accepted")
		}
	}
}

func TestCastControlRequestValidation(t *testing.T) {
	muted := false
	for _, level := range []float64{-1, 1.1, math.NaN(), math.Inf(1)} {
		if e := (castControlRequest{Action: "volume", Level: &level}).validate(); e == nil {
			t.Fatal("invalid volume accepted", level)
		}
	}
	for _, request := range []castControlRequest{
		{Action: "LAUNCH"}, {Action: "volume"}, {Action: "mute"},
		{Action: "pause", Muted: &muted}, {Action: "volume", Muted: &muted},
	} {
		if e := request.validate(); e == nil {
			t.Fatal("invalid request accepted", request)
		}
	}
}

func TestCastControlCorrelationHeartbeatAndRejection(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	_ = server.SetDeadline(time.Now().Add(time.Second))
	done := make(chan error, 1)
	go func() {
		// A reply intended for the background monitor must not acknowledge
		// a control request, even when every correlation field matches.
		if e := writeFakeCast(server, "fixture-transport", castMedia, `{"type":"MEDIA_STATUS","requestId":3,"status":[{"playerState":"PLAYING"}]}`); e != nil {
			done <- e
			return
		}
		for _, message := range []struct{ source, namespace, payload string }{
			{"receiver-0", castHeartbeat, `{"type":"PING"}`},
			{"other", castMedia, `{"type":"MEDIA_STATUS","requestId":3}`},
			{"fixture-transport", castMedia, `{"type":"MEDIA_STATUS"}`},
			{"fixture-transport", castMedia, `{"type":"MEDIA_STATUS","requestId":0}`},
			{"fixture-transport", castReceiver, `{"type":"RECEIVER_STATUS","requestId":3}`},
			{"fixture-transport", castMedia, `{"type":"INVALID_REQUEST","requestId":3,"reason":"private data must not escape"}`},
		} {
			if e := writeFakeCastControl(server, message.source, message.namespace, message.payload); e != nil {
				done <- e
				return
			}
			if message.namespace == castHeartbeat {
				if _, e := fakeCastControlRead(server, castHeartbeat, "receiver-0", "PONG", 0); e != nil {
					done <- e
					return
				}
			}
		}
		done <- nil
	}()
	_, e := awaitCastControl(client, castMedia, "fixture-transport", "MEDIA_STATUS", 3)
	if e == nil || !strings.Contains(e.Error(), "INVALID_REQUEST") || strings.Contains(e.Error(), "private data") {
		t.Fatal("rejection must be correlated and bounded", e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}

func castControlTestCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestCastControlTLSAndCancellation(t *testing.T) {
	certificate := castControlTestCertificate(t)
	for _, stall := range []bool{false, true} {
		t.Run(fmt.Sprint("stall=", stall), func(t *testing.T) {
			client, rawServer := net.Pipe()
			defer rawServer.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				server := tls.Server(rawServer, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
				defer rawServer.Close()
				if e := server.Handshake(); e != nil {
					done <- e
					return
				}
				if stall {
					if _, e := fakeCastControlRead(server, castConnection, "receiver-0", "CONNECT", 0); e != nil {
						done <- e
						return
					}
					if _, e := fakeCastControlRead(server, castReceiver, "receiver-0", "GET_STATUS", 1); e != nil {
						done <- e
						return
					}
					cancel()
					var b [1]byte
					_, e := server.Read(b[:])
					if e == nil {
						done <- errors.New("client connection not closed after cancellation")
					} else {
						done <- nil
					}
					return
				}
				if e := fakeCastControlReceiver(server, strings.Replace(castControlReceiverFixture, `"muted":false`, `"muted":true`, 1)); e != nil {
					done <- e
					return
				}
				if _, e := fakeCastControlRead(server, castReceiver, "receiver-0", "SET_VOLUME", 2); e != nil {
					done <- e
					return
				}
				done <- writeFakeCastControl(server, "receiver-0", castReceiver, `{"type":"RECEIVER_STATUS","requestId":2,"status":{"volume":{"muted":false}}}`)
			}()
			muted := false
			started := time.Now()
			result, e := castControlTLS(ctx, client, castControlRequest{Action: "mute", Muted: &muted})
			if stall && e == nil || !stall && (e != nil || result.Muted == nil || *result.Muted) {
				t.Fatal(result, e)
			}
			if time.Since(started) > time.Second {
				t.Fatal("cancellation did not close bounded TLS request")
			}
			if e := <-done; e != nil {
				t.Fatal(e)
			}
		})
	}
}
