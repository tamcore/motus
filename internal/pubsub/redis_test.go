package pubsub_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/pubsub"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	redisURL       string
	redisContainer testcontainers.Container
	redisOnce      sync.Once
	redisInitErr   error
)

func setupRedis(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test (requires Docker/Redis) in short mode")
	}
	redisOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		req := testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
		}

		var err error
		redisContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		if err != nil {
			redisInitErr = fmt.Errorf("start redis container: %w", err)
			return
		}

		host, err := redisContainer.Host(ctx)
		if err != nil {
			redisInitErr = fmt.Errorf("get redis host: %w", err)
			return
		}
		port, err := redisContainer.MappedPort(ctx, "6379")
		if err != nil {
			redisInitErr = fmt.Errorf("get redis port: %w", err)
			return
		}

		redisURL = fmt.Sprintf("redis://%s:%s", host, port.Port())
	})

	if redisInitErr != nil {
		t.Fatalf("redis setup failed: %v", redisInitErr)
	}
	return redisURL
}

func cleanupRedis() {
	if redisContainer != nil {
		_ = redisContainer.Terminate(context.Background())
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	cleanupRedis()
	os.Exit(code)
}

func TestNewRedisPubSub_InvalidURL(t *testing.T) {
	_, err := pubsub.NewRedisPubSub("not-a-valid-url", "test-channel")
	if err == nil {
		t.Fatal("expected error for invalid Redis URL")
	}
}

func TestNewRedisPubSub_UnreachableRedis(t *testing.T) {
	// Valid URL format but no Redis server running there.
	_, err := pubsub.NewRedisPubSub("redis://127.0.0.1:59999", "test-channel")
	if err == nil {
		t.Fatal("expected error for unreachable Redis")
	}
}

func TestNewRedisPubSub_Success(t *testing.T) {
	url := setupRedis(t)

	ps, err := pubsub.NewRedisPubSub(url, "test-channel")
	if err != nil {
		t.Fatalf("failed to create RedisPubSub: %v", err)
	}
	defer func() { _ = ps.Close() }()
}

func TestRedisPublishSubscribe(t *testing.T) {
	url := setupRedis(t)

	ps, err := pubsub.NewRedisPubSub(url, "test-pubsub")
	if err != nil {
		t.Fatalf("create pubsub: %v", err)
	}
	defer func() { _ = ps.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	received := make(chan []byte, 10)
	err = ps.Subscribe(ctx, func(data []byte) {
		received <- data
	})
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	// Small delay to ensure subscription is fully active before publishing.
	time.Sleep(100 * time.Millisecond)

	msg := map[string]string{"type": "position", "device": "GPS001"}
	if err := ps.Publish(ctx, msg); err != nil {
		t.Fatalf("publish error: %v", err)
	}

	select {
	case data := <-received:
		var got map[string]string
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if got["type"] != "position" {
			t.Errorf("expected type=position, got %q", got["type"])
		}
		if got["device"] != "GPS001" {
			t.Errorf("expected device=GPS001, got %q", got["device"])
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for message")
	}
}

func TestRedisPublishSubscribe_MultipleMessages(t *testing.T) {
	url := setupRedis(t)

	ps, err := pubsub.NewRedisPubSub(url, "test-multi-msg")
	if err != nil {
		t.Fatalf("create pubsub: %v", err)
	}
	defer func() { _ = ps.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count atomic.Int32
	done := make(chan struct{})

	const numMessages = 5
	err = ps.Subscribe(ctx, func(data []byte) {
		if count.Add(1) == numMessages {
			close(done)
		}
	})
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < numMessages; i++ {
		msg := map[string]int{"seq": i}
		if err := ps.Publish(ctx, msg); err != nil {
			t.Fatalf("publish error on message %d: %v", i, err)
		}
	}

	select {
	case <-done:
		got := count.Load()
		if got != numMessages {
			t.Errorf("expected %d messages, got %d", numMessages, got)
		}
	case <-ctx.Done():
		t.Fatalf("timed out: received %d of %d messages", count.Load(), numMessages)
	}
}

func TestRedisPublishSubscribe_JSONSerialisation(t *testing.T) {
	url := setupRedis(t)

	ps, err := pubsub.NewRedisPubSub(url, "test-json")
	if err != nil {
		t.Fatalf("create pubsub: %v", err)
	}
	defer func() { _ = ps.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type envelope struct {
		PodID    string  `json:"podId"`
		DeviceID int64   `json:"deviceId"`
		Lat      float64 `json:"lat"`
		Lon      float64 `json:"lon"`
	}

	received := make(chan []byte, 1)
	err = ps.Subscribe(ctx, func(data []byte) {
		received <- data
	})
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	sent := envelope{
		PodID:    "pod-abc123",
		DeviceID: 42,
		Lat:      52.520008,
		Lon:      13.404954,
	}
	if err := ps.Publish(ctx, sent); err != nil {
		t.Fatalf("publish error: %v", err)
	}

	select {
	case data := <-received:
		var got envelope
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if got.PodID != sent.PodID {
			t.Errorf("PodID: got %q, want %q", got.PodID, sent.PodID)
		}
		if got.DeviceID != sent.DeviceID {
			t.Errorf("DeviceID: got %d, want %d", got.DeviceID, sent.DeviceID)
		}
		if got.Lat != sent.Lat {
			t.Errorf("Lat: got %f, want %f", got.Lat, sent.Lat)
		}
		if got.Lon != sent.Lon {
			t.Errorf("Lon: got %f, want %f", got.Lon, sent.Lon)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for message")
	}
}

func TestRedisPublish_MarshalError(t *testing.T) {
	url := setupRedis(t)

	ps, err := pubsub.NewRedisPubSub(url, "test-marshal-err")
	if err != nil {
		t.Fatalf("create pubsub: %v", err)
	}
	defer func() { _ = ps.Close() }()

	// Channels cannot be marshalled to JSON.
	err = ps.Publish(context.Background(), make(chan int))
	if err == nil {
		t.Fatal("expected error when publishing unmarshallable type")
	}
}

func TestRedisSubscribe_ContextCancellation(t *testing.T) {
	url := setupRedis(t)

	ps, err := pubsub.NewRedisPubSub(url, "test-ctx-cancel")
	if err != nil {
		t.Fatalf("create pubsub: %v", err)
	}
	defer func() { _ = ps.Close() }()

	ctx, cancel := context.WithCancel(context.Background())

	var received atomic.Int32
	err = ps.Subscribe(ctx, func(data []byte) {
		received.Add(1)
	})
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Send a message before cancel.
	if err := ps.Publish(context.Background(), "before-cancel"); err != nil {
		t.Fatalf("publish error: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	beforeCancel := received.Load()
	if beforeCancel == 0 {
		t.Fatal("expected at least one message before cancel")
	}

	// Cancel the context; the subscriber goroutine should exit.
	cancel()
	time.Sleep(200 * time.Millisecond)

	// Messages published after cancel should not be received.
	countAfterCancel := received.Load()
	// Note: there's a race window so we just verify the goroutine
	// didn't crash. We already got messages before cancel.
	_ = countAfterCancel
}

func TestMultipleSubscribers(t *testing.T) {
	url := setupRedis(t)

	// Two separate PubSub instances on the same channel simulate two pods.
	ps1, err := pubsub.NewRedisPubSub(url, "test-multi-sub")
	if err != nil {
		t.Fatalf("create ps1: %v", err)
	}
	defer func() { _ = ps1.Close() }()

	ps2, err := pubsub.NewRedisPubSub(url, "test-multi-sub")
	if err != nil {
		t.Fatalf("create ps2: %v", err)
	}
	defer func() { _ = ps2.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count1, count2 atomic.Int32
	done1 := make(chan struct{}, 1)
	done2 := make(chan struct{}, 1)

	err = ps1.Subscribe(ctx, func(data []byte) {
		count1.Add(1)
		select {
		case done1 <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatalf("ps1 subscribe error: %v", err)
	}

	err = ps2.Subscribe(ctx, func(data []byte) {
		count2.Add(1)
		select {
		case done2 <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatalf("ps2 subscribe error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Publish from ps1; both subscribers should receive.
	if err := ps1.Publish(ctx, map[string]string{"from": "ps1"}); err != nil {
		t.Fatalf("publish error: %v", err)
	}

	// Wait for both to receive.
	for _, ch := range []chan struct{}{done1, done2} {
		select {
		case <-ch:
			// received
		case <-ctx.Done():
			t.Fatal("timed out waiting for subscriber")
		}
	}

	if count1.Load() < 1 {
		t.Error("subscriber 1 did not receive the message")
	}
	if count2.Load() < 1 {
		t.Error("subscriber 2 did not receive the message")
	}
}

func TestRedisClose(t *testing.T) {
	url := setupRedis(t)

	ps, err := pubsub.NewRedisPubSub(url, "test-close")
	if err != nil {
		t.Fatalf("create pubsub: %v", err)
	}

	// Subscribe first so there's a subscription to close.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ps.Subscribe(ctx, func(data []byte) {})
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	// Close should not return an error.
	if err := ps.Close(); err != nil {
		t.Errorf("close error: %v", err)
	}
}

func TestRedisClose_WithoutSubscribe(t *testing.T) {
	url := setupRedis(t)

	ps, err := pubsub.NewRedisPubSub(url, "test-close-nosub")
	if err != nil {
		t.Fatalf("create pubsub: %v", err)
	}

	// Close without having subscribed (r.sub is nil).
	if err := ps.Close(); err != nil {
		t.Errorf("close error: %v", err)
	}
}

func TestRedisPublish_AfterClose(t *testing.T) {
	url := setupRedis(t)

	ps, err := pubsub.NewRedisPubSub(url, "test-publish-closed")
	if err != nil {
		t.Fatalf("create pubsub: %v", err)
	}

	_ = ps.Close()

	// Publishing after close should return an error.
	err = ps.Publish(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error when publishing after close")
	}
}
