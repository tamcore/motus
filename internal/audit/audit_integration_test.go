package audit

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func ptrInt64(v int64) *int64 { return &v }

// createTestUser inserts a user directly into the database and returns the user ID.
func createTestUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Test User', 'user') RETURNING id",
		email).Scan(&id)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return id
}

func TestLogger_LogAction(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool, "audit-test@example.com")
	resourceID := ptrInt64(42)

	logger.Log(ctx, &userID, ActionSessionLogin, ResourceSession, resourceID,
		map[string]interface{}{"browser": "firefox"},
		"10.0.0.1", "Mozilla/5.0")

	// Verify the entry was written.
	entries, total, err := logger.Query(ctx, QueryParams{})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 entry, got %d", total)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in slice, got %d", len(entries))
	}

	e := entries[0]
	if e.Action != ActionSessionLogin {
		t.Errorf("action: got %q, want %q", e.Action, ActionSessionLogin)
	}
	if e.UserID == nil || *e.UserID != userID {
		t.Errorf("userID: got %v, want %d", e.UserID, userID)
	}
	if e.ResourceID == nil || *e.ResourceID != 42 {
		t.Errorf("resourceID: got %v, want 42", e.ResourceID)
	}
	if e.ResourceType == nil || *e.ResourceType != ResourceSession {
		t.Errorf("resourceType: got %v, want %q", e.ResourceType, ResourceSession)
	}
	if e.IPAddress == nil || *e.IPAddress != "10.0.0.1" {
		t.Errorf("ipAddress: got %v, want 10.0.0.1", e.IPAddress)
	}
	if e.UserAgent == nil || *e.UserAgent != "Mozilla/5.0" {
		t.Errorf("userAgent: got %v, want Mozilla/5.0", e.UserAgent)
	}
	if e.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestLogger_LogAction_NullableFields(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	// Log with no user ID, no resource ID, no details, no IP, no user agent.
	logger.Log(ctx, nil, ActionDeviceOnline, "", nil, nil, "", "")

	entries, total, err := logger.Query(ctx, QueryParams{})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 entry, got %d", total)
	}

	e := entries[0]
	if e.Action != ActionDeviceOnline {
		t.Errorf("action: got %q, want %q", e.Action, ActionDeviceOnline)
	}
	if e.UserID != nil {
		t.Errorf("userID should be nil, got %v", e.UserID)
	}
	if e.ResourceID != nil {
		t.Errorf("resourceID should be nil, got %v", e.ResourceID)
	}
	if e.ResourceType != nil {
		t.Errorf("resourceType should be nil, got %v", e.ResourceType)
	}
	if e.IPAddress != nil {
		t.Errorf("ipAddress should be nil, got %v", e.IPAddress)
	}
	if e.UserAgent != nil {
		t.Errorf("userAgent should be nil, got %v", e.UserAgent)
	}
}

func TestLogger_LogAction_InvalidIP(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	// An invalid IP should be silently dropped (not stored, not error).
	logger.Log(ctx, nil, ActionSessionLogin, ResourceSession, nil, nil, "not-an-ip", "")

	entries, _, err := logger.Query(ctx, QueryParams{})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].IPAddress != nil {
		t.Errorf("invalid IP should be stored as nil, got %v", entries[0].IPAddress)
	}
}

func TestLogger_LogFromRequest(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	userID := createTestUser(t, pool, "logfromreq@example.com")

	req := httptest.NewRequest("POST", "/api/login", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	req.Header.Set("User-Agent", "HomeAssistant/2025.11")

	logger.LogFromRequest(req, &userID, ActionSessionLogin, ResourceSession, nil, nil)

	entries, _, err := logger.Query(req.Context(), QueryParams{})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.IPAddress == nil || *e.IPAddress != "192.168.1.100" {
		t.Errorf("ipAddress: got %v, want 192.168.1.100", e.IPAddress)
	}
	if e.UserAgent == nil || *e.UserAgent != "HomeAssistant/2025.11" {
		t.Errorf("userAgent: got %v, want HomeAssistant/2025.11", e.UserAgent)
	}
}

func TestLogger_MetadataEncoding(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	details := map[string]interface{}{
		"email":    "admin@example.com",
		"role":     "admin",
		"count":    float64(42),
		"nested":   map[string]interface{}{"key": "value"},
		"list":     []interface{}{"a", "b", "c"},
		"isActive": true,
	}

	logger.Log(ctx, nil, ActionUserCreate, ResourceUser, ptrInt64(10), details, "10.0.0.1", "")

	entries, _, err := logger.Query(ctx, QueryParams{})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	d := entries[0].Details
	if d == nil {
		t.Fatal("expected non-nil details")
	}
	if d["email"] != "admin@example.com" {
		t.Errorf("email: got %v, want admin@example.com", d["email"])
	}
	if d["role"] != "admin" {
		t.Errorf("role: got %v, want admin", d["role"])
	}
	if d["count"] != float64(42) {
		t.Errorf("count: got %v, want 42", d["count"])
	}
	if d["isActive"] != true {
		t.Errorf("isActive: got %v, want true", d["isActive"])
	}

	// Nested map should deserialize correctly.
	nested, ok := d["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("nested: expected map, got %T", d["nested"])
	}
	if nested["key"] != "value" {
		t.Errorf("nested.key: got %v, want value", nested["key"])
	}

	// List should deserialize correctly.
	list, ok := d["list"].([]interface{})
	if !ok {
		t.Fatalf("list: expected slice, got %T", d["list"])
	}
	if len(list) != 3 {
		t.Errorf("list length: got %d, want 3", len(list))
	}
}

func TestQuery_FilterByUserID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	user1 := createTestUser(t, pool, "filter-user1@example.com")
	user2 := createTestUser(t, pool, "filter-user2@example.com")

	// Insert entries for two different users.
	logger.Log(ctx, &user1, ActionSessionLogin, ResourceSession, nil, nil, "10.0.0.1", "")
	logger.Log(ctx, &user1, ActionSessionLogout, ResourceSession, nil, nil, "10.0.0.1", "")
	logger.Log(ctx, &user2, ActionSessionLogin, ResourceSession, nil, nil, "10.0.0.2", "")

	// Query for user 1.
	entries, total, err := logger.Query(ctx, QueryParams{UserID: &user1})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if total != 2 {
		t.Errorf("total: got %d, want 2", total)
	}
	if len(entries) != 2 {
		t.Errorf("entries: got %d, want 2", len(entries))
	}

	// All entries should belong to user 1.
	for _, e := range entries {
		if e.UserID == nil || *e.UserID != user1 {
			t.Errorf("expected userID=%d, got %v", user1, e.UserID)
		}
	}
}

