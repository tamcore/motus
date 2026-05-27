package handlers

import (
	"context"
	"fmt"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/storage/repository"
)

// SecurityHandler validates credentials for all ogen-generated routes.
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
func (s *SecurityHandler) HandleCookieAuth(ctx context.Context, op oas.OperationName, t oas.CookieAuth) (context.Context, error) {
	session, err := s.sessions.GetByID(ctx, t.APIKey)
	if err != nil || session == nil {
		return ctx, fmt.Errorf("unauthorized")
	}
	user, err := s.users.GetByID(ctx, session.UserID)
	if err != nil || user == nil {
		return ctx, fmt.Errorf("unauthorized")
	}
	user.PopulateTraccarFields()
	ctx = api.ContextWithUser(ctx, user)
	ctx = api.ContextWithSession(ctx, session)
	return ctx, nil
}

// HandleBearerAuth validates a Bearer API key token.
func (s *SecurityHandler) HandleBearerAuth(ctx context.Context, op oas.OperationName, t oas.BearerAuth) (context.Context, error) {
	key, err := s.apikeys.GetByToken(ctx, t.Token)
	if err != nil || key == nil || key.IsExpired() {
		return ctx, fmt.Errorf("unauthorized")
	}
	user, err := s.users.GetByID(ctx, key.UserID)
	if err != nil || user == nil {
		return ctx, fmt.Errorf("unauthorized")
	}
	user.PopulateTraccarFields()
	ctx = api.ContextWithUser(ctx, user)
	ctx = api.ContextWithApiKey(ctx, key)
	return ctx, nil
}

// HandleXAuthToken validates an X-Auth-Token header (iOS PWA fallback).
// The header value is treated as a session_id.
func (s *SecurityHandler) HandleXAuthToken(ctx context.Context, op oas.OperationName, t oas.XAuthToken) (context.Context, error) {
	session, err := s.sessions.GetByID(ctx, t.APIKey)
	if err != nil || session == nil {
		return ctx, fmt.Errorf("unauthorized")
	}
	user, err := s.users.GetByID(ctx, session.UserID)
	if err != nil || user == nil {
		return ctx, fmt.Errorf("unauthorized")
	}
	user.PopulateTraccarFields()
	ctx = api.ContextWithUser(ctx, user)
	ctx = api.ContextWithSession(ctx, session)
	return ctx, nil
}
