package usbsetup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/dns/dnsmessage"
)

// Real SSH handshakes/channels against a loopback fixture; no native device calls.
func sshFixture(t *testing.T, handle func(string, []byte) (string, uint32, bool)) (*SSHUpdater, string, *atomic.Int32) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	auth := new(atomic.Int32)
	config := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		auth.Add(1)
		if meta.User() != "root" || string(password) != "fixture-login-password" {
			return nil, errors.New("bad password")
		}
		return nil, nil
	}}
	config.AddHostKey(key)
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() { l.Close(); wg.Wait() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					conn.Close()
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for ch := range channels {
					if ch.ChannelType() != "session" {
						ch.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					channel, requests, err := ch.Accept()
					if err != nil {
						continue
					}
					for req := range requests {
						if req.Type != "exec" {
							req.Reply(false, nil)
							continue
						}
						var payload struct{ Command string }
						ssh.Unmarshal(req.Payload, &payload)
						req.Reply(true, nil)
						input, _ := io.ReadAll(channel)
						out, code, drop := handle(payload.Command, input)
						if drop {
							server.Close()
							break
						}
						io.WriteString(channel, out)
						channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{code}))
						channel.Close()
						break
					}
				}
			}()
		}
	}()
	u := &SSHUpdater{KnownHosts: filepath.Join(t.TempDir(), "known_hosts"), Resolve: func(context.Context, string) (string, error) { return "192.168.1.42", nil }, Poll: time.Millisecond}
	u.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "192.168.1.42:2222" {
			return nil, errors.New("wrong destination")
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp4", l.Addr().String())
	}
	return u, ssh.FingerprintSHA256(key.PublicKey()), auth
}

func sshPackage(t *testing.T) *Package {
	t.Helper()
	p, err := LoadPackage(fixtureZip(t, fixtureFiles()))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSSHTrustBeforeAuthenticationAndChangedIdentity(t *testing.T) {
	p := sshPackage(t)
	u, fingerprint, auth := sshFixture(t, func(cmd string, _ []byte) (string, uint32, bool) {
		if !strings.Contains(cmd, "MANAGED_TOOLKIT_REQUIRED") {
			t.Error("check mutated device", cmd)
		}
		return "READY\n" + strings.Repeat("a", 64) + "  /var/JBLToolkit/package.manifest\n", 0, false
	})
	req := SSHRequest{Host: "jbl.local", Password: "fixture-login-password"}
	result, err := u.Check(context.Background(), p, req)
	if err != nil || !result.NeedsTrust || result.Fingerprint != fingerprint || auth.Load() != 0 {
		t.Fatal(result, err, auth.Load())
	}
	if _, err := os.Stat(u.KnownHosts); !os.IsNotExist(err) {
		t.Fatal("unconfirmed identity saved")
	}
	req.Fingerprint = fingerprint
	result, err = u.Check(context.Background(), p, req)
	if err != nil || !result.Success || result.Current || auth.Load() != 1 {
		t.Fatal(result, err, auth.Load())
	}
	data, _ := os.ReadFile(u.KnownHosts)
	if !bytes.Contains(data, []byte("[jbl.local]:2222")) || bytes.Contains(data, []byte(req.Password)) {
		t.Fatal("invalid host key storage")
	}
	info, _ := os.Stat(u.KnownHosts)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("host key permissions")
	}
	u2, other, auth2 := sshFixture(t, func(string, []byte) (string, uint32, bool) {
		t.Error("changed key reached commands")
		return "", 1, false
	})
	u2.KnownHosts = u.KnownHosts
	req.Fingerprint = other // A supplied fingerprint cannot override a saved mismatch.
	if _, err = u2.Check(context.Background(), p, req); err == nil || auth2.Load() != 0 {
		t.Fatal("changed key accepted", err)
	}
}

func TestSSHWrongPasswordDoesNotPinIdentity(t *testing.T) {
	p := sshPackage(t)
	u, key, _ := sshFixture(t, func(string, []byte) (string, uint32, bool) { t.Error("unauthenticated command"); return "", 1, false })
	_, err := u.Check(context.Background(), p, SSHRequest{Host: "jbl.local", Password: "wrong", Fingerprint: key})
	if err == nil {
		t.Fatal("wrong password accepted")
	}
	if _, err = os.Stat(u.KnownHosts); !os.IsNotExist(err) {
		t.Fatal("failed login pinned key")
	}
}

func TestSSHConnectionFailureExplainsCause(t *testing.T) {
	u := &SSHUpdater{
		Resolve: func(context.Context, string) (string, error) { return "192.168.1.42", nil },
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("connect: connection refused")
		},
	}
	_, _, err := u.connect(context.Background(), SSHRequest{Host: "jbl.local", Password: "fixture-login-password"})
	if err == nil || !strings.Contains(err.Error(), "192.168.1.42:2222") || !strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "fixture-login-password") {
		t.Fatal(err)
	}
}

