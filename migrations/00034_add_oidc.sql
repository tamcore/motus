-- +goose Up
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS oidc_subject TEXT,
    ADD COLUMN IF NOT EXISTS oidc_issuer  TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS users_oidc_subject_issuer
    ON users (oidc_subject, oidc_issuer)
    WHERE oidc_subject IS NOT NULL;

-- Short-lived state tokens used to prevent CSRF in the OAuth2 redirect flow.
-- Each state is single-use and expires after 10 minutes.
CREATE TABLE IF NOT EXISTS oidc_states (
    state      TEXT PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS oidc_states;
DROP INDEX IF EXISTS users_oidc_subject_issuer;
ALTER TABLE users
    DROP COLUMN IF EXISTS oidc_subject,
    DROP COLUMN IF EXISTS oidc_issuer;
