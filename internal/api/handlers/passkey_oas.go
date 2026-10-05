package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gorilla/securecookie"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/demo"
	"github.com/tamcore/motus/internal/model"
)

const (
	// passkeyRegCookie / passkeyLoginCookie carry the signed WebAuthn
	// SessionData between the begin and finish steps of a ceremony.
	passkeyRegCookie   = "passkey_reg_session"   // #nosec G101 -- cookie name, not a credential
	passkeyLoginCookie = "passkey_login_session" // #nosec G101 -- cookie name, not a credential
	// defaultPasskeyName labels a passkey when the client sends no name.
	defaultPasskeyName = "Passkey"
)

// passkeyChallengeTTL bounds how long a ceremony may take.
var passkeyChallengeTTL = 5 * time.Minute

// PasskeyRegisterBegin starts a passkey registration for the authenticated user.
func (h *Handler) PasskeyRegisterBegin(ctx context.Context) (oas.PasskeyRegisterBeginRes, error) {
	if h.cfg.WebAuthn == nil {
		return &oas.PasskeyRegisterBeginNotImplemented{Error: "passkeys are not enabled"}, nil
	}
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.PasskeyRegisterBeginUnauthorized{Error: "not authenticated"}, nil
	}

	wu, err := h.loadWebauthnUser(ctx, user)
	if err != nil {
		return &oas.PasskeyRegisterBeginUnauthorized{Error: "failed to load user"}, nil
	}

	creation, sessionData, err := h.cfg.WebAuthn.BeginRegistration(wu,
		webauthn.WithExclusions(webauthn.Credentials(wu.WebAuthnCredentials()).CredentialDescriptors()),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
	)
	if err != nil {
		return &oas.PasskeyRegisterBeginUnauthorized{Error: "failed to begin registration"}, nil
	}

	if err := h.setChallengeCookie(ctx, passkeyRegCookie, sessionData); err != nil {
		return &oas.PasskeyRegisterBeginUnauthorized{Error: "failed to start ceremony"}, nil
	}

	// Return the inner PublicKeyCredentialCreationOptions (creation.Response),
	// not the {"publicKey": ...} wrapper: the browser's
	// PublicKeyCredential.parseCreationOptionsFromJSON expects the options object directly.
	opts, err := toRawObject[oas.WebAuthnCredentialCreationOptions](creation.Response)
	if err != nil {
		return &oas.PasskeyRegisterBeginUnauthorized{Error: "failed to encode options"}, nil
	}
	return &opts, nil
}

// PasskeyRegisterFinish verifies the attestation and stores the new credential.
func (h *Handler) PasskeyRegisterFinish(ctx context.Context, req oas.WebAuthnAttestationResponse, params oas.PasskeyRegisterFinishParams) (oas.PasskeyRegisterFinishRes, error) {
	if h.cfg.WebAuthn == nil {
		return &oas.PasskeyRegisterFinishNotImplemented{Error: "passkeys are not enabled"}, nil
	}
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.PasskeyRegisterFinishUnauthorized{Error: "not authenticated"}, nil
	}

	sessionData, err := h.consumeChallengeCookie(ctx, passkeyRegCookie)
	if err != nil {
		return &oas.PasskeyRegisterFinishBadRequest{Error: "registration session expired; please try again"}, nil
	}

	body, _ := req.MarshalJSON()
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	if err != nil {
		return &oas.PasskeyRegisterFinishBadRequest{Error: "invalid attestation"}, nil
	}

	wu, err := h.loadWebauthnUser(ctx, user)
	if err != nil {
		return &oas.PasskeyRegisterFinishBadRequest{Error: "failed to load user"}, nil
	}

	cred, err := h.cfg.WebAuthn.CreateCredential(wu, *sessionData, parsed)
	if err != nil {
		return &oas.PasskeyRegisterFinishBadRequest{Error: "attestation verification failed"}, nil
	}

	name := strings.TrimSpace(params.Name.Or(""))
	if name == "" {
		name = defaultPasskeyName
	}

	mc := fromWebauthnCredential(user.ID, cred, name)
	if err := h.cfg.Passkeys.Create(ctx, mc); err != nil {
		return &oas.PasskeyRegisterFinishBadRequest{Error: "failed to store passkey"}, nil
	}

	h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionUserUpdate, audit.ResourceUser, &user.ID,
		map[string]any{"passkeyRegistered": name})

	return new(passkeyToOAS(mc)), nil
}

