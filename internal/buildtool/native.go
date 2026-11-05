package buildtool

import (
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type nativeManifest struct {
	Schema     int    `json:"schema"`
	Profile    string `json:"profile"`
	NoticeHash string `json:"notice_sha256"`
	Helpers    map[string]struct {
		Source       string            `json:"source"`
		SourceHash   string            `json:"source_sha256"`
		Builder      string            `json:"builder"`
		BuilderHash  string            `json:"builder_sha256"`
		Hash         string            `json:"sha256"`
		Dependencies map[string]string `json:"dependencies"`
	} `json:"helpers"`
}

// Native binaries contain only project code and dynamically use pinned on-device
// libraries. Source/dependency hashes prevent stale helper builds from shipping.
func (b *Builder) nativeHelpers(p profile) (bluetooth, equalizer []byte, err error) {
	const folder = "models/bar91/remote/native/"
	data, err := b.read(folder + "bin/manifest.json")
	if err != nil {
		return nil, nil, err
	}
	var manifest nativeManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return nil, nil, err
	}
	if manifest.Schema != 1 || manifest.Profile != p.ID || len(manifest.Helpers) != 2 {
		return nil, nil, errors.New("unsupported native helper manifest")
	}
	notice, err := b.read(folder + "bin/NOTICE.txt")
	if err != nil {
		return nil, nil, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(notice)) != manifest.NoticeHash {
		return nil, nil, errors.New("native helper notices changed")
	}
	for _, spec := range [][4]string{{"bt-remote", "bt_remote.c", "tools/build_bt.py", "bluetooth"}, {"eq-parser", "eq_parser.c", "tools/build_eq.py", "eq_parser"}} {
		item, ok := manifest.Helpers[spec[0]]
		if !ok || item.Source != folder+spec[1] || item.Builder != spec[2] || !reflect.DeepEqual(item.Dependencies, p.Helpers[spec[3]]) {
			return nil, nil, errors.New("native helper source/dependency mismatch")
		}
		var binary []byte
		for _, file := range []struct{ path, hash string }{{item.Source, item.SourceHash}, {item.Builder, item.BuilderHash}, {folder + "bin/" + spec[0], item.Hash}} {
			data, err = b.read(file.path)
			if err != nil {
				return nil, nil, err
			}
			if fmt.Sprintf("%x", sha256.Sum256(data)) != file.hash {
				return nil, nil, fmt.Errorf("native helper changed: %s; rebuild and review", file.path)
			}
			binary = data
		}
		if err = validateNativeHelper(binary); err != nil {
			return nil, nil, err
		}
		if spec[0] == "bt-remote" {
			bluetooth = binary
		} else {
			equalizer = binary
		}
	}
	return bluetooth, equalizer, nil
}

func validateNativeHelper(data []byte) error {
	if len(data) < 52 || len(data) > 65536 {
		return errors.New("native helper size outside bounds")
	}
	file, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer file.Close()
	if file.Class != elf.ELFCLASS32 || file.Data != elf.ELFDATA2LSB || file.Machine != elf.EM_ARM || file.Type != elf.ET_EXEC {
		return errors.New("native helper must be ARM32 Linux executable")
	}
	for _, p := range file.Progs {
		if p.Type == elf.PT_INTERP {
			name, err := io.ReadAll(io.LimitReader(p.Open(), 128))
			if err == nil && string(name) == "/system/bdplib/ld-linux.so.3\x00" {
				return nil
			}
		}
	}
	return errors.New("native helper does not use the reviewed on-device loader")
}

// An overlay embeds the helper strings without modifying the checkout or
// exceeding Windows' process command-line limit with two base64 -X arguments.
func (b *Builder) nativeOverlay(work string, bluetooth, equalizer []byte) (string, error) {
	replacements := map[string]string{}
	for _, item := range []struct {
		file, name string
		data       []byte
	}{{"bt_rpc.go", "bluetoothPayload", bluetooth}, {"eq.go", "eqParserPayload", equalizer}} {
		source := filepath.Join(b.Root, "apps/remote", item.file)
		code, err := os.ReadFile(source)
		if err != nil {
			return "", err
		}
		declaration := "var " + item.name + " string"
		if strings.Count(string(code), declaration) != 1 {
			return "", fmt.Errorf("native payload declaration changed: %s", item.file)
		}
		value, err := json.Marshal(base64.StdEncoding.EncodeToString(item.data))
		if err != nil {
			return "", err
		}
		generated := strings.Replace(string(code), declaration, "var "+item.name+" = "+string(value), 1)
		target := filepath.Join(work, item.file)
		if err = os.WriteFile(target, []byte(generated), 0600); err != nil {
			return "", err
		}
		replacements[source] = target
	}
	data, err := json.Marshal(map[string]any{"Replace": replacements})
	if err != nil {
		return "", err
	}
	path := filepath.Join(work, "native-overlay.json")
	return path, os.WriteFile(path, data, 0600)
}
