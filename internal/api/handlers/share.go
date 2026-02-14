package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// ShareHandler handles device sharing endpoints.
type ShareHandler struct {
	shares         repository.DeviceShareRepo
	devices        repository.DeviceRepo
	positions      repository.PositionRepo
	audit          *audit.Logger
	uniqueIDPrefix string
}

// NewShareHandler creates a new share handler.
func NewShareHandler(
	shares repository.DeviceShareRepo,
	devices repository.DeviceRepo,
	positions repository.PositionRepo,
	uniqueIDPrefix string,
) *ShareHandler {
	return &ShareHandler{shares: shares, devices: devices, positions: positions, uniqueIDPrefix: uniqueIDPrefix}
}

// SetAuditLogger configures audit logging for share events.
func (h *ShareHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

type createShareRequest struct {
	ExpiresInHours *int `json:"expiresInHours,omitempty"`
}

type createShareResponse struct {
	*model.DeviceShare
	ShareURL string `json:"shareUrl"`
}

// CreateShare creates a new shareable link for a device.
// POST /api/devices/{id}/share
func (h *ShareHandler) CreateShare(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	deviceID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid device id")
		return
	}

	// Verify user has access to the device.
	if !h.devices.UserHasAccess(r.Context(), user, deviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	var req createShareRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	share := &model.DeviceShare{
		DeviceID:  deviceID,
		CreatedBy: user.ID,
	}

	// Set expiry if specified.
	if req.ExpiresInHours != nil && *req.ExpiresInHours > 0 {
		expires := time.Now().Add(time.Duration(*req.ExpiresInHours) * time.Hour)
		share.ExpiresAt = &expires
	}

	if err := h.shares.Create(r.Context(), share); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create share link")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionShareCreate, audit.ResourceShare, &share.ID,
			map[string]interface{}{"deviceId": deviceID})
	}

	resp := createShareResponse{
		DeviceShare: share,
		ShareURL:    "/share/" + share.Token,
	}
	api.RespondJSON(w, http.StatusCreated, resp)
}

// ListShares returns all active share links for a device.
// GET /api/devices/{id}/shares
func (h *ShareHandler) ListShares(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	deviceID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid device id")
		return
	}

	if !h.devices.UserHasAccess(r.Context(), user, deviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	shares, err := h.shares.ListByDevice(r.Context(), deviceID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list shares")
		return
	}
	if shares == nil {
		shares = []*model.DeviceShare{}
	}
	api.RespondJSON(w, http.StatusOK, shares)
}

// DeleteShare removes a share link.
// DELETE /api/shares/{id}
func (h *ShareHandler) DeleteShare(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	shareID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid share id")
		return
	}

	// Look up the share to verify it exists and get the device ID.
	share, err := h.shares.GetByID(r.Context(), shareID)
	if err != nil || share == nil {
		api.RespondError(w, http.StatusNotFound, "share not found")
		return
	}

	// Verify the user has access to the device this share belongs to.
	if !h.devices.UserHasAccess(r.Context(), user, share.DeviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	if err := h.shares.Delete(r.Context(), shareID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to delete share")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionShareDelete, audit.ResourceShare, &shareID,
			map[string]interface{}{"deviceId": share.DeviceID})
	}

	w.WriteHeader(http.StatusNoContent)
}

// sharedDeviceResponse wraps device info and latest positions for public access.
type sharedDeviceResponse struct {
	Device    *model.Device     `json:"device"`
	Positions []*model.Position `json:"positions"`
}

// GetSharedDevice returns device info and latest position for a public share link.
// GET /api/share/{token}
func (h *ShareHandler) GetSharedDevice(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		api.RespondError(w, http.StatusBadRequest, "missing share token")
		return
	}

	share, err := h.shares.GetByToken(r.Context(), token)
	if err != nil || share == nil {
		api.RespondError(w, http.StatusNotFound, "share link not found or expired")
		return
	}

	device, err := h.devices.GetByID(r.Context(), share.DeviceID)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "device not found")
		return
	}

	// Get latest position for the shared device.
	position, err := h.positions.GetLatestByDevice(r.Context(), share.DeviceID)
	var positions []*model.Position
	if err == nil && position != nil {
		positions = []*model.Position{position}
	}

	resp := sharedDeviceResponse{
		Device:    device,
		Positions: positions,
	}
	model.ApplyUniqueIDPrefix([]*model.Device{device}, effectivePrefix(r, h.uniqueIDPrefix))
	api.RespondJSON(w, http.StatusOK, resp)
}
