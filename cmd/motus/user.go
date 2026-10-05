package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/validation"
)

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users",
	}
	cmd.AddCommand(
		newUserAddCmd(),
		newUserListCmd(),
		newUserDeleteCmd(),
		newUserUpdateCmd(),
		newUserSetPasswordCmd(),
		newUserKeysCmd(),
		newUserSessionsCmd(),
	)
	return cmd
}

func newUserAddCmd() *cobra.Command {
	var email, name, password, role string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new user",
		Run: func(cmd *cobra.Command, args []string) {
			if !model.IsValidRole(role) {
				fmt.Fprintf(os.Stderr, "Error: invalid role %q (must be admin, user, or readonly)\n", role)
				os.Exit(1)
			}

			if err := validation.ValidateEmail(email); err != nil {
				fatalFn("invalid email", slog.Any("error", err))
				return
			}
			if err := validation.ValidateName(name); err != nil {
				fatalFn("invalid name", slog.Any("error", err))
				return
			}
			hash, err := validation.HashPassword(password)
			if err != nil {
				fatalFn("invalid password", slog.Any("error", err))
				return
			}

			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				u := &model.User{Email: email, Name: name, PasswordHash: hash, Role: role}
				if err := repository.NewUserRepository(pool).Create(ctx, u); err != nil {
					if strings.Contains(err.Error(), "duplicate key") {
						fatal("user already exists", slog.String("email", email))
					}
					fatal("failed to create user", slog.Any("error", err))
				}

				fmt.Printf("Created user: id=%d, email=%s, name=%s, role=%s\n", u.ID, u.Email, u.Name, u.Role)
			})
		},
	}

	f := cmd.Flags()
	f.StringVar(&email, "email", "", "User email")
	f.StringVar(&name, "name", "", "User display name")
	f.StringVar(&password, "password", "", "User password")
	f.StringVar(&role, "role", "user", "User role: admin, user, readonly")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("password")

	return cmd
}

func newUserListCmd() *cobra.Command {
	var output, filter, sortField string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all users",
		Run: func(cmd *cobra.Command, args []string) {
			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				users, err := repository.NewUserRepository(pool).ListAll(ctx)
				if err != nil {
					fatal("failed to list users", slog.Any("error", err))
				}

				if len(users) == 0 {
					fmt.Println("No users found.")
					return
				}

				if filter != "" {
					users = filterList(users, filter, userFilters)
					if len(users) == 0 {
						fmt.Println("No users match the filter.")
						return
					}
				}

				sortList(users, sortField, userSorts)

				items := make([]map[string]any, len(users))
				rows := make([][]string, len(users))
				for i, u := range users {
					items[i] = map[string]any{
						"id":        u.ID,
						"email":     u.Email,
						"name":      u.Name,
						"role":      u.Role,
						"createdAt": u.CreatedAt.Format(time.RFC3339),
					}
					rows[i] = []string{
						fmt.Sprint(u.ID), u.Email, u.Name, u.Role,
						u.CreatedAt.Format("2006-01-02"),
					}
				}
				headers := []string{"ID", "EMAIL", "NAME", "ROLE", "CREATED"}
				render(output, items, headers, rows)
			})
		},
	}

	f := cmd.Flags()
	f.StringVar(&output, "output", "table", "Output format: table, json, csv")
	f.StringVar(&filter, "filter", "", "Filter by field=value (e.g. role=admin)")
	f.StringVar(&sortField, "sort", "id", "Sort by field: id, email, name, role, created")

	return cmd
}

func newUserDeleteCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a user by email",
		Run: func(cmd *cobra.Command, args []string) {
			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				tag, err := pool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
				if err != nil {
					fatal("failed to delete user", slog.Any("error", err))
				}
				if tag.RowsAffected() == 0 {
					fmt.Fprintf(os.Stderr, "No user found with email %q\n", email)
					os.Exit(1)
				}

				fmt.Printf("Deleted user: %s\n", email)
			})
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Email of user to delete")
	_ = cmd.MarkFlagRequired("email")

	return cmd
}

func newUserUpdateCmd() *cobra.Command {
	var email, newEmail, name, role string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a user's name, email, or role",
		Run: func(cmd *cobra.Command, args []string) {
			if newEmail == "" && name == "" && role == "" {
				fmt.Fprintln(os.Stderr, "Error: at least one of --new-email, --name, or --role must be specified")
				os.Exit(1)
			}
			if newEmail != "" {
				if err := validation.ValidateEmail(newEmail); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
			if name != "" {
				if err := validation.ValidateName(name); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
			if role != "" && !model.IsValidRole(role) {
				fmt.Fprintf(os.Stderr, "Error: invalid role %q (must be admin, user, or readonly)\n", role)
				os.Exit(1)
			}

			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				userRepo := repository.NewUserRepository(pool)
				u, err := userRepo.GetByEmail(ctx, email)
				if err != nil {
					fatal("user not found", slog.String("email", email))
				}

				if newEmail != "" {
					u.Email = newEmail
				}
				if name != "" {
					u.Name = name
				}
				if role != "" {
					u.Role = role
				}

				if err := userRepo.Update(ctx, u); err != nil {
					fatal("failed to update user", slog.Any("error", err))
				}

				fmt.Printf("Updated user: id=%d, email=%s, name=%s, role=%s\n", u.ID, u.Email, u.Name, u.Role)
			})
		},
	}

	f := cmd.Flags()
	f.StringVar(&email, "email", "", "Identify user by email")
	f.StringVar(&newEmail, "new-email", "", "New email address")
	f.StringVar(&name, "name", "", "New display name")
	f.StringVar(&role, "role", "", "New role: admin, user, readonly")
	_ = cmd.MarkFlagRequired("email")

	return cmd
}

func newUserSetPasswordCmd() *cobra.Command {
	var email, password string

	cmd := &cobra.Command{
		Use:   "set-password",
		Short: "Reset a user's password",
		Run: func(cmd *cobra.Command, args []string) {
			hash, err := validation.HashPassword(password)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				userRepo := repository.NewUserRepository(pool)
				u, err := userRepo.GetByEmail(ctx, email)
				if err != nil {
					fatal("user not found", slog.String("email", email))
				}

				if err := userRepo.UpdatePassword(ctx, u.ID, hash); err != nil {
					fatal("failed to update password", slog.Any("error", err))
				}

				fmt.Printf("Password reset for %s\n", email)
			})
		},
	}

	f := cmd.Flags()
	f.StringVar(&email, "email", "", "User email")
	f.StringVar(&password, "password", "", "New password")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("password")

	return cmd
}

var userFilters = map[string]listFilter[*model.User]{
	"role":  {exact: true, get: func(u *model.User) string { return u.Role }},
	"email": {get: func(u *model.User) string { return u.Email }},
	"name":  {get: func(u *model.User) string { return u.Name }},
}

var userSorts = map[string]func(a, b *model.User) int{
	"id":      by(func(u *model.User) int64 { return u.ID }),
	"email":   by(func(u *model.User) string { return u.Email }),
	"name":    by(func(u *model.User) string { return u.Name }),
	"role":    by(func(u *model.User) string { return u.Role }),
	"created": func(a, b *model.User) int { return a.CreatedAt.Compare(b.CreatedAt) },
}
