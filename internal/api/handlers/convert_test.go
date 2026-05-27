package handlers

import (
	"net/url"
	"testing"
	"time"

	"github.com/go-faster/jx"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
)

func ptr[T any](v T) *T { return &v }

func TestRawToAttrs(t *testing.T) {
	raw := map[string]jx.Raw{
		"foo": jx.Raw(`"bar"`),
		"num": jx.Raw(`42`),
	}
	got := rawToAttrs(raw)
	if got["foo"] != "bar" {
		t.Errorf("foo = %v, want bar", got["foo"])
	}
	if got["num"] != float64(42) {
		t.Errorf("num = %v, want 42", got["num"])
	}
}

func TestRawToAttrs_Nil(t *testing.T) {
	if rawToAttrs(nil) != nil {
		t.Error("expected nil result for nil input")
	}
}

func TestAttrsToRaw(t *testing.T) {
	attrs := map[string]interface{}{"key": "value", "n": float64(7)}
	raw := attrsToRaw(attrs)
	if string(raw["key"]) != `"value"` {
		t.Errorf("key = %s, want %q", raw["key"], `"value"`)
	}
	if string(raw["n"]) != "7" {
		t.Errorf("n = %s, want 7", raw["n"])
	}
}

func TestAttrsToRaw_Nil(t *testing.T) {
	if attrsToRaw(nil) != nil {
		t.Error("expected nil result for nil input")
	}
}

func TestOptStr(t *testing.T) {
	if o := optStr(""); o.Set {
		t.Error("empty string should not be set")
	}
	if o := optStr("hello"); !o.Set || o.Value != "hello" {
		t.Errorf("optStr(hello) = %+v, want {Value:hello Set:true}", o)
	}
}

func TestPtrToOptStr(t *testing.T) {
	if o := ptrToOptStr(nil); !o.Set || !o.Null {
		t.Errorf("nil ptr should be {Set:true Null:true}, got %+v", o)
	}
	if o := ptrToOptStr(ptr("x")); !o.Set || o.Null || o.Value != "x" {
		t.Errorf("got %+v, want {Value:x Set:true}", o)
	}
}

func TestPtrToOptInt64(t *testing.T) {
	if o := ptrToOptInt64(nil); !o.Set || !o.Null {
		t.Errorf("nil should be null, got %+v", o)
	}
	if o := ptrToOptInt64(ptr(int64(99))); !o.Set || o.Null || o.Value != 99 {
		t.Errorf("got %+v, want {Value:99 Set:true}", o)
	}
}

func TestDerefTime(t *testing.T) {
	if derefTime(nil) != (time.Time{}) {
		t.Error("nil should return zero time")
	}
	now := time.Now()
	if derefTime(&now) != now {
		t.Error("should return pointed-to time")
	}
}

func TestDerefFloat64(t *testing.T) {
	if derefFloat64(nil) != 0 {
		t.Error("nil should return 0")
	}
	if derefFloat64(ptr(3.14)) != 3.14 {
		t.Error("should return pointed-to value")
	}
}

func TestDeviceToOAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := &model.Device{
		ID:         42,
		UniqueID:   "ABC123",
		Name:       "Tracker",
		Status:     "online",
		Disabled:   false,
		PositionID: ptr(int64(7)),
		Phone:      ptr("555-1234"),
		Attributes: map[string]interface{}{"color": "red"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	got := deviceToOAS(d)

	if got.ID != 42 {
		t.Errorf("ID = %d, want 42", got.ID)
	}
	if got.UniqueId != "ABC123" {
		t.Errorf("UniqueId = %s, want ABC123", got.UniqueId)
	}
	if !got.PositionId.Set || got.PositionId.Value != 7 {
		t.Errorf("PositionId = %+v, want {Value:7 Set:true}", got.PositionId)
	}
	if !got.Phone.Set || got.Phone.Null || got.Phone.Value != "555-1234" {
		t.Errorf("Phone = %+v, want {Value:555-1234 Set:true}", got.Phone)
	}
	if got.Attributes["color"] == nil {
		t.Error("Attributes[color] should be set")
	}
}

func TestUserToOAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	u := &model.User{
		ID:            1,
		Email:         "alice@example.com",
		Name:          "Alice",
		Administrator: true,
		Readonly:      false,
		Disabled:      false,
		CreatedAt:     now,
	}
	got := userToOAS(u)
	if got.ID != 1 || got.Email != "alice@example.com" {
		t.Errorf("unexpected user: %+v", got)
	}
	if !got.Administrator {
		t.Error("Administrator should be true")
	}
	if got.Attributes.Set {
		t.Error("Attributes should not be set when nil")
	}
}

