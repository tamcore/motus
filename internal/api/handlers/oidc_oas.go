package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
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
		return &oas.UnexpectedErrorStatusCode{StatusCode: http.StatusNotFound, Response: oas.Error{Error: "OIDC not enabled"}}
	}

	state := repository.NewToken()

	if err := h.cfg.OIDCStateRepo.Create(ctx, state); err != nil {
		return fmt.Errorf("failed to store state")
	}

	_, oauth2Cfg, err := h.buildOIDCOAuth2Config(ctx)
	if err != nil {
		return fmt.Errorf("failed to build auth URL: %w", err)
	}

	if w := api.ResponseWriterFromContext(ctx); w != nil {
		w.Header().Set("Location", oauth2Cfg.AuthCodeURL(state))
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

	provider, oauth2Cfg, err := h.buildOIDCOAuth2Config(ctx)
	if err != nil {
		slog.Warn("oidc: provider discovery failed", slog.Any("error", err))
		return &oas.Error{Error: "code exchange failed"}, nil
	}

	token, err := oauth2Cfg.Exchange(ctx, code)
	if err != nil {
		slog.Warn("oidc: code exchange failed", slog.Any("error", err))
		return &oas.Error{Error: "code exchange failed"}, nil
	}

	rawIDToken, idOK := token.Extra("id_token").(string)
	if !idOK {
		slog.Warn("oidc: no id_token in token response")
		return &oas.Error{Error: "no id_token in token response"}, nil
	}

	idToken, err := provider.Verifier(&gooidc.Config{ClientID: oauth2Cfg.ClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		slog.Warn("oidc: id_token verification failed", slog.Any("error", err))
		return &oas.Error{Error: "id_token verification failed"}, nil
	}

	var allClaims map[string]any
	if err := idToken.Claims(&allClaims); err != nil {
		slog.Warn("oidc: failed to decode id_token claims", slog.Any("error", err))
		return &oas.Error{Error: "failed to decode id_token claims"}, nil
	}
	email, _ := allClaims["email"].(string)
	name, _ := allClaims["name"].(string)

	// email_verified is read from the raw claims map because some IdPs emit
	// it as the string "true" rather than a JSON boolean.
	emailVerified := claimBool(allClaims, "email_verified")
	user, err := h.resolveOIDCUserFromCtx(ctx, idToken.Subject, email, name, emailVerified)
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

	if user.Role != model.RoleAdmin && h.oidcIsAdminByFilter(email, allClaims) {
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

	h.setSessionCookie(ctx, session.ID, session.ExpiresAt)
	if w := api.ResponseWriterFromContext(ctx); w != nil {
		w.Header().Set("Location", "/")
	}

	h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionSessionLogin, audit.ResourceSession, nil,
		map[string]any{"method": "oidc", "email": user.Email})

	return &oas.OidcCallbackFound{}, nil
}

// buildOIDCOAuth2Config constructs an OIDC provider and oauth2.Config from the handler config.
func (h *Handler) buildOIDCOAuth2Config(ctx context.Context) (*gooidc.Provider, oauth2.Config, error) {
	cfg := h.cfg.OIDCConfig
	provider, err := gooidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, oauth2.Config{}, fmt.Errorf("oidc: fetch provider: %w", err)
	}

	scopes := []string{gooidc.ScopeOpenID, "email", "profile"}
	// MOTUS_OIDC_SCOPES: space-separated additional scopes (e.g. "groups").
	scopes = append(scopes, strings.Fields(cfg.Scopes)...)

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}
	return provider, oauth2Cfg, nil
}

// errSignupDisabled is returned when a new OIDC user cannot be created
// because signup is disabled.
var errSignupDisabled = errors.New("oidc signup disabled")

// claimBool reads a boolean claim from a raw claims map. It accepts both a
// JSON boolean and the string forms "true"/"false" (used by some IdPs, e.g.
// Azure AD B2C). Missing or unrecognised values return false.
func claimBool(claims map[string]any, key string) bool {
	switch v := claims[key].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	default:
		return false
	}
}

// resolveOIDCUserFromCtx finds an existing user by OIDC subject, falls back
// to email lookup and links the subject, or creates a new account when
// signup is enabled.
//
// The email fallback only links when the IdP asserts the email is verified
// (or TrustUnverifiedEmail is configured): linking on an unverified email
// would let an attacker take over a local account by registering the same
// address at a lax IdP.
func (h *Handler) resolveOIDCUserFromCtx(ctx context.Context, subject, email, name string, emailVerified bool) (*model.User, error) {
	issuer := h.cfg.OIDCConfig.Issuer

	user, err := h.cfg.Users.GetByOIDCSubject(ctx, subject, issuer)
	if err == nil {
		return user, nil
	}

	if email != "" && (emailVerified || h.cfg.OIDCConfig.TrustUnverifiedEmail) {
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
func (h *Handler) oidcIsAdminByFilter(email string, allClaims map[string]any) bool {
	cfg := h.cfg.OIDCConfig

	if cfg.AdminEmailRegex != "" {
		re, err := regexp.Compile(cfg.AdminEmailRegex)
		if err == nil && re.MatchString(email) {
			return true
		}
	}

	if cfg.AdminClaim == "" || cfg.AdminClaimValue == "" {
		return false
	}
	switch v := allClaims[cfg.AdminClaim].(type) {
	case string:
		return v == cfg.AdminClaimValue
	case []any:
		return slices.Contains(v, any(cfg.AdminClaimValue))
	}
	return false
}
