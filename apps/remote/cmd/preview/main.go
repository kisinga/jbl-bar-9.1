// Preview is a loopback-only development composition. It never probes a bar.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jbl-bar-toolkit/remote/internal/preview"
	"jbl-bar-toolkit/remote/internal/server"
	webui "jbl-bar-toolkit/remote/www"
)

func main() {
	address := flag.String("listen", "127.0.0.1:8080", "loopback development address")
	fixture := flag.String("fixture", "", "optional normalized JSON snapshot; disables simulated actions")
	flag.Parse()
	host, _, err := net.SplitHostPort(*address)
	if err != nil || host != "127.0.0.1" {
		log.Fatal("preview must listen on 127.0.0.1:<port>")
	}
	var input *os.File
	if *fixture != "" {
		input, err = os.Open(*fixture)
		if err != nil {
			log.Fatal(err)
		}
		defer input.Close()
	}
	var backend *preview.Backend
	if input == nil {
		backend, err = preview.New(nil)
	} else {
		backend, err = preview.New(input)
	}
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app, err := server.New[preview.State](backend, server.Options[preview.State]{AllowedCIDR: "127.0.0.1/32", Assets: webui.Files, Done: ctx.Done})
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		log.Fatal(err)
	}
	httpServer := &http.Server{Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	go app.Run(ctx)
	shutdown := make(chan struct{})
	go func() {
		defer close(shutdown)
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(c)
	}()
	fmt.Printf("Preview: http://%s (simulated data; no hardware access)\n", listener.Addr())
	if err = httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		stop()
		log.Print(err)
	}
	stop()
	<-shutdown
}
