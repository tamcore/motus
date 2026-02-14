// Package traccarimport reads a Traccar PostgreSQL dump file and imports
// devices, positions, and geofences into the Motus database.
//
// It parses COPY sections directly from the dump (tab-separated values),
// avoiding the need to restore the dump into a separate database.
package traccarimport

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/tamcore/motus/internal/geocoding"
)

// Config holds all CLI flags.
type Config struct {
	// Source: dump file mode
	SourceDump string // --source-dump: path to Traccar PostgreSQL dump file

	// Source: live database mode
	SourceDBHost string // --source-dbhost
	SourceDBPort int    // --source-dbport
	SourceDBName string // --source-dbname
	SourceDBUser string // --source-dbuser
	SourceDBPass string // --source-dbpass

	TargetHost     string
	TargetPort     int
	TargetDB       string
	TargetUser     string
	TargetPassword string

	AdminEmail   string
	DeviceFilter string // Only import devices matching this unique_id or name
	MaxPositions int
	RecentDays   int
	Verbose      bool
	DryRun       bool

	// Import scope flags control which data types get imported.
	ImportDevices   bool
	ImportPositions bool
	ImportGeofences bool
	ImportCalendars bool

	// ExcludeUnknown skips devices with status "unknown" during import.
	ExcludeUnknown bool

	// GeocodeLastN enables reverse geocoding for the last N imported positions.
	// Set to 0 to disable geocoding. Default: 100.
	GeocodeLastN int
}

// sourceMode returns "dump" or "db" based on which source flags were set.
func (c *Config) sourceMode() string {
	if c.SourceDump != "" {
		return "dump"
	}
	return "db"
}

// validateConfig validates source and target configuration before running import.
func validateConfig(config *Config) error {
	hasDump := config.SourceDump != ""
	// hasDB is true when the user has explicitly provided DB connection details.
	// Empty SourceDBHost is treated as "not set" (distinct from the default "localhost").
	hasDB := config.SourceDBPass != "" || (config.SourceDBHost != "" && config.SourceDBHost != "localhost")

	if hasDump && hasDB {
		return fmt.Errorf("--source-dump and --source-db* flags are mutually exclusive")
	}
	if !hasDump && !hasDB {
		return fmt.Errorf("one of --source-dump or --source-db* flags is required")
	}
	if hasDB && config.SourceDBPass == "" {
		return fmt.Errorf("--source-dbpass is required when using --source-db* mode")
	}
	if config.TargetPassword == "" && !config.DryRun {
		return fmt.Errorf("--target-password is required (or use --dry-run)")
	}
	return validateImportScopes(config)
}

// TraccarDevice represents a row from tc_devices COPY data.
type TraccarDevice struct {
	ID       int64
	Name     string
	UniqueID string
	Phone    string
	Model    string
	Category string
	Disabled bool
	Status   string
}

// TraccarPosition represents a row from tc_positions COPY data.
type TraccarPosition struct {
	ID         int64
	Protocol   string
	DeviceID   int64
	ServerTime time.Time
	DeviceTime time.Time
	FixTime    time.Time
	Valid      bool
	Latitude   float64
	Longitude  float64
	Altitude   float64
	Speed      float64
	Course     float64
	Address    string
	Attributes string
}

// TraccarGeofence represents a row from tc_geofences COPY data.
type TraccarGeofence struct {
	ID          int64
	Name        string
	Description string
	Area        string // WKT format
	CalendarID  *int64 // Traccar calendar reference
}

// TraccarCalendar represents a row from tc_calendars COPY data.
type TraccarCalendar struct {
	ID   int64
	Name string
	Data string // Base64-encoded iCalendar data in Traccar
}

