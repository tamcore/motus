-- +goose Up
ALTER TABLE users ADD COLUMN role VARCHAR(20) NOT NULL DEFAULT 'user';
ALTER TABLE users ADD CONSTRAINT valid_user_role
CHECK (role IN ('admin', 'user', 'readonly'));

-- Set the default admin user.
UPDATE users SET role = 'admin' WHERE email = 'admin@motus.local';

-- +goose Down
ALTER TABLE users DROP CONSTRAINT IF EXISTS valid_user_role;
ALTER TABLE users DROP COLUMN IF EXISTS role;
