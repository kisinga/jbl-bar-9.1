package buildtool

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

// CLI is the repository/CI entry point for the shared Go builder.
func CLI(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	root := fs.String("repo", ".", "Source checkout")
	output := fs.String("output-dir", "", "Empty output directory under build/ or dist/")
	model := fs.String("model", "auto", "auto or bar91")
	payload := fs.Bool("payload-only", false, "Build USB and recovery only")
	apps := fs.Bool("apps-only", false, "Build six setup apps and offline bundles")
	pack := fs.String("package", "", "USB ZIP for --apps-only")
	recovery := fs.String("recovery", "", "Recovery ZIP for --apps-only")
	version := fs.String("version", "", "Version for --apps-only")
	matrix := fs.Bool("matrix", false, "Print CI target matrix")
	licenses := fs.Bool("write-licenses", false, "Regenerate embedded dependency licenses")
	machine := fs.Bool("json", false, "Print built paths as JSON")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected build arguments")
	}
	if *matrix {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"include": []map[string]string{{"model": "auto", "artifact": "jbl-bar-toolkit-usb-development"}, {"model": "bar91", "artifact": "jbl-bar91-usb-development"}}})
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	b, e := New(ctx, *root)
	if e != nil {
		return e
	}
	if *licenses {
		return b.WriteLicenses(ctx)
	}
	if *output == "" {
		*output = filepath.Join(b.Root, "dist/release")
	}
	var paths []string
	if *apps {
		if *pack == "" || *recovery == "" {
			return fmt.Errorf("--apps-only requires package and recovery")
		}
		if *version == "" {
			*version, e = b.Version()
			if e != nil {
				return e
			}
		}
		paths, e = b.Applications(ctx, *output, *version, *pack, *recovery)
	} else if *payload {
		paths, e = b.Payload(ctx, *output, *model)
	} else {
		paths, e = b.Distribution(ctx, *output, *model)
	}
	if e != nil {
		return e
	}
	if *machine {
		return json.NewEncoder(os.Stdout).Encode(paths)
	}
	for _, n := range paths {
		fmt.Println(n)
	}
	return nil
}
