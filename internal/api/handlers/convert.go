package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-faster/jx"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
)

// rawToAttrs converts a jx.Raw attribute map to map[string]interface{}.
func rawToAttrs(raw map[string]jx.Raw) map[string]interface{} {
	if raw == nil {
		return nil
	}
	out := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		var x interface{}
		if err := json.Unmarshal(v, &x); err == nil {
			out[k] = x
		}
	}
	return out
}

// attrsToRaw converts a map[string]interface{} to a jx.Raw attribute map.
func attrsToRaw(attrs map[string]interface{}) map[string]jx.Raw {
	if attrs == nil {
		return nil
	}
	out := make(map[string]jx.Raw, len(attrs))
	for k, v := range attrs {
		if b, err := json.Marshal(v); err == nil {
			out[k] = jx.Raw(b)
		}
	}
	return out
}

// optStr wraps a non-empty string in OptString.
func optStr(s string) oas.OptString {
	if s == "" {
		return oas.OptString{}
	}
	return oas.OptString{Value: s, Set: true}
}

// ptrToOptStr converts *string to OptNilString (null when nil).
func ptrToOptStr(s *string) oas.OptNilString {
	if s == nil {
		return oas.OptNilString{Set: true, Null: true}
	}
	return oas.OptNilString{Value: *s, Set: true}
}

// ptrToOptInt64 converts *int64 to OptNilInt64 (null when nil).
func ptrToOptInt64(i *int64) oas.OptNilInt64 {
	if i == nil {
		return oas.OptNilInt64{Set: true, Null: true}
	}
	return oas.OptNilInt64{Value: *i, Set: true}
}

// ptrToOptFloat64 converts *float64 to OptNilFloat64 (null when nil).
func ptrToOptFloat64(f *float64) oas.OptNilFloat64 {
	if f == nil {
		return oas.OptNilFloat64{Set: true, Null: true}
	}
	return oas.OptNilFloat64{Value: *f, Set: true}
}

// ptrToOptTime converts *time.Time to OptNilDateTime (null when nil).
func ptrToOptTime(t *time.Time) oas.OptNilDateTime {
	if t == nil {
		return oas.OptNilDateTime{Set: true, Null: true}
	}
	return oas.OptNilDateTime{Value: *t, Set: true}
}

