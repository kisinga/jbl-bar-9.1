package usbsetup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/minio/selfupdate"
	"golang.org/x/mod/semver"
)

const ReleasesURL = "https://api.github.com/repos/kisinga/jbl-bar-toolkit/releases?per_page=20"

var releaseVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}
type Release struct {
	Tag       string            `json:"tag_name"`
	Draft     bool              `json:"draft"`
	Published time.Time         `json:"published_at"`
	Assets    []Asset           `json:"assets"`
	Page      string            `json:"html_url"`
	Checksums map[string]string `json:"-"`
}
type Downloads struct {
	Client *http.Client
	Cache  string
	API    string
}

func NewDownloads() (*Downloads, error) {
	dir, e := os.UserCacheDir()
	if e != nil {
		return nil, e
	}
	return &Downloads{Cache: filepath.Join(dir, "jbl-bar-toolkit"), API: ReleasesURL, Client: &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || !publicDownloadURL(r.URL) {
			return errors.New("unexpected download redirect")
		}
		return nil
	}}}, nil
}
func publicDownloadURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && (u.Host == "github.com" || u.Host == "api.github.com" || strings.HasSuffix(u.Host, ".githubusercontent.com"))
}
func (d *Downloads) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	u, e := url.Parse(address)
	if e != nil {
		return nil, e
	}
	if d.API == ReleasesURL && !publicDownloadURL(u) {
		return nil, errors.New("download is not from GitHub")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", address, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "JBL-Bar-Toolkit-Setup")
	resp, e := d.Client.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("public download returned HTTP %d; retry later or use a downloaded bundle", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, errors.New("download exceeds size limit")
	}
	return b, nil
}
func (d *Downloads) Latest(ctx context.Context) (*Release, error) {
	raw, e := d.get(ctx, d.API, 1<<20)
	if e != nil {
		return nil, e
	}
	var releases []Release
	if e = json.Unmarshal(raw, &releases); e != nil {
		return nil, e
	}
	sort.Slice(releases, func(i, j int) bool { return releases[i].Published.After(releases[j].Published) })
	for _, r := range releases {
		if r.Draft || r.Published.IsZero() || !strings.HasPrefix(r.Tag, "v") || !releaseVersion.MatchString(strings.TrimPrefix(r.Tag, "v")) {
			continue
		}
		var found bool
		for _, a := range r.Assets {
			if a.Name == "jbl-bar-toolkit-"+r.Tag+"-usb.zip" {
				found = true
			}
		}
		if !found {
			continue
		}
		a, e := r.asset("SHA256SUMS")
		if e != nil {
			continue
		}
		data, e := d.get(ctx, a.URL, 1<<20)
		if e != nil {
			return nil, e
		}
		r.Checksums, e = parseChecksums(data)
		if e != nil {
			return nil, e
		}
		return &r, nil
	}
	return nil, errors.New("no published automatic-toolkit release found")
}
func parseChecksums(data []byte) (map[string]string, error) {
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		p := strings.Split(line, "  ")
		if len(p) != 2 || len(p[0]) != 64 || filepath.Base(p[1]) != p[1] || strings.ContainsAny(p[1], `\/`) {
			return nil, errors.New("invalid release checksum manifest")
		}
		if _, e := hex.DecodeString(p[0]); e != nil {
			return nil, e
		}
		if _, ok := m[p[1]]; ok {
			return nil, errors.New("duplicate release checksum")
		}
		m[p[1]] = p[0]
	}
	return m, nil
}
func (r *Release) asset(name string) (Asset, error) {
	var a Asset
	count := 0
	for _, v := range r.Assets {
		if v.Name == name {
			a = v
			count++
		}
	}
	if count != 1 {
		return a, fmt.Errorf("release asset %s is missing or duplicated", name)
	}
	return a, nil
}
func (d *Downloads) Asset(ctx context.Context, r *Release, name string) (string, error) {
	expected, ok := r.Checksums[name]
	if !ok {
		return "", fmt.Errorf("no checksum for %s", name)
	}
	a, e := r.asset(name)
	if e != nil {
		return "", e
	}
	if a.Size > maxPackage {
		return "", errors.New("release asset is too large")
	}
	dir := filepath.Join(d.Cache, r.Tag)
	if !releaseVersion.MatchString(strings.TrimPrefix(r.Tag, "v")) {
		return "", errors.New("invalid release version")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	dest := filepath.Join(dir, name)
	if b, e := os.ReadFile(dest); e == nil {
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) == expected {
			return dest, nil
		}
	}
	b, e := d.get(ctx, a.URL, maxPackage)
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != expected {
		return "", errors.New("download checksum mismatch; nothing was written to USB")
	}
	f, e := os.CreateTemp(dir, ".download-")
	if e != nil {
		return "", e
	}
	temp := f.Name()
	defer os.Remove(temp)
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return "", e
	}
	if closeErr != nil {
		return "", closeErr
	}
	if e = os.Rename(temp, dest); e != nil {
		return "", e
	}
	return dest, nil
}
func (d *Downloads) Toolkit(ctx context.Context, r *Release) (string, string, error) {
	stem := "jbl-bar-toolkit-" + r.Tag
	metadata, e := d.Asset(ctx, r, "release.json")
	if e != nil {
		return "", "", e
	}
	b, e := os.ReadFile(metadata)
	if e != nil {
		return "", "", e
	}
	var meta struct {
		Schema                   int
		Version, Profile, Target string
		Assets                   []string
	}
	if json.Unmarshal(b, &meta) != nil || meta.Schema != 1 || meta.Version != strings.TrimPrefix(r.Tag, "v") || meta.Profile != "bar91-23.06.41.80" || meta.Target != "auto" {
		return "", "", errors.New("release metadata is incompatible")
	}
	p, e := d.Asset(ctx, r, stem+"-usb.zip")
	if e != nil {
		return "", "", e
	}
	payload, e := LoadPackage(p)
	if e != nil {
		return "", "", e
	}
	if payload.Version != meta.Version {
		return "", "", errors.New("package/release version mismatch")
	}
	recovery, e := d.Asset(ctx, r, stem+"-recovery.zip")
	if e == nil {
		e = VerifyRecovery(recovery, payload)
	}
	return p, recovery, e
}
func BinaryName(goos, arch string) string {
	n := "jbl-setup-" + goos + "-" + arch
	if goos == "windows" {
		n += ".exe"
	}
	return n
}
func (d *Downloads) UpdateSelf(ctx context.Context, r *Release, target string) error {
	name := BinaryName(runtime.GOOS, runtime.GOARCH)
	file, e := d.Asset(ctx, r, name)
	if e != nil {
		return e
	}
	b, e := os.ReadFile(file)
	if e != nil {
		return e
	}
	if e = validateExecutable(b, runtime.GOOS, runtime.GOARCH); e != nil {
		return e
	}
	checksum, e := hex.DecodeString(r.Checksums[name])
	if e != nil {
		return e
	}
	return selfupdate.Apply(bytes.NewReader(b), selfupdate.Options{TargetPath: target, Checksum: checksum})
}

