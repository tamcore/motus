package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OIDCStateRepository persists short-lived OAuth2 state tokens used to
// prevent CSRF attacks in the OIDC redirect flow. Each state is single-use
// and is automatically rejected after 10 minutes.
type OIDCStateRepository struct {
	pool *pgxpool.Pool
}

// NewOIDCStateRepository creates a new OIDC state repository.
func NewOIDCStateRepository(pool *pgxpool.Pool) *OIDCStateRepository {
	return &OIDCStateRepository{pool: pool}
}

// Create stores a new state token.
func (r *OIDCStateRepository) Create(ctx context.Context, state string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO oidc_states (state) VALUES ($1)`,
		state,
	)
	if err != nil {
		return fmt.Errorf("create oidc state: %w", err)
	}
	return nil
}

// Consume deletes the state and returns true if it existed and was created
// within the last 10 minutes. Returns false when the state is unknown or
// has expired, providing single-use CSRF protection.
func (r *OIDCStateRepository) Consume(ctx context.Context, state string) (bool, error) {
	result, err := r.pool.Exec(ctx,
		`DELETE FROM oidc_states
		 WHERE state = $1
		   AND created_at > NOW() - INTERVAL '10 minutes'`,
		state,
	)
	if err != nil {
		return false, fmt.Errorf("consume oidc state: %w", err)
	}
	return result.RowsAffected() == 1, nil
}