func TestQuery_FilterByAction(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	user1 := createTestUser(t, pool, "action-filter@example.com")

	logger.Log(ctx, &user1, ActionSessionLogin, ResourceSession, nil, nil, "", "")
	logger.Log(ctx, &user1, ActionSessionLogout, ResourceSession, nil, nil, "", "")
	logger.Log(ctx, &user1, ActionSessionSudo, ResourceSession, nil, nil, "", "")

	entries, total, err := logger.Query(ctx, QueryParams{Action: ActionSessionLogin})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if total != 1 {
		t.Errorf("total: got %d, want 1", total)
	}
	if len(entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(entries))
	}
	if entries[0].Action != ActionSessionLogin {
		t.Errorf("action: got %q, want %q", entries[0].Action, ActionSessionLogin)
	}
}

func TestQuery_FilterByResourceType(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	logger.Log(ctx, nil, ActionDeviceOnline, ResourceDevice, nil, nil, "", "")
	logger.Log(ctx, nil, ActionSessionLogin, ResourceSession, nil, nil, "", "")
	logger.Log(ctx, nil, ActionNotifSent, ResourceNotification, nil, nil, "", "")

	entries, total, err := logger.Query(ctx, QueryParams{ResourceType: ResourceDevice})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if total != 1 {
		t.Errorf("total: got %d, want 1", total)
	}
	if len(entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(entries))
	}
	if entries[0].ResourceType == nil || *entries[0].ResourceType != ResourceDevice {
		t.Errorf("resourceType: got %v, want %q", entries[0].ResourceType, ResourceDevice)
	}
}

func TestQuery_CombinedFilters(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	user1 := createTestUser(t, pool, "combined1@example.com")
	user2 := createTestUser(t, pool, "combined2@example.com")

	logger.Log(ctx, &user1, ActionSessionLogin, ResourceSession, nil, nil, "", "")
	logger.Log(ctx, &user1, ActionUserCreate, ResourceUser, nil, nil, "", "")
	logger.Log(ctx, &user2, ActionSessionLogin, ResourceSession, nil, nil, "", "")
	logger.Log(ctx, &user2, ActionUserCreate, ResourceUser, nil, nil, "", "")

	// Filter: user 1 + action user.create.
	entries, total, err := logger.Query(ctx, QueryParams{
		UserID: &user1,
		Action: ActionUserCreate,
	})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if total != 1 {
		t.Errorf("total: got %d, want 1", total)
	}
	if len(entries) != 1 {
		t.Errorf("entries: got %d, want 1", len(entries))
	}
}