func TestSSHUpdateTransferReconnectReceiptsAndFailure(t *testing.T) {
	for _, mode := range []string{"success", "transfer_failure", "bad_checksum", "device_failure", "ssh_failure", "bad_receipt", "unowned", "already_current"} {
		t.Run(mode, func(t *testing.T) {
			p := sshPackage(t)
			var mu sync.Mutex
			updated, launched, dropped, cleaned, uploaded := false, false, false, false, false
			u, key, auth := sshFixture(t, func(cmd string, input []byte) (string, uint32, bool) {
				mu.Lock()
				defer mu.Unlock()
				if strings.Contains(cmd, "fixture-login-password") {
					t.Error("password in shell command")
				}
				switch {
				case strings.Contains(cmd, "MANAGED_TOOLKIT_REQUIRED"):
					if mode == "unowned" {
						return "MANAGED_TOOLKIT_REQUIRED_USE_USB", 1, false
					}
					digest := strings.Repeat("a", 64)
					if updated || mode == "already_current" {
						digest = manifestDigest(p)
					}
					return "READY\n" + digest + "  /var/JBLToolkit/package.manifest\n", 0, false
				case strings.Contains(cmd, "df -Pk /tmp"):
					if mode == "transfer_failure" {
						return "", 1, false
					}
					return "", 0, false
				case strings.Contains(cmd, "tar -xf"):
					uploaded = true
					r := tar.NewReader(bytes.NewReader(input))
					seen := map[string]bool{}
					for {
						h, err := r.Next()
						if err == io.EOF {
							break
						}
						if err != nil {
							t.Error(err)
							break
						}
						if !strings.HasPrefix(h.Name, "JBLToolkit/") {
							t.Error("unsafe transfer path", h.Name)
						}
						if h.Typeflag == tar.TypeDir {
							if h.Mode != 0700 {
								t.Error("public staging directory")
							}
							continue
						}
						data, _ := io.ReadAll(r)
						seen[h.Name] = true
						if h.Name == configPath {
							if !bytes.Contains(data, []byte("action=update\n")) {
								t.Error("not update-only")
							}
						} else if !bytes.Equal(data, p.Files[h.Name]) {
							t.Error("changed transfer", h.Name)
						}
					}
					if !seen["JBLToolkit/files/remote"] || !seen["JBLToolkit/ssh-update.sh"] {
						t.Error("incomplete transfer")
					}
					return "", 0, false
				case strings.Contains(cmd, "sha256sum -c SHA256SUMS"):
					if mode == "bad_checksum" {
						return "", 1, false
					}
					return "", 0, false
				case strings.Contains(cmd, "setsid"):
					launched = true
					updated = mode != "device_failure" && mode != "ssh_failure"
					return "", 0, false
				case strings.Contains(cmd, "SSH-EXIT.txt"):
					if mode == "bad_receipt" {
						return "invalid", 0, false
					}
					if !dropped && mode == "success" {
						dropped = true
						return "", 0, true
					}
					if mode == "device_failure" || mode == "ssh_failure" {
						return "1\n", 0, false
					}
					return "0\n", 0, false
				case strings.Contains(cmd, "/ssh-input/STATUS.txt"):
					return "SSH_LISTENER_NOT_CONFIRMED\n", 0, false
				case strings.Contains(cmd, "/ssh-input/ssh-start.log"):
					return "mount: mounting /var/JBLLocalSSH/passwd on /etc/passwd failed: No such file or directory\n", 0, false
				case strings.Contains(cmd, "STATUS.txt"):
					if mode == "ssh_failure" {
						return "SSH_INSTALL_FAILED\n", 0, false
					}
					if mode == "device_failure" {
						return "PREFLIGHT_FAILED\n", 0, false
					}
					return "INSTALLED_PENDING_REBOOT_TEST\n", 0, false
				case strings.Contains(cmd, "tail -c"):
					return "fixture report", 0, false
				case strings.Contains(cmd, "rm -rf"):
					cleaned = true
					return "", 0, false
				default:
					t.Error("unexpected command", cmd)
					return "", 1, false
				}
			})
			result, err := u.Update(context.Background(), p, SSHRequest{Host: "jbl.local", Password: "fixture-login-password", Fingerprint: key}, nil)
			mu.Lock()
			defer mu.Unlock()
			switch mode {
			case "success":
				if err != nil || !result.Success || !launched || !uploaded || !cleaned || !dropped || auth.Load() != 2 {
					t.Fatal(result, err, launched, cleaned, auth.Load())
				}
			case "transfer_failure", "unowned":
				if err == nil || launched || uploaded || cleaned {
					t.Fatal(result, err, launched, uploaded, cleaned)
				}
			case "bad_checksum":
				if err == nil || launched || !cleaned {
					t.Fatal(result, err, launched, cleaned)
				}
			case "device_failure":
				if err != nil || result.Success || cleaned || !strings.Contains(result.Message, "PREFLIGHT_FAILED") {
					t.Fatal(result, err, cleaned)
				}
			case "bad_receipt":
				if err != nil || result.Success || cleaned || !strings.Contains(result.Message, "could not be confirmed") {
					t.Fatal(result, err, cleaned)
				}
			case "ssh_failure":
				if err != nil || result.Success || cleaned || !strings.Contains(result.Message, "SSH_LISTENER_NOT_CONFIRMED") || !strings.Contains(result.Message, "mount: mounting /var/JBLLocalSSH/passwd on /etc/passwd failed: No such file or directory") || !strings.Contains(result.Message, "Reports retained at /tmp/jbl-toolkit-ssh.") {
					t.Fatal(result, err, cleaned)
				}
			case "already_current":
				if err != nil || !result.Current || !result.Success || launched || uploaded || cleaned {
					t.Fatal(result, err)
				}
			}
		})
	}
}

