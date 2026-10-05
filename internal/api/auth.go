package api

import (
	"context"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// SessionExpiryRememberMe is the lifetime of "remember me" and token-login
// sessions; the auth middleware rolls it forward once half of it has passed.
const SessionExpiryRememberMe = 30 * 24 * time.Hour

// ResolveToken resolves a bearer/login token to its user: api_keys first,
// then the legacy users.token column (key is nil). Callers handle expired keys.
func ResolveToken(ctx context.Context, users repository.UserRepo, apiKeys repository.ApiKeyRepo, token string) (*model.User, *model.ApiKey) {
	if token == "" {
		return nil, nil
	}
	if apiKeys != nil {
		if key, err := apiKeys.GetByToken(ctx, token); err == nil && key != nil {
			if user, err := users.GetByID(ctx, key.UserID); err == nil && user != nil {
				return user, key
			}
		}
	}
	if user, err := users.GetByToken(ctx, token); err == nil && user != nil {
		return user, nil
	}
	return nil, nil
}

// ResolveSession resolves a session ID to its user and session, plus the API
// key the session was created from (nil if none or already deleted).
// Callers handle expired keys.
func ResolveSession(ctx context.Context, users repository.UserRepo, sessions repository.SessionRepo, apiKeys repository.ApiKeyRepo, sessionID string) (*model.User, *model.Session, *model.ApiKey) {
	if sessionID == "" {
		return nil, nil, nil
	}
	session, err := sessions.GetByID(ctx, sessionID)
	if err != nil || session == nil {
		return nil, nil, nil
	}
	user, err := users.GetByID(ctx, session.UserID)
	if err != nil || user == nil {
		return nil, nil, nil
	}
	if session.ApiKeyID == nil || apiKeys == nil {
		return user, session, nil
	}
	key, err := apiKeys.GetByID(ctx, *session.ApiKeyID)
	if err != nil {
		key = nil
	}
	return user, session, key
}
