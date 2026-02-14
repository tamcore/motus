package serve

import (
	"encoding/hex"
	"testing"
)

func TestParseCSRFSecret_Valid(t *testing.T) {
	// 32 random bytes encoded as hex
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	hexStr := hex.EncodeToString(raw)

	got, err := parseCSRFSecret(hexStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 32 {
		t.Errorf("got %d bytes, want 32", len(got))
	}
	for i, b := range got {
		if b != raw[i] {
			t.Errorf("byte[%d] = %d, want %d", i, b, raw[i])
		}
	}
}

func TestParseCSRFSecret_InvalidHex(t *testing.T) {
	_, err := parseCSRFSecret("not-valid-hex!!!")
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestParseCSRFSecret_WrongLength(t *testing.T) {
	// 16 bytes (too short)
	raw := make([]byte, 16)
	hexStr := hex.EncodeToString(raw)
	_, err := parseCSRFSecret(hexStr)
	if err == nil {
		t.Error("expected error for wrong length")
	}
}

func TestParseCSRFSecret_TooLong(t *testing.T) {
	// 64 bytes (too long)
	raw := make([]byte, 64)
	hexStr := hex.EncodeToString(raw)
	_, err := parseCSRFSecret(hexStr)
	if err == nil {
		t.Error("expected error for too-long secret")
	}
}

func TestLoadCSRFSecret_Empty(t *testing.T) {
	// Empty string should generate a random key without panic.
	secret := loadCSRFSecret("")
	if len(secret) != 32 {
		t.Errorf("got %d bytes, want 32", len(secret))
	}
}
