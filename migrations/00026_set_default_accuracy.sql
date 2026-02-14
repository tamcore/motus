-- +goose Up
-- Update existing NULL accuracy values to 0.0 for Home Assistant compatibility.
-- Home Assistant expects accuracy to be a number for zone calculations.
UPDATE positions SET accuracy = 0.0 WHERE accuracy IS NULL;

-- +goose Down
-- No need to reverse this change (NULL vs 0.0 are both "unknown accuracy")
