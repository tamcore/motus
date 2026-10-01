package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				u, err := repository.NewUserRepository(pool).GetByEmail(ctx, email)
				if err != nil {
					fatal("user not found", slog.String("email", email))
				}

				keys, err := repository.NewApiKeyRepository(pool).ListByUser(ctx, u.ID)
				if err != nil {
					fatal("failed to list API keys", slog.Any("error", err))
				}

				if len(keys) == 0 {
					fmt.Printf("No API keys for %s.\n", email)
					return
				}

				items := make([]map[string]any, len(keys))
				rows := make([][]string, len(keys))
				for i, k := range keys {
					item := map[string]any{
						"id":          k.ID,
						"name":        k.Name,
						"permissions": k.Permissions,
						"createdAt":   k.CreatedAt.Format(time.RFC3339),
					}
					expiresAt := "never"
					if k.ExpiresAt != nil {
						item["expiresAt"] = k.ExpiresAt.Format(time.RFC3339)
						expiresAt = k.ExpiresAt.Format("2006-01-02")
					}
					lastUsed := "-"
					if k.LastUsedAt != nil {
						item["lastUsedAt"] = k.LastUsedAt.Format(time.RFC3339)
						lastUsed = k.LastUsedAt.Format("2006-01-02 15:04")
					}
					items[i] = item
					rows[i] = []string{
						fmt.Sprint(k.ID), k.Name, k.Permissions,
						expiresAt, lastUsed, k.CreatedAt.Format("2006-01-02"),
					}
				}
				headers := []string{"ID", "NAME", "PERMISSIONS", "EXPIRES", "LAST USED", "CREATED"}
				if output == "csv" {
					headers = []string{"ID", "Name", "Permissions", "ExpiresAt", "LastUsedAt", "CreatedAt"}
				}
				render(output, items, headers, rows)
			})
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

			key := &model.ApiKey{Name: name, Permissions: permissions}
			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				u, err := repository.NewUserRepository(pool).GetByEmail(ctx, email)
				if err != nil {
					fatal("user not found", slog.String("email", email))
				}
				key.UserID = u.ID
				if expiresIn > 0 {
					key.ExpiresAt = new(time.Now().Add(time.Duration(expiresIn) * time.Hour))
				}
				if err := repository.NewApiKeyRepository(pool).Create(ctx, key); err != nil {
					fatal("failed to create API key", slog.Any("error", err))
				}
			})

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

			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				if err := repository.NewApiKeyRepository(pool).Delete(ctx, id); err != nil {
					fatal("failed to delete API key", slog.Any("error", err))
				}
			})

			fmt.Printf("Deleted API key: id=%d\n", id)
		},
	}

	cmd.Flags().Int64Var(&id, "id", 0, "API key ID to delete")
	_ = cmd.MarkFlagRequired("id")

	return cmd
}
