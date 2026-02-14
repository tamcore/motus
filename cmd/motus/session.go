package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tamcore/motus/internal/storage/repository"
)

func newUserSessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Manage sessions on behalf of a user",
	}
	cmd.AddCommand(
		newUserSessionsListCmd(),
		newUserSessionsRevokeCmd(),
	)
	return cmd
}

func newUserSessionsListCmd() *cobra.Command {
	var email, output string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List active sessions for a user",
		Run: func(cmd *cobra.Command, args []string) {
			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			userRepo := repository.NewUserRepository(pool)
			sessionRepo := repository.NewSessionRepository(pool)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			u, err := userRepo.GetByEmail(ctx, email)
			if err != nil {
				fatal("user not found", slog.String("email", email))
			}

			sessions, err := sessionRepo.ListByUser(ctx, u.ID)
			if err != nil {
				fatal("failed to list sessions", slog.Any("error", err))
			}

			if len(sessions) == 0 {
				fmt.Printf("No active sessions for %s.\n", email)
				return
			}

			switch output {
			case "json":
				items := make([]map[string]interface{}, len(sessions))
				for i, s := range sessions {
					item := map[string]interface{}{
						"id":         s.ID,
						"rememberMe": s.RememberMe,
						"isSudo":     s.IsSudo,
						"createdAt":  s.CreatedAt.Format(time.RFC3339),
						"expiresAt":  s.ExpiresAt.Format(time.RFC3339),
					}
					if s.ApiKeyName != nil {
						item["apiKeyName"] = *s.ApiKeyName
					}
					if s.ApiKeyID != nil {
						item["apiKeyId"] = *s.ApiKeyID
					}
					items[i] = item
				}
				printJSON(items)
			case "csv":
				headers := []string{"ID", "API KEY", "CREATED", "EXPIRES", "REMEMBER", "SUDO"}
				rows := make([][]string, len(sessions))
				for i, s := range sessions {
					apiKey := "-"
					if s.ApiKeyName != nil {
						apiKey = *s.ApiKeyName
					}
					rows[i] = []string{
						truncateID(s.ID),
						apiKey,
						s.CreatedAt.Format("2006-01-02 15:04"),
						s.ExpiresAt.Format("2006-01-02 15:04"),
						fmt.Sprint(s.RememberMe),
						fmt.Sprint(s.IsSudo),
					}
				}
				printCSV(headers, rows)
			default:
				tw := NewTableWriter(os.Stdout)
				tw.WriteHeader("ID", "API KEY", "CREATED", "EXPIRES", "REMEMBER", "SUDO")
				for _, s := range sessions {
					apiKey := "-"
					if s.ApiKeyName != nil {
						apiKey = *s.ApiKeyName
					}
					tw.WriteRow(
						truncateID(s.ID),
						apiKey,
						s.CreatedAt.Format("2006-01-02 15:04"),
						s.ExpiresAt.Format("2006-01-02 15:04"),
						fmt.Sprint(s.RememberMe),
						fmt.Sprint(s.IsSudo),
					)
				}
				tw.Flush()
			}
		},
	}

	f := cmd.Flags()
	f.StringVar(&email, "email", "", "User email")
	f.StringVar(&output, "output", "table", "Output format: table, json, csv")
	_ = cmd.MarkFlagRequired("email")

	return cmd
}

func newUserSessionsRevokeCmd() *cobra.Command {
	var id string

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke a session by ID",
		Run: func(cmd *cobra.Command, args []string) {
			if id == "" {
				fmt.Fprintln(os.Stderr, "Error: --id is required")
				os.Exit(1)
			}

			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			sessionRepo := repository.NewSessionRepository(pool)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := sessionRepo.Delete(ctx, id); err != nil {
				fatal("failed to revoke session", slog.Any("error", err))
			}

			fmt.Printf("Revoked session: %s\n", truncateID(id))
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "Session ID to revoke")
	_ = cmd.MarkFlagRequired("id")

	return cmd
}

// truncateID returns the first 12 characters of a session ID for display.
func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12] + "..."
	}
	return id
}
