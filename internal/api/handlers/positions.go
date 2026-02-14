package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// PositionHandler handles position-related API endpoints.
type PositionHandler struct {
	positions repository.PositionRepo
	devices   repository.DeviceRepo
}

// NewPositionHandler creates a new position handler.
func NewPositionHandler(positions repository.PositionRepo, devices repository.DeviceRepo) *PositionHandler {
	return &PositionHandler{positions: positions, devices: devices}
}

// GetPositions returns positions filtered by query parameters.
// Traccar-compatible endpoint supporting three modes:
//   - GET /api/positions                                    -> latest position per user device
//   - GET /api/positions?id=31&id=42                        -> specific positions by ID
//   - GET /api/positions?deviceId=123&from=ISO8601&to=ISO8601 -> position history for device
//
// The id parameter takes precedence over deviceId when both are present.
func (h *PositionHandler) GetPositions(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	query := r.URL.Query()

	// Mode 1: Fetch specific positions by ID (Traccar: ?id=31&id=42).
	if idValues := query["id"]; len(idValues) > 0 {
		h.getPositionsByIDs(w, r, user, idValues)
		return
	}

	deviceIDStr := query.Get("deviceId")

	// Mode 2: No deviceId — return latest position for all user's devices.
	if deviceIDStr == "" {
		positions, err := h.positions.GetLatestByUser(r.Context(), user.ID)
		if err != nil {
			slog.Error("GetLatestByUser failed",
				slog.Int64("userID", user.ID),
				slog.Any("error", err),
			)
			api.RespondError(w, http.StatusInternalServerError, "failed to get positions")
			return
		}
		if positions == nil {
			api.RespondJSON(w, http.StatusOK, []struct{}{})
			return
		}
		api.RespondJSON(w, http.StatusOK, positionsInKnots(positions))
		return
	}

	// Mode 3: With deviceId — return time range for specific device.
	deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid deviceId")
		return
	}

	// Verify user has access to this device.
	if !h.devices.UserHasAccess(r.Context(), user, deviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	// Default limit is 0, which means "use the repository maximum".
	// The Traccar API does not support a limit parameter and returns all
	// positions in the requested time range. We honour this behaviour so
	// that Home Assistant and Traccar Manager get a complete trail instead
	// of a truncated one (which renders as a straight line on the map).
	limit := 0

	if v := query.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := query.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	if v := query.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	positions, err := h.positions.GetByDeviceAndTimeRange(r.Context(), deviceID, from, to, limit)
	if err != nil {
		slog.Error("GetByDeviceAndTimeRange failed",
			slog.Int64("deviceID", deviceID),
			slog.Any("error", err),
		)
		api.RespondError(w, http.StatusInternalServerError, "failed to get positions")
		return
	}
	api.RespondJSON(w, http.StatusOK, positionsInKnots(positions))
}

// kmhToKnots converts a speed value from km/h to knots.
// Traccar's REST API contract specifies speed in knots; internal storage uses km/h.
const kmhToKnotsRatio = 1.0 / 1.852

func positionInKnots(p *model.Position) *model.Position {
	if p.Speed == nil {
		return p
	}
	cp := *p
	knots := *p.Speed * kmhToKnotsRatio
	cp.Speed = &knots
	return &cp
}

func positionsInKnots(positions []*model.Position) []*model.Position {
	result := make([]*model.Position, len(positions))
	for i, p := range positions {
		result[i] = positionInKnots(p)
	}
	return result
}

// getPositionsByIDs handles the ?id=X&id=Y mode.
// Each returned position is verified against user device access.
func (h *PositionHandler) getPositionsByIDs(w http.ResponseWriter, r *http.Request, user *model.User, idValues []string) {
	ids := make([]int64, 0, len(idValues))
	for _, v := range idValues {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid id parameter")
			return
		}
		ids = append(ids, id)
	}

	positions, err := h.positions.GetByIDs(r.Context(), ids)
	if err != nil {
		slog.Error("GetByIDs failed", slog.Any("error", err))
		api.RespondError(w, http.StatusInternalServerError, "failed to get positions")
		return
	}

	// Filter to only positions the user has access to.
	allowed := make([]*model.Position, 0, len(positions))
	for _, p := range positions {
		if h.devices.UserHasAccess(r.Context(), user, p.DeviceID) {
			allowed = append(allowed, p)
		}
	}

	api.RespondJSON(w, http.StatusOK, positionsInKnots(allowed))
}

// AdminGetAllPositions returns the latest position for every device (admin only).
// GET /api/admin/positions
func (h *PositionHandler) AdminGetAllPositions(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil || !user.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "admin access required")
		return
	}

	positions, err := h.positions.GetLatestAll(r.Context())
	if err != nil {
		slog.Error("GetLatestAll failed", slog.Any("error", err))
		api.RespondError(w, http.StatusInternalServerError, "failed to get positions")
		return
	}
	if positions == nil {
		api.RespondJSON(w, http.StatusOK, []struct{}{})
		return
	}
	api.RespondJSON(w, http.StatusOK, positionsInKnots(positions))
}
