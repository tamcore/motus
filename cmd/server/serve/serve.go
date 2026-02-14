// Package serve implements the Motus HTTP and GPS protocol servers.
package serve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/config"
	"github.com/tamcore/motus/internal/demo"
	"github.com/tamcore/motus/internal/geocoding"
	"github.com/tamcore/motus/internal/logger"
	"github.com/tamcore/motus/internal/protocol"
	"github.com/tamcore/motus/internal/pubsub"
	"github.com/tamcore/motus/internal/services"
	"github.com/tamcore/motus/internal/storage/partition"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// shareTokenAdapter wraps a DeviceShareRepository to implement
// websocket.ShareTokenValidator. This adapter avoids an import cycle between
// the websocket and repository packages.
type shareTokenAdapter struct {
	shares *repository.DeviceShareRepository
}

func (a *shareTokenAdapter) ValidateShareToken(ctx context.Context, token string) (int64, error) {
	share, err := a.shares.GetByToken(ctx, token)
	if err != nil || share == nil {
		return 0, err
	}
	return share.DeviceID, nil
}

// Run starts the Motus server (HTTP API, GPS protocol listeners, background
// services) and blocks until a SIGINT or SIGTERM is received.
func Run() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.Error("failed to load config", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize structured logger and set as the process-wide default.
	// This ensures any code using slog.Info/slog.Error/etc. picks up our
	// format and level configuration.
	logFormat := cfg.Log.Format
	if logFormat == "" {
		logFormat = logger.FormatForEnv(cfg.Security.Env)
	}
	appLogger := logger.New(logger.Options{
		Level:  cfg.Log.Level,
		Format: logFormat,
	})
	slog.SetDefault(appLogger)

	slog.Info("logger initialized",
		slog.String("level", cfg.Log.Level),
		slog.String("format", logFormat),
	)

	// Database connection with pool configuration.
	poolCfg, err := pgxpool.ParseConfig(cfg.Database.URL())
	if err != nil {
		slog.Error("failed to parse database URL", slog.Any("error", err))
		os.Exit(1)
	}
	poolCfg.MaxConns = cfg.Database.Pool.MaxConns
	poolCfg.MinConns = cfg.Database.Pool.MinConns
	poolCfg.MaxConnLifetime = cfg.Database.Pool.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.Database.Pool.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		slog.Error("failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		slog.Error("failed to ping database", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("connected to database",
		slog.Int("maxConns", int(cfg.Database.Pool.MaxConns)),
		slog.Int("minConns", int(cfg.Database.Pool.MinConns)),
		slog.String("maxConnLifetime", cfg.Database.Pool.MaxConnLifetime.String()),
		slog.String("maxConnIdleTime", cfg.Database.Pool.MaxConnIdleTime.String()),
	)

	// Repositories.
	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	positionRepo := repository.NewPositionRepository(pool)
	commandRepo := repository.NewCommandRepository(pool)
	geofenceRepo := repository.NewGeofenceRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	notificationRepo := repository.NewNotificationRepository(pool)
	shareRepo := repository.NewDeviceShareRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	// Notification service (needs to be created before geofence event service).
	notificationService := services.NewNotificationService(notificationRepo, deviceRepo, geofenceRepo, positionRepo)

	calendarRepo := repository.NewCalendarRepository(pool)

	// OIDC state repository.
	oidcStateRepo := repository.NewOIDCStateRepository(pool)

	// Statistics repository.
	statsRepo := repository.NewStatisticsRepository(pool)

	// Audit logger.
	auditLogger := audit.NewLogger(pool)

	// Handlers.
	serverHandler := handlers.NewServerHandler()
	sessionHandler := handlers.NewSessionHandler(userRepo, sessionRepo, apiKeyRepo)
	deviceHandler := handlers.NewDeviceHandler(deviceRepo, cfg.Device.UniqueIDPrefix)
	positionHandler := handlers.NewPositionHandler(positionRepo, deviceRepo)
	deviceRegistry := protocol.NewDeviceRegistry()
	commandHandler := handlers.NewCommandHandler(commandRepo, deviceRepo, deviceRegistry, protocol.NewEncoderRegistry())
	commandHandler.SetAuditLogger(auditLogger)
	geofenceHandler := handlers.NewGeofenceHandler(geofenceRepo)
	eventHandler := handlers.NewEventHandler(eventRepo, deviceRepo)
	reportsHandler := handlers.NewReportsHandler(eventRepo, deviceRepo)
	notificationHandler := handlers.NewNotificationHandler(notificationRepo, notificationService)
	userHandler := handlers.NewUserHandler(userRepo, deviceRepo, cfg.Device.UniqueIDPrefix)
	gpxHandler := handlers.NewGPXImportHandler(deviceRepo, positionRepo, auditLogger)
	shareHandler := handlers.NewShareHandler(shareRepo, deviceRepo, positionRepo, cfg.Device.UniqueIDPrefix)
	sudoHandler := handlers.NewSudoHandler(userRepo, sessionRepo)
	sudoHandler.SetAuditLogger(auditLogger)
	sessionHandler.SetAuditLogger(auditLogger)
	deviceHandler.SetAuditLogger(auditLogger)
	geofenceHandler.SetAuditLogger(auditLogger)
	notificationHandler.SetAuditLogger(auditLogger)
	userHandler.SetAuditLogger(auditLogger)
	shareHandler.SetAuditLogger(auditLogger)
	statisticsHandler := handlers.NewStatisticsHandler(statsRepo)
	auditHandler := handlers.NewAuditHandler(auditLogger)
	calendarHandler := handlers.NewCalendarHandler(calendarRepo)
	apiKeyHandler := handlers.NewApiKeyHandler(apiKeyRepo)
	calendarHandler.SetAuditLogger(auditLogger)
	apiKeyHandler.SetAuditLogger(auditLogger)

	// OIDC authentication (optional).
	oidcConfigFn := func(w http.ResponseWriter, r *http.Request) {
		api.RespondJSON(w, http.StatusOK, map[string]bool{"enabled": false})
	}
	var oidcLoginFn, oidcCallbackFn http.HandlerFunc
	if cfg.OIDC.Enabled {
		oidcHandler, err := handlers.NewOIDCHandler(context.Background(), cfg.OIDC, userRepo, sessionRepo, oidcStateRepo)
		if err != nil {
			slog.Error("failed to initialize OIDC provider", slog.Any("error", err))
			os.Exit(1)
		}
		oidcHandler.SetAuditLogger(auditLogger)
		oidcConfigFn = oidcHandler.GetConfig
		oidcLoginFn = oidcHandler.Login
		oidcCallbackFn = oidcHandler.Callback
		slog.Info("OIDC authentication enabled",
			slog.String("issuer", cfg.OIDC.Issuer),
			slog.Bool("signupEnabled", cfg.OIDC.SignupEnabled),
		)
	}

	// Redis pub/sub for cross-pod WebSocket broadcasting (optional).
	var redisPubSub pubsub.PubSub
	if cfg.Redis.Enabled && cfg.Redis.URL != "" {
		ps, err := pubsub.NewRedisPubSub(cfg.Redis.URL, "motus:updates")
		if err != nil {
			slog.Warn("Redis unavailable, continuing without cross-pod broadcasting",
				slog.Any("error", err),
			)
		} else {
			redisPubSub = ps
			defer func() { _ = ps.Close() }()
			slog.Info("Redis pub/sub enabled for cross-pod broadcasting")
		}
	}

	// WebSocket hub with origin validation and per-user filtering.
	// Since /api/socket is outside auth middleware, we must parse session cookie manually.
	hub := websocket.NewHub(cfg.WebSocket.AllowedOrigins, deviceRepo, func(r *http.Request) int64 {
		// Try context first (in case request went through auth middleware)
		user := api.UserFromContext(r.Context())
		if user != nil {
			return user.ID
		}

		// Parse session cookie manually (since outside auth middleware)
		cookie, err := r.Cookie("session_id")
		if err != nil {
			return 0
		}

		session, err := sessionRepo.GetByID(r.Context(), cookie.Value)
		if err != nil || session == nil {
			return 0
		}

		return session.UserID
	})
	if redisPubSub != nil {
		hub.SetPubSub(redisPubSub)
	}
	hub.SetShareTokenValidator(&shareTokenAdapter{shares: shareRepo})
	if cfg.Security.Env == "development" {
		hub.SetDevelopmentMode(true)
	}
	userHandler.SetCacheInvalidator(hub)

	// Inject structured logger into components that produce high-value logs.
	wsLogger := appLogger.With(slog.String("component", "websocket"))
	hub.SetLogger(wsLogger)

	protoLogger := appLogger.With(slog.String("component", "protocol"))
	svcLogger := appLogger.With(slog.String("component", "services"))

	// Auth middleware.
	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)

	// Router.
	h := api.Handlers{
		GetServer:         serverHandler.GetServer,
		Login:             sessionHandler.Login,
		GetCurrentSession: sessionHandler.GetCurrentSession,
		GenerateToken:     sessionHandler.GenerateToken,
		Logout:            sessionHandler.Logout,
		ListDevices:       deviceHandler.List,
		GetDevice:         deviceHandler.Get,
		CreateDevice:      deviceHandler.Create,
		UpdateDevice:      deviceHandler.Update,
		DeleteDevice:      deviceHandler.Delete,
		GetPositions:      positionHandler.GetPositions,
		CreateCommand:     commandHandler.Create,
		SendCommand:       commandHandler.Send,
		GetCmdTypes:       commandHandler.GetTypes,
		ListCommands:      commandHandler.List,
		ListGeofences:     geofenceHandler.List,
		GetGeofence:       geofenceHandler.Get,
		CreateGeofence:    geofenceHandler.Create,
		UpdateGeofence:    geofenceHandler.Update,
		DeleteGeofence:    geofenceHandler.Delete,
		ListEvents:        eventHandler.List,
		ReportEvents:      reportsHandler.GetEvents,

		ListNotifications:  notificationHandler.List,
		CreateNotification: notificationHandler.Create,
		UpdateNotification: notificationHandler.Update,
		DeleteNotification: notificationHandler.Delete,
		TestNotification:   notificationHandler.Test,
		NotificationLogs:   notificationHandler.Logs,

		ImportGPX: gpxHandler.Import,

		CreateShare:     shareHandler.CreateShare,
		ListShares:      shareHandler.ListShares,
		DeleteShare:     shareHandler.DeleteShare,
		GetSharedDevice: shareHandler.GetSharedDevice,

		ListUsers:           userHandler.List,
		CreateUser:          userHandler.Create,
		UpdateUser:          userHandler.Update,
		DeleteUser:          userHandler.Delete,
		ListUserDevs:        userHandler.ListDevices,
		AdminListAllDevices: userHandler.AdminListAllDevices,
		AssignDevice:        userHandler.AssignDevice,
		UnassignDevice:      userHandler.UnassignDevice,

		AdminListAllGeofences:     geofenceHandler.AdminListAll,
		AdminListAllCalendars:     calendarHandler.AdminListAll,
		AdminListAllNotifications: notificationHandler.AdminListAll,
		AdminGetAllPositions:      positionHandler.AdminGetAllPositions,

		StartSudo:     sudoHandler.StartSudo,
		EndSudo:       sudoHandler.EndSudo,
		GetSudoStatus: sudoHandler.GetSudoStatus,

		GetPlatformStats: statisticsHandler.GetPlatformStats,
		GetUserStats:     statisticsHandler.GetUserStats,

		GetAuditLog: auditHandler.GetAuditLog,

		ListCalendars:  calendarHandler.List,
		CreateCalendar: calendarHandler.Create,
		UpdateCalendar: calendarHandler.Update,
		DeleteCalendar: calendarHandler.Delete,
		CheckCalendar:  calendarHandler.Check,

		CreateApiKey:      apiKeyHandler.Create,
		ListApiKeys:       apiKeyHandler.List,
		DeleteApiKey:      apiKeyHandler.Delete,
		AdminListUserKeys: apiKeyHandler.AdminListUserKeys,

		ListSessions:       sessionHandler.ListSessions,
		DeleteSession:      sessionHandler.DeleteSession,
		AdminDeleteSession: sessionHandler.AdminDeleteSession,

		OIDCConfig:   oidcConfigFn,
		OIDCLogin:    oidcLoginFn,
		OIDCCallback: oidcCallbackFn,
	}
	adminMW := middleware.RequireAdmin

	// CSRF protection: load or generate the 32-byte secret key.
	csrfSecret := loadCSRFSecret(cfg.Security.CSRFSecret)
	csrfSecure := cfg.Security.Env != "development"

	routerCfg := api.RouterConfig{
		LoginRateLimit:  middleware.LoginRateLimit(),
		APIRateLimit:    middleware.APIRateLimit(),
		SecurityHeaders: middleware.SecurityHeaders,
		WriteAccess:     middleware.RequireWriteAccess,
		Logger:          middleware.Logger,
		CSRFProtect: middleware.CSRF(middleware.CSRFConfig{
			Secret: csrfSecret,
			Secure: csrfSecure,
		}),
	}
	router := api.NewRouter(h, authMW, adminMW, hub, routerCfg)

	// Geofence event detection service.
	geofenceEventService := services.NewGeofenceEventService(geofenceRepo, eventRepo, deviceRepo, positionRepo, hub, notificationService)
	geofenceEventService.SetCalendarRepo(calendarRepo)
	geofenceEventService.SetLogger(svcLogger)

	// Overspeed detection service.
	overspeedService := services.NewOverspeedService(eventRepo, hub, notificationService)
	overspeedService.SetLogger(svcLogger)

	// Motion detection service.
	motionService := services.NewMotionService(positionRepo, eventRepo, hub, notificationService)
	motionService.SetLogger(svcLogger)

	// Ignition detection service.
	ignitionService := services.NewIgnitionService(positionRepo, eventRepo, hub, notificationService)
	ignitionService.SetLogger(svcLogger)

	// Alarm detection service (SOS, power cut, vibration, overspeed from H02 flags).
	alarmService := services.NewAlarmService(eventRepo, hub, notificationService)
	alarmService.SetLogger(svcLogger)

	// Reverse geocoding service (optional, enabled by default).
	var cachedGeocoder *geocoding.CachedGeocoder
	if cfg.Geocoding.Enabled {
		nominatim := geocoding.NewNominatimGeocoder(geocoding.NominatimConfig{
			URL:       cfg.Geocoding.URL,
			RateLimit: cfg.Geocoding.RateLimit,
		})
		geocodeLogger := appLogger.With(slog.String("component", "geocoding"))
		nominatim.SetLogger(geocodeLogger)

		cachedGeocoder = geocoding.NewCachedGeocoder(nominatim, cfg.Geocoding.CacheTTL)
		cachedGeocoder.SetLogger(geocodeLogger)

		slog.Info("geocoding enabled",
			slog.String("provider", cfg.Geocoding.Provider),
			slog.String("cacheTTL", cfg.Geocoding.CacheTTL.String()),
			slog.Float64("rateLimit", cfg.Geocoding.RateLimit),
		)
	}

	// Idle detection service.
	idleService := services.NewIdleService(deviceRepo, positionRepo, eventRepo, hub, notificationService)
	idleService.SetLogger(svcLogger)
	if cachedGeocoder != nil {
		idleService.SetGeocoder(cachedGeocoder, positionRepo)
	}

	// Mileage tracking service.
	mileageService := services.NewMileageService(positionRepo, deviceRepo, eventRepo, hub, notificationService)
	mileageService.SetLogger(svcLogger)
	idleService.SetMileageService(mileageService)

	// GPS protocol position handler (stores positions and broadcasts via WebSocket).
	gpsHandler := protocol.NewPositionHandler(positionRepo, deviceRepo, hub, geofenceEventService)
	gpsHandler.SetOverspeedChecker(overspeedService)
	gpsHandler.SetMotionChecker(motionService)
	gpsHandler.SetIgnitionChecker(ignitionService)
	gpsHandler.SetAlarmChecker(alarmService)
	gpsHandler.SetMileageChecker(mileageService)
	gpsHandler.SetLogger(protoLogger)
	if cachedGeocoder != nil {
		gpsHandler.SetAddressLookup(cachedGeocoder)
	}

	// Background context for long-running services and protocol servers.
	gpsCtx, gpsCancel := context.WithCancel(context.Background())

	// Start Redis subscriber to relay messages from other pods to local clients.
	go hub.StartSubscriber(gpsCtx)

	// Start geocoding cache cleanup (if enabled).
	if cachedGeocoder != nil {
		go cachedGeocoder.StartCleanup(gpsCtx, 5*time.Minute)
	}

	// GPS protocol TCP servers.

	// Device auto-creation configuration for GPS protocol servers.
	autoCreateCfg := protocol.AutoCreateConfig{
		Enabled:          cfg.Device.AutoCreateDevices,
		DefaultUserEmail: cfg.Device.AutoCreateDefaultUser,
	}
	// In demo pod mode, force auto-create on so the simulator's devices are
	// registered to the demo user when they first send a position.
	if cfg.Demo.Enabled && os.Getenv("MOTUS_DEMO_POD") == "true" {
		autoCreateCfg.Enabled = true
		autoCreateCfg.DefaultUserEmail = "demo@motus.local"
	}
	if autoCreateCfg.Enabled {
		slog.Info("device auto-creation enabled",
			slog.String("defaultUser", autoCreateCfg.DefaultUserEmail),
		)
	}

	h02Server := protocol.NewH02Server(cfg.GPS.H02Port, deviceRepo, gpsHandler)
	h02Server.SetAutoCreate(autoCreateCfg, userRepo)
	h02Server.SetRegistry(deviceRegistry)
	h02Server.SetCommandRepo(commandRepo)
	if cfg.GPS.H02RelayTarget != "" {
		h02Server.SetRelay(cfg.GPS.H02RelayTarget)
		slog.Info("H02 relay enabled", slog.String("target", cfg.GPS.H02RelayTarget))
	}
	h02Server.SetLogger(protoLogger.With(slog.String("protocol", "h02")))
	go func() {
		if err := h02Server.Start(gpsCtx); err != nil {
			slog.Error("H02 server error", slog.Any("error", err))
		}
	}()

	watchServer := protocol.NewWatchServer(cfg.GPS.WatchPort, deviceRepo, gpsHandler)
	watchServer.SetAutoCreate(autoCreateCfg, userRepo)
	if cfg.GPS.WatchRelayTarget != "" {
		watchServer.SetRelay(cfg.GPS.WatchRelayTarget)
		slog.Info("WATCH relay enabled", slog.String("target", cfg.GPS.WatchRelayTarget))
	}
	watchServer.SetLogger(protoLogger.With(slog.String("protocol", "watch")))
	go func() {
		if err := watchServer.Start(gpsCtx); err != nil {
			slog.Error("WATCH server error", slog.Any("error", err))
		}
	}()

	// Background command dispatcher: delivers pending commands to locally online
	// devices. Runs on every replica so the pod that holds a device's TCP
	// connection will always pick up commands saved by any pod.
	dispatcher := protocol.NewCommandDispatcher(deviceRegistry, commandRepo, deviceRepo, protocol.NewEncoderRegistry())
	dispatcher.SetLogger(protoLogger.With(slog.String("component", "dispatcher")))
	go dispatcher.Start(gpsCtx)

	// Device timeout monitor marks devices offline after inactivity.
	timeoutService := services.NewDeviceTimeoutService(
		deviceRepo, hub,
		cfg.Device.Timeout(), cfg.Device.CheckInterval(),
	)
	timeoutService.SetLogger(svcLogger)
	go timeoutService.Start(gpsCtx)

	// Idle detection service runs as a background task.
	go idleService.Start(gpsCtx)

	// Partition manager for positions table: creates future partitions and
	// optionally drops expired ones based on retention configuration.
	partitionMgr := partition.NewManager(pool, cfg.Positions.RetentionDays, 1*time.Hour)
	partitionMgr.SetLogger(appLogger.With(slog.String("component", "partition")))
	go partitionMgr.Start(gpsCtx)

	// Cleanup service: removes expired sessions and device shares to prevent
	// unbounded table growth. Runs daily.
	cleanupService := services.NewCleanupService(pool, 24*time.Hour)
	cleanupService.SetLogger(svcLogger)
	go cleanupService.Start(gpsCtx)

	// Demo mode: enable demo account protection. Seeding, reset, and GPS
	// simulation only run in the dedicated demo pod (MOTUS_DEMO_POD=true).
	if cfg.Demo.Enabled {
		demo.Enable()
		slog.Info("demo mode enabled")

		isDemoPod := os.Getenv("MOTUS_DEMO_POD") == "true"

		if isDemoPod {
			slog.Info("demo pod mode: seeding data and starting simulator")

			demoService := demo.NewService(pool, cfg.Demo.ResetTime)

			// Seed demo accounts and devices if they don't already exist.
			if err := demoService.SeedIfNeeded(context.Background()); err != nil {
				slog.Error("failed to seed demo data", slog.Any("error", err))
				os.Exit(1)
			}

			// Start periodic database reset.
			go demoService.Start(gpsCtx)

			// Load GPX routes and start the GPS simulator.
			// Give the H02 server a moment to start accepting connections.
			go func() {
				time.Sleep(2 * time.Second)

				routes, err := demo.LoadRoutes(cfg.Demo.GPXDir)
				if err != nil {
					slog.Error("failed to load GPX routes",
						slog.String("dir", cfg.Demo.GPXDir),
						slog.Any("error", err),
					)
					return
				}
				slog.Info("loaded GPX routes",
					slog.Int("count", len(routes)),
				)

				// Smooth routes: estimate speeds, interpolate gaps, smooth transitions.
				// Use configured interpolation interval for point density.
				interpInterval := cfg.Demo.InterpolationInterval
				if interpInterval <= 0 {
					interpInterval = 100.0 // default 100m
				}
				slog.Debug("demo interpolation interval",
					slog.Float64("intervalMeters", interpInterval),
				)
				for i, r := range routes {
					routes[i] = demo.SmoothRouteWithInterval(r, interpInterval)
				}

				for _, r := range routes {
					slog.Debug("demo route loaded",
						slog.String("name", r.Name),
						slog.Int("points", len(r.Points)),
						slog.Float64("distanceKm", r.TotalDistance()),
					)
				}

				simulator := demo.NewSimulator(routes, cfg.Demo.H02Target, cfg.Demo.DeviceIMEIs, cfg.Demo.SpeedMultiplier)
				slog.Info("starting GPS simulation",
					slog.Any("devices", cfg.Demo.DeviceIMEIs),
				)
				simulator.Start(gpsCtx)
			}()
		} else {
			slog.Info("demo mode: account protection active (seeding/simulator disabled, not demo pod)")
		}
	}

	// Start Prometheus metrics server on a separate port.
	if cfg.Metrics.Enabled {
		metricsAddr := fmt.Sprintf(":%s", cfg.Metrics.Port)
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		go func() {
			slog.Info("metrics server listening", slog.String("addr", metricsAddr))
			if err := http.ListenAndServe(metricsAddr, metricsMux); err != nil && err != http.ErrServerClosed {
				slog.Error("metrics server error", slog.Any("error", err))
			}
		}()
	}

	// HTTP server with graceful shutdown.
	// WriteTimeout must be 0 because WebSocket connections are long-lived.
	// A non-zero WriteTimeout sets a deadline on the underlying net.Conn
	// before the handler runs; after that deadline expires, Go's net/http
	// closes the connection -- killing any active WebSocket. We use
	// ReadHeaderTimeout to defend against slowloris attacks instead.
	srv := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("starting HTTP server", slog.String("addr", ":"+cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server")

	// Stop GPS protocol servers first (they have long-lived connections).
	gpsCancel()

	// Shutdown HTTP server with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("server stopped")
}

// parseCSRFSecret decodes a hex-encoded 32-byte CSRF secret. Returns an error
// on invalid hex or wrong length.
func parseCSRFSecret(hexSecret string) ([]byte, error) {
	secret, err := hex.DecodeString(hexSecret)
	if err != nil {
		return nil, fmt.Errorf("MOTUS_CSRF_SECRET is not valid hex: %w", err)
	}
	if len(secret) != 32 {
		return nil, fmt.Errorf("MOTUS_CSRF_SECRET must be exactly 32 bytes (64 hex chars), got %d bytes", len(secret))
	}
	return secret, nil
}

// loadCSRFSecret decodes a hex-encoded CSRF secret from the environment, or
// generates a random 32-byte key if the secret is empty. A generated key means
// CSRF tokens will not survive server restarts, which is acceptable for single
// instances but not for multi-pod deployments.
func loadCSRFSecret(hexSecret string) []byte {
	if hexSecret != "" {
		secret, err := parseCSRFSecret(hexSecret)
		if err != nil {
			slog.Error("invalid CSRF secret", slog.Any("error", err))
			os.Exit(1)
		}
		return secret
	}

	// Generate a random 32-byte key.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		slog.Error("failed to generate CSRF secret", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Warn("no MOTUS_CSRF_SECRET set, using random key (tokens will not survive restarts)")
	return secret
}