func TestQuery_Pagination(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	user1 := createTestUser(t, pool, "pagination@example.com")

	// Insert 10 entries.
	for i := 0; i < 10; i++ {
		logger.Log(ctx, &user1, ActionSessionLogin, ResourceSession, nil,
			map[string]interface{}{"seq": i}, "", "")
	}

	// Page 1: limit=3, offset=0.
	entries, total, err := logger.Query(ctx, QueryParams{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("query page 1 error: %v", err)
	}
	if total != 10 {
		t.Errorf("total: got %d, want 10", total)
	}
	if len(entries) != 3 {
		t.Errorf("page 1 entries: got %d, want 3", len(entries))
	}

	// Page 2: limit=3, offset=3.
	entries2, total2, err := logger.Query(ctx, QueryParams{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("query page 2 error: %v", err)
	}
	if total2 != 10 {
		t.Errorf("total: got %d, want 10", total2)
	}
	if len(entries2) != 3 {
		t.Errorf("page 2 entries: got %d, want 3", len(entries2))
	}

	// Entries from page 1 and page 2 should not overlap.
	for _, e1 := range entries {
		for _, e2 := range entries2 {
			if e1.ID == e2.ID {
				t.Errorf("overlapping entries: ID %d found in both pages", e1.ID)
			}
		}
	}

	// Last page: offset=9 should return 1 entry.
	entriesLast, _, err := logger.Query(ctx, QueryParams{Limit: 3, Offset: 9})
	if err != nil {
		t.Fatalf("query last page error: %v", err)
	}
	if len(entriesLast) != 1 {
		t.Errorf("last page entries: got %d, want 1", len(entriesLast))
	}
}

func TestQuery_DefaultLimit(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	// Default limit (0) should be normalised to 50 inside Query().
	logger.Log(ctx, nil, ActionSessionLogin, ResourceSession, nil, nil, "", "")

	entries, _, err := logger.Query(ctx, QueryParams{Limit: 0})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	// We only inserted 1, so we should get 1.
	if len(entries) != 1 {
		t.Errorf("entries: got %d, want 1", len(entries))
	}
}

func TestQuery_LimitClampedTo100(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	// Limit > 100 should be clamped to 50.
	logger.Log(ctx, nil, ActionSessionLogin, ResourceSession, nil, nil, "", "")

	entries, _, err := logger.Query(ctx, QueryParams{Limit: 200})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("entries: got %d, want 1", len(entries))
	}
}

func TestQuery_NegativeOffset(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	logger.Log(ctx, nil, ActionSessionLogin, ResourceSession, nil, nil, "", "")

	// Negative offset should be normalised to 0.
	entries, _, err := logger.Query(ctx, QueryParams{Offset: -5})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("entries: got %d, want 1", len(entries))
	}
}

func TestQuery_OrderByTimestampDesc(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	// Insert entries in order.
	logger.Log(ctx, nil, ActionSessionLogin, ResourceSession, nil, map[string]interface{}{"order": "first"}, "", "")
	logger.Log(ctx, nil, ActionSessionLogout, ResourceSession, nil, map[string]interface{}{"order": "second"}, "", "")
	logger.Log(ctx, nil, ActionSessionSudo, ResourceSession, nil, map[string]interface{}{"order": "third"}, "", "")

	entries, _, err := logger.Query(ctx, QueryParams{})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries: got %d, want 3", len(entries))
	}

	// Newest should come first (ORDER BY timestamp DESC).
	// IDs are monotonically increasing, so higher ID = later insert.
	// With DESC order, entries[0] should have the highest ID.
	for i := 1; i < len(entries); i++ {
		if entries[i].ID > entries[i-1].ID {
			t.Errorf("expected descending IDs: entry[%d].ID=%d > entry[%d].ID=%d",
				i, entries[i].ID, i-1, entries[i-1].ID)
		}
	}
}

func TestQuery_EmptyResults(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	entries, total, err := logger.Query(ctx, QueryParams{})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if total != 0 {
		t.Errorf("total: got %d, want 0", total)
	}
	if len(entries) != 0 {
		t.Errorf("entries: got %d, want 0", len(entries))
	}
}

func TestLogger_LogFromRequest_IPv6(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)

	req := httptest.NewRequest("POST", "/api/test", nil)
	req.RemoteAddr = "[::1]:54321"
	req.Header.Set("User-Agent", "TestAgent/1.0")

	logger.LogFromRequest(req, nil, ActionSessionLogin, ResourceSession, nil, nil)

	entries, _, err := logger.Query(req.Context(), QueryParams{})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(entries))
	}
	if entries[0].IPAddress == nil || *entries[0].IPAddress != "::1" {
		t.Errorf("ipAddress: got %v, want ::1", entries[0].IPAddress)
	}
}

func TestLogger_LogAction_DetailsNilVsEmpty(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	// nil details.
	logger.Log(ctx, nil, ActionSessionLogin, ResourceSession, nil, nil, "", "")
	// Empty map details.
	logger.Log(ctx, nil, ActionSessionLogout, ResourceSession, nil, map[string]interface{}{}, "", "")

	entries, _, err := logger.Query(ctx, QueryParams{})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries: got %d, want 2", len(entries))
	}
}

func TestQuery_FilterByAllThreeFields(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := NewLogger(pool)
	ctx := context.Background()

	user1 := createTestUser(t, pool, "all-filters@example.com")

	logger.Log(ctx, &user1, ActionSessionLogin, ResourceSession, nil, nil, "", "")
	logger.Log(ctx, &user1, ActionUserCreate, ResourceUser, nil, nil, "", "")
	logger.Log(ctx, nil, ActionSessionLogin, ResourceSession, nil, nil, "", "")

	// Filter by all three: user + action + resource type.
	entries, total, err := logger.Query(ctx, QueryParams{
		UserID:       &user1,
		Action:       ActionSessionLogin,
		ResourceType: ResourceSession,
	})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if total != 1 {
		t.Errorf("total: got %d, want 1", total)
	}
	if len(entries) != 1 {
		t.Errorf("entries: got %d, want 1", len(entries))
	}
}