// NewCmd returns a cobra command for the import subcommand.
func NewCmd() *cobra.Command {
	config := &Config{}

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import data from a Traccar PostgreSQL dump or live database",
		Long: `Import devices, positions, geofences, and calendars from a Traccar source
into a Motus database. Supports two source modes:

  --source-dump   Read from a PostgreSQL dump file (COPY sections, no restore needed)
  --source-db*    Connect directly to a live Traccar PostgreSQL database

Features:
  - Automatically normalizes malformed Traccar calendars (adds UNTIL to RRULE)
  - Decodes bytea hex format calendar data
  - Imports calendar-geofence relationships
  - Preserves phone, model, category, and disabled fields
  - Filters positions by date range
  - Handles WKT coordinate swapping for PostGIS
  - Optional reverse geocoding for imported positions (--geocode-last-n)

Examples:
  # Import last 60 days from dump file
  motus import --source-dump traccar.sql --target-password mypass --recent-days 60

  # Import specific device from dump to Kubernetes database
  kubectl port-forward svc/motus-postgres 15432:5432 &
  motus import --source-dump traccar.sql --target-host localhost --target-port 15432 \
    --target-password motus123 --device-filter GT3 RS --recent-days 90

  # Dry run from dump file to validate format
  motus import --source-dump traccar.sql --dry-run --verbose

  # Live DB dry run (reads from Traccar, writes nothing to Motus)
  motus import --source-dbhost=postgresql.local --source-dbpass=secret \
    --dry-run --verbose

  # Full live DB import
  motus import \
    --source-dbhost=postgresql.local --source-dbpass=secret \
    --target-host=motus-db --target-password=motuspass \
    --recent-days=90 --exclude-unknown

  # Import only devices and geofences (no positions or calendars)
  motus import --source-dump traccar.sql --target-password mypass --positions=false --calendars=false

  # Import only known devices (skip status='unknown')
  motus import --source-dump traccar.sql --target-password mypass --exclude-unknown

  # Import only calendars
  motus import --source-dump traccar.sql --target-password mypass --devices=false --positions=false --geofences=false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateConfig(config); err != nil {
				return err
			}
			if err := runImport(config); err != nil {
				slog.Error("import failed", slog.Any("error", err))
				os.Exit(1)
			}
			slog.Info("import completed successfully")
			return nil
		},
	}

	f := cmd.Flags()

	// Source: dump file mode
	f.StringVar(&config.SourceDump, "source-dump", "", "Path to Traccar PostgreSQL dump file")

	// Source: live database mode
	f.StringVar(&config.SourceDBHost, "source-dbhost", "localhost", "Traccar source database host")
	f.IntVar(&config.SourceDBPort, "source-dbport", 5432, "Traccar source database port")
	f.StringVar(&config.SourceDBName, "source-dbname", "traccar", "Traccar source database name")
	f.StringVar(&config.SourceDBUser, "source-dbuser", "traccar", "Traccar source database user")
	f.StringVar(&config.SourceDBPass, "source-dbpass", "", "Traccar source database password")

	// Target: Motus destination
	f.StringVar(&config.TargetHost, "target-host", "localhost", "Target database host")
	f.IntVar(&config.TargetPort, "target-port", 5432, "Target database port")
	f.StringVar(&config.TargetDB, "target-db", "motus", "Target database name")
	f.StringVar(&config.TargetUser, "target-user", "motus", "Target database user")
	f.StringVar(&config.TargetPassword, "target-password", "", "Target database password")

	f.StringVar(&config.AdminEmail, "admin-email", "admin@motus.local", "Admin user email to associate devices with")
	f.StringVar(&config.DeviceFilter, "device-filter", "", "Only import device with this unique_id or name (e.g., '123456789012345' or 'GT3 RS')")
	f.IntVar(&config.MaxPositions, "max-positions", 50000, "Maximum number of positions to import (0 = unlimited)")
	f.IntVar(&config.RecentDays, "recent-days", 90, "Only import positions from the last N days (0 = all)")
	f.BoolVar(&config.ExcludeUnknown, "exclude-unknown", false, "Skip devices with status 'unknown'")
	f.BoolVar(&config.Verbose, "verbose", false, "Verbose logging")
	f.BoolVar(&config.DryRun, "dry-run", false, "Dry run (parse and log without writing to database)")

	f.BoolVar(&config.ImportDevices, "devices", true, "Import devices")
	f.BoolVar(&config.ImportPositions, "positions", true, "Import positions")
	f.BoolVar(&config.ImportGeofences, "geofences", true, "Import geofences")
	f.BoolVar(&config.ImportCalendars, "calendars", true, "Import calendars")
	f.IntVar(&config.GeocodeLastN, "geocode-last-n", 100, "Reverse geocode last N imported positions (0 = disable)")

	return cmd
}

// validateImportScopes checks that the import scope flags form a valid combination.
// At least one scope must be enabled (or geocoding enabled), and positions require
// devices to be enabled (since positions reference device IDs).
func validateImportScopes(config *Config) error {
	hasAnyScope := config.ImportDevices || config.ImportPositions || config.ImportGeofences || config.ImportCalendars
	hasGeocoding := config.GeocodeLastN > 0

	if !hasAnyScope && !hasGeocoding {
		return fmt.Errorf("at least one import scope or --geocode-last-n must be enabled")
	}
	if config.ImportPositions && !config.ImportDevices {
		return fmt.Errorf("--positions requires --devices (positions reference device IDs)")
	}
	return nil
}

func runImport(config *Config) error {
	ctx := context.Background()

	var (
		devices   []TraccarDevice
		positions []TraccarPosition
		geofences []TraccarGeofence
		calendars []TraccarCalendar
		err       error
	)

	switch config.sourceMode() {
	case "db":
		slog.Info("extracting from live traccar database",
			slog.String("host", config.SourceDBHost),
			slog.Int("port", config.SourceDBPort),
			slog.String("db", config.SourceDBName))
		devices, positions, geofences, calendars, err = extractFromDB(ctx, config)
		if err != nil {
			return fmt.Errorf("extract from traccar db: %w", err)
		}
	default: // "dump"
		slog.Info("parsing dump file", slog.String("file", config.SourceDump))
		devices, positions, geofences, calendars, err = parseDump(config)
		if err != nil {
			return fmt.Errorf("parse dump: %w", err)
		}
	}

	slog.Info("source data loaded",
		slog.Int("devices", len(devices)),
		slog.Int("positions", len(positions)),
		slog.Int("geofences", len(geofences)),
		slog.Int("calendars", len(calendars)))

	if config.DryRun {
		logParsedData(devices, positions, geofences, calendars, config)
		slog.Info("dry run complete - no data written")
		return nil
	}

	// Connect to target database
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		config.TargetUser, config.TargetPassword,
		config.TargetHost, config.TargetPort, config.TargetDB,
	)
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return fmt.Errorf("connect to target: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping target: %w", err)
	}
	slog.Info("connected to target database")

	// Look up admin user
	var adminID int64
	err = pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, config.AdminEmail).Scan(&adminID)
	if err != nil {
		return fmt.Errorf("admin user %q not found: %w", config.AdminEmail, err)
	}
	slog.Info("admin user found", slog.Int64("id", adminID), slog.String("email", config.AdminEmail))

	// Import devices
	var deviceMap map[int64]int64
	if config.ImportDevices && len(devices) > 0 {
		deviceMap, err = importDevices(ctx, pool, devices, adminID, config)
		if err != nil {
			return fmt.Errorf("import devices: %w", err)
		}
	}

	// Import positions (requires deviceMap from device import)
	if config.ImportPositions && len(positions) > 0 && len(deviceMap) > 0 {
		if err := importPositions(ctx, pool, positions, deviceMap, config); err != nil {
			return fmt.Errorf("import positions: %w", err)
		}

		// Update device lastUpdate from latest imported position timestamp
		if err := updateDeviceLastUpdate(ctx, pool, deviceMap); err != nil {
			return fmt.Errorf("update device last updates: %w", err)
		}
	}

	// Import calendars (before geofences, since geofences may reference calendars).
	var calendarMap map[int64]int64
	if config.ImportCalendars && len(calendars) > 0 {
		calendarMap, err = importCalendars(ctx, pool, calendars, adminID, config)
		if err != nil {
			return fmt.Errorf("import calendars: %w", err)
		}
	}

	// Import geofences (with calendar associations).
	if config.ImportGeofences && len(geofences) > 0 {
		if err := importGeofences(ctx, pool, geofences, adminID, calendarMap, config); err != nil {
			return fmt.Errorf("import geofences: %w", err)
		}
	}

	// Geocode last N positions if enabled (works on existing positions too)
	if config.GeocodeLastN > 0 {
		if err := geocodeRecentPositions(ctx, pool, config); err != nil {
			slog.Warn("geocoding failed", slog.Any("error", err))
		}
	}

	return nil
}

// parseDump reads the dump file and extracts devices, positions, and geofences
// from the PostgreSQL COPY sections.
func parseDump(config *Config) ([]TraccarDevice, []TraccarPosition, []TraccarGeofence, []TraccarCalendar, error) {
	f, err := os.Open(config.SourceDump)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer func() { _ = f.Close() }()

	var (
		devices        []TraccarDevice
		positions      []TraccarPosition
		geofences      []TraccarGeofence
		calendars      []TraccarCalendar
		section        string // current COPY section being parsed
		cutoffTime     time.Time
		allowedDevIDs  map[int64]bool // Track device IDs that match filter (populated after devices parsed)
		excludedDevIDs map[int64]bool // Track device IDs excluded by --exclude-unknown
		excludedCount  int            // Count of devices excluded by --exclude-unknown
	)

	if config.RecentDays > 0 {
		cutoffTime = time.Now().AddDate(0, 0, -config.RecentDays)
		slog.Info("position cutoff set", slog.String("after", cutoffTime.Format("2006-01-02")))
	}

	if config.DeviceFilter != "" {
		slog.Info("device filter active", slog.String("filter", config.DeviceFilter))
		allowedDevIDs = make(map[int64]bool)
	}

	if config.ExcludeUnknown {
		slog.Info("excluding devices with status 'unknown'")
		excludedDevIDs = make(map[int64]bool)
	}

	scanner := bufio.NewScanner(f)
	// Increase buffer size for long lines (positions with attributes can be very long)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	lineNum := 0
	positionCount := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Detect COPY section starts
		if strings.HasPrefix(line, "COPY public.tc_devices ") {
			section = "devices"
			if config.Verbose {
				slog.Debug("found COPY section", slog.String("table", "tc_devices"), slog.Int("line", lineNum))
			}
			continue
		}
		if strings.HasPrefix(line, "COPY public.tc_positions ") {
			section = "positions"
			if config.Verbose {
				slog.Debug("found COPY section", slog.String("table", "tc_positions"), slog.Int("line", lineNum))
			}
			continue
		}
		if strings.HasPrefix(line, "COPY public.tc_geofences ") {
			section = "geofences"
			if config.Verbose {
				slog.Debug("found COPY section", slog.String("table", "tc_geofences"), slog.Int("line", lineNum))
			}
			continue
		}
		if strings.HasPrefix(line, "COPY public.tc_calendars ") {
			section = "calendars"
			if config.Verbose {
				slog.Debug("found COPY section", slog.String("table", "tc_calendars"), slog.Int("line", lineNum))
			}
			continue
		}

		// End of COPY section
		if line == `\.` {
			if section != "" && config.Verbose {
				slog.Debug("end of COPY section", slog.String("section", section), slog.Int("line", lineNum))
			}
			section = ""
			continue
		}

		// Parse data rows (skip sections disabled by scope flags)
		switch section {
		case "devices":
			if !config.ImportDevices {
				continue
			}
			d, err := parseDevice(line)
			if err != nil {
				slog.Warn("skipping device", slog.Int("line", lineNum), slog.Any("error", err))
				continue
			}
			// Exclude devices with status "unknown" if flag is set
			if config.ExcludeUnknown && d.Status == "unknown" {
				excludedDevIDs[d.ID] = true
				excludedCount++
				if config.Verbose {
					slog.Debug("excluding unknown device",
						slog.Int64("id", d.ID),
						slog.String("name", d.Name),
						slog.String("uniqueID", d.UniqueID))
				}
				continue
			}
			// Apply device filter if specified
			if config.DeviceFilter != "" {
				if d.UniqueID != config.DeviceFilter && d.Name != config.DeviceFilter {
					continue // Skip devices that don't match filter
				}
				allowedDevIDs[d.ID] = true // Track this device ID for position filtering
			}
			devices = append(devices, d)

		case "positions":
			if !config.ImportPositions {
				continue
			}
			if config.MaxPositions > 0 && positionCount >= config.MaxPositions {
				continue
			}
			p, err := parsePosition(line)
			if err != nil {
				if config.Verbose {
					slog.Debug("skipping position", slog.Int("line", lineNum), slog.Any("error", err))
				}
				continue
			}
			// Skip positions for devices excluded by --exclude-unknown
			if len(excludedDevIDs) > 0 && excludedDevIDs[p.DeviceID] {
				continue
			}
			// Filter by device if filter is active
			if allowedDevIDs != nil && !allowedDevIDs[p.DeviceID] {
				continue // Skip positions for devices not in filter
			}
			// Filter by cutoff time
			if !cutoffTime.IsZero() && p.FixTime.Before(cutoffTime) {
				continue
			}
			positions = append(positions, p)
			positionCount++

			if positionCount%10000 == 0 {
				slog.Info("parsing positions", slog.Int("count", positionCount))
			}

		case "geofences":
			if !config.ImportGeofences {
				continue
			}
			g, err := parseGeofence(line)
			if err != nil {
				slog.Warn("skipping geofence", slog.Int("line", lineNum), slog.Any("error", err))
				continue
			}
			geofences = append(geofences, g)

		case "calendars":
			if !config.ImportCalendars {
				continue
			}
			c, err := parseCalendar(line)
			if err != nil {
				slog.Warn("skipping calendar", slog.Int("line", lineNum), slog.Any("error", err))
				continue
			}
			calendars = append(calendars, c)
		}
	}

	if config.ExcludeUnknown && excludedCount > 0 {
		slog.Info("excluded devices with status 'unknown'", slog.Int("count", excludedCount))
	}

	return devices, positions, geofences, calendars, scanner.Err()
}

// extractFromDB connects to a live Traccar PostgreSQL database and extracts
// devices, positions, geofences, and calendars. The signature mirrors parseDump
// so all downstream import functions are unchanged.
func extractFromDB(ctx context.Context, config *Config) ([]TraccarDevice, []TraccarPosition, []TraccarGeofence, []TraccarCalendar, error) {
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		config.SourceDBUser, config.SourceDBPass,
		config.SourceDBHost, config.SourceDBPort, config.SourceDBName)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("connect to source: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("ping source: %w", err)
	}
	slog.Info("connected to source traccar database")

	var (
		devices   []TraccarDevice
		positions []TraccarPosition
		geofences []TraccarGeofence
		calendars []TraccarCalendar
	)

	// --- Devices ---
	var allowedIDs []int64 // IDs of all returned devices, used to filter positions

	if config.ImportDevices {
		devQ := `SELECT id, name, uniqueid,
			COALESCE(phone,''), COALESCE(model,''), COALESCE(category,''),
			disabled, COALESCE(status,'')
		FROM tc_devices`
		var conditions []string
		var devArgs []interface{}
		argN := 1

		if config.ExcludeUnknown {
			conditions = append(conditions, "status != 'unknown'")
			slog.Info("excluding devices with status 'unknown'")
		}
		if config.DeviceFilter != "" {
			conditions = append(conditions, fmt.Sprintf("(uniqueid = $%d OR name = $%d)", argN, argN))
			devArgs = append(devArgs, config.DeviceFilter)
			argN++
			slog.Info("device filter active", slog.String("filter", config.DeviceFilter))
		}
		if len(conditions) > 0 {
			devQ += " WHERE " + strings.Join(conditions, " AND ")
		}
		devQ += " ORDER BY id"
		_ = argN // may not be used further

		devRows, err := pool.Query(ctx, devQ, devArgs...)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("query tc_devices: %w", err)
		}
		defer devRows.Close()

		for devRows.Next() {
			var d TraccarDevice
			if err := devRows.Scan(&d.ID, &d.Name, &d.UniqueID, &d.Phone, &d.Model, &d.Category, &d.Disabled, &d.Status); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("scan device: %w", err)
			}
			d.Status = strings.TrimSpace(d.Status)
			devices = append(devices, d)
			allowedIDs = append(allowedIDs, d.ID)

			if config.Verbose {
				slog.Debug("loaded device",
					slog.Int64("id", d.ID),
					slog.String("name", d.Name),
					slog.String("uniqueID", d.UniqueID),
					slog.String("status", d.Status))
			}
		}
		if err := devRows.Err(); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("iterate tc_devices: %w", err)
		}
		devRows.Close()

		slog.Info("devices loaded", slog.Int("count", len(devices)))
	}

	// --- Positions ---
	if config.ImportPositions {
		if config.RecentDays > 0 {
			cutoffTime := time.Now().AddDate(0, 0, -config.RecentDays)
			slog.Info("position cutoff set", slog.String("after", cutoffTime.Format("2006-01-02")))
		}

		posQ := `SELECT id, COALESCE(protocol,''), deviceid,
			servertime, devicetime, fixtime, valid,
			latitude, longitude, altitude, speed, course,
			COALESCE(address,''), COALESCE(attributes,'')
		FROM tc_positions`
		var conditions []string
		var posArgs []interface{}
		argN := 1

		if len(allowedIDs) > 0 {
			conditions = append(conditions, fmt.Sprintf("deviceid = ANY($%d::bigint[])", argN))
			posArgs = append(posArgs, allowedIDs)
			argN++
		}
		if config.RecentDays > 0 {
			cutoff := time.Now().AddDate(0, 0, -config.RecentDays)
			conditions = append(conditions, fmt.Sprintf("fixtime >= $%d", argN))
			posArgs = append(posArgs, cutoff)
			argN++
		}
		if len(conditions) > 0 {
			posQ += " WHERE " + strings.Join(conditions, " AND ")
		}
		if config.MaxPositions > 0 {
			posQ += fmt.Sprintf(" LIMIT $%d", argN)
			posArgs = append(posArgs, config.MaxPositions)
		}

		posRows, err := pool.Query(ctx, posQ, posArgs...)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("query tc_positions: %w", err)
		}
		defer posRows.Close()

		positionCount := 0
		for posRows.Next() {
			var p TraccarPosition
			if err := posRows.Scan(
				&p.ID, &p.Protocol, &p.DeviceID,
				&p.ServerTime, &p.DeviceTime, &p.FixTime, &p.Valid,
				&p.Latitude, &p.Longitude, &p.Altitude, &p.Speed, &p.Course,
				&p.Address, &p.Attributes,
			); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("scan position: %w", err)
			}
			positions = append(positions, p)
			positionCount++
			if positionCount%10000 == 0 {
				slog.Info("loading positions", slog.Int("count", positionCount))
			}
		}
		if err := posRows.Err(); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("iterate tc_positions: %w", err)
		}
		posRows.Close()

		slog.Info("positions loaded", slog.Int("count", len(positions)))
	}

	// --- Geofences ---
	if config.ImportGeofences {
		geoRows, err := pool.Query(ctx, `
			SELECT id, name, COALESCE(description,''), area, calendarid
			FROM tc_geofences`)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("query tc_geofences: %w", err)
		}
		defer geoRows.Close()

		for geoRows.Next() {
			var g TraccarGeofence
			var calID *int64
			if err := geoRows.Scan(&g.ID, &g.Name, &g.Description, &g.Area, &calID); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("scan geofence: %w", err)
			}
			g.CalendarID = calID
			geofences = append(geofences, g)
		}
		if err := geoRows.Err(); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("iterate tc_geofences: %w", err)
		}
		geoRows.Close()

		slog.Info("geofences loaded", slog.Int("count", len(geofences)))
	}

	// --- Calendars ---
	// pgx returns BYTEA as []byte (already decoded from hex by the driver).
	// importCalendars then applies the existing base64 decode + normalizeTraccarCalendar.
	if config.ImportCalendars {
		calRows, err := pool.Query(ctx, `SELECT id, name, data FROM tc_calendars`)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("query tc_calendars: %w", err)
		}
		defer calRows.Close()

		for calRows.Next() {
			var c TraccarCalendar
			var dataBytes []byte
			if err := calRows.Scan(&c.ID, &c.Name, &dataBytes); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("scan calendar: %w", err)
			}
			c.Data = string(dataBytes)
			calendars = append(calendars, c)
		}
		if err := calRows.Err(); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("iterate tc_calendars: %w", err)
		}
		calRows.Close()

		slog.Info("calendars loaded", slog.Int("count", len(calendars)))
	}

	return devices, positions, geofences, calendars, nil
}

// parseDevice parses a tab-separated tc_devices row.
// COPY columns: id, name, uniqueid, lastupdate, positionid, groupid, attributes,
// phone, model, contact, category, disabled, status, ...
func parseDevice(line string) (TraccarDevice, error) {
	fields := strings.Split(line, "\t")
	if len(fields) < 13 {
		return TraccarDevice{}, fmt.Errorf("expected at least 13 fields, got %d", len(fields))
	}

	id, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return TraccarDevice{}, fmt.Errorf("parse id: %w", err)
	}

	disabled := fields[11] == "t"
	status := strings.TrimSpace(nullStr(fields[12]))

	return TraccarDevice{
		ID:       id,
		Name:     fields[1],
		UniqueID: fields[2],
		Phone:    nullStr(fields[7]),
		Model:    nullStr(fields[8]),
		Category: nullStr(fields[10]),
		Disabled: disabled,
		Status:   status,
	}, nil
}

// parsePosition parses a tab-separated tc_positions row.
// COPY columns: id, protocol, deviceid, servertime, devicetime, fixtime, valid,
// latitude, longitude, altitude, speed, course, address, attributes, accuracy, network, geofenceids
func parsePosition(line string) (TraccarPosition, error) {
	fields := strings.Split(line, "\t")
	if len(fields) < 14 {
		return TraccarPosition{}, fmt.Errorf("expected at least 14 fields, got %d", len(fields))
	}

	id, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return TraccarPosition{}, fmt.Errorf("parse id: %w", err)
	}

	deviceID, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return TraccarPosition{}, fmt.Errorf("parse deviceid: %w", err)
	}

	fixTime, err := parseTimestamp(fields[5])
	if err != nil {
		return TraccarPosition{}, fmt.Errorf("parse fixtime: %w", err)
	}

	valid := fields[6] == "t"
	lat, _ := strconv.ParseFloat(fields[7], 64)
	lon, _ := strconv.ParseFloat(fields[8], 64)
	alt, _ := strconv.ParseFloat(fields[9], 64)
	speed, _ := strconv.ParseFloat(fields[10], 64)
	course, _ := strconv.ParseFloat(fields[11], 64)

	return TraccarPosition{
		ID:         id,
		Protocol:   fields[1],
		DeviceID:   deviceID,
		FixTime:    fixTime,
		Valid:      valid,
		Latitude:   lat,
		Longitude:  lon,
		Altitude:   alt,
		Speed:      speed,
		Course:     course,
		Address:    nullStr(fields[12]),
		Attributes: nullStr(fields[13]),
	}, nil
}

// parseGeofence parses a tab-separated tc_geofences row.
// COPY columns: id, name, description, area, attributes, calendarid
func parseGeofence(line string) (TraccarGeofence, error) {
	fields := strings.Split(line, "\t")
	if len(fields) < 4 {
		return TraccarGeofence{}, fmt.Errorf("expected at least 4 fields, got %d", len(fields))
	}

	id, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return TraccarGeofence{}, fmt.Errorf("parse id: %w", err)
	}

	g := TraccarGeofence{
		ID:          id,
		Name:        fields[1],
		Description: nullStr(fields[2]),
		Area:        fields[3],
	}

	// Parse calendarid if present (field index 5).
	if len(fields) > 5 {
		calStr := nullStr(fields[5])
		if calStr != "" {
			calID, err := strconv.ParseInt(calStr, 10, 64)
			if err == nil {
				g.CalendarID = &calID
			}
		}
	}

	return g, nil
}

// parseCalendar parses a tab-separated tc_calendars row.
// COPY columns: id, name, data, attributes
// Traccar stores calendar data as base64-encoded iCalendar.
func parseCalendar(line string) (TraccarCalendar, error) {
	fields := strings.Split(line, "\t")
	if len(fields) < 3 {
		return TraccarCalendar{}, fmt.Errorf("expected at least 3 fields, got %d", len(fields))
	}

	id, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return TraccarCalendar{}, fmt.Errorf("parse id: %w", err)
	}

	data := nullStr(fields[2])

	// Decode PostgreSQL bytea hex format (\\x... in COPY format) to readable iCalendar text
	// Note: COPY format uses backslash escaping, so \\x means \x
	if strings.HasPrefix(data, "\\\\x") {
		hexData := strings.TrimPrefix(data, "\\\\x")
		decoded, err := hex.DecodeString(hexData)
		if err != nil {
			return TraccarCalendar{}, fmt.Errorf("decode bytea hex: %w", err)
		}
		data = string(decoded)
	}

	return TraccarCalendar{
		ID:   id,
		Name: fields[1],
		Data: data,
	}, nil
}

// importDevices inserts devices into the Motus database and returns a mapping
// from Traccar device ID to Motus device ID.
func importDevices(ctx context.Context, pool *pgxpool.Pool, devices []TraccarDevice, adminID int64, config *Config) (map[int64]int64, error) {
	slog.Info("importing devices")

	deviceMap := make(map[int64]int64) // traccar ID -> motus ID
	imported := 0

	for _, d := range devices {
		// Determine protocol based on model
		protocol := "h02" // default for this tracker fleet
		if strings.Contains(strings.ToLower(d.Model), "watch") {
			protocol = "watch"
		}

		status := d.Status
		if status == "" {
			status = "offline"
		}

		name := d.Name
		// If the name is the same as the uniqueID (placeholder devices), make it more descriptive
		if name == d.UniqueID {
			name = fmt.Sprintf("Device %s", d.UniqueID[:minInt(8, len(d.UniqueID))])
		}

		if config.Verbose {
			slog.Debug("importing device",
				slog.String("name", name),
				slog.String("uniqueID", d.UniqueID),
				slog.String("model", d.Model),
				slog.String("phone", d.Phone),
				slog.String("protocol", protocol))
		}

		// Convert empty strings to nil for nullable TEXT columns
		phone := nullToNil(d.Phone)
		model := nullToNil(d.Model)
		category := nullToNil(d.Category)

		var motusID int64
		err := pool.QueryRow(ctx, `
			INSERT INTO devices (unique_id, name, protocol, status, phone, model, category, disabled, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
			ON CONFLICT (unique_id) DO UPDATE SET
				name = EXCLUDED.name,
				status = EXCLUDED.status,
				phone = EXCLUDED.phone,
				model = EXCLUDED.model,
				category = EXCLUDED.category,
				disabled = EXCLUDED.disabled,
				updated_at = NOW()
			RETURNING id
		`, d.UniqueID, name, protocol, status, phone, model, category, d.Disabled).Scan(&motusID)

		if err != nil {
			slog.Warn("failed to insert device", slog.String("uniqueID", d.UniqueID), slog.Any("error", err))
			continue
		}

		deviceMap[d.ID] = motusID

		// Associate with admin user
		_, err = pool.Exec(ctx, `
			INSERT INTO user_devices (user_id, device_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, adminID, motusID)
		if err != nil {
			slog.Warn("failed to associate device with admin", slog.Int64("deviceID", motusID), slog.Any("error", err))
		}

		imported++
	}

	slog.Info("devices imported", slog.Int("imported", imported), slog.Int("mapped", len(deviceMap)))
	return deviceMap, nil
}

