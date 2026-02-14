package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/storage/repository"
)

// StatisticsHandler handles admin statistics endpoints.
type StatisticsHandler struct {
	stats repository.StatisticsRepo
}

// NewStatisticsHandler creates a new statistics handler.
func NewStatisticsHandler(stats repository.StatisticsRepo) *StatisticsHandler {
	return &StatisticsHandler{stats: stats}
}

// GetPlatformStats returns platform-wide statistics.
// GET /api/admin/statistics
func (h *StatisticsHandler) GetPlatformStats(w http.ResponseWriter, r *http.Request) {
	admin := api.UserFromContext(r.Context())
	if admin == nil || !admin.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "admin access required")
		return
	}

	stats, err := h.stats.GetPlatformStats(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to get statistics")
		return
	}

	api.RespondJSON(w, http.StatusOK, stats)
}

// GetUserStats returns statistics for a specific user.
// GET /api/admin/statistics/users/{id}
func (h *StatisticsHandler) GetUserStats(w http.ResponseWriter, r *http.Request) {
	admin := api.UserFromContext(r.Context())
	if admin == nil || !admin.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "admin access required")
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	stats, err := h.stats.GetUserStats(r.Context(), userID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to get user statistics")
		return
	}

	api.RespondJSON(w, http.StatusOK, stats)
}
