package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishDirectoryNoReplacementOrMerge(t *testing.T) {
	for _, existing := range []string{"absent", "empty-directory", "directory", "file", "symlink", "dangling-symlink"} {
		t.Run(existing, func(t *testing.T) {
			parent := t.TempDir()
			target := filepath.Join(parent, "store")
			stage := target + ".stage.fixture"
			if err := os.Mkdir(stage, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stage, "OWNER.txt"), []byte("owned"), 0600); err != nil {
				t.Fatal(err)
			}
			switch existing {
			case "empty-directory", "directory":
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if existing == "directory" {
					os.WriteFile(filepath.Join(target, "sentinel"), []byte("preserved"), 0600)
				}
			case "file":
				os.WriteFile(target, []byte("preserved"), 0600)
			case "symlink":
				os.Symlink(stage, target)
			case "dangling-symlink":
				os.Symlink(filepath.Join(parent, "absent"), target)
			}
			err := publishDirectory(stage, target)
			if existing == "absent" {
				if err != nil {
					t.Fatal(err)
				}
				if b, err := os.ReadFile(filepath.Join(target, "OWNER.txt")); err != nil || string(b) != "owned" {
					t.Fatalf("lost source: %q %v", b, err)
				}
				if _, err := os.Lstat(stage); !os.IsNotExist(err) {
					t.Fatalf("stage still exists: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("existing entry replaced")
				}
				if _, err := os.Stat(filepath.Join(stage, "OWNER.txt")); err != nil {
					t.Fatal("stage was moved or merged", err)
				}
				if existing == "directory" {
					if b, err := os.ReadFile(filepath.Join(target, "sentinel")); err != nil || string(b) != "preserved" {
						t.Fatal("changed destination")
					}
				}
				if existing == "file" {
					if b, err := os.ReadFile(target); err != nil || string(b) != "preserved" {
						t.Fatal("changed destination file")
					}
				}
			}
		})
	}
}

func TestPublishDirectoryRejectsUnsafeSources(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "store")
	for _, source := range []string{"relative", filepath.Join(parent, "unrelated"), target + ".stage."} {
		if err := publishDirectory(source, target); err == nil {
			t.Fatalf("accepted %s", source)
		}
	}
	real := filepath.Join(parent, "real")
	os.Mkdir(real, 0700)
	for _, kind := range []string{"symlink", "file"} {
		stage := target + ".stage." + kind
		if kind == "symlink" {
			os.Symlink(real, stage)
		} else {
			os.WriteFile(stage, []byte("x"), 0600)
		}
		if err := publishDirectory(stage, target); err == nil {
			t.Fatalf("accepted %s", kind)
		}
	}
}