// importPositions inserts positions into the Motus database in batches.
func importPositions(ctx context.Context, pool *pgxpool.Pool, positions []TraccarPosition, deviceMap map[int64]int64, config *Config) error {
	slog.Info("importing positions", slog.Int("count", len(positions)))

	if len(positions) == 0 {
		slog.Info("no positions to import")
		return nil
	}

	const batchSize = 500
	imported := 0
	skipped := 0

	for i := 0; i < len(positions); i += batchSize {
		end := i + batchSize
		if end > len(positions) {
			end = len(positions)
		}
		batch := positions[i:end]

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx at batch %d: %w", i/batchSize, err)
		}

		for _, p := range batch {
			motusDeviceID, ok := deviceMap[p.DeviceID]
			if !ok {
				skipped++
				continue
			}

			// Only import valid positions with sensible coordinates
			if !p.Valid || (p.Latitude == 0 && p.Longitude == 0) {
				skipped++
				continue
			}

			// Insert position with proper timestamps from Traccar
			_, err := tx.Exec(ctx, `
				INSERT INTO positions (device_id, latitude, longitude, altitude, speed, course, timestamp, device_time, server_time)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			`, motusDeviceID, p.Latitude, p.Longitude, p.Altitude,
				knotsToKmh(p.Speed), p.Course, p.FixTime, p.DeviceTime, p.ServerTime)

			if err != nil {
				if config.Verbose {
					slog.Debug("failed to insert position",
						slog.Int64("deviceID", motusDeviceID),
						slog.String("fixTime", p.FixTime.Format(time.RFC3339)),
						slog.Any("error", err))
				}
				skipped++
				continue
			}
			imported++
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit batch at %d: %w", i/batchSize, err)
		}

		if (i/batchSize+1)%10 == 0 || end == len(positions) {
			slog.Info("position import progress",
				slog.Int("imported", imported),
				slog.Int("total", len(positions)),
				slog.Int("skipped", skipped))
		}
	}

	slog.Info("positions imported", slog.Int("imported", imported), slog.Int("skipped", skipped))
	return nil
}

