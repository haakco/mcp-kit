package consent

import (
	"context"

	"github.com/haakco/mcp-kit/oauth"
)

// Authenticator verifies credentials and returns the resource owner subject.
type Authenticator interface {
	Authenticate(ctx context.Context, username string, password string) (oauth.Subject, error)
}

// AuthenticatorFunc adapts a function to Authenticator.
type AuthenticatorFunc func(ctx context.Context, username string, password string) (oauth.Subject, error)

// Authenticate calls f.
func (f AuthenticatorFunc) Authenticate(ctx context.Context, username string, password string) (oauth.Subject, error) {
	return f(ctx, username, password)
}
