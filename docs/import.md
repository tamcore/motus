# Data Import Guide

## Overview

Motus includes an import tool that reads from a Traccar source and imports
devices, positions, geofences, and calendars into the Motus database.

Two source modes are supported:

- **Dump file** (`--source-dump`): Parses COPY sections directly from a PostgreSQL dump file, no restore needed.
- **Live database** (`--source-db*`): Connects directly to a running Traccar PostgreSQL database.

## Usage

### Dump file mode

```bash
motus import --source-dump=/path/to/traccar_dump.sql --target-password=PASSWORD [options]
```

### Live database mode

```bash
motus import --source-dbhost=traccar-db --source-dbpass=PASSWORD \
  --target-password=MOTUS_PASSWORD [options]
```

### Full Dump Example

```bash
motus import \
  --source-dump=/path/to/traccar_dump.sql \
  --target-host=localhost \
  --target-port=5432 \
  --target-db=motus \
  --target-user=motus \
  --target-password=PASSWORD \
  --admin-email=admin@motus.example.com \
  --max-positions=50000 \
  --recent-days=180 \
  --verbose
```

### Full Live DB Example

```bash
motus import \
  --source-dbhost=traccar-db.local \
  --source-dbport=5432 \
  --source-dbname=traccar \
  --source-dbuser=traccar \
  --source-dbpass=TRACCAR_PASSWORD \
  --target-host=motus-db \
  --target-password=MOTUS_PASSWORD \
  --recent-days=90 \
  --exclude-unknown
```

## Flags

### Source flags (one group required, mutually exclusive)

| Flag | Default | Description |
|------|---------|-------------|
| `--source-dump` | (none) | Path to Traccar PostgreSQL dump file |
| `--source-dbhost` | `localhost` | Traccar source database host |
| `--source-dbport` | `5432` | Traccar source database port |
| `--source-dbname` | `traccar` | Traccar source database name |
| `--source-dbuser` | `traccar` | Traccar source database user |
| `--source-dbpass` | (none) | Traccar source database password (required for `--source-db*` mode) |

### Target flags

| Flag | Default | Description |
|------|---------|-------------|
| `--target-host` | `localhost` | Target database host |
| `--target-port` | `5432` | Target database port |
| `--target-db` | `motus` | Target database name |
| `--target-user` | `motus` | Target database user |
| `--target-password` | (required unless `--dry-run`) | Target database password |

### Import options

| Flag | Default | Description |
|------|---------|-------------|
| `--admin-email` | `admin@motus.local` | Admin user to associate imported devices with |
| `--device-filter` | (none) | Only import devices matching this unique_id or name |
| `--exclude-unknown` | `false` | Skip devices with status `unknown` |
| `--max-positions` | `50000` | Maximum positions to import (0 = unlimited) |
| `--recent-days` | `90` | Only import positions from the last N days (0 = all) |
| `--devices` | `true` | Import devices |
| `--positions` | `true` | Import positions |
| `--geofences` | `true` | Import geofences |
| `--calendars` | `true` | Import calendars |
| `--geocode-last-n` | `100` | Reverse geocode last N imported positions (0 = disable) |
| `--dry-run` | `false` | Parse and log without writing to database |
| `--verbose` | `false` | Enable verbose logging |

## What Gets Imported

- **Devices**: All devices from `tc_devices` with unique ID, name, phone, model, category
- **Positions**: Positions from `tc_positions` filtered by recency and count limits
- **Geofences**: Geofences from `tc_geofences` with WKT geometry (PostGIS)
- **Calendars**: Calendars from `tc_calendars` with iCalendar normalization
- **Relationships**: Calendar-geofence associations

## Examples

### Dry Run (validate dump format)

```bash
motus import --source-dump=traccar_dump.sql --dry-run --verbose
```

### Live DB Dry Run (reads from Traccar, writes nothing to Motus)

```bash
motus import \
  --source-dbhost=postgresql.local \
  --source-dbpass=secret \
  --dry-run --verbose
```

### Import Last 60 Days from Dump

```bash
motus import \
  --source-dump=traccar_dump.sql \
  --target-password=mypass \
  --recent-days=60
```

### Import Specific Device from Dump

```bash
motus import \
  --source-dump=traccar_dump.sql \
  --target-password=mypass \
  --device-filter=GT3 RS \
  --recent-days=90
```

### Import Only Known Devices (Skip Unknown)

```bash
motus import \
  --source-dump=traccar_dump.sql \
  --target-password=mypass \
  --exclude-unknown
```

### Import Only Devices and Geofences

```bash
motus import \
  --source-dump=traccar_dump.sql \
  --target-password=mypass \
  --positions=false \
  --calendars=false
```

### Import via Kubernetes Port-Forward

```bash
kubectl port-forward -n <namespace> statefulset/motus-postgres 15432:5432 &

motus import \
  --source-dump=traccar_dump.sql \
  --target-host=localhost \
  --target-port=15432 \
  --target-password=motus123 \
  --recent-days=90
```

## Troubleshooting

### Import fails with "connection refused"

Verify the target database is accessible:

```bash
psql -h localhost -p 5432 -U motus -d motus -c '\l'
```

If using Kubernetes, set up port-forwarding first:

```bash
kubectl port-forward -n <namespace> statefulset/motus-postgres 15432:5432
```

### Duplicate key violations

The import tool uses `ON CONFLICT DO UPDATE` for devices and skips errors for
positions. It is safe to re-run the import.

### No positions imported

Check the `--recent-days` flag. If the dump is old and `--recent-days` is set to
a small value, no positions will fall within the time window. Use `--recent-days=0`
to import all positions regardless of age.