// updateDeviceLastUpdate sets each device's last_update to the latest position
// timestamp for that device. This ensures imported devices show a meaningful
// "last seen" value rather than NULL.
func updateDeviceLastUpdate(ctx context.Context, pool *pgxpool.Pool, deviceMap map[int64]int64) error {
	slog.Info("updating device lastUpdate timestamps")

	updated := 0
	for _, motusDeviceID := range deviceMap {
		_, err := pool.Exec(ctx, `
			UPDATE devices
			SET last_update = (
				SELECT MAX(timestamp)
				FROM positions
				WHERE device_id = $1
			)
			WHERE id = $1
		`, motusDeviceID)

		if err != nil {
			slog.Warn("failed to update last_update for device", slog.Int64("deviceID", motusDeviceID), slog.Any("error", err))
			continue
		}
		updated++
	}

	slog.Info("updated device lastUpdate timestamps", slog.Int("updated", updated))
	return nil
}

// logParsedData prints a summary of parsed data in dry-run mode.
func logParsedData(devices []TraccarDevice, positions []TraccarPosition, geofences []TraccarGeofence, calendars []TraccarCalendar, config *Config) {
	slog.Info("=== DRY RUN SUMMARY ===")

	for _, d := range devices {
		hasPositions := false
		for _, p := range positions {
			if p.DeviceID == d.ID {
				hasPositions = true
				break
			}
		}
		slog.Info("device",
			slog.Int64("traccarID", d.ID),
			slog.String("name", d.Name),
			slog.String("uniqueID", d.UniqueID),
			slog.String("model", d.Model),
			slog.String("status", d.Status),
			slog.Bool("hasPositions", hasPositions))
	}

	for _, c := range calendars {
		dataPreview := c.Data[:minInt(60, len(c.Data))]
		slog.Info("calendar",
			slog.Int64("traccarID", c.ID),
			slog.String("name", c.Name),
			slog.String("dataPreview", dataPreview))
	}

	for _, g := range geofences {
		calStr := "none"
		if g.CalendarID != nil {
			calStr = fmt.Sprintf("%d", *g.CalendarID)
		}
		slog.Info("geofence",
			slog.Int64("traccarID", g.ID),
			slog.String("name", g.Name),
			slog.String("calendarID", calStr),
			slog.String("areaPreview", g.Area[:minInt(80, len(g.Area))]))
	}

	if len(positions) > 0 {
		// Find time range
		earliest := positions[0].FixTime
		latest := positions[0].FixTime
		for _, p := range positions {
			if p.FixTime.Before(earliest) {
				earliest = p.FixTime
			}
			if p.FixTime.After(latest) {
				latest = p.FixTime
			}
		}
		slog.Info("positions summary",
			slog.Int("total", len(positions)),
			slog.String("from", earliest.Format("2006-01-02")),
			slog.String("to", latest.Format("2006-01-02")))
	}
}

