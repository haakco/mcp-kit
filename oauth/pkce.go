package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// PKCEPair is a freshly generated PKCE verifier + challenge pair.
type PKCEPair struct {
	Verifier  string
	Challenge string
	Method    string
}

// NewPKCEPair generates an RFC 7636 S256 verifier/challenge pair.
func NewPKCEPair() (PKCEPair, error) {
	verifier, err := randomURLValue(32)
	if err != nil {
		return PKCEPair{}, err
	}
	return PKCEPair{
		Verifier:  verifier,
		Challenge: PKCEChallenge(verifier),
		Method:    "S256",
	}, nil
}

// PKCEChallenge returns the RFC 7636 S256 challenge for verifier.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// RandomState returns a URL-safe random OAuth state parameter.
func RandomState() (string, error) {
	return randomURLValue(16)
}

func randomURLValue(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
