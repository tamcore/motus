package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/tamcore/motus/internal/ai/chat"
	"github.com/tamcore/motus/internal/ai/chathistory"
	"github.com/tamcore/motus/internal/api"
)

// NewChatHistoryHandler returns an http.Handler for GET and DELETE
// /api/chat/history; the router registers it for those two methods only.
func NewChatHistoryHandler(store *chathistory.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := api.UserFromContext(r.Context())
		if user == nil {
			api.RespondError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if r.Method == http.MethodDelete {
			serveChatHistoryDelete(w, r, store, user.ID)
			return
		}
		serveChatHistoryGet(w, r, store, user.ID)
	})
}

func serveChatHistoryGet(w http.ResponseWriter, r *http.Request, store *chathistory.Store, userID int64) {
	var msgs []chat.Message
	if store != nil {
		var err error
		msgs, err = store.Get(r.Context(), userID)
		if err != nil {
			api.RespondError(w, http.StatusInternalServerError, "failed to load history")
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
			api.RespondError(w, http.StatusInternalServerError, "failed to clear history")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