// importGeofences inserts geofences into the Motus database using PostGIS WKT->geometry conversion.
// Traccar stores WKT in latitude,longitude order but PostGIS expects longitude,latitude.
// We swap the coordinates before inserting, and handle Traccar's CIRCLE format by converting
// it to a buffered point using ST_Buffer.
func importGeofences(ctx context.Context, pool *pgxpool.Pool, geofences []TraccarGeofence, adminID int64, calendarMap map[int64]int64, config *Config) error {
	slog.Info("importing geofences", slog.Int("count", len(geofences)))

	imported := 0
	for _, g := range geofences {
		if config.Verbose {
			slog.Debug("importing geofence",
				slog.String("name", g.Name),
				slog.String("areaPreview", g.Area[:minInt(60, len(g.Area))]))
		}

		if config.DryRun {
			continue
		}

		var geofenceID int64
		var err error

		if isTraccarCircle(g.Area) {
			// Traccar CIRCLE format: CIRCLE (lat lon, radius)
			// Convert to PostGIS buffered point.
			lat, lon, radius, parseErr := parseTraccarCircle(g.Area)
			if parseErr != nil {
				slog.Warn("failed to parse CIRCLE for geofence", slog.String("name", g.Name), slog.Any("error", parseErr))
				continue
			}
			if config.Verbose {
				slog.Debug("converting CIRCLE geofence",
					slog.Float64("lat", lat),
					slog.Float64("lon", lon),
					slog.Float64("radiusM", radius))
			}
			err = pool.QueryRow(ctx, `
				INSERT INTO geofences (name, description, geometry, created_at, updated_at)
				VALUES ($1, $2, ST_FlipCoordinates(ST_Buffer(ST_MakePoint($3, $4)::geography, $5)::geometry), NOW(), NOW())
				RETURNING id
			`, g.Name, g.Description, lon, lat, radius).Scan(&geofenceID)
		} else {
			// POLYGON or other WKT: swap coordinates from Traccar's lat,lon to PostGIS's lon,lat.
			swapped := swapWKTCoordinates(g.Area)
			if config.Verbose {
				slog.Debug("swapped WKT coordinates", slog.String("wktPreview", swapped[:minInt(80, len(swapped))]))
			}
			err = pool.QueryRow(ctx, `
				INSERT INTO geofences (name, description, geometry, created_at, updated_at)
				VALUES ($1, $2, ST_GeomFromText($3, 4326), NOW(), NOW())
				RETURNING id
			`, g.Name, g.Description, swapped).Scan(&geofenceID)
		}

		if err != nil {
			slog.Warn("failed to import geofence", slog.String("name", g.Name), slog.Any("error", err))
			continue
		}

		// Associate with admin user.
		_, err = pool.Exec(ctx, `
			INSERT INTO user_geofences (user_id, geofence_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, adminID, geofenceID)

		if err != nil {
			slog.Warn("failed to associate geofence with admin", slog.Int64("geofenceID", geofenceID), slog.Any("error", err))
		}

		// Link calendar if present.
		if g.CalendarID != nil && calendarMap != nil {
			motusCalID, ok := calendarMap[*g.CalendarID]
			if ok {
				_, err = pool.Exec(ctx, `UPDATE geofences SET calendar_id = $1 WHERE id = $2`, motusCalID, geofenceID)
				if err != nil {
					slog.Warn("failed to link calendar to geofence",
						slog.Int64("calendarID", motusCalID),
						slog.Int64("geofenceID", geofenceID),
						slog.Any("error", err))
				} else if config.Verbose {
					slog.Debug("linked geofence to calendar",
						slog.Int64("geofenceID", geofenceID),
						slog.Int64("calendarID", motusCalID))
				}
			}
		}

		imported++
	}

	slog.Info("geofences imported", slog.Int("imported", imported))
	return nil
}

// importCalendars inserts calendars into the Motus database and returns a mapping
// from Traccar calendar ID to Motus calendar ID.
// Traccar stores calendar data as base64-encoded iCalendar.
func importCalendars(ctx context.Context, pool *pgxpool.Pool, calendars []TraccarCalendar, adminID int64, config *Config) (map[int64]int64, error) {
	slog.Info("importing calendars", slog.Int("count", len(calendars)))

	calendarMap := make(map[int64]int64) // traccar ID -> motus ID
	imported := 0

	for _, c := range calendars {
		if config.Verbose {
			slog.Debug("importing calendar", slog.String("name", c.Name), slog.Int64("traccarID", c.ID))
		}

		// Decode base64 iCalendar data (Traccar stores it encoded).
		// Note: parseCalendar already decoded bytea hex format, so c.Data is readable text.
		icalData := c.Data
		if decoded, err := base64.StdEncoding.DecodeString(c.Data); err == nil {
			icalData = string(decoded)
		} else if config.Verbose {
			slog.Debug("calendar data is not base64-encoded", slog.String("name", c.Name))
		}

		// Normalize malformed Traccar calendars: add UNTIL to RRULE when DTEND
		// represents the series end date rather than a single-event end.
		before := icalData
		icalData = normalizeTraccarCalendar(icalData)
		if config.Verbose && icalData != before {
			slog.Debug("normalized calendar RRULE", slog.String("name", c.Name))
		}

		var motusID int64
		err := pool.QueryRow(ctx, `
			INSERT INTO calendars (user_id, name, data, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			RETURNING id
		`, adminID, c.Name, icalData).Scan(&motusID)

		if err != nil {
			slog.Warn("failed to insert calendar", slog.String("name", c.Name), slog.Any("error", err))
			continue
		}

		calendarMap[c.ID] = motusID

		// Associate with admin user.
		_, err = pool.Exec(ctx, `
			INSERT INTO user_calendars (user_id, calendar_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, adminID, motusID)
		if err != nil {
			slog.Warn("failed to associate calendar with admin", slog.Int64("calendarID", motusID), slog.Any("error", err))
		}

		imported++
	}

	slog.Info("calendars imported", slog.Int("imported", imported))
	return calendarMap, nil
}

// normalizeTraccarCalendar fixes malformed Traccar iCalendar data where DTEND
// is used as the series end date instead of a single-event end. Traccar sets
// DTEND to the date the recurring series should stop, but omits UNTIL from the
// RRULE, producing infinite recurring events.
//
// When the function detects:
//  1. An RRULE exists
//  2. RRULE has no UNTIL and no COUNT
//  3. DTEND - DTSTART > 24 hours (multi-day span indicating a series end)
//
// It adds UNTIL=<DTEND_value> to the RRULE, and adjusts DTEND to DTSTART+24h
// (the actual single-event duration).
//
// If the data cannot be parsed or does not match the pattern, it is returned unchanged.
func normalizeTraccarCalendar(icalData string) string {
	if strings.TrimSpace(icalData) == "" {
		return icalData
	}

	// Detect line ending style.
	lineEnding := "\r\n"
	if !strings.Contains(icalData, "\r\n") {
		lineEnding = "\n"
	}

	lines := strings.Split(icalData, lineEnding)

	// Extract DTSTART, DTEND, and RRULE lines from the first VEVENT.
	var (
		dtstartLine string
		dtstartIdx  int
		dtendLine   string
		dtendIdx    int
		rruleLine   string
		rruleIdx    int
		inEvent     bool
	)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "BEGIN:VEVENT" {
			inEvent = true
			continue
		}
		if trimmed == "END:VEVENT" {
			break // Only process the first VEVENT.
		}
		if !inEvent {
			continue
		}
		if strings.HasPrefix(trimmed, "DTSTART") {
			dtstartLine = trimmed
			dtstartIdx = i
		}
		if strings.HasPrefix(trimmed, "DTEND") {
			dtendLine = trimmed
			dtendIdx = i
		}
		if strings.HasPrefix(trimmed, "RRULE:") {
			rruleLine = trimmed
			rruleIdx = i
		}
	}

	// No RRULE means nothing to fix.
	if rruleLine == "" {
		return icalData
	}

	// If RRULE already has UNTIL or COUNT, don't modify.
	rruleUpper := strings.ToUpper(rruleLine)
	if strings.Contains(rruleUpper, "UNTIL=") || strings.Contains(rruleUpper, "COUNT=") {
		return icalData
	}

	// Parse DTSTART and DTEND values.
	dtstartVal := extractICalTimestamp(dtstartLine)
	dtendVal := extractICalTimestamp(dtendLine)
	if dtstartVal == "" || dtendVal == "" {
		return icalData
	}

	dtstart, err := parseICalTimestamp(dtstartVal)
	if err != nil {
		return icalData
	}
	dtend, err := parseICalTimestamp(dtendVal)
	if err != nil {
		return icalData
	}

	// Only normalize when DTEND - DTSTART > 24 hours (multi-day span).
	if dtend.Sub(dtstart) <= 24*time.Hour {
		return icalData
	}

	// Build the UNTIL value from the DTEND timestamp, preserving its format.
	untilVal := dtendVal

	// Add UNTIL to the RRULE.
	newRRule := rruleLine + ";UNTIL=" + untilVal

	// Adjust DTEND to DTSTART + 24h (the actual event duration for one occurrence).
	newDTEnd := buildAdjustedDTEnd(dtendLine, dtstartLine, dtstart)

	// Replace the lines.
	_ = dtstartIdx // DTSTART stays unchanged.
	lines[rruleIdx] = newRRule
	lines[dtendIdx] = newDTEnd

	return strings.Join(lines, lineEnding)
}

// extractICalTimestamp extracts the timestamp value from a DTSTART or DTEND line.
// Example: "DTSTART;TZID=Europe/Berlin:20251105T200000" -> "20251105T200000"
// Example: "DTEND:20251105T200000Z" -> "20251105T200000Z"
// Example: "DTSTART;VALUE=DATE:20251105" -> "20251105"
func extractICalTimestamp(line string) string {
	// The value is after the last colon.
	idx := strings.LastIndex(line, ":")
	if idx < 0 || idx >= len(line)-1 {
		return ""
	}
	return strings.TrimSpace(line[idx+1:])
}

// parseICalTimestamp parses a bare iCalendar timestamp value (no property prefix).
// Handles: 20251105T200000, 20251105T200000Z, 20251105
func parseICalTimestamp(val string) (time.Time, error) {
	for _, layout := range []string{
		"20060102T150405Z",
		"20060102T150405",
		"20060102",
	} {
		t, err := time.Parse(layout, val)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse iCal timestamp %q", val)
}

// buildAdjustedDTEnd creates a new DTEND line with the time from the original DTEND
// but on the same date as DTSTART (to represent the daily event duration).
// If this would result in DTEND <= DTSTART, uses DTSTART + 24h instead.
// It preserves the DTEND property parameters (TZID, VALUE, etc.).
func buildAdjustedDTEnd(dtendLine, dtstartLine string, dtstart time.Time) string {
	// Extract the original DTEND time (hour, minute, second)
	dtendVal := extractICalTimestamp(dtendLine)
	dtend, err := parseICalTimestamp(dtendVal)
	var newEnd time.Time

	if err != nil {
		// Fallback: use DTSTART + 24h
		newEnd = dtstart.Add(24 * time.Hour)
	} else {
		// Build new end time: DTSTART date + DTEND time
		newEnd = time.Date(
			dtstart.Year(), dtstart.Month(), dtstart.Day(),
			dtend.Hour(), dtend.Minute(), dtend.Second(),
			0, dtstart.Location(),
		)

		// If end <= start on same day, use DTSTART + 24h instead
		if !newEnd.After(dtstart) {
			newEnd = dtstart.Add(24 * time.Hour)
		}
	}

	// Determine the format from the original DTSTART value.
	dtstartVal := extractICalTimestamp(dtstartLine)
	var formatted string
	switch {
	case strings.HasSuffix(dtstartVal, "Z"):
		formatted = newEnd.Format("20060102T150405Z")
	case strings.Contains(dtstartVal, "T"):
		formatted = newEnd.Format("20060102T150405")
	default:
		// Date-only format.
		formatted = newEnd.Format("20060102")
	}

	// Replace the value portion of the DTEND line (everything after the last colon).
	colonIdx := strings.LastIndex(dtendLine, ":")
	if colonIdx < 0 {
		return dtendLine
	}
	return dtendLine[:colonIdx+1] + formatted
}

// --- Helpers ---

// isTraccarCircle checks if the WKT is a Traccar-style CIRCLE.
// Traccar uses: CIRCLE (lat lon, radius)
func isTraccarCircle(wkt string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(wkt)), "CIRCLE")
}

