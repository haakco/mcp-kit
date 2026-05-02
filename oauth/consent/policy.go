package consent

import (
	"context"

	"github.com/ory/fosite"

	"github.com/haakco/mcp-kit/oauth"
)

// ConsentPolicy decides whether consent can be skipped and whether requested
// scopes are grantable to the subject.
type ConsentPolicy interface {
	AllowsSkip(ctx context.Context, client fosite.Client, subject oauth.Subject, scopes []string) bool
	ValidateScopes(ctx context.Context, subject oauth.Subject, scopes []string) error
}

// AlwaysAsk returns the default ConsentPolicy.
func AlwaysAsk() ConsentPolicy { return alwaysAskPolicy{} }

type alwaysAskPolicy struct{}

func (alwaysAskPolicy) AllowsSkip(context.Context, fosite.Client, oauth.Subject, []string) bool {
	return false
}

func (alwaysAskPolicy) ValidateScopes(context.Context, oauth.Subject, []string) error {
	return nil
}
