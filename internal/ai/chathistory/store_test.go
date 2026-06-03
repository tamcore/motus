package chathistory

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/tamcore/motus/internal/ai/chat"
)

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewStore(rdb, time.Hour), mr
}

func TestStore_RoundTrip(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	msgs := []chat.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	if err := store.Append(ctx, 1, msgs...); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := store.Get(ctx, 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 messages, got %d", len(got))
	}
	if got[0].Content != "hello" || got[1].Content != "world" {
		t.Errorf("unexpected messages: %+v", got)
	}
}

func TestStore_MissingKeyReturnsEmpty(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	got, err := store.Get(ctx, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %d messages", len(got))
	}
}

func TestStore_Clear(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	_ = store.Append(ctx, 1, chat.Message{Role: "user", Content: "hi"})
	if err := store.Clear(ctx, 1); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	got, _ := store.Get(ctx, 1)
	if len(got) != 0 {
		t.Errorf("want empty after Clear, got %d messages", len(got))
	}
}

func TestStore_SlidingTTL(t *testing.T) {
	store, mr := newTestStore(t)
	ctx := context.Background()

	_ = store.Append(ctx, 1, chat.Message{Role: "user", Content: "hi"})
	mr.FastForward(50 * time.Minute)

	// Append again — should refresh TTL to another hour.
	_ = store.Append(ctx, 1, chat.Message{Role: "assistant", Content: "hello"})
	mr.FastForward(50 * time.Minute) // 100m total, but TTL refreshed at 50m mark

	got, _ := store.Get(ctx, 1)
	if len(got) != 2 {
		t.Errorf("want 2 messages (TTL refreshed), got %d", len(got))
	}
}

func TestStore_CrossUserIsolation(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	_ = store.Append(ctx, 1, chat.Message{Role: "user", Content: "user1"})
	_ = store.Append(ctx, 2, chat.Message{Role: "user", Content: "user2"})

	got1, _ := store.Get(ctx, 1)
	got2, _ := store.Get(ctx, 2)

	if len(got1) != 1 || got1[0].Content != "user1" {
		t.Errorf("user1 isolation failed: %+v", got1)
	}
	if len(got2) != 1 || got2[0].Content != "user2" {
		t.Errorf("user2 isolation failed: %+v", got2)
	}
}