// parseTraccarCircle extracts lat, lon, and radius from Traccar's CIRCLE format.
// Format: CIRCLE (lat lon, radius)
func parseTraccarCircle(wkt string) (lat, lon, radius float64, err error) {
	// Remove "CIRCLE" prefix and parentheses.
	s := strings.TrimSpace(wkt)
	s = strings.TrimPrefix(strings.ToUpper(s), "CIRCLE")
	s = strings.TrimPrefix(s, " ")
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	s = strings.TrimSpace(s)

	// Split into "lat lon" and "radius".
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0, 0, fmt.Errorf("expected 'lat lon, radius' in CIRCLE, got: %s", wkt)
	}

	// Parse lat lon (Traccar order: lat first).
	coords := strings.Fields(strings.TrimSpace(parts[0]))
	if len(coords) != 2 {
		return 0, 0, 0, fmt.Errorf("expected 2 coordinates in CIRCLE center, got %d", len(coords))
	}
	lat, err = strconv.ParseFloat(coords[0], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse lat: %w", err)
	}
	lon, err = strconv.ParseFloat(coords[1], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse lon: %w", err)
	}

	radius, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse radius: %w", err)
	}

	return lat, lon, radius, nil
}

// swapWKTCoordinates swaps coordinate pairs in a WKT string from lat,lon to lon,lat order.
// Traccar stores WKT as POLYGON((lat lon, lat lon, ...)) but PostGIS expects
// POLYGON((lon lat, lon lat, ...)).
func swapWKTCoordinates(wkt string) string {
	// Find the coordinate data between the outermost parentheses.
	// We need to handle nested parens for POLYGON((...)), MULTIPOLYGON(((...)))
	firstParen := strings.Index(wkt, "(")
	if firstParen < 0 {
		return wkt
	}

	prefix := wkt[:firstParen]
	rest := wkt[firstParen:]

	// Process coordinate pairs within the parenthesized section.
	// We strip all parens, split by comma, swap each pair, then reconstruct.
	var result strings.Builder
	result.WriteString(prefix)

	i := 0
	for i < len(rest) {
		ch := rest[i]
		if ch == '(' || ch == ')' || ch == ',' {
			result.WriteByte(ch)
			i++
			continue
		}
		if ch == ' ' && (i == 0 || rest[i-1] == '(' || rest[i-1] == ',') {
			// Leading whitespace after delimiter
			result.WriteByte(ch)
			i++
			continue
		}

		// Read a coordinate pair: "lat lon" (two numbers separated by space)
		j := i
		for j < len(rest) && rest[j] != ',' && rest[j] != ')' {
			j++
		}
		pair := strings.TrimSpace(rest[i:j])
		parts := strings.Fields(pair)
		if len(parts) == 2 {
			// Swap: lat lon -> lon lat
			result.WriteString(parts[1])
			result.WriteByte(' ')
			result.WriteString(parts[0])
		} else {
			// Not a coordinate pair, write as-is
			result.WriteString(pair)
		}
		i = j
	}

	return result.String()
}

