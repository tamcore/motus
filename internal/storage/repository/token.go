package repository

import (
	"encoding/hex"

	"golang.org/x/crypto/argon2"
)

const (
	tokenHashSalt           = "motus-token-hash-v1" // #nosec G101 -- fixed Argon2id salt for deterministic token lookup, not a credential
	tokenHashTime    uint32 = 1
	tokenHashMemory  uint32 = 8 * 1024
	tokenHashThreads uint8  = 1
	tokenHashLen     uint32 = 32
)

// hashToken returns a deterministic Argon2id hash of the raw token string.
// The fixed salt keeps lookups deterministic while avoiding plaintext storage.
func hashToken(raw string) string {
	h := argon2.IDKey([]byte(raw), []byte(tokenHashSalt), tokenHashTime, tokenHashMemory, tokenHashThreads, tokenHashLen)
	return hex.EncodeToString(h)
}

// HashToken exposes hashToken for callers that seed token columns directly
// (e.g. demo data reset), so the stored value matches what GetByToken looks up.
func HashToken(raw string) string {
	return hashToken(raw)
}
