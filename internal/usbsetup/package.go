package usbsetup

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

const entry = "AutoScript/AutoScript"
const configPath = "JBLToolkit/config.txt"
const maxPackage = 128 << 20

var safeName = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
var cryptHash = regexp.MustCompile(`^\$6\$[./0-9A-Za-z]{1,16}\$[./0-9A-Za-z]{86}$`)
var hexName = regexp.MustCompile(`^[0-9a-f]+$`)

type Package struct {
	Files     map[string][]byte
	Version   string
	Digest    string
	Bluetooth bool
	FrontEQ   bool
}

func LoadPackage(filename string) (*Package, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if stat.Size() > maxPackage {
		return nil, errors.New("package exceeds 128 MiB")
	}
	z, err := zip.NewReader(f, stat.Size())
	if err != nil {
		return nil, err
	}
	p := &Package{Files: map[string][]byte{}}
	seen := map[string]bool{}
	var total uint64
	for _, item := range z.File {
		name := item.Name
		parts := strings.Split(name, "/")
		total += item.UncompressedSize64
		if item.UncompressedSize64 > maxPackage || total > maxPackage || len(z.File) > 2048 {
			return nil, errors.New("package expands beyond limits")
		}
		if !safeName.MatchString(name) || path.Clean(name) != name || strings.HasPrefix(name, "/") || len(parts) > 5 || seen[strings.ToLower(name)] || !item.Mode().IsRegular() {
			return nil, fmt.Errorf("unsafe package entry: %q", name)
		}
		for _, part := range parts {
			if part == ".." || part == "." || part == "" {
				return nil, errors.New("unsafe package path")
			}
		}
		switch parts[0] {
		case "AutoScript", "JBLToolkit", "README.txt", "LICENSE.txt", "SETUP.html":
		default:
			return nil, errors.New("unexpected package root")
		}
		if parts[0] == "AutoScript" && name != entry || parts[0] != "AutoScript" && parts[0] != "JBLToolkit" && len(parts) != 1 {
			return nil, errors.New("unexpected package path")
		}
		seen[strings.ToLower(name)] = true
		r, e := item.Open()
		if e != nil {
			return nil, e
		}
		data, e := io.ReadAll(io.LimitReader(r, maxPackage+1))
		r.Close()
		if e != nil {
			return nil, e
		}
		p.Files[name] = data
	}
	if string(p.Files[entry]) != "CLI(CLI_exec sh /mnt/sda1/JBLToolkit/dispatch.sh &)\r\n" {
		return nil, errors.New("unrecognized USB entry point")
	}
	for _, name := range []string{"package.json", "config.txt", "SHA256SUMS", "BOOTSTRAP.SHA256SUMS", "dispatch.sh", "launch.sh", "common.sh", "check.sh", "verify.sh", "recover.sh", "parse-config.awk", "files/remote"} {
		if len(p.Files["JBLToolkit/"+name]) == 0 {
			return nil, fmt.Errorf("incomplete package: %s", name)
		}
	}
	var meta struct {
		Schema                                  int
		Model, Profile, Architecture, Selection string
		Version                                 string
		DispatcherSchema                        int  `json:"dispatcher_schema"`
		Bluetooth                               bool `json:"bluetooth_helper"`
		FrontEQ                                 bool `json:"front_eq_helper"`
	}
	if json.Unmarshal(p.Files["JBLToolkit/package.json"], &meta) != nil || meta.Schema != 1 || meta.Model != "bar91" || meta.Profile != "bar91-23.06.41.80" || meta.Architecture != "linux-armv7" || meta.Selection != "automatic" || meta.DispatcherSchema != 1 {
		return nil, errors.New("unsupported package/profile")
	}
	elf := p.Files["JBLToolkit/files/remote"]
	if len(elf) < 20 || !bytes.Equal(elf[:7], []byte{127, 'E', 'L', 'F', 1, 1, 1}) || !bytes.Equal(elf[18:20], []byte{40, 0}) {
		return nil, errors.New("package remote is not ARMv7 ELF")
	}
	covered := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(p.Files["JBLToolkit/SHA256SUMS"])), "\n") {
		pair := strings.Split(line, "  ")
		if len(pair) != 2 || len(pair[0]) != 64 {
			return nil, errors.New("invalid package manifest")
		}
		name := "JBLToolkit/" + pair[1]
		data, ok := p.Files[name]
		sum := sha256.Sum256(data)
		if !ok || name == configPath || name == "JBLToolkit/SHA256SUMS" || covered[name] || hex.EncodeToString(sum[:]) != pair[0] {
			return nil, errors.New("package integrity check failed")
		}
		covered[name] = true
	}
	for name := range p.Files {
		if strings.HasPrefix(name, "JBLToolkit/") && name != configPath && name != "JBLToolkit/SHA256SUMS" && !covered[name] {
			return nil, errors.New("manifest does not cover complete payload")
		}
	}
	var bootstrap strings.Builder
	for _, name := range []string{"common.sh", "dispatch.sh", "launch.sh", "recover.sh"} {
		sum := sha256.Sum256(p.Files["JBLToolkit/"+name])
		fmt.Fprintf(&bootstrap, "%x  %s\n", sum, name)
	}
	if bootstrap.String() != string(p.Files["JBLToolkit/BOOTSTRAP.SHA256SUMS"]) {
		return nil, errors.New("invalid recovery bootstrap manifest")
	}
	if _, err = ParseConfig(string(p.Files[configPath])); err != nil {
		return nil, err
	}
	hash := sha256.New()
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	if _, err = io.Copy(hash, f); err != nil {
		return nil, err
	}
	p.Digest = hex.EncodeToString(hash.Sum(nil))
	p.Version = meta.Version
	p.Bluetooth, p.FrontEQ = meta.Bluetooth, meta.FrontEQ
	return p, nil
}

