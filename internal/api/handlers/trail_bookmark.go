package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/validation"
)

// Trail bookmarks belong to the user who created them. Listing returns only
// the caller's own bookmarks (also for admins). Updating and deleting by ID
// is allowed for the owner and for admins; updates additionally require
// access to the bookmarked device, so revoking a device assignment also hides
// its bookmarks, while deleting only needs ownership so owners can clean up.
// Access failures answer 404 so the existence of other users' bookmarks is
// not leaked. Storage failures are returned as handler errors (HTTP 500)
// with a generic message. Readonly API keys are rejected for writes by the
// RequireWriteAccess middleware.

const bookmarkNotFound = "trail bookmark not found"

// Length limits in characters (Unicode code points), matching the web UI.
const (
	trailBookmarkNameMaxChars        = 200
	trailBookmarkDescriptionMaxChars = 2000
)

// validateTrailBookmarkInput normalises and validates a bookmark payload,
// returning the trimmed name and description.
func validateTrailBookmarkInput(req *oas.TrailBookmarkInput) (name, description string, err error) {
	name = strings.TrimSpace(req.Name)
	if name == "" {
		return "", "", errors.New("name is required")
	}
	if err := validation.ValidateText(name, trailBookmarkNameMaxChars); err != nil {
		return "", "", fmt.Errorf("invalid name: %w", err)
	}
	description = strings.TrimSpace(req.Description.Or(""))
	if err := validation.ValidateText(description, trailBookmarkDescriptionMaxChars); err != nil {
		return "", "", fmt.Errorf("invalid description: %w", err)
	}
	if req.From.IsZero() || req.To.IsZero() {
		return "", "", errors.New("from and to are required")
	}
	if !req.To.After(req.From) {
		return "", "", errors.New("to must be after from")
	}
	return name, description, nil
}

// ownBookmark loads a bookmark the user owns (admins: any). It returns
// (nil, nil) when the bookmark does not exist or belongs to someone else, and
// an error only for storage failures.
func (h *Handler) ownBookmark(ctx context.Context, user *model.User, id int64) (*model.TrailBookmark, error) {
	b, err := h.cfg.TrailBookmarks.GetByID(ctx, id)
	if errors.Is(err, repository.ErrTrailBookmarkNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, bookmarkStorageError("load", err)
	}
	if b.UserID != user.ID && !user.IsAdmin() {
		return nil, nil
	}
	return b, nil
}

// bookmarkStorageError logs a storage failure and returns a safe error for
// the client (ogen's NewError turns it into HTTP 500).
func bookmarkStorageError(op string, err error) error {
	slog.Error("trail bookmark storage failure", slog.String("op", op), slog.Any("error", err))
	return fmt.Errorf("failed to %s trail bookmark", op)
}

// fillDeviceName sets DeviceName from the device repo (best-effort).
func (h *Handler) fillDeviceName(ctx context.Context, b *model.TrailBookmark) {
	if d, err := h.cfg.Devices.GetByID(ctx, b.DeviceID); err == nil && d != nil {
		b.DeviceName = d.Name
	}
}

// ListTrailBookmarks returns the caller's bookmarks, optionally for one device.
func (h *Handler) ListTrailBookmarks(ctx context.Context, params oas.ListTrailBookmarksParams) (oas.ListTrailBookmarksRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}
	var deviceID *int64
	if v, ok := params.DeviceId.Get(); ok {
		deviceID = &v
	}
	bookmarks, err := h.cfg.TrailBookmarks.ListForUser(ctx, user, deviceID)
	if err != nil {
		return nil, bookmarkStorageError("list", err)
	}
	result := make(oas.ListTrailBookmarksOKApplicationJSON, len(bookmarks))
	for i, b := range bookmarks {
		result[i] = trailBookmarkToOAS(b)
	}
	return &result, nil
}

