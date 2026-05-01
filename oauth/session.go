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
}

// NewSession creates an OIDC session for subject.
func NewSession(subject Subject) *openid.DefaultSession {
	return &openid.DefaultSession{
		Claims: &jwt.IDTokenClaims{
			Subject:     subject.ID,
			RequestedAt: time.Now().UTC(),
			Extra: map[string]any{
				"email": subject.Email,
			},
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
