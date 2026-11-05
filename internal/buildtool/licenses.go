package buildtool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WriteLicenses refreshes the notices embedded in every portable executable.
func (b *Builder) WriteLicenses(ctx context.Context) error {
	if err := b.run(ctx, b.Root, nil, "mod", "download", "all"); err != nil {
		return err
	}
	data, err := b.command(ctx, b.Root, nil, "list", "-m", "-json", "all").Output()
	if err != nil {
		return err
	}
	type module struct {
		Path, Version, Dir string
		Main               bool
	}
	var modules []module
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var m module
		if err = decoder.Decode(&m); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if !m.Main {
			modules = append(modules, m)
		}
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Path < modules[j].Path })
	project, err := os.ReadFile(filepath.Join(b.Root, "LICENSE"))
	if err != nil {
		return err
	}
	runtime, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(b.Go)), "LICENSE"))
	if err != nil {
		return err
	}
	nativeNotice, err := b.read("models/bar91/remote/native/bin/NOTICE.txt")
	if err != nil {
		return err
	}
	parts := []string{"JBL Bar Toolkit\n" + string(project), "Third-party software included in the portable setup executable.\n", "Go runtime\n" + string(runtime), string(nativeNotice)}
	for _, m := range modules {
		entries, err := os.ReadDir(m.Dir)
		if err != nil {
			return err
		}
		parts = append(parts, "\n=== "+m.Path+" "+m.Version+" ===\n")
		count := 0
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			base, _, _ := strings.Cut(strings.ToLower(entry.Name()), ".")
			switch base {
			case "license", "licence", "copying", "notice":
			default:
				continue
			}
			data, err := os.ReadFile(filepath.Join(m.Dir, entry.Name()))
			if err != nil {
				return err
			}
			count++
			parts = append(parts, entry.Name()+"\n"+strings.ToValidUTF8(string(data), "\uFFFD"))
		}
		if count == 0 {
			return fmt.Errorf("review dependency licensing: %s", m.Path)
		}
	}
	lines := strings.Split(strings.TrimSuffix(strings.Join(parts, "\n"), "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return os.WriteFile(filepath.Join(b.Root, "packaging/wizard/licenses.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
