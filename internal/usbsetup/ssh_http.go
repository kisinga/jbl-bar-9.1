package usbsetup

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type sshTicket struct {
	Token, Host, Fingerprint, Package string
	Expires                           time.Time
}

// Called under the server's operation lock, shared with downloads and USB writes.
func (s *Server) serveSSH(w http.ResponseWriter, r *http.Request) {
	if s.RestartRequired {
		failure(w, errors.New("restart the updated setup app before updating the bar"))
		return
	}
	var req SSHRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF {
		failure(w, errors.New("invalid network update request"))
		return
	}
	req.Host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(req.Host), "."))
	if req.Host == "" {
		req.Host = "jbl.local"
	}
	if s.PackagePath == "" || s.RecoveryPath == "" {
		failure(w, errors.New("load the toolkit and matching recovery files first"))
		return
	}
	p, err := LoadPackage(s.PackagePath)
	if err != nil {
		failure(w, err)
		return
	}
	if err = VerifyRecovery(s.RecoveryPath, p); err != nil {
		failure(w, err)
		return
	}
	if len(p.Files["JBLToolkit/ssh-update.sh"]) == 0 {
		failure(w, errors.New("this package predates network updates; load a newer toolkit build"))
		return
	}
	if r.URL.Path == "/api/ssh/check" {
		s.sshTicket = nil
		result, err := s.SSH.Check(r.Context(), p, req)
		if err != nil {
			failure(w, err)
			return
		}
		if result.Success && !result.Current {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				failure(w, err)
				return
			}
			result.Ticket = hex.EncodeToString(b)
			s.sshTicket = &sshTicket{Token: result.Ticket, Host: req.Host, Fingerprint: result.Fingerprint, Package: p.Digest, Expires: time.Now().Add(10 * time.Minute)}
		}
		respond(w, result)
		return
	}
	ticket := s.sshTicket
	if ticket == nil || req.Ticket != ticket.Token || req.Host != ticket.Host || p.Digest != ticket.Package || !time.Now().Before(ticket.Expires) {
		failure(w, errors.New("check the bar’s connection again before updating"))
		return
	}
	req.Fingerprint = ticket.Fingerprint
	s.sshTicket = nil
	// One streaming request gives progress without a second connection or job poll.
	w.Header().Set("Content-Type", "application/x-ndjson")
	encoder := json.NewEncoder(w)
	send := func(value any) {
		encoder.Encode(value)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	result, err := s.SSH.Update(r.Context(), p, req, func(message string) { send(map[string]string{"progress": message}) })
	if err != nil {
		send(map[string]any{"done": true, "success": false, "message": err.Error()})
		return
	}
	send(struct {
		SSHResult
		Done bool `json:"done"`
	}{result, true})
}
