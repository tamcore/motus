package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"golang.org/x/oauth2"
)

// GetOIDCConfig returns OIDC availability status for the frontend.
// GET /api/auth/oidc/config
func (h *Handler) GetOIDCConfig(ctx context.Context) (*oas.OIDCConfig, error) {
	cfg := &oas.OIDCConfig{
		Enabled: h.cfg.OIDCConfig.Enabled,
	}
	if h.cfg.OIDCConfig.Enabled && h.cfg.OIDCConfig.Issuer != "" {
		cfg.Issuer = oas.OptString{Value: h.cfg.OIDCConfig.Issuer, Set: true}
	}
	return cfg, nil
}

// OidcLogin initiates the OIDC authorization code flow.
// GET /api/auth/oidc/login
func (h *Handler) OidcLogin(ctx context.Context) error {
	if !h.cfg.OIDCConfig.Enabled {
		return fmt.Errorf("oidc not enabled")
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("failed to generate state")
	}
	state := hex.EncodeToString(b)

	if err := h.cfg.OIDCStateRepo.Create(ctx, state); err != nil {
		return fmt.Errorf("failed to store state")
	}

	authURL, err := h.oidcAuthURL(ctx, state)
	if err != nil {
		return fmt.Errorf("failed to build auth URL: %w", err)
	}

	if w := api.ResponseWriterFromContext(ctx); w != nil {
		w.Header().Set("Location", authURL)
	}
	return nil
}

// OidcCallback handles the redirect from the OIDC provider.
// GET /api/auth/oidc/callback
func (h *Handler) OidcCallback(ctx context.Context, params oas.OidcCallbackParams) (oas.OidcCallbackRes, error) {
	if !h.cfg.OIDCConfig.Enabled {
		return &oas.Error{Error: "OIDC not enabled"}, nil
	}

	state, _ := params.State.Get()
	code, _ := params.Code.Get()

	if state == "" || code == "" {
		return &oas.Error{Error: "missing state or code"}, nil
	}

	ok, err := h.cfg.OIDCStateRepo.Consume(ctx, state)
	if err != nil || !ok {
		if err != nil {
			slog.Error("oidc: consume state", slog.Any("error", err))
		}
		return &oas.Error{Error: "invalid or expired state"}, nil
	}

	token, err := h.oidcExchangeCode(ctx, code)
	if err != nil {
		slog.Warn("oidc: code exchange failed", slog.Any("error", err))
		return &oas.Error{Error: "code exchange failed"}, nil
	}

	rawIDToken, idOK := token.Extra("id_token").(string)
	if !idOK {
		slog.Warn("oidc: no id_token in token response")
		return &oas.Error{Error: "no id_token in token response"}, nil
	}

	idToken, err := h.oidcVerifyToken(ctx, rawIDToken)
	if err != nil {
		slog.Warn("oidc: id_token verification failed", slog.Any("error", err))
		return &oas.Error{Error: "id_token verification failed"}, nil
	}

	var stdClaims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&stdClaims); err != nil {
		slog.Warn("oidc: failed to decode id_token claims", slog.Any("error", err))
		return &oas.Error{Error: "failed to decode id_token claims"}, nil
	}

	var allClaims map[string]interface{}
	_ = idToken.Claims(&allClaims)

	user, err := h.resolveOIDCUserFromCtx(ctx, idToken.Subject, stdClaims.Email, stdClaims.Name)
	if errors.Is(err, errSignupDisabled) {
		if w := api.ResponseWriterFromContext(ctx); w != nil {
			w.Header().Set("Location", "/login?error=signup_disabled")
		}
		return &oas.OidcCallbackFound{}, nil
	}
	if err != nil {
		slog.Error("oidc: resolve user", slog.Any("error", err))
		return &oas.Error{Error: "failed to resolve user"}, nil
	}

	if user.Role != model.RoleAdmin && h.oidcIsAdminByFilter(stdClaims.Email, allClaims) {
		user.Role = model.RoleAdmin
		if err := h.cfg.Users.Update(ctx, user); err != nil {
			slog.Warn("oidc: failed to set admin role", slog.Any("error", err))
		}
	}

	session, err := h.cfg.Sessions.CreateWithExpiry(ctx, user.ID, time.Now().Add(sessionExpiryDefault), false)
	if err != nil {
		slog.Error("oidc: create session", slog.Any("error", err))
		return &oas.Error{Error: "failed to create session"}, nil
	}

	if w := api.ResponseWriterFromContext(ctx); w != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    session.ID,
			Path:     "/",
			Expires:  session.ExpiresAt,
			HttpOnly: true,
			Secure:   isSecureEnvironment(),
			SameSite: http.SameSiteLaxMode,
		})
		w.Header().Set("Location", "/")
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionSessionLogin, audit.ResourceSession, nil,
			map[string]interface{}{"method": "oidc", "email": user.Email}, "", "")
	}

	return &oas.OidcCallbackFound{}, nil
}

