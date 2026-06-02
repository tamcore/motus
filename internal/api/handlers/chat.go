package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tamcore/motus/internal/ai/chat"
	"github.com/tamcore/motus/internal/api"
)

// NewChatHandler returns an http.Handler for POST /api/chat.
func NewChatHandler(svc *chat.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := api.UserFromContext(r.Context())
		if user == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var body struct {
			Messages []chat.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		sink := &sseSink{w: w, rc: http.NewResponseController(w)}
		_ = svc.Stream(r.Context(), body.Messages, sink)
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
