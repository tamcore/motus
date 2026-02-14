package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/spf13/cobra"
	"github.com/tamcore/motus/internal/config"
	"github.com/tamcore/motus/migrations"
)

func newDBMigrateCmd() *cobra.Command {
	var dbURL string

	cmd := &cobra.Command{
		Use:   "db-migrate [command]",
		Short: "Run database migrations",
		Long: `Run goose database migrations against the configured database.

The optional [command] argument is passed directly to goose (default: up).
Available goose commands: up, up-one, down, down-to, redo, status, version, reset.`,
		Args: cobra.MaximumNArgs(1),
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

			db, err := sql.Open("pgx", dbURL)
			if err != nil {
				slog.Error("failed to open database", slog.Any("error", err))
				os.Exit(1)
			}
			defer func() { _ = db.Close() }()

			if err := db.Ping(); err != nil {
				slog.Error("failed to ping database", slog.Any("error", err))
				os.Exit(1)
			}

			goose.SetBaseFS(migrations.FS)
			if err := goose.SetDialect("postgres"); err != nil {
				slog.Error("failed to set dialect", slog.Any("error", err))
				os.Exit(1)
			}

			command := "up"
			if len(args) > 0 {
				command = args[0]
			}

			slog.Info("running goose migration", slog.String("command", command))
			if err := goose.RunContext(context.Background(), command, db, "."); err != nil {
				slog.Error("goose migration failed", slog.String("command", command), slog.Any("error", err))
				os.Exit(1)
			}
			slog.Info("goose migration completed", slog.String("command", command))
		},
	}

	cmd.Flags().StringVar(&dbURL, "db-url", "", "Database URL (overrides POSTGRES_URI and MOTUS_DATABASE_* env vars)")

	return cmd
}