func TestPositionAttrsToOAS(t *testing.T) {
	attrs := map[string]interface{}{
		"motion":     true,
		"ignition":   false,
		"flags":      "0x0001",
		"alarm":      "sos",
		"mcc":        float64(262),
		"mnc":        float64(1),
		"lac":        float64(100),
		"cellId":     float64(5000),
		"iccid":      "89490200000010001234",
		"satellites": float64(8),
		"custom":     "extra",
	}
	got := positionAttrsToOAS(attrs)
	if !got.Motion.Set || !got.Motion.Value {
		t.Errorf("Motion = %+v, want {Value:true Set:true}", got.Motion)
	}
	if !got.Ignition.Set || got.Ignition.Value {
		t.Errorf("Ignition = %+v, want {Value:false Set:true}", got.Ignition)
	}
	if !got.Flags.Set || got.Flags.Value != "0x0001" {
		t.Errorf("Flags = %+v, want 0x0001", got.Flags)
	}
	if !got.Alarm.Set || got.Alarm.Value != "sos" {
		t.Errorf("Alarm = %+v, want sos", got.Alarm)
	}
	if !got.Mcc.Set || got.Mcc.Value != 262 {
		t.Errorf("Mcc = %+v, want 262", got.Mcc)
	}
	if !got.Satellites.Set || got.Satellites.Value != 8 {
		t.Errorf("Satellites = %+v, want 8", got.Satellites)
	}
	if !got.Iccid.Set || got.Iccid.Value != "89490200000010001234" {
		t.Errorf("Iccid = %+v", got.Iccid)
	}
	if got.AdditionalProps["custom"] == nil {
		t.Error("extra key should appear in AdditionalProps")
	}
}

func TestPositionAttrsToOAS_Empty(t *testing.T) {
	got := positionAttrsToOAS(map[string]interface{}{})
	if got.Motion.Set || got.Ignition.Set || got.Satellites.Set {
		t.Error("empty attrs should produce zero-value struct")
	}
	if len(got.AdditionalProps) != 0 {
		t.Error("AdditionalProps should be empty")
	}
}

func TestPositionToOAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	p := &model.Position{
		ID:         10,
		DeviceID:   5,
		Timestamp:  now,
		Valid:      true,
		Latitude:   48.1,
		Longitude:  11.5,
		Altitude:   ptr(500.0),
		Speed:      ptr(30.0),
		Course:     ptr(90.0),
		Address:    ptr("Main St"),
		Accuracy:   5.0,
		Attributes: map[string]interface{}{},
	}
	got := positionToOAS(p)
	if got.Altitude != 500.0 {
		t.Errorf("Altitude = %f, want 500.0", got.Altitude)
	}
	if got.Address.Value != "Main St" {
		t.Errorf("Address = %v, want Main St", got.Address)
	}
}

func TestPositionToOAS_NilPointers(t *testing.T) {
	p := &model.Position{
		ID:         1,
		DeviceID:   2,
		Timestamp:  time.Now(),
		Attributes: map[string]interface{}{},
	}
	got := positionToOAS(p)
	if got.Altitude != 0 || got.Speed != 0 || got.Course != 0 {
		t.Error("nil pointers should produce zero values")
	}
	if got.Address.Set {
		t.Error("nil address should produce unset OptString")
	}
}

func TestSessionToOAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	s := &model.Session{
		ID:         "abcdef123456789xyz",
		UserID:     1,
		RememberMe: true,
		CreatedAt:  now,
		ExpiresAt:  now.Add(24 * time.Hour),
	}
	got := sessionToOAS(s)
	if got.ID == s.ID {
		t.Errorf("session ID should be truncated, got the full token %q", got.ID)
	}
	if len([]rune(got.ID)) > 13 {
		t.Errorf("session ID rune length should be <=13, got %q", got.ID)
	}
	if !got.IsCurrent.Set {
		t.Error("IsCurrent should be set")
	}
	if !got.OriginalUserId.Null {
		t.Error("OriginalUserId should be null when nil")
	}
}

func TestApiKeyToOAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	k := &model.ApiKey{
		ID:          3,
		UserID:      1,
		Token:       "secret-token",
		Name:        "My Key",
		Permissions: "full",
		CreatedAt:   now,
	}
	withToken := apiKeyToOAS(k, true)
	if !withToken.Token.Set || withToken.Token.Value != "secret-token" {
		t.Error("token should be included when includeToken=true")
	}

	withoutToken := apiKeyToOAS(k, false)
	if withoutToken.Token.Set {
		t.Error("token should not be included when includeToken=false")
	}
	if withoutToken.Permissions != oas.ApiKeyPermissionsFull {
		t.Errorf("Permissions = %v, want full", withoutToken.Permissions)
	}
}

func TestGeofenceToOAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	g := &model.Geofence{
		ID:          5,
		Name:        "Zone A",
		Description: "test zone",
		Area:        "POLYGON((0 0,1 0,1 1,0 1,0 0))",
		Geometry:    `{"type":"Polygon","coordinates":[]}`,
		CalendarID:  ptr(int64(2)),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	got := geofenceToOAS(g)
	if got.Description.Value != "test zone" {
		t.Errorf("Description = %v, want test zone", got.Description)
	}
	if !got.CalendarId.Set || got.CalendarId.Value != 2 {
		t.Errorf("CalendarId = %+v", got.CalendarId)
	}
}

func TestCommandToOAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	c := &model.Command{
		ID:         1,
		DeviceID:   5,
		Type:       "custom",
		Status:     "pending",
		CreatedAt:  now,
		Attributes: map[string]interface{}{"text": "AT+GPSON"},
	}
	got := commandToOAS(c)
	if got.Type != "custom" {
		t.Errorf("Type = %s", got.Type)
	}
	if !got.Attributes.Set {
		t.Error("Attributes should be set for custom command")
	}
	if !got.Attributes.Value.IsCommandAttrCustom() {
		t.Error("Attributes variant should be CommandAttrCustom")
	}
	if got.Attributes.Value.CommandAttrCustom.Text != "AT+GPSON" {
		t.Errorf("Text = %q, want AT+GPSON", got.Attributes.Value.CommandAttrCustom.Text)
	}
}

func TestCommandToOAS_NoAttributes(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	c := &model.Command{
		ID:        2,
		DeviceID:  5,
		Type:      "rebootDevice",
		Status:    "pending",
		CreatedAt: now,
	}
	got := commandToOAS(c)
	if got.Attributes.Set {
		t.Error("Attributes should not be set for type with no attrs")
	}
}

func TestEventToOAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	e := &model.Event{
		ID:         9,
		DeviceID:   3,
		Type:       "geofenceEnter",
		GeofenceID: ptr(int64(7)),
		Timestamp:  now,
	}
	got := eventToOAS(e)
	if got.Type != "geofenceEnter" {
		t.Errorf("Type = %s", got.Type)
	}
	if !got.GeofenceId.Set || got.GeofenceId.Value != 7 {
		t.Errorf("GeofenceId = %+v", got.GeofenceId)
	}
	if got.Attributes.Set {
		t.Error("geofenceEnter with nil attrs should have Attributes unset")
	}
}

func TestBuildEventAttributes_Ignition(t *testing.T) {
	attrs := map[string]interface{}{"ignition": true}
	got := buildEventAttributes("ignitionOn", attrs)
	if !got.Set {
		t.Fatal("expected Attributes to be set")
	}
	if !got.Value.IsEventAttrIgnition() {
		t.Fatal("expected EventAttrIgnition variant")
	}
	v, _ := got.Value.GetEventAttrIgnition()
	if !v.Ignition {
		t.Error("Ignition should be true")
	}
}