func CanUpdate(version string, r *Release) bool {
	if version == "dev" || !semver.IsValid("v"+version) || !semver.IsValid(r.Tag) || semver.Compare(r.Tag, "v"+version) <= 0 {
		return false
	}
	name := BinaryName(runtime.GOOS, runtime.GOARCH)
	_, e := r.asset(name)
	return e == nil && len(r.Checksums[name]) == 64
}

func validateExecutable(data []byte, platform, arch string) error {
	good := false
	switch platform {
	case "linux":
		f, e := elf.NewFile(bytes.NewReader(data))
		if e == nil {
			good = (arch == "amd64" && f.Machine == elf.EM_X86_64) || (arch == "arm64" && f.Machine == elf.EM_AARCH64)
			f.Close()
		}
	case "windows":
		f, e := pe.NewFile(bytes.NewReader(data))
		if e == nil {
			good = (arch == "amd64" && f.Machine == 0x8664) || (arch == "arm64" && f.Machine == 0xaa64)
			f.Close()
		}
	case "darwin":
		f, e := macho.NewFile(bytes.NewReader(data))
		if e == nil {
			good = (arch == "amd64" && f.Cpu == macho.CpuAmd64) || (arch == "arm64" && f.Cpu == macho.CpuArm64)
			f.Close()
		}
	}
	if !good {
		return errors.New("downloaded executable does not match this platform")
	}
	return nil
}

func VerifyRecovery(filename string, p *Package) error {
	expected := map[string][]byte{"JBLToolkit/RECOVER": []byte("RECOVER\n")}
	for _, name := range []string{entry, "JBLToolkit/launch.sh", "JBLToolkit/common.sh", "JBLToolkit/recover.sh", "JBLToolkit/dispatch.sh", "JBLToolkit/BOOTSTRAP.SHA256SUMS", "README.txt", "LICENSE.txt"} {
		expected[name] = p.Files[name]
	}
	f, e := zip.OpenReader(filename)
	if e != nil {
		return e
	}
	defer f.Close()
	if len(f.File) != len(expected) {
		return errors.New("unexpected recovery package contents")
	}
	seen := map[string]bool{}
	for _, item := range f.File {
		want, ok := expected[item.Name]
		if !ok || seen[item.Name] || item.UncompressedSize64 != uint64(len(want)) {
			return errors.New("invalid recovery entry")
		}
		seen[item.Name] = true
		r, e := item.Open()
		if e != nil {
			return e
		}
		data, e := io.ReadAll(io.LimitReader(r, int64(len(want))+1))
		r.Close()
		if e != nil || !bytes.Equal(data, want) {
			return errors.New("recovery package does not match the toolkit bootstrap")
		}
	}
	return nil
}
