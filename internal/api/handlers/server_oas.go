package handlers

import (
	"context"
	"net/http"

	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/version"
)

// NewError maps a handler error to an HTTP error response.
// Uses the error's Code() method when available (e.g. 401 for SecurityError, 400 for DecodeRequestError).
func (h *Handler) NewError(_ context.Context, err error) *oas.UnexpectedErrorStatusCode {
	code := http.StatusInternalServerError
	type coder interface{ Code() int }
	if c, ok := err.(coder); ok {
		code = c.Code()
	}
	return &oas.UnexpectedErrorStatusCode{
		StatusCode: code,
		Response:   oas.Error{Error: err.Error()},
	}
}

// GetHealth returns the service health status.
// GET /api/health
func (h *Handler) GetHealth(ctx context.Context) (*oas.GetHealthOK, error) {
	return &oas.GetHealthOK{Status: "ok"}, nil
}

// GetServer returns Traccar-compatible server information.
// GET /api/server
func (h *Handler) GetServer(ctx context.Context) (*oas.ServerInfo, error) {
	return &oas.ServerInfo{
		ID:             1,
		Registration:   true,
		Readonly:       false,
		DeviceReadonly: false,
		LimitCommands:  false,
		Version:        "3.0.0",
		Map:            oas.OptString{Value: "osm", Set: true},
		Latitude:       oas.OptFloat64{Value: 49.79, Set: true},
		Longitude:      oas.OptFloat64{Value: 9.95, Set: true},
		Zoom:           oas.OptInt{Value: 13, Set: true},
		OpenIdEnabled:  oas.OptBool{Value: false, Set: true},
		OpenIdForce:    oas.OptBool{Value: false, Set: true},
		Attributes:     oas.OptAttributes{Value: oas.Attributes{}, Set: true},
		AiEnabled:      oas.OptBool{Value: h.cfg.AIEnabled, Set: true},
	}, nil
}

// GetVersion returns the build version information.
// GET /api/version
func (h *Handler) GetVersion(ctx context.Context) (*oas.VersionInfo, error) {
	v := &oas.VersionInfo{
		Version: version.Version,
	}
	if version.Commit != "" && version.Commit != "unknown" {
		v.Commit = oas.OptString{Value: version.Commit, Set: true}
	}
	if version.BuildDate != "" && version.BuildDate != "unknown" {
		v.BuildTime = oas.OptString{Value: version.BuildDate, Set: true}
	}
	return v, nil
}

// AdminListPositions returns all latest positions for all devices (admin only).
// GET /api/admin/positions
func (h *Handler) AdminListPositions(ctx context.Context) (oas.AdminListPositionsRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminListPositionsForbidden{Error: "admin access required"}, nil
	}

	positions, err := h.cfg.Positions.GetLatestAll(ctx)
	if err != nil {
		return &oas.AdminListPositionsForbidden{Error: "failed to list positions"}, nil
	}

	result := make(oas.AdminListPositionsOKApplicationJSON, 0, len(positions))
	for _, p := range positions {
		result = append(result, positionToOAS(p))
	}
	return &result, nil
}
