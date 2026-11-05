package display

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNamedEventsAndBoundedRequests(t *testing.T) {
	for _, event := range []string{"install_start", "ssh_ready", "remote_ready", "standard_ready", "install_pending_reboot", "action_done", "action_failed"} {
		request, err := Event("install-4", event)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Decode(data)
		if err != nil || got != request {
			t.Fatal(got, err)
		}
	}
	for _, data := range []string{
		`{"v":1,"id":"x","text":"DONE","address":6450544}`,
		`{"v":2,"id":"x","text":"DONE"}`, `{"v":1,"id":"","text":"DONE"}`,
		`{"v":1,"id":"x","text":""}`, `{"v":1,"id":"x","text":"   "}`,
		`{"v":1,"id":"x","text":"Loading..."}`, `{"v":1,"id":"x","text":"DONE\n"}`,
		`{"v":1,"id":"x","text":"DONE\u0000"}`, `{"v":1,"id":"x","text":"DONE"} {}`,
		strings.Repeat(" ", MaxRequestBytes+1),
	} {
		if _, err := Decode([]byte(data)); err == nil {
			t.Fatal("invalid request accepted", data)
		}
	}
	if _, err := Event("x", "upgrade_animation"); err == nil {
		t.Fatal("unqualified animation accepted")
	}
	if err := (Request{1, "x", strings.Repeat("A", 32)}).Validate(); err == nil {
		t.Fatal("oversize text accepted")
	}
}
func TestReplyOutcomesAndRestartIdentity(t *testing.T) {
	request, _ := Event("install-4", "remote_ready")
	for _, outcome := range []Outcome{Submitted, Consumed, SkippedBusy, Unavailable, Invalid, Timeout} {
		reply := Reply{request.ID, outcome, 123, "native-start"}
		if err := reply.Match(request, 123, "native-start"); err != nil {
			t.Fatal(err)
		}
		reply.ID = "old-request"
		if err := reply.Match(request, 123, "native-start"); err == nil {
			t.Fatal("stale reply accepted")
		}
	}
	for _, outcome := range []Outcome{Submitted, Consumed} {
		reply := Reply{request.ID, outcome, 123, "old-start"}
		if err := reply.Match(request, 123, "new-start"); err == nil {
			t.Fatal("native restart accepted")
		}
	}
	if err := (Reply{ID: request.ID, Outcome: "displayed"}).Match(request, 123, "start"); err == nil {
		t.Fatal("unproven displayed claim accepted")
	}
}
