package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	oas "github.com/tamcore/motus/internal/api/oas"
)

// positionQueryTimeout caps the server-side wall time for a single
// position range query. Unbounded queries (no limit) can be large; this
// prevents a slow or stalled client from holding a DB connection open
// indefinitely. WriteTimeout is intentionally 0 (WebSocket compat), so
// this is the only ceiling on this path.
const positionQueryTimeout = 120 * time.Second

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

	// Mode 2: No deviceId.
	//   - Without from/to: return latest position for each of the user's
	//     devices (Traccar-compatible default).
	//   - With from/to:    return positions for every user's device within
	//     the time range. Used by the dashboard "positions today" tile.
	if deviceIDStr == "" {
		fromStr := query.Get("from")
		toStr := query.Get("to")
		if fromStr != "" || toStr != "" {
			from := time.Now().Add(-24 * time.Hour)
			to := time.Now()
			if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
				from = t
			}
			if t, err := time.Parse(time.RFC3339, toStr); err == nil {
				to = t
			}
			limit := 0
			if v := query.Get("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					limit = n
				}
			}
			queryCtx, cancel := context.WithTimeout(r.Context(), positionQueryTimeout)
			defer cancel()
			writeStreamedPositions(w, func(emit func(*model.Position)) error {
				return h.positions.StreamByUserAndTimeRange(queryCtx, user.ID, from, to, limit, func(p *model.Position) error {
					emit(positionInKnots(p))
					return nil
				})
			})
			return
		}

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

	queryCtx, cancel := context.WithTimeout(r.Context(), positionQueryTimeout)
	defer cancel()

	writeStreamedPositions(w, func(emit func(*model.Position)) error {
		return h.positions.StreamByDeviceAndTimeRange(queryCtx, deviceID, from, to, limit, func(p *model.Position) error {
			emit(positionInKnots(p))
			return nil
		})
	})
}

// writeStreamedPositions writes a JSON array to w by invoking populate, which
// must call emit once per position. The response header is committed before
// populate runs; errors mid-stream are logged but cannot change the HTTP
// status (already 200 OK at that point).
func writeStreamedPositions(w http.ResponseWriter, populate func(emit func(*model.Position)) error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	first := true
	_, _ = io.WriteString(w, "[")
	err := populate(func(p *model.Position) {
		if !first {
			_, _ = io.WriteString(w, ",")
		}
		first = false
		_ = enc.Encode(p)
	})
	_, _ = io.WriteString(w, "]")
	if err != nil {
		slog.Error("position stream failed mid-response", slog.Any("error", err))
	}
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

// AdminGetAllPositions returns positions across every device (admin only).
//   - Without from/to: latest position per device (Traccar-compatible default).
//   - With from/to:    every position within the window. Used by the
//     dashboard's admin "show all" path for the "positions today" tile.
//
// GET /api/admin/positions
func (h *PositionHandler) AdminGetAllPositions(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil || !user.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "admin access required")
		return
	}

	query := r.URL.Query()
	fromStr := query.Get("from")
	toStr := query.Get("to")
	if fromStr != "" || toStr != "" {
		from := time.Now().Add(-24 * time.Hour)
		to := time.Now()
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
		limit := 0
		if v := query.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		queryCtx, cancel := context.WithTimeout(r.Context(), positionQueryTimeout)
		defer cancel()
		writeStreamedPositions(w, func(emit func(*model.Position)) error {
			return h.positions.StreamAllByTimeRange(queryCtx, from, to, limit, func(p *model.Position) error {
				emit(positionInKnots(p))
				return nil
			})
		})
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

// --- ogen Handler methods ---

// GetPositions implements oas.Handler for GET /api/positions.
// Supports two modes based on the OAS params (DeviceId, From, To):
//   - No DeviceId: latest per user device; or time-range for all user devices when From/To set
//   - With DeviceId: time range for that specific device (defaults to last 24 h)
func (h *Handler) GetPositions(ctx context.Context, params oas.GetPositionsParams) (oas.GetPositionsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}

	deviceID, hasDevice := params.DeviceId.Get()
	from, hasFrom := params.From.Get()
	to, hasTo := params.To.Get()

	// No deviceId, no time range: latest position per user device.
	if !hasDevice && !hasFrom && !hasTo {
		positions, err := h.cfg.Positions.GetLatestByUser(ctx, user.ID)
		if err != nil {
			slog.Error("GetLatestByUser failed", slog.Int64("userID", user.ID), slog.Any("error", err))
			return &oas.Error{Error: "failed to get positions"}, nil
		}
		result := make(oas.GetPositionsOKApplicationJSON, len(positions))
		for i, p := range positions {
			result[i] = positionToOAS(positionInKnots(p))
		}
		return &result, nil
	}

	// No deviceId, with time range: stream all user devices in window.
	if !hasDevice {
		if !hasFrom {
			from = time.Now().Add(-24 * time.Hour)
		}
		if !hasTo {
			to = time.Now()
		}
		queryCtx, cancel := context.WithTimeout(ctx, positionQueryTimeout)
		defer cancel()
		var result oas.GetPositionsOKApplicationJSON
		streamErr := h.cfg.Positions.StreamByUserAndTimeRange(queryCtx, user.ID, from, to, 0, func(p *model.Position) error {
			result = append(result, positionToOAS(positionInKnots(p)))
			return nil
		})
		if streamErr != nil {
			slog.Error("StreamByUserAndTimeRange failed", slog.Any("error", streamErr))
			return &oas.Error{Error: "failed to get positions"}, nil
		}
		if result == nil {
			result = oas.GetPositionsOKApplicationJSON{}
		}
		return &result, nil
	}

	// With deviceId: verify access, then stream time range.
	if !h.cfg.Devices.UserHasAccess(ctx, user, deviceID) {
		return &oas.Error{Error: "access denied"}, nil
	}
	if !hasFrom {
		from = time.Now().Add(-24 * time.Hour)
	}
	if !hasTo {
		to = time.Now()
	}
	queryCtx, cancel := context.WithTimeout(ctx, positionQueryTimeout)
	defer cancel()
	var result oas.GetPositionsOKApplicationJSON
	streamErr := h.cfg.Positions.StreamByDeviceAndTimeRange(queryCtx, deviceID, from, to, 0, func(p *model.Position) error {
		result = append(result, positionToOAS(positionInKnots(p)))
		return nil
	})
	if streamErr != nil {
		slog.Error("StreamByDeviceAndTimeRange failed", slog.Any("error", streamErr))
		return &oas.Error{Error: "failed to get positions"}, nil
	}
	if result == nil {
		result = oas.GetPositionsOKApplicationJSON{}
	}
	return &result, nil
}
