package websocket

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/pubsub"
	"github.com/tamcore/motus/internal/ttlcache"
)

const (
	// pongWait is the maximum time to wait for a pong response from the client.
	// This should be longer than the client's ping interval (30s in the frontend).
	pongWait = 60 * time.Second

	// pingInterval is how often the server sends pings to the client.
	// Must be less than pongWait.
	pingInterval = 30 * time.Second

	// writeWait is the time allowed to write a message to the client.
	writeWait = 10 * time.Second

	// defaultCacheTTL is how long cached user-device access entries remain valid.
	defaultCacheTTL = 30 * time.Second
)

// DeviceAccessChecker resolves which users have access to a given device.
type DeviceAccessChecker interface {
	GetUserIDs(ctx context.Context, deviceID int64) ([]int64, error)
}

// Client represents a connected WebSocket user.
type Client struct {
	UserID         int64
	SharedDeviceID int64 // non-zero for share-token connections (scoped to one device)
	IsAdmin        bool  // bypasses per-device access filtering; set once at connection time
	Conn           *websocket.Conn
	mu             sync.Mutex // protects Conn.WriteMessage from concurrent calls
}

// TraccarMessage is the Traccar-compatible WebSocket message format.
type TraccarMessage struct {
	Devices   []model.Device   `json:"devices,omitempty"`
	Positions []model.Position `json:"positions,omitempty"`
	Events    []model.Event    `json:"events,omitempty"`
}

// redisEnvelope wraps a TraccarMessage with the originating device ID so that
// receiving pods can perform per-user access filtering. The OriginPodID
// identifies which pod published the message so that the same pod's
// subscriber can skip self-echoed messages. Cache-invalidation events carry
// no Message.
type redisEnvelope struct {
	OriginPodID string         `json:"originPodId,omitempty"`
	DeviceID    int64          `json:"deviceId"`
	Message     TraccarMessage `json:"message,omitzero"`
}

// Hub manages WebSocket client connections and broadcasts.
type Hub struct {
	mu                 sync.RWMutex
	clients            map[*Client]bool
	allowedOrigins     []string
	isDevelopment      bool
	upgrader           websocket.Upgrader
	accessChecker      DeviceAccessChecker
	adminChecker       func(ctx context.Context, userID int64) bool
	extractUserID      func(r *http.Request) int64
	shareValidator     func(ctx context.Context, token string) (deviceID int64, err error)
	pubsub             pubsub.PubSub
	invalidationPubSub pubsub.PubSub
	podID              string // unique identifier for this pod instance
	accessCache        *ttlcache.Cache[int64, []int64]
	logger             *slog.Logger
}

// NewHub creates a new WebSocket hub with origin validation and per-user filtering.
// If allowedOrigins is empty, only localhost origins are permitted (dev mode).
// accessChecker determines which users can see which device data.
// extractUserID returns the authenticated user ID from a request, or 0.
func NewHub(allowedOrigins []string, accessChecker DeviceAccessChecker, extractUserID func(r *http.Request) int64) *Hub {
	h := &Hub{
		clients:        make(map[*Client]bool),
		allowedOrigins: allowedOrigins,
		accessChecker:  accessChecker,
		extractUserID:  extractUserID,
		podID:          rand.Text(),
		accessCache:    ttlcache.New[int64, []int64](defaultCacheTTL),
		logger:         slog.Default(),
	}
	h.upgrader = websocket.Upgrader{
		CheckOrigin: h.checkOrigin,
	}
	return h
}

// SetDevelopmentMode marks the hub as running in development mode, which
// relaxes the WebSocket origin check to also allow localhost origins.
func (h *Hub) SetDevelopmentMode(dev bool) {
	h.isDevelopment = dev
}

// SetLogger configures the structured logger for this hub.
func (h *Hub) SetLogger(l *slog.Logger) {
	if l == nil {
		return
	}
	h.logger = l
}

// SetPubSub configures cross-pod broadcasting via Redis pub/sub. When set,
// broadcast calls publish messages to Redis so that all pods relay them to
// their local WebSocket clients. If pubsub is nil, broadcasting is local-only.
func (h *Hub) SetPubSub(ps pubsub.PubSub) {
	h.pubsub = ps
}

// SetInvalidationPubSub configures cross-pod cache invalidation via a dedicated
// Redis pub/sub channel. When set, InvalidateDevice publishes an invalidation
// event so that all other pods evict the same cache entry immediately rather
// than waiting for TTL expiry.
func (h *Hub) SetInvalidationPubSub(ps pubsub.PubSub) {
	h.invalidationPubSub = ps
}

