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
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/config"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"golang.org/x/oauth2"
)

// errSignupDisabled is returned by resolveOIDCUser when the user is not found
// and OIDC signup is disabled.
var errSignupDisabled = errors.New("oidc: signup disabled")

// OIDCHandler handles OIDC authentication endpoints.
type OIDCHandler struct {
	cfg             config.OIDCConfig
	provider        *gooidc.Provider
	oauth2Config    oauth2.Config
	adminEmailRegex *regexp.Regexp // compiled from cfg.AdminEmailRegex; nil if unset
	users           repository.UserRepo
	sessions        repository.SessionRepo
	states          repository.OIDCStateRepo
	audit           *audit.Logger
}

// NewOIDCHandler creates a new OIDCHandler and connects to the OIDC provider.
// An error is returned if the OIDC discovery document cannot be fetched or if
// the configuration is invalid.
func NewOIDCHandler(
	ctx context.Context,
	cfg config.OIDCConfig,
	users repository.UserRepo,
	sessions repository.SessionRepo,
	states repository.OIDCStateRepo,
) (*OIDCHandler, error) {
	provider, err := gooidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: fetch provider discovery document: %w", err)
	}

	scopes := []string{gooidc.ScopeOpenID, "email", "profile"}
	if cfg.Scopes != "" {
		scopes = append(scopes, strings.Fields(cfg.Scopes)...)
	}

	oauth2Config := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	var adminRe *regexp.Regexp
	if cfg.AdminEmailRegex != "" {
		adminRe, err = regexp.Compile(cfg.AdminEmailRegex)
		if err != nil {
			// Already validated in config.Validate; this is a safety net.
			return nil, fmt.Errorf("oidc: compile admin email regex: %w", err)
		}
	}

	return &OIDCHandler{
		cfg:             cfg,
		provider:        provider,
		oauth2Config:    oauth2Config,
		adminEmailRegex: adminRe,
		users:           users,
		sessions:        sessions,
		states:          states,
	}, nil
}

// SetAuditLogger configures audit logging for OIDC login events.
func (h *OIDCHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

// GetConfig returns the OIDC availability status for the frontend.
// GET /api/auth/oidc/config
func (h *OIDCHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	api.RespondJSON(w, http.StatusOK, map[string]bool{"enabled": h.cfg.Enabled})
}

// Login initiates the OIDC authorization code flow.
// GET /api/auth/oidc/login
func (h *OIDCHandler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Enabled {
		api.RespondError(w, http.StatusNotFound, "OIDC not enabled")
		return
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}
	state := hex.EncodeToString(b)

	if err := h.states.Create(r.Context(), state); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to store state")
		return
	}

	http.Redirect(w, r, h.oauth2Config.AuthCodeURL(state), http.StatusFound)
}

