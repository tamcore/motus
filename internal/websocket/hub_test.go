package websocket

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/tamcore/motus/internal/model"
)

// mockAccessChecker implements DeviceAccessChecker for testing.
type mockAccessChecker struct {
	deviceUsers map[int64][]int64
	callCount   int // tracks how many times GetUserIDs was called
	err         error
}

func (m *mockAccessChecker) GetUserIDs(_ context.Context, deviceID int64) ([]int64, error) {
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	return m.deviceUsers[deviceID], nil
}

// errShareValidator implements ShareTokenValidator but always returns an error.
type errShareValidator struct{}

func (e *errShareValidator) ValidateShareToken(_ context.Context, _ string) (int64, error) {
	return 0, errors.New("validator error")
}

// errPubSub implements pubsub.PubSub but Publish always returns an error.
type errPubSub struct {
	mu               sync.Mutex
	subscribeHandler func([]byte)
}

func (e *errPubSub) Publish(_ context.Context, _ interface{}) error {
	return errors.New("redis publish error")
}

func (e *errPubSub) Subscribe(_ context.Context, handler func([]byte)) error {
	e.mu.Lock()
	e.subscribeHandler = handler
	e.mu.Unlock()
	return nil
}

func (e *errPubSub) getHandler() func([]byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.subscribeHandler
}

func (e *errPubSub) Close() error { return nil }

// errSubscribePubSub.Subscribe always returns an error.
type errSubscribePubSub struct{}

func (e *errSubscribePubSub) Publish(_ context.Context, _ interface{}) error { return nil }
func (e *errSubscribePubSub) Subscribe(_ context.Context, _ func([]byte)) error {
	return errors.New("subscribe error")
}
func (e *errSubscribePubSub) Close() error { return nil }

// dummyExtractor is a UserIDExtractor that always returns 0.
func dummyExtractor(_ *http.Request) int64 { return 0 }

func TestCheckOrigin(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		origin         string
		devMode        bool
		want           bool
	}{
		// No origin header (same-origin or non-browser clients).
		{"no origin header", nil, "", false, true},

		// Localhost allowed in development mode.
		{"http localhost dev", nil, "http://localhost:3000", true, true},
		{"http 127.0.0.1 dev", nil, "http://127.0.0.1:8080", true, true},
		{"https localhost dev", nil, "https://localhost", true, true},
		{"https 127.0.0.1 dev", nil, "https://127.0.0.1:443", true, true},

		// Localhost rejected in production mode.
		{"http localhost prod", nil, "http://localhost:3000", false, false},
		{"http 127.0.0.1 prod", nil, "http://127.0.0.1:8080", false, false},

		// Configured allowed origins.
		{"allowed exact match", []string{"https://motus.example.com"}, "https://motus.example.com", false, true},
		{"allowed mismatch", []string{"https://motus.example.com"}, "https://evil.example.com", false, false},
		{"multiple allowed", []string{"https://a.example.com", "https://b.example.com"}, "https://b.example.com", false, true},

		// No allowed origins configured and not localhost.
		{"external origin no allowlist", nil, "https://evil.example.com", false, false},
		{"external origin empty allowlist", []string{}, "https://evil.example.com", false, false},

		// Localhost still works in dev mode even when allowlist is set.
		{"localhost with allowlist dev", []string{"https://motus.example.com"}, "http://localhost:3000", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := NewHub(tt.allowedOrigins, nil, dummyExtractor)
			hub.SetDevelopmentMode(tt.devMode)

			r, _ := http.NewRequest("GET", "/api/socket", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}

			got := hub.checkOrigin(r)
			if got != tt.want {
				t.Errorf("checkOrigin(origin=%q, allowed=%v) = %v, want %v",
					tt.origin, tt.allowedOrigins, got, tt.want)
			}
		})
	}
}

func TestUserIDInSlice(t *testing.T) {
	tests := []struct {
		name string
		id   int64
		ids  []int64
		want bool
	}{
		{"found", 1, []int64{1, 2, 3}, true},
		{"not found", 4, []int64{1, 2, 3}, false},
		{"empty slice", 1, nil, false},
		{"single match", 5, []int64{5}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := userIDInSlice(tt.id, tt.ids)
			if got != tt.want {
				t.Errorf("userIDInSlice(%d, %v) = %v, want %v", tt.id, tt.ids, got, tt.want)
			}
		})
	}
}