// nullStr converts PostgreSQL COPY \N (null) to empty string.
func nullStr(s string) string {
	if s == `\N` {
		return ""
	}
	return s
}

// nullToNil converts an empty string to nil (SQL NULL) for nullable columns.
// Non-empty strings are returned as a *string pointer.
func nullToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// parseTimestamp parses a PostgreSQL timestamp string.
func parseTimestamp(s string) (time.Time, error) {
	if s == `\N` || s == "" {
		return time.Time{}, fmt.Errorf("null timestamp")
	}
	// Try common formats
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
	} {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp format: %q", s)
}

// knotsToKmh converts speed from knots (Traccar) to km/h (Motus).
func knotsToKmh(knots float64) float64 {
	return knots * 1.852
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// geocodeRecentPositions reverse-geocodes recently imported positions that don't have addresses.
// It uses the Nominatim geocoder with rate limiting (1 req/sec per OSM policy).
// If RecentDays is set, only geocodes positions within that time range (matching import filter).
func geocodeRecentPositions(ctx context.Context, pool *pgxpool.Pool, config *Config) error {
	slog.Info("reverse geocoding positions without addresses", slog.Int("limit", config.GeocodeLastN))

	// Build query with optional time filter to match imported data range
	query := `
		SELECT id, latitude, longitude
		FROM positions
		WHERE (address IS NULL OR address = '')
	`
	var args []interface{}
	argNum := 1

	// If RecentDays is set, only geocode positions within that range (matching import scope)
	if config.RecentDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -config.RecentDays)
		query += fmt.Sprintf(" AND timestamp >= $%d", argNum)
		args = append(args, cutoff)
		argNum++
		slog.Info("filtering geocode to recent positions", slog.String("after", cutoff.Format("2006-01-02")))
	}

	query += fmt.Sprintf(`
		ORDER BY speed ASC NULLS FIRST, timestamp DESC
		LIMIT $%d
	`, argNum)
	args = append(args, config.GeocodeLastN)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query positions: %w", err)
	}
	defer rows.Close()

	type positionToGeocode struct {
		id  int64
		lat float64
		lon float64
	}

	var positions []positionToGeocode
	for rows.Next() {
		var p positionToGeocode
		if err := rows.Scan(&p.id, &p.lat, &p.lon); err != nil {
			return fmt.Errorf("scan position: %w", err)
		}
		positions = append(positions, p)
	}

	if len(positions) == 0 {
		slog.Info("no positions to geocode (all have addresses)")
		return nil
	}

	slog.Info("positions to geocode", slog.Int("count", len(positions)))

	// Create geocoder with OSM Nominatim (1 req/sec rate limit)
	geocoder := geocoding.NewNominatimGeocoder(geocoding.NominatimConfig{
		URL:       "https://nominatim.openstreetmap.org/reverse",
		RateLimit: 1.0,
		Timeout:   10 * time.Second,
		UserAgent: "Motus GPS Tracker Import Tool (https://github.com/tamcore/motus)",
	})
	geocoder.SetLogger(slog.Default())

	geocoded := 0
	failed := 0

	for i, p := range positions {
		if config.Verbose && i%10 == 0 {
			slog.Debug("geocoding progress", slog.Int("current", i), slog.Int("total", len(positions)))
		}

		address, err := geocoder.ReverseGeocode(ctx, p.lat, p.lon)
		if err != nil {
			if config.Verbose {
				slog.Debug("geocoding failed for position",
					slog.Int64("positionID", p.id),
					slog.Float64("lat", p.lat),
					slog.Float64("lon", p.lon),
					slog.Any("error", err))
			}
			failed++
			// Use coordinate fallback
			address = fmt.Sprintf("%.5f, %.5f", p.lat, p.lon)
		}

		// Update position with address
		result, err := pool.Exec(ctx, `
			UPDATE positions
			SET address = $1
			WHERE id = $2
		`, address, p.id)
		if err != nil {
			if config.Verbose {
				slog.Debug("failed to update address for position",
					slog.Int64("positionID", p.id),
					slog.Any("error", err))
			}
			failed++
			continue
		}

		rowsAffected := result.RowsAffected()
		if rowsAffected == 0 {
			if config.Verbose {
				slog.Debug("UPDATE affected 0 rows for position", slog.Int64("positionID", p.id))
			}
			failed++
		} else {
			geocoded++
			if config.Verbose && geocoded%10 == 0 {
				slog.Debug("geocoded positions", slog.Int("geocoded", geocoded), slog.Int("total", len(positions)))
			}
		}
	}

	slog.Info("geocoding complete", slog.Int("successful", geocoded), slog.Int("failed", failed))
	return nil
}
