-- +goose Up
-- Allow password_hash to be NULL so that OIDC-only accounts do not require
-- a local password. Existing rows are unaffected; the column retains its
-- VARCHAR(60) type from migration 00020.
ALTER TABLE users
    ALTER COLUMN password_hash DROP NOT NULL;

-- +goose Down
-- Re-add the NOT NULL constraint.  Any OIDC-only rows (NULL password_hash)
-- must be backfilled before this can succeed; use a placeholder if needed.
ALTER TABLE users
    ALTER COLUMN password_hash SET NOT NULL;
