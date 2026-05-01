package testkit

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func randomID(t testing.TB) string {
	t.Helper()
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("generate random ID: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
