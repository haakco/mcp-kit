package oauth_test

import (
	"fmt"
	"testing"

	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/userstore"
)

func TestClassifyLoginError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: "ok"},
		{name: "not found sentinel", err: userstore.ErrNotFound, want: "bad_credentials"},
		{name: "wrapped not found", err: fmt.Errorf("find user: %w", userstore.ErrNotFound), want: "bad_credentials"},
		{name: "verify password", err: fmt.Errorf("verify password: mismatch"), want: "bad_credentials"},
		{name: "inactive", err: fmt.Errorf("user inactive"), want: "locked"},
		{name: "unknown", err: fmt.Errorf("database unavailable"), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oauth.ClassifyLoginError(tt.err); got != tt.want {
				t.Fatalf("ClassifyLoginError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyResetError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: "ok"},
		{name: "not found", err: userstore.ErrNotFound, want: "not_found"},
		{name: "expired", err: fmt.Errorf("token expired"), want: "expired"},
		{name: "invalid", err: fmt.Errorf("invalid token"), want: "invalid_token"},
		{name: "smtp", err: fmt.Errorf("send email: smtp unavailable"), want: "smtp_failed"},
		{name: "weak", err: fmt.Errorf("weak password"), want: "weak_password"},
		{name: "unknown", err: fmt.Errorf("database unavailable"), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oauth.ClassifyResetError(tt.err); got != tt.want {
				t.Fatalf("ClassifyResetError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyVerifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: "ok"},
		{name: "not found", err: userstore.ErrNotFound, want: "not_found"},
		{name: "expired", err: fmt.Errorf("expired"), want: "expired"},
		{name: "already verified", err: fmt.Errorf("already verified"), want: "already_verified"},
		{name: "invalid", err: fmt.Errorf("token mismatch"), want: "invalid_token"},
		{name: "unknown", err: fmt.Errorf("database unavailable"), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oauth.ClassifyVerifyError(tt.err); got != tt.want {
				t.Fatalf("ClassifyVerifyError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifySignupError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: "ok"},
		{name: "duplicate email", err: fmt.Errorf("email already registered"), want: "duplicate_email"},
		{name: "duplicate username", err: fmt.Errorf("username already taken"), want: "duplicate_username"},
		{name: "invalid username", err: fmt.Errorf("invalid username"), want: "invalid_username"},
		{name: "weak password", err: fmt.Errorf("password too short"), want: "weak_password"},
		{name: "invalid email", err: fmt.Errorf("invalid email"), want: "invalid_email"},
		{name: "unknown", err: fmt.Errorf("database unavailable"), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oauth.ClassifySignupError(tt.err); got != tt.want {
				t.Fatalf("ClassifySignupError() = %q, want %q", got, tt.want)
			}
		})
	}
}
