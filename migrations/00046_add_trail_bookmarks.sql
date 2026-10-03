-- +goose Up

-- Trail bookmarks: named time ranges of a device's trail, owned by a user
-- (e.g. a hike) that can be reopened on the map. Removed together with the
-- owning user or the device.
CREATE TABLE trail_bookmarks (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    from_time TIMESTAMPTZ NOT NULL,
    to_time TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT trail_bookmarks_range_check CHECK (to_time > from_time),
    CONSTRAINT trail_bookmarks_name_check CHECK (name <> '')
);

CREATE INDEX idx_trail_bookmarks_user_from ON trail_bookmarks(user_id, from_time DESC);
CREATE INDEX idx_trail_bookmarks_device_id ON trail_bookmarks(device_id);

-- +goose Down
DROP TABLE IF EXISTS trail_bookmarks;
