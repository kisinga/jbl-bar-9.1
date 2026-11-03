// Inspect collects two fixed community-observed getters, without installing
// anything or changing audio settings. Raw output is private research evidence.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"jbl-bar-toolkit/remote/internal/jblone"
)

func run(args []string) error {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	host := flags.String("host", "", "explicit private IPv4 address of the soundbar")
	output := flags.String("output-dir", "", "new private directory for report and UI fixture")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *output == "" {
		return fmt.Errorf("provide --host <bar-ip> and --output-dir <new-private-directory>")
	}
	client, err := jblone.NewClient(*host)
	if err != nil {
		return err
	}
	// Never merge captures into an existing session, or follow its symlink.
	if err := os.MkdirAll(filepath.Dir(*output), 0700); err != nil {
		return err
	}
	if err := os.Mkdir(*output, 0700); err != nil {
		return fmt.Errorf("need a new output directory: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	report := client.Inspect(ctx)
	for _, file := range []struct {
		name string
		data any
	}{{"report.json", report}, {"remote-state.json", report.Fixture()}} {
		data, err := json.MarshalIndent(file.data, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*output, file.name), append(data, '\n'), 0600); err != nil {
			return err
		}
	}
	failures := 0
	for _, observation := range report.Observations {
		status := "response captured; compare with the bar before trusting its meaning"
		if observation.Error != "" {
			failures++
			status = observation.Error
		}
		fmt.Println(observation.Action + ": " + status)
	}
	fmt.Println("Private reports:", *output)
	fmt.Println("Replay the snapshot: ./jbl dev --fixture", filepath.Join(*output, "remote-state.json"))
	if failures > 0 {
		return fmt.Errorf("%d of 2 getters failed; partial reports were saved; model support remains undetermined", failures)
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