// derefTime returns *t or the zero time.Time if t is nil.
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// derefFloat64 returns *f or 0.0 if f is nil.
func derefFloat64(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// deviceToOAS converts a model.Device to oas.Device.
func deviceToOAS(d *model.Device) oas.Device {
	return oas.Device{
		ID:             d.ID,
		UniqueId:       d.UniqueID,
		Name:           d.Name,
		Protocol:       optStr(d.Protocol),
		Status:         d.Status,
		SpeedLimit:     ptrToOptFloat64(d.SpeedLimit),
		LastUpdate:     ptrToOptTime(d.LastUpdate),
		PositionId:     ptrToOptInt64(d.PositionID),
		GroupId:        ptrToOptInt64(d.GroupID),
		Phone:          ptrToOptStr(d.Phone),
		Model:          ptrToOptStr(d.Model),
		Contact:        ptrToOptStr(d.Contact),
		Category:       ptrToOptStr(d.Category),
		CalendarId:     ptrToOptInt64(d.CalendarID),
		ExpirationTime: ptrToOptTime(d.ExpirationTime),
		Disabled:       d.Disabled,
		Mileage:        ptrToOptFloat64(d.Mileage),
		Attributes:     oas.DeviceAttributes(attrsToRaw(d.Attributes)),
		OwnerName:      optStr(d.OwnerName),
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}

// userToOAS converts a model.User to oas.User.
func userToOAS(u *model.User) oas.User {
	var attrs oas.OptUserAttributes
	if u.Attributes != nil {
		attrs = oas.OptUserAttributes{Value: oas.UserAttributes(attrsToRaw(u.Attributes)), Set: true}
	}
	return oas.User{
		ID:            u.ID,
		Email:         u.Email,
		Name:          u.Name,
		Administrator: u.Administrator,
		Readonly:      u.Readonly,
		Disabled:      u.Disabled,
		CreatedAt:     u.CreatedAt,
		Attributes:    attrs,
	}
}

// positionToOAS converts a model.Position to oas.Position.
func positionToOAS(p *model.Position) oas.Position {
	var addr string
	if p.Address != nil {
		addr = *p.Address
	}
	return oas.Position{
		ID:         p.ID,
		DeviceId:   p.DeviceID,
		Protocol:   optStr(p.Protocol),
		ServerTime: derefTime(p.ServerTime),
		DeviceTime: derefTime(p.DeviceTime),
		FixTime:    p.Timestamp,
		Outdated:   oas.OptBool{Value: p.Outdated, Set: true},
		Valid:      p.Valid,
		Latitude:   p.Latitude,
		Longitude:  p.Longitude,
		Altitude:   derefFloat64(p.Altitude),
		Speed:      derefFloat64(p.Speed),
		Course:     derefFloat64(p.Course),
		Address:    optStr(addr),
		Accuracy:   p.Accuracy,
		Attributes: oas.PositionAttributes(attrsToRaw(p.Attributes)),
		Network:    oas.PositionNetwork(attrsToRaw(p.Network)),
	}
}

// sessionToOAS converts a model.Session to oas.Session.
func sessionToOAS(s *model.Session) oas.Session {
	return oas.Session{
		ID:                s.TruncatedID(),
		UserId:            s.UserID,
		RememberMe:        s.RememberMe,
		OriginalUserId:    ptrToOptInt64(s.OriginalUserID),
		IsSudo:            oas.OptBool{Value: s.IsSudo, Set: true},
		ApiKeyId:          ptrToOptInt64(s.ApiKeyID),
		ApiKeyName:        ptrToOptStr(s.ApiKeyName),
		IsCurrent:         oas.OptBool{Value: s.IsCurrent, Set: true},
		CreatedAt:         s.CreatedAt,
		ExpiresAt:         s.ExpiresAt,
		LastSeenAt:        ptrToOptTime(s.LastSeenAt),
		LastSeenIp:        ptrToOptStr(s.LastSeenIP),
		LastSeenUserAgent: ptrToOptStr(s.LastSeenUserAgent),
	}
}

// apiKeyToOAS converts a model.ApiKey to oas.ApiKey.
// includeToken controls whether the raw token is exposed (only on creation).
func apiKeyToOAS(k *model.ApiKey, includeToken bool) oas.ApiKey {
	var token oas.OptString
	if includeToken && k.Token != "" {
		token = oas.OptString{Value: k.Token, Set: true}
	}
	return oas.ApiKey{
		ID:          k.ID,
		UserId:      k.UserID,
		Token:       token,
		Name:        k.Name,
		Permissions: oas.ApiKeyPermissions(k.Permissions),
		ExpiresAt:   ptrToOptTime(k.ExpiresAt),
		CreatedAt:   k.CreatedAt,
		LastUsedAt:  ptrToOptTime(k.LastUsedAt),
	}
}

// geofenceToOAS converts a model.Geofence to oas.Geofence.
func geofenceToOAS(g *model.Geofence) oas.Geofence {
	return oas.Geofence{
		ID:          g.ID,
		Name:        g.Name,
		Description: optStr(g.Description),
		Area:        g.Area,
		Geometry:    optStr(g.Geometry),
		CalendarId:  ptrToOptInt64(g.CalendarID),
		OwnerName:   optStr(g.OwnerName),
		Attributes:  oas.GeofenceAttributes(attrsToRaw(g.Attributes)),
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}

// calendarToOAS converts a model.Calendar to oas.Calendar.
func calendarToOAS(c *model.Calendar) oas.Calendar {
	return oas.Calendar{
		ID:        c.ID,
		Name:      c.Name,
		Data:      c.Data,
		OwnerName: optStr(c.OwnerName),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// commandToOAS converts a model.Command to oas.Command.
func commandToOAS(c *model.Command) oas.Command {
	var attrs oas.OptCommandAttributes
	if c.Attributes != nil {
		attrs = oas.OptCommandAttributes{Value: oas.CommandAttributes(attrsToRaw(c.Attributes)), Set: true}
	}
	return oas.Command{
		ID:         c.ID,
		DeviceId:   c.DeviceID,
		Type:       c.Type,
		Status:     c.Status,
		Result:     ptrToOptStr(c.Result),
		ExecutedAt: ptrToOptTime(c.ExecutedAt),
		CreatedAt:  c.CreatedAt,
		Attributes: attrs,
	}
}

// eventToOAS converts a model.Event to oas.Event.
func eventToOAS(e *model.Event) oas.Event {
	var attrs oas.OptEventAttributes
	if e.Attributes != nil {
		attrs = oas.OptEventAttributes{Value: oas.EventAttributes(attrsToRaw(e.Attributes)), Set: true}
	}
	return oas.Event{
		ID:         e.ID,
		DeviceId:   e.DeviceID,
		GeofenceId: ptrToOptInt64(e.GeofenceID),
		Type:       e.Type,
		PositionId: ptrToOptInt64(e.PositionID),
		EventTime:  e.Timestamp,
		Attributes: attrs,
	}
}

// notificationRuleToOAS converts a model.NotificationRule to oas.NotificationRule.
func notificationRuleToOAS(n *model.NotificationRule) oas.NotificationRule {
	return oas.NotificationRule{
		ID:         n.ID,
		UserId:     n.UserID,
		Name:       n.Name,
		EventTypes: n.EventTypes,
		Channel:    n.Channel,
		Config:     oas.NotificationRuleConfig(attrsToRaw(n.Config)),
		Template:   n.Template,
		Enabled:    n.Enabled,
		OwnerName:  optStr(n.OwnerName),
		CreatedAt:  n.CreatedAt,
		UpdatedAt:  n.UpdatedAt,
	}
}

// notificationLogToOAS converts a model.NotificationLog to oas.NotificationLog.
func notificationLogToOAS(l *model.NotificationLog) oas.NotificationLog {
	var code oas.OptInt
	if l.ResponseCode != 0 {
		code = oas.OptInt{Value: l.ResponseCode, Set: true}
	}
	return oas.NotificationLog{
		ID:           l.ID,
		RuleId:       l.RuleID,
		EventId:      ptrToOptInt64(l.EventID),
		Status:       l.Status,
		SentAt:       ptrToOptTime(l.SentAt),
		Error:        optStr(l.Error),
		ResponseCode: code,
	}
}

// deviceShareToOAS converts a model.DeviceShare to oas.DeviceShare.
func deviceShareToOAS(s *model.DeviceShare) oas.DeviceShare {
	return oas.DeviceShare{
		ID:        s.ID,
		DeviceId:  s.DeviceID,
		Token:     s.Token,
		CreatedBy: s.CreatedBy,
		ExpiresAt: ptrToOptTime(s.ExpiresAt),
		CreatedAt: s.CreatedAt,
	}
}

// auditEntryToOAS converts an audit.Entry to oas.AuditEntry.
func auditEntryToOAS(e audit.Entry) oas.AuditEntry {
	var (
		userID       int64
		resourceType oas.OptString
		resourceID   oas.OptString
		metadata     oas.OptAuditEntryMetadata
		ipAddress    oas.OptString
	)
	if e.UserID != nil {
		userID = *e.UserID
	}
	if e.ResourceType != nil {
		resourceType = oas.OptString{Value: *e.ResourceType, Set: true}
	}
	if e.ResourceID != nil {
		resourceID = oas.OptString{Value: fmt.Sprintf("%d", *e.ResourceID), Set: true}
	}
	if e.IPAddress != nil {
		ipAddress = oas.OptString{Value: *e.IPAddress, Set: true}
	}
	if e.Details != nil {
		metadata = oas.OptAuditEntryMetadata{Value: oas.AuditEntryMetadata(attrsToRaw(e.Details)), Set: true}
	}
	return oas.AuditEntry{
		ID:           e.ID,
		Action:       e.Action,
		UserId:       userID,
		ResourceType: resourceType,
		ResourceId:   resourceID,
		Metadata:     metadata,
		IpAddress:    ipAddress,
		CreatedAt:    e.Timestamp,
	}
}

// oasInputToDevice converts an oas.DeviceInput to a new model.Device.
// Fields not set in the input are left as zero values.
func oasInputToDevice(req *oas.DeviceInput) *model.Device {
	d := &model.Device{
		UniqueID: req.UniqueId,
		Name:     req.Name,
		Status:   "unknown",
	}
	if v, ok := req.Phone.Get(); ok {
		d.Phone = &v
	}
	if v, ok := req.Model.Get(); ok {
		d.Model = &v
	}
	if v, ok := req.Contact.Get(); ok {
		d.Contact = &v
	}
	if v, ok := req.Category.Get(); ok {
		d.Category = &v
	}
	if v, ok := req.CalendarId.Get(); ok {
		d.CalendarID = &v
	}
	if v, ok := req.SpeedLimit.Get(); ok {
		d.SpeedLimit = &v
	}
	if v, ok := req.Disabled.Get(); ok {
		d.Disabled = v
	}
	if req.Attributes.Set {
		d.Attributes = rawToAttrs(map[string]jx.Raw(req.Attributes.Value))
	}
	return d
}
