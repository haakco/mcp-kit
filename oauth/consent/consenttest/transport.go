package consenttest

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/haakco/mcp-kit/oauth/consent"
)

// NoFollowClient returns an HTTP client that exposes redirects to callers.
func NoFollowClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

// S256Challenge returns the PKCE S256 code_challenge for verifier.
func S256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// HiddenInputValue returns the value for name from inputs.
func HiddenInputValue(inputs []consent.HiddenInput, name string) string {
	for _, input := range inputs {
		if input.Name == name {
			return input.Value
		}
	}
	return ""
}

// FormRequest builds an authorize POST request.
func FormRequest(values url.Values) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return request
}
