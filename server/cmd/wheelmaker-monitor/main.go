// Command wheelmaker-monitor runs a background HTTP server that provides
// a web dashboard for monitoring and managing local WheelMaker services.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "wheelmaker-monitor: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("wheelmaker-monitor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	addr := fs.String("addr", ":9631", "HTTP listen address")
	wmDir := fs.String("dir", "", "WheelMaker home directory (default: ~/.wheelmaker)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	baseDir := *wmDir
	if baseDir == "" {
		baseDir = filepath.Join(home, ".wheelmaker")
	}

	mon := NewMonitor(baseDir)

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", *addr, err)
	}
	fmt.Fprintf(os.Stderr, "wheelmaker-monitor listening on %s\n", ln.Addr())

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}
