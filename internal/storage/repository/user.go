package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// UserRepository handles user persistence operations.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new user repository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create inserts a new user into the database.
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	if user.Role == "" {
		user.Role = model.RoleUser
	}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, role)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		user.Email, user.PasswordHash, user.Name, user.Role,
	).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// CreateOIDCUser inserts a new password-less user created via OIDC login.
func (r *UserRepository) CreateOIDCUser(ctx context.Context, email, name, role, subject, issuer string) (*model.User, error) {
	if role == "" {
		role = model.RoleUser
	}
	u := &model.User{}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, name, role, oidc_subject, oidc_issuer)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, email, name, role, created_at, oidc_subject, oidc_issuer`,
		email, name, role, subject, issuer,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt, &u.OIDCSubject, &u.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("create oidc user: %w", err)
	}
	return u, nil
}

// GetByOIDCSubject retrieves a user by their OIDC subject and issuer.
func (r *UserRepository) GetByOIDCSubject(ctx context.Context, subject, issuer string) (*model.User, error) {
	u := &model.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, COALESCE(password_hash, ''), name, role, token, created_at, oidc_subject, oidc_issuer
		 FROM users WHERE oidc_subject = $1 AND oidc_issuer = $2`,
		subject, issuer,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.Token, &u.CreatedAt, &u.OIDCSubject, &u.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("get user by oidc subject: %w", err)
	}
	return u, nil
}

// SetOIDCSubject links an OIDC subject and issuer to an existing user account.
func (r *UserRepository) SetOIDCSubject(ctx context.Context, userID int64, subject, issuer string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET oidc_subject = $1, oidc_issuer = $2 WHERE id = $3`,
		subject, issuer, userID,
	)
	if err != nil {
		return fmt.Errorf("set oidc subject: %w", err)
	}
	return nil
}

// GetByEmail retrieves a user by email address.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	u := &model.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, COALESCE(password_hash, ''), name, role, token, created_at, oidc_subject, oidc_issuer
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.Token, &u.CreatedAt, &u.OIDCSubject, &u.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

// GetByID retrieves a user by ID.
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	u := &model.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, COALESCE(password_hash, ''), name, role, token, created_at, oidc_subject, oidc_issuer
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.Token, &u.CreatedAt, &u.OIDCSubject, &u.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

// GetByToken retrieves a user by their API token.
func (r *UserRepository) GetByToken(ctx context.Context, token string) (*model.User, error) {
	u := &model.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, COALESCE(password_hash, ''), name, role, token, created_at, oidc_subject, oidc_issuer
		 FROM users WHERE token = $1`, token,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.Token, &u.CreatedAt, &u.OIDCSubject, &u.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("get user by token: %w", err)
	}
	return u, nil
}

// ListAll returns all users ordered by email. Passwords are excluded from
// the returned objects.
func (r *UserRepository) ListAll(ctx context.Context) ([]*model.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, email, COALESCE(password_hash, ''), name, role, token, created_at, oidc_subject, oidc_issuer
		 FROM users ORDER BY email`,
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]*model.User, 0, 16)
	for rows.Next() {
		u := &model.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.Token, &u.CreatedAt, &u.OIDCSubject, &u.OIDCIssuer); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// Update modifies an existing user's name, email, and role.
func (r *UserRepository) Update(ctx context.Context, user *model.User) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET name = $1, email = $2, role = $3 WHERE id = $4`,
		user.Name, user.Email, user.Role, user.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// UpdatePassword changes the password hash for a user.
func (r *UserRepository) UpdatePassword(ctx context.Context, userID int64, hash string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1 WHERE id = $2`,
		hash, userID,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// Delete removes a user by ID. Foreign key cascades clean up sessions, user_devices, etc.
func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// GetDevicesForUser returns all device IDs assigned to a user.
func (r *UserRepository) GetDevicesForUser(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT device_id FROM user_devices WHERE user_id = $1 ORDER BY device_id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get devices for user: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan device id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AssignDevice associates a device with a user. If the association already
// exists the operation is a no-op.
func (r *UserRepository) AssignDevice(ctx context.Context, userID, deviceID int64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_devices (user_id, device_id)
		 VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		userID, deviceID,
	)
	if err != nil {
		return fmt.Errorf("assign device: %w", err)
	}
	return nil
}

// UnassignDevice removes a device association from a user.
func (r *UserRepository) UnassignDevice(ctx context.Context, userID, deviceID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM user_devices WHERE user_id = $1 AND device_id = $2`,
		userID, deviceID,
	)
	if err != nil {
		return fmt.Errorf("unassign device: %w", err)
	}
	return nil
}

// GenerateToken creates a random API token and stores it for the user.
func (r *UserRepository) GenerateToken(ctx context.Context, userID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	token := hex.EncodeToString(b)

	_, err := r.pool.Exec(ctx,
		`UPDATE users SET token = $1 WHERE id = $2`,
		token, userID,
	)
	if err != nil {
		return "", fmt.Errorf("store token: %w", err)
	}
	return token, nil
}
