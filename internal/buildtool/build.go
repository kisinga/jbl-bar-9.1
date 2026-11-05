// Package buildtool owns the public USB and portable application build used by CI
// and local setup. Running these builders never accesses a physical drive.
package buildtool

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"jbl-bar-toolkit/internal/usbsetup"
)

const PublicConfig = "# Run the portable JBL setup executable to configure your USB.\nformat=1\naction=check\nssh=false\nweb_remote=true\nauto_standard=false\nallowed_cidr=192.168.1.0/24\nweb_port=80\n"

var Targets = [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}}
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)

type Builder struct{ Root, Go string }

func New(ctx context.Context, root string) (*Builder, error) {
	root, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return nil, e
	}
	if _, e = os.Stat(filepath.Join(root, "apps/usbsetup/main.go")); e != nil {
		return nil, errors.New("not a toolkit source checkout")
	}
	tool, e := Compiler(ctx, root)
	if e != nil {
		return nil, e
	}
	return &Builder{Root: root, Go: tool}, nil
}
func (b *Builder) Version() (string, error) {
	data, e := os.ReadFile(filepath.Join(b.Root, "VERSION"))
	v := strings.TrimSpace(string(data))
	if e != nil {
		return "", e
	}
	if !versionPattern.MatchString(v) {
		return "", errors.New("invalid VERSION")
	}
	return v, nil
}
func (b *Builder) command(ctx context.Context, dir string, env map[string]string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, b.Go, args...)
	c.Dir = dir
	// Do not inherit a caller's cross-compilation, overlay or output flags.
	values := map[string]string{"GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH, "GOARM": "", "CGO_ENABLED": "0", "GOFLAGS": "", "GOWORK": "off", "GOTOOLCHAIN": "local", "GOROOT": filepath.Dir(filepath.Dir(b.Go))}
	for k, v := range env {
		values[k] = v
	}
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if _, ok := values[k]; !ok {
			c.Env = append(c.Env, v)
		}
	}
	for k, v := range values {
		c.Env = append(c.Env, k+"="+v)
	}
	return c
}
func (b *Builder) run(ctx context.Context, dir string, env map[string]string, args ...string) error {
	c := b.command(ctx, dir, env, args...)
	c.Stdout, c.Stderr = os.Stderr, os.Stderr
	return c.Run()
}
func (b *Builder) safeOutput(out string) (string, error) {
	out, e := filepath.Abs(out)
	if e != nil {
		return "", e
	}
	// Resolve existing parents too, so an output symlink cannot escape build/dist.
	parent := out
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			out = filepath.Join(append([]string{resolved}, tail...)...)
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		tail = append([]string{filepath.Base(parent)}, tail...)
		next := filepath.Dir(parent)
		if next == parent {
			return "", errors.New("invalid output path")
		}
		parent = next
	}
	for _, base := range []string{"build", "dist"} {
		rel, e := filepath.Rel(filepath.Join(b.Root, base), out)
		if e == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return out, nil
		}
	}
	return "", errors.New("outputs must stay inside this checkout's build/ or dist/")
}
func (b *Builder) emptyOutput(out string) (string, error) {
	out, e := b.safeOutput(out)
	if e != nil {
		return "", e
	}
	entries, e := os.ReadDir(out)
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return "", e
	}
	if len(entries) > 0 {
		return "", errors.New("use an empty output directory")
	}
	return out, os.MkdirAll(out, 0700)
}
func manifest(files map[string][]byte) []byte {
	var text strings.Builder
	for _, name := range names(files) {
		fmt.Fprintf(&text, "%x  %s\n", sha256.Sum256(files[name]), name)
	}
	return []byte(text.String())
}
func names(files map[string][]byte) []string {
	out := make([]string, 0, len(files))
	for n := range files {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
func writeZip(name string, files map[string][]byte, executable string) error {
	f, e := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	good := false
	defer func() {
		f.Close()
		if !good {
			os.Remove(name)
		}
	}()
	z := zip.NewWriter(f)
	for _, n := range names(files) {
		h := &zip.FileHeader{Name: n, Method: zip.Deflate, ModifiedDate: 33}
		h.SetMode(0600)
		if executable != "" {
			h.SetMode(0644)
			if n == executable {
				h.SetMode(0755)
			}
		}
		w, err := z.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err = w.Write(files[n]); err != nil {
			return err
		}
	}
	if e = z.Close(); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	good = true
	return nil
}
func (b *Builder) read(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(b.Root, filepath.FromSlash(name)))
}

type profile struct {
	Schema      int `json:"schema_version"`
	ID, Model   string
	Executables map[string]string
	Helpers     map[string]map[string]string `json:"native_helpers"`
}

func (b *Builder) checkedProfile() (*profile, error) {
	data, e := b.read("models/bar91/profile.json")
	if e != nil {
		return nil, e
	}
	var p profile
	if json.Unmarshal(data, &p) != nil || p.Schema != 1 || p.Model != "bar91" || p.ID != "bar91-23.06.41.80" || len(p.Executables) != 2 {
		return nil, errors.New("unsupported firmware profile")
	}
	hash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, m := range append([]map[string]string{p.Executables}, p.Helpers["bluetooth"], p.Helpers["eq_parser"]) {
		if len(m) == 0 {
			return nil, errors.New("incomplete profile")
		}
		for _, v := range m {
			if !hash.MatchString(v) {
				return nil, errors.New("invalid profile hash")
			}
		}
	}
	if p.Executables["bdpprog"] == "" || p.Executables["cli"] == "" {
		return nil, errors.New("incomplete executable identities")
	}
	var source strings.Builder
	source.WriteString("// Code generated by tools/check_profiles.py --write; DO NOT EDIT.\n// Source: models/bar91/profile.json. Runtime eligibility remains firmware-pinned.\npackage main\n\n")
	fmt.Fprintf(&source, "const firmwareHash = %q\nconst cliHash = %q\n\n", p.Executables["bdpprog"], p.Executables["cli"])
	for _, pair := range [][2]string{{"bluetoothLibraries", "bluetooth"}, {"eqParserDependencies", "eq_parser"}} {
		fmt.Fprintf(&source, "var %s = map[string]string{\n", pair[0])
		keys := []string{}
		for k := range p.Helpers[pair[1]] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&source, "\t%q: %q,\n", k, p.Helpers[pair[1]][k])
		}
		source.WriteString("}\n\n")
	}
	expected, e := format.Source([]byte(source.String()))
	if e != nil {
		return nil, e
	}
	actual, e := b.read("apps/remote/profile_bar91_generated.go")
	if e != nil {
		return nil, e
	}
	if string(expected) != string(actual) {
		return nil, errors.New("generated remote profile differs from reviewed metadata")
	}
	for _, name := range []string{"run.sh", "once.sh", "controller.sh", "helper.sh"} {
		data, e = b.read("models/bar91/smart-mode/JBLInstall/payload/" + name)
		if e != nil {
			return nil, e
		}
		for _, pair := range [][2]string{{"EXPECTED_BDP", "bdpprog"}, {"EXPECTED_CLI", "cli"}} {
			if name == "run.sh" && pair[0] == "EXPECTED_CLI" {
				continue
			}
			re := regexp.MustCompile(`(?m)^` + pair[0] + `=([0-9a-f]{64})$`)
			match := re.FindSubmatch(data)
			if len(match) != 2 || string(match[1]) != p.Executables[pair[1]] {
				return nil, errors.New("profile differs from pinned standalone guards")
			}
		}
	}
	data, e = b.read("models/bar91/usb/preflight.sh")
	if e != nil {
		return nil, e
	}
	matches := regexp.MustCompile(`\[ "\$\{hash%% \*\}" = ([0-9a-f]{64}) \]`).FindAllSubmatch(data, -1)
	if len(matches) != 2 || string(matches[0][1]) != p.Executables["bdpprog"] || string(matches[1][1]) != p.Executables["cli"] {
		return nil, errors.New("USB preflight differs from firmware profile")
	}
	data, e = b.read("apps/remote/eq_native.go")
	if e != nil {
		return nil, e
	}
	guard := regexp.MustCompile(`const eqHDSPHash = "([0-9a-f]{64})"`).FindSubmatch(data)
	if len(guard) != 2 || string(guard[1]) != p.Helpers["eq_parser"]["/usr/lib/libhdsp.so"] {
		return nil, errors.New("native EQ guard differs from profile")
	}
	return &p, nil
}

