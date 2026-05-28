package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestHashToken_IsHexEncoded(t *testing.T) {
	raw := "test-token-value"
	h := hashToken(raw)

	if len(h) != 64 {
		t.Errorf("expected 64 hex chars, got %d: %s", len(h), h)
	}
	if strings.ToLower(h) != h {
		t.Errorf("expected lowercase hex, got %s", h)
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	raw := "some-api-key"
	h1 := hashToken(raw)
	h2 := hashToken(raw)
	if h1 != h2 {
		t.Errorf("hashToken should be deterministic: %s != %s", h1, h2)
	}
}

func TestHashToken_DifferentInputsDifferentOutputs(t *testing.T) {
	if hashToken("token-a") == hashToken("token-b") {
		t.Error("different inputs should produce different hashes")
	}
}

func TestHashToken_NotPlaintext(t *testing.T) {
	raw := "my-secret-token"
	h := hashToken(raw)
	if h == raw {
		t.Error("hash should not equal the raw token")
	}
}

func TestHashToken_NotRawSHA256(t *testing.T) {
	raw := "my-secret-token"
	legacy := sha256.Sum256([]byte(raw))
	if hashToken(raw) == hex.EncodeToString(legacy[:]) {
		t.Error("hashToken should not use a plain SHA-256 digest")
	}
}
