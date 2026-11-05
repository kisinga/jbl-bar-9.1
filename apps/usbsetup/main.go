// Portable local USB setup. Run from the repo: go run ./apps/usbsetup --repo .
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/pkg/browser"
	"jbl-bar-toolkit/internal/buildtool"
	"jbl-bar-toolkit/internal/usbsetup"
	setupassets "jbl-bar-toolkit/packaging"
)

var version = "dev"

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "Setup:", e)
		os.Exit(1)
	}
}
func run() error {

	repo := flag.String("repo", "", "Build USB files from this checkout")
	pack := flag.String("package", "", "Use a local toolkit USB ZIP (offline)")
	noOpen := flag.Bool("no-open", false, "Print the local interface URL without opening a browser")
	list := flag.Bool("list", false, "List removable USB drives as JSON; never modify them")
	job := flag.String("run-job", "", "Internal elevated formatting job")
	digest := flag.String("job-hash", "", "Internal job checksum")
	licenses := flag.Bool("licenses", false, "Print bundled third-party licenses")
	showVersion := flag.Bool("version", false, "Print setup version")
	flag.Parse()
	if *licenses {
		data, err := setupassets.Files.ReadFile("wizard/licenses.txt")
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	}
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if *job != "" {
		return usbsetup.RunFormatJob(*job, *digest)
	}
	if *list {
		drives, e := usbsetup.Discover(context.Background())
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(drives)
	}
	if runtime.GOOS != "windows" && os.Geteuid() == 0 {
		return fmt.Errorf("run setup as your normal user; formatting requests administrator permission separately")
	}
	if version == "dev" && *repo == "" && *pack == "" {
		if _, err := os.Stat("apps/usbsetup/main.go"); err == nil {
			*repo = "."
		}
	}
	if *repo != "" && *pack == "" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		builder, err := buildtool.New(ctx, *repo)
		if err != nil {
			return err
		}
		base := filepath.Join(builder.Root, "build")
		if err = os.MkdirAll(base, 0700); err != nil {
			return err
		}
		work, err := os.MkdirTemp(base, "setup-payload-")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Building USB and recovery files from current source…")
		paths, err := builder.Payload(ctx, filepath.Join(work, "payload"), "auto")
		if err != nil {
			os.RemoveAll(work)
			return err
		}
		fmt.Fprintln(os.Stderr, "Built files:", work)
		*pack = paths[0]
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	exe, e = filepath.EvalSymlinks(exe)
	if e != nil {
		return e
	}
	if *pack == "" {
		candidate := filepath.Join(filepath.Dir(exe), "toolkit-usb.zip")
		if _, e = os.Stat(candidate); e == nil {
			*pack = candidate
		}
	}
	if *pack != "" {
		*pack, e = filepath.Abs(*pack)
		if e != nil {
			return e
		}
	}
	server, e := usbsetup.NewServer(version, exe, *pack)
	if e != nil {
		return e
	}
	server.LocalSource = *repo != ""
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return e
	}
	server.Origin = "http://" + listener.Addr().String()
	address := server.Origin + "/#" + server.Token
	httpServer := &http.Server{Handler: server, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 6 * time.Minute, IdleTimeout: 60 * time.Second, ErrorLog: log.New(os.Stderr, "Setup: ", 0)}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdown)
	}()
	fmt.Printf("JBL Bar setup %s\n%s\nKeep this program running while using the interface. Ctrl+C closes it.\n", version, address)
	if !*noOpen {
		if e = browser.OpenURL(address); e != nil {
			fmt.Println("Open the local link above in your browser.")
		}
	}
	e = httpServer.Serve(listener)
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
