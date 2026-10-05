package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// --- sortDevices ---

func TestSortDevices(t *testing.T) {
	devices := []model.Device{
		{ID: 3, Name: "Charlie", UniqueID: "UID-C", Status: "online", Protocol: "watch"},
		{ID: 1, Name: "Alpha", UniqueID: "UID-A", Status: "offline", Protocol: "h02"},
		{ID: 2, Name: "Bravo", UniqueID: "UID-B", Status: "online", Protocol: "h02"},
	}

	tests := []struct {
		field   string
		wantIDs []int64
	}{
		{"id", []int64{1, 2, 3}},
		{"", []int64{1, 2, 3}},          // default → by ID
		{"unknown", []int64{1, 2, 3}},   // unrecognized → by ID
		{"name", []int64{1, 2, 3}},      // Alpha, Bravo, Charlie
		{"unique-id", []int64{1, 2, 3}}, // UID-A, UID-B, UID-C
		{"uniqueid", []int64{1, 2, 3}},  // alias
		{"status", []int64{1, 3, 2}},    // offline(1) first; online order is non-stable: Charlie(3), Bravo(2)
		{"protocol", []int64{1, 2, 3}},  // h02, h02, watch
	}

	for _, tt := range tests {
		d := make([]model.Device, len(devices))
		copy(d, devices)
		sortList(d, tt.field, deviceSorts)
		for i, want := range tt.wantIDs {
			if d[i].ID != want {
				t.Errorf("sortDevices(%q)[%d]: got ID %d, want %d", tt.field, i, d[i].ID, want)
			}
		}
	}
}

// --- sortUsers ---

func TestSortUsers(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	users := []*model.User{
		{ID: 3, Email: "charlie@test.com", Name: "Charlie", Role: "user", CreatedAt: t3},
		{ID: 1, Email: "alice@test.com", Name: "Alice", Role: "admin", CreatedAt: t1},
		{ID: 2, Email: "bob@test.com", Name: "Bob", Role: "user", CreatedAt: t2},
	}

	tests := []struct {
		field   string
		wantIDs []int64
	}{
		{"id", []int64{1, 2, 3}},
		{"", []int64{1, 2, 3}},        // default
		{"unknown", []int64{1, 2, 3}}, // unrecognized → by ID
		{"email", []int64{1, 2, 3}},   // alice < bob < charlie
		{"name", []int64{1, 2, 3}},    // Alice < Bob < Charlie
		{"role", []int64{1, 3, 2}},    // admin(1) first; user order is non-stable: Charlie(3), Bob(2)
		{"created", []int64{1, 2, 3}}, // t1 < t2 < t3
	}

	for _, tt := range tests {
		u := make([]*model.User, len(users))
		copy(u, users)
		sortList(u, tt.field, userSorts)
		for i, want := range tt.wantIDs {
			if u[i].ID != want {
				t.Errorf("sortUsers(%q)[%d]: got ID %d, want %d", tt.field, i, u[i].ID, want)
			}
		}
	}
}

// --- printTableTo ---

func TestPrintTableTo(t *testing.T) {
	var buf bytes.Buffer
	printTableTo(&buf, []string{"ID", "NAME", "STATUS"}, [][]string{{"1", "Alpha", "online"}, {"2", "Bravo", "offline"}})

	want := "ID  NAME   STATUS\n1   Alpha  online\n2   Bravo  offline\n"
	if got := buf.String(); got != want {
		t.Errorf("printTableTo = %q, want %q", got, want)
	}
}

// --- newVersionCmd ---

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd()
	if cmd.Use != "version" {
		t.Errorf("Use = %q, want %q", cmd.Use, "version")
	}
	if cmd.Run == nil {
		t.Error("Run should be set")
	}

	// Execute and capture output.
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.Run(cmd, nil)
	// Version output goes to fmt.Printf (stdout), not cmd.SetOut — just verify no panic.
}

// --- command constructors ---

func TestNewDBMigrateCmd(t *testing.T) {
	cmd := newDBMigrateCmd()
	if cmd.Use != "db-migrate [command]" {
		t.Errorf("Use = %q", cmd.Use)
	}
	if cmd.Flags().Lookup("db-url") == nil {
		t.Error("flag 'db-url' not registered")
	}
}

