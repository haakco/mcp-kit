package testkit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/haakco/mcp-kit/oauth"
)

var testTokenKey = []byte("mcp-kit-test-token-key-v1")

type tokenClaims struct {
	UserID    string   `json:"uid"`
	TokenID   string   `json:"tid"`
	Scopes    []string `json:"scp"`
	ExpiresAt int64    `json:"exp"`
}

// MintToken issues a signed test bearer token accepted by TokenValidator.
func MintToken(t testing.TB, scopes ...string) string {
	t.Helper()
	if len(scopes) == 0 {
		scopes = []string{"mcp.read"}
	}
	claims := tokenClaims{
		UserID:    "test-user",
		TokenID:   randomID(t),
		Scopes:    append([]string{}, scopes...),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal test token: %v", err)
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, testTokenKey)
	mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + sig
}

// TokenValidator returns a test-only validator for tokens produced by MintToken.
func TokenValidator(t testing.TB) oauth.TokenValidator {
	t.Helper()
	return tokenValidator{}
}

type tokenValidator struct{}

func (tokenValidator) ValidateAndResolve(_ context.Context, rawToken string) (*oauth.PATAuthResult, error) {
	claims, err := parseToken(rawToken)
	if err != nil {
		return nil, err
	}
	if time.Now().Unix() >= claims.ExpiresAt {
		return nil, errors.New("test token expired")
	}
	return &oauth.PATAuthResult{
		UserID:  claims.UserID,
		TokenID: claims.TokenID,
		Scopes:  append([]string{}, claims.Scopes...),
	}, nil
}

func (tokenValidator) RecordUsage(context.Context, string) {}

func parseToken(rawToken string) (tokenClaims, error) {
	body, sig, ok := strings.Cut(rawToken, ".")
	if !ok {
		return tokenClaims{}, errors.New("invalid test token")
	}
	mac := hmac.New(sha256.New, testTokenKey)
	mac.Write([]byte(body))
	want := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return tokenClaims{}, errors.New("invalid test token signature")
	}
	if !hmac.Equal(got, want) {
		return tokenClaims{}, errors.New("invalid test token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return tokenClaims{}, errors.New("invalid test token payload")
	}
	var claims tokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return tokenClaims{}, errors.New("invalid test token claims")
	}
	return claims, nil
}
