package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/tamcore/motus/internal/model"
)

type contextKey string

const userContextKey contextKey = "user"
const apiKeyContextKey contextKey = "apiKey"

// RespondJSON writes a JSON response with the given status code.
func RespondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// RespondError writes a JSON error response.
func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, map[string]string{"error": message})
}

// UserFromContext extracts the authenticated user from the request context.
func UserFromContext(ctx context.Context) *model.User {
	u, _ := ctx.Value(userContextKey).(*model.User)
	return u
}

// ContextWithUser returns a new context with the user stored in it.
func ContextWithUser(ctx context.Context, user *model.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// ApiKeyFromContext extracts the API key from the request context, if present.
// Returns nil when the request was authenticated via session cookie rather
// than an API key.
func ApiKeyFromContext(ctx context.Context) *model.ApiKey {
	k, _ := ctx.Value(apiKeyContextKey).(*model.ApiKey)
	return k
}

// ContextWithApiKey returns a new context with the API key stored in it.
func ContextWithApiKey(ctx context.Context, key *model.ApiKey) context.Context {
	return context.WithValue(ctx, apiKeyContextKey, key)
}

const sessionContextKey contextKey = "session"

// ContextWithSession returns a new context with the session stored in it.
func ContextWithSession(ctx context.Context, session *model.Session) context.Context {
	return context.WithValue(ctx, sessionContextKey, session)
}

// SessionFromContext extracts the current session from the context.
// Returns nil when the request was authenticated via API key rather than session.
func SessionFromContext(ctx context.Context) *model.Session {
	s, _ := ctx.Value(sessionContextKey).(*model.Session)
	return s
}

const responseWriterContextKey contextKey = "responseWriter"

// ContextWithResponseWriter returns a new context with the http.ResponseWriter
// stored in it. This is used to allow ogen handlers (which receive only a
// context.Context) to set cookies or headers on the response.
func ContextWithResponseWriter(ctx context.Context, w http.ResponseWriter) context.Context {
	return context.WithValue(ctx, responseWriterContextKey, w)
}

// ResponseWriterFromContext extracts the http.ResponseWriter from the context.
// Returns nil when no writer has been stored (e.g. in unit tests).
func ResponseWriterFromContext(ctx context.Context) http.ResponseWriter {
	w, _ := ctx.Value(responseWriterContextKey).(http.ResponseWriter)
	return w
}
