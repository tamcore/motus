// Package resetdemo implements the `motus reset-demo` CLI command.
//
// It performs a comprehensive demo environment reset: cleaning all demo-managed
// resources and re-creating them from scratch. This is the same logic used by
// the nightly reset timer and can be used in init containers.
package resetdemo

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/tamcore/motus/internal/config"
	"github.com/tamcore/motus/internal/demo"
)

// NewCmd returns a cobra command for the reset-demo subcommand.
func NewCmd() *cobra.Command {
	var dbURL string

	cmd := &cobra.Command{
		Use:   "reset-demo",
		Short: "Reset the demo environment (cleanup + re-seed)",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.LoadFromEnv()
			if err != nil {
				slog.Error("failed to load config", slog.Any("error", err))
				os.Exit(1)
			}

			// Use explicit DB URL if provided, otherwise fall back to config.
			connURL := dbURL
			if connURL == "" {
				connURL = cfg.Database.URL()
			}
			if connURL == "" {
				slog.Error("database URL required (--db-url, POSTGRES_URI, or MOTUS_DATABASE_* variables)")
				os.Exit(1)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			pool, err := pgxpool.New(ctx, connURL)
			if err != nil {
				slog.Error("failed to connect to database", slog.Any("error", err))
				os.Exit(1)
			}
			defer pool.Close()

			if err := pool.Ping(ctx); err != nil {
				slog.Error("failed to ping database", slog.Any("error", err))
				os.Exit(1)
			}

			result, err := demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
			if err != nil {
				slog.Error("demo reset failed", slog.Any("error", err))
				os.Exit(1)
			}

			demo.LogResult(result)
			fmt.Println("Demo environment reset complete.")
		},
	}

	cmd.Flags().StringVar(&dbURL, "db-url", "", "Database URL (overrides POSTGRES_URI and MOTUS_DATABASE_* env vars)")

	return cmd
}
