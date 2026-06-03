package chathistory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tamcore/motus/internal/ai/chat"
)

// Store persists per-user chat history in Redis with a sliding TTL.
// The key is motus:chat:history:<userID> — one conversation per user.
type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewStore creates a Store backed by rdb with the given TTL.
func NewStore(rdb *redis.Client, ttl time.Duration) *Store {
	return &Store{rdb: rdb, ttl: ttl}
}

func histKey(userID int64) string {
	return fmt.Sprintf("motus:chat:history:%d", userID)
}

// Get returns the stored messages for userID. Returns an empty slice when
// no history exists (missing key or expired TTL) — never an error in that case.
func (s *Store) Get(ctx context.Context, userID int64) ([]chat.Message, error) {
	vals, err := s.rdb.LRange(ctx, histKey(userID), 0, -1).Result()
	if err == redis.Nil || len(vals) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	msgs := make([]chat.Message, 0, len(vals))
	for _, v := range vals {
		var m chat.Message
		if json.Unmarshal([]byte(v), &m) == nil {
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}

// Append serialises msgs and pushes them to the tail of the Redis list,
// resets the sliding TTL, then trims the list to stay within bounds.
func (s *Store) Append(ctx context.Context, userID int64, msgs ...chat.Message) error {
	k := histKey(userID)

	pipe := s.rdb.Pipeline()
	for _, m := range msgs {
		b, err := json.Marshal(m)
		if err != nil {
			return err
		}
		pipe.RPush(ctx, k, string(b))
	}
	pipe.Expire(ctx, k, s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	return trimKey(ctx, s.rdb, k)
}

// Clear deletes the user's chat history. Idempotent — a missing key is a no-op.
func (s *Store) Clear(ctx context.Context, userID int64) error {
	return s.rdb.Del(ctx, histKey(userID)).Err()
}