// Callback handles the redirect from the OIDC provider.
// GET /api/auth/oidc/callback
func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Enabled {
		http.Redirect(w, r, "/login?error=provider_error", http.StatusFound)
		return
	}

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		slog.Warn("oidc: provider returned error",
			slog.String("error", errParam),
			slog.String("description", r.URL.Query().Get("error_description")),
		)
		http.Redirect(w, r, "/login?error=provider_error", http.StatusFound)
		return
	}

	if state == "" || code == "" {
		http.Redirect(w, r, "/login?error=provider_error", http.StatusFound)
		return
	}

	// Validate and consume the single-use state token.
	ok, err := h.states.Consume(r.Context(), state)
	if err != nil || !ok {
		if err != nil {
			slog.Error("oidc: consume state", slog.Any("error", err))
		}
		http.Redirect(w, r, "/login?error=provider_error", http.StatusFound)
		return
	}

	// Exchange the authorization code for tokens.
	token, err := h.oauth2Config.Exchange(r.Context(), code)
	if err != nil {
		slog.Warn("oidc: code exchange failed", slog.Any("error", err))
		http.Redirect(w, r, "/login?error=provider_error", http.StatusFound)
		return
	}

	// Extract and verify the ID token.
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		slog.Warn("oidc: no id_token in token response")
		http.Redirect(w, r, "/login?error=provider_error", http.StatusFound)
		return
	}

	verifier := h.provider.Verifier(&gooidc.Config{ClientID: h.oauth2Config.ClientID})
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		slog.Warn("oidc: id_token verification failed", slog.Any("error", err))
		http.Redirect(w, r, "/login?error=provider_error", http.StatusFound)
		return
	}

	// Decode standard claims.
	var stdClaims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&stdClaims); err != nil {
		slog.Warn("oidc: failed to decode id_token claims", slog.Any("error", err))
		http.Redirect(w, r, "/login?error=provider_error", http.StatusFound)
		return
	}

	// Decode all claims for admin filter evaluation.
	var allClaims map[string]interface{}
	_ = idToken.Claims(&allClaims)

	// Resolve or create the local user account.
	user, err := h.resolveOIDCUser(r.Context(), idToken.Subject, stdClaims.Email, stdClaims.Name)
	if errors.Is(err, errSignupDisabled) {
		http.Redirect(w, r, "/login?error=signup_disabled", http.StatusFound)
		return
	}
	if err != nil {
		slog.Error("oidc: resolve user", slog.Any("error", err))
		http.Redirect(w, r, "/login?error=provider_error", http.StatusFound)
		return
	}

	// Apply admin filter: upgrade to admin if a filter matches.
	// Never auto-demote: a user who was manually made admin stays admin.
	if user.Role != model.RoleAdmin && h.isAdminByFilter(stdClaims.Email, allClaims) {
		user.Role = model.RoleAdmin
		if err := h.users.Update(r.Context(), user); err != nil {
			slog.Warn("oidc: failed to set admin role", slog.Any("error", err))
		}
	}

	// Create a standard login session.
	session, err := h.sessions.CreateWithExpiry(r.Context(), user.ID, time.Now().Add(sessionExpiryDefault), false)
	if err != nil {
		slog.Error("oidc: create session", slog.Any("error", err))
		http.Redirect(w, r, "/login?error=provider_error", http.StatusFound)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   isSecureEnvironment(),
		SameSite: http.SameSiteLaxMode,
	})

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionSessionLogin, audit.ResourceSession, nil,
			map[string]interface{}{"method": "oidc", "email": user.Email})
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// resolveOIDCUser finds an existing user by OIDC subject, falls back to
// email lookup and links the subject, or creates a new account when signup
// is enabled. Returns errSignupDisabled if the user is new and signup is off.
func (h *OIDCHandler) resolveOIDCUser(ctx context.Context, subject, email, name string) (*model.User, error) {
	issuer := h.cfg.Issuer

	// 1. Primary lookup: OIDC subject (survives email changes).
	user, err := h.users.GetByOIDCSubject(ctx, subject, issuer)
	if err == nil {
		return user, nil
	}

	// 2. Email fallback: link subject to an existing account.
	if email != "" {
		user, err = h.users.GetByEmail(ctx, email)
		if err == nil {
			if linkErr := h.users.SetOIDCSubject(ctx, user.ID, subject, issuer); linkErr != nil {
				slog.Warn("oidc: failed to link subject to existing user", slog.Any("error", linkErr))
			}
			user.OIDCSubject = &subject
			user.OIDCIssuer = &issuer
			return user, nil
		}
	}

	// 3. New account.
	if !h.cfg.SignupEnabled {
		return nil, errSignupDisabled
	}

	displayName := name
	if displayName == "" {
		displayName = email
	}
	user, err = h.users.CreateOIDCUser(ctx, email, displayName, model.RoleUser, subject, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: create user: %w", err)
	}
	slog.Info("oidc: new user registered", slog.String("email", email))
	return user, nil
}

// isAdminByFilter returns true if either the email regex or the claim filter
// matches, indicating that this user should receive the admin role.
func (h *OIDCHandler) isAdminByFilter(email string, allClaims map[string]interface{}) bool {
	// Email regex check.
	if h.adminEmailRegex != nil && h.adminEmailRegex.MatchString(email) {
		return true
	}

	// Claim check.
	if h.cfg.AdminClaim != "" && h.cfg.AdminClaimValue != "" {
		claimVal, ok := allClaims[h.cfg.AdminClaim]
		if ok {
			switch v := claimVal.(type) {
			case string:
				if v == h.cfg.AdminClaimValue {
					return true
				}
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok && s == h.cfg.AdminClaimValue {
						return true
					}
				}
			}
		}
	}

	return false
}
