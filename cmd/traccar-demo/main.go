// traccar-demo connects a single demo device to an external Traccar server
// and streams H02 position messages from a GPX route directory.
// Usage: go run ./cmd/traccar-demo [host:port] [imei] [gpx-dir]
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tamcore/motus/internal/demo"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
}

func main() {
	target := "localhost:5013"
	imei := "9000000000001"
	dir := "data/demo"

	if len(os.Args) > 1 {
		target = os.Args[1]
	}
	if len(os.Args) > 2 {
		imei = os.Args[2]
	}
	if len(os.Args) > 3 {
		dir = os.Args[3]
	}

	routes, err := demo.LoadRoutes(dir)
	if err != nil {
		slog.Error("failed to load GPX routes", slog.Any("error", err))
		os.Exit(1)
	}

	slog.Info("starting traccar demo",
		slog.String("target", target),
		slog.String("imei", imei),
		slog.Int("routes", len(routes)))

	sim := demo.NewSimulator(routes, target, []string{imei}, 30.0)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sim.Start(ctx)
	slog.Info("simulator stopped")
}