func TestNewWaitForDBCmd(t *testing.T) {
	cmd := newWaitForDBCmd()
	if cmd.Use != "wait-for-db" {
		t.Errorf("Use = %q", cmd.Use)
	}
	for _, flag := range []string{"db-url", "timeout", "interval"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag %q not registered", flag)
		}
	}
}

func TestNewUserCmd(t *testing.T) {
	cmd := newUserCmd()
	if cmd.Use != "user" {
		t.Errorf("Use = %q", cmd.Use)
	}
}

func TestUserAdd_RejectsInvalidInput(t *testing.T) {
	origFatal, origConnect := fatalFn, connectDBFn
	defer func() { fatalFn, connectDBFn = origFatal, origConnect }()
	connectDBFn = func() (*pgxpool.Pool, error) {
		t.Fatal("database must not be reached for invalid input")
		return nil, nil
	}

	for _, tc := range []struct{ email, password string }{
		{"not-an-email", "Password1!"},
		{"ok@example.com", "short"},
	} {
		var fatalCalled bool
		fatalFn = func(msg string, args ...any) { fatalCalled = true }
		cmd := newUserAddCmd()
		_ = cmd.Flags().Set("email", tc.email)
		_ = cmd.Flags().Set("name", "Name")
		_ = cmd.Flags().Set("password", tc.password)
		cmd.Run(cmd, nil)
		if !fatalCalled {
			t.Errorf("email=%q password=%q: expected rejection", tc.email, tc.password)
		}
	}
}

func TestNewDeviceCmd(t *testing.T) {
	cmd := newDeviceCmd()
	if cmd.Use != "device" {
		t.Errorf("Use = %q", cmd.Use)
	}
}

// --- filterDevices ---

func TestFilterDevices(t *testing.T) {
	devices := []model.Device{
		{ID: 1, Name: "Car Alpha", UniqueID: "ALPHA-001", Status: "online", Protocol: "h02"},
		{ID: 2, Name: "Bike Bravo", UniqueID: "BRAVO-001", Status: "offline", Protocol: "watch"},
		{ID: 3, Name: "Truck Charlie", UniqueID: "CHARLIE-001", Status: "online", Protocol: "h02"},
	}

	tests := []struct {
		filter  string
		wantIDs []int64
	}{
		{"status=online", []int64{1, 3}},
		{"status=offline", []int64{2}},
		{"protocol=h02", []int64{1, 3}},
		{"protocol=watch", []int64{2}},
		{"name=bike", []int64{2}},
		{"name=alpha", []int64{1}},
		{"unique-id=alpha", []int64{1}},
		{"uniqueid=bravo", []int64{2}},
		{"status=unknown", nil}, // no match
	}

	for _, tt := range tests {
		got := filterList(devices, tt.filter, deviceFilters)
		if len(got) != len(tt.wantIDs) {
			t.Errorf("filterDevices(%q): got %d results, want %d", tt.filter, len(got), len(tt.wantIDs))
			continue
		}
		for i, want := range tt.wantIDs {
			if got[i].ID != want {
				t.Errorf("filterDevices(%q)[%d]: got ID %d, want %d", tt.filter, i, got[i].ID, want)
			}
		}
	}
}

func TestFilterDevices_BadFormat(t *testing.T) {
	orig := fatalFn
	var fatalCalled bool
	fatalFn = func(msg string, args ...any) { fatalCalled = true }
	defer func() { fatalFn = orig }()

	filterList([]model.Device{{ID: 1}}, "no-equals-sign", deviceFilters)
	if !fatalCalled {
		t.Error("expected fatalFn to be called for bad filter format")
	}
}

func TestFilterDevices_UnknownField(t *testing.T) {
	orig := fatalFn
	var fatalCalled bool
	fatalFn = func(msg string, args ...any) { fatalCalled = true }
	defer func() { fatalFn = orig }()

	filterList([]model.Device{{ID: 1, Name: "x"}}, "unknownfield=x", deviceFilters)
	if !fatalCalled {
		t.Error("expected fatalFn to be called for unknown filter field")
	}
}

// --- filterUsers ---

