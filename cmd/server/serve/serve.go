// Package serve implements the Motus HTTP and GPS protocol servers.
package serve

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	redislib "github.com/redis/go-redis/v9"
	aiChat "github.com/tamcore/motus/internal/ai/chat"
	aiMCP "github.com/tamcore/motus/internal/ai/mcp"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/config"
	"github.com/tamcore/motus/internal/demo"
	"github.com/tamcore/motus/internal/geocoding"
	"github.com/tamcore/motus/internal/logger"
	"github.com/tamcore/motus/internal/notification"
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

	// Configure the SSRF allowlist for webhook destinations on internal
	// networks. Must run before any notification service is created so the
	// validator and runtime dialer both pick up the configured hosts.
	notification.SetAllowedHosts(cfg.Security.WebhookAllowedHosts)
	if len(cfg.Security.WebhookAllowedHosts) > 0 {
		slog.Info("webhook SSRF allowlist configured",
			slog.Any("hosts", cfg.Security.WebhookAllowedHosts),
		)
	}

	// Notification service (needs to be created before geofence event service).
	notificationService := services.NewNotificationService(notificationRepo, deviceRepo, geofenceRepo, positionRepo)

	calendarRepo := repository.NewCalendarRepository(pool)

	// OIDC state repository.
	oidcStateRepo := repository.NewOIDCStateRepository(pool)

	// Statistics repository.
	statsRepo := repository.NewStatisticsRepository(pool)

	// Audit logger.
	auditLogger := audit.NewLogger(pool)

	// Geofence service (shared by OAS handler and AI MCP tools).
	geofenceService := services.NewGeofenceService(geofenceRepo, auditLogger)

	// Device registry (used by protocol servers below).
	deviceRegistry := protocol.NewDeviceRegistry()

	if cfg.OIDC.Enabled {
		slog.Info("OIDC authentication enabled",
			slog.String("issuer", cfg.OIDC.Issuer),
			slog.Bool("signupEnabled", cfg.OIDC.SignupEnabled),
		)
	}

	// Redis client (shared across pub/sub and rate limiting when enabled).
	var redisClient *redislib.Client
	if cfg.Redis.Enabled && cfg.Redis.URL != "" {
		rc, err := pubsub.NewRedisClient(cfg.Redis.URL)
		if err != nil {
			slog.Warn("Redis unavailable, continuing without cross-pod features",
				slog.Any("error", err),
			)
		} else {
			redisClient = rc
			defer func() { _ = rc.Close() }()
			slog.Info("Redis connected")
		}
	}

	// Redis pub/sub for cross-pod WebSocket broadcasting (optional).
	var redisPubSub pubsub.PubSub
	if redisClient != nil {
		ps, err := pubsub.NewRedisPubSubFromClient(redisClient, "motus:updates")
		if err != nil {
			slog.Warn("Redis pub/sub setup failed", slog.Any("error", err))
		} else {
			redisPubSub = ps
			slog.Info("Redis pub/sub enabled for cross-pod broadcasting")
		}
	}

	// Redis pub/sub for cross-pod device-access cache invalidation (optional).
	var redisInvalidationPubSub pubsub.PubSub
	if redisClient != nil {
		ps, err := pubsub.NewRedisPubSubFromClient(redisClient, cfg.Redis.InvalidationChannel)
		if err != nil {
			slog.Warn("Redis cache-invalidation pub/sub setup failed", slog.Any("error", err))
		} else {
			redisInvalidationPubSub = ps
			slog.Info("Redis pub/sub enabled for cross-pod cache invalidation")
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
	if redisInvalidationPubSub != nil {
		hub.SetInvalidationPubSub(redisInvalidationPubSub)
	}
	hub.SetShareTokenValidator(&shareTokenAdapter{shares: shareRepo})
	hub.SetAdminChecker(func(ctx context.Context, userID int64) bool {
		user, err := userRepo.GetByID(ctx, userID)
		if err != nil || user == nil {
			return false
		}
		return user.IsAdmin()
	})
	if cfg.Security.Env == "development" {
		hub.SetDevelopmentMode(true)
	}
	// Inject structured logger into components that produce high-value logs.
	wsLogger := appLogger.With(slog.String("component", "websocket"))
	hub.SetLogger(wsLogger)

	protoLogger := appLogger.With(slog.String("component", "protocol"))
	svcLogger := appLogger.With(slog.String("component", "services"))

	// Unified API handler.
	handler := handlers.NewHandler(handlers.HandlerConfig{
		Users:               userRepo,
		Sessions:            sessionRepo,
		Devices:             deviceRepo,
		Positions:           positionRepo,
		Commands:            commandRepo,
		Geofences:           geofenceRepo,
		Events:              eventRepo,
		Notifications:       notificationRepo,
		Shares:              shareRepo,
		ApiKeys:             apiKeyRepo,
		Calendars:           calendarRepo,
		Stats:               statsRepo,
		OIDCStateRepo:       oidcStateRepo,
		NotificationService: notificationService,
		GeofenceService:     geofenceService,
		DeviceRegistry:      deviceRegistry,
		EncoderRegistry:     protocol.NewEncoderRegistry(),
		Hub:                 hub,
		AuditLogger:         auditLogger,
		UniqueIDPrefix:      cfg.Device.UniqueIDPrefix,
		OIDCConfig:          cfg.OIDC,
	})
	secHandler := handlers.NewSecurityHandler(sessionRepo, apiKeyRepo, userRepo)

	// CSRF protection: load or generate the 32-byte secret key.
	csrfSecret := loadCSRFSecret(cfg.Security.CSRFSecret, cfg.Security.Env)
	csrfSecure := cfg.Security.Env != "development"

	// Login rate limiter: Redis-backed (cluster-wide) when Redis is available,
	// in-process (per-pod only) otherwise.
	loginRateLimit := middleware.LoginRateLimit()
	if redisClient != nil {
		loginRateLimit = middleware.NewRedisLoginRateLimit(redisClient, middleware.DefaultLoginRateLimit())
	} else if cfg.Redis.Enabled {
		slog.Warn("Redis enabled but unavailable — login rate limit is per-pod only")
	}

	// Reverse geocoding (moved up so the AI block can reuse the nominatim instance).
	var cachedGeocoder *geocoding.CachedGeocoder
	var forwardGeocoder geocoding.ForwardGeocoder
	if cfg.Geocoding.Enabled || cfg.AI.Enabled {
		nominatim := geocoding.NewNominatimGeocoder(geocoding.NominatimConfig{
			URL:       cfg.Geocoding.URL,
			RateLimit: cfg.Geocoding.RateLimit,
		})
		if cfg.Geocoding.Enabled {
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
		forwardGeocoder = nominatim
	}
	var chatHandler http.Handler
	if cfg.AI.Enabled {
		calendarService := services.NewCalendarService(calendarRepo, auditLogger)
		mcpSrv := aiMCP.NewServer(aiMCP.Deps{
			Devices:         deviceRepo,
			Positions:       positionRepo,
			Events:          eventRepo,
			Geofences:       geofenceRepo,
			GeofenceService: geofenceService,
			Calendars:       calendarRepo,
			CalendarService: calendarService,
			Notifications:   notificationRepo,
			AuditLogger:     auditLogger,
			ForwardGeocoder: forwardGeocoder,
		})
		chatSvc := aiChat.NewService(aiChat.Config{
			BaseURL:      cfg.AI.BaseURL,
			APIKey:       cfg.AI.APIKey,
			Model:        cfg.AI.Model,
			MaxTokens:    cfg.AI.MaxTokens,
			Temperature:  cfg.AI.Temperature,
			SystemPrompt: cfg.AI.SystemPrompt,
			MaxLoops:     cfg.AI.MaxToolLoops,
			Timeout:      cfg.AI.Timeout,
			MCPServer:    mcpSrv,
		})
		chatHandler = handlers.NewChatHandler(chatSvc)
		slog.Info("AI chat enabled", slog.String("model", cfg.AI.Model), slog.String("baseURL", cfg.AI.BaseURL))
	}
	handler.SetAIEnabled(cfg.AI.Enabled)

	routerCfg := api.RouterConfig{
		LoginRateLimit:  loginRateLimit,
		APIRateLimit:    middleware.APIRateLimit(),
		SecurityHeaders: middleware.SecurityHeaders,
		Auth:            middleware.LoadAuthContext(userRepo, sessionRepo, apiKeyRepo),
		WriteAccess:     middleware.RequireWriteAccess,
		Logger:          middleware.Logger,
		Chat:            chatHandler,
		CSRFProtect: middleware.CSRF(middleware.CSRFConfig{
			Secret: csrfSecret,
			Secure: csrfSecure,
			ValidateXAuthToken: func(ctx context.Context, token string) bool {
				s, err := sessionRepo.GetByID(ctx, token)
				return err == nil && s != nil
			},
		}),
	}
	router := api.NewRouter(handler, secHandler, hub, routerCfg)

	// Geofence event detection service.
	geofenceEventService := services.NewGeofenceEventService(geofenceRepo, eventRepo, positionRepo, hub, notificationService)
	geofenceEventService.SetCalendarRepo(calendarRepo)
	geofenceEventService.SetLogger(svcLogger)

	// Motion detection service.
	motionService := services.NewMotionService(positionRepo, eventRepo, hub, notificationService)
	motionService.SetLogger(svcLogger)

	// Ignition detection service.
	ignitionService := services.NewIgnitionService(deviceRepo, eventRepo, hub, notificationService)
	ignitionService.SetLogger(svcLogger)

	// Alarm detection service (SOS, power cut, vibration, overspeed from H02 flags).
	alarmService := services.NewAlarmService(eventRepo, hub, notificationService)
	alarmService.SetLogger(svcLogger)

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
	// Start Redis subscriber to invalidate local device-access cache entries on
	// assignment changes made by other pods.
	go hub.StartInvalidationSubscriber(gpsCtx)

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

// loadCSRFSecret returns the 32-byte CSRF secret. In non-development
// environments config.Validate() already guarantees a non-empty, valid secret,
// so reaching the empty branch in production is a programming error.
func loadCSRFSecret(hexSecret, env string) []byte {
	if hexSecret != "" {
		secret, err := config.ParseCSRFSecret(hexSecret)
		if err != nil {
			slog.Error("invalid CSRF secret", slog.Any("error", err))
			os.Exit(1)
		}
		return secret
	}

	if env != "development" {
		// Should be unreachable: config.Validate() rejects this combination.
		panic("MOTUS_CSRF_SECRET must be set in non-development environments")
	}

	// Development only: generate a random 32-byte key per restart.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		slog.Error("failed to generate CSRF secret", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Warn("no MOTUS_CSRF_SECRET set, using random key (tokens will not survive restarts)")
	return secret
}
