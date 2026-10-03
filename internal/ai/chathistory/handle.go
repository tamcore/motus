package chathistory

import (
	"context"
	"log/slog"
	"slices"

	"github.com/tamcore/motus/internal/ai/chat"
)

// RedisHandle implements chat.HistoryHandle for a single user's conversation
// stored in Redis. It is built once per HTTP request.
type RedisHandle struct {
	store  *Store
	userID int64
	cached []chat.Message // loaded once at request start
}

// NewRedisHandle loads the current history for userID and returns a handle
// ready for use. Errors loading from Redis are logged and treated as empty
// history so a Redis outage never breaks the chat endpoint.
func NewRedisHandle(ctx context.Context, store *Store, userID int64) *RedisHandle {
	msgs, err := store.Get(ctx, userID)
	if err != nil {
		slog.Warn("chathistory: failed to load history, starting fresh",
			slog.Int64("userID", userID), slog.Any("error", err))
		msgs = nil
	}
	return &RedisHandle{store: store, userID: userID, cached: msgs}
}

// Messages returns the conversation messages (not including the system prompt).
func (h *RedisHandle) Messages() []chat.Message {
	return h.cached
}

// Append persists msgs to Redis (when a store is set) and updates the local
// cache. On Redis error it logs and continues so the current turn still
// succeeds in-memory. It always returns nil.
func (h *RedisHandle) Append(ctx context.Context, msgs ...chat.Message) error {
	if h.store != nil {
		if err := h.store.Append(ctx, h.userID, msgs...); err != nil {
			slog.Warn("chathistory: failed to persist messages",
				slog.Int64("userID", h.userID), slog.Any("error", err))
		}
	}
	h.cached = append(h.cached, msgs...)
	return nil
}

// NewMemHandle returns a non-persistent handle pre-populated with msgs, used
// when Redis is unavailable.
func NewMemHandle(msgs ...chat.Message) *RedisHandle {
	return &RedisHandle{cached: slices.Clone(msgs)}
}