func TestBuildEventAttributes_Alarm(t *testing.T) {
	attrs := map[string]interface{}{"alarm": "sos"}
	got := buildEventAttributes("alarm", attrs)
	if !got.Set || !got.Value.IsEventAttrAlarm() {
		t.Fatal("expected EventAttrAlarm variant set")
	}
	v, _ := got.Value.GetEventAttrAlarm()
	if v.Alarm != "sos" {
		t.Errorf("Alarm = %s, want sos", v.Alarm)
	}
}

func TestBuildEventAttributes_Motion(t *testing.T) {
	attrs := map[string]interface{}{"speed": 60.0, "previousSpeed": 0.0}
	got := buildEventAttributes("motion", attrs)
	if !got.Set || !got.Value.IsEventAttrMotion() {
		t.Fatal("expected EventAttrMotion variant set")
	}
	v, _ := got.Value.GetEventAttrMotion()
	if v.Speed != 60.0 || v.PreviousSpeed != 0.0 {
		t.Errorf("speed=%v previousSpeed=%v", v.Speed, v.PreviousSpeed)
	}
}

func TestBuildEventAttributes_Trip(t *testing.T) {
	attrs := map[string]interface{}{"distance": 1234.5, "mileage": 50000.0}
	got := buildEventAttributes("tripCompleted", attrs)
	if !got.Set || !got.Value.IsEventAttrTrip() {
		t.Fatal("expected EventAttrTrip variant set")
	}
	v, _ := got.Value.GetEventAttrTrip()
	if v.Distance != 1234.5 || v.Mileage != 50000.0 {
		t.Errorf("distance=%v mileage=%v", v.Distance, v.Mileage)
	}
}

func TestBuildEventAttributes_Idle(t *testing.T) {
	attrs := map[string]interface{}{"idleDuration": 15.5}
	got := buildEventAttributes("deviceIdle", attrs)
	if !got.Set || !got.Value.IsEventAttrIdle() {
		t.Fatal("expected EventAttrIdle variant set")
	}
	v, _ := got.Value.GetEventAttrIdle()
	if v.IdleDuration != 15.5 {
		t.Errorf("idleDuration = %v", v.IdleDuration)
	}
}

func TestBuildEventAttributes_Unknown(t *testing.T) {
	got := buildEventAttributes("unknownType", map[string]interface{}{"foo": "bar"})
	if got.Set {
		t.Error("unknown event type should produce unset OptEventAttributes")
	}
}

func TestBuildEventAttributes_NilAttrs(t *testing.T) {
	got := buildEventAttributes("alarm", nil)
	if got.Set {
		t.Error("nil attrs should produce unset OptEventAttributes")
	}
}

func TestDeviceShareToOAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	s := &model.DeviceShare{
		ID:        1,
		DeviceID:  2,
		Token:     "tok",
		CreatedBy: 3,
		CreatedAt: now,
	}
	got := deviceShareToOAS(s)
	if got.Token != "tok" {
		t.Errorf("Token = %s", got.Token)
	}
	if !got.ExpiresAt.Null {
		t.Error("ExpiresAt should be null when nil")
	}
}

func TestAuditEntryToOAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	e := audit.Entry{
		ID:           1,
		Timestamp:    now,
		UserID:       ptr(int64(5)),
		Action:       "user.create",
		ResourceType: ptr("user"),
		ResourceID:   ptr(int64(10)),
		IPAddress:    ptr("192.168.1.1"),
	}
	got := auditEntryToOAS(e)
	if got.UserId != 5 {
		t.Errorf("UserId = %d, want 5", got.UserId)
	}
	if got.ResourceId.Value != "10" {
		t.Errorf("ResourceId = %s, want 10", got.ResourceId.Value)
	}
	if got.IpAddress.Value != "192.168.1.1" {
		t.Errorf("IpAddress = %v", got.IpAddress)
	}
	if got.CreatedAt != now {
		t.Errorf("CreatedAt mismatch")
	}
}

func TestAuditEntryToOAS_NilUserID(t *testing.T) {
	e := audit.Entry{
		ID:        2,
		Timestamp: time.Now(),
		Action:    "system.boot",
	}
	got := auditEntryToOAS(e)
	if got.UserId != 0 {
		t.Errorf("UserId should be 0 when nil, got %d", got.UserId)
	}
}

