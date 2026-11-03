package jblone

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"jbl-bar-toolkit/remote/internal/preview"
	"jbl-bar-toolkit/remote/internal/server"
)

// Synthetic wire fixtures based on the pinned community protocol shapes;
// these are deliberately not represented as physical Bar 1300 captures.
func envelope(service, action, fields string) string {
	return `<s:Envelope xmlns:s="` + soapNamespace + `"><s:Body><u:` + action + `Response xmlns:u="urn:schemas-upnp-org:service:` + service + `:1">` + fields + `</u:` + action + `Response></s:Body></s:Envelope>`
}

func fixtureClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient("192.168.9.10")
	if err != nil {
		t.Fatal(err)
	}
	// Only tests replace the private-IP origin with a loopback fixture server.
	client.origin = server.URL
	return client
}

func TestInspectFixedGettersAndConservativeFixture(t *testing.T) {
	var actions []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("SOAPAction")
		actions = append(actions, action)
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || !strings.Contains(string(body), "<InstanceID>0</InstanceID>") {
			t.Error("invalid getter request", r.Method, string(body))
		}
		switch r.URL.Path {
		case "/upnp/control/rendercontrol1":
			if action != `"urn:schemas-upnp-org:service:RenderingControl:1#GetControlDeviceInfo"` {
				t.Error("unexpected metadata action", action)
			}
			fmt.Fprint(w, envelope("RenderingControl", "GetControlDeviceInfo", `<Status>{"hm_product_name":"Fixture bar","hardware":"fixture-hw","firmware":"fixture-version","serial_number":"private-fixture"}</Status>`))
		case "/upnp/control/rendertransport1":
			if action != `"urn:schemas-upnp-org:service:AVTransport:1#GetInfoEx"` {
				t.Error("unexpected state action", action)
			}
			fmt.Fprint(w, envelope("AVTransport", "GetInfoEx", `<CurrentVolume>19</CurrentVolume><CurrentMute>0</CurrentMute><PlayMedium>unmapped-vendor-source</PlayMedium><CurrentTransportState>PLAYING</CurrentTransportState>`))
		default:
			t.Error("unexpected endpoint", r.URL.Path)
		}
	})
	report := client.Inspect(context.Background())
	if len(actions) != 2 || len(report.Observations) != 2 || report.Observations[0].Error != "" || report.Observations[1].Error != "" {
		t.Fatalf("fixed getters did not succeed: %+v", report)
	}
	if !reflect.DeepEqual(report.Identity, map[string]string{"hm_product_name": "Fixture bar", "hardware": "fixture-hw", "firmware": "fixture-version"}) {
		t.Fatal("identity was inferred or private fields included", report.Identity)
	}
	fixture := report.Fixture()
	if fixture["volume"] != uint64(19) || fixture["muted"] != false || fixture["available"] != true {
		t.Fatal("direct observations lost", fixture)
	}
	for _, key := range []string{"power", "mode", "input"} {
		if fixture[key] != "Unknown" {
			t.Fatal("unverified semantics invented", fixture)
		}
	}
	if _, exists := fixture["source_state"]; exists {
		t.Fatal("vendor source identifier leaked into shared state")
	}
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := preview.New(strings.NewReader(string(encoded)))
	if err != nil {
		t.Fatal("collector fixture incompatible with shared preview", err)
	}
	if snapshot := backend.Snapshot(); snapshot["volume"] != float64(19) || snapshot["power"] != "Unknown" {
		t.Fatal("preview changed observed or unknown state", snapshot)
	}
	if len(backend.Features().Actions) != 0 {
		t.Fatal("research capture became advertised controls")
	}
	if _, err := backend.Execute(context.Background(), server.Command{Action: "volume_up"}); err == nil {
		t.Fatal("research capture permitted mutation")
	}
}

func TestMissingIdentityDoesNotDiscardState(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "rendercontrol") {
			fmt.Fprint(w, envelope("RenderingControl", "GetControlDeviceInfo", `<Status>[]</Status>`))
		} else {
			fmt.Fprint(w, envelope("AVTransport", "GetInfoEx", `<CurrentMute>1</CurrentMute>`))
		}
	})
	report := client.Inspect(context.Background())
	if report.Observations[0].Error == "" || report.Observations[1].Error != "" || len(report.Identity) != 0 {
		t.Fatal("identity failure contaminated state", report)
	}
	if s := report.Fixture(); s["muted"] != true || s["volume"] != nil || s["available"] != true {
		t.Fatal("partial observed state lost or fabricated", s)
	}
}

