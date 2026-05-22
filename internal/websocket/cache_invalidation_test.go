package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

// mockInvalidationPubSub records published invalidation envelopes and lets
// tests inject incoming messages. Implements pubsub.PubSub.
type mockInvalidationPubSub struct {
	mu         sync.Mutex
	published  []invalidationEnvelope
	handler    func([]byte)
	publishErr error
}

func (m *mockInvalidationPubSub) Publish(_ context.Context, message interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.publishErr != nil {
		return m.publishErr
	}
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	var env invalidationEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	m.published = append(m.published, env)
	return nil
}

func (m *mockInvalidationPubSub) Subscribe(_ context.Context, handler func([]byte)) error {
	m.mu.Lock()
	m.handler = handler
	m.mu.Unlock()
	return nil
}

func (m *mockInvalidationPubSub) Close() error { return nil }

func (m *mockInvalidationPubSub) getPublished() []invalidationEnvelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]invalidationEnvelope, len(m.published))
	copy(result, m.published)
	return result
}

func (m *mockInvalidationPubSub) simulateRemoteInvalidation(originPodID string, deviceID int64) {
	env := invalidationEnvelope{OriginPodID: originPodID, DeviceID: deviceID}
	data, _ := json.Marshal(env)
	m.mu.Lock()
	h := m.handler
	m.mu.Unlock()
	if h != nil {
		h(data)
	}
}

// pollUntil polls cond every 5ms until it returns true or 500ms elapse.
func pollUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within deadline")
}

// --- Tests ---

func TestInvalidateDevice_LocalInvalidateAlwaysHappens(t *testing.T) {
	hub := NewHub(nil, &mockAccessChecker{deviceUsers: map[int64][]int64{}}, func(_ *http.Request) int64 { return 0 })
	hub.accessCache.set(1, []int64{10})

	hub.InvalidateDevice(1)

	if _, ok := hub.accessCache.get(1); ok {
		t.Error("expected cache entry to be removed after InvalidateDevice")
	}
}

func TestInvalidateDevice_NoPublishWhenPubSubNil(t *testing.T) {
	hub := NewHub(nil, &mockAccessChecker{deviceUsers: map[int64][]int64{}}, func(_ *http.Request) int64 { return 0 })
	hub.accessCache.set(1, []int64{10})

	hub.InvalidateDevice(1)

	if _, ok := hub.accessCache.get(1); ok {
		t.Error("expected cache entry removed")
	}
}

func TestInvalidateDevice_PublishesEnvelopeWhenPubSubSet(t *testing.T) {
	hub := NewHub(nil, &mockAccessChecker{deviceUsers: map[int64][]int64{}}, func(_ *http.Request) int64 { return 0 })
	ps := &mockInvalidationPubSub{}
	hub.SetInvalidationPubSub(ps)
	hub.accessCache.set(42, []int64{10})

	hub.InvalidateDevice(42)

	published := ps.getPublished()
	if len(published) != 1 {
		t.Fatalf("expected 1 published envelope, got %d", len(published))
	}
	if published[0].DeviceID != 42 {
		t.Errorf("expected DeviceID 42, got %d", published[0].DeviceID)
	}
	if published[0].OriginPodID != hub.podID {
		t.Errorf("expected OriginPodID %q, got %q", hub.podID, published[0].OriginPodID)
	}
	if _, ok := hub.accessCache.get(42); ok {
		t.Error("expected local cache entry removed")
	}
}

func TestInvalidateDevice_LocalInvalidateHappensEvenOnPublishError(t *testing.T) {
	hub := NewHub(nil, &mockAccessChecker{deviceUsers: map[int64][]int64{}}, func(_ *http.Request) int64 { return 0 })
	ps := &mockInvalidationPubSub{publishErr: errors.New("redis down")}
	hub.SetInvalidationPubSub(ps)
	hub.accessCache.set(5, []int64{10})

	hub.InvalidateDevice(5)

	if _, ok := hub.accessCache.get(5); ok {
		t.Error("expected local cache entry removed even when publish fails")
	}
}

func TestStartInvalidationSubscriber_InvalidatesOnRemoteEvent(t *testing.T) {
	hub := NewHub(nil, &mockAccessChecker{deviceUsers: map[int64][]int64{}}, func(_ *http.Request) int64 { return 0 })
	ps := &mockInvalidationPubSub{}
	hub.SetInvalidationPubSub(ps)
	hub.accessCache.set(7, []int64{10})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.StartInvalidationSubscriber(ctx)

	pollUntil(t, func() bool {
		ps.mu.Lock()
		defer ps.mu.Unlock()
		return ps.handler != nil
	})

	ps.simulateRemoteInvalidation("other-pod", 7)

	pollUntil(t, func() bool {
		_, ok := hub.accessCache.get(7)
		return !ok
	})
}

func TestStartInvalidationSubscriber_IgnoresSelfEcho(t *testing.T) {
	hub := NewHub(nil, &mockAccessChecker{deviceUsers: map[int64][]int64{}}, func(_ *http.Request) int64 { return 0 })
	ps := &mockInvalidationPubSub{}
	hub.SetInvalidationPubSub(ps)
	hub.accessCache.set(9, []int64{10})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.StartInvalidationSubscriber(ctx)

	pollUntil(t, func() bool {
		ps.mu.Lock()
		defer ps.mu.Unlock()
		return ps.handler != nil
	})

	ps.simulateRemoteInvalidation(hub.podID, 9)

	time.Sleep(50 * time.Millisecond)
	if _, ok := hub.accessCache.get(9); !ok {
		t.Error("self-echo: cache entry should NOT be removed")
	}
}

func TestStartInvalidationSubscriber_NoOpWhenPubSubNil(t *testing.T) {
	hub := NewHub(nil, &mockAccessChecker{deviceUsers: map[int64][]int64{}}, func(_ *http.Request) int64 { return 0 })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hub.StartInvalidationSubscriber(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StartInvalidationSubscriber did not return after context cancel")
	}
}
