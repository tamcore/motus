-- +goose Up
-- Cascade-delete sessions when their linked API key is deleted.
-- Previously the FK used ON DELETE SET NULL which left dangling sessions.
ALTER TABLE sessions DROP CONSTRAINT sessions_api_key_id_fkey;
ALTER TABLE sessions
    ADD CONSTRAINT sessions_api_key_id_fkey
    FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE sessions DROP CONSTRAINT sessions_api_key_id_fkey;
ALTER TABLE sessions
    ADD CONSTRAINT sessions_api_key_id_fkey
    FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL;
