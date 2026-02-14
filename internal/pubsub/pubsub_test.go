package pubsub_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/pubsub"
)

// mockPubSub is a test double that implements pubsub.PubSub.
type mockPubSub struct {
	mu        sync.Mutex
	published [][]byte
	handler   func([]byte)
}

func newMockPubSub() *mockPubSub {
	return &mockPubSub{}
}

func (m *mockPubSub) Publish(_ context.Context, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.published = append(m.published, data)
	m.mu.Unlock()
	// Simulate Redis by delivering to the subscriber immediately.
	if m.handler != nil {
		m.handler(data)
	}
	return nil
}

func (m *mockPubSub) Subscribe(_ context.Context, handler func([]byte)) error {
	m.handler = handler
	return nil
}

func (m *mockPubSub) Close() error {
	return nil
}

// Verify mockPubSub satisfies the PubSub interface.
var _ pubsub.PubSub = (*mockPubSub)(nil)

func TestMockPubSub_PublishSubscribe(t *testing.T) {
	mock := newMockPubSub()

	received := make(chan []byte, 1)
	err := mock.Subscribe(context.Background(), func(data []byte) {
		received <- data
	})
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	msg := map[string]string{"hello": "world"}
	if err := mock.Publish(context.Background(), msg); err != nil {
		t.Fatalf("publish error: %v", err)
	}

	select {
	case data := <-received:
		var got map[string]string
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if got["hello"] != "world" {
			t.Errorf("expected hello=world, got %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}

	mock.mu.Lock()
	if len(mock.published) != 1 {
		t.Errorf("expected 1 published message, got %d", len(mock.published))
	}
	mock.mu.Unlock()
}

func TestMockPubSub_Close(t *testing.T) {
	mock := newMockPubSub()
	if err := mock.Close(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
