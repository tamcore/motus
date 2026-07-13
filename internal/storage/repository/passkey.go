package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// PasskeyRepository handles WebAuthn passkey credential persistence.
type PasskeyRepository struct {
	pool *pgxpool.Pool
}

// NewPasskeyRepository creates a new passkey repository.
func NewPasskeyRepository(pool *pgxpool.Pool) *PasskeyRepository {
	return &PasskeyRepository{pool: pool}
}

// Create inserts a new passkey credential and populates c.ID / c.CreatedAt.
func (r *PasskeyRepository) Create(ctx context.Context, c *model.PasskeyCredential) error {
	// The transports column is NOT NULL; a nil Go slice would insert SQL NULL,
	// so coerce it to an empty array.
	transports := c.Transports
	if transports == nil {
		transports = []string{}
	}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO passkey_credentials
		 (user_id, credential_id, public_key, attestation_type, aaguid,
		  sign_count, transports, backup_eligible, backup_state, name)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, created_at`,
		c.UserID, c.CredentialID, c.PublicKey, c.AttestationType, c.AAGUID,
		int64(c.SignCount), transports, c.BackupEligible, c.BackupState, c.Name,
	).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return fmt.Errorf("create passkey credential: %w", err)
	}
	return nil
}

// ListByUser returns all passkey credentials for a user, newest first.
func (r *PasskeyRepository) ListByUser(ctx context.Context, userID int64) ([]*model.PasskeyCredential, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, credential_id, public_key, attestation_type, aaguid,
		        sign_count, transports, backup_eligible, backup_state, name,
		        created_at, last_used_at
		 FROM passkey_credentials WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list passkey credentials: %w", err)
	}
	defer rows.Close()

	creds := make([]*model.PasskeyCredential, 0, 4)
	for rows.Next() {
		c, err := scanPasskey(rows)
		if err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// GetByCredentialID retrieves a passkey credential by its raw credential ID.
func (r *PasskeyRepository) GetByCredentialID(ctx context.Context, credID []byte) (*model.PasskeyCredential, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, user_id, credential_id, public_key, attestation_type, aaguid,
		        sign_count, transports, backup_eligible, backup_state, name,
		        created_at, last_used_at
		 FROM passkey_credentials WHERE credential_id = $1`,
		credID,
	)
	c, err := scanPasskey(row)
	if err != nil {
		return nil, fmt.Errorf("get passkey by credential id: %w", err)
	}
	return c, nil
}

// UpdateSignCount updates the stored authenticator signature counter and bumps
// last_used_at to now. Called on every successful assertion for clone detection.
func (r *PasskeyRepository) UpdateSignCount(ctx context.Context, id int64, count uint32) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE passkey_credentials SET sign_count = $2, last_used_at = NOW() WHERE id = $1`,
		id, int64(count),
	)
	if err != nil {
		return fmt.Errorf("update passkey sign count: %w", err)
	}
	return nil
}

// Delete removes a passkey credential owned by userID. Scoping the delete by
// user_id prevents one user from deleting another user's credential.
func (r *PasskeyRepository) Delete(ctx context.Context, id, userID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM passkey_credentials WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete passkey credential: %w", err)
	}
	return nil
}

// DeleteAllByUser removes every passkey credential for a user.
func (r *PasskeyRepository) DeleteAllByUser(ctx context.Context, userID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM passkey_credentials WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("delete passkey credentials for user: %w", err)
	}
	return nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanPasskey scans a single passkey row. sign_count is stored as BIGINT and
// read back through an int64 before narrowing to the uint32 the WebAuthn library
// expects.
func scanPasskey(row rowScanner) (*model.PasskeyCredential, error) {
	c := &model.PasskeyCredential{}
	var signCount int64
	if err := row.Scan(
		&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &c.AttestationType, &c.AAGUID,
		&signCount, &c.Transports, &c.BackupEligible, &c.BackupState, &c.Name,
		&c.CreatedAt, &c.LastUsedAt,
	); err != nil {
		return nil, fmt.Errorf("scan passkey credential: %w", err)
	}
	c.SignCount = uint32(signCount)
	return c, nil
}
