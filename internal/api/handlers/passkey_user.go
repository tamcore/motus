package handlers

import (
	"encoding/binary"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/tamcore/motus/internal/model"
)

// webauthnUser adapts a *model.User plus its stored credentials to the
// webauthn.User interface expected by the go-webauthn library.
type webauthnUser struct {
	user  *model.User
	creds []*model.PasskeyCredential
}

// WebAuthnID returns a stable opaque handle for the user, derived from the
// user's database ID (8-byte big-endian). Deriving it from the immutable,
// never-reused users.id avoids storing a separate handle column.
func (u *webauthnUser) WebAuthnID() []byte {
	return encodeUserHandle(u.user.ID)
}

func (u *webauthnUser) WebAuthnName() string        { return u.user.Email }
func (u *webauthnUser) WebAuthnDisplayName() string { return u.user.Name }

// WebAuthnCredentials maps the stored credentials to webauthn.Credential.
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(u.creds))
	for _, c := range u.creds {
		out = append(out, toWebauthnCredential(c))
	}
	return out
}

// encodeUserHandle encodes a user ID as an 8-byte big-endian WebAuthn handle.
func encodeUserHandle(id int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}

// decodeUserHandle reverses encodeUserHandle. ok is false when the handle is
// not the expected 8-byte length.
func decodeUserHandle(h []byte) (id int64, ok bool) {
	if len(h) != 8 {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(h)), true
}

// toWebauthnCredential converts a stored credential to the library type.
func toWebauthnCredential(c *model.PasskeyCredential) webauthn.Credential {
	transports := make([]protocol.AuthenticatorTransport, 0, len(c.Transports))
	for _, t := range c.Transports {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}
	return webauthn.Credential{
		ID:              c.CredentialID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Transport:       transports,
		Flags: webauthn.CredentialFlags{
			BackupEligible: c.BackupEligible,
			BackupState:    c.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:    c.AAGUID,
			SignCount: c.SignCount,
		},
	}
}

// fromWebauthnCredential builds a storable model credential from a freshly
// created library credential owned by userID.
func fromWebauthnCredential(userID int64, cred *webauthn.Credential, name string) *model.PasskeyCredential {
	transports := make([]string, 0, len(cred.Transport))
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	return &model.PasskeyCredential{
		UserID:          userID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		AAGUID:          cred.Authenticator.AAGUID,
		SignCount:       cred.Authenticator.SignCount,
		Transports:      transports,
		BackupEligible:  cred.Flags.BackupEligible,
		BackupState:     cred.Flags.BackupState,
		Name:            name,
	}
}
