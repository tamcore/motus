package handlers

import (
	"encoding/json"
	"fmt"
	"net/url"
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

// attrBool extracts a bool value from an attribute map by key.
func attrBool(attrs map[string]interface{}, key string) oas.OptBool {
	v, ok := attrs[key]
	if !ok {
		return oas.OptBool{}
	}
	b, ok := v.(bool)
	if !ok {
		return oas.OptBool{}
	}
	return oas.OptBool{Value: b, Set: true}
}

// attrString extracts a string value from an attribute map by key.
func attrString(attrs map[string]interface{}, key string) oas.OptString {
	v, ok := attrs[key]
	if !ok {
		return oas.OptString{}
	}
	s, ok := v.(string)
	if !ok {
		return oas.OptString{}
	}
	return oas.OptString{Value: s, Set: true}
}

// attrInt extracts an integer value from an attribute map by key.
// JSON numbers decode as float64, so both float64 and int are handled.
func attrInt(attrs map[string]interface{}, key string) oas.OptInt {
	v, ok := attrs[key]
	if !ok {
		return oas.OptInt{}
	}
	switch n := v.(type) {
	case float64:
		return oas.OptInt{Value: int(n), Set: true}
	case int:
		return oas.OptInt{Value: n, Set: true}
	case int64:
		return oas.OptInt{Value: int(n), Set: true}
	default:
		return oas.OptInt{}
	}
}

var positionKnownKeys = map[string]struct{}{
	"motion": {}, "ignition": {}, "flags": {}, "alarm": {},
	"mcc": {}, "mnc": {}, "lac": {}, "cellId": {}, "iccid": {}, "satellites": {},
}

// positionAttrsToOAS converts a model attribute map to a typed oas.PositionAttributes.
// The 10 known protocol keys are extracted into typed fields; remaining keys go into AdditionalProps.
func positionAttrsToOAS(attrs map[string]interface{}) oas.PositionAttributes {
	pa := oas.PositionAttributes{
		Motion:     attrBool(attrs, "motion"),
		Ignition:   attrBool(attrs, "ignition"),
		Flags:      attrString(attrs, "flags"),
		Alarm:      attrString(attrs, "alarm"),
		Mcc:        attrInt(attrs, "mcc"),
		Mnc:        attrInt(attrs, "mnc"),
		Lac:        attrInt(attrs, "lac"),
		CellId:     attrInt(attrs, "cellId"),
		Iccid:      attrString(attrs, "iccid"),
		Satellites: attrInt(attrs, "satellites"),
	}
	for k, v := range attrs {
		if _, known := positionKnownKeys[k]; known {
			continue
		}
		if pa.AdditionalProps == nil {
			pa.AdditionalProps = make(oas.PositionAttributesAdditional)
		}
		if b, err := json.Marshal(v); err == nil {
			pa.AdditionalProps[k] = jx.Raw(b)
		}
	}
	return pa
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
		Attributes:     oas.Attributes(attrsToRaw(d.Attributes)),
		OwnerName:      optStr(d.OwnerName),
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}

// userToOAS converts a model.User to oas.User.
func userToOAS(u *model.User) oas.User {
	var attrs oas.OptAttributes
	if u.Attributes != nil {
		attrs = oas.OptAttributes{Value: oas.Attributes(attrsToRaw(u.Attributes)), Set: true}
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
		Attributes: positionAttrsToOAS(p.Attributes),
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
		Attributes:  oas.Attributes(attrsToRaw(g.Attributes)),
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

// buildCommandAttributes converts a model command type+attrs map to a typed oas.OptCommandAttributes.
func buildCommandAttributes(cmdType string, modelAttrs map[string]interface{}) oas.OptCommandAttributes {
	if modelAttrs == nil {
		return oas.OptCommandAttributes{}
	}
	var ca oas.CommandAttributes
	switch cmdType {
	case "custom":
		text, _ := modelAttrs["text"].(string)
		ca.SetCommandAttrCustom(oas.CommandAttrCustom{Type: oas.CommandAttrCustomTypeCustom, Text: text})
	case "positionPeriodic":
		ca.SetCommandAttrPositionPeriodic(oas.CommandAttrPositionPeriodic{
			Type:      oas.CommandAttrPositionPeriodicTypePositionPeriodic,
			Frequency: attrInt(modelAttrs, "frequency").Value,
		})
	case "sosNumber":
		phone, _ := modelAttrs["phoneNumber"].(string)
		ca.SetCommandAttrSosNumber(oas.CommandAttrSosNumber{
			Type:        oas.CommandAttrSosNumberTypeSosNumber,
			PhoneNumber: phone,
		})
	case "setSpeedAlarm":
		speed, _ := modelAttrs["speed"].(float64)
		ca.SetCommandAttrSetSpeedAlarm(oas.CommandAttrSetSpeedAlarm{
			Type:  oas.CommandAttrSetSpeedAlarmTypeSetSpeedAlarm,
			Speed: speed,
		})
	default:
		return oas.OptCommandAttributes{}
	}
	return oas.OptCommandAttributes{Value: ca, Set: true}
}

// oasCommandAttrsToModel converts a typed oas.OptCommandAttributes to a model attribute map.
func oasCommandAttrsToModel(attrs oas.OptCommandAttributes) map[string]interface{} {
	if !attrs.Set {
		return nil
	}
	switch {
	case attrs.Value.IsCommandAttrCustom():
		return map[string]interface{}{"text": attrs.Value.CommandAttrCustom.Text}
	case attrs.Value.IsCommandAttrPositionPeriodic():
		return map[string]interface{}{"frequency": attrs.Value.CommandAttrPositionPeriodic.Frequency}
	case attrs.Value.IsCommandAttrSosNumber():
		return map[string]interface{}{"phoneNumber": attrs.Value.CommandAttrSosNumber.PhoneNumber}
	case attrs.Value.IsCommandAttrSetSpeedAlarm():
		return map[string]interface{}{"speed": attrs.Value.CommandAttrSetSpeedAlarm.Speed}
	}
	return nil
}

// commandToOAS converts a model.Command to oas.Command.
func commandToOAS(c *model.Command) oas.Command {
	return oas.Command{
		ID:         c.ID,
		DeviceId:   c.DeviceID,
		Type:       c.Type,
		Status:     c.Status,
		Result:     ptrToOptStr(c.Result),
		ExecutedAt: ptrToOptTime(c.ExecutedAt),
		CreatedAt:  c.CreatedAt,
		Attributes: buildCommandAttributes(c.Type, c.Attributes),
	}
}

// buildEventAttributes converts a model event type+attrs map to a typed oas.OptEventAttributes.
func buildEventAttributes(evtType string, modelAttrs map[string]interface{}) oas.OptEventAttributes {
	if modelAttrs == nil {
		return oas.OptEventAttributes{}
	}
	var ea oas.EventAttributes
	switch evtType {
	case "ignitionOn":
		ignition, _ := modelAttrs["ignition"].(bool)
		ea.SetEventAttrIgnition(oas.EventAttributesIgnitionOnEventAttributes,
			oas.EventAttrIgnition{Type: evtType, Ignition: ignition})
	case "ignitionOff":
		ignition, _ := modelAttrs["ignition"].(bool)
		ea.SetEventAttrIgnition(oas.EventAttributesIgnitionOffEventAttributes,
			oas.EventAttrIgnition{Type: evtType, Ignition: ignition})
	case "alarm":
		alarm, _ := modelAttrs["alarm"].(string)
		ea.SetEventAttrAlarm(oas.EventAttrAlarm{Type: evtType, Alarm: alarm})
	case "motion":
		speed, _ := modelAttrs["speed"].(float64)
		prevSpeed, _ := modelAttrs["previousSpeed"].(float64)
		ea.SetEventAttrMotion(oas.EventAttrMotion{Type: evtType, Speed: speed, PreviousSpeed: prevSpeed})
	case "tripCompleted":
		distance, _ := modelAttrs["distance"].(float64)
		mileage, _ := modelAttrs["mileage"].(float64)
		ea.SetEventAttrTrip(oas.EventAttrTrip{Type: evtType, Distance: distance, Mileage: mileage})
	case "deviceIdle":
		idleDuration, _ := modelAttrs["idleDuration"].(float64)
		ea.SetEventAttrIdle(oas.EventAttrIdle{Type: evtType, IdleDuration: idleDuration})
	default:
		return oas.OptEventAttributes{}
	}
	return oas.OptEventAttributes{Value: ea, Set: true}
}

// eventToOAS converts a model.Event to oas.Event.
func eventToOAS(e *model.Event) oas.Event {
	return oas.Event{
		ID:         e.ID,
		DeviceId:   e.DeviceID,
		GeofenceId: ptrToOptInt64(e.GeofenceID),
		Type:       e.Type,
		PositionId: ptrToOptInt64(e.PositionID),
		EventTime:  e.Timestamp,
		Attributes: buildEventAttributes(e.Type, e.Attributes),
	}
}

// notificationRuleToOAS converts a model.NotificationRule to oas.NotificationRule.
func notificationRuleToOAS(n *model.NotificationRule) oas.NotificationRule {
	var config oas.NotificationRuleConfig
	switch n.Channel {
	case "webhook":
		webhookURL, _ := n.Config["webhookUrl"].(string)
		u, err := url.Parse(webhookURL)
		if err != nil || u == nil {
			u = &url.URL{}
		}
		wh := oas.NotificationConfigWebhook{
			Channel:    oas.NotificationConfigWebhookChannelWebhook,
			WebhookUrl: *u,
		}
		if h, ok := n.Config["headers"].(map[string]interface{}); ok && len(h) > 0 {
			headers := make(oas.NotificationConfigWebhookHeaders, len(h))
			for k, v := range h {
				if s, ok := v.(string); ok {
					headers[k] = s
				}
			}
			wh.Headers = oas.OptNotificationConfigWebhookHeaders{Value: headers, Set: true}
		}
		config.SetNotificationConfigWebhook(wh)
	}
	return oas.NotificationRule{
		ID:         n.ID,
		UserId:     n.UserID,
		Name:       n.Name,
		EventTypes: n.EventTypes,
		Channel:    n.Channel,
		Config:     config,
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

// detailStr extracts a string value from an audit details map.
func detailStr(details map[string]interface{}, key string) string {
	if details == nil {
		return ""
	}
	v, _ := details[key].(string)
	return v
}

// detailInt64 extracts an int64 value from an audit details map.
// JSON numbers unmarshal as float64, so both numeric types are handled.
func detailInt64(details map[string]interface{}, key string) int64 {
	if details == nil {
		return 0
	}
	switch n := details[key].(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

// detailStringSlice extracts a []string from an audit details map.
// JSON arrays unmarshal as []interface{}, so each element is type-asserted.
func detailStringSlice(details map[string]interface{}, key string) []string {
	if details == nil {
		return nil
	}
	raw, ok := details[key]
	if !ok {
		return nil
	}
	if ss, ok := raw.([]string); ok {
		return ss
	}
	if si, ok := raw.([]interface{}); ok {
		out := make([]string, 0, len(si))
		for _, v := range si {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// buildAuditMetadata converts an action string + details map to a typed oas.OptAuditMetadata.
// Returns an unset optional for unknown actions.
func buildAuditMetadata(action string, details map[string]interface{}) oas.OptAuditMetadata {
	var am oas.AuditMetadata
	switch action {
	// Empty variants (no payload beyond the action field).
	case "session.logout":
		am.SetAuditMetaEmpty(oas.AuditMetadataSessionLogoutAuditMetadata, oas.AuditMetaEmpty{Action: action})
	case "device.delete":
		am.SetAuditMetaEmpty(oas.AuditMetadataDeviceDeleteAuditMetadata, oas.AuditMetaEmpty{Action: action})
	case "geofence.delete":
		am.SetAuditMetaEmpty(oas.AuditMetadataGeofenceDeleteAuditMetadata, oas.AuditMetaEmpty{Action: action})
	case "calendar.delete":
		am.SetAuditMetaEmpty(oas.AuditMetadataCalendarDeleteAuditMetadata, oas.AuditMetaEmpty{Action: action})
	case "notification.delete":
		am.SetAuditMetaEmpty(oas.AuditMetadataNotificationDeleteAuditMetadata, oas.AuditMetaEmpty{Action: action})
	case "user.delete":
		am.SetAuditMetaEmpty(oas.AuditMetadataUserDeleteAuditMetadata, oas.AuditMetaEmpty{Action: action})
	case "device.online":
		am.SetAuditMetaEmpty(oas.AuditMetadataDeviceOnlineAuditMetadata, oas.AuditMetaEmpty{Action: action})
	case "device.offline":
		am.SetAuditMetaEmpty(oas.AuditMetadataDeviceOfflineAuditMetadata, oas.AuditMetaEmpty{Action: action})
	case "session.login":
		am.SetAuditMetaSessionLogin(oas.AuditMetaSessionLogin{Action: action, Email: detailStr(details, "email")})
	case "session.login_failed":
		am.SetAuditMetaSessionLoginFailed(oas.AuditMetaSessionLoginFailed{
			Action: action,
			Email:  detailStr(details, "email"),
			Reason: detailStr(details, "reason"),
		})
	case "session.sudo":
		am.SetAuditMetaSessionSudo(oas.AuditMetadataSessionSudoAuditMetadata, oas.AuditMetaSessionSudo{
			Action:      action,
			AdminEmail:  detailStr(details, "adminEmail"),
			TargetEmail: detailStr(details, "targetEmail"),
		})
	case "session.sudo_end":
		am.SetAuditMetaSessionSudo(oas.AuditMetadataSessionSudoEndAuditMetadata, oas.AuditMetaSessionSudo{
			Action:      action,
			AdminEmail:  detailStr(details, "adminEmail"),
			TargetEmail: detailStr(details, "targetEmail"),
		})
	case "session.revoke":
		rev := oas.AuditMetaSessionRevoke{Action: action}
		if s := detailStr(details, "scope"); s != "" {
			rev.Scope = oas.OptString{Value: s, Set: true}
		}
		if s := detailStr(details, "revokedSessionId"); s != "" {
			rev.RevokedSessionId = oas.OptString{Value: s, Set: true}
		}
		if uid := detailInt64(details, "sessionOwnerUserId"); uid != 0 {
			rev.SessionOwnerUserId = oas.OptInt64{Value: uid, Set: true}
		}
		am.SetAuditMetaSessionRevoke(rev)
	case "user.create":
		am.SetAuditMetaUserCreate(oas.AuditMetaUserCreate{
			Action: action,
			Email:  detailStr(details, "email"),
			Role:   detailStr(details, "role"),
		})
	case "user.update":
		upd := oas.AuditMetaUserUpdate{Action: action, Email: detailStr(details, "email")}
		upd.OldEmail = attrString(details, "oldEmail")
		upd.NewEmail = attrString(details, "newEmail")
		upd.OldName = attrString(details, "oldName")
		upd.NewName = attrString(details, "newName")
		upd.OldRole = attrString(details, "oldRole")
		upd.NewRole = attrString(details, "newRole")
		upd.Disabled = attrBool(details, "disabled")
		upd.Readonly = attrBool(details, "readonly")
		am.SetAuditMetaUserUpdate(upd)
	case "device.create":
		am.SetAuditMetaDeviceCreate(oas.AuditMetaDeviceCreate{
			Action:   action,
			Name:     detailStr(details, "name"),
			UniqueId: detailStr(details, "uniqueId"),
		})
	case "device.update":
		am.SetAuditMetaDeviceUpdate(oas.AuditMetaDeviceUpdate{
			Action: action,
			Name:   detailStr(details, "name"),
		})
	case "device.assign":
		userID := detailInt64(details, "userId")
		if userID == 0 {
			userID = detailInt64(details, "targetUserId")
		}
		am.SetAuditMetaDeviceAssign(oas.AuditMetadataDeviceAssignAuditMetadata, oas.AuditMetaDeviceAssign{
			Action: action, UserId: userID,
		})
	case "device.unassign":
		userID := detailInt64(details, "userId")
		if userID == 0 {
			userID = detailInt64(details, "targetUserId")
		}
		am.SetAuditMetaDeviceAssign(oas.AuditMetadataDeviceUnassignAuditMetadata, oas.AuditMetaDeviceAssign{
			Action: action, UserId: userID,
		})
	case "device.gpx_import":
		am.SetAuditMetaDeviceGpxImport(oas.AuditMetaDeviceGpxImport{
			Action:    action,
			DeviceId:  detailInt64(details, "deviceId"),
			Positions: int(detailInt64(details, "positions")),
		})
	case "geofence.create":
		am.SetAuditMetaNamedResource(oas.AuditMetadataGeofenceCreateAuditMetadata, oas.AuditMetaNamedResource{Action: action, Name: detailStr(details, "name")})
	case "geofence.update":
		am.SetAuditMetaNamedResource(oas.AuditMetadataGeofenceUpdateAuditMetadata, oas.AuditMetaNamedResource{Action: action, Name: detailStr(details, "name")})
	case "calendar.create":
		am.SetAuditMetaNamedResource(oas.AuditMetadataCalendarCreateAuditMetadata, oas.AuditMetaNamedResource{Action: action, Name: detailStr(details, "name")})
	case "calendar.update":
		am.SetAuditMetaNamedResource(oas.AuditMetadataCalendarUpdateAuditMetadata, oas.AuditMetaNamedResource{Action: action, Name: detailStr(details, "name")})
	case "notification.create":
		am.SetAuditMetaNotificationRule(oas.AuditMetadataNotificationCreateAuditMetadata, oas.AuditMetaNotificationRule{
			Action:     action,
			Name:       detailStr(details, "name"),
			EventTypes: detailStringSlice(details, "eventTypes"),
			Channel:    detailStr(details, "channel"),
		})
	case "notification.update":
		am.SetAuditMetaNotificationRule(oas.AuditMetadataNotificationUpdateAuditMetadata, oas.AuditMetaNotificationRule{
			Action:     action,
			Name:       detailStr(details, "name"),
			EventTypes: detailStringSlice(details, "eventTypes"),
			Channel:    detailStr(details, "channel"),
		})
	case "notification.sent":
		d := oas.AuditMetaNotifDelivery{
			Action:    action,
			RuleName:  detailStr(details, "ruleName"),
			EventType: detailStr(details, "eventType"),
			Channel:   detailStr(details, "channel"),
			DeviceId:  detailInt64(details, "deviceId"),
		}
		if rc := attrInt(details, "responseCode"); rc.Set {
			d.ResponseCode = oas.OptInt{Value: rc.Value, Set: true}
		}
		am.SetAuditMetaNotifDelivery(oas.AuditMetadataNotificationSentAuditMetadata, d)
	case "notification.failed":
		d := oas.AuditMetaNotifDelivery{
			Action:    action,
			RuleName:  detailStr(details, "ruleName"),
			EventType: detailStr(details, "eventType"),
			Channel:   detailStr(details, "channel"),
			DeviceId:  detailInt64(details, "deviceId"),
		}
		if rc := attrInt(details, "responseCode"); rc.Set {
			d.ResponseCode = oas.OptInt{Value: rc.Value, Set: true}
		}
		if errStr := detailStr(details, "error"); errStr != "" {
			d.Error = oas.OptString{Value: errStr, Set: true}
		}
		am.SetAuditMetaNotifDelivery(oas.AuditMetadataNotificationFailedAuditMetadata, d)
	case "apikey.create":
		am.SetAuditMetaApiKeyCreate(oas.AuditMetaApiKeyCreate{
			Action:      action,
			Name:        detailStr(details, "name"),
			Permissions: detailStr(details, "permissions"),
		})
	case "apikey.delete":
		am.SetAuditMetaApiKeyDelete(oas.AuditMetaApiKeyDelete{
			Action:         action,
			Name:           detailStr(details, "name"),
			KeyOwnerUserId: detailInt64(details, "keyOwnerUserId"),
		})
	case "share.create":
		am.SetAuditMetaShare(oas.AuditMetadataShareCreateAuditMetadata, oas.AuditMetaShare{Action: action, DeviceId: detailInt64(details, "deviceId")})
	case "share.delete":
		am.SetAuditMetaShare(oas.AuditMetadataShareDeleteAuditMetadata, oas.AuditMetaShare{Action: action, DeviceId: detailInt64(details, "deviceId")})
	case "command.send":
		am.SetAuditMetaCommandSend(oas.AuditMetaCommandSend{
			Action:        action,
			CommandType:   detailStr(details, "commandType"),
			CommandStatus: detailStr(details, "commandStatus"),
			DeviceName:    detailStr(details, "deviceName"),
		})
	default:
		return oas.OptAuditMetadata{}
	}
	return oas.OptAuditMetadata{Value: am, Set: true}
}

// auditEntryToOAS converts an audit.Entry to oas.AuditEntry.
func auditEntryToOAS(e audit.Entry) oas.AuditEntry {
	var (
		userID       int64
		resourceType oas.OptString
		resourceID   oas.OptString
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
	return oas.AuditEntry{
		ID:           e.ID,
		Action:       e.Action,
		UserId:       userID,
		ResourceType: resourceType,
		ResourceId:   resourceID,
		Metadata:     buildAuditMetadata(e.Action, e.Details),
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
	if v, ok := req.Protocol.Get(); ok {
		d.Protocol = v
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
