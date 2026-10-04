package demo

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/ticker"
)

// DemoAccount defines a fixed demo account.
type DemoAccount struct {
	Email    string
	Name     string
	Password string
	Role     string
}

// DefaultAccounts are the immutable demo accounts.
var DefaultAccounts = []DemoAccount{
	{Email: "demo@motus.local", Name: "Demo User", Password: "demo", Role: "user"},
	{Email: "admin@motus.local", Name: "Admin User", Password: "admin", Role: "admin"},
}

// Service manages demo mode lifecycle: database seeding and periodic resets.
type Service struct {
	pool        *pgxpool.Pool
	resetTime   string // "HH:MM" format
	accounts    []DemoAccount
	deviceIMEIs []string
}

// NewService creates a demo service.
//
// resetTime should be in "HH:MM" format (e.g., "00:00" for midnight).
// deviceIMEIs identify the simulated devices removed on each reset.
func NewService(pool *pgxpool.Pool, resetTime string, deviceIMEIs []string) *Service {
	return &Service{
		pool:        pool,
		resetTime:   resetTime,
		accounts:    DefaultAccounts,
		deviceIMEIs: deviceIMEIs,
	}
}

// SeedIfNeeded performs a full reset: deletes all demo-managed data and
// re-creates it from scratch. Call this at startup to ensure the demo pod
// starts with a clean slate.
func (s *Service) SeedIfNeeded(ctx context.Context) error {
	slog.Info("reinitializing demo data")

	result, err := Reset(ctx, s.pool, s.accounts, s.deviceIMEIs)
	if err != nil {
		return fmt.Errorf("reset demo data: %w", err)
	}

	LogResult(result)
	return nil
}

// Start runs the periodic database reset loop. It checks every minute
// whether the current time matches the configured reset time and triggers
// a full reset when it does.
func (s *Service) Start(ctx context.Context) {
	var lastResetDay int
	ticker.Every(ctx, time.Minute, func() {
		now := time.Now()
		if now.Format("15:04") != s.resetTime || now.Day() == lastResetDay {
			return
		}
		lastResetDay = now.Day()
		slog.Info("nightly reset triggered")
		result, err := Reset(ctx, s.pool, s.accounts, s.deviceIMEIs)
		if err != nil {
			slog.Error("nightly reset failed", slog.Any("error", err))
			return
		}
		LogResult(result)
	})
	slog.Info("demo reset service stopped")
}

// IsDemoAccount checks whether the given email belongs to a demo account.
func IsDemoAccount(email string) bool {
	for _, acct := range DefaultAccounts {
		if acct.Email == email {
			return true
		}
	}
	return false
}
