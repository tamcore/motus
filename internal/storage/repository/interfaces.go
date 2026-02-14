package repository

import (
	"context"
	"time"

	"github.com/tamcore/motus/internal/model"
)

// DeviceRepo defines the operations on the devices table used by handlers,
// services, protocol servers, and middleware. All handler/service code should
// depend on this interface rather than the concrete *DeviceRepository.
type DeviceRepo interface {
	UserHasAccess(ctx context.Context, user *model.User, deviceID int64) bool
	GetByID(ctx context.Context, id int64) (*model.Device, error)
	GetByUniqueID(ctx context.Context, uniqueID string) (*model.Device, error)
	GetByUser(ctx context.Context, userID int64) ([]*model.Device, error)
	GetAll(ctx context.Context) ([]model.Device, error)
	GetAllWithOwners(ctx context.Context) ([]model.Device, error)
	GetTimedOut(ctx context.Context, cutoff time.Time) ([]model.Device, error)
	GetUserIDs(ctx context.Context, deviceID int64) ([]int64, error)
	Create(ctx context.Context, d *model.Device, userID int64) error
	Update(ctx context.Context, d *model.Device) error
	Delete(ctx context.Context, id int64) error
}

// UserRepo defines the operations on the users table used by handlers,
// auth middleware, and admin endpoints.
type UserRepo interface {
	Create(ctx context.Context, user *model.User) error
	CreateOIDCUser(ctx context.Context, email, name, role, subject, issuer string) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByToken(ctx context.Context, token string) (*model.User, error)
	GetByOIDCSubject(ctx context.Context, subject, issuer string) (*model.User, error)
	SetOIDCSubject(ctx context.Context, userID int64, subject, issuer string) error
	ListAll(ctx context.Context) ([]*model.User, error)
	Update(ctx context.Context, user *model.User) error
	UpdatePassword(ctx context.Context, userID int64, hash string) error
	Delete(ctx context.Context, id int64) error
	GetDevicesForUser(ctx context.Context, userID int64) ([]int64, error)
	AssignDevice(ctx context.Context, userID, deviceID int64) error
	UnassignDevice(ctx context.Context, userID, deviceID int64) error
	GenerateToken(ctx context.Context, userID int64) (string, error)
}

// OIDCStateRepo defines the operations on the oidc_states table used to
// prevent CSRF attacks in the OIDC redirect flow.
type OIDCStateRepo interface {
	Create(ctx context.Context, state string) error
	Consume(ctx context.Context, state string) (bool, error)
}

// SessionRepo defines the operations on the sessions table used by auth
// middleware, session handler, and sudo handler.
type SessionRepo interface {
	Create(ctx context.Context, userID int64) (*model.Session, error)
	CreateWithExpiry(ctx context.Context, userID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error)
	CreateWithApiKey(ctx context.Context, userID int64, apiKeyID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error)
	CreateSudo(ctx context.Context, targetUserID, originalUserID int64) (*model.Session, error)
	GetByID(ctx context.Context, id string) (*model.Session, error)
	GetByIDPrefix(ctx context.Context, userID int64, prefix string) (*model.Session, error)
	Delete(ctx context.Context, id string) error
	ListByUser(ctx context.Context, userID int64) ([]*model.Session, error)
}

// PositionRepo defines the operations on the positions table used by
// handlers, services, and the protocol position handler.
type PositionRepo interface {
	Create(ctx context.Context, p *model.Position) error
	GetLatestByDevice(ctx context.Context, deviceID int64) (*model.Position, error)
	GetLatestByUser(ctx context.Context, userID int64) ([]*model.Position, error)
	GetLatestAll(ctx context.Context) ([]*model.Position, error)
	GetByDeviceAndTimeRange(ctx context.Context, deviceID int64, from, to time.Time, limit int) ([]*model.Position, error)
	GetPreviousByDevice(ctx context.Context, deviceID int64, beforeTime time.Time) (*model.Position, error)
	GetByID(ctx context.Context, id int64) (*model.Position, error)
	GetByIDs(ctx context.Context, ids []int64) ([]*model.Position, error)
	UpdateAddress(ctx context.Context, positionID int64, address string) error
	UpdateGeofenceIDs(ctx context.Context, positionID int64, ids []int64) error
	GetLastMovingPosition(ctx context.Context, deviceID int64, speedThreshold float64) (*model.Position, error)
}

// GeofenceRepo defines the operations on the geofences table used by
// handlers and the geofence event service.
type GeofenceRepo interface {
	Create(ctx context.Context, g *model.Geofence) error
	GetByID(ctx context.Context, id int64) (*model.Geofence, error)
	GetByUser(ctx context.Context, userID int64) ([]*model.Geofence, error)
	GetAll(ctx context.Context) ([]*model.Geofence, error)
	GetAllWithOwners(ctx context.Context) ([]*model.Geofence, error)
	Update(ctx context.Context, g *model.Geofence) error
	Delete(ctx context.Context, id int64) error
	AssociateUser(ctx context.Context, userID, geofenceID int64) error
	UserHasAccess(ctx context.Context, user *model.User, geofenceID int64) bool
	CheckContainment(ctx context.Context, userID int64, lat, lon float64) ([]int64, error)
}

