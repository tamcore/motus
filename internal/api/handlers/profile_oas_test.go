package handlers_test

import (
	"context"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
	"golang.org/x/crypto/bcrypt"
)

const profileTestOldPassword = "old-password-1234"

func setupProfileHandler(t *testing.T) (*handlers.Handler, *repository.UserRepository, *repository.SessionRepository, *model.User) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)

	hash, err := bcrypt.GenerateFromPassword([]byte(profileTestOldPassword), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &model.User{
		Email:        "profile@test.com",
		PasswordHash: string(hash),
		Name:         "Profile User",
		Role:         model.RoleUser,
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	h := handlers.NewHandler(handlers.HandlerConfig{
		Users:    userRepo,
		Sessions: sessionRepo,
	})
	return h, userRepo, sessionRepo, user
}

func TestUpdateProfile_PasswordChange_RequiresCurrentPassword(t *testing.T) {
	h, userRepo, sessionRepo, user := setupProfileHandler(t)
	ctx := context.Background()

	session, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	authCtx := api.ContextWithSession(api.ContextWithUser(ctx, user), session)

	t.Run("missing current password is rejected", func(t *testing.T) {
		res, err := h.UpdateProfile(authCtx, &oas.UpdateProfileRequest{
			Password: oas.NewOptString("new-password-5678"),
		})
		if err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}
		if _, ok := res.(*oas.UpdateProfileBadRequest); !ok {
			t.Fatalf("expected UpdateProfileBadRequest, got %T", res)
		}
	})

	t.Run("wrong current password is rejected", func(t *testing.T) {
		res, err := h.UpdateProfile(authCtx, &oas.UpdateProfileRequest{
			Password:        oas.NewOptString("new-password-5678"),
			CurrentPassword: oas.NewOptString("not-the-old-password"),
		})
		if err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}
		if _, ok := res.(*oas.UpdateProfileBadRequest); !ok {
			t.Fatalf("expected UpdateProfileBadRequest, got %T", res)
		}
	})

	t.Run("password is unchanged after rejected attempts", func(t *testing.T) {
		fresh, err := userRepo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("get user: %v", err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(fresh.PasswordHash), []byte(profileTestOldPassword)); err != nil {
			t.Errorf("old password no longer matches: %v", err)
		}
	})

	t.Run("correct current password is accepted", func(t *testing.T) {
		res, err := h.UpdateProfile(authCtx, &oas.UpdateProfileRequest{
			Password:        oas.NewOptString("new-password-5678"),
			CurrentPassword: oas.NewOptString(profileTestOldPassword),
		})
		if err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}
		if _, ok := res.(*oas.User); !ok {
			t.Fatalf("expected *oas.User, got %T", res)
		}
		fresh, err := userRepo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("get user: %v", err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(fresh.PasswordHash), []byte("new-password-5678")); err != nil {
			t.Errorf("new password does not match: %v", err)
		}
	})
}

func TestUpdateProfile_PasswordChange_RevokesOtherSessions(t *testing.T) {
	h, _, sessionRepo, user := setupProfileHandler(t)
	ctx := context.Background()

	current, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create current session: %v", err)
	}
	other1, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}
	other2, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}

	authCtx := api.ContextWithSession(api.ContextWithUser(ctx, user), current)
	res, err := h.UpdateProfile(authCtx, &oas.UpdateProfileRequest{
		Password:        oas.NewOptString("new-password-5678"),
		CurrentPassword: oas.NewOptString(profileTestOldPassword),
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if _, ok := res.(*oas.User); !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}

	if _, err := sessionRepo.GetByID(ctx, current.ID); err != nil {
		t.Errorf("current session should survive password change: %v", err)
	}
	if _, err := sessionRepo.GetByID(ctx, other1.ID); err == nil {
		t.Errorf("other session 1 should be revoked after password change")
	}
	if _, err := sessionRepo.GetByID(ctx, other2.ID); err == nil {
		t.Errorf("other session 2 should be revoked after password change")
	}
}

func TestUpdateProfile_NameOnlyUpdate_KeepsSessions(t *testing.T) {
	h, _, sessionRepo, user := setupProfileHandler(t)
	ctx := context.Background()

	current, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create current session: %v", err)
	}
	other, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}

	authCtx := api.ContextWithSession(api.ContextWithUser(ctx, user), current)
	res, err := h.UpdateProfile(authCtx, &oas.UpdateProfileRequest{
		Name: oas.NewOptString("Renamed User"),
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if _, ok := res.(*oas.User); !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}

	if _, err := sessionRepo.GetByID(ctx, current.ID); err != nil {
		t.Errorf("current session should survive name update: %v", err)
	}
	if _, err := sessionRepo.GetByID(ctx, other.ID); err != nil {
		t.Errorf("other session should survive name update: %v", err)
	}
}

func TestAdminUpdateUser_PasswordReset_RevokesAllTargetSessions(t *testing.T) {
	h, userRepo, sessionRepo, target := setupProfileHandler(t)
	ctx := context.Background()

	admin := &model.User{
		Email:        "admin-reset@test.com",
		PasswordHash: "$2a$10$fakehash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	if err := userRepo.Create(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	s1, err := sessionRepo.Create(ctx, target.ID)
	if err != nil {
		t.Fatalf("create target session: %v", err)
	}
	s2, err := sessionRepo.Create(ctx, target.ID)
	if err != nil {
		t.Fatalf("create target session: %v", err)
	}

	adminCtx := api.ContextWithUser(ctx, admin)
	res, err := h.AdminUpdateUser(adminCtx, &oas.UserInput{
		Name:     target.Name,
		Email:    target.Email,
		Password: oas.NewOptString("admin-set-password-1"),
	}, oas.AdminUpdateUserParams{ID: target.ID})
	if err != nil {
		t.Fatalf("AdminUpdateUser: %v", err)
	}
	if _, ok := res.(*oas.User); !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}

	if _, err := sessionRepo.GetByID(ctx, s1.ID); err == nil {
		t.Errorf("target session 1 should be revoked after admin password reset")
	}
	if _, err := sessionRepo.GetByID(ctx, s2.ID); err == nil {
		t.Errorf("target session 2 should be revoked after admin password reset")
	}
}

func TestAdminUpdateUser_NoPasswordChange_KeepsSessions(t *testing.T) {
	h, userRepo, sessionRepo, target := setupProfileHandler(t)
	ctx := context.Background()

	admin := &model.User{
		Email:        "admin-keep@test.com",
		PasswordHash: "$2a$10$fakehash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	if err := userRepo.Create(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	s1, err := sessionRepo.Create(ctx, target.ID)
	if err != nil {
		t.Fatalf("create target session: %v", err)
	}

	adminCtx := api.ContextWithUser(ctx, admin)
	res, err := h.AdminUpdateUser(adminCtx, &oas.UserInput{
		Name:  "Updated Name",
		Email: target.Email,
	}, oas.AdminUpdateUserParams{ID: target.ID})
	if err != nil {
		t.Fatalf("AdminUpdateUser: %v", err)
	}
	if _, ok := res.(*oas.User); !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}

	if _, err := sessionRepo.GetByID(ctx, s1.ID); err != nil {
		t.Errorf("target session should survive non-password update: %v", err)
	}
}
