package repository

import (
	"encoding/hex"

	"golang.org/x/crypto/argon2"
)

const (
	tokenHashSalt           = "motus-token-hash-v1"
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
