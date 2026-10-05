// Command motus is the unified CLI for the Motus GPS tracking system.
//
// Usage:
//
//	motus <command> [flags]
//
// Run "motus --help" or "motus <command> --help" for usage details.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/tamcore/motus/cmd/import/traccarimport"
	"github.com/tamcore/motus/cmd/replay/gpsreplay"
	"github.com/tamcore/motus/cmd/resetdemo"
	"github.com/tamcore/motus/cmd/server/serve"
	"github.com/tamcore/motus/internal/config"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/version"
)

func main() {
	root := &cobra.Command{
		Use:          "motus",
		Short:        "Motus GPS Tracking System",
		SilenceUsage: true,
	}

	root.AddCommand(
		newServeCmd(),
		traccarimport.NewCmd(),
		gpsreplay.NewCmd(),
		newUserCmd(),
		newDeviceCmd(),
		resetdemo.NewCmd(),
		newDBMigrateCmd(),
		newWaitForDBCmd(),
		newVersionCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP API and GPS protocol servers",
		Run:   func(cmd *cobra.Command, args []string) { serve.Run() },
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("motus %s (commit: %s, built: %s, branch: %s)\n",
				version.Version, version.Commit, version.BuildDate, version.Branch)
		},
	}
}

// fatal logs a structured error and terminates the process.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

// fatalFn is the fatal function used by filter helpers; overridable in tests.
var fatalFn = fatal

// connectDBFn is the DB connection factory; overridable in tests.
var connectDBFn = connectDB

// connectDB opens a pgxpool connection using environment configuration.
func connectDB() (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return repository.Connect(ctx, dbURLOrConfig(""))
}

// dbURLOrConfig returns flagURL, or the database URL from the environment config.
func dbURLOrConfig(flagURL string) string {
	if flagURL != "" {
		return flagURL
	}
	return loadConfig().Database.URL()
}

// loadConfig loads the environment configuration, exiting on error.
func loadConfig() *config.Config {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		fatal("failed to load config", slog.Any("error", err))
	}
	return cfg
}

// withDB runs fn with a connected pool and a 10s context, exiting on connection failure.
func withDB(fn func(ctx context.Context, pool *pgxpool.Pool)) {
	pool, err := connectDBFn()
	if err != nil {
		fatal("database connection failed", slog.Any("error", err))
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fn(ctx, pool)
}
