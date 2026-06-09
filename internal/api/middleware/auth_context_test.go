package middleware_test

// Tests that goroutines spawned by the auth middleware to update session/key
// timestamps inherit values from the request context rather than using a bare
// context.Background() that would lose trace spans and request IDs.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

type authCtxKey struct{}

// stubSessionRepo is a minimal SessionRepo for auth context tests.
type stubSessionRepo struct {
	getByIDFn        func(ctx context.Context, id string) (*model.Session, error)
	updateLastSeenFn func(ctx context.Context, id, ip, ua string) error
	updateExpiryFn   func(ctx context.Context, id string, t time.Time) error
}

var _ repository.SessionRepo = (*stubSessionRepo)(nil)

func (s *stubSessionRepo) Create(_ context.Context, _ int64) (*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (s *stubSessionRepo) CreateWithExpiry(_ context.Context, _ int64, _ time.Time, _ bool) (*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (s *stubSessionRepo) CreateWithApiKey(_ context.Context, _ int64, _ int64, _ time.Time, _ bool) (*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (s *stubSessionRepo) CreateSudo(_ context.Context, _, _ int64) (*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (s *stubSessionRepo) GetByID(ctx context.Context, id string) (*model.Session, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (s *stubSessionRepo) GetByIDPrefix(_ context.Context, _ int64, _ string) (*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (s *stubSessionRepo) Delete(_ context.Context, _ string) error { return nil }
func (s *stubSessionRepo) ListByUser(_ context.Context, _ int64) ([]*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (s *stubSessionRepo) UpdateLastSeen(ctx context.Context, id, ip, ua string) error {
	if s.updateLastSeenFn != nil {
		return s.updateLastSeenFn(ctx, id, ip, ua)
	}
	return nil
}
func (s *stubSessionRepo) UpdateExpiry(ctx context.Context, id string, t time.Time) error {
	if s.updateExpiryFn != nil {
		return s.updateExpiryFn(ctx, id, t)
	}
	return nil
}
func (s *stubSessionRepo) DeleteAllByUser(_ context.Context, _ int64, _ string) error { return nil }

// stubUserRepo is a minimal UserRepo for auth context tests.
type stubUserRepo struct {
	getByIDFn func(ctx context.Context, id int64) (*model.User, error)
}

var _ repository.UserRepo = (*stubUserRepo)(nil)

func (u *stubUserRepo) Create(_ context.Context, _ *model.User) error {
	return errors.New("not impl")
}
func (u *stubUserRepo) CreateOIDCUser(_ context.Context, _, _, _, _, _ string) (*model.User, error) {
	return nil, errors.New("not impl")
}
func (u *stubUserRepo) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not impl")
}
func (u *stubUserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	if u.getByIDFn != nil {
		return u.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (u *stubUserRepo) GetByToken(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not impl")
}
func (u *stubUserRepo) GetByOIDCSubject(_ context.Context, _, _ string) (*model.User, error) {
	return nil, errors.New("not impl")
}
func (u *stubUserRepo) SetOIDCSubject(_ context.Context, _ int64, _, _ string) error { return nil }
func (u *stubUserRepo) ListAll(_ context.Context) ([]*model.User, error)             { return nil, nil }
func (u *stubUserRepo) Update(_ context.Context, _ *model.User) error                { return nil }
func (u *stubUserRepo) UpdatePassword(_ context.Context, _ int64, _ string) error    { return nil }
func (u *stubUserRepo) Delete(_ context.Context, _ int64) error                      { return nil }
func (u *stubUserRepo) GetDevicesForUser(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (u *stubUserRepo) AssignDevice(_ context.Context, _, _ int64) error   { return nil }
func (u *stubUserRepo) UnassignDevice(_ context.Context, _, _ int64) error { return nil }
func (u *stubUserRepo) GenerateToken(_ context.Context, _ int64) (string, error) {
	return "", errors.New("not impl")
}

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

	sessions := &stubSessionRepo{
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
	users := &stubUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 1, Email: "test@example.com"}, nil
		},
	}

	mw := middleware.Auth(users, sessions, nil)
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
