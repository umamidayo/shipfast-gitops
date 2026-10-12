// Command status-check is a small internal status API. It periodically runs a
// configured set of HTTP, TCP and DNS checks and serves the aggregated result.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shipfast/status-check/internal/api"
	"github.com/shipfast/status-check/internal/checks"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the local /healthz endpoint and exit (used by Docker HEALTHCHECK)")
	flag.Parse()

	addr := env("LISTEN_ADDR", ":8080")
	if *healthcheck {
		os.Exit(probe(addr))
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(addr); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(addr string) error {
	cfg, err := checks.LoadConfig(env("CHECKS_FILE", "/etc/status-check/checks.json"))
	if err != nil {
		return err
	}
	interval, err := time.ParseDuration(env("CHECK_INTERVAL", "30s"))
	if err != nil || interval <= 0 {
		return fmt.Errorf("invalid CHECK_INTERVAL %q", os.Getenv("CHECK_INTERVAL"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Bind before the first check round so checks against this service itself pass.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	runner := checks.NewRunner(cfg.Checks, interval)
	go runner.Start(ctx)

	srv := &http.Server{
		Handler:           api.New(runner, version),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	slog.Info("listening", "addr", addr, "version", version, "checks", len(cfg.Checks), "interval", interval.String())

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// probe lets the distroless image (which has no curl/wget) health-check itself.
func probe(addr string) int {
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
