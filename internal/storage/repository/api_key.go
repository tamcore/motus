package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

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

// generateApiKeyToken creates a cryptographically random 32-byte hex token.
func generateApiKeyToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Create inserts a new API key with an auto-generated token.
// If key.ExpiresAt is set, the key will expire at that time.
func (r *ApiKeyRepository) Create(ctx context.Context, key *model.ApiKey) error {
	token, err := generateApiKeyToken()
	if err != nil {
		return err
	}
	key.Token = token

	if key.Permissions == "" {
		key.Permissions = model.PermissionFull
	}

	err = r.pool.QueryRow(ctx,
		`INSERT INTO api_keys (user_id, token, name, permissions, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		key.UserID, key.Token, key.Name, key.Permissions, key.ExpiresAt,
	).Scan(&key.ID, &key.CreatedAt)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

// GetByToken retrieves an API key by its token value.
func (r *ApiKeyRepository) GetByToken(ctx context.Context, token string) (*model.ApiKey, error) {
	k := &model.ApiKey{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, token, name, permissions, expires_at, created_at, last_used_at
		 FROM api_keys WHERE token = $1`,
		token,
	).Scan(&k.ID, &k.UserID, &k.Token, &k.Name, &k.Permissions, &k.ExpiresAt, &k.CreatedAt, &k.LastUsedAt)
	if err != nil {
		return nil, fmt.Errorf("get api key by token: %w", err)
	}
	return k, nil
}

// GetByID retrieves an API key by its ID.
func (r *ApiKeyRepository) GetByID(ctx context.Context, id int64) (*model.ApiKey, error) {
	k := &model.ApiKey{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, token, name, permissions, expires_at, created_at, last_used_at
		 FROM api_keys WHERE id = $1`,
		id,
	).Scan(&k.ID, &k.UserID, &k.Token, &k.Name, &k.Permissions, &k.ExpiresAt, &k.CreatedAt, &k.LastUsedAt)
	if err != nil {
		return nil, fmt.Errorf("get api key by id: %w", err)
	}
	return k, nil
}

// ListByUser returns all API keys for a user, ordered by creation date.
// Tokens are redacted in the response (only first 8 chars shown).
func (r *ApiKeyRepository) ListByUser(ctx context.Context, userID int64) ([]*model.ApiKey, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, token, name, permissions, expires_at, created_at, last_used_at
		 FROM api_keys WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]*model.ApiKey, 0, 8)
	for rows.Next() {
		k := &model.ApiKey{}
		if err := rows.Scan(&k.ID, &k.UserID, &k.Token, &k.Name, &k.Permissions, &k.ExpiresAt, &k.CreatedAt, &k.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
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