// PasskeyLoginBegin starts a discoverable passkey login. No user identifier is
// required; the authenticator selects the resident credential.
func (h *Handler) PasskeyLoginBegin(ctx context.Context) (oas.PasskeyLoginBeginRes, error) {
	if h.cfg.WebAuthn == nil {
		return &oas.Error{Error: "passkeys are not enabled"}, nil
	}

	assertion, sessionData, err := h.cfg.WebAuthn.BeginDiscoverableLogin()
	if err != nil {
		return &oas.Error{Error: "failed to begin login"}, nil
	}

	if err := h.setChallengeCookie(ctx, passkeyLoginCookie, sessionData); err != nil {
		return &oas.Error{Error: "failed to start ceremony"}, nil
	}

	// Return the inner PublicKeyCredentialRequestOptions (assertion.Response),
	// not the {"publicKey": ...} wrapper: the browser's
	// PublicKeyCredential.parseRequestOptionsFromJSON expects the options object directly.
	opts, err := toRawObject[oas.WebAuthnCredentialRequestOptions](assertion.Response)
	if err != nil {
		return &oas.Error{Error: "failed to encode options"}, nil
	}
	return &opts, nil
}

// PasskeyLoginFinish verifies the assertion, resolves the user, and establishes
// a session. For demo accounts the session is linked to the account's read-only
// API key so demo read-only enforcement carries over to passkey logins.
func (h *Handler) PasskeyLoginFinish(ctx context.Context, req oas.WebAuthnAssertionResponse) (oas.PasskeyLoginFinishRes, error) {
	if h.cfg.WebAuthn == nil {
		return &oas.PasskeyLoginFinishNotImplemented{Error: "passkeys are not enabled"}, nil
	}

	sessionData, err := h.consumeChallengeCookie(ctx, passkeyLoginCookie)
	if err != nil {
		return &oas.PasskeyLoginFinishUnauthorized{Error: "login session expired; please try again"}, nil
	}

	body, _ := req.MarshalJSON()
	parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
	if err != nil {
		return &oas.PasskeyLoginFinishUnauthorized{Error: "invalid assertion"}, nil
	}

	// The discoverable handler resolves the raw credential/user handle to a
	// motus user. Prefer the user handle (our 8-byte user id), then fall back
	// to a credential-ID lookup.
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		if id, ok := decodeUserHandle(userHandle); ok {
			if u, err := h.userWithCredentials(ctx, id); err == nil {
				return u, nil
			}
		}
		stored, err := h.cfg.Passkeys.GetByCredentialID(ctx, rawID)
		if err != nil {
			return nil, err
		}
		return h.userWithCredentials(ctx, stored.UserID)
	}

	wuUser, cred, err := h.cfg.WebAuthn.ValidatePasskeyLogin(handler, *sessionData, parsed)
	if err != nil {
		return &oas.PasskeyLoginFinishUnauthorized{Error: "authentication failed"}, nil
	}

	wu, ok := wuUser.(*webauthnUser)
	if !ok {
		return &oas.PasskeyLoginFinishUnauthorized{Error: "authentication failed"}, nil
	}
	user := wu.user

	// Persist the updated signature counter and surface clone-detection warnings.
	// go-webauthn sets CloneWarning when the authenticator-reported counter did
	// not increase past the stored value — the WebAuthn signal for a possibly
	// cloned credential. We do not fail the login (synced passkeys legitimately
	// report a static 0 counter) but we log and audit it so it is not silent.
	if stored, err := h.cfg.Passkeys.GetByCredentialID(ctx, cred.ID); err != nil {
		slog.Warn("passkey: could not load credential to update sign count", slog.Any("error", err))
	} else {
		if err := h.cfg.Passkeys.UpdateSignCount(ctx, stored.ID, cred.Authenticator.SignCount); err != nil {
			slog.Warn("passkey: failed to persist sign count",
				slog.Int64("credentialId", stored.ID), slog.Any("error", err))
		}
		if cred.Authenticator.CloneWarning {
			slog.Warn("passkey: authenticator clone warning — signature counter did not increase",
				slog.Int64("userId", user.ID), slog.Int64("credentialId", stored.ID))
			h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionSessionLogin, audit.ResourceSession, nil,
				map[string]any{"method": "passkey", "cloneWarning": true})
		}
	}

	session, err := h.createPasskeySession(ctx, user)
	if err != nil {
		return &oas.PasskeyLoginFinishUnauthorized{Error: err.Error()}, nil
	}

	h.setSessionCookie(ctx, session.ID, session.ExpiresAt)

	h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionSessionLogin, audit.ResourceSession, nil,
		map[string]any{"method": "passkey"})

	out := userToOAS(user)
	return &out, nil
}

