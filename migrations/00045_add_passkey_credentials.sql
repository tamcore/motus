-- +goose Up
CREATE TABLE passkey_credentials (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id    BYTEA NOT NULL,
    public_key       BYTEA NOT NULL,
    attestation_type TEXT NOT NULL DEFAULT '',
    aaguid           BYTEA,
    sign_count       BIGINT NOT NULL DEFAULT 0,
    transports       TEXT[] NOT NULL DEFAULT '{}',
    backup_eligible  BOOLEAN NOT NULL DEFAULT FALSE,
    backup_state     BOOLEAN NOT NULL DEFAULT FALSE,
    name             TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMP NOT NULL DEFAULT NOW(),
    last_used_at     TIMESTAMP,
    CONSTRAINT passkey_credentials_credential_id_key UNIQUE (credential_id)
);

CREATE INDEX idx_passkey_credentials_user_id ON passkey_credentials(user_id);

-- +goose Down
DROP TABLE IF EXISTS passkey_credentials;
