package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
)

func TestUserHandleRoundTrip(t *testing.T) {
	for _, id := range []int64{0, 1, 42, 9_000_000_000, 1<<62 + 7} {
		h := encodeUserHandle(id)
		if len(h) != 8 {
			t.Fatalf("handle for %d has length %d, want 8", id, len(h))
		}
		got, ok := decodeUserHandle(h)
		if !ok || got != id {
			t.Fatalf("round trip for %d: got %d ok=%v", id, got, ok)
		}
	}
}

func TestDecodeUserHandleRejectsWrongLength(t *testing.T) {
	if _, ok := decodeUserHandle([]byte{1, 2, 3}); ok {
		t.Fatal("expected decode of 3-byte handle to fail")
	}
}

// TestRawObjectRoundTrip confirms a JSON object survives the
// value -> ogen free-form map -> JSON passthrough unchanged.
func TestRawObjectRoundTrip(t *testing.T) {
	src := map[string]any{
		"challenge": "abc123",
		"rp":        map[string]any{"id": "example.com", "name": "Motus"},
	}
	opts, err := toRawObject[oas.WebAuthnCredentialCreationOptions](src)
	if err != nil {
		t.Fatalf("toRawObject: %v", err)
	}
	if _, ok := opts["challenge"]; !ok {
		t.Fatalf("expected challenge key, got %v", opts)
	}

	b, err := oas.WebAuthnAttestationResponse(opts).MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, src) {
		t.Fatalf("round trip: got %v want %v", got, src)
	}
}

func TestChallengeCookieRoundTrip(t *testing.T) {
	h := &Handler{cfg: HandlerConfig{WebAuthnCookieKey: []byte("test-key-32-bytes-long-padding!!")}}

	sd := &webauthn.SessionData{
		Challenge: "test-challenge",
		UserID:    []byte{0, 0, 0, 0, 0, 0, 0, 1},
		Expires:   time.Now().Add(5 * time.Minute),
	}

	rec := httptest.NewRecorder()
	setCtx := api.ContextWithResponseWriter(t.Context(), rec)
	if err := h.setChallengeCookie(setCtx, passkeyRegCookie, sd); err != nil {
		t.Fatalf("setChallengeCookie: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookie set")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/session/passkey/register/finish", nil)
	req.AddCookie(cookies[0])
	useCtx := api.ContextWithResponseWriter(api.ContextWithRequest(t.Context(), req), httptest.NewRecorder())

	got, err := h.consumeChallengeCookie(useCtx, passkeyRegCookie)
	if err != nil {
		t.Fatalf("consumeChallengeCookie: %v", err)
	}
	if got.Challenge != sd.Challenge {
		t.Fatalf("challenge mismatch: got %q want %q", got.Challenge, sd.Challenge)
	}
}

func TestChallengeCookieTamperRejected(t *testing.T) {
	h := &Handler{cfg: HandlerConfig{WebAuthnCookieKey: []byte("test-key-32-bytes-long-padding!!")}}
	sd := &webauthn.SessionData{Challenge: "x", Expires: time.Now().Add(time.Minute)}

	rec := httptest.NewRecorder()
	_ = h.setChallengeCookie(api.ContextWithResponseWriter(t.Context(), rec), passkeyRegCookie, sd)
	cookie := rec.Result().Cookies()[0]

	cookie.Value = cookie.Value[:len(cookie.Value)-1] + "X"

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.AddCookie(cookie)
	ctx := api.ContextWithResponseWriter(api.ContextWithRequest(t.Context(), req), httptest.NewRecorder())

	if _, err := h.consumeChallengeCookie(ctx, passkeyRegCookie); err == nil {
		t.Fatal("expected tampered cookie to be rejected")
	}
}

func TestChallengeCookieWrongKeyRejected(t *testing.T) {
	writer := &Handler{cfg: HandlerConfig{WebAuthnCookieKey: []byte("key-A-aaaaaaaaaaaaaaaaaaaaaaaaaa")}}
	reader := &Handler{cfg: HandlerConfig{WebAuthnCookieKey: []byte("key-B-bbbbbbbbbbbbbbbbbbbbbbbbbb")}}
	sd := &webauthn.SessionData{Challenge: "x", Expires: time.Now().Add(time.Minute)}

	rec := httptest.NewRecorder()
	_ = writer.setChallengeCookie(api.ContextWithResponseWriter(t.Context(), rec), passkeyRegCookie, sd)
	cookie := rec.Result().Cookies()[0]

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.AddCookie(cookie)
	ctx := api.ContextWithResponseWriter(api.ContextWithRequest(t.Context(), req), httptest.NewRecorder())

	if _, err := reader.consumeChallengeCookie(ctx, passkeyRegCookie); err == nil {
		t.Fatal("expected cookie signed with a different key to be rejected")
	}
}

func TestChallengeCookieExpiredRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the cookie max age to pass")
	}
	ttl := passkeyChallengeTTL
	passkeyChallengeTTL = time.Second
	t.Cleanup(func() { passkeyChallengeTTL = ttl })

	h := &Handler{cfg: HandlerConfig{WebAuthnCookieKey: []byte("test-key-32-bytes-long-padding!!")}}
	sd := &webauthn.SessionData{Challenge: "x", Expires: time.Now().Add(time.Minute)}

	rec := httptest.NewRecorder()
	if err := h.setChallengeCookie(api.ContextWithResponseWriter(t.Context(), rec), passkeyRegCookie, sd); err != nil {
		t.Fatalf("setChallengeCookie: %v", err)
	}
	cookie := rec.Result().Cookies()[0]

	time.Sleep(2100 * time.Millisecond)

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.AddCookie(cookie)
	ctx := api.ContextWithResponseWriter(api.ContextWithRequest(t.Context(), req), httptest.NewRecorder())

	if _, err := h.consumeChallengeCookie(ctx, passkeyRegCookie); err == nil {
		t.Fatal("expected expired cookie to be rejected")
	}
}
