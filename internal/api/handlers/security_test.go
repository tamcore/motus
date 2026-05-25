package handlers_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
)

// TestHandleCookieAuth_valid verifies that a valid session cookie authenticates
// the user and populates the user in the request context.
func TestHandleCookieAuth_valid(t *testing.T) {
	// Arrange
	userID := int64(1)
	sess := &model.Session{ID: "valid-session", UserID: userID, ExpiresAt: time.Now().Add(24 * time.Hour)}
	user := &model.User{ID: userID, Email: "test@example.com", Role: "user"}

	sh := handlers.NewSecurityHandler(
		&mockSessionRepo{
			getByIDFn: func(_ context.Context, id string) (*model.Session, error) {
				if id == "valid-session" {
					return sess, nil
				}
				return nil, errors.New("not found")
			},
		},
		&mockApiKeyRepo{},
		&mockUserRepo{
			getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
				if id == userID {
					return user, nil
				}
				return nil, errors.New("not found")
			},
		},
	)

	// Act
	ctx, err := sh.HandleCookieAuth(context.Background(), "listDevices", oas.CookieAuth{APIKey: "valid-session"})

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if api.UserFromContext(ctx) == nil {
		t.Fatal("expected user in context")
	}
	if api.UserFromContext(ctx).Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", api.UserFromContext(ctx).Email)
	}
}

// TestHandleCookieAuth_invalid verifies that an unknown session cookie returns
// an unauthorized error.
func TestHandleCookieAuth_invalid(t *testing.T) {
	// Arrange
	sh := handlers.NewSecurityHandler(
		&mockSessionRepo{
			getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
				return nil, errors.New("not found")
			},
		},
		&mockApiKeyRepo{},
		&mockUserRepo{},
	)

	// Act
	_, err := sh.HandleCookieAuth(context.Background(), "listDevices", oas.CookieAuth{APIKey: "bad-session"})

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid session")
	}
}

// TestHandleCookieAuth_userNotFound verifies that a valid session whose user no
// longer exists returns an unauthorized error.
func TestHandleCookieAuth_userNotFound(t *testing.T) {
	// Arrange
	sess := &model.Session{ID: "orphan-session", UserID: 99, ExpiresAt: time.Now().Add(time.Hour)}

	sh := handlers.NewSecurityHandler(
		&mockSessionRepo{
			getByIDFn: func(_ context.Context, id string) (*model.Session, error) {
				if id == "orphan-session" {
					return sess, nil
				}
				return nil, errors.New("not found")
			},
		},
		&mockApiKeyRepo{},
		&mockUserRepo{
			getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
				return nil, errors.New("not found")
			},
		},
	)

	// Act
	_, err := sh.HandleCookieAuth(context.Background(), "listDevices", oas.CookieAuth{APIKey: "orphan-session"})

	// Assert
	if err == nil {
		t.Fatal("expected error when user not found")
	}
}

// TestHandleBearerAuth_valid verifies that a valid non-expired API key
// authenticates the user and populates the key and user in the context.
func TestHandleBearerAuth_valid(t *testing.T) {
	// Arrange
	userID := int64(2)
	key := &model.ApiKey{ID: 10, UserID: userID, Token: "valid-token", Permissions: "full"}
	user := &model.User{ID: userID, Email: "bearer@example.com", Role: "user"}

	sh := handlers.NewSecurityHandler(
		&mockSessionRepo{},
		&mockApiKeyRepo{
			getByTokenFn: func(_ context.Context, token string) (*model.ApiKey, error) {
				if token == "valid-token" {
					return key, nil
				}
				return nil, errors.New("not found")
			},
		},
		&mockUserRepo{
			getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
				if id == userID {
					return user, nil
				}
				return nil, errors.New("not found")
			},
		},
	)

	// Act
	ctx, err := sh.HandleBearerAuth(context.Background(), "listDevices", oas.BearerAuth{Token: "valid-token"})

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if api.UserFromContext(ctx) == nil {
		t.Fatal("expected user in context")
	}
	if api.ApiKeyFromContext(ctx) == nil {
		t.Fatal("expected api key in context")
	}
}

// TestHandleBearerAuth_invalid verifies that an unknown token returns an
// unauthorized error.
func TestHandleBearerAuth_invalid(t *testing.T) {
	// Arrange
	sh := handlers.NewSecurityHandler(
		&mockSessionRepo{},
		&mockApiKeyRepo{
			getByTokenFn: func(_ context.Context, _ string) (*model.ApiKey, error) {
				return nil, errors.New("not found")
			},
		},
		&mockUserRepo{},
	)

	// Act
	_, err := sh.HandleBearerAuth(context.Background(), "listDevices", oas.BearerAuth{Token: "bad-token"})

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

// TestHandleBearerAuth_expiredKey verifies that an expired API key returns an
// unauthorized error.
func TestHandleBearerAuth_expiredKey(t *testing.T) {
	// Arrange
	past := time.Now().Add(-time.Hour)
	expiredKey := &model.ApiKey{ID: 5, UserID: 1, Token: "expired-token", Permissions: "full", ExpiresAt: &past}

	sh := handlers.NewSecurityHandler(
		&mockSessionRepo{},
		&mockApiKeyRepo{
			getByTokenFn: func(_ context.Context, token string) (*model.ApiKey, error) {
				if token == "expired-token" {
					return expiredKey, nil
				}
				return nil, errors.New("not found")
			},
		},
		&mockUserRepo{},
	)

	// Act
	_, err := sh.HandleBearerAuth(context.Background(), "listDevices", oas.BearerAuth{Token: "expired-token"})

	// Assert
	if err == nil {
		t.Fatal("expected error for expired API key")
	}
}

// TestHandleXAuthToken_valid verifies that a valid X-Auth-Token (treated as a
// session ID) authenticates the user and populates the session in context.
func TestHandleXAuthToken_valid(t *testing.T) {
	// Arrange
	userID := int64(3)
	sess := &model.Session{ID: "xauth-session", UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}
	user := &model.User{ID: userID, Email: "xauth@example.com", Role: "user"}

	sh := handlers.NewSecurityHandler(
		&mockSessionRepo{
			getByIDFn: func(_ context.Context, id string) (*model.Session, error) {
				if id == "xauth-session" {
					return sess, nil
				}
				return nil, errors.New("not found")
			},
		},
		&mockApiKeyRepo{},
		&mockUserRepo{
			getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
				if id == userID {
					return user, nil
				}
				return nil, errors.New("not found")
			},
		},
	)

	// Act
	ctx, err := sh.HandleXAuthToken(context.Background(), "listDevices", oas.XAuthToken{APIKey: "xauth-session"})

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if api.UserFromContext(ctx) == nil {
		t.Fatal("expected user in context")
	}
}

// TestHandleXAuthToken_invalid verifies that an unknown X-Auth-Token returns an
// unauthorized error.
func TestHandleXAuthToken_invalid(t *testing.T) {
	// Arrange
	sh := handlers.NewSecurityHandler(
		&mockSessionRepo{
			getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
				return nil, errors.New("not found")
			},
		},
		&mockApiKeyRepo{},
		&mockUserRepo{},
	)

	// Act
	_, err := sh.HandleXAuthToken(context.Background(), "listDevices", oas.XAuthToken{APIKey: "bad-xauth"})

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid X-Auth-Token")
	}
}
