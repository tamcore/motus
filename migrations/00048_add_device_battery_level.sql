-- +goose Up
-- Last battery charge (percent, 0-100) reported by the device, maintained on
-- position ingest so device lists can show it without reading positions.
ALTER TABLE devices ADD COLUMN battery_level DOUBLE PRECISION;

-- Backfill from each device's latest position. The cast is guarded by CASE
-- because PostgreSQL does not guarantee the evaluation order of WHERE
-- conditions; SET only runs for rows that pass WHERE.
UPDATE devices d
SET battery_level = (p.attributes->>'batteryLevel')::DOUBLE PRECISION
FROM positions p
WHERE p.id = d.position_id
  AND CASE WHEN jsonb_typeof(p.attributes->'batteryLevel') = 'number'
           THEN (p.attributes->>'batteryLevel')::DOUBLE PRECISION BETWEEN 0 AND 100
      END;

-- +goose Down
ALTER TABLE devices DROP COLUMN IF EXISTS battery_level;
