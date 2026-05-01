package oauth

import (
	"errors"
	"strings"

	"github.com/haakco/mcp-kit/userstore"
)

// ClassifyLoginError reduces authentication failures to a fixed vocabulary.
func ClassifyLoginError(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, userstore.ErrNotFound) {
		return "bad_credentials"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "verify password"),
		strings.Contains(msg, "find user"),
		strings.Contains(msg, "not found"):
		return "bad_credentials"
	case strings.Contains(msg, "locked"),
		strings.Contains(msg, "disabled"),
		strings.Contains(msg, "inactive"):
		return "locked"
	default:
		return "unknown"
	}
}

// ClassifyResetError reduces password-reset failures to a fixed vocabulary.
func ClassifyResetError(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, userstore.ErrNotFound) {
		return "not_found"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "expired"):
		return "expired"
	case strings.Contains(msg, "invalid token"),
		strings.Contains(msg, "token mismatch"):
		return "invalid_token"
	case strings.Contains(msg, "send email"),
		strings.Contains(msg, "smtp"):
		return "smtp_failed"
	case strings.Contains(msg, "weak password"),
		strings.Contains(msg, "password too short"):
		return "weak_password"
	default:
		return "unknown"
	}
}

// ClassifyVerifyError reduces email-verification failures to a fixed vocabulary.
func ClassifyVerifyError(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, userstore.ErrNotFound) {
		return "not_found"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "expired"):
		return "expired"
	case strings.Contains(msg, "already verified"):
		return "already_verified"
	case strings.Contains(msg, "invalid token"),
		strings.Contains(msg, "token mismatch"):
		return "invalid_token"
	default:
		return "unknown"
	}
}

// ClassifySignupError reduces registration failures to a fixed vocabulary.
func ClassifySignupError(err error) string {
	if err == nil {
		return "ok"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "email already registered"),
		strings.Contains(msg, "already registered"):
		return "duplicate_email"
	case strings.Contains(msg, "username already taken"):
		return "duplicate_username"
	case strings.Contains(msg, "username must be"),
		strings.Contains(msg, "username is reserved"),
		strings.Contains(msg, "invalid username"):
		return "invalid_username"
	case strings.Contains(msg, "password is required"),
		strings.Contains(msg, "password too short"),
		strings.Contains(msg, "weak password"):
		return "weak_password"
	case strings.Contains(msg, "email is required"),
		strings.Contains(msg, "invalid email"):
		return "invalid_email"
	default:
		return "unknown"
	}
}
