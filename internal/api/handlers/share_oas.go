package handlers

import (
	"context"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
)

// ListShares returns all active share links for a device.
// GET /api/devices/{id}/shares
func (h *Handler) ListShares(ctx context.Context, params oas.ListSharesParams) (oas.ListSharesRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.ListSharesUnauthorized{Error: "not authenticated"}, nil
	}

	if !h.cfg.Devices.UserHasAccess(ctx, user, params.ID) {
		return &oas.ListSharesForbidden{Error: "access denied"}, nil
	}

	shares, err := h.cfg.Shares.ListByDevice(ctx, params.ID)
	if err != nil {
		return &oas.ListSharesUnauthorized{Error: "failed to list shares"}, nil
	}

	result := make(oas.ListSharesOKApplicationJSON, 0, len(shares))
	for _, s := range shares {
		result = append(result, deviceShareToOAS(s))
	}
	return &result, nil
}

// CreateShare creates a new shareable link for a device.
// POST /api/devices/{id}/share
func (h *Handler) CreateShare(ctx context.Context, req oas.OptCreateShareRequest, params oas.CreateShareParams) (oas.CreateShareRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.CreateShareUnauthorized{Error: "not authenticated"}, nil
	}

	if !h.cfg.Devices.UserHasAccess(ctx, user, params.ID) {
		return &oas.CreateShareForbidden{Error: "access denied"}, nil
	}

	share := &model.DeviceShare{
		DeviceID:  params.ID,
		CreatedBy: user.ID,
	}

	if body, ok := req.Get(); ok {
		if expiresAt, ok := body.ExpiresAt.Get(); ok {
			t := time.Time(expiresAt)
			share.ExpiresAt = &t
		}
	}

	if err := h.cfg.Shares.Create(ctx, share); err != nil {
		return &oas.CreateShareForbidden{Error: "failed to create share link"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionShareCreate, audit.ResourceShare, &share.ID,
			map[string]any{"deviceId": params.ID}, "", "")
	}

	result := deviceShareToOAS(share)
	return &result, nil
}

// DeleteShare removes a share link.
// DELETE /api/shares/{id}
func (h *Handler) DeleteShare(ctx context.Context, params oas.DeleteShareParams) (oas.DeleteShareRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.DeleteShareUnauthorized{Error: "not authenticated"}, nil
	}

	share, err := h.cfg.Shares.GetByID(ctx, params.ID)
	if err != nil || share == nil {
		return &oas.DeleteShareNotFound{Error: "share not found"}, nil
	}

	if !h.cfg.Devices.UserHasAccess(ctx, user, share.DeviceID) {
		return &oas.DeleteShareForbidden{Error: "access denied"}, nil
	}

	if err := h.cfg.Shares.Delete(ctx, params.ID); err != nil {
		return &oas.DeleteShareForbidden{Error: "failed to delete share"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionShareDelete, audit.ResourceShare, &params.ID,
			map[string]any{"deviceId": share.DeviceID}, "", "")
	}

	return &oas.DeleteShareNoContent{}, nil
}

// GetSharedDevice returns device info and latest position for a public share link.
// GET /api/share/{token}
func (h *Handler) GetSharedDevice(ctx context.Context, params oas.GetSharedDeviceParams) (oas.GetSharedDeviceRes, error) {
	share, err := h.cfg.Shares.GetByToken(ctx, params.Token)
	if err != nil || share == nil {
		return &oas.Error{Error: "share link not found or expired"}, nil
	}

	device, err := h.cfg.Devices.GetByID(ctx, share.DeviceID)
	if err != nil {
		return &oas.Error{Error: "device not found"}, nil
	}

	prefix := effectivePrefixCtx(ctx, h.cfg.UniqueIDPrefix)
	model.ApplyUniqueIDPrefix([]*model.Device{device}, prefix)

	result := deviceToOAS(device)
	return &result, nil
}
