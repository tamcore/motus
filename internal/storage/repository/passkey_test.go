package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func newPasskeyTestUser(t *testing.T, userRepo *repository.UserRepository, email string) *model.User {
	t.Helper()
	user := &model.User{Email: email, PasswordHash: "hash", Name: "Passkey User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func TestPasskeyRepository_CreateAndList(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	repo := repository.NewPasskeyRepository(pool)
	user := newPasskeyTestUser(t, userRepo, "pk-create@example.com")

	cred := &model.PasskeyCredential{
		UserID:          user.ID,
		CredentialID:    []byte("cred-id-1"),
		PublicKey:       []byte("public-key-bytes"),
		AttestationType: "none",
		AAGUID:          []byte("aaguid--16-bytes"),
		SignCount:       0,
		Transports:      []string{"internal", "hybrid"},
		BackupEligible:  true,
		BackupState:     false,
		Name:            "iPhone",
	}
	if err := repo.Create(ctx, cred); err != nil {
		t.Fatalf("create passkey: %v", err)
	}
	if cred.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}
	if cred.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}

	list, err := repo.ListByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(list))
	}
	got := list[0]
	if got.Name != "iPhone" || string(got.CredentialID) != "cred-id-1" {
		t.Errorf("unexpected credential: %+v", got)
	}
	if len(got.Transports) != 2 || got.Transports[0] != "internal" {
		t.Errorf("transports not round-tripped: %v", got.Transports)
	}
	if !got.BackupEligible || got.BackupState {
		t.Errorf("backup flags not round-tripped: eligible=%v state=%v", got.BackupEligible, got.BackupState)
	}
}

func TestPasskeyRepository_GetByCredentialID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	repo := repository.NewPasskeyRepository(pool)
	user := newPasskeyTestUser(t, userRepo, "pk-get@example.com")

	cred := &model.PasskeyCredential{
		UserID: user.ID, CredentialID: []byte("lookup-id"), PublicKey: []byte("pk"), Name: "Key",
	}
	if err := repo.Create(ctx, cred); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByCredentialID(ctx, []byte("lookup-id"))
	if err != nil {
		t.Fatalf("get by credential id: %v", err)
	}
	if got.ID != cred.ID || got.UserID != user.ID {
		t.Errorf("wrong credential returned: %+v", got)
	}

	if _, err := repo.GetByCredentialID(ctx, []byte("missing")); err == nil {
		t.Error("expected error for missing credential")
	}
}

func TestPasskeyRepository_UpdateSignCount(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	repo := repository.NewPasskeyRepository(pool)
	user := newPasskeyTestUser(t, userRepo, "pk-sc@example.com")

	cred := &model.PasskeyCredential{UserID: user.ID, CredentialID: []byte("sc"), PublicKey: []byte("pk")}
	if err := repo.Create(ctx, cred); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.UpdateSignCount(ctx, cred.ID, 42); err != nil {
		t.Fatalf("update sign count: %v", err)
	}

	got, err := repo.GetByCredentialID(ctx, []byte("sc"))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SignCount != 42 {
		t.Errorf("sign count = %d, want 42", got.SignCount)
	}
	if got.LastUsedAt == nil {
		t.Error("expected LastUsedAt to be set after sign count update")
	}
}

func TestPasskeyRepository_Delete_ScopedByUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	repo := repository.NewPasskeyRepository(pool)
	owner := newPasskeyTestUser(t, userRepo, "pk-owner@example.com")
	other := newPasskeyTestUser(t, userRepo, "pk-other@example.com")

	cred := &model.PasskeyCredential{UserID: owner.ID, CredentialID: []byte("del"), PublicKey: []byte("pk")}
	if err := repo.Create(ctx, cred); err != nil {
		t.Fatalf("create: %v", err)
	}

	// A different user cannot delete it: the scoped DELETE affects 0 rows and
	// reports not-found rather than silently succeeding.
	if err := repo.Delete(ctx, cred.ID, other.ID); !errors.Is(err, repository.ErrPasskeyNotFound) {
		t.Fatalf("delete (wrong user): expected ErrPasskeyNotFound, got %v", err)
	}
	if list, _ := repo.ListByUser(ctx, owner.ID); len(list) != 1 {
		t.Fatal("credential should survive delete by another user")
	}

	// The owner can delete it.
	if err := repo.Delete(ctx, cred.ID, owner.ID); err != nil {
		t.Fatalf("delete (owner): %v", err)
	}
	if list, _ := repo.ListByUser(ctx, owner.ID); len(list) != 0 {
		t.Fatal("credential should be deleted by owner")
	}
}

func TestPasskeyRepository_DeleteAllByUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	repo := repository.NewPasskeyRepository(pool)
	user := newPasskeyTestUser(t, userRepo, "pk-all@example.com")

	for _, id := range [][]byte{[]byte("a"), []byte("b"), []byte("c")} {
		if err := repo.Create(ctx, &model.PasskeyCredential{UserID: user.ID, CredentialID: id, PublicKey: []byte("pk")}); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	if err := repo.DeleteAllByUser(ctx, user.ID); err != nil {
		t.Fatalf("delete all: %v", err)
	}
	if list, _ := repo.ListByUser(ctx, user.ID); len(list) != 0 {
		t.Fatalf("expected 0 after DeleteAllByUser, got %d", len(list))
	}
}