func TestClientCanReceive(t *testing.T) {
	tests := []struct {
		name           string
		client         *Client
		deviceID       int64
		allowedUserIDs []int64
		want           bool
	}{
		{
			name:           "authenticated user with access",
			client:         &Client{UserID: 1},
			deviceID:       10,
			allowedUserIDs: []int64{1, 2},
			want:           true,
		},
		{
			name:           "authenticated user without access",
			client:         &Client{UserID: 3},
			deviceID:       10,
			allowedUserIDs: []int64{1, 2},
			want:           false,
		},
		{
			name:           "share client matching device",
			client:         &Client{SharedDeviceID: 10},
			deviceID:       10,
			allowedUserIDs: nil,
			want:           true,
		},
		{
			name:           "share client non-matching device",
			client:         &Client{SharedDeviceID: 20},
			deviceID:       10,
			allowedUserIDs: nil,
			want:           false,
		},
		{
			name:           "share client ignores allowedUserIDs",
			client:         &Client{SharedDeviceID: 10},
			deviceID:       10,
			allowedUserIDs: []int64{1}, // should not matter for share clients
			want:           true,
		},
		{
			name:           "authenticated user nil allowed list",
			client:         &Client{UserID: 1},
			deviceID:       10,
			allowedUserIDs: nil,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clientCanReceive(tt.client, tt.deviceID, tt.allowedUserIDs)
			if got != tt.want {
				t.Errorf("clientCanReceive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAllowedUserIDs(t *testing.T) {
	t.Run("nil checker returns nil", func(t *testing.T) {
		hub := NewHub(nil, nil, dummyExtractor)
		ids := hub.getAllowedUserIDs(1)
		if ids != nil {
			t.Errorf("expected nil, got %v", ids)
		}
	})

	t.Run("returns user IDs from checker", func(t *testing.T) {
		checker := &mockAccessChecker{
			deviceUsers: map[int64][]int64{
				10: {1, 2},
				20: {3},
			},
		}
		hub := NewHub(nil, checker, dummyExtractor)

		ids := hub.getAllowedUserIDs(10)
		if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
			t.Errorf("expected [1 2], got %v", ids)
		}

		ids = hub.getAllowedUserIDs(20)
		if len(ids) != 1 || ids[0] != 3 {
			t.Errorf("expected [3], got %v", ids)
		}

		ids = hub.getAllowedUserIDs(99)
		if len(ids) != 0 {
			t.Errorf("expected empty, got %v", ids)
		}
	})
}

// --- Cache integration tests ---

func TestGetAllowedUserIDs_CacheHit(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{
			10: {1, 2},
		},
	}
	hub := NewHub(nil, checker, dummyExtractor)

	// First call: cache miss, should query the checker.
	ids := hub.getAllowedUserIDs(10)
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Errorf("expected [1 2], got %v", ids)
	}
	if checker.callCount != 1 {
		t.Errorf("expected 1 DB call, got %d", checker.callCount)
	}

	// Second call: cache hit, should NOT query the checker again.
	ids = hub.getAllowedUserIDs(10)
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Errorf("expected [1 2], got %v", ids)
	}
	if checker.callCount != 1 {
		t.Errorf("expected still 1 DB call after cache hit, got %d", checker.callCount)
	}

	// Third call for a different device: cache miss.
	hub.getAllowedUserIDs(20)
	if checker.callCount != 2 {
		t.Errorf("expected 2 DB calls after second miss, got %d", checker.callCount)
	}
}

func TestGetAllowedUserIDs_CacheExpiration(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{
			10: {1},
		},
	}
	hub := NewHub(nil, checker, dummyExtractor)

	// Override the cache clock for testing.
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	hub.accessCache.now = func() time.Time { return now }

	// First call: populates cache.
	hub.getAllowedUserIDs(10)
	if checker.callCount != 1 {
		t.Fatalf("expected 1 call, got %d", checker.callCount)
	}

	// Advance time past TTL.
	now = now.Add(defaultCacheTTL + 1*time.Second)

	// Cache expired: should query again.
	hub.getAllowedUserIDs(10)
	if checker.callCount != 2 {
		t.Errorf("expected 2 calls after TTL expiration, got %d", checker.callCount)
	}
}

