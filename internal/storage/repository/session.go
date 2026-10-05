package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
	return r.insert(ctx, &model.Session{UserID: userID, RememberMe: rememberMe, ExpiresAt: expiresAt})
}

// CreateWithApiKey generates a new session linked to the API key that was used
// to create it. This allows the auth middleware to restore the API key's
// permission level on subsequent cookie-authenticated requests.
func (r *SessionRepository) CreateWithApiKey(ctx context.Context, userID int64, apiKeyID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
	return r.insert(ctx, &model.Session{UserID: userID, ApiKeyID: &apiKeyID, RememberMe: rememberMe, ExpiresAt: expiresAt})
}

// CreateSudo generates a sudo session that allows an admin to impersonate
// another user. The originalUserID is stored so the admin can restore
// their own session later. Sudo sessions expire after 1 hour.
func (r *SessionRepository) CreateSudo(ctx context.Context, targetUserID, originalUserID int64) (*model.Session, error) {
	return r.insert(ctx, &model.Session{
		UserID:         targetUserID,
		OriginalUserID: &originalUserID,
		IsSudo:         true,
		ExpiresAt:      time.Now().Add(time.Hour),
	})
}

// insert assigns s a random ID and creation time and stores it.
func (r *SessionRepository) insert(ctx context.Context, s *model.Session) (*model.Session, error) {
	s.ID = NewToken()
	s.CreatedAt = time.Now()

	_, err := r.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, api_key_id, original_user_id, is_sudo, remember_me, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		s.ID, s.UserID, s.ApiKeyID, s.OriginalUserID, s.IsSudo, s.RememberMe, s.CreatedAt, s.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

// GetByID retrieves a session by its ID, returning nil if expired.
func (r *SessionRepository) GetByID(ctx context.Context, id string) (*model.Session, error) {
	s := &model.Session{}
	err := scanSession(r.pool.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM sessions s WHERE s.id = $1 AND s.expires_at > NOW()`, id,
	), s)
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
	err := scanSession(r.pool.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM sessions s
		 WHERE s.user_id = $1 AND s.id LIKE $2 || '%' AND s.expires_at > NOW()
		 LIMIT 1`, userID, prefix,
	), s)
	if err != nil {
		return nil, fmt.Errorf("get session by prefix: %w", err)
	}
	return s, nil
}

const sessionColumns = `s.id, s.user_id, s.remember_me, s.original_user_id, s.is_sudo, s.api_key_id, s.created_at, s.expires_at`

// scanSession scans sessionColumns, followed by extra, into s.
func scanSession(row pgx.Row, s *model.Session, extra ...any) error {
	return row.Scan(append([]any{&s.ID, &s.UserID, &s.RememberMe, &s.OriginalUserID, &s.IsSudo, &s.ApiKeyID, &s.CreatedAt, &s.ExpiresAt}, extra...)...)
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
		SELECT `+sessionColumns+`, k.name, s.last_seen_at, s.last_seen_ip, s.last_seen_user_agent
		FROM sessions s
		LEFT JOIN api_keys k ON s.api_key_id = k.id
		WHERE s.user_id = $1 AND s.expires_at > NOW()
		ORDER BY s.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions by user: %w", err)
	}
	sessions, err := pgx.AppendRows([]*model.Session(nil), rows, func(row pgx.CollectableRow) (*model.Session, error) {
		s := &model.Session{}
		err := scanSession(row, s, &s.ApiKeyName, &s.LastSeenAt, &s.LastSeenIP, &s.LastSeenUserAgent)
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("list session rows: %w", err)
	}
	return sessions, nil
}

// UpdateExpiry extends the expiry of a session to the given time.
func (r *SessionRepository) UpdateExpiry(ctx context.Context, id string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sessions SET expires_at = $1 WHERE id = $2`,
		expiresAt, id,
	)
	if err != nil {
		return fmt.Errorf("update session expiry: %w", err)
	}
	return nil
}

// UpdateLastSeen records the IP address and user agent of the most recent
// authenticated request for this session.
func (r *SessionRepository) UpdateLastSeen(ctx context.Context, id, ip, userAgent string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sessions SET last_seen_at = NOW(), last_seen_ip = $2, last_seen_user_agent = $3 WHERE id = $1`,
		id, ip, userAgent,
	)
	if err != nil {
		return fmt.Errorf("update session last seen: %w", err)
	}
	return nil
}

// DeleteAllByUser removes all sessions for a user except the one with the given ID.
func (r *SessionRepository) DeleteAllByUser(ctx context.Context, userID int64, exceptID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM sessions WHERE user_id = $1 AND id != $2`,
		userID, exceptID,
	)
	if err != nil {
		return fmt.Errorf("delete all sessions for user: %w", err)
	}
	return nil
}
