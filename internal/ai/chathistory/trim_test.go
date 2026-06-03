package chathistory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/tamcore/motus/internal/ai/chat"
)

func newTestRDB(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, mr
}

func pushMessages(t *testing.T, rdb *redis.Client, k string, msgs []chat.Message) {
	t.Helper()
	for _, m := range msgs {
		b, _ := json.Marshal(m)
		rdb.RPush(context.Background(), k, string(b))
	}
}

func buildTurns(n int) []chat.Message {
	msgs := make([]chat.Message, 0, n*2)
	for i := 0; i < n; i++ {
		msgs = append(msgs,
			chat.Message{Role: "user", Content: "q"},
			chat.Message{Role: "assistant", Content: "a"},
		)
	}
	return msgs
}

func TestTrim_WithinLimits(t *testing.T) {
	rdb, _ := newTestRDB(t)
	k := "test:trim:within"
	pushMessages(t, rdb, k, buildTurns(5))

	if err := trimKey(context.Background(), rdb, k); err != nil {
		t.Fatalf("trimKey: %v", err)
	}

	count := rdb.LLen(context.Background(), k).Val()
	if count != 10 {
		t.Errorf("want 10 entries (5 turns × 2), got %d", count)
	}
}

func TestTrim_ExceedMaxTurns(t *testing.T) {
	rdb, _ := newTestRDB(t)
	k := "test:trim:turns"
	pushMessages(t, rdb, k, buildTurns(MaxTurns+5))

	if err := trimKey(context.Background(), rdb, k); err != nil {
		t.Fatalf("trimKey: %v", err)
	}

	vals, _ := rdb.LRange(context.Background(), k, 0, -1).Result()
	userCount := 0
	for _, v := range vals {
		var m chat.Message
		if json.Unmarshal([]byte(v), &m) == nil && m.Role == "user" {
			userCount++
		}
	}
	if userCount > MaxTurns {
		t.Errorf("want ≤%d user turns after trim, got %d", MaxTurns, userCount)
	}
}

func TestTrim_ExceedMaxBytes(t *testing.T) {
	rdb, _ := newTestRDB(t)
	k := "test:trim:bytes"

	bigContent := strings.Repeat("x", MaxBytes/4)
	msgs := []chat.Message{
		{Role: "user", Content: bigContent},
		{Role: "assistant", Content: bigContent},
		{Role: "user", Content: bigContent},
		{Role: "assistant", Content: bigContent},
		{Role: "user", Content: bigContent},
	}
	pushMessages(t, rdb, k, msgs)

	if err := trimKey(context.Background(), rdb, k); err != nil {
		t.Fatalf("trimKey: %v", err)
	}

	vals, _ := rdb.LRange(context.Background(), k, 0, -1).Result()
	total := 0
	for _, v := range vals {
		total += len(v)
	}
	if total > MaxBytes {
		t.Errorf("want total ≤%d bytes after trim, got %d", MaxBytes, total)
	}
}

func TestTrim_OrphanToolMessageDropped(t *testing.T) {
	rdb, _ := newTestRDB(t)
	k := "test:trim:orphan"

	bigContent := strings.Repeat("x", MaxBytes/3)
	msgs := []chat.Message{
		{Role: "user", Content: bigContent},
		{Role: "assistant", ToolCalls: []chat.ToolCall{{ID: "t1", Name: "fn", Arguments: "{}"}}},
		{Role: "tool", Content: `{"ok":true}`, ToolCallID: "t1"},
		// Second turn to keep:
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	}
	pushMessages(t, rdb, k, msgs)

	if err := trimKey(context.Background(), rdb, k); err != nil {
		t.Fatalf("trimKey: %v", err)
	}

	vals, _ := rdb.LRange(context.Background(), k, 0, -1).Result()
	if len(vals) == 0 {
		t.Fatal("list is empty after trim")
	}
	var first chat.Message
	_ = json.Unmarshal([]byte(vals[0]), &first)
	if first.Role == "tool" {
		t.Errorf("head should not be a tool message after trim, got role=%q", first.Role)
	}
}

func TestTrimKey_EmptyListIsNoop(t *testing.T) {
	rdb, _ := newTestRDB(t)
	k := "test:trim:empty"

	if err := trimKey(context.Background(), rdb, k); err != nil {
		t.Fatalf("trimKey on empty key: %v", err)
	}
}

func TestStore_AppendTrimsAutomatically(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewStore(rdb, time.Hour)
	ctx := context.Background()

	for i := 0; i < MaxTurns+5; i++ {
		_ = store.Append(ctx, 1,
			chat.Message{Role: "user", Content: "q"},
			chat.Message{Role: "assistant", Content: "a"},
		)
	}

	got, _ := store.Get(ctx, 1)
	userCount := 0
	for _, m := range got {
		if m.Role == "user" {
			userCount++
		}
	}
	if userCount > MaxTurns {
		t.Errorf("want ≤%d user turns after auto-trim, got %d", MaxTurns, userCount)
	}
}
