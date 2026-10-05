package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// ApiKeyRepository handles API key persistence operations.
type ApiKeyRepository struct {
	pool *pgxpool.Pool
}

// NewApiKeyRepository creates a new API key repository.
func NewApiKeyRepository(pool *pgxpool.Pool) *ApiKeyRepository {
	return &ApiKeyRepository{pool: pool}
}

// Create inserts a new API key with an auto-generated token.
// The raw token is stored in key.Token for the caller; only the SHA-256 hash
// is persisted in the database.
// If key.ExpiresAt is set, the key will expire at that time.
func (r *ApiKeyRepository) Create(ctx context.Context, key *model.ApiKey) error {
	token := NewToken()
	key.Token = token // return raw token to caller

	if key.Permissions == "" {
		key.Permissions = model.PermissionFull
	}

	err := r.pool.QueryRow(ctx,
		`INSERT INTO api_keys (user_id, token, name, permissions, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		key.UserID, HashToken(token), key.Name, key.Permissions, key.ExpiresAt,
	).Scan(&key.ID, &key.CreatedAt)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

// GetByToken retrieves an API key by its raw token value.
// The token is hashed before lookup so the database never stores plaintext.
func (r *ApiKeyRepository) GetByToken(ctx context.Context, token string) (*model.ApiKey, error) {
	k, err := scanApiKey(r.pool.QueryRow(ctx, `SELECT `+apiKeyColumns+` FROM api_keys WHERE token = $1`, HashToken(token)))
	if err != nil {
		return nil, fmt.Errorf("get api key by token: %w", err)
	}
	return k, nil
}

// GetByID retrieves an API key by its ID.
func (r *ApiKeyRepository) GetByID(ctx context.Context, id int64) (*model.ApiKey, error) {
	k, err := scanApiKey(r.pool.QueryRow(ctx, `SELECT `+apiKeyColumns+` FROM api_keys WHERE id = $1`, id))
	if err != nil {
		return nil, fmt.Errorf("get api key by id: %w", err)
	}
	return k, nil
}

const apiKeyColumns = `id, user_id, token, name, permissions, expires_at, created_at, last_used_at` // #nosec G101 -- SQL column list, not a credential

func scanApiKey(row pgx.Row) (*model.ApiKey, error) {
	k := &model.ApiKey{}
	err := row.Scan(&k.ID, &k.UserID, &k.Token, &k.Name, &k.Permissions, &k.ExpiresAt, &k.CreatedAt, &k.LastUsedAt)
	return k, err
}

// ListByUser returns all API keys for a user, ordered by creation date.
// Tokens are redacted in the response (only first 8 chars shown).
func (r *ApiKeyRepository) ListByUser(ctx context.Context, userID int64) ([]*model.ApiKey, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+apiKeyColumns+` FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.ApiKey, error) {
		k, err := scanApiKey(row)
		if err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		return k, nil
	})
}

// Delete removes an API key by ID.
func (r *ApiKeyRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	return nil
}

// UpdateLastUsed sets the last_used_at timestamp to now for the given key.
func (r *ApiKeyRepository) UpdateLastUsed(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("update api key last used: %w", err)
	}
	return nil
}
