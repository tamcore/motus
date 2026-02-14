package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// SessionRepository handles session persistence.
type SessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository creates a new session repository.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// Create generates a new session for the given user with a default 24-hour expiry.
func (r *SessionRepository) Create(ctx context.Context, userID int64) (*model.Session, error) {
	return r.CreateWithExpiry(ctx, userID, time.Now().Add(24*time.Hour), false)
}

// CreateWithExpiry generates a new session with a specific expiration time.
func (r *SessionRepository) CreateWithExpiry(ctx context.Context, userID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	s := &model.Session{
		ID:         hex.EncodeToString(b),
		UserID:     userID,
		RememberMe: rememberMe,
		CreatedAt:  time.Now(),
		ExpiresAt:  expiresAt,
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, remember_me, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		s.ID, s.UserID, s.RememberMe, s.CreatedAt, s.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

// CreateWithApiKey generates a new session linked to the API key that was used
// to create it. This allows the auth middleware to restore the API key's
// permission level on subsequent cookie-authenticated requests.
func (r *SessionRepository) CreateWithApiKey(ctx context.Context, userID int64, apiKeyID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	s := &model.Session{
		ID:         hex.EncodeToString(b),
		UserID:     userID,
		ApiKeyID:   &apiKeyID,
		RememberMe: rememberMe,
		CreatedAt:  time.Now(),
		ExpiresAt:  expiresAt,
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, api_key_id, remember_me, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		s.ID, s.UserID, s.ApiKeyID, s.RememberMe, s.CreatedAt, s.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create session with api key: %w", err)
	}
	return s, nil
}

// CreateSudo generates a sudo session that allows an admin to impersonate
// another user. The originalUserID is stored so the admin can restore
// their own session later.
func (r *SessionRepository) CreateSudo(ctx context.Context, targetUserID, originalUserID int64) (*model.Session, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	s := &model.Session{
		ID:             hex.EncodeToString(b),
		UserID:         targetUserID,
		OriginalUserID: &originalUserID,
		IsSudo:         true,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(1 * time.Hour), // Sudo sessions expire after 1 hour.
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, original_user_id, is_sudo, remember_me, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		s.ID, s.UserID, s.OriginalUserID, s.IsSudo, s.RememberMe, s.CreatedAt, s.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create sudo session: %w", err)
	}
	return s, nil
}

// GetByID retrieves a session by its ID, returning nil if expired.
func (r *SessionRepository) GetByID(ctx context.Context, id string) (*model.Session, error) {
	s := &model.Session{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, remember_me, original_user_id, is_sudo, api_key_id, created_at, expires_at
		 FROM sessions WHERE id = $1 AND expires_at > NOW()`, id,
	).Scan(&s.ID, &s.UserID, &s.RememberMe, &s.OriginalUserID, &s.IsSudo, &s.ApiKeyID, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return s, nil
}

// GetByIDPrefix finds a session owned by userID whose ID starts with the
// given prefix. This supports the truncated display IDs returned by the
// API — the frontend never sees the full session token.
func (r *SessionRepository) GetByIDPrefix(ctx context.Context, userID int64, prefix string) (*model.Session, error) {
	s := &model.Session{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, remember_me, original_user_id, is_sudo, api_key_id, created_at, expires_at
		 FROM sessions WHERE user_id = $1 AND id LIKE $2 || '%' AND expires_at > NOW()
		 LIMIT 1`, userID, prefix,
	).Scan(&s.ID, &s.UserID, &s.RememberMe, &s.OriginalUserID, &s.IsSudo, &s.ApiKeyID, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("get session by prefix: %w", err)
	}
	return s, nil
}

// Delete removes a session by ID.
func (r *SessionRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// ListByUser returns all non-expired sessions for a user, ordered by creation
// time descending. Each session includes the linked API key name (if any).
func (r *SessionRepository) ListByUser(ctx context.Context, userID int64) ([]*model.Session, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.user_id, s.remember_me, s.original_user_id, s.is_sudo,
		       s.api_key_id, s.created_at, s.expires_at, k.name
		FROM sessions s
		LEFT JOIN api_keys k ON s.api_key_id = k.id
		WHERE s.user_id = $1 AND s.expires_at > NOW()
		ORDER BY s.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions by user: %w", err)
	}
	defer rows.Close()

	var sessions []*model.Session
	for rows.Next() {
		s := &model.Session{}
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.RememberMe, &s.OriginalUserID, &s.IsSudo,
			&s.ApiKeyID, &s.CreatedAt, &s.ExpiresAt, &s.ApiKeyName,
		); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session rows: %w", err)
	}
	return sessions, nil
}
