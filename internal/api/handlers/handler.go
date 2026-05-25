package handlers

import (
	"context"
	"errors"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/config"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol"
	"github.com/tamcore/motus/internal/services"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// HandlerConfig holds all dependencies for the unified Handler.
type HandlerConfig struct {
	Users         repository.UserRepo
	Sessions      repository.SessionRepo
	Devices       repository.DeviceRepo
	Positions     repository.PositionRepo
	Commands      repository.CommandRepo
	Geofences     repository.GeofenceRepo
	Events        repository.EventRepo
	Notifications repository.NotificationRepo
	Shares        repository.DeviceShareRepo
	ApiKeys       repository.ApiKeyRepo
	Calendars     repository.CalendarRepo
	Stats         repository.StatisticsRepo
	OIDCStateRepo repository.OIDCStateRepo

	NotificationService *services.NotificationService
	DeviceRegistry      *protocol.DeviceRegistry
	EncoderRegistry     *protocol.EncoderRegistry
	Hub                 *websocket.Hub

	AuditLogger    *audit.Logger
	UniqueIDPrefix string
	OIDCConfig     config.OIDCConfig
}

// Handler is the single implementation of oas.Handler.
// Domain methods are defined in their respective files (device.go, session.go, etc.).
type Handler struct {
	cfg          HandlerConfig
	loginLimiter *loginLimiter
}

// NewHandler creates a Handler with the given configuration.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{cfg: cfg, loginLimiter: newLoginLimiter()}
}

// requireAdminCtx checks that the context contains an authenticated admin user.
// Returns the admin user and nil on success.
// NOTE: replaces the old http.HandlerFunc-based requireAdmin in users.go (Task 15).
func requireAdminCtx(ctx context.Context) (*model.User, error) {
	user := api.UserFromContext(ctx)
	if user == nil || !user.IsAdmin() {
		return nil, errors.New("forbidden: admin access required")
	}
	return user, nil
}
