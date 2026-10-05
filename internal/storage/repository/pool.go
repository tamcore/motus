package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// Connect opens a pool for url, applies tune to its config, and pings it.
func Connect(ctx context.Context, url string, tune ...func(*pgxpool.Config)) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	for _, f := range tune {
		f(cfg)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// userHasAccess reports whether user is an admin or existsQuery, given the
// user ID and id, returns true.
func userHasAccess(ctx context.Context, pool *pgxpool.Pool, existsQuery string, user *model.User, id int64) bool {
	if user.IsAdmin() {
		return true
	}
	var exists bool
	err := pool.QueryRow(ctx, existsQuery, user.ID, id).Scan(&exists)
	return err == nil && exists
}
