package oauth

import (
	"context"

	"github.com/ory/fosite"
)

// TokenIntrospector validates OAuth bearer tokens. Fosite providers implement this interface.
type TokenIntrospector interface {
	IntrospectToken(context.Context, string, fosite.TokenType, fosite.Session, ...string) (
		fosite.TokenType,
		fosite.AccessRequester,
		error,
	)
}

// PATAuthResult contains the identity and scopes resolved from a PAT.
type PATAuthResult struct {
	UserID      string
	TokenID     string
	ScopeType   string
	ScopeTarget string
	Scopes      []string
}

// TokenValidator validates personal access tokens.
type TokenValidator interface {
	ValidateAndResolve(ctx context.Context, rawToken string) (*PATAuthResult, error)
	RecordUsage(ctx context.Context, tokenID string)
}
