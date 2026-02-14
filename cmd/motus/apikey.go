package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

func newUserKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Manage API keys on behalf of a user",
	}
	cmd.AddCommand(
		newUserKeysListCmd(),
		newUserKeysAddCmd(),
		newUserKeysDeleteCmd(),
	)
	return cmd
}

func newUserKeysListCmd() *cobra.Command {
	var email, output string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List API keys for a user",
		Run: func(cmd *cobra.Command, args []string) {
			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			userRepo := repository.NewUserRepository(pool)
			apiKeyRepo := repository.NewApiKeyRepository(pool)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			u, err := userRepo.GetByEmail(ctx, email)
			if err != nil {
				fatal("user not found", slog.String("email", email))
			}

			keys, err := apiKeyRepo.ListByUser(ctx, u.ID)
			if err != nil {
				fatal("failed to list API keys", slog.Any("error", err))
			}

			if len(keys) == 0 {
				fmt.Printf("No API keys for %s.\n", email)
				return
			}

			switch output {
			case "json":
				items := make([]map[string]interface{}, len(keys))
				for i, k := range keys {
					item := map[string]interface{}{
						"id":          k.ID,
						"name":        k.Name,
						"permissions": k.Permissions,
						"createdAt":   k.CreatedAt.Format(time.RFC3339),
					}
					if k.ExpiresAt != nil {
						item["expiresAt"] = k.ExpiresAt.Format(time.RFC3339)
					}
					if k.LastUsedAt != nil {
						item["lastUsedAt"] = k.LastUsedAt.Format(time.RFC3339)
					}
					items[i] = item
				}
				printJSON(items)
			case "csv":
				headers := []string{"ID", "Name", "Permissions", "ExpiresAt", "LastUsedAt", "CreatedAt"}
				rows := make([][]string, len(keys))
				for i, k := range keys {
					expiresAt := "never"
					if k.ExpiresAt != nil {
						expiresAt = k.ExpiresAt.Format("2006-01-02")
					}
					lastUsed := "-"
					if k.LastUsedAt != nil {
						lastUsed = k.LastUsedAt.Format("2006-01-02 15:04")
					}
					rows[i] = []string{
						fmt.Sprint(k.ID), k.Name, k.Permissions,
						expiresAt, lastUsed, k.CreatedAt.Format("2006-01-02"),
					}
				}
				printCSV(headers, rows)
			default:
				tw := NewTableWriter(os.Stdout)
				tw.WriteHeader("ID", "NAME", "PERMISSIONS", "EXPIRES", "LAST USED", "CREATED")
				for _, k := range keys {
					expiresAt := "never"
					if k.ExpiresAt != nil {
						expiresAt = k.ExpiresAt.Format("2006-01-02")
					}
					lastUsed := "-"
					if k.LastUsedAt != nil {
						lastUsed = k.LastUsedAt.Format("2006-01-02 15:04")
					}
					tw.WriteRow(fmt.Sprint(k.ID), k.Name, k.Permissions,
						expiresAt, lastUsed, k.CreatedAt.Format("2006-01-02"))
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

func newUserKeysAddCmd() *cobra.Command {
	var email, name, permissions string
	var expiresIn int

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create an API key for a user",
		Run: func(cmd *cobra.Command, args []string) {
			if !model.IsValidPermission(permissions) {
				fmt.Fprintf(os.Stderr, "Error: invalid permissions %q (must be full or readonly)\n", permissions)
				os.Exit(1)
			}

			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			userRepo := repository.NewUserRepository(pool)
			apiKeyRepo := repository.NewApiKeyRepository(pool)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			u, err := userRepo.GetByEmail(ctx, email)
			if err != nil {
				fatal("user not found", slog.String("email", email))
			}

			key := &model.ApiKey{
				UserID:      u.ID,
				Name:        name,
				Permissions: permissions,
			}
			if expiresIn > 0 {
				t := time.Now().Add(time.Duration(expiresIn) * time.Hour)
				key.ExpiresAt = &t
			}

			if err := apiKeyRepo.Create(ctx, key); err != nil {
				fatal("failed to create API key", slog.Any("error", err))
			}

			fmt.Printf("Created API key for %s:\n", email)
			fmt.Printf("  ID:          %d\n", key.ID)
			fmt.Printf("  Name:        %s\n", key.Name)
			fmt.Printf("  Permissions: %s\n", key.Permissions)
			if key.ExpiresAt != nil {
				fmt.Printf("  Expires:     %s\n", key.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC"))
			} else {
				fmt.Printf("  Expires:     never\n")
			}
			fmt.Printf("  Token:       %s\n", key.Token)
			fmt.Println("  (Store the token securely — it will not be shown again.)")
		},
	}

	f := cmd.Flags()
	f.StringVar(&email, "email", "", "User email")
	f.StringVar(&name, "name", "", "Key name")
	f.StringVar(&permissions, "permissions", "full", "Permissions: full, readonly")
	f.IntVar(&expiresIn, "expires-in", 0, "Hours until expiration (0 = no expiration)")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newUserKeysDeleteCmd() *cobra.Command {
	var id int64

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an API key by ID",
		Run: func(cmd *cobra.Command, args []string) {
			if id <= 0 {
				fmt.Fprintln(os.Stderr, "Error: --id must be a positive integer")
				os.Exit(1)
			}

			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			apiKeyRepo := repository.NewApiKeyRepository(pool)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := apiKeyRepo.Delete(ctx, id); err != nil {
				fatal("failed to delete API key", slog.Any("error", err))
			}

			fmt.Printf("Deleted API key: id=%d\n", id)
		},
	}

	cmd.Flags().Int64Var(&id, "id", 0, "API key ID to delete")
	_ = cmd.MarkFlagRequired("id")

	return cmd
}
