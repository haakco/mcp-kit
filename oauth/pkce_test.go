package oauth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/haakco/mcp-kit/oauth"
)

func TestPKCEPairUsesRawBase64URL(t *testing.T) {
	pair, err := oauth.NewPKCEPair()
	if err != nil {
		t.Fatalf("NewPKCEPair() error = %v", err)
	}

	if pair.Method != "S256" {
		t.Fatalf("method = %q, want S256", pair.Method)
	}
	for _, value := range []string{pair.Verifier, pair.Challenge} {
		if strings.ContainsAny(value, "+/=") {
			t.Fatalf("PKCE value %q contains non-raw-base64url character", value)
		}
	}
	if len(pair.Verifier) != 43 {
		t.Fatalf("verifier length = %d, want 43", len(pair.Verifier))
	}
	if got := oauth.PKCEChallenge(pair.Verifier); got != pair.Challenge {
		t.Fatalf("PKCEChallenge() = %q, want %q", got, pair.Challenge)
	}
}

func TestPKCEChallengeMatchesRFC7636S256(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])

	if got := oauth.PKCEChallenge(verifier); got != want {
		t.Fatalf("PKCEChallenge() = %q, want %q", got, want)
	}
}

func TestPKCERandomStateUsesRawBase64URL(t *testing.T) {
	state, err := oauth.RandomState()
	if err != nil {
		t.Fatalf("RandomState() error = %v", err)
	}
	if strings.ContainsAny(state, "+/=") {
		t.Fatalf("state %q contains non-raw-base64url character", state)
	}
	if len(state) < 16 {
		t.Fatalf("state length = %d, want at least 16", len(state))
	}
}
