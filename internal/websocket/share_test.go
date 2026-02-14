package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/tamcore/motus/internal/model"
)

// mockShareValidator implements ShareTokenValidator for testing.
// It returns a device ID for known tokens, 0 for unknown ones.
type mockShareValidator struct {
	tokens map[string]int64 // token -> deviceID
}

func (m *mockShareValidator) ValidateShareToken(ctx context.Context, token string) (deviceID int64, err error) {
	if id, ok := m.tokens[token]; ok {
		return id, nil
	}
	return 0, nil
}

// connectShareClient connects a WebSocket client using a share token query parameter.
func connectShareClient(t *testing.T, hub *Hub, shareToken string) (*ws.Conn, *http.Response) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(hub.HandleConnect))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "?shareToken=" + shareToken
	conn, resp, err := ws.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, resp
	}
	t.Cleanup(func() { _ = conn.Close() })

	time.Sleep(50 * time.Millisecond)
	return conn, resp
}

func TestHandleConnect_ShareToken_Valid(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	validator := &mockShareValidator{
		tokens: map[string]int64{"valid-token": 10},
	}

	hub := NewHub(nil, checker, dummyExtractor)
	hub.SetShareTokenValidator(validator)

	conn, _ := connectShareClient(t, hub, "valid-token")
	if conn == nil {
		t.Fatal("expected successful connection with valid share token")
	}

	// Verify the client was registered with the correct SharedDeviceID.
	hub.mu.RLock()
	var found bool
	for client := range hub.clients {
		if client.SharedDeviceID == 10 {
			found = true
			break
		}
	}
	hub.mu.RUnlock()

	if !found {
		t.Error("expected a client with SharedDeviceID=10 in hub")
	}
}

func TestHandleConnect_ShareToken_Invalid(t *testing.T) {
	validator := &mockShareValidator{
		tokens: map[string]int64{}, // no valid tokens
	}

	hub := NewHub(nil, nil, dummyExtractor)
	hub.SetShareTokenValidator(validator)

	conn, _ := connectShareClient(t, hub, "invalid-token")
	if conn != nil {
		t.Error("expected connection to fail with invalid share token")
		_ = conn.Close()
	}
}

func TestHandleConnect_ShareToken_NoValidator(t *testing.T) {
	// When no share validator is configured, share tokens should be rejected.
	hub := NewHub(nil, nil, dummyExtractor)

	conn, _ := connectShareClient(t, hub, "some-token")
	if conn != nil {
		t.Error("expected connection to fail when no share validator is configured")
		_ = conn.Close()
	}
}

func TestBroadcastPosition_ShareTokenClient_ReceivesOwnDevice(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	validator := &mockShareValidator{
		tokens: map[string]int64{"token-for-10": 10},
	}

	hub := NewHub(nil, checker, dummyExtractor)
	hub.SetShareTokenValidator(validator)

	conn, _ := connectShareClient(t, hub, "token-for-10")
	if conn == nil {
		t.Fatal("expected successful connection")
	}

	// Broadcast position for device 10. The share client should receive it.
	pos := &model.Position{ID: 1, DeviceID: 10, Latitude: 52.0, Longitude: 13.0, Timestamp: time.Now().UTC()}
	hub.BroadcastPosition(pos)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("share client should have received position: %v", err)
	}

	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Positions) != 1 {
		t.Errorf("expected 1 position, got %d", len(traccarMsg.Positions))
	}
	if traccarMsg.Positions[0].DeviceID != 10 {
		t.Errorf("expected device ID 10, got %d", traccarMsg.Positions[0].DeviceID)
	}
}

func TestBroadcastPosition_ShareTokenClient_DoesNotReceiveOtherDevices(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{
			10: {1},
			20: {1},
		},
	}
	validator := &mockShareValidator{
		tokens: map[string]int64{"token-for-10": 10},
	}

	hub := NewHub(nil, checker, dummyExtractor)
	hub.SetShareTokenValidator(validator)

	conn, _ := connectShareClient(t, hub, "token-for-10")
	if conn == nil {
		t.Fatal("expected successful connection")
	}

	// Broadcast position for device 20. The share client (for device 10) should NOT receive it.
	pos := &model.Position{ID: 2, DeviceID: 20, Latitude: 48.0, Longitude: 11.0, Timestamp: time.Now().UTC()}
	hub.BroadcastPosition(pos)

	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Error("share client for device 10 should not receive updates for device 20")
	}
}

