package usbsetup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	setupassets "jbl-bar-toolkit/packaging"
)

type Server struct {
	Origin, Token, Version, Executable string
	PackagePath, RecoveryPath          string
	Downloads                          *Downloads
	List                               func(context.Context) ([]Drive, error)
	Format                             func(context.Context, Drive, string, string) error
	Eject                              func(context.Context, Drive) error
	RestartRequired                    bool
	LocalSource                        bool
	mu                                 sync.Mutex
}

func NewServer(version, exe, pack string) (*Server, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return nil, e
	}
	d, e := NewDownloads()
	if e != nil {
		return nil, e
	}
	s := &Server{Token: hex.EncodeToString(b), Version: version, Executable: exe, PackagePath: pack, Downloads: d, List: Discover, Format: ElevatedPrepare, Eject: Eject}
	if pack != "" {
		payload, err := LoadPackage(pack)
		if err != nil {
			return nil, err
		}
		recovery := filepath.Join(filepath.Dir(pack), "toolkit-recovery.zip")
		if strings.HasSuffix(pack, "-usb.zip") {
			recovery = strings.TrimSuffix(pack, "-usb.zip") + "-recovery.zip"
		}
		if _, err = os.Stat(recovery); err == nil {
			if err = VerifyRecovery(recovery, payload); err != nil {
				return nil, err
			}
			s.RecoveryPath = recovery
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return s, nil
}
func respond(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, e error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": e.Error()})
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	if "http://"+r.Host != s.Origin {
		http.Error(w, "Invalid local host", 403)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		if r.Header.Get("X-Session-Token") != s.Token || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.Origin {
			http.Error(w, "Invalid local session", 403)
			return
		}
		if r.Method != "GET" && r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		if r.Method == "POST" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "JSON required", 415)
			return
		}
		if r.Method == "POST" {
			if !s.mu.TryLock() {
				http.Error(w, "Another operation is running", 409)
				return
			}
			defer s.mu.Unlock()
		}
		switch r.URL.Path {
		case "/api/info":
			if r.Method != "GET" {
				break
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			version := ""
			features := map[string]bool{}
			if s.PackagePath != "" {
				p, e := LoadPackage(s.PackagePath)
				if e != nil {
					failure(w, e)
					return
				}
				version = p.Version
				features["bluetooth"] = p.Bluetooth
				features["frontEQ"] = p.FrontEQ
			}
			respond(w, map[string]any{"version": s.Version, "packageVersion": version, "features": features, "localSource": s.LocalSource, "networks": Networks(), "canUpdateSelf": s.Version != "dev" && !s.LocalSource, "hasRecovery": s.RecoveryPath != ""})
			return
		case "/api/drives":
			if r.Method != "GET" {
				break
			}
			drives, e := s.List(r.Context())
			if e != nil {
				failure(w, e)
				return
			}
			result := []map[string]any{}
			for _, d := range drives {
				row := map[string]any{"drive": d}
				if d.Eligible {
					root, e := openDrive(d)
					if e == nil {
						if b, e := readConfig(root); e == nil {
							if cfg, e := ParseConfig(string(b)); e == nil {
								pub := map[string]string{}
								for _, key := range []string{"action", "ssh", "web_remote", "auto_standard", "smart_mode_memory", "allowed_cidr"} {
									pub[key] = cfg[key]
								}
								row["config"] = pub
								row["hasSSH"] = cryptHash.MatchString(cfg["ssh_password_hash"])
							}
						}
						reports := map[string]string{}
						for _, name := range []string{"STATUS.txt", "HARDWARE.txt", "CHECKS.txt", "PREFLIGHT.txt", "native-check.json", "setup.log"} {
							f, e := root.Open("JBLToolkit/" + name)
							if e == nil {
								if info, err := f.Stat(); err == nil && info.Size() > 65536 {
									if _, err = f.Seek(-65536, io.SeekEnd); err != nil {
										f.Close()
										continue
									}
								}
								b, e := io.ReadAll(io.LimitReader(f, 65536))
								f.Close()
								if e == nil {
									reports[name] = string(b)
								}
							}
						}
						row["reports"] = reports
						root.Close()
					}
				}
				result = append(result, row)
			}
			respond(w, result)
			return
		case "/api/download":
			if s.LocalSource {
				failure(w, errors.New("repository setup uses local files; restart to rebuild the checkout"))
				return
			}
			if r.Method != "POST" {
				break
			}
			release, e := s.Downloads.Latest(r.Context())
			if e != nil {
				failure(w, e)
				return
			}
			if s.PackagePath != "" {
				current, err := LoadPackage(s.PackagePath)
				if err != nil {
					failure(w, err)
					return
				}
				if semver.IsValid("v"+current.Version) && semver.Compare(release.Tag, "v"+current.Version) < 0 {
					failure(w, errors.New("the published toolkit is older than your local package; keeping local files"))
					return
				}
			}
			pack, recovery, e := s.Downloads.Toolkit(r.Context(), release)
			if e != nil {
				failure(w, e)
				return
			}
			s.PackagePath = pack
			s.RecoveryPath = recovery
			respond(w, map[string]any{"version": strings.TrimPrefix(release.Tag, "v"), "recoverySaved": true, "selfUpdateAvailable": CanUpdate(s.Version, release)})
			return
		case "/api/recovery":
			if r.Method != "GET" {
				break
			}
			s.mu.Lock()
			name := s.RecoveryPath
			s.mu.Unlock()
			if name == "" {
				failure(w, errors.New("download the matching recovery package first"))
				return
			}
			f, e := os.Open(name)
			if e != nil {
				failure(w, e)
				return
			}
			defer f.Close()
			info, e := f.Stat()
			if e != nil {
				failure(w, e)
				return
			}
			w.Header().Set("Content-Disposition", `attachment; filename="jbl-toolkit-recovery.zip"`)
			w.Header().Set("Content-Type", "application/zip")
			http.ServeContent(w, r, "jbl-toolkit-recovery.zip", info.ModTime(), f)
			return
		case "/api/self-update":
			if s.LocalSource {
				failure(w, errors.New("repository setup uses current source; self-update is for downloaded executables"))
				return
			}
			if r.Method != "POST" {
				break
			}
			release, e := s.Downloads.Latest(r.Context())
			if e != nil {
				failure(w, e)
				return
			}
			if !CanUpdate(s.Version, release) {
				failure(w, errors.New("no newer compatible setup executable is published"))
				return
			}
			if e = s.Downloads.UpdateSelf(r.Context(), release, s.Executable); e != nil {
				failure(w, e)
				return
			}
			s.RestartRequired = true
			respond(w, map[string]string{"message": "Setup updated. Close this window and launch the same executable again."})
			return
		case "/api/prepare":
			if r.Method != "POST" {
				break
			}
			if s.RestartRequired {
				failure(w, errors.New("restart the updated setup executable before preparing a USB"))
				return
			}
			var req struct {
				ID, Config, Confirm string
				Format, KeepSSH     bool
			}
			dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
			dec.DisallowUnknownFields()
			if e := dec.Decode(&req); e != nil {
				failure(w, errors.New("invalid setup request"))
				return
			}
			if e := dec.Decode(new(any)); e != io.EOF {
				failure(w, errors.New("unexpected request data"))
				return
			}
			if s.PackagePath == "" {
				failure(w, errors.New("download or select a toolkit package first"))
				return
			}
			p, e := LoadPackage(s.PackagePath)
			if e != nil {
				failure(w, e)
				return
			}
			drives, e := s.List(r.Context())
			if e != nil {
				failure(w, e)
				return
			}
			var d Drive
			for _, v := range drives {
				if v.ID == req.ID {
					d = v
				}
			}
			if d.ID == "" {
				failure(w, errors.New("selected USB changed or was removed; refresh and select it again"))
				return
			}
			wanted := d.Device
			if req.Format {
				wanted = "ERASE " + d.Device
			}
			if req.Confirm != wanted {
				failure(w, errors.New("confirm the selected drive before writing"))
				return
			}
			cfg, e := ParseConfig(req.Config)
			if e != nil {
				failure(w, e)
				return
			}
			if req.KeepSSH {
				root, e := openDrive(d)
				if e != nil {
					failure(w, e)
					return
				}
				old, e := readConfig(root)
				root.Close()
				if e != nil {
					failure(w, errors.New("no saved SSH credentials; enter a new password"))
					return
				}
				settings, e := ParseConfig(string(old))
				if e != nil || !cryptHash.MatchString(settings["ssh_password_hash"]) {
					failure(w, errors.New("no saved SSH credentials; enter a new password"))
					return
				}
				if _, ok := cfg["ssh_password_hash"]; !ok {
					req.Config += "ssh_password_hash=" + settings["ssh_password_hash"] + "\n"
					cfg["ssh_password_hash"] = settings["ssh_password_hash"]
				}
			}
			if cfg["ssh"] == "true" && !cryptHash.MatchString(cfg["ssh_password_hash"]) && len(p.Files["JBLToolkit/files/ssh/password.hash"]) == 0 {
				failure(w, errors.New("choose an SSH password before enabling SSH"))
				return
			}
			if req.Format {
				if d.ReadOnly || d.Size < 64<<20 || d.Size > 2<<40 {
					failure(w, errors.New("this USB cannot be formatted"))
					return
				}
				e = s.Format(context.WithoutCancel(r.Context()), d, s.PackagePath, req.Config)
			} else {
				if !d.Eligible {
					failure(w, fmt.Errorf("USB is not ready: %s", d.Reason))
					return
				}
				var root *os.Root
				root, e = openDrive(d)
				if e == nil {

					fresh, err := s.List(r.Context())
					if err != nil {
						root.Close()
						failure(w, err)
						return
					}
					matched := false
					for _, v := range fresh {
						if v.ID == d.ID && v.Fingerprint == d.Fingerprint && v.Mount == d.Mount && v.Eligible {
							matched = true
						}
					}
					if !matched {
						root.Close()
						failure(w, errors.New("USB changed before writing; refresh and retry"))
						return
					}
					e = p.Apply(root, req.Config)
					closeErr := root.Close()
					if e == nil {
						e = closeErr
					}
				}
			}
			if e != nil {
				failure(w, e)
				return
			}
			ejectCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
			ejectErr := s.Eject(ejectCtx, d)
			cancel()
			result := map[string]any{"written": true, "ejected": ejectErr == nil, "action": cfg["action"]}
			if ejectErr == nil {
				result["message"] = "USB written, verified, and safely ejected. Insert it into the awake soundbar, then reconnect it here to read the results."
			} else {
				result["message"] = "USB written and verified. Automatic eject failed; safely eject it using your operating system before removing it. " + ejectErr.Error()
				result["warning"] = ejectErr.Error()
			}
			respond(w, result)
			return
		}
		http.Error(w, "Unknown operation or method", 404)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	name := "wizard" + r.URL.Path
	ctype := "text/plain; charset=utf-8"
	switch r.URL.Path {
	case "/":
		name = "wizard/index.html"
		ctype = "text/html; charset=utf-8"
	case "/app.js":
		ctype = "text/javascript; charset=utf-8"
	case "/favicon.svg":
		ctype = "image/svg+xml"
	case "/style.css":
		ctype = "text/css; charset=utf-8"
	case "/licenses":
		name = "wizard/licenses.txt"
	case "/setup.js":
		name = "setup.js"
		ctype = "text/javascript; charset=utf-8"
	default:
		http.NotFound(w, r)
		return
	}
	data, e := setupassets.Files.ReadFile(name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Write(data)
}