// Payload builds the same public files for local setup and CI. No host shell,
// BusyBox or Python is involved; firmware guards remain inside the payload.
func (b *Builder) Payload(ctx context.Context, out, model string) ([]string, error) {
	if model != "auto" && model != "bar91" {
		return nil, fmt.Errorf("%s: installation is not qualified", model)
	}
	p, e := b.checkedProfile()
	if e != nil {
		return nil, e
	}
	v, e := b.Version()
	if e != nil {
		return nil, e
	}
	out, e = b.emptyOutput(out)
	if e != nil {
		return nil, e
	}
	work, e := os.MkdirTemp(out, ".build-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(work)
	files := map[string][]byte{}
	for _, group := range [][2]string{{"models/bar91/usb", ""}, {"models/bar91/ssh", "files/ssh/"}} {
		entries, e := os.ReadDir(filepath.Join(b.Root, group[0]))
		if e != nil {
			return nil, e
		}
		for _, f := range entries {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".sh") {
				data, e := b.read(group[0] + "/" + f.Name())
				if e != nil {
					return nil, e
				}
				files[group[1]+f.Name()] = data
			}
		}
	}
	for _, pair := range [][2]string{{"parse-config.awk", "models/bar91/usb/parse-config.awk"}, {"files/remote-start.sh", "models/bar91/remote/start.sh"}, {"setup.js", "packaging/setup.js"}} {
		files[pair[0]], e = b.read(pair[1])
		if e != nil {
			return nil, e
		}
	}
	bt, eq, e := b.nativeHelpers(*p)
	if e != nil {
		return nil, e
	}
	overlay, e := b.nativeOverlay(work, bt, eq)
	if e != nil {
		return nil, e
	}
	remote := filepath.Join(work, "remote")
	if e = b.run(ctx, filepath.Join(b.Root, "apps/remote"), map[string]string{"GOOS": "linux", "GOARCH": "arm", "GOARM": "7"}, "build", "-overlay="+overlay, "-buildvcs=false", "-trimpath", "-ldflags=-s -w -buildid=", "-o", remote, "."); e != nil {
		return nil, e
	}
	files["files/remote"], e = os.ReadFile(remote)
	if e != nil {
		return nil, e
	}
	files["VERSION.txt"] = []byte(v + "\n")
	template, e := b.read("packaging/usb/dispatch.sh.in")
	if e != nil {
		return nil, e
	}
	text := string(template)
	for k, val := range map[string]string{"MODEL": "bar91", "PROFILE": p.ID, "BDP_HASH": p.Executables["bdpprog"], "CLI_HASH": p.Executables["cli"]} {
		text = strings.ReplaceAll(text, "@"+k+"@", val)
	}
	if regexp.MustCompile(`@[A-Z_]+@`).MatchString(text) {
		return nil, errors.New("incomplete dispatcher template")
	}
	files["dispatch.sh"] = []byte(text)
	bootstrap := map[string][]byte{}
	for _, n := range []string{"dispatch.sh", "launch.sh", "common.sh", "recover.sh"} {
		bootstrap[n] = files[n]
	}
	files["BOOTSTRAP.SHA256SUMS"] = manifest(bootstrap)
	meta := struct {
		Schema        int    `json:"schema"`
		Version       string `json:"version"`
		Model         string `json:"model"`
		Profile       string `json:"profile"`
		Architecture  string `json:"architecture"`
		Bluetooth     bool   `json:"bluetooth_helper"`
		EQ            bool   `json:"front_eq_helper"`
		Qualification string `json:"qualification"`
		Selection     string `json:"selection"`
		Bootstrap     string `json:"bootstrap"`
		Dispatcher    int    `json:"dispatcher_schema"`
	}{1, v, "bar91", p.ID, "linux-armv7", true, true, "hardware-test-pending", "automatic", "bar91-autoscript", 1}
	files["package.json"], e = json.MarshalIndent(meta, "", "  ")
	if e != nil {
		return nil, e
	}
	files["package.json"] = append(files["package.json"], '\n')
	files["SHA256SUMS"] = manifest(files)
	files["config.txt"] = []byte(PublicConfig)
	archive := map[string][]byte{}
	for name, data := range files {
		archive["JBLToolkit/"+name] = data
	}
	archive["AutoScript/AutoScript"] = []byte("CLI(CLI_exec sh /mnt/sda1/JBLToolkit/dispatch.sh &)\r\n")
	for _, pair := range [][2]string{{"README.txt", "models/bar91/usb/USB-README.txt"}, {"SETUP.html", "packaging/SETUP.html"}, {"LICENSE.txt", "LICENSE"}} {
		archive[pair[0]], e = b.read(pair[1])
		if e != nil {
			return nil, e
		}
	}
	notice, e := b.read("models/bar91/remote/native/bin/NOTICE.txt")
	if e != nil {
		return nil, e
	}
	archive["LICENSE.txt"] = append(archive["LICENSE.txt"], append([]byte("\n"), notice...)...)
	recovery := map[string][]byte{"AutoScript/AutoScript": archive["AutoScript/AutoScript"], "JBLToolkit/RECOVER": []byte("RECOVER\n"), "README.txt": archive["README.txt"], "LICENSE.txt": archive["LICENSE.txt"]}
	for n, data := range bootstrap {
		recovery["JBLToolkit/"+n] = data
	}
	recovery["JBLToolkit/BOOTSTRAP.SHA256SUMS"] = files["BOOTSTRAP.SHA256SUMS"]
	stem := "jbl-bar-toolkit-v" + v
	if model != "auto" {
		stem = "jbl-bar91-v" + v
	}
	install := filepath.Join(out, stem+"-usb.zip")
	rescue := filepath.Join(out, stem+"-recovery.zip")
	if e = writeZip(install, archive, ""); e != nil {
		return nil, e
	}
	if e = writeZip(rescue, recovery, ""); e != nil {
		return nil, e
	}
	if _, e = usbsetup.LoadPackage(install); e != nil {
		return nil, e
	}
	return []string{install, rescue}, nil
}
func (b *Builder) Executable(ctx context.Context, out, version, system, arch string) error {
	if version != "dev" && !versionPattern.MatchString(version) {
		return errors.New("invalid build version")
	}
	valid := false
	for _, t := range Targets {
		if t == [2]string{system, arch} {
			valid = true
		}
	}
	if !valid {
		return errors.New("unsupported setup target")
	}
	out, e := b.safeOutput(out)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(out); !errors.Is(e, os.ErrNotExist) {
		return errors.New("setup executable already exists")
	}
	if e = os.MkdirAll(filepath.Dir(out), 0700); e != nil {
		return e
	}
	e = b.run(ctx, b.Root, map[string]string{"GOOS": system, "GOARCH": arch}, "build", "-buildvcs=false", "-trimpath", "-ldflags=-s -w -buildid= -X main.version="+version, "-o", out, "./apps/usbsetup")
	if e != nil {
		return e
	}
	return os.Chmod(out, 0755)
}
func (b *Builder) Applications(ctx context.Context, out, version, pack, recovery string) ([]string, error) {
	payload, err := usbsetup.LoadPackage(pack)
	if err != nil {
		return nil, err
	}
	if payload.Version != version {
		return nil, errors.New("setup/package version mismatch")
	}
	if err = usbsetup.VerifyRecovery(recovery, payload); err != nil {
		return nil, err
	}
	out, e := b.safeOutput(out)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(out, 0700); e != nil {
		return nil, e
	}
	assets := []string{}
	for _, t := range Targets {
		binary := filepath.Join(out, usbsetup.BinaryName(t[0], t[1]))
		bundle := filepath.Join(out, "jbl-bar-toolkit-v"+version+"-setup-"+t[0]+"-"+t[1]+".zip")
		for _, n := range []string{binary, bundle} {
			if _, err := os.Lstat(n); !errors.Is(err, os.ErrNotExist) {
				return nil, errors.New("setup assets already exist")
			}
		}
	}
	for _, t := range Targets {
		binary := filepath.Join(out, usbsetup.BinaryName(t[0], t[1]))
		if e = b.Executable(ctx, binary, version, t[0], t[1]); e != nil {
			return nil, e
		}
		name := "jbl-setup"
		if t[0] == "windows" {
			name += ".exe"
		}
		data := map[string][]byte{}
		for key, path := range map[string]string{name: binary, "toolkit-usb.zip": pack, "toolkit-recovery.zip": recovery, "LICENSE.txt": filepath.Join(b.Root, "packaging/wizard/licenses.txt")} {
			data[key], e = os.ReadFile(path)
			if e != nil {
				return nil, e
			}
		}
		data["README.txt"] = []byte("Run jbl-setup (jbl-setup.exe on Windows) from your computer. Select your USB in the local interface. No installation is needed. Do not run from a USB you plan to format. Keep toolkit-recovery.zip.\n")
		bundle := filepath.Join(out, "jbl-bar-toolkit-v"+version+"-setup-"+t[0]+"-"+t[1]+".zip")
		if e = writeZip(bundle, data, name); e != nil {
			return nil, e
		}
		assets = append(assets, binary, bundle)
	}
	return assets, nil
}
func (b *Builder) Distribution(ctx context.Context, out, model string) ([]string, error) {
	paths, e := b.Payload(ctx, out, model)
	if e != nil {
		return nil, e
	}
	out = filepath.Dir(paths[0])
	v, e := b.Version()
	if e != nil {
		return nil, e
	}
	var apps []string
	if model == "auto" {
		apps, e = b.Applications(ctx, out, v, paths[0], paths[1])
		if e != nil {
			return nil, e
		}
	}
	commit := "unknown"
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = b.Root
	if data, err := cmd.Output(); err == nil {
		commit = strings.TrimSpace(string(data))
	}
	prefix := ""
	if model != "auto" {
		prefix = model + "-"
	}
	info := filepath.Join(out, prefix+"release.json")
	sums := filepath.Join(out, prefix+"SHA256SUMS")
	assets := []string{filepath.Base(paths[0]), filepath.Base(paths[1])}
	for _, n := range apps {
		assets = append(assets, filepath.Base(n))
	}
	report := map[string]any{"schema": 1, "version": v, "source_commit": commit, "model": "bar91", "target": model, "selection": "automatic", "included_models": []string{"bar91"}, "profile": "bar91-23.06.41.80", "architecture": "linux-armv7", "qualification": "hardware-test-pending", "default_action": "check", "setup": "Run the portable jbl-setup executable for your platform. No installation is needed.", "credentials": "Owner adds credentials locally; never upload configured USB files.", "assets": assets}
	data, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		return nil, e
	}
	if e = os.WriteFile(info, append(data, '\n'), 0600); e != nil {
		return nil, e
	}
	files := map[string][]byte{}
	for _, n := range append(append([]string{}, paths...), append(apps, info)...) {
		files[filepath.Base(n)], e = os.ReadFile(n)
		if e != nil {
			return nil, e
		}
	}
	if e = os.WriteFile(sums, manifest(files), 0600); e != nil {
		return nil, e
	}
	return append(append(paths, info, sums), apps...), nil
}
