package chathistory

import (
	"context"
	"log/slog"

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
// history so a Redis outage never breaks the chat endpoint. A nil store
// yields a non-persistent, in-memory handle.
func NewRedisHandle(ctx context.Context, store *Store, userID int64) *RedisHandle {
	h := &RedisHandle{store: store, userID: userID}
	if store == nil {
		return h
	}
	msgs, err := store.Get(ctx, userID)
	if err != nil {
		slog.Warn("chathistory: failed to load history, starting fresh",
			slog.Int64("userID", userID), slog.Any("error", err))
	}
	h.cached = msgs
	return h
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
