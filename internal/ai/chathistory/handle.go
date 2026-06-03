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

// Append persists msgs to Redis and updates the local cache. On Redis error
// it logs and continues so the current turn still succeeds in-memory.
func (h *RedisHandle) Append(ctx context.Context, msgs ...chat.Message) error {
	if err := h.store.Append(ctx, h.userID, msgs...); err != nil {
		slog.Warn("chathistory: failed to persist messages",
			slog.Int64("userID", h.userID), slog.Any("error", err))
		// Fall through — update cache even if Redis write failed.
	}
	h.cached = append(h.cached, msgs...)
	return nil
}

// MemHandle is a non-persistent in-memory fallback used when Redis is
// unavailable. It provides the same interface but never writes to external storage.
type MemHandle struct {
	cached []chat.Message
}

// NewMemHandle creates a MemHandle pre-populated with the given messages.
func NewMemHandle(msgs ...chat.Message) *MemHandle {
	c := make([]chat.Message, len(msgs))
	copy(c, msgs)
	return &MemHandle{cached: c}
}

func (h *MemHandle) Messages() []chat.Message { return h.cached }

func (h *MemHandle) Append(_ context.Context, msgs ...chat.Message) error {
	h.cached = append(h.cached, msgs...)
	return nil
}
