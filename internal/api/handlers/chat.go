package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tamcore/motus/internal/ai/chat"
	"github.com/tamcore/motus/internal/ai/chathistory"
	"github.com/tamcore/motus/internal/api"
)

// ChatHandler handles POST /api/chat (SSE streaming).
type ChatHandler struct {
	svc       *chat.Service
	histStore *chathistory.Store // nil when Redis is unavailable
}

// NewChatHandler returns an http.Handler for POST /api/chat.
// histStore may be nil; when nil the handler falls back to single-turn
// in-memory behaviour.
func NewChatHandler(svc *chat.Service, histStore *chathistory.Store) http.Handler {
	h := &ChatHandler{svc: svc, histStore: histStore}
	return http.HandlerFunc(h.serve)
}

func (h *ChatHandler) serve(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var body struct {
		Message chat.Message `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if body.Message.Role != "user" || body.Message.Content == "" {
		http.Error(w, `{"error":"message must be a non-empty user message"}`, http.StatusBadRequest)
		return
	}

	// Build the history handle — Redis-backed when available, in-memory otherwise.
	var hist chat.HistoryHandle
	if h.histStore != nil {
		rh := chathistory.NewRedisHandle(r.Context(), h.histStore, user.ID)
		_ = rh.Append(r.Context(), body.Message)
		hist = rh
	} else {
		hist = chathistory.NewMemHandle(body.Message)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sink := &sseSink{w: w, rc: http.NewResponseController(w)}
	_ = h.svc.Stream(r.Context(), hist, sink)
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