// StartInvalidationSubscriber listens for cache-invalidation events from other
// pods and evicts the named device from the local access cache on receipt.
// It blocks until ctx is cancelled. Call this in a goroutine after
// SetInvalidationPubSub. If no invalidation PubSub is configured, this is a
// no-op that blocks until the context is done.
func (h *Hub) StartInvalidationSubscriber(ctx context.Context) {
	h.subscribe(ctx, h.invalidationPubSub, "cache-invalidation", func(env redisEnvelope) {
		h.logger.Debug("cache invalidation from remote pod",
			slog.Int64("deviceID", env.DeviceID),
			slog.String("fromPod", env.OriginPodID),
		)
		h.accessCache.Delete(env.DeviceID)
	})
}

// SetShareTokenValidator configures share token validation for the hub.
// When set, unauthenticated WebSocket connections can provide a shareToken
// query parameter to receive position updates for a specific shared device.
// v returns the shared device ID, or 0 if the token is invalid or expired.
func (h *Hub) SetShareTokenValidator(v func(ctx context.Context, token string) (deviceID int64, err error)) {
	h.shareValidator = v
}

// SetAdminChecker configures admin detection for the hub. When set, it is called
// once per authenticated WebSocket connection. Admin clients bypass per-device
// access filtering and receive broadcasts for all devices. fn returns false on error.
func (h *Hub) SetAdminChecker(fn func(ctx context.Context, userID int64) bool) {
	h.adminChecker = fn
}

// StartSubscriber begins listening for messages from Redis pub/sub and relays
// them to local WebSocket clients. It blocks until ctx is cancelled. Call this
// in a goroutine after SetPubSub. If no PubSub is configured, this is a no-op
// that blocks until the context is done.
func (h *Hub) StartSubscriber(ctx context.Context) {
	h.subscribe(ctx, h.pubsub, "redis", func(env redisEnvelope) {
		h.logger.Debug("redis: relaying remote message",
			slog.Int64("deviceID", env.DeviceID),
			slog.String("fromPod", env.OriginPodID),
		)
		// Relay to local clients only (do not re-publish to Redis).
		h.broadcastForDevice(env.DeviceID, env.Message)
	})
}

// subscribe passes envelopes published by other pods on ps to fn until ctx
// is cancelled. A nil ps only blocks.
func (h *Hub) subscribe(ctx context.Context, ps pubsub.PubSub, name string, fn func(redisEnvelope)) {
	if ps == nil {
		<-ctx.Done()
		return
	}

	h.logger.Info("starting "+name+" subscriber", slog.String("podID", h.podID))

	err := ps.Subscribe(ctx, func(data []byte) {
		var env redisEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			h.logger.Error(name+" unmarshal error", slog.Any("error", err))
			return
		}
		if env.OriginPodID == h.podID {
			return
		}
		fn(env)
	})
	if err != nil {
		h.logger.Error(name+" subscribe error", slog.Any("error", err))
	}

	<-ctx.Done()
}

// checkOrigin validates the Origin header against the allowed list.
func (h *Hub) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")

	// If no Origin header, allow the connection (same-origin or non-browser).
	if origin == "" {
		return true
	}

	// If allowed origins are configured, check against the list.
	if len(h.allowedOrigins) > 0 {
		if slices.Contains(h.allowedOrigins, origin) {
			return true
		}
	}

	// Allow localhost origins only in development mode. Use proper URL
	// parsing to prevent subdomain bypass (e.g. localhost.evil.com).
	if h.isDevelopment {
		if parsed, err := url.Parse(origin); err == nil {
			host := parsed.Hostname()
			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				return true
			}
		}
	}

	h.logger.Warn("WebSocket connection rejected: origin not allowed",
		slog.String("origin", origin),
	)
	return false
}

