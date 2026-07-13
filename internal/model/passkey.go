package model

import "time"

// PasskeyCredential represents a single WebAuthn/FIDO2 credential (passkey)
// registered by a user. The binary fields (CredentialID, PublicKey, AAGUID)
// are stored raw; encoding for transport is handled at the API boundary.
//
// This type is intentionally free of any WebAuthn library dependency so the
// model package stays dependency-free; the handlers package adapts it to the
// webauthn.Credential shape.
type PasskeyCredential struct {
	ID              int64
	UserID          int64
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	AAGUID          []byte
	SignCount       uint32
	Transports      []string
	BackupEligible  bool
	BackupState     bool
	Name            string
	CreatedAt       time.Time
	LastUsedAt      *time.Time
}