func TestNotificationRuleToOAS_Webhook(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	n := &model.NotificationRule{
		ID:         1,
		UserID:     2,
		Name:       "My Webhook",
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config: map[string]interface{}{
			"webhookUrl": "https://example.com/hook",
			"headers":    map[string]interface{}{"X-Token": "abc"},
		},
		Template:  "{{.Type}}",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	got := notificationRuleToOAS(n)
	if !got.Config.IsNotificationConfigWebhook() {
		t.Fatal("Config should be NotificationConfigWebhook variant")
	}
	wh := got.Config.NotificationConfigWebhook
	if wh.WebhookUrl.String() != "https://example.com/hook" {
		t.Errorf("WebhookUrl = %s, want https://example.com/hook", wh.WebhookUrl.String())
	}
	if !wh.Headers.Set || wh.Headers.Value["X-Token"] != "abc" {
		t.Errorf("Headers = %+v, want {X-Token: abc}", wh.Headers)
	}
}

func TestOasNotificationConfigToModel_Webhook(t *testing.T) {
	u, _ := url.Parse("https://example.com/hook")
	var config oas.NotificationRuleConfig
	config.SetNotificationConfigWebhook(oas.NotificationConfigWebhook{
		Channel:    oas.NotificationConfigWebhookChannelWebhook,
		WebhookUrl: *u,
	})
	cfg, err := oasNotificationConfigToModel(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg["webhookUrl"] != "https://example.com/hook" {
		t.Errorf("webhookUrl = %v, want https://example.com/hook", cfg["webhookUrl"])
	}
}

func TestOasNotificationConfigToModel_Invalid(t *testing.T) {
	var config oas.NotificationRuleConfig
	_, err := oasNotificationConfigToModel(config)
	if err == nil {
		t.Error("expected error for zero-value config")
	}
}

// --- Stage 6: buildAuditMetadata tests ---

func TestBuildAuditMetadata_Empty(t *testing.T) {
	for _, action := range []string{
		"session.logout", "device.delete", "geofence.delete",
		"calendar.delete", "notification.delete", "user.delete",
		"device.online", "device.offline",
	} {
		got := buildAuditMetadata(action, nil)
		if !got.Set {
			t.Errorf("action %s: expected Set=true", action)
			continue
		}
		if !got.Value.IsAuditMetaEmpty() {
			t.Errorf("action %s: expected IsAuditMetaEmpty", action)
			continue
		}
		v, _ := got.Value.GetAuditMetaEmpty()
		if v.Action != action {
			t.Errorf("action %s: Action field = %s", action, v.Action)
		}
	}
}

func TestBuildAuditMetadata_Unknown(t *testing.T) {
	got := buildAuditMetadata("not.a.real.action", nil)
	if got.Set {
		t.Error("expected Set=false for unknown action")
	}
}

func TestBuildAuditMetadata_SessionLogin(t *testing.T) {
	got := buildAuditMetadata("session.login", map[string]interface{}{"email": "user@example.com"})
	if !got.Set || !got.Value.IsAuditMetaSessionLogin() {
		t.Fatal("expected AuditMetaSessionLogin variant")
	}
	v, _ := got.Value.GetAuditMetaSessionLogin()
	if v.Email != "user@example.com" {
		t.Errorf("Email = %s, want user@example.com", v.Email)
	}
}

func TestBuildAuditMetadata_SessionLoginFailed(t *testing.T) {
	got := buildAuditMetadata("session.login_failed", map[string]interface{}{
		"email": "bad@example.com", "reason": "wrong_password",
	})
	if !got.Set || !got.Value.IsAuditMetaSessionLoginFailed() {
		t.Fatal("expected AuditMetaSessionLoginFailed variant")
	}
	v, _ := got.Value.GetAuditMetaSessionLoginFailed()
	if v.Email != "bad@example.com" || v.Reason != "wrong_password" {
		t.Errorf("got Email=%s Reason=%s, want bad@example.com/wrong_password", v.Email, v.Reason)
	}
}

func TestBuildAuditMetadata_SessionSudo(t *testing.T) {
	for _, action := range []string{"session.sudo", "session.sudo_end"} {
		got := buildAuditMetadata(action, map[string]interface{}{
			"adminEmail": "admin@example.com", "targetEmail": "target@example.com",
		})
		if !got.Set || !got.Value.IsAuditMetaSessionSudo() {
			t.Fatalf("action %s: expected AuditMetaSessionSudo variant", action)
		}
		v, _ := got.Value.GetAuditMetaSessionSudo()
		if v.AdminEmail != "admin@example.com" || v.TargetEmail != "target@example.com" {
			t.Errorf("action %s: got AdminEmail=%s TargetEmail=%s", action, v.AdminEmail, v.TargetEmail)
		}
	}
}

func TestBuildAuditMetadata_SessionRevoke_Bulk(t *testing.T) {
	got := buildAuditMetadata("session.revoke", map[string]interface{}{"scope": "all_other_sessions"})
	if !got.Set || !got.Value.IsAuditMetaSessionRevoke() {
		t.Fatal("expected AuditMetaSessionRevoke variant")
	}
	v, _ := got.Value.GetAuditMetaSessionRevoke()
	if !v.Scope.Set || v.Scope.Value != "all_other_sessions" {
		t.Errorf("Scope = %v, want all_other_sessions", v.Scope)
	}
	if v.RevokedSessionId.Set {
		t.Error("RevokedSessionId should not be set for bulk revoke")
	}
}

func TestBuildAuditMetadata_SessionRevoke_Specific(t *testing.T) {
	got := buildAuditMetadata("session.revoke", map[string]interface{}{
		"revokedSessionId": "abc123", "sessionOwnerUserId": float64(42),
	})
	if !got.Set || !got.Value.IsAuditMetaSessionRevoke() {
		t.Fatal("expected AuditMetaSessionRevoke variant")
	}
	v, _ := got.Value.GetAuditMetaSessionRevoke()
	if !v.RevokedSessionId.Set || v.RevokedSessionId.Value != "abc123" {
		t.Errorf("RevokedSessionId = %v, want abc123", v.RevokedSessionId)
	}
	if !v.SessionOwnerUserId.Set || v.SessionOwnerUserId.Value != 42 {
		t.Errorf("SessionOwnerUserId = %v, want 42", v.SessionOwnerUserId)
	}
}

func TestBuildAuditMetadata_UserCreate(t *testing.T) {
	got := buildAuditMetadata("user.create", map[string]interface{}{"email": "new@example.com", "role": "user"})
	if !got.Set || !got.Value.IsAuditMetaUserCreate() {
		t.Fatal("expected AuditMetaUserCreate variant")
	}
	v, _ := got.Value.GetAuditMetaUserCreate()
	if v.Email != "new@example.com" || v.Role != "user" {
		t.Errorf("got Email=%s Role=%s", v.Email, v.Role)
	}
}

func TestBuildAuditMetadata_UserUpdate(t *testing.T) {
	got := buildAuditMetadata("user.update", map[string]interface{}{
		"email": "user@example.com", "oldEmail": "old@example.com", "newEmail": "new@example.com",
		"oldRole": "user", "newRole": "admin",
	})
	if !got.Set || !got.Value.IsAuditMetaUserUpdate() {
		t.Fatal("expected AuditMetaUserUpdate variant")
	}
	v, _ := got.Value.GetAuditMetaUserUpdate()
	if v.Email != "user@example.com" {
		t.Errorf("Email = %s", v.Email)
	}
	if !v.OldEmail.Set || v.OldEmail.Value != "old@example.com" {
		t.Errorf("OldEmail = %v", v.OldEmail)
	}
	if !v.NewRole.Set || v.NewRole.Value != "admin" {
		t.Errorf("NewRole = %v", v.NewRole)
	}
}

func TestBuildAuditMetadata_DeviceCreate(t *testing.T) {
	got := buildAuditMetadata("device.create", map[string]interface{}{"name": "Truck 1", "uniqueId": "IMEI123"})
	if !got.Set || !got.Value.IsAuditMetaDeviceCreate() {
		t.Fatal("expected AuditMetaDeviceCreate variant")
	}
	v, _ := got.Value.GetAuditMetaDeviceCreate()
	if v.Name != "Truck 1" || v.UniqueId != "IMEI123" {
		t.Errorf("got Name=%s UniqueId=%s", v.Name, v.UniqueId)
	}
}

func TestBuildAuditMetadata_DeviceAssign_UserId(t *testing.T) {
	got := buildAuditMetadata("device.assign", map[string]interface{}{"userId": float64(7)})
	if !got.Set || !got.Value.IsAuditMetaDeviceAssign() {
		t.Fatal("expected AuditMetaDeviceAssign variant")
	}
	v, _ := got.Value.GetAuditMetaDeviceAssign()
	if v.UserId != 7 {
		t.Errorf("UserId = %d, want 7", v.UserId)
	}
}

func TestBuildAuditMetadata_DeviceUnassign_TargetUserId(t *testing.T) {
	// Fallback key "targetUserId" used by the old handler.
	got := buildAuditMetadata("device.unassign", map[string]interface{}{"targetUserId": float64(9)})
	if !got.Set || !got.Value.IsAuditMetaDeviceAssign() {
		t.Fatal("expected AuditMetaDeviceAssign variant")
	}
	v, _ := got.Value.GetAuditMetaDeviceAssign()
	if v.UserId != 9 {
		t.Errorf("UserId = %d (via targetUserId fallback), want 9", v.UserId)
	}
}

func TestBuildAuditMetadata_GpxImport(t *testing.T) {
	got := buildAuditMetadata("device.gpx_import", map[string]interface{}{"deviceId": float64(3), "positions": float64(150)})
	if !got.Set || !got.Value.IsAuditMetaDeviceGpxImport() {
		t.Fatal("expected AuditMetaDeviceGpxImport variant")
	}
	v, _ := got.Value.GetAuditMetaDeviceGpxImport()
	if v.DeviceId != 3 || v.Positions != 150 {
		t.Errorf("got DeviceId=%d Positions=%d", v.DeviceId, v.Positions)
	}
}

func TestBuildAuditMetadata_NamedResource(t *testing.T) {
	for _, action := range []string{"geofence.create", "geofence.update", "calendar.create", "calendar.update"} {
		got := buildAuditMetadata(action, map[string]interface{}{"name": "Home Zone"})
		if !got.Set || !got.Value.IsAuditMetaNamedResource() {
			t.Errorf("action %s: expected AuditMetaNamedResource variant", action)
			continue
		}
		v, _ := got.Value.GetAuditMetaNamedResource()
		if v.Name != "Home Zone" {
			t.Errorf("action %s: Name = %s", action, v.Name)
		}
	}
}

func TestBuildAuditMetadata_NotificationRule(t *testing.T) {
	details := map[string]interface{}{
		"name": "Speed Alert", "eventTypes": []interface{}{"geofenceEnter", "alarm"}, "channel": "webhook",
	}
	got := buildAuditMetadata("notification.create", details)
	if !got.Set || !got.Value.IsAuditMetaNotificationRule() {
		t.Fatal("expected AuditMetaNotificationRule variant")
	}
	v, _ := got.Value.GetAuditMetaNotificationRule()
	if v.Name != "Speed Alert" || v.Channel != "webhook" {
		t.Errorf("got Name=%s Channel=%s", v.Name, v.Channel)
	}
	if len(v.EventTypes) != 2 || v.EventTypes[0] != "geofenceEnter" {
		t.Errorf("EventTypes = %v", v.EventTypes)
	}
}

func TestBuildAuditMetadata_NotifDelivery_Sent(t *testing.T) {
	details := map[string]interface{}{
		"ruleName": "Speed Alert", "eventType": "alarm", "channel": "webhook",
		"deviceId": float64(5), "responseCode": float64(200),
	}
	got := buildAuditMetadata("notification.sent", details)
	if !got.Set || !got.Value.IsAuditMetaNotifDelivery() {
		t.Fatal("expected AuditMetaNotifDelivery variant")
	}
	v, _ := got.Value.GetAuditMetaNotifDelivery()
	if v.RuleName != "Speed Alert" || v.DeviceId != 5 {
		t.Errorf("got RuleName=%s DeviceId=%d", v.RuleName, v.DeviceId)
	}
	if !v.ResponseCode.Set || v.ResponseCode.Value != 200 {
		t.Errorf("ResponseCode = %v, want 200", v.ResponseCode)
	}
}

func TestBuildAuditMetadata_NotifDelivery_Failed(t *testing.T) {
	details := map[string]interface{}{
		"ruleName": "Speed Alert", "eventType": "alarm", "channel": "webhook",
		"deviceId": float64(5), "error": "connection refused",
	}
	got := buildAuditMetadata("notification.failed", details)
	if !got.Set || !got.Value.IsAuditMetaNotifDelivery() {
		t.Fatal("expected AuditMetaNotifDelivery variant")
	}
	v, _ := got.Value.GetAuditMetaNotifDelivery()
	if !v.Error.Set || v.Error.Value != "connection refused" {
		t.Errorf("Error = %v, want 'connection refused'", v.Error)
	}
}

func TestBuildAuditMetadata_ApiKeyCreate(t *testing.T) {
	got := buildAuditMetadata("apikey.create", map[string]interface{}{"name": "CI Key", "permissions": "read"})
	if !got.Set || !got.Value.IsAuditMetaApiKeyCreate() {
		t.Fatal("expected AuditMetaApiKeyCreate variant")
	}
	v, _ := got.Value.GetAuditMetaApiKeyCreate()
	if v.Name != "CI Key" || v.Permissions != "read" {
		t.Errorf("got Name=%s Permissions=%s", v.Name, v.Permissions)
	}
}

func TestBuildAuditMetadata_ApiKeyDelete(t *testing.T) {
	got := buildAuditMetadata("apikey.delete", map[string]interface{}{"name": "Old Key", "keyOwnerUserId": float64(11)})
	if !got.Set || !got.Value.IsAuditMetaApiKeyDelete() {
		t.Fatal("expected AuditMetaApiKeyDelete variant")
	}
	v, _ := got.Value.GetAuditMetaApiKeyDelete()
	if v.Name != "Old Key" || v.KeyOwnerUserId != 11 {
		t.Errorf("got Name=%s KeyOwnerUserId=%d", v.Name, v.KeyOwnerUserId)
	}
}

func TestBuildAuditMetadata_Share(t *testing.T) {
	for _, action := range []string{"share.create", "share.delete"} {
		got := buildAuditMetadata(action, map[string]interface{}{"deviceId": float64(8)})
		if !got.Set || !got.Value.IsAuditMetaShare() {
			t.Errorf("action %s: expected AuditMetaShare variant", action)
			continue
		}
		v, _ := got.Value.GetAuditMetaShare()
		if v.DeviceId != 8 {
			t.Errorf("action %s: DeviceId = %d", action, v.DeviceId)
		}
	}
}

func TestBuildAuditMetadata_CommandSend(t *testing.T) {
	got := buildAuditMetadata("command.send", map[string]interface{}{
		"commandType": "custom", "commandStatus": "sent", "deviceName": "Truck 1",
	})
	if !got.Set || !got.Value.IsAuditMetaCommandSend() {
		t.Fatal("expected AuditMetaCommandSend variant")
	}
	v, _ := got.Value.GetAuditMetaCommandSend()
	if v.CommandType != "custom" || v.CommandStatus != "sent" || v.DeviceName != "Truck 1" {
		t.Errorf("got CommandType=%s CommandStatus=%s DeviceName=%s", v.CommandType, v.CommandStatus, v.DeviceName)
	}
}

func TestBuildAuditMetadata_AuditEntryToOAS(t *testing.T) {
	uid := int64(1)
	resType := "session"
	resID := int64(99)
	ip := "10.0.0.1"
	now := time.Now().Truncate(time.Second)
	e := audit.Entry{
		ID:           42,
		UserID:       &uid,
		Action:       "session.login",
		ResourceType: &resType,
		ResourceID:   &resID,
		Details:      map[string]interface{}{"email": "user@example.com"},
		IPAddress:    &ip,
		Timestamp:    now,
	}
	got := auditEntryToOAS(e)
	if got.ID != 42 || got.UserId != 1 {
		t.Errorf("ID=%d UserId=%d", got.ID, got.UserId)
	}
	if !got.Metadata.Set || !got.Metadata.Value.IsAuditMetaSessionLogin() {
		t.Error("expected AuditMetaSessionLogin in Metadata")
	}
	v, _ := got.Metadata.Value.GetAuditMetaSessionLogin()
	if v.Email != "user@example.com" {
		t.Errorf("Email = %s", v.Email)
	}
}
