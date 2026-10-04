package handlers

import (
	"context"

	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// SecurityHandler validates credentials for all ogen-generated routes.
//
// Invalid or expired credentials return ogenerrors.ErrSkipServerSecurity
// instead of hard-failing, so ogen tries the operation's remaining security
// requirements: auth-required operations still end in 401, while operations
// with an anonymous requirement (getSession token login) proceed even when
// the client carries a stale session cookie.
type SecurityHandler struct {
	sessions repository.SessionRepo
	apikeys  repository.ApiKeyRepo
	users    repository.UserRepo
}

// NewSecurityHandler creates a new SecurityHandler.
func NewSecurityHandler(sessions repository.SessionRepo, apikeys repository.ApiKeyRepo, users repository.UserRepo) *SecurityHandler {
	return &SecurityHandler{sessions: sessions, apikeys: apikeys, users: users}
}

// HandleCookieAuth validates a session_id cookie.
func (s *SecurityHandler) HandleCookieAuth(ctx context.Context, _ oas.OperationName, t oas.CookieAuth) (context.Context, error) {
	return s.sessionAuth(ctx, t.APIKey)
}

// HandleXAuthToken validates an X-Auth-Token header (iOS PWA fallback).
// The header value is treated as a session_id.
func (s *SecurityHandler) HandleXAuthToken(ctx context.Context, _ oas.OperationName, t oas.XAuthToken) (context.Context, error) {
	return s.sessionAuth(ctx, t.APIKey)
}

// HandleBearerAuth validates a Bearer API key token.
func (s *SecurityHandler) HandleBearerAuth(ctx context.Context, _ oas.OperationName, t oas.BearerAuth) (context.Context, error) {
	key, err := s.apikeys.GetByToken(ctx, t.Token)
	if err != nil || key == nil || key.IsExpired() {
		return ctx, ogenerrors.ErrSkipServerSecurity
	}
	user := s.user(ctx, key.UserID)
	if user == nil {
		return ctx, ogenerrors.ErrSkipServerSecurity
	}
	return api.ContextWithApiKey(api.ContextWithUser(ctx, user), key), nil
}

func (s *SecurityHandler) sessionAuth(ctx context.Context, sessionID string) (context.Context, error) {
	session, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil || session == nil {
		return ctx, ogenerrors.ErrSkipServerSecurity
	}
	user := s.user(ctx, session.UserID)
	if user == nil {
		return ctx, ogenerrors.ErrSkipServerSecurity
	}
	return api.ContextWithSession(api.ContextWithUser(ctx, user), session), nil
}

func (s *SecurityHandler) user(ctx context.Context, id int64) *model.User {
	user, err := s.users.GetByID(ctx, id)
	if err != nil || user == nil {
		return nil
	}
	return user
}
