// Package userstore defines the user-lookup interface that mcp-kit consumers
// implement to expose their existing user table to the OAuth flow.
//
// The kit never owns user data — it only needs FindByEmail (for the
// password-grant login form) and FindByID (for token validation).
package userstore

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrNotFound indicates the requested user could not be located. Consumers
// MUST return this exact sentinel from FindByEmail / FindByID when the user
// is missing, so the kit can map it to a stable login-error classification
// without log-leaking.
var ErrNotFound = errors.New("userstore: user not found")

// ErrInvalidCredentials is returned by VerifyPassword when the email or
// password is wrong. It deliberately does not distinguish between
// "user not found" and "wrong password" — that's a security boundary.
var ErrInvalidCredentials = errors.New("userstore: invalid credentials")

// User is the minimum surface the kit needs from a consumer's user record.
type User interface {
	ID() uuid.UUID
	Email() string
	// PasswordHash returns the bcrypt hash. Returns nil if the user has no
	// password (e.g. SSO-only). Callers must treat nil as "this user cannot
	// log in via password".
	PasswordHash() []byte
	IsActive() bool
}

// Store resolves users by stable identifiers. Implementations must be safe
// for concurrent use.
type Store interface {
	FindByEmail(ctx context.Context, email string) (User, error)
	FindByID(ctx context.Context, id uuid.UUID) (User, error)
}
