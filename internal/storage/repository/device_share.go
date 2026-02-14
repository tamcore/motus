package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// DeviceShareRepository handles device share link persistence.
type DeviceShareRepository struct {
	pool *pgxpool.Pool
}

// NewDeviceShareRepository creates a new device share repository.
func NewDeviceShareRepository(pool *pgxpool.Pool) *DeviceShareRepository {
	return &DeviceShareRepository{pool: pool}
}

// generateToken creates a cryptographically random 32-byte hex token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate share token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Create inserts a new device share link.
func (r *DeviceShareRepository) Create(ctx context.Context, share *model.DeviceShare) error {
	token, err := generateToken()
	if err != nil {
		return err
	}
	share.Token = token

	err = r.pool.QueryRow(ctx,
		`INSERT INTO device_shares (device_id, token, created_by, expires_at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		share.DeviceID, share.Token, share.CreatedBy, share.ExpiresAt,
	).Scan(&share.ID, &share.CreatedAt)
	if err != nil {
		return fmt.Errorf("create device share: %w", err)
	}
	return nil
}

// GetByToken retrieves a device share by its token, checking expiry.
// Returns nil, nil if the token is not found or expired.
func (r *DeviceShareRepository) GetByToken(ctx context.Context, token string) (*model.DeviceShare, error) {
	s := &model.DeviceShare{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, device_id, token, created_by, expires_at, created_at
		 FROM device_shares
		 WHERE token = $1 AND (expires_at IS NULL OR expires_at > NOW())`,
		token,
	).Scan(&s.ID, &s.DeviceID, &s.Token, &s.CreatedBy, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get device share by token: %w", err)
	}
	return s, nil
}

// ListByDevice returns all active share links for a device.
func (r *DeviceShareRepository) ListByDevice(ctx context.Context, deviceID int64) ([]*model.DeviceShare, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, device_id, token, created_by, expires_at, created_at
		 FROM device_shares
		 WHERE device_id = $1 AND (expires_at IS NULL OR expires_at > NOW())
		 ORDER BY created_at DESC`,
		deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list device shares: %w", err)
	}
	defer rows.Close()

	var shares []*model.DeviceShare
	for rows.Next() {
		s := &model.DeviceShare{}
		if err := rows.Scan(&s.ID, &s.DeviceID, &s.Token, &s.CreatedBy, &s.ExpiresAt, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan device share: %w", err)
		}
		shares = append(shares, s)
	}
	return shares, rows.Err()
}

// GetByID retrieves a device share by its ID.
// Returns nil, nil if the share is not found.
func (r *DeviceShareRepository) GetByID(ctx context.Context, id int64) (*model.DeviceShare, error) {
	s := &model.DeviceShare{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, device_id, token, created_by, expires_at, created_at
		 FROM device_shares
		 WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.DeviceID, &s.Token, &s.CreatedBy, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get device share by id: %w", err)
	}
	return s, nil
}

// Delete removes a device share by ID.
func (r *DeviceShareRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM device_shares WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete device share: %w", err)
	}
	return nil
}