// oidcAuthURL builds the authorization URL using the OIDC provider.
func (h *Handler) oidcAuthURL(ctx context.Context, state string) (string, error) {
	_, oauth2Cfg, err := h.buildOIDCOAuth2Config(ctx)
	if err != nil {
		return "", err
	}
	return oauth2Cfg.AuthCodeURL(state), nil
}

// oidcExchangeCode exchanges an authorization code for tokens.
func (h *Handler) oidcExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	_, oauth2Cfg, err := h.buildOIDCOAuth2Config(ctx)
	if err != nil {
		return nil, err
	}
	return oauth2Cfg.Exchange(ctx, code)
}

// oidcVerifyToken verifies and parses the raw ID token.
func (h *Handler) oidcVerifyToken(ctx context.Context, rawIDToken string) (*gooidc.IDToken, error) {
	provider, oauth2Cfg, err := h.buildOIDCOAuth2Config(ctx)
	if err != nil {
		return nil, err
	}
	verifier := provider.Verifier(&gooidc.Config{ClientID: oauth2Cfg.ClientID})
	return verifier.Verify(ctx, rawIDToken)
}

// buildOIDCOAuth2Config constructs an OIDC provider and oauth2.Config from the handler config.
func (h *Handler) buildOIDCOAuth2Config(ctx context.Context) (*gooidc.Provider, oauth2.Config, error) {
	cfg := h.cfg.OIDCConfig
	provider, err := gooidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, oauth2.Config{}, fmt.Errorf("oidc: fetch provider: %w", err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID, "email", "profile"},
	}
	return provider, oauth2Cfg, nil
}

// resolveOIDCUserFromCtx mirrors OIDCHandler.resolveOIDCUser for context-based calls.
func (h *Handler) resolveOIDCUserFromCtx(ctx context.Context, subject, email, name string) (*model.User, error) {
	issuer := h.cfg.OIDCConfig.Issuer

	user, err := h.cfg.Users.GetByOIDCSubject(ctx, subject, issuer)
	if err == nil {
		return user, nil
	}

	if email != "" {
		user, err = h.cfg.Users.GetByEmail(ctx, email)
		if err == nil {
			if linkErr := h.cfg.Users.SetOIDCSubject(ctx, user.ID, subject, issuer); linkErr != nil {
				slog.Warn("oidc: failed to link subject to existing user", slog.Any("error", linkErr))
			}
			user.OIDCSubject = &subject
			user.OIDCIssuer = &issuer
			return user, nil
		}
	}

	if !h.cfg.OIDCConfig.SignupEnabled {
		return nil, errSignupDisabled
	}

	displayName := name
	if displayName == "" {
		displayName = email
	}
	user, err = h.cfg.Users.CreateOIDCUser(ctx, email, displayName, model.RoleUser, subject, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: create user: %w", err)
	}
	slog.Info("oidc: new user registered", slog.String("email", email))
	return user, nil
}

// oidcIsAdminByFilter checks if the user should get the admin role
// based on email regex or claim filter.
func (h *Handler) oidcIsAdminByFilter(email string, allClaims map[string]interface{}) bool {
	cfg := h.cfg.OIDCConfig

	if cfg.AdminEmailRegex != "" {
		re, err := regexp.Compile(cfg.AdminEmailRegex)
		if err == nil && re.MatchString(email) {
			return true
		}
	}

	if cfg.AdminClaim != "" && cfg.AdminClaimValue != "" {
		claimVal, ok := allClaims[cfg.AdminClaim]
		if ok {
			switch v := claimVal.(type) {
			case string:
				if v == cfg.AdminClaimValue {
					return true
				}
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok && s == cfg.AdminClaimValue {
						return true
					}
				}
			}
		}
	}

	return false
}