// ParseConfig mirrors the device's strict data grammar; it never executes text.
func ParseConfig(text string) (map[string]string, error) {
	fail := func() (map[string]string, error) { return nil, errors.New("invalid toolkit configuration") }
	if len(text) > 8192 || strings.ContainsRune(text, 0) {
		return fail()
	}
	m := map[string]string{}
	allowed := map[string]bool{}
	for _, k := range strings.Fields("format action ssh web_remote auto_standard smart_mode_memory allowed_cidr web_port ssh_password_hash wifi_ssid_hex wifi_psk_hex") {
		allowed[k] = true
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if len(line) > 256 {
			return fail()
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "=")
		if len(parts) != 2 {
			return fail()
		}
		k, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if !allowed[k] {
			return fail()
		}
		if _, ok := m[k]; ok {
			return fail()
		}
		m[k] = v
	}
	for _, k := range strings.Fields("format action ssh web_remote auto_standard allowed_cidr web_port") {
		if _, ok := m[k]; !ok {
			return fail()
		}
	}
	actions := map[string]bool{"check": true, "install": true, "update": true, "configure": true, "verify": true, "uninstall": true, "migrate": true}
	if m["format"] != "1" || !actions[m["action"]] {
		return fail()
	}
	if _, ok := m["smart_mode_memory"]; !ok {
		m["smart_mode_memory"] = "false"
	}
	for _, k := range []string{"ssh", "web_remote", "auto_standard", "smart_mode_memory"} {
		if m[k] != "true" && m[k] != "false" {
			return fail()
		}
	}
	if m["smart_mode_memory"] == "true" && m["auto_standard"] != "true" {
		return fail()
	}
	if !validNetwork(m["allowed_cidr"]) || !validPort(m["web_port"]) {
		return fail()
	}
	if v, ok := m["ssh_password_hash"]; ok && !cryptHash.MatchString(v) {
		return fail()
	}
	ssid, sok := m["wifi_ssid_hex"]
	psk, pok := m["wifi_psk_hex"]
	if sok != pok || sok && (len(ssid) < 2 || len(ssid) > 64 || len(ssid)%2 != 0 || !hexName.MatchString(ssid) || len(psk) != 64 || !hexName.MatchString(psk)) {
		return fail()
	}
	return m, nil
}

func (p *Package) Verify(root *os.Root) error {
	for name, data := range p.Files {
		if name == configPath {
			continue
		}
		info, e := root.Lstat(name)
		if e != nil || !info.Mode().IsRegular() || info.Size() != int64(len(data)) {
			return fmt.Errorf("existing toolkit incomplete or modified: %s", name)
		}
		got, e := root.ReadFile(name)
		if e != nil || !bytes.Equal(got, data) {
			return fmt.Errorf("existing toolkit differs: %s; use an empty or explicitly formatted USB", name)
		}
	}
	if _, e := root.Lstat("JBLToolkit/RECOVER"); !errors.Is(e, fs.ErrNotExist) {
		return errors.New("USB has a recovery marker; use a separate installation USB")
	}
	return nil
}

func writeFile(root *os.Root, name string, data []byte) error {
	if err := root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	got, err := root.ReadFile(name)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, data) {
		return errors.New("USB readback mismatch")
	}
	return nil
}

func (p *Package) Apply(root *os.Root, config string) error {
	if _, err := ParseConfig(config); err != nil {
		return err
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	items, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return err
	}
	// OS-created metadata is normal on an otherwise empty formatted USB.
	var meaningful []string
	for _, item := range items {
		switch item.Name() {
		case ".Spotlight-V100", ".Trashes", ".fseventsd", "System Volume Information", "$RECYCLE.BIN", "lost+found":
			continue
		}
		meaningful = append(meaningful, item.Name())
	}
	if len(meaningful) > 0 {
		if err = p.Verify(root); err != nil {
			return err
		}
		temp := "JBLToolkit/config-" + rand.Text() + ".tmp"
		defer root.Remove(temp)
		if err = writeFile(root, temp, []byte(config)); err != nil {
			return err
		}
		if err = root.Rename(temp, configPath); err != nil {
			return err
		}
	} else {
		names := make([]string, 0, len(p.Files))
		for name := range p.Files {
			if name != entry && name != configPath {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			if err = writeFile(root, name, p.Files[name]); err != nil {
				return err
			}
		}
		if err = writeFile(root, configPath, []byte(config)); err != nil {
			return err
		}
		// Publish AutoScript only after the rest has been flushed and verified.
		if err = writeFile(root, entry, p.Files[entry]); err != nil {
			return err
		}
	}
	got, err := root.ReadFile(configPath)
	if err != nil || string(got) != config {
		return errors.New("configuration readback failed")
	}
	return p.Verify(root)
}

func readConfig(root *os.Root) ([]byte, error) {
	f, e := root.Open(configPath)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Size() > 8192 {
		return nil, errors.New("invalid saved configuration")
	}
	return io.ReadAll(io.LimitReader(f, 8193))
}
