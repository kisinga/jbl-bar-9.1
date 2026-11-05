package usbsetup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type SSHRequest struct {
	Host, Password, Fingerprint, Ticket string
}

type SSHResult struct {
	Message     string            `json:"message"`
	Fingerprint string            `json:"fingerprint,omitempty"`
	NeedsTrust  bool              `json:"needsTrust,omitempty"`
	Current     bool              `json:"current,omitempty"`
	Success     bool              `json:"success,omitempty"`
	Ticket      string            `json:"ticket,omitempty"`
	Reports     map[string]string `json:"reports,omitempty"`
}

// Passwords exist only in the current request, never files, command arguments or logs.
type SSHUpdater struct {
	KnownHosts string
	Resolve    func(context.Context, string) (string, error)
	Dial       func(context.Context, string, string) (net.Conn, error)
	Poll       time.Duration
}

type sshConnection struct {
	*ssh.Client
	stop func() bool
}

func (c *sshConnection) Close() { c.stop(); c.Client.Close() }

func (u *SSHUpdater) connect(ctx context.Context, req SSHRequest) (*sshConnection, SSHResult, error) {
	var result SSHResult
	if len(req.Password) == 0 || len(req.Password) > 256 || strings.ContainsRune(req.Password, 0) {
		return nil, result, errors.New("enter the SSH password chosen when enabling SSH on the bar")
	}
	ip, err := u.Resolve(ctx, req.Host)
	if err != nil {
		return nil, result, err
	}
	dialCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, err := u.Dial(dialCtx, "tcp4", net.JoinHostPort(ip, "2222"))
	if err != nil {
		return nil, result, fmt.Errorf("couldn’t reach SSH at %s:2222: %w; check that SSH is enabled and this computer is allowed", ip, err)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	callback := ssh.HostKeyCallback(func(string, net.Addr, ssh.PublicKey) error { return &knownhosts.KeyError{} })
	if _, err := os.Stat(u.KnownHosts); err == nil {
		callback, err = knownhosts.New(u.KnownHosts)
		if err != nil {
			stop()
			conn.Close()
			return nil, result, errors.New("couldn’t read saved speaker identities")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		stop()
		conn.Close()
		return nil, result, err
	}
	var save ssh.PublicKey
	var identityErr error
	config := &ssh.ClientConfig{User: "root", Auth: []ssh.AuthMethod{ssh.Password(req.Password)}, HostKeyCallback: func(host string, addr net.Addr, key ssh.PublicKey) error {
		result.Fingerprint = ssh.FingerprintSHA256(key)
		if req.Fingerprint != "" && req.Fingerprint != result.Fingerprint {
			identityErr = errors.New("the speaker’s identity changed since the connection check; verify its host key before updating")
			return identityErr
		}
		err := callback(host, addr, key)
		if err == nil {
			return nil
		}
		var unknown *knownhosts.KeyError
		if !errors.As(err, &unknown) || len(unknown.Want) > 0 {
			identityErr = errors.New("the bar’s SSH identity changed; verify its host key before updating; the saved identity has been preserved")
			return identityErr
		}
		if req.Fingerprint == result.Fingerprint {
			save = key
			return nil
		}
		result.NeedsTrust = true
		result.Message = "First connection: confirm this speaker’s identity to continue."
		return errors.New("speaker identity needs confirmation")
	}}
	clientConn, channels, requests, err := ssh.NewClientConn(conn, net.JoinHostPort(req.Host, "2222"), config)
	if err != nil {
		stop()
		conn.Close()
		if identityErr != nil {
			return nil, result, identityErr
		}
		if result.NeedsTrust {
			return nil, result, nil
		}
		return nil, result, errors.New("SSH login failed; check the password and that the bar allows this computer")
	}
	conn.SetDeadline(time.Time{})
	c := &sshConnection{Client: ssh.NewClient(clientConn, channels, requests), stop: stop}
	if save != nil {
		if err := u.saveHost(req.Host, save); err != nil {
			c.Close()
			return nil, result, err
		}
	}
	return c, result, nil
}

func (u *SSHUpdater) saveHost(host string, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(u.KnownHosts), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(u.KnownHosts, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = fmt.Fprintln(f, knownhosts.Line([]string{knownhosts.Normalize(net.JoinHostPort(host, "2222"))}, key)); err != nil {
		return err
	}
	return f.Sync()
}

type tailBuffer struct{ bytes []byte }

func (b *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= 65536 {
		b.bytes = append(b.bytes[:0], p[len(p)-65536:]...)
	} else {
		if len(b.bytes)+len(p) > 65536 {
			b.bytes = b.bytes[len(b.bytes)+len(p)-65536:]
		}
		b.bytes = append(b.bytes, p...)
	}
	return n, nil
}

func sshRun(c *sshConnection, command string, input io.Reader) (string, error) {
	session, err := c.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	var out, stderr tailBuffer
	session.Stdout, session.Stderr, session.Stdin = &out, &stderr, input
	err = session.Run("PATH=/bin:/sbin:/usr/bin:/usr/sbin; export PATH; unset LD_LIBRARY_PATH LD_PRELOAD; umask 077;\n" + command)
	return string(out.bytes), err
}

// Fixed commands only: credentials and user-supplied addresses never reach a shell.
const sshInspect = `PATH=/bin:/sbin:/usr/bin:/usr/sbin; export PATH
unset LD_LIBRARY_PATH LD_PRELOAD
BB=/bin/busybox
fail() { printf '%s\n' "$1"; exit 1; }
[ "$("$BB" id -u)" = 0 ] || fail ROOT_REQUIRED
STORE=/var/JBLToolkit
for dir in "$STORE" /var/JBLLocalSSH /var/JBLRemote; do
  [ ! -L "$dir" ] || fail MANAGED_STORE_UNSAFE
  [ ! -d "$dir" ] || [ -z "$("$BB" find "$dir" -type l)" ] || fail MANAGED_STORE_UNSAFE
done
[ -d "$STORE" ] && [ -f "$STORE/OWNER.txt" ] && [ "$("$BB" cat "$STORE/OWNER.txt")" = JBL_TOOLKIT_REV1 ] || fail MANAGED_TOOLKIT_REQUIRED_USE_USB
[ ! -e "$STORE/DISABLED" ] || fail TOOLKIT_DISABLED_USE_USB_RECOVERY
[ -f "$STORE/runtime.manifest" ] && "$BB" sha256sum -c "$STORE/runtime.manifest" >/dev/null 2>&1 || fail INSTALLED_RUNTIME_MODIFIED_NO_CHANGES
[ -f /var/AutoScript.TSS ] && [ ! -L /var/AutoScript.TSS ] && "$BB" cmp -s /var/AutoScript.TSS "$STORE/hook.managed" || fail STARTUP_HOOK_CHANGED_NO_CHANGES
"$BB" awk -f "$STORE/parse-config.awk" "$STORE/config.txt" | "$BB" grep -qx SSH=true || fail MANAGED_SSH_REQUIRED_USE_USB
for app in tar nohup setsid tail; do "$BB" --list | "$BB" grep -qx "$app" || fail NATIVE_TRANSFER_TOOLS_UNAVAILABLE; done
printf 'READY\n'
"$BB" sha256sum "$STORE/package.manifest"
`

func manifestDigest(p *Package) string {
	sum := sha256.Sum256(p.Files["JBLToolkit/SHA256SUMS"])
	return hex.EncodeToString(sum[:])
}

func inspectSSH(c *sshConnection, p *Package) (bool, error) {
	out, err := sshRun(c, sshInspect, nil)
	fields := strings.Fields(out)
	if err != nil || len(fields) != 3 || fields[0] != "READY" || len(fields[1]) != 64 {
		return false, fmt.Errorf("bar check failed: %s", strings.TrimSpace(out))
	}
	return fields[1] == manifestDigest(p), nil
}

func (u *SSHUpdater) Check(ctx context.Context, p *Package, req SSHRequest) (SSHResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c, result, err := u.connect(ctx, req)
	if err != nil || c == nil {
		return result, err
	}
	defer c.Close()
	result.Current, err = inspectSSH(c, p)
	if err != nil {
		return result, err
	}
	result.Success = true
	result.Message = "Connected. Ready to update; installed settings, SSH keys and sound settings will be kept."
	if result.Current {
		result.Message = "Your bar already has this toolkit build."
	}
	return result, nil
}

func sshArchive(p *Package) ([]byte, error) {
	var buffer bytes.Buffer
	w := tar.NewWriter(&buffer)
	dirs := map[string]bool{}
	names := []string{}
	for name := range p.Files {
		if strings.HasPrefix(name, "JBLToolkit/") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var parents []string
		for dir := path.Dir(name); dir != "." && !dirs[dir]; dir = path.Dir(dir) {
			parents = append(parents, dir)
		}
		for i := len(parents) - 1; i >= 0; i-- {
			dir := parents[i]
			if err := w.WriteHeader(&tar.Header{Name: dir + "/", Typeflag: tar.TypeDir, Mode: 0700}); err != nil {
				return nil, err
			}
			dirs[dir] = true
		}
		data := p.Files[name]
		if name == configPath {
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				if strings.HasPrefix(line, "action=") {
					lines[i] = "action=update"
				}
			}
			data = []byte(strings.Join(lines, "\n"))
			if _, err := ParseConfig(string(data)); err != nil {
				return nil, err
			}
		}
		if err := w.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(data))}); err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (u *SSHUpdater) Update(ctx context.Context, p *Package, req SSHRequest, progress func(string)) (SSHResult, error) {
	if progress == nil {
		progress = func(string) {}
	}
	progress("Connecting to your bar…")
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Minute)
	defer cancel()
	c, result, err := u.connect(ctx, req)
	if err != nil || c == nil {
		return result, err
	}
	defer func() { c.Close() }()
	progress("Checking the installed toolkit and saved settings…")
	current, err := inspectSSH(c, p)
	if err != nil {
		return result, err
	}
	if current {
		result.Success, result.Current, result.Message = true, true, "Your bar already has this toolkit build."
		return result, nil
	}
	data, err := sshArchive(p)
	if err != nil {
		return result, err
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return result, err
	}
	stage := "/tmp/jbl-toolkit-ssh." + hex.EncodeToString(nonce)
	root := stage + "/JBLToolkit"
	progress("Checking free memory and transferring toolkit files…")
	init := fmt.Sprintf(`BB=/bin/busybox; available=$("$BB" df -Pk /tmp | "$BB" awk 'NR==2 {print $4}'); case "$available" in ''|*[!0-9]*) exit 1;; esac; [ "$available" -ge %d ] && [ ! -e %s ] && [ ! -L %s ] && "$BB" mkdir -m 700 %s`, (len(data)*3+1023)/1024+4096, stage, stage, stage)
	if _, err = sshRun(c, init, nil); err != nil {
		return result, errors.New("couldn’t create a private transfer directory; check the connection and free memory; the installed toolkit was not changed")
	}
	if _, err = sshRun(c, "/bin/busybox tar -xf - -C "+stage, bytes.NewReader(data)); err != nil {
		sshRun(c, "/bin/busybox rm -rf "+stage, nil)
		return result, errors.New("couldn’t transfer toolkit files; check free memory and the SSH connection; the installed toolkit was not changed")
	}
	if _, err = sshRun(c, "cd "+root+" && /bin/busybox sha256sum -c SHA256SUMS >/dev/null", nil); err != nil {
		sshRun(c, "/bin/busybox rm -rf "+stage, nil)
		return result, errors.New("transferred files failed verification; the installed toolkit was not changed")
	}
	progress("Transfer verified. Starting the bar’s guarded updater…")
	launch := "JBL_TOOLKIT_USB=" + root + " /bin/busybox setsid /bin/busybox nohup /bin/sh " + root + "/ssh-update.sh </dev/null >" + stage + "/worker.log 2>&1 &"
	if _, err = sshRun(c, launch, nil); err != nil {
		return result, fmt.Errorf("update start could not be confirmed; staging retained at %s; check the bar before retrying", stage)
	}
	// The worker survives the updater restarting SSH, or the browser closing.
	result.Reports = map[string]string{}
	interval := u.Poll
	if interval <= 0 {
		interval = time.Second
	}
	started, lastProgress := time.Now(), time.Now()
	for {
		out, pollErr := sshRun(c, "if [ -f "+root+"/SSH-EXIT.txt ]; then /bin/busybox cat "+root+"/SSH-EXIT.txt; fi", nil)
		if pollErr == nil && strings.TrimSpace(out) != "" {
			exit, parseErr := strconv.Atoi(strings.TrimSpace(out))
			if parseErr != nil {
				break
			}
			for _, name := range []string{"STATUS.txt", "native-check.json", "setup.log"} {
				text, _ := sshRun(c, "/bin/busybox tail -c 65536 "+root+"/"+name, nil)
				result.Reports[name] = text
			}
			for label, file := range map[string]string{
				"ssh-status.txt": "/var/JBLToolkit/ssh-input/STATUS.txt",
				"ssh-setup.log":  "/var/JBLToolkit/ssh-input/setup.log",
				"ssh-start.log":  "/var/JBLToolkit/ssh-input/ssh-start.log",
				"sshd.log":       "/var/JBLToolkit/ssh-input/sshd.log",
			} {
				text, _ := sshRun(c, "if [ -f "+file+" ]; then /bin/busybox tail -c 16384 "+file+"; fi", nil)
				if text != "" {
					result.Reports[label] = text
				}
			}
			status := strings.TrimSpace(result.Reports["STATUS.txt"])
			if exit != 0 || status != "INSTALLED_PENDING_REBOOT_TEST" && status != "ALREADY_INSTALLED_LOGS_EXPORTED" {
				result.Message = "The bar stopped the update: " + status + ". Reports retained at " + stage + ". Keep the recovery ZIP."
				if status == "SSH_INSTALL_FAILED" {
					reason := strings.TrimSpace(result.Reports["ssh-status.txt"])
					if reason == "" {
						reason = "no SSH status was recorded"
					}
					result.Message = "SSH setup failed: " + reason + "."
					// Show the native error in the main message, keeping full logs below.
				logs:
					for _, name := range []string{"ssh-start.log", "sshd.log", "ssh-setup.log"} {
						lines := strings.Split(strings.TrimSpace(result.Reports[name]), "\n")
						for i := len(lines) - 1; i >= 0; i-- {
							line := strings.TrimSpace(lines[i])
							for _, term := range []string{"error", "failed", "cannot", "denied", "no space", "not permitted", "unable", "invalid", "not found", "address already", "fatal", "read-only"} {
								if strings.Contains(strings.ToLower(line), term) {
									if len(line) > 512 {
										line = line[:512]
									}
									result.Message += "\n" + line
									break logs
								}
							}
						}
					}
					result.Message += "\nReports retained at " + stage + ". Keep the recovery ZIP."
				}
				return result, nil
			}
			verified, err := inspectSSH(c, p)
			if err != nil || !verified {
				break
			}
			result.Success = true
			result.Message = "Bar updated and installed files verified. Settings and SSH keys kept. Restart and check normal playback; keep your recovery ZIP."
			sshRun(c, "/bin/busybox rm -rf "+stage, nil)
			return result, nil
		}
		if pollErr != nil {
			progress("Reconnecting to your bar…")
			c.Close()
			next, _, err := u.connect(ctx, req)
			if err == nil && next != nil {
				c = next
			}
		}
		if time.Since(lastProgress) >= 10*time.Second {
			progress(fmt.Sprintf("Waiting for the bar’s update result… %d seconds. Keep it powered on.", int(time.Since(started).Seconds())))
			lastProgress = time.Now()
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			goto uncertain
		case <-timer.C:
		}
	}
uncertain:
	result.Message = "Update outcome could not be confirmed. The bar may still be updating; check its connection before retrying. Reports retained at " + stage + ". Keep the recovery ZIP."
	return result, nil
}
