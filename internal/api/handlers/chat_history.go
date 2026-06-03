package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/tamcore/motus/internal/ai/chat"
	"github.com/tamcore/motus/internal/ai/chathistory"
	"github.com/tamcore/motus/internal/api"
)

// NewChatHistoryHandler returns an http.Handler for GET and DELETE
// /api/chat/history. It dispatches by method internally.
func NewChatHistoryHandler(store *chathistory.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := api.UserFromContext(r.Context())
		if user == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			serveChatHistoryGet(w, r, store, user.ID)
		case http.MethodDelete:
			serveChatHistoryDelete(w, r, store, user.ID)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}

func serveChatHistoryGet(w http.ResponseWriter, r *http.Request, store *chathistory.Store, userID int64) {
	var msgs []chat.Message
	if store != nil {
		var err error
		msgs, err = store.Get(r.Context(), userID)
		if err != nil {
			http.Error(w, `{"error":"failed to load history"}`, http.StatusInternalServerError)
			return
		}
	}
	if msgs == nil {
		msgs = []chat.Message{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"messages": msgs})
}

func serveChatHistoryDelete(w http.ResponseWriter, r *http.Request, store *chathistory.Store, userID int64) {
	if store != nil {
		if err := store.Clear(r.Context(), userID); err != nil {
			http.Error(w, `{"error":"failed to clear history"}`, http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