func TestFilterUsers(t *testing.T) {
	users := []*model.User{
		{ID: 1, Email: "alice@example.com", Name: "Alice Smith", Role: "admin"},
		{ID: 2, Email: "bob@example.com", Name: "Bob Jones", Role: "user"},
		{ID: 3, Email: "charlie@example.com", Name: "Charlie Brown", Role: "user"},
	}

	tests := []struct {
		filter  string
		wantIDs []int64
	}{
		{"role=admin", []int64{1}},
		{"role=user", []int64{2, 3}},
		{"email=alice", []int64{1}},
		{"email=example", []int64{1, 2, 3}},
		{"name=bob", []int64{2}},
		{"role=unknown", nil},
	}

	for _, tt := range tests {
		got := filterList(users, tt.filter, userFilters)
		if len(got) != len(tt.wantIDs) {
			t.Errorf("filterUsers(%q): got %d results, want %d", tt.filter, len(got), len(tt.wantIDs))
			continue
		}
		for i, want := range tt.wantIDs {
			if got[i].ID != want {
				t.Errorf("filterUsers(%q)[%d]: got ID %d, want %d", tt.filter, i, got[i].ID, want)
			}
		}
	}
}

func TestFilterUsers_BadFormat(t *testing.T) {
	orig := fatalFn
	var fatalCalled bool
	fatalFn = func(msg string, args ...any) { fatalCalled = true }
	defer func() { fatalFn = orig }()

	filterList([]*model.User{{ID: 1}}, "noequalssign", userFilters)
	if !fatalCalled {
		t.Error("expected fatalFn to be called for bad filter format")
	}
}

func TestFilterUsers_UnknownField(t *testing.T) {
	orig := fatalFn
	var fatalCalled bool
	fatalFn = func(msg string, args ...any) { fatalCalled = true }
	defer func() { fatalFn = orig }()

	filterList([]*model.User{{ID: 1, Email: "x@x.com"}}, "unknownfield=x", userFilters)
	if !fatalCalled {
		t.Error("expected fatalFn to be called for unknown filter field")
	}
}

// --- printJSONTo / printCSVTo ---

func TestPrintJSONTo(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}
	printJSONTo(&buf, data)
	out := buf.String()
	if !strings.Contains(out, `"key"`) || !strings.Contains(out, `"value"`) {
		t.Errorf("printJSONTo output missing expected content: %q", out)
	}
}

func TestPrintCSVTo(t *testing.T) {
	var buf bytes.Buffer
	printCSVTo(&buf, []string{"ID", "Name"}, [][]string{{"1", "Alice"}, {"2", "Bob"}})
	out := buf.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "Name") {
		t.Errorf("printCSVTo output missing headers: %q", out)
	}
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") {
		t.Errorf("printCSVTo output missing rows: %q", out)
	}
}

func TestPrintCSVTo_Empty(t *testing.T) {
	var buf bytes.Buffer
	printCSVTo(&buf, []string{"ID"}, nil)
	out := buf.String()
	if !strings.Contains(out, "ID") {
		t.Errorf("printCSVTo empty rows: output missing header: %q", out)
	}
}

// --- deviceOwnerEmail ---

// setValidConfigEnv sets the env vars config.LoadFromEnv requires.
func setValidConfigEnv(t *testing.T) {
	t.Setenv("MOTUS_ENV", "development")
	t.Setenv("MOTUS_DATABASE_PASSWORD", "test")
}

func TestDeviceOwnerEmail(t *testing.T) {
	setValidConfigEnv(t)
	t.Setenv("MOTUS_DEVICE_AUTO_CREATE_USER", "")
	if got := deviceOwnerEmail(""); got != "admin@motus.local" {
		t.Errorf("default: got %q, want admin@motus.local", got)
	}

	t.Setenv("MOTUS_DEVICE_AUTO_CREATE_USER", "fleet@example.com")
	if got := deviceOwnerEmail(""); got != "fleet@example.com" {
		t.Errorf("env: got %q, want fleet@example.com", got)
	}
	if got := deviceOwnerEmail("owner@example.com"); got != "owner@example.com" {
		t.Errorf("flag: got %q, want owner@example.com", got)
	}
}
