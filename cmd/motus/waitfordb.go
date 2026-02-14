package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
	"github.com/tamcore/motus/internal/config"
)

func newWaitForDBCmd() *cobra.Command {
	var dbURL string
	var timeout, interval time.Duration

	cmd := &cobra.Command{
		Use:   "wait-for-db",
		Short: "Wait until the database is reachable",
		Run: func(cmd *cobra.Command, args []string) {
			if dbURL == "" {
				cfg, err := config.LoadFromEnv()
				if err != nil {
					slog.Error("failed to load config", slog.Any("error", err))
					os.Exit(1)
				}
				dbURL = cfg.Database.URL()
			}
			if dbURL == "" {
				slog.Error("database URL required (--db-url, POSTGRES_URI, or MOTUS_DATABASE_* env vars)")
				os.Exit(1)
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			slog.Info("waiting for database")

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			attempt := 0
			for {
				attempt++
				conn, err := pgx.Connect(ctx, dbURL)
				if err == nil {
					if pingErr := conn.Ping(ctx); pingErr == nil {
						_ = conn.Close(ctx)
						slog.Info("database is ready", slog.Int("attempts", attempt))
						return
					}
					_ = conn.Close(ctx)
				}

				select {
				case <-ctx.Done():
					slog.Error("timeout waiting for database", slog.Int("attempts", attempt))
					os.Exit(1)
				case <-ticker.C:
					slog.Info("database not ready, waiting", slog.Int("attempt", attempt))
				}
			}
		},
	}

	f := cmd.Flags()
	f.StringVar(&dbURL, "db-url", "", "Database URL (overrides POSTGRES_URI and MOTUS_DATABASE_* env vars)")
	f.DurationVar(&timeout, "timeout", 5*time.Minute, "Maximum time to wait")
	f.DurationVar(&interval, "interval", 2*time.Second, "Check interval")

	return cmd
}
