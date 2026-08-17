package protocol

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"
)

// v2Channel is the fixed rendezvous channel for token-derived sessions. One
// token maps to exactly one opaque route, which the relay groups as one
// target plus any number of edges.
const v2Channel = "molex/v2"

// DeriveCredentials expands a relay access token into the end-to-end
// pre-shared secret and rendezvous channel used by the hello exchange and
// key derivation. Every holder of the token, including the relay operator,
// can derive the same values: the v2 trust model explicitly trusts the
// relay operator while the relay implementation still forwards only opaque
// ciphertext.
func DeriveCredentials(token string) ([]byte, string) {
	reader := hkdf.New(sha256.New, []byte(token), nil, []byte("molex/v2/e2e-psk"))
	secret := make([]byte, 32)
	if _, err := io.ReadFull(reader, secret); err != nil {
		// SHA-256 HKDF cannot fail for a 32-byte read; keep the signature simple.
		panic("derive v2 credentials: " + err.Error())
	}
	return secret, v2Channel
}

// RouteForToken pre-computes the opaque route the relay expects for one
// token, pinning each hello to its admission identity.
func RouteForToken(token string) [32]byte {
	secret, channel := DeriveCredentials(token)
	return routeFor(secret, channel)
}