// CreateTrailBookmark saves a device trail range for the caller.
func (h *Handler) CreateTrailBookmark(ctx context.Context, req *oas.TrailBookmarkInput) (oas.CreateTrailBookmarkRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.CreateTrailBookmarkUnauthorized{Error: "unauthorized"}, nil
	}
	name, description, err := validateTrailBookmarkInput(req)
	if err != nil {
		return &oas.CreateTrailBookmarkBadRequest{Error: err.Error()}, nil
	}
	if !h.cfg.Devices.UserHasAccess(ctx, user, req.DeviceId) {
		return &oas.CreateTrailBookmarkNotFound{Error: "device not found"}, nil
	}

	b := &model.TrailBookmark{
		UserID:      user.ID,
		DeviceID:    req.DeviceId,
		Name:        name,
		Description: description,
		From:        req.From,
		To:          req.To,
	}
	if err := h.cfg.TrailBookmarks.Create(ctx, b); err != nil {
		return nil, bookmarkStorageError("create", err)
	}
	h.fillDeviceName(ctx, b)

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionTrailBookmarkCreate, audit.ResourceTrailBookmark, &b.ID,
			map[string]any{"name": b.Name, "deviceId": b.DeviceID}, "", "")
	}
	out := trailBookmarkToOAS(b)
	return &out, nil
}

// UpdateTrailBookmark replaces a bookmark's device, name, description and range.
func (h *Handler) UpdateTrailBookmark(ctx context.Context, req *oas.TrailBookmarkInput, params oas.UpdateTrailBookmarkParams) (oas.UpdateTrailBookmarkRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.UpdateTrailBookmarkUnauthorized{Error: "unauthorized"}, nil
	}
	existing, err := h.ownBookmark(ctx, user, params.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil || !h.cfg.Devices.UserHasAccess(ctx, user, existing.DeviceID) {
		return &oas.UpdateTrailBookmarkNotFound{Error: bookmarkNotFound}, nil
	}
	name, description, err := validateTrailBookmarkInput(req)
	if err != nil {
		return &oas.UpdateTrailBookmarkBadRequest{Error: err.Error()}, nil
	}
	if req.DeviceId != existing.DeviceID && !h.cfg.Devices.UserHasAccess(ctx, user, req.DeviceId) {
		return &oas.UpdateTrailBookmarkNotFound{Error: "device not found"}, nil
	}

	updated := *existing
	updated.DeviceID = req.DeviceId
	updated.Name = name
	updated.Description = description
	updated.From = req.From
	updated.To = req.To
	if err := h.cfg.TrailBookmarks.Update(ctx, &updated); err != nil {
		return nil, bookmarkStorageError("update", err)
	}
	if updated.DeviceID != existing.DeviceID {
		h.fillDeviceName(ctx, &updated)
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionTrailBookmarkUpdate, audit.ResourceTrailBookmark, &updated.ID,
			map[string]any{"name": updated.Name, "deviceId": updated.DeviceID}, "", "")
	}
	out := trailBookmarkToOAS(&updated)
	return &out, nil
}

// DeleteTrailBookmark removes a bookmark. Only ownership (or admin) is
// required, so owners can clean up bookmarks of devices they lost access to.
func (h *Handler) DeleteTrailBookmark(ctx context.Context, params oas.DeleteTrailBookmarkParams) (oas.DeleteTrailBookmarkRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.DeleteTrailBookmarkUnauthorized{Error: "unauthorized"}, nil
	}
	existing, err := h.ownBookmark(ctx, user, params.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return &oas.DeleteTrailBookmarkNotFound{Error: bookmarkNotFound}, nil
	}
	if err := h.cfg.TrailBookmarks.Delete(ctx, params.ID); err != nil {
		return nil, bookmarkStorageError("delete", err)
	}
	if h.cfg.AuditLogger != nil {
		id := params.ID
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionTrailBookmarkDelete, audit.ResourceTrailBookmark, &id,
			nil, "", "")
	}
	return &oas.DeleteTrailBookmarkNoContent{}, nil
}

func trailBookmarkToOAS(b *model.TrailBookmark) oas.TrailBookmark {
	return oas.TrailBookmark{
		ID:          b.ID,
		UserId:      oas.NewOptInt64(b.UserID),
		DeviceId:    b.DeviceID,
		DeviceName:  optStr(b.DeviceName),
		Name:        b.Name,
		Description: b.Description,
		From:        b.From,
		To:          b.To,
		CreatedAt:   b.CreatedAt,
		UpdatedAt:   b.UpdatedAt,
	}
}
