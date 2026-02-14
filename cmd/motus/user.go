package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/validation"
	"golang.org/x/crypto/bcrypt"
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

			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				fatal("failed to hash password", slog.Any("error", err))
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var userID int64
			err = pool.QueryRow(ctx, `
				INSERT INTO users (email, name, password_hash, role, created_at)
				VALUES ($1, $2, $3, $4, NOW())
				RETURNING id
			`, email, name, string(hash), role).Scan(&userID)
			if err != nil {
				if strings.Contains(err.Error(), "duplicate key") {
					fatal("user already exists", slog.String("email", email))
				}
				fatal("failed to create user", slog.Any("error", err))
			}

			fmt.Printf("Created user: id=%d, email=%s, name=%s, role=%s\n", userID, email, name, role)
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
			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			userRepo := repository.NewUserRepository(pool)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			users, err := userRepo.ListAll(ctx)
			if err != nil {
				fatal("failed to list users", slog.Any("error", err))
			}

			if len(users) == 0 {
				fmt.Println("No users found.")
				return
			}

			if filter != "" {
				users = filterUsers(users, filter)
				if len(users) == 0 {
					fmt.Println("No users match the filter.")
					return
				}
			}

			sortUsers(users, sortField)

			switch output {
			case "json":
				items := make([]map[string]interface{}, len(users))
				for i, u := range users {
					items[i] = map[string]interface{}{
						"id":        u.ID,
						"email":     u.Email,
						"name":      u.Name,
						"role":      u.Role,
						"createdAt": u.CreatedAt.Format(time.RFC3339),
					}
				}
				printJSON(items)
			case "csv":
				headers := []string{"ID", "Email", "Name", "Role", "Created"}
				rows := make([][]string, len(users))
				for i, u := range users {
					rows[i] = []string{
						fmt.Sprint(u.ID), u.Email, u.Name, u.Role,
						u.CreatedAt.Format("2006-01-02"),
					}
				}
				printCSV(headers, rows)
			default:
				tw := NewTableWriter(os.Stdout)
				tw.WriteHeader("ID", "EMAIL", "NAME", "ROLE", "CREATED")
				for _, u := range users {
					tw.WriteRow(fmt.Sprint(u.ID), u.Email, u.Name, u.Role,
						u.CreatedAt.Format("2006-01-02"))
				}
				tw.Flush()
			}
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
			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			tag, err := pool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
			if err != nil {
				fatal("failed to delete user", slog.Any("error", err))
			}
			if tag.RowsAffected() == 0 {
				fmt.Fprintf(os.Stderr, "No user found with email %q\n", email)
				os.Exit(1)
			}

			fmt.Printf("Deleted user: %s\n", email)
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

			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			userRepo := repository.NewUserRepository(pool)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

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
		Short: "Reset a user's password (auto-generates if --password is omitted)",
		Run: func(cmd *cobra.Command, args []string) {
			pwd := password
			generated := false
			if pwd == "" {
				var genErr error
				pwd, genErr = generatePassword(16)
				if genErr != nil {
					fatal("failed to generate password", slog.Any("error", genErr))
				}
				generated = true
			} else {
				if err := validation.ValidatePassword(pwd); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}

			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			userRepo := repository.NewUserRepository(pool)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			u, err := userRepo.GetByEmail(ctx, email)
			if err != nil {
				fatal("user not found", slog.String("email", email))
			}

			hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
			if err != nil {
				fatal("failed to hash password", slog.Any("error", err))
			}

			if err := userRepo.UpdatePassword(ctx, u.ID, string(hash)); err != nil {
				fatal("failed to update password", slog.Any("error", err))
			}

			if generated {
				fmt.Printf("Password reset for %s\nGenerated password: %s\n", email, pwd)
			} else {
				fmt.Printf("Password reset for %s\n", email)
			}
		},
	}

	f := cmd.Flags()
	f.StringVar(&email, "email", "", "User email")
	f.StringVar(&password, "password", "", "New password (auto-generated if omitted)")
	_ = cmd.MarkFlagRequired("email")

	return cmd
}

// filterUsers returns users matching the given field=value filter.
func filterUsers(users []*model.User, filter string) []*model.User {
	parts := strings.SplitN(filter, "=", 2)
	if len(parts) != 2 {
		fatalFn("invalid filter format (expected field=value)", slog.String("filter", filter))
		return nil
	}
	field, value := strings.ToLower(parts[0]), strings.ToLower(parts[1])

	var result []*model.User
	for _, u := range users {
		switch field {
		case "role":
			if strings.ToLower(u.Role) == value {
				result = append(result, u)
			}
		case "email":
			if strings.Contains(strings.ToLower(u.Email), value) {
				result = append(result, u)
			}
		case "name":
			if strings.Contains(strings.ToLower(u.Name), value) {
				result = append(result, u)
			}
		default:
			fatalFn("unknown filter field (supported: role, email, name)", slog.String("field", field))
			return nil
		}
	}
	return result
}

// sortUsers sorts users in-place by the given field.
func sortUsers(users []*model.User, field string) {
	switch strings.ToLower(field) {
	case "email":
		sort.Slice(users, func(i, j int) bool { return users[i].Email < users[j].Email })
	case "name":
		sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	case "role":
		sort.Slice(users, func(i, j int) bool { return users[i].Role < users[j].Role })
	case "created":
		sort.Slice(users, func(i, j int) bool { return users[i].CreatedAt.Before(users[j].CreatedAt) })
	default: // "id" or unrecognized
		sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })
	}
}

// generatePassword creates a cryptographically random alphanumeric password.
func generatePassword(length int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}