func TestInvalidateDevice(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{
			10: {1, 2},
		},
	}
	hub := NewHub(nil, checker, dummyExtractor)

	// Populate cache.
	hub.getAllowedUserIDs(10)
	if checker.callCount != 1 {
		t.Fatalf("expected 1 call, got %d", checker.callCount)
	}

	// Invalidate the cached entry.
	hub.InvalidateDevice(10)

	// Next call should query the checker again (cache miss).
	hub.getAllowedUserIDs(10)
	if checker.callCount != 2 {
		t.Errorf("expected 2 calls after invalidation, got %d", checker.callCount)
	}
}

func TestInvalidateAllDevices(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{
			10: {1},
			20: {2},
		},
	}
	hub := NewHub(nil, checker, dummyExtractor)

	// Populate cache for two devices.
	hub.getAllowedUserIDs(10)
	hub.getAllowedUserIDs(20)
	if checker.callCount != 2 {
		t.Fatalf("expected 2 calls, got %d", checker.callCount)
	}

	// Invalidate all.
	hub.InvalidateAllDevices()

	// Both should be cache misses now.
	hub.getAllowedUserIDs(10)
	hub.getAllowedUserIDs(20)
	if checker.callCount != 4 {
		t.Errorf("expected 4 calls after invalidateAll, got %d", checker.callCount)
	}
}

func TestGetAllowedUserIDs_CacheEmptyResult(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{
			10: {}, // Device exists but no users assigned.
		},
	}
	hub := NewHub(nil, checker, dummyExtractor)

	// Empty results should also be cached (avoids repeated queries for
	// devices with no assigned users).
	ids := hub.getAllowedUserIDs(10)
	if len(ids) != 0 {
		t.Errorf("expected empty, got %v", ids)
	}
	if checker.callCount != 1 {
		t.Fatalf("expected 1 call, got %d", checker.callCount)
	}

	// Second call: should be a cache hit (still empty).
	ids = hub.getAllowedUserIDs(10)
	if len(ids) != 0 {
		t.Errorf("expected empty, got %v", ids)
	}
	if checker.callCount != 1 {
		t.Errorf("expected still 1 call for cached empty result, got %d", checker.callCount)
	}
}

func TestHub_SetLogger(t *testing.T) {
	hub := &Hub{logger: slog.Default()}
	initial := hub.logger
	hub.SetLogger(nil) // nil should not change logger
	if hub.logger != initial {
		t.Error("SetLogger(nil) should not change logger")
	}
	custom := slog.New(slog.Default().Handler())
	hub.SetLogger(custom)
	if hub.logger != custom {
		t.Error("SetLogger(custom) should replace logger")
	}
}

func TestHub_Log_NilLogger(t *testing.T) {
	// Hub with nil logger should fall back to slog.Default().
	hub := &Hub{} // logger field is nil
	l := hub.log()
	if l == nil {
		t.Fatal("log() should never return nil")
	}
}

func TestGetAllowedUserIDs_Error(t *testing.T) {
	checker := &mockAccessChecker{
		err: errors.New("db error"),
	}
	hub := NewHub(nil, checker, dummyExtractor)

	ids := hub.getAllowedUserIDs(10)
	if ids != nil {
		t.Errorf("expected nil on DB error, got %v", ids)
	}
}

func TestValidateShareToken_Error(t *testing.T) {
	hub := NewHub(nil, nil, dummyExtractor)
	hub.SetShareTokenValidator(&errShareValidator{})

	deviceID := hub.validateShareToken(context.Background(), "some-token")
	if deviceID != 0 {
		t.Errorf("expected 0 on validator error, got %d", deviceID)
	}
}

func TestBroadcastPosition_WithSpeed(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleConnect))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer func() { _ = conn.Close() }()
	time.Sleep(50 * time.Millisecond)

	speed := 100.0 // km/h
	pos := &model.Position{ID: 1, DeviceID: 10, Speed: &speed}
	hub.BroadcastPosition(pos)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected to receive message: %v", err)
	}
}