func TestSSHHTTPRequiresCheckedPackageAndHost(t *testing.T) {
	s, _ := testServer(t)
	p := sshPackage(t)
	recovery := map[string][]byte{"JBLToolkit/RECOVER": []byte("RECOVER\n")}
	for _, name := range []string{entry, "JBLToolkit/launch.sh", "JBLToolkit/common.sh", "JBLToolkit/recover.sh", "JBLToolkit/dispatch.sh", "JBLToolkit/BOOTSTRAP.SHA256SUMS", "README.txt", "LICENSE.txt"} {
		recovery[name] = p.Files[name]
	}
	s.RecoveryPath = fixtureZip(t, recovery)
	u, key, _ := sshFixture(t, func(string, []byte) (string, uint32, bool) {
		return "READY\n" + strings.Repeat("a", 64) + "  /var/JBLToolkit/package.manifest\n", 0, false
	})
	s.SSH = u
	req := SSHRequest{Host: "jbl.local", Password: "fixture-login-password", Fingerprint: key}
	if w := call(s, "POST", "/api/ssh/check", req, false); w.Code != 403 {
		t.Fatal("SSH CSRF missing")
	}
	if w := call(s, "POST", "/api/ssh/update", req, true); w.Code != 400 {
		t.Fatal("update without connection check")
	}
	w := call(s, "POST", "/api/ssh/check", req, true)
	if w.Code != 200 || s.sshTicket == nil {
		t.Fatal(w.Body.String())
	}
	req.Ticket = s.sshTicket.Token
	req.Host = "192.168.1.10"
	if w = call(s, "POST", "/api/ssh/update", req, true); w.Code != 400 {
		t.Fatal("ticket followed a different bar")
	}
	req.Host = "jbl.local"
	s.sshTicket.Package = "changed-package"
	if w = call(s, "POST", "/api/ssh/update", req, true); w.Code != 400 {
		t.Fatal("ticket followed another package")
	}
	if strings.Contains(w.Body.String(), req.Password) {
		t.Fatal("credential reflected")
	}
}

func TestMDNSAnswerAndPrivateTargets(t *testing.T) {
	name := dnsmessage.MustNewName("jbl.local.")
	for _, tc := range []struct {
		host string
		ip   [4]byte
		want string
	}{
		{"jbl.local", [4]byte{192, 168, 1, 50}, "192.168.1.50"},
		{"other.local", [4]byte{192, 168, 1, 50}, ""},
		{"jbl.local", [4]byte{8, 8, 8, 8}, ""},
		{"jbl.local", [4]byte{127, 0, 0, 1}, ""},
	} {
		data, err := (&dnsmessage.Message{Header: dnsmessage.Header{Response: true}, Answers: []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 120}, Body: &dnsmessage.AResource{A: tc.ip}}}}).Pack()
		if err != nil || mdnsAnswer(data, tc.host) != tc.want {
			t.Fatal(tc, err)
		}
	}
	if mdnsAnswer([]byte("corrupt"), "jbl.local") != "" {
		t.Fatal("malformed DNS accepted")
	}
	for _, host := range []string{"127.0.0.1", "8.8.8.8", "169.254.169.254", "http://192.168.1.2", "jbl.local;id", "192.168.1.2:22", "::1"} {
		if ip, err := resolveBar(context.Background(), host); err == nil {
			t.Fatal("unsafe target accepted", host, ip)
		}
	}
	if ip, err := resolveBar(context.Background(), "192.168.1.50"); err != nil || ip != "192.168.1.50" {
		t.Fatal(ip, err)
	}
}

func TestTailBufferBounds(t *testing.T) {
	var b tailBuffer
	fmt.Fprint(&b, strings.Repeat("a", 65000))
	fmt.Fprint(&b, strings.Repeat("b", 1000))
	if len(b.bytes) != 65536 || b.bytes[len(b.bytes)-1] != 'b' {
		t.Fatal("unbounded report")
	}
}
