package middleware_test

// Tests that goroutines spawned by the auth middleware to update session/key
// timestamps inherit values from the request context rather than using a bare
// context.Background() that would lose trace spans and request IDs.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/model"
)

type authCtxKey struct{}

// TestAuth_SessionGoroutine_InheritsRequestContext verifies that the goroutine
// spawned to update session timestamps receives a context that inherits values
// from the original request context (not a bare context.Background()).
func TestAuth_SessionGoroutine_InheritsRequestContext(t *testing.T) {
	const sentinel = "trace-id-abc123"
	baseCtx, cancelBase := context.WithCancel(
		context.WithValue(context.Background(), authCtxKey{}, sentinel),
	)
	defer cancelBase()

	gotCtx := make(chan context.Context, 1)

	sessions := &mockSessionRepo{
		getByIDFn: func(_ context.Context, id string) (*model.Session, error) {
			return &model.Session{
				ID:        id,
				UserID:    1,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
		updateLastSeenFn: func(ctx context.Context, _, _, _ string) error {
			select {
			case gotCtx <- ctx:
			default:
			}
			return nil
		},
	}
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 1, Email: "test@example.com"}, nil
		},
	}

	mw := middleware.LoadAuthContext(users, sessions, nil)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(baseCtx)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sess-abc"})
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// Cancel the originating request context to simulate connection close.
	cancelBase()

	select {
	case ctx := <-gotCtx:
		// The goroutine context must inherit values from the request context.
		if got := ctx.Value(authCtxKey{}); got != sentinel {
			t.Errorf("goroutine ctx missing request value: got %v, want %q", got, sentinel)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: UpdateLastSeen goroutine did not run within 1s")
	}
}
