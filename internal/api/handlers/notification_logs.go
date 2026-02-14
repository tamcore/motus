package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/model"
)

// Logs returns recent delivery logs for a notification rule.
// GET /api/notifications/{id}/logs
func (h *NotificationHandler) Logs(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid notification rule id")
		return
	}

	// Verify the rule belongs to the authenticated user.
	rule, err := h.notifications.GetByID(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "notification rule not found")
		return
	}
	if rule.UserID != user.ID && !user.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	logs, err := h.notifications.GetLogsByRule(r.Context(), id, 50)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to get notification logs")
		return
	}
	if logs == nil {
		logs = []*model.NotificationLog{}
	}
	api.RespondJSON(w, http.StatusOK, logs)
}