// createPasskeySession creates a session for a passkey login. Demo accounts are
// linked to their read-only API key so read-only enforcement carries over; if
// that key is missing the login fails closed rather than granting full access.
func (h *Handler) createPasskeySession(ctx context.Context, user *model.User) (*model.Session, error) {
	expiry := time.Now().Add(api.SessionExpiryRememberMe)

	if demo.IsDemoAccount(user.Email) {
		token, _, _ := strings.Cut(user.Email, "@")
		apiKey, err := h.cfg.ApiKeys.GetByToken(ctx, token)
		if err != nil || apiKey == nil {
			return nil, errPasskeyDemoUnavailable
		}
		return h.cfg.Sessions.CreateWithApiKey(ctx, user.ID, apiKey.ID, expiry, true)
	}
	return h.cfg.Sessions.CreateWithExpiry(ctx, user.ID, expiry, true)
}

// ListPasskeys returns the authenticated user's registered passkeys.
func (h *Handler) ListPasskeys(ctx context.Context) (oas.ListPasskeysRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "not authenticated"}, nil
	}
	creds, err := h.cfg.Passkeys.ListByUser(ctx, user.ID)
	if err != nil {
		return &oas.Error{Error: "failed to list passkeys"}, nil
	}
	return new(mapSlice[oas.ListPasskeysOKApplicationJSON](creds, passkeyToOAS)), nil
}

// DeletePasskey removes one of the authenticated user's passkeys.
func (h *Handler) DeletePasskey(ctx context.Context, params oas.DeletePasskeyParams) (oas.DeletePasskeyRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.DeletePasskeyUnauthorized{Error: "not authenticated"}, nil
	}
	if err := h.cfg.Passkeys.Delete(ctx, params.ID, user.ID); err != nil {
		return &oas.DeletePasskeyNotFound{Error: "passkey not found"}, nil
	}
	h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionUserUpdate, audit.ResourceUser, &user.ID,
		map[string]any{"passkeyDeleted": params.ID})
	return &oas.DeletePasskeyNoContent{}, nil
}

var (
	// errPasskeyDemoUnavailable is returned when a demo account has no read-only
	// API key to bind the session to. Failing closed prevents a full-access
	// demo session.
	errPasskeyDemoUnavailable = errors.New("demo passkey login is temporarily unavailable")
	errPasskeyNoWriter        = errors.New("no response writer in context")
	errPasskeyNoRequest       = errors.New("no request in context")
)

// loadWebauthnUser builds a webauthn.User adapter for the given user with all
// their stored credentials.
func (h *Handler) loadWebauthnUser(ctx context.Context, user *model.User) (*webauthnUser, error) {
	creds, err := h.cfg.Passkeys.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return &webauthnUser{user: user, creds: creds}, nil
}

// userWithCredentials loads a user by ID plus their credentials as a
// webauthn.User (used by the discoverable-login resolver).
func (h *Handler) userWithCredentials(ctx context.Context, userID int64) (*webauthnUser, error) {
	user, err := h.cfg.Users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return h.loadWebauthnUser(ctx, user)
}

// passkeyToOAS converts a stored credential to its API representation.
func passkeyToOAS(c *model.PasskeyCredential) oas.PasskeyCredentialInfo {
	return oas.PasskeyCredentialInfo{
		ID:         c.ID,
		Name:       c.Name,
		CreatedAt:  c.CreatedAt,
		LastUsedAt: ptrToOptTime(c.LastUsedAt),
	}
}

// challengeCodec signs challenge cookies and rejects them after passkeyChallengeTTL.
func (h *Handler) challengeCodec() *securecookie.SecureCookie {
	return securecookie.New(h.cfg.WebAuthnCookieKey, nil).
		MaxAge(int(passkeyChallengeTTL.Seconds())).
		SetSerializer(securecookie.JSONEncoder{})
}

// setChallengeCookie serializes and signs the WebAuthn SessionData into a
// short-lived HttpOnly cookie.
func (h *Handler) setChallengeCookie(ctx context.Context, name string, sd *webauthn.SessionData) error {
	w := api.ResponseWriterFromContext(ctx)
	if w == nil {
		return errPasskeyNoWriter
	}
	value, err := h.challengeCodec().Encode(name, sd)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/api/session/passkey/",
		MaxAge:   int(passkeyChallengeTTL.Seconds()),
		HttpOnly: true,
		Secure:   !h.cfg.Development,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// consumeChallengeCookie reads, verifies, and clears a challenge cookie.
func (h *Handler) consumeChallengeCookie(ctx context.Context, name string) (*webauthn.SessionData, error) {
	r := api.RequestFromContext(ctx)
	if r == nil {
		return nil, errPasskeyNoRequest
	}
	cookie, err := r.Cookie(name)
	if err != nil {
		return nil, err
	}

	// Clear the cookie regardless of outcome (single-use).
	if w := api.ResponseWriterFromContext(ctx); w != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/api/session/passkey/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   !h.cfg.Development,
			SameSite: http.SameSiteLaxMode,
		})
	}

	var sd webauthn.SessionData
	if err := h.challengeCodec().Decode(name, cookie.Value, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}