// EventRepo defines the operations on the events table used by handlers
// and event detection services.
type EventRepo interface {
	Create(ctx context.Context, e *model.Event) error
	GetByDevice(ctx context.Context, deviceID int64, limit int) ([]*model.Event, error)
	GetRecentByDeviceAndType(ctx context.Context, deviceID int64, eventType string, limit int) ([]*model.Event, error)
	GetByUser(ctx context.Context, userID int64, limit int) ([]*model.Event, error)
	GetByFilters(ctx context.Context, userID int64, deviceIDs []int64, eventTypes []string, from, to time.Time) ([]*model.Event, error)
}

// CommandRepo defines the operations on the commands table.
type CommandRepo interface {
	Create(ctx context.Context, cmd *model.Command) error
	GetPendingByDevice(ctx context.Context, deviceID int64) ([]*model.Command, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
	ListByDevice(ctx context.Context, deviceID int64, limit int) ([]*model.Command, error)
	AppendResult(ctx context.Context, id int64, chunk string) error
	GetLatestSentByDevice(ctx context.Context, deviceID int64) (*model.Command, error)
}

// NotificationRepo defines the operations on the notification_rules and
// notification_log tables.
type NotificationRepo interface {
	Create(ctx context.Context, rule *model.NotificationRule) error
	GetByID(ctx context.Context, id int64) (*model.NotificationRule, error)
	GetByUser(ctx context.Context, userID int64) ([]*model.NotificationRule, error)
	GetAll(ctx context.Context) ([]*model.NotificationRule, error)
	GetByEventType(ctx context.Context, userID int64, eventType string) ([]*model.NotificationRule, error)
	Update(ctx context.Context, rule *model.NotificationRule) error
	Delete(ctx context.Context, id int64) error
	LogDelivery(ctx context.Context, entry *model.NotificationLog) error
	GetLogsByRule(ctx context.Context, ruleID int64, limit int) ([]*model.NotificationLog, error)
}

// DeviceShareRepo defines the operations on the device_shares table.
type DeviceShareRepo interface {
	Create(ctx context.Context, share *model.DeviceShare) error
	GetByToken(ctx context.Context, token string) (*model.DeviceShare, error)
	ListByDevice(ctx context.Context, deviceID int64) ([]*model.DeviceShare, error)
	GetByID(ctx context.Context, id int64) (*model.DeviceShare, error)
	Delete(ctx context.Context, id int64) error
}

// ApiKeyRepo defines the operations on the api_keys table used by auth
// middleware and API key management handlers.
type ApiKeyRepo interface {
	Create(ctx context.Context, key *model.ApiKey) error
	GetByToken(ctx context.Context, token string) (*model.ApiKey, error)
	GetByID(ctx context.Context, id int64) (*model.ApiKey, error)
	ListByUser(ctx context.Context, userID int64) ([]*model.ApiKey, error)
	Delete(ctx context.Context, id int64) error
	UpdateLastUsed(ctx context.Context, id int64) error
}

// CalendarRepo defines the operations on the calendars table used by
// handlers and the geofence event service for time-based triggers.
type CalendarRepo interface {
	Create(ctx context.Context, c *model.Calendar) error
	GetByID(ctx context.Context, id int64) (*model.Calendar, error)
	GetByUser(ctx context.Context, userID int64) ([]*model.Calendar, error)
	GetAll(ctx context.Context) ([]*model.Calendar, error)
	Update(ctx context.Context, c *model.Calendar) error
	Delete(ctx context.Context, id int64) error
	UserHasAccess(ctx context.Context, user *model.User, calendarID int64) bool
	AssociateUser(ctx context.Context, userID, calendarID int64) error
}

// StatisticsRepo defines the operations for platform and user statistics.
type StatisticsRepo interface {
	GetPlatformStats(ctx context.Context) (*PlatformStats, error)
	GetUserStats(ctx context.Context, userID int64) (*UserStats, error)
}

// Compile-time assertions: ensure concrete types satisfy their interfaces.
var (
	_ DeviceRepo       = (*DeviceRepository)(nil)
	_ UserRepo         = (*UserRepository)(nil)
	_ SessionRepo      = (*SessionRepository)(nil)
	_ PositionRepo     = (*PositionRepository)(nil)
	_ GeofenceRepo     = (*GeofenceRepository)(nil)
	_ EventRepo        = (*EventRepository)(nil)
	_ CommandRepo      = (*CommandRepository)(nil)
	_ NotificationRepo = (*NotificationRepository)(nil)
	_ DeviceShareRepo  = (*DeviceShareRepository)(nil)
	_ ApiKeyRepo       = (*ApiKeyRepository)(nil)
	_ StatisticsRepo   = (*StatisticsRepository)(nil)
	_ CalendarRepo     = (*CalendarRepository)(nil)
	_ OIDCStateRepo    = (*OIDCStateRepository)(nil)
)
