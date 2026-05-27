package handlers

import (
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
		Type:       "rebootDevice",
		Status:     "pending",
		CreatedAt:  now,
		Attributes: map[string]interface{}{"param": "val"},
	}
	got := commandToOAS(c)
	if got.Type != "rebootDevice" {
		t.Errorf("Type = %s", got.Type)
	}
	if !got.Attributes.Set {
		t.Error("Attributes should be set")
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