// HandleConnect upgrades an HTTP connection to WebSocket.
// Supports two authentication modes:
//  1. Session-based: user must be authenticated (cookie or Bearer token).
//  2. Share token: unauthenticated clients provide ?shareToken=xxx to receive
//     updates for a single shared device.
func (h *Hub) HandleConnect(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("HandleConnect called",
		slog.String("method", r.Method),
		slog.String("upgrade", r.Header.Get("Upgrade")),
		slog.String("connection", r.Header.Get("Connection")),
	)

	var userID int64
	var sharedDeviceID int64

	// Check for share token in query parameters first.
	shareToken := r.URL.Query().Get("shareToken")
	if shareToken != "" {
		sharedDeviceID = h.validateShareToken(r.Context(), shareToken)
		if sharedDeviceID == 0 {
			h.logger.Warn("invalid or expired share token")
			http.Error(w, "Invalid or expired share token", http.StatusUnauthorized)
			return
		}
		h.logger.Debug("share token validated", slog.Int64("deviceID", sharedDeviceID))
	} else {
		// Standard user authentication.
		userID = h.extractUserID(r)
		h.logger.Debug("extracted userID", slog.Int64("userID", userID))

		if userID == 0 {
			h.logger.Debug("auth failed: returning 401")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// nosemgrep: go.gorilla.security.audit.websocket-missing-origin-check.websocket-missing-origin-check -- CheckOrigin is configured on the Upgrader (see newUpgrader)
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("WebSocket upgrade error", slog.Any("error", err))
		return
	}

	var isAdmin bool
	if userID != 0 && h.adminChecker != nil {
		isAdmin = h.adminChecker(r.Context(), userID)
	}

	client := &Client{
		UserID:         userID,
		SharedDeviceID: sharedDeviceID,
		IsAdmin:        isAdmin,
		Conn:           conn,
	}

	h.mu.Lock()
	h.clients[client] = true
	clientCount := len(h.clients)
	h.mu.Unlock()
	metrics.WebSocketConnections.Inc()

	if sharedDeviceID > 0 {
		h.logger.Info("share client connected",
			slog.Int64("deviceID", sharedDeviceID),
			slog.Int("totalClients", clientCount),
			slog.String("podID", h.podID),
		)
	} else {
		h.logger.Info("client connected",
			slog.Int64("userID", userID),
			slog.Int("totalClients", clientCount),
			slog.String("podID", h.podID),
		)
	}

	// Set initial read deadline; the pong handler resets it on each pong.
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Server-side ping ticker keeps the connection alive and detects
	// dead clients. Runs in a separate goroutine that exits when the
	// read loop closes the done channel.
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				client.mu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				client.mu.Unlock()
				if err != nil {
					h.logger.Warn("ping failed",
						slog.Int64("userID", userID),
						slog.Any("error", err),
					)
					return
				}
			case <-done:
				return
			}
		}
	}()

	// Read loop to detect disconnections. When the client sends text
	// messages (e.g. pings from the JS side), we simply discard them.
	go func() {
		defer func() {
			close(done) // stop the ping ticker
			h.mu.Lock()
			delete(h.clients, client)
			remaining := len(h.clients)
			h.mu.Unlock()
			_ = conn.Close()
			metrics.WebSocketConnections.Dec()
			if sharedDeviceID > 0 {
				h.logger.Info("share client disconnected",
					slog.Int64("deviceID", sharedDeviceID),
					slog.Int("remaining", remaining),
				)
			} else {
				h.logger.Info("client disconnected",
					slog.Int64("userID", userID),
					slog.Int("remaining", remaining),
				)
			}
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					h.logger.Warn("read error",
						slog.Int64("userID", userID),
						slog.Any("error", err),
					)
				}
				break
			}
		}
	}()
}

// BroadcastPosition sends a position update to users who have access to the device.
func (h *Hub) BroadcastPosition(position *model.Position) {
	h.logger.Debug("BroadcastPosition",
		slog.Int64("deviceID", position.DeviceID),
		slog.Int64("positionID", position.ID),
		slog.Float64("lat", position.Latitude),
		slog.Float64("lon", position.Longitude),
	)
	msg := TraccarMessage{
		Positions: []model.Position{*position.InKnots()},
	}
	metrics.WebSocketMessagesSent.WithLabelValues("position").Inc()
	h.publishAndBroadcast(position.DeviceID, msg)
}

// BroadcastDeviceStatus sends a device status update to users who have access.
func (h *Hub) BroadcastDeviceStatus(device *model.Device) {
	msg := TraccarMessage{
		Devices: []model.Device{*device},
	}
	metrics.WebSocketMessagesSent.WithLabelValues("device").Inc()
	h.publishAndBroadcast(device.ID, msg)
}

// BroadcastEvent sends an event to users who have access to the device.
func (h *Hub) BroadcastEvent(event *model.Event) {
	msg := TraccarMessage{
		Events: []model.Event{*event},
	}
	metrics.WebSocketMessagesSent.WithLabelValues("event").Inc()
	h.publishAndBroadcast(event.DeviceID, msg)
}

// publishAndBroadcast publishes the message to Redis (if configured) for
// cross-pod delivery, and also broadcasts to local WebSocket clients.
// When Redis is active, remote pods receive the message via their subscriber
// goroutine and broadcast to their own local clients. The envelope includes
// this pod's ID so that the local subscriber can skip self-echoed messages.
func (h *Hub) publishAndBroadcast(deviceID int64, msg TraccarMessage) {
	// Publish to Redis for other pods. Errors are logged but do not prevent
	// local delivery. The OriginPodID allows the local subscriber to skip
	// messages that this pod itself published.
	if h.pubsub != nil {
		env := redisEnvelope{
			OriginPodID: h.podID,
			DeviceID:    deviceID,
			Message:     msg,
		}
		if err := h.pubsub.Publish(context.Background(), env); err != nil {
			h.logger.Error("redis publish error",
				slog.Int64("deviceID", deviceID),
				slog.Any("error", err),
			)
		}
	}

	// Always broadcast to local clients on this pod.
	h.broadcastForDevice(deviceID, msg)
}

