// Package display defines the host-testable notification contract. No native
// adapter or deployable entry mechanism is qualified; production stays disabled.
package display

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

const UnavailableReason = "native display entry mechanism is not qualified"
const MaxRequestBytes = 256

var correlation = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
var text = regexp.MustCompile(`^[A-Z0-9 ]{1,31}$`)
var events = map[string]string{
	"install_start": "INSTALLING", "ssh_ready": "SSH READY", "remote_ready": "REMOTE READY",
	"standard_ready": "STANDARD", "install_pending_reboot": "REBOOT", "action_done": "DONE", "action_failed": "FAILED",
}

type Request struct {
	Version int    `json:"v"`
	ID      string `json:"id"`
	Text    string `json:"text"`
}

type Outcome string

const (
	Submitted   Outcome = "submitted"
	Consumed    Outcome = "consumed"
	SkippedBusy Outcome = "skipped_busy"
	Unavailable Outcome = "unavailable"
	Invalid     Outcome = "invalid"
	Timeout     Outcome = "timeout"
)

type Reply struct {
	ID      string  `json:"id"`
	Outcome Outcome `json:"outcome"`
	PID     int     `json:"pid,omitempty"`
	Start   string  `json:"start,omitempty"`
}

func Event(id, event string) (Request, error) {
	value, ok := events[event]
	if !ok {
		return Request{}, errors.New("unknown display event")
	}
	r := Request{Version: 1, ID: id, Text: value}
	return r, r.Validate()
}
func (r Request) Validate() error {
	if r.Version != 1 || !correlation.MatchString(r.ID) || !text.MatchString(r.Text) || strings.TrimSpace(r.Text) == "" {
		return errors.New("invalid display request")
	}
	return nil
}
func Decode(data []byte) (Request, error) {
	var r Request
	if len(data) > MaxRequestBytes {
		return r, errors.New("display request exceeds 256 bytes")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&r); err != nil {
		return r, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return r, errors.New("unexpected display request data")
	}
	return r, r.Validate()
}

// Match checks correlation and native-process identity. Neither submitted nor
// consumed claims that a human observed the display. A timeout must not retry.
func (r Reply) Match(request Request, pid int, start string) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if r.ID != request.ID {
		return errors.New("display reply ID mismatch")
	}
	switch r.Outcome {
	case Submitted, Consumed:
		if pid <= 0 || start == "" || r.PID != pid || r.Start != start {
			return errors.New("display native process changed or unknown")
		}
	case SkippedBusy, Unavailable, Invalid, Timeout:
	default:
		return errors.New("unknown display outcome")
	}
	return nil
}
