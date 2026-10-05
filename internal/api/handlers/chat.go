package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tamcore/motus/internal/ai/chat"
	"github.com/tamcore/motus/internal/ai/chathistory"
	"github.com/tamcore/motus/internal/api"
)

// NewChatHandler returns an http.Handler for POST /api/chat (SSE streaming).
// histStore may be nil; when nil the handler falls back to single-turn
// in-memory behaviour.
func NewChatHandler(svc *chat.Service, histStore *chathistory.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := api.UserFromContext(r.Context())
		if user == nil {
			api.RespondError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var body struct {
			Message chat.Message `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if body.Message.Role != "user" || body.Message.Content == "" {
			api.RespondError(w, http.StatusBadRequest, "message must be a non-empty user message")
			return
		}

		hist := chathistory.NewRedisHandle(r.Context(), histStore, user.ID)
		_ = hist.Append(r.Context(), body.Message)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		sink := &sseSink{w: w, rc: http.NewResponseController(w)}
		_ = svc.Stream(r.Context(), hist, sink)
	})
}

type sseSink struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (s *sseSink) Send(event chat.ChatEvent) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.w, "data: %s\n\n", b)
	return err
}

func (s *sseSink) Flush() error {
	return s.rc.Flush()
}
