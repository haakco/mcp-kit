// Package authz defines the authorization interface that mcp-kit consumers
// implement to expose their existing RBAC / permission model to the kit.
//
// The kit never owns roles or permissions. Tool handlers in consumer code
// use the consumer's existing authz machinery directly. This package exists
// so kit-internal code (e.g. audit-log read endpoints in future versions)
// has a stable surface to call into.
package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrForbidden indicates the user lacks the required permission. Consumers
// MUST return this exact sentinel so the kit can map it to a 403 response.
var ErrForbidden = errors.New("authz: forbidden")

// Service is the consumer's authorization checker. Implementations must be
// safe for concurrent use.
type Service interface {
	// Check returns nil when userID has the named permission. Returns
	// ErrForbidden when they don't. Any other error is treated as a 500
	// (e.g. database unavailable).
	Check(ctx context.Context, userID uuid.UUID, permission string) error
}

// AlwaysAllow returns a Service that approves every check. Intended for
// tests and for consumers wiring in dev/local mode without a real RBAC
// system. Never use in production.
func AlwaysAllow() Service { return allowAll{} }

type allowAll struct{}

func (allowAll) Check(context.Context, uuid.UUID, string) error { return nil }