func TestBroadcastDeviceStatus_ShareTokenClient(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{42: {1}},
	}
	validator := &mockShareValidator{
		tokens: map[string]int64{"token-42": 42},
	}

	hub := NewHub(nil, checker, dummyExtractor)
	hub.SetShareTokenValidator(validator)

	conn, _ := connectShareClient(t, hub, "token-42")
	if conn == nil {
		t.Fatal("expected successful connection")
	}

	device := &model.Device{ID: 42, UniqueID: "test", Name: "Test", Status: "online"}
	hub.BroadcastDeviceStatus(device)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("share client should have received device status: %v", err)
	}

	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(traccarMsg.Devices))
	}
}

func TestBroadcastEvent_ShareTokenClient(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{42: {1}},
	}
	validator := &mockShareValidator{
		tokens: map[string]int64{"token-42": 42},
	}

	hub := NewHub(nil, checker, dummyExtractor)
	hub.SetShareTokenValidator(validator)

	conn, _ := connectShareClient(t, hub, "token-42")
	if conn == nil {
		t.Fatal("expected successful connection")
	}

	event := &model.Event{ID: 1, DeviceID: 42, Type: "geofenceEnter", Timestamp: time.Now().UTC()}
	hub.BroadcastEvent(event)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("share client should have received event: %v", err)
	}

	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(traccarMsg.Events))
	}
}

func TestBroadcast_MixedClients(t *testing.T) {
	// Test that authenticated users and share token clients coexist correctly.
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	validator := &mockShareValidator{
		tokens: map[string]int64{"share-10": 10},
	}

	// First connection will be an authenticated user (userID=1).
	callCount := 0
	hub := NewHub(nil, checker, func(_ *http.Request) int64 {
		callCount++
		if callCount == 1 {
			return 1 // First call: authenticated user
		}
		return 0 // Subsequent calls: unauthenticated (share token path)
	})
	hub.SetShareTokenValidator(validator)

	// Connect authenticated user.
	authConn := connectClient(t, hub)

	// Connect share token client.
	shareConn, _ := connectShareClient(t, hub, "share-10")
	if shareConn == nil {
		t.Fatal("expected successful share connection")
	}

	// Broadcast position for device 10.
	pos := &model.Position{ID: 1, DeviceID: 10, Latitude: 52.0, Longitude: 13.0, Timestamp: time.Now().UTC()}
	hub.BroadcastPosition(pos)

	// Both should receive the message.
	_ = authConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := authConn.ReadMessage()
	if err != nil {
		t.Fatalf("authenticated client should have received position: %v", err)
	}

	_ = shareConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = shareConn.ReadMessage()
	if err != nil {
		t.Fatalf("share client should have received position: %v", err)
	}
}

func TestBroadcast_ShareClient_ViaRedis(t *testing.T) {
	// Share token clients should also receive messages relayed from Redis.
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	validator := &mockShareValidator{
		tokens: map[string]int64{"share-10": 10},
	}
	mock := &mockPubSub{}

	hub := NewHub(nil, checker, dummyExtractor)
	hub.SetShareTokenValidator(validator)
	hub.SetPubSub(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.StartSubscriber(ctx)
	time.Sleep(50 * time.Millisecond)

	conn, _ := connectShareClient(t, hub, "share-10")
	if conn == nil {
		t.Fatal("expected successful share connection")
	}

	// Simulate a remote message for device 10.
	remoteMsg := TraccarMessage{
		Positions: []model.Position{{ID: 99, DeviceID: 10, Latitude: 48.0, Longitude: 11.0, Timestamp: time.Now().UTC()}},
	}
	mock.simulateRemoteMessage(10, remoteMsg)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("share client should have received relayed message: %v", err)
	}

	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Positions) != 1 || traccarMsg.Positions[0].ID != 99 {
		t.Errorf("expected position ID 99, got %v", traccarMsg.Positions)
	}
}
