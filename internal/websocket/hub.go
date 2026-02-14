package websocket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/pubsub"
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
)

// DeviceAccessChecker resolves which users have access to a given device.
type DeviceAccessChecker interface {
	GetUserIDs(ctx context.Context, deviceID int64) ([]int64, error)
}

// UserIDExtractor extracts a user ID from an HTTP request. Returns 0
// if the user is not authenticated.
type UserIDExtractor func(r *http.Request) int64

// ShareTokenValidator validates a share token and returns the associated device ID.
// Returns deviceID > 0 if valid, 0 if invalid/expired.
type ShareTokenValidator interface {
	ValidateShareToken(ctx context.Context, token string) (deviceID int64, err error)
}

// Client represents a connected WebSocket user.
type Client struct {
	UserID         int64
	SharedDeviceID int64 // non-zero for share-token connections (scoped to one device)
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
// subscriber can skip self-echoed messages.
type redisEnvelope struct {
	OriginPodID string         `json:"originPodId,omitempty"`
	DeviceID    int64          `json:"deviceId"`
	Message     TraccarMessage `json:"message"`
}

// Hub manages WebSocket client connections and broadcasts.
type Hub struct {
	mu             sync.RWMutex
	clients        map[*Client]bool
	allowedOrigins []string
	isDevelopment  bool
	upgrader       websocket.Upgrader
	accessChecker  DeviceAccessChecker
	extractUserID  UserIDExtractor
	shareValidator ShareTokenValidator
	pubsub         pubsub.PubSub
	podID          string // unique identifier for this pod instance
	accessCache    *deviceAccessCache
	logger         *slog.Logger
}

// NewHub creates a new WebSocket hub with origin validation and per-user filtering.
// If allowedOrigins is empty, only localhost origins are permitted (dev mode).
// accessChecker determines which users can see which device data.
// extractUserID extracts the authenticated user ID from an HTTP request.
func NewHub(allowedOrigins []string, accessChecker DeviceAccessChecker, extractUserID UserIDExtractor) *Hub {
	h := &Hub{
		clients:        make(map[*Client]bool),
		allowedOrigins: allowedOrigins,
		accessChecker:  accessChecker,
		extractUserID:  extractUserID,
		podID:          generatePodID(),
		accessCache:    newDeviceAccessCache(0), // uses defaultCacheTTL (30s)
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

// generatePodID creates a random 8-byte hex string to uniquely identify this
// pod instance. Used to prevent Redis pub/sub self-echo.
func generatePodID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback: this should never happen but avoids a panic on startup.
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// SetLogger configures the structured logger for this hub.
func (h *Hub) SetLogger(l *slog.Logger) {
	if l != nil {
		h.logger = l
	}
}

// log returns the hub's logger, falling back to slog.Default() if nil.
func (h *Hub) log() *slog.Logger {
	if h.logger != nil {
		return h.logger
	}
	return slog.Default()
}

// SetPubSub configures cross-pod broadcasting via Redis pub/sub. When set,
// broadcast calls publish messages to Redis so that all pods relay them to
// their local WebSocket clients. If pubsub is nil, broadcasting is local-only.
func (h *Hub) SetPubSub(ps pubsub.PubSub) {
	h.pubsub = ps
}

// SetShareTokenValidator configures share token validation for the hub.
// When set, unauthenticated WebSocket connections can provide a shareToken
// query parameter to receive position updates for a specific shared device.
func (h *Hub) SetShareTokenValidator(v ShareTokenValidator) {
	h.shareValidator = v
}

// StartSubscriber begins listening for messages from Redis pub/sub and relays
// them to local WebSocket clients. It blocks until ctx is cancelled. Call this
// in a goroutine after SetPubSub. If no PubSub is configured, this is a no-op
// that blocks until the context is done.
func (h *Hub) StartSubscriber(ctx context.Context) {
	if h.pubsub == nil {
		<-ctx.Done()
		return
	}

	h.log().Info("starting Redis subscriber", slog.String("podID", h.podID))

	err := h.pubsub.Subscribe(ctx, func(data []byte) {
		var env redisEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			h.log().Error("redis unmarshal error", slog.Any("error", err))
			return
		}
		// Skip messages that this pod itself published (self-echo prevention).
		if env.OriginPodID == h.podID {
			return
		}
		h.log().Debug("redis: relaying remote message",
			slog.Int64("deviceID", env.DeviceID),
			slog.String("fromPod", env.OriginPodID),
		)
		// Relay to local clients only (do not re-publish to Redis).
		h.broadcastForDevice(env.DeviceID, env.Message)
	})
	if err != nil {
		h.log().Error("redis subscribe error", slog.Any("error", err))
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
		for _, allowed := range h.allowedOrigins {
			if origin == allowed {
				return true
			}
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

	h.log().Warn("WebSocket connection rejected: origin not allowed",
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
	h.log().Debug("HandleConnect called",
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
			h.log().Warn("invalid or expired share token")
			http.Error(w, "Invalid or expired share token", http.StatusUnauthorized)
			return
		}
		h.log().Debug("share token validated", slog.Int64("deviceID", sharedDeviceID))
	} else {
		// Standard user authentication.
		userID = h.extractUserID(r)
		h.log().Debug("extracted userID", slog.Int64("userID", userID))

		if userID == 0 {
			h.log().Debug("auth failed: returning 401")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log().Error("WebSocket upgrade error", slog.Any("error", err))
		return
	}

	client := &Client{
		UserID:         userID,
		SharedDeviceID: sharedDeviceID,
		Conn:           conn,
	}

	h.mu.Lock()
	h.clients[client] = true
	clientCount := len(h.clients)
	h.mu.Unlock()
	metrics.WebSocketConnections.Inc()
	metrics.WebSocketConnectionsByPod.WithLabelValues(h.podID).Inc()

	if sharedDeviceID > 0 {
		h.log().Info("share client connected",
			slog.Int64("deviceID", sharedDeviceID),
			slog.Int("totalClients", clientCount),
			slog.String("podID", h.podID),
		)
	} else {
		h.log().Info("client connected",
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
					h.log().Warn("ping failed",
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
			metrics.WebSocketConnectionsByPod.WithLabelValues(h.podID).Dec()
			if sharedDeviceID > 0 {
				h.log().Info("share client disconnected",
					slog.Int64("deviceID", sharedDeviceID),
					slog.Int("remaining", remaining),
				)
			} else {
				h.log().Info("client disconnected",
					slog.Int64("userID", userID),
					slog.Int("remaining", remaining),
				)
			}
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					h.log().Warn("read error",
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
	h.log().Debug("BroadcastPosition",
		slog.Int64("deviceID", position.DeviceID),
		slog.Int64("positionID", position.ID),
		slog.Float64("lat", position.Latitude),
		slog.Float64("lon", position.Longitude),
	)
	// Convert speed from internal km/h to knots for Traccar API compatibility.
	pos := *position
	if position.Speed != nil {
		knots := *position.Speed / 1.852
		pos.Speed = &knots
	}
	msg := TraccarMessage{
		Positions: []model.Position{pos},
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
			h.log().Error("redis publish error",
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
	deviceID, err := h.shareValidator.ValidateShareToken(ctx, token)
	if err != nil {
		h.log().Warn("share token validation error", slog.Any("error", err))
		return 0
	}
	return deviceID
}

// clientCanReceive checks whether a client should receive a broadcast for
// the given device. Authenticated clients are checked against allowedUserIDs.
// Share-token clients only receive updates for their specific SharedDeviceID.
func clientCanReceive(client *Client, deviceID int64, allowedUserIDs []int64) bool {
	if client.SharedDeviceID > 0 {
		// Share-token client: only receives updates for its scoped device.
		return client.SharedDeviceID == deviceID
	}
	// Regular authenticated client: checked against the access list.
	return userIDInSlice(client.UserID, allowedUserIDs)
}

// broadcastForDevice sends a message only to clients whose user has access to
// the specified device, or share-token clients scoped to that device.
func (h *Hub) broadcastForDevice(deviceID int64, msg TraccarMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.log().Error("marshal error", slog.Any("error", err))
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
			h.log().Warn("write error",
				slog.Int64("userID", client.UserID),
				slog.Any("error", writeErr),
			)
			_ = client.Conn.Close()
			stale = append(stale, client)
		}
	}
	h.mu.RUnlock()

	h.log().Debug("broadcast complete",
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
	if ids, ok := h.accessCache.get(deviceID); ok {
		return ids
	}

	// Cache miss: query the database.
	userIDs, err := h.accessChecker.GetUserIDs(context.Background(), deviceID)
	if err != nil {
		h.log().Error("failed to get user IDs for device",
			slog.Int64("deviceID", deviceID),
			slog.Any("error", err),
		)
		return nil
	}

	// Store in cache for subsequent broadcasts.
	h.accessCache.set(deviceID, userIDs)
	return userIDs
}

// InvalidateDevice removes the cached user-device access entry for a device.
// Call this when user-device assignments change (assign or unassign) to ensure
// the next broadcast queries the database for fresh data. This only affects the
// local pod's cache; in a multi-pod deployment each pod's cache expires
// independently via TTL.
func (h *Hub) InvalidateDevice(deviceID int64) {
	h.accessCache.invalidate(deviceID)
}

// InvalidateAllDevices removes all cached user-device access entries.
// Useful for bulk operations like user deletion.
func (h *Hub) InvalidateAllDevices() {
	h.accessCache.invalidateAll()
}

func userIDInSlice(id int64, ids []int64) bool {
	for _, uid := range ids {
		if uid == id {
			return true
		}
	}
	return false
}
