package repository_test

import (
	"context"
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// ── UserRepository OIDC methods ──────────────────────────────────────────────

func TestUserRepository_CreateOIDCUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user, err := repo.CreateOIDCUser(ctx,
		"oidc@example.com", "OIDC User", model.RoleUser,
		"sub-create-1", "https://issuer.example.com",
	)
	if err != nil {
		t.Fatalf("CreateOIDCUser failed: %v", err)
	}
	if user.ID == 0 {
		t.Error("expected non-zero ID after CreateOIDCUser")
	}
	if user.Email != "oidc@example.com" {
		t.Errorf("expected email 'oidc@example.com', got %q", user.Email)
	}
	if user.OIDCSubject == nil || *user.OIDCSubject != "sub-create-1" {
		t.Errorf("expected OIDCSubject 'sub-create-1', got %v", user.OIDCSubject)
	}
	if user.OIDCIssuer == nil || *user.OIDCIssuer != "https://issuer.example.com" {
		t.Errorf("expected OIDCIssuer 'https://issuer.example.com', got %v", user.OIDCIssuer)
	}
	if user.Role != model.RoleUser {
		t.Errorf("expected role 'user', got %q", user.Role)
	}
	if user.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestUserRepository_CreateOIDCUser_DefaultRole(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user, err := repo.CreateOIDCUser(ctx,
		"oidc-default@example.com", "OIDC Default", "", /* empty role */
		"sub-default", "https://issuer.example.com",
	)
	if err != nil {
		t.Fatalf("CreateOIDCUser with empty role failed: %v", err)
	}
	if user.Role != model.RoleUser {
		t.Errorf("expected default role 'user', got %q", user.Role)
	}
}

func TestUserRepository_CreateOIDCUser_DuplicateSubjectIssuer(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, err := repo.CreateOIDCUser(ctx, "oidc1@example.com", "User 1", model.RoleUser, "sub-dup", "https://issuer.example.com")
	if err != nil {
		t.Fatalf("first CreateOIDCUser failed: %v", err)
	}

	_, err = repo.CreateOIDCUser(ctx, "oidc2@example.com", "User 2", model.RoleUser, "sub-dup", "https://issuer.example.com")
	if err == nil {
		t.Error("expected error for duplicate (subject, issuer), got nil")
	}
}

func TestUserRepository_GetByOIDCSubject(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	created, err := repo.CreateOIDCUser(ctx,
		"oidc-lookup@example.com", "Lookup User", model.RoleUser,
		"sub-lookup", "https://issuer.example.com",
	)
	if err != nil {
		t.Fatalf("CreateOIDCUser failed: %v", err)
	}

	found, err := repo.GetByOIDCSubject(ctx, "sub-lookup", "https://issuer.example.com")
	if err != nil {
		t.Fatalf("GetByOIDCSubject failed: %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %d, got %d", created.ID, found.ID)
	}
	if found.Email != "oidc-lookup@example.com" {
		t.Errorf("expected email 'oidc-lookup@example.com', got %q", found.Email)
	}
}

func TestUserRepository_GetByOIDCSubject_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, err := repo.GetByOIDCSubject(ctx, "nonexistent-sub", "https://issuer.example.com")
	if err == nil {
		t.Error("expected error for nonexistent subject, got nil")
	}
}

func TestUserRepository_GetByOIDCSubject_WrongIssuer(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, err := repo.CreateOIDCUser(ctx,
		"oidc-issuer@example.com", "Issuer User", model.RoleUser,
		"sub-issuer", "https://issuer-a.example.com",
	)
	if err != nil {
		t.Fatalf("CreateOIDCUser failed: %v", err)
	}

	// Same subject, different issuer — must not be found.
	_, err = repo.GetByOIDCSubject(ctx, "sub-issuer", "https://issuer-b.example.com")
	if err == nil {
		t.Error("expected error for different issuer, got nil")
	}
}

func TestUserRepository_SetOIDCSubject(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Create a regular password-based user.
	user := &model.User{Email: "link-oidc@example.com", PasswordHash: "hash", Name: "Link OIDC"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.SetOIDCSubject(ctx, user.ID, "sub-linked", "https://issuer.example.com"); err != nil {
		t.Fatalf("SetOIDCSubject failed: %v", err)
	}

	// Subject lookup must now succeed.
	found, err := repo.GetByOIDCSubject(ctx, "sub-linked", "https://issuer.example.com")
	if err != nil {
		t.Fatalf("GetByOIDCSubject failed after SetOIDCSubject: %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, found.ID)
	}
}

// ── OIDCStateRepository ───────────────────────────────────────────────────────

func TestOIDCStateRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewOIDCStateRepository(pool)
	ctx := context.Background()

	if err := repo.Create(ctx, "test-state-token"); err != nil {
		t.Fatalf("Create state failed: %v", err)
	}
}

func TestOIDCStateRepository_Consume_Valid(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewOIDCStateRepository(pool)
	ctx := context.Background()

	const state = "consume-valid-state"
	if err := repo.Create(ctx, state); err != nil {
		t.Fatalf("Create state failed: %v", err)
	}

	ok, err := repo.Consume(ctx, state)
	if err != nil {
		t.Fatalf("Consume failed: %v", err)
	}
	if !ok {
		t.Error("expected Consume to return true for a fresh state")
	}
}

func TestOIDCStateRepository_Consume_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewOIDCStateRepository(pool)
	ctx := context.Background()

	ok, err := repo.Consume(ctx, "nonexistent-state")
	if err != nil {
		t.Fatalf("Consume returned unexpected error: %v", err)
	}
	if ok {
		t.Error("expected Consume to return false for a nonexistent state")
	}
}

func TestOIDCStateRepository_Consume_SingleUse(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewOIDCStateRepository(pool)
	ctx := context.Background()

	const state = "single-use-state"
	if err := repo.Create(ctx, state); err != nil {
		t.Fatalf("Create state failed: %v", err)
	}

	// First consume must succeed.
	ok, err := repo.Consume(ctx, state)
	if err != nil || !ok {
		t.Fatalf("first Consume failed: err=%v ok=%v", err, ok)
	}

	// Second consume must return false — state has been deleted.
	ok, err = repo.Consume(ctx, state)
	if err != nil {
		t.Fatalf("second Consume returned unexpected error: %v", err)
	}
	if ok {
		t.Error("expected second Consume to return false (single-use)")
	}
}
