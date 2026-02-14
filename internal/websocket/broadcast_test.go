package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/tamcore/motus/internal/model"
)

func connectClient(t *testing.T, hub *Hub) *ws.Conn {
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

func TestBroadcastPosition_FiltersByUser(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{
			10: {1},
		},
	}

	userIDCounter := int64(0)
	hub := NewHub(nil, checker, func(_ *http.Request) int64 {
		userIDCounter++
		return userIDCounter
	})

	// Client 1 (userID=1) should receive, client 2 (userID=2) should not.
	conn1 := connectClient(t, hub)
	conn2 := connectClient(t, hub)

	pos := &model.Position{ID: 1, DeviceID: 10, Latitude: 52.0, Longitude: 13.0, Timestamp: time.Now().UTC()}
	hub.BroadcastPosition(pos)

	// Client 1 should get the message.
	_ = conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn1.ReadMessage()
	if err != nil {
		t.Fatalf("client 1 failed to read: %v", err)
	}

	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Positions) != 1 {
		t.Errorf("client 1: expected 1 position, got %d", len(traccarMsg.Positions))
	}

	// Client 2 should NOT get the message (timeout expected).
	_ = conn2.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = conn2.ReadMessage()
	if err == nil {
		t.Error("client 2 should not have received the message")
	}
}

func TestBroadcastDeviceStatus_AllowedUser(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{
			42: {1},
		},
	}

	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	conn := connectClient(t, hub)

	device := &model.Device{ID: 42, UniqueID: "test", Name: "Test", Status: "online"}
	hub.BroadcastDeviceStatus(device)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(traccarMsg.Devices))
	}
	if traccarMsg.Devices[0].Status != "online" {
		t.Errorf("expected status 'online', got %q", traccarMsg.Devices[0].Status)
	}
}

func TestBroadcastEvent_AllowedUser(t *testing.T) {
	checker := &mockAccessChecker{
		deviceUsers: map[int64][]int64{
			42: {1},
		},
	}

	hub := NewHub(nil, checker, func(_ *http.Request) int64 { return 1 })
	conn := connectClient(t, hub)

	event := &model.Event{ID: 1, DeviceID: 42, Type: "geofenceEnter", Timestamp: time.Now().UTC()}
	hub.BroadcastEvent(event)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	var traccarMsg TraccarMessage
	_ = json.Unmarshal(msg, &traccarMsg)
	if len(traccarMsg.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(traccarMsg.Events))
	}
}

func TestBroadcast_NilAccessChecker(t *testing.T) {
	// With nil access checker, getAllowedUserIDs returns nil,
	// and userIDInSlice returns false for any ID, so no messages delivered.
	hub := NewHub(nil, nil, func(_ *http.Request) int64 { return 1 })
	conn := connectClient(t, hub)

	pos := &model.Position{ID: 1, DeviceID: 1, Latitude: 52.0, Longitude: 13.0, Timestamp: time.Now().UTC()}
	hub.BroadcastPosition(pos)

	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Error("expected no message when access checker is nil")
	}
}
