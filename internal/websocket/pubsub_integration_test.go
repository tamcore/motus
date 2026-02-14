package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/tamcore/motus/internal/model"
)

// mockPubSub is a test double that records published messages and allows
// injecting received messages. It implements pubsub.PubSub.
type mockPubSub struct {
	mu        sync.Mutex
	published []redisEnvelope
	handler   func([]byte)
}

func (m *mockPubSub) Publish(_ context.Context, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	var env redisEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	m.mu.Lock()
	m.published = append(m.published, env)
	m.mu.Unlock()
	return nil
}

func (m *mockPubSub) Subscribe(_ context.Context, handler func([]byte)) error {
	m.mu.Lock()
	m.handler = handler
	m.mu.Unlock()
	return nil
}

func (m *mockPubSub) Close() error { return nil }

// simulateRemoteMessage simulates a message arriving from Redis (from another pod).
// The envelope uses "remote-pod" as the origin, which differs from any hub's podID.
func (m *mockPubSub) simulateRemoteMessage(deviceID int64, msg TraccarMessage) {
	m.simulateMessageFromPod("remote-pod", deviceID, msg)
}

// simulateMessageFromPod simulates a Redis message with a specific origin pod ID.
func (m *mockPubSub) simulateMessageFromPod(originPodID string, deviceID int64, msg TraccarMessage) {
	env := redisEnvelope{
		OriginPodID: originPodID,
		DeviceID:    deviceID,
		Message:     msg,
	}
	data, _ := json.Marshal(env)
	m.mu.Lock()
	h := m.handler
	m.mu.Unlock()
	if h != nil {
		h(data)
	}
}

func (m *mockPubSub) getPublished() []redisEnvelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]redisEnvelope, len(m.published))
	copy(result, m.published)
	return result
}

// connectTestClient connects a WebSocket client to the hub's test server.
func connectTestClient(t *testing.T, hub *Hub) *ws.Conn {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(hub.HandleConnect))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	time.Sleep(50 * time.Millisecond)
	return conn
}

func TestBroadcastPosition_PublishesToRedis(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	mock := &mockPubSub{}
	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	hub.SetPubSub(mock)

	conn := connectTestClient(t, hub)

	pos := &model.Position{ID: 1, DeviceID: 10, Latitude: 52.0, Longitude: 13.0, Timestamp: time.Now().UTC()}
	hub.BroadcastPosition(pos)

	// Verify published to Redis.
	published := mock.getPublished()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}
	if published[0].DeviceID != 10 {
		t.Errorf("expected deviceID 10, got %d", published[0].DeviceID)
	}
	if len(published[0].Message.Positions) != 1 {
		t.Errorf("expected 1 position in envelope, got %d", len(published[0].Message.Positions))
	}
	// Verify envelope includes origin pod ID.
	if published[0].OriginPodID != hub.podID {
		t.Errorf("expected originPodId %q, got %q", hub.podID, published[0].OriginPodID)
	}

	// Verify local client still received the message.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("client failed to read: %v", err)
	}
	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Positions) != 1 {
		t.Errorf("expected 1 position, got %d", len(traccarMsg.Positions))
	}
}

func TestBroadcastDeviceStatus_PublishesToRedis(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{42: {1}},
	}
	mock := &mockPubSub{}
	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	hub.SetPubSub(mock)

	connectTestClient(t, hub)

	device := &model.Device{ID: 42, UniqueID: "test", Name: "Test", Status: "online"}
	hub.BroadcastDeviceStatus(device)

	published := mock.getPublished()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}
	if published[0].DeviceID != 42 {
		t.Errorf("expected deviceID 42, got %d", published[0].DeviceID)
	}
	if len(published[0].Message.Devices) != 1 {
		t.Errorf("expected 1 device in envelope, got %d", len(published[0].Message.Devices))
	}
}