func TestInspectFailuresStayIndependent(t *testing.T) {
	cases := []struct {
		name, state string
		status      int
	}{
		{"soap-fault", `<s:Envelope xmlns:s="` + soapNamespace + `"><s:Body><s:Fault><faultcode>s:Client</faultcode></s:Fault></s:Body></s:Envelope>`, 200},
		{"wrong-response", envelope("RenderingControl", "SetVolume", `<CurrentVolume>99</CurrentVolume>`), 200},
		{"malformed", `<broken>`, 200},
		{"invalid-utf8", string([]byte{0xff, 0xfe}), 200},
		{"too-large", strings.Repeat("x", maxResponse+1), 200},
		{"http-error", envelope("AVTransport", "GetInfoEx", `<CurrentVolume>99</CurrentVolume>`), 500},
		{"duplicate-field", envelope("AVTransport", "GetInfoEx", `<CurrentVolume>1</CurrentVolume><CurrentVolume>99</CurrentVolume>`), 200},
		{"trailing-document", envelope("AVTransport", "GetInfoEx", `<CurrentVolume>99</CurrentVolume>`) + `<extra/>`, 200},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if strings.Contains(r.URL.Path, "rendercontrol") {
					fmt.Fprint(w, envelope("RenderingControl", "GetControlDeviceInfo", `<Status>{"hm_product_name":"Fixture"}</Status>`))
					return
				}
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.state)
			})
			report := client.Inspect(context.Background())
			if requests != 2 || report.Identity["hm_product_name"] != "Fixture" || report.Observations[1].Error == "" {
				t.Fatal("partial failure lost or retried", report)
			}
			if s := report.Fixture(); s["volume"] != nil || s["muted"] != nil || s["available"] != false {
				t.Fatal("failed reply became valid state", s)
			}
			if len(report.Observations[1].Response) > maxResponse {
				t.Fatal("response capture was not bounded")
			}
			if test.name == "invalid-utf8" {
				data, err := base64.StdEncoding.DecodeString(report.Observations[1].ResponseBase64)
				if err != nil || string(data) != test.state {
					t.Fatal("invalid response bytes were altered", report.Observations[1])
				}
			}
		})
	}
}

func TestInspectRedirectsNeverContactAnotherTarget(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) })
	report := client.Inspect(context.Background())
	if redirected || report.Observations[0].HTTPStatus != 307 || report.Observations[1].Error == "" {
		t.Fatal("redirect followed or hidden", report)
	}
}

func TestInspectCancellationAndUnknownValues(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("canceled request reached server") })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	report := client.Inspect(ctx)
	if time.Since(started) > time.Second || report.Observations[0].Error == "" || report.Observations[1].Error == "" {
		t.Fatal("cancellation was not bounded", report)
	}
	for _, fields := range []map[string]string{nil, {"CurrentVolume": "-1", "CurrentMute": "unknown"}, {"CurrentVolume": "NaN", "CurrentMute": "2"}} {
		r := Report{Observations: []Observation{{Action: "GetInfoEx", Fields: fields}}}
		s := r.Fixture()
		if s["available"] != false || s["volume"] != nil || s["muted"] != nil {
			t.Fatal("missing/invalid values became defaults", s)
		}
	}
}

func TestInspectRejectsNonlocalAndAmbiguousTargets(t *testing.T) {
	for _, host := range []string{"", "jbl.local", "8.8.8.8", "127.0.0.1", "169.254.1.1", "192.168.1.3:59152", "http://192.168.1.3", "192.168.001.3", "::ffff:192.168.1.3"} {
		if _, err := NewClient(host); err == nil {
			t.Error("accepted ambiguous/non-private target", host)
		}
	}
	for _, host := range []string{"10.0.0.2", "172.16.0.2", "192.168.1.3"} {
		if _, err := NewClient(host); err != nil {
			t.Error("rejected private IPv4", host, err)
		}
	}
}
