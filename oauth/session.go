package oauth

import (
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/token/jwt"
)

// Subject is the authenticated resource owner authorizing an OAuth client.
type Subject struct {
	ID            string
	Email         string
	GrantedScopes []string

	// Extra carries consumer-specific session data. Values are copied into
	// OIDC token claims as-is, so callers should use JSON-serializable values.
	Extra map[string]any
}

// NewSession creates an OIDC session for subject.
func NewSession(subject Subject) *openid.DefaultSession {
	extra := map[string]any{
		"email": subject.Email,
	}
	for key, value := range subject.Extra {
		extra[key] = value
	}

	return &openid.DefaultSession{
		Claims: &jwt.IDTokenClaims{
			Subject:     subject.ID,
			RequestedAt: time.Now().UTC(),
			Extra:       extra,
		},
		Headers:   &jwt.Headers{},
		Subject:   subject.ID,
		Username:  subject.Email,
		ExpiresAt: map[fosite.TokenType]time.Time{},
	}
}

// NewEmptySession creates an empty session for token exchange deserialization.
func NewEmptySession() *openid.DefaultSession {
	return openid.NewDefaultSession()
}