// validateShareToken checks a share token via the configured validator.
// Returns the device ID if the token is valid, 0 otherwise.
func (h *Hub) validateShareToken(ctx context.Context, token string) int64 {
	if h.shareValidator == nil {
		return 0
	}
	deviceID, err := h.shareValidator(ctx, token)
	if err != nil {
		h.logger.Warn("share token validation error", slog.Any("error", err))
		return 0
	}
	return deviceID
}

// clientCanReceive checks whether a client should receive a broadcast for
// the given device. Authenticated clients are checked against allowedUserIDs.
// Share-token clients only receive updates for their specific SharedDeviceID.
// Admin clients receive broadcasts for all devices.
func clientCanReceive(client *Client, deviceID int64, allowedUserIDs []int64) bool {
	if client.SharedDeviceID > 0 {
		// Share-token client: only receives updates for its scoped device.
		return client.SharedDeviceID == deviceID
	}
	if client.IsAdmin {
		return true
	}
	// Regular authenticated client: checked against the access list.
	return slices.Contains(allowedUserIDs, client.UserID)
}

// broadcastForDevice sends a message only to clients whose user has access to
// the specified device, or share-token clients scoped to that device.
func (h *Hub) broadcastForDevice(deviceID int64, msg TraccarMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("marshal error", slog.Any("error", err))
		return
	}

	// Determine which users have access to this device.
	allowedUserIDs := h.getAllowedUserIDs(deviceID)

	// Collect stale clients to remove after releasing the read lock.
	var stale []*Client

	h.mu.RLock()
	clientCount := len(h.clients)
	for client := range h.clients {
		if !clientCanReceive(client, deviceID, allowedUserIDs) {
			continue
		}
		// Serialize writes per-connection: gorilla/websocket does not
		// support concurrent WriteMessage calls on the same Conn.
		client.mu.Lock()
		_ = client.Conn.SetWriteDeadline(time.Now().Add(writeWait))
		writeErr := client.Conn.WriteMessage(websocket.TextMessage, data)
		client.mu.Unlock()

		if writeErr != nil {
			h.logger.Warn("write error",
				slog.Int64("userID", client.UserID),
				slog.Any("error", writeErr),
			)
			_ = client.Conn.Close()
			stale = append(stale, client)
		}
	}
	h.mu.RUnlock()

	h.logger.Debug("broadcast complete",
		slog.Int64("deviceID", deviceID),
		slog.Int("totalClients", clientCount),
		slog.Int("allowedUsers", len(allowedUserIDs)),
		slog.Int("stale", len(stale)),
	)

	// Remove stale clients under a write lock (not RLock).
	if len(stale) > 0 {
		h.mu.Lock()
		for _, client := range stale {
			delete(h.clients, client)
		}
		h.mu.Unlock()
	}
}

// getAllowedUserIDs returns the user IDs that are allowed to see data
// for the given device. Results are cached per-pod with a TTL to reduce
// database load during high-frequency broadcasts. If the access checker
// is nil or fails, an empty slice is returned (fail closed).
func (h *Hub) getAllowedUserIDs(deviceID int64) []int64 {
	if h.accessChecker == nil {
		return nil
	}

	// Check cache first.
	if ids, ok := h.accessCache.Get(deviceID); ok {
		return ids
	}

	// Cache miss: query the database.
	userIDs, err := h.accessChecker.GetUserIDs(context.Background(), deviceID)
	if err != nil {
		h.logger.Error("failed to get user IDs for device",
			slog.Int64("deviceID", deviceID),
			slog.Any("error", err),
		)
		return nil
	}

	// Store in cache for subsequent broadcasts.
	h.accessCache.Set(deviceID, userIDs)
	return userIDs
}

// InvalidateDevice removes the cached user-device access entry for a device
// and, when a cross-pod invalidation pub/sub is configured, publishes an event
// so that all other pods evict the same entry immediately. The local eviction
// always happens regardless of whether the publish succeeds.
func (h *Hub) InvalidateDevice(deviceID int64) {
	h.accessCache.Delete(deviceID)
	if h.invalidationPubSub != nil {
		env := redisEnvelope{OriginPodID: h.podID, DeviceID: deviceID}
		if err := h.invalidationPubSub.Publish(context.Background(), env); err != nil {
			h.logger.Error("cache invalidation publish error",
				slog.Int64("deviceID", deviceID),
				slog.Any("error", err),
			)
		}
	}
}