func TestPublishAndBroadcast_RedisError(t *testing.T) {
	// When Redis publish fails, local broadcast still works.
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	ps := &errPubSub{}
	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	hub.SetPubSub(ps)

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleConnect))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer func() { _ = conn.Close() }()
	time.Sleep(50 * time.Millisecond)

	pos := &model.Position{ID: 1, DeviceID: 10}
	hub.BroadcastPosition(pos)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("local broadcast should still work after Redis error: %v", err)
	}
}

func TestHandleConnect_UpgradeError(t *testing.T) {
	// A plain HTTP GET without WebSocket headers causes the upgrader to fail.
	// We use httptest.ResponseRecorder (which does not implement http.Hijacker),
	// so gorilla's upgrader will return an error and HandleConnect logs it.
	hub := NewHub(nil, nil, func(_ *http.Request) int64 { return 1 }) // auth always succeeds

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()
	hub.HandleConnect(w, req) // should log "WebSocket upgrade error" and return
}

func TestHandleConnect_PongHandler(t *testing.T) {
	// Send an unsolicited pong from the client to trigger the server's pong
	// handler (which resets the read deadline). This covers the pong handler
	// body in HandleConnect.
	hub := NewHub(nil, &mockAccessChecker{}, func(_ *http.Request) int64 { return 1 })

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleConnect))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// An unsolicited pong frame is valid per RFC 6455 §5.5.3 and is picked
	// up by gorilla's read loop, which calls the registered pong handler.
	if err := conn.WriteControl(ws.PongMessage, []byte("test"), time.Now().Add(time.Second)); err != nil {
		t.Fatalf("failed to send pong: %v", err)
	}
	time.Sleep(30 * time.Millisecond) // allow the server's read goroutine to process
}

func TestHandleConnect_Unauthorized(t *testing.T) {
	// No share token, extractor returns 0 → 401.
	hub := NewHub(nil, nil, dummyExtractor)

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleConnect))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	_, resp, err := ws.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected connection to fail with 401")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestBroadcastForDevice_WriteError(t *testing.T) {
	// Directly add a client backed by a closed WebSocket connection to the hub,
	// bypassing HandleConnect so there is no competing read goroutine that
	// could race to remove the client before the broadcast write fails.
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	hub := NewHub(nil, checker, dummyExtractor)

	// Stand up a WebSocket server just to capture the server-side *Conn.
	connCh := make(chan *ws.Conn, 1)
	wsUpgrader := ws.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connCh <- conn
		// Block until server is closed (keeps the handler goroutine alive).
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	// Dial to trigger the upgrade and get the server-side conn.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	serverConn := <-connCh

	// Close both sides: closing the underlying net.Conn on the server side
	// guarantees that WriteMessage fails immediately (not just on the second
	// write after the TCP send buffer drains).
	_ = clientConn.Close()
	_ = serverConn.NetConn().Close()

	// Add the server-side conn directly to hub.clients (no read goroutine).
	fakeClient := &Client{UserID: 1, Conn: serverConn}
	hub.mu.Lock()
	hub.clients[fakeClient] = true
	hub.mu.Unlock()

	// Broadcast: WriteMessage on the broken conn fails → stale-client cleanup.
	pos := &model.Position{ID: 1, DeviceID: 10}
	hub.BroadcastPosition(pos)
	time.Sleep(50 * time.Millisecond)

	hub.mu.RLock()
	_, stillPresent := hub.clients[fakeClient]
	hub.mu.RUnlock()
	if stillPresent {
		t.Error("stale client was not removed after write error")
	}
}

func TestStartSubscriber_UnmarshalError(t *testing.T) {
	// Sending invalid JSON to the subscriber should log an error and not panic.
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	ps := &errPubSub{}
	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	hub.SetPubSub(ps)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.StartSubscriber(ctx)
	time.Sleep(50 * time.Millisecond)

	// Inject invalid JSON via the subscribe handler.
	if h := ps.getHandler(); h != nil {
		h([]byte("not valid json"))
	}
	time.Sleep(50 * time.Millisecond) // should not panic
}

func TestStartSubscriber_SubscribeError(t *testing.T) {
	// When Subscribe returns an error, StartSubscriber should log it and
	// then block until context is cancelled.
	hub := NewHub(nil, nil, dummyExtractor)
	hub.SetPubSub(&errSubscribePubSub{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hub.StartSubscriber(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartSubscriber did not return after context cancellation")
	}
}
