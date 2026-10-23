package core

// X25519 key generation for imported wisteria profiles. The share link
// only carries the SERVER public key (or nothing at all — the first XX
// handshake learns it); the client half of the keypair is generated
// locally on first import and stored with the profile. Format matches
// wisteria-keygen: base64 std encoding of the 32-byte scalar.

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

func GenerateX25519() (privB64, pubB64 string, err error) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return "", "", fmt.Errorf("rand: %w", err)
	}
	// Standard X25519 scalar clamping (RFC 7748 §5).
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("x25519: %w", err)
	}
	return base64.StdEncoding.EncodeToString(priv), base64.StdEncoding.EncodeToString(pub), nil
}