func TestBroadcastEvent_PublishesToRedis(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{42: {1}},
	}
	mock := &mockPubSub{}
	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	hub.SetPubSub(mock)

	connectTestClient(t, hub)

	event := &model.Event{ID: 1, DeviceID: 42, Type: "geofenceEnter", Timestamp: time.Now().UTC()}
	hub.BroadcastEvent(event)

	published := mock.getPublished()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}
	if published[0].DeviceID != 42 {
		t.Errorf("expected deviceID 42, got %d", published[0].DeviceID)
	}
	if len(published[0].Message.Events) != 1 {
		t.Errorf("expected 1 event in envelope, got %d", len(published[0].Message.Events))
	}
}

func TestBroadcastPosition_NoPubSub_LocalOnly(t *testing.T) {
	// Without pub/sub configured, should still work (local broadcast only).
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	// No SetPubSub call.

	conn := connectTestClient(t, hub)

	pos := &model.Position{ID: 1, DeviceID: 10, Latitude: 52.0, Longitude: 13.0, Timestamp: time.Now().UTC()}
	hub.BroadcastPosition(pos)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("client failed to read: %v", err)
	}
	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Positions) != 1 {
		t.Errorf("expected 1 position, got %d", len(traccarMsg.Positions))
	}
}

func TestStartSubscriber_RelaysRemoteMessages(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	mock := &mockPubSub{}
	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	hub.SetPubSub(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the subscriber (this registers the handler on mock).
	go hub.StartSubscriber(ctx)
	time.Sleep(50 * time.Millisecond) // Let subscriber start.

	// Connect a client.
	conn := connectTestClient(t, hub)

	// Simulate a message arriving from another pod via Redis.
	remotePos := model.Position{ID: 99, DeviceID: 10, Latitude: 48.0, Longitude: 11.0, Timestamp: time.Now().UTC()}
	remoteMsg := TraccarMessage{Positions: []model.Position{remotePos}}
	mock.simulateRemoteMessage(10, remoteMsg)

	// The local client should receive the relayed message.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("client failed to read relayed message: %v", err)
	}
	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Positions) != 1 {
		t.Errorf("expected 1 relayed position, got %d", len(traccarMsg.Positions))
	}
	if traccarMsg.Positions[0].ID != 99 {
		t.Errorf("expected position ID 99, got %d", traccarMsg.Positions[0].ID)
	}
}

func TestStartSubscriber_FiltersRemoteByAccess(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{
			10: {1}, // User 1 has access to device 10
			// User 2 does NOT have access to device 10
		},
	}
	mock := &mockPubSub{}

	userIDCounter := int64(0)
	hub := NewHub(nil, checker, func(_ *http.Request) int64 {
		userIDCounter++
		return userIDCounter
	})
	hub.SetPubSub(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.StartSubscriber(ctx)
	time.Sleep(50 * time.Millisecond)

	// Client 1 (user 1) - has access to device 10.
	conn1 := connectTestClient(t, hub)
	// Client 2 (user 2) - does NOT have access to device 10.
	conn2 := connectTestClient(t, hub)

	// Simulate remote message for device 10.
	remoteMsg := TraccarMessage{
		Positions: []model.Position{{ID: 100, DeviceID: 10, Latitude: 48.0, Longitude: 11.0, Timestamp: time.Now().UTC()}},
	}
	mock.simulateRemoteMessage(10, remoteMsg)

	// Client 1 should receive.
	_ = conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn1.ReadMessage()
	if err != nil {
		t.Fatalf("client 1 should have received the message: %v", err)
	}

	// Client 2 should NOT receive.
	_ = conn2.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = conn2.ReadMessage()
	if err == nil {
		t.Error("client 2 should not have received the message (no access to device 10)")
	}
}

func TestStartSubscriber_NoPubSub_Blocks(t *testing.T) {
	// With no pub/sub configured, StartSubscriber should block until context done.
	hub := NewHub(nil, nil, dummyExtractor)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hub.StartSubscriber(ctx)
		close(done)
	}()

	// Cancel and verify it returns.
	cancel()
	select {
	case <-done:
		// Expected.
	case <-time.After(2 * time.Second):
		t.Fatal("StartSubscriber did not return after context cancellation")
	}
}

