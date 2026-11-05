package buildtool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Compiler uses the repository's installed Go tooling. Go itself handles its
// standard toolchain selection; the toolkit never downloads or installs an SDK.
func Compiler(ctx context.Context, root string) (string, error) {
	installed, e := exec.LookPath("go")
	if e != nil {
		return "", fmt.Errorf("source builds require Go; use a platform release executable without developer tools: %w", e)
	}
	data, e := os.ReadFile(filepath.Join(root, "go.mod"))
	if e != nil {
		return "", e
	}
	match := regexp.MustCompile(`(?m)^go ([0-9]+\.[0-9]+\.[0-9]+)$`).FindSubmatch(data)
	if len(match) != 2 {
		return "", fmt.Errorf("go.mod must pin a Go patch version")
	}
	cmd := exec.CommandContext(ctx, installed, "env", "GOROOT")
	cmd.Dir = root
	for _, s := range os.Environ() {
		k, _, _ := strings.Cut(s, "=")
		if k != "GOTOOLCHAIN" && k != "GOROOT" && k != "GOWORK" && k != "GOFLAGS" {
			cmd.Env = append(cmd.Env, s)
		}
	}
	cmd.Env = append(cmd.Env, "GOTOOLCHAIN=go"+string(match[1]), "GOWORK=off", "GOFLAGS=")
	output, e := cmd.Output()
	if e != nil {
		return "", fmt.Errorf("select repository Go toolchain: %w", e)
	}
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(strings.TrimSpace(string(output)), "bin", name), nil
}