func TestStartSubscriber_SkipsSelfEcho(t *testing.T) {
	// When a pod publishes a message, its own subscriber should skip it
	// (self-echo prevention via OriginPodID matching).
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}
	mock := &mockPubSub{}
	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	hub.SetPubSub(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the subscriber.
	go hub.StartSubscriber(ctx)
	time.Sleep(50 * time.Millisecond)

	// Connect a client.
	conn := connectTestClient(t, hub)

	// Simulate a Redis message that came from THIS pod (self-echo).
	selfMsg := TraccarMessage{
		Positions: []model.Position{{ID: 999, DeviceID: 10, Latitude: 50.0, Longitude: 10.0, Timestamp: time.Now().UTC()}},
	}
	mock.simulateMessageFromPod(hub.podID, 10, selfMsg)

	// Then immediately simulate a message from a DIFFERENT pod.
	// If self-echo was NOT skipped, the client would receive TWO messages
	// (self-echo first, then remote). If correctly skipped, only the remote
	// message arrives.
	remoteMsg := TraccarMessage{
		Positions: []model.Position{{ID: 1000, DeviceID: 10, Latitude: 48.0, Longitude: 11.0, Timestamp: time.Now().UTC()}},
	}
	mock.simulateRemoteMessage(10, remoteMsg)

	// Read the first (and only expected) message.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("client should have received message from remote pod: %v", err)
	}
	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Positions) != 1 {
		t.Errorf("expected 1 position, got %d", len(traccarMsg.Positions))
	}
	// The message should be from the remote pod (ID 1000), not the self-echo (ID 999).
	if traccarMsg.Positions[0].ID != 1000 {
		t.Errorf("expected position ID 1000 (remote), got %d (self-echo was not prevented)", traccarMsg.Positions[0].ID)
	}

	// Verify no second message arrives (self-echo was skipped).
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("received unexpected second message -- self-echo was NOT prevented")
	}
}

func TestBroadcastPosition_NoDoubleDelivery(t *testing.T) {
	// When Redis pub/sub is active and the subscriber receives the self-echo,
	// the client should receive exactly ONE message (from the local broadcast),
	// not two (local + self-echo via subscriber).
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{10: {1}},
	}

	// This mock simulates realistic Redis behavior: when Publish is called,
	// it also delivers the message to the subscriber handler (self-echo).
	selfEchoMock := &selfEchoPubSub{}
	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	hub.SetPubSub(selfEchoMock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.StartSubscriber(ctx)
	time.Sleep(50 * time.Millisecond)

	conn := connectTestClient(t, hub)

	// BroadcastPosition will:
	// 1. Publish to Redis (which triggers self-echo to subscriber)
	// 2. Broadcast locally to clients
	// The subscriber should skip the self-echo, so client gets exactly 1 message.
	pos := &model.Position{ID: 1, DeviceID: 10, Latitude: 52.0, Longitude: 13.0, Timestamp: time.Now().UTC()}
	hub.BroadcastPosition(pos)

	// Read the first message (from local broadcast).
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("client should have received the local broadcast: %v", err)
	}
	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Positions) != 1 {
		t.Errorf("expected 1 position, got %d", len(traccarMsg.Positions))
	}

	// There should NOT be a second message (the self-echo should be skipped).
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("client received a second message (double delivery) -- self-echo was NOT prevented")
	}
}

// selfEchoPubSub simulates real Redis behavior: when Publish is called, it
// also delivers the message to the Subscribe handler (self-echo), just like
// Redis pub/sub would do when a client is both publisher and subscriber.
type selfEchoPubSub struct {
	mu      sync.Mutex
	handler func([]byte)
}

func (s *selfEchoPubSub) Publish(_ context.Context, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	s.mu.Lock()
	h := s.handler
	s.mu.Unlock()
	// Deliver to subscriber (self-echo), like real Redis would.
	if h != nil {
		go h(data) // async, like real Redis delivery
	}
	return nil
}

func (s *selfEchoPubSub) Subscribe(_ context.Context, handler func([]byte)) error {
	s.mu.Lock()
	s.handler = handler
	s.mu.Unlock()
	return nil
}

func (s *selfEchoPubSub) Close() error { return nil }
