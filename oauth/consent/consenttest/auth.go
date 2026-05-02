package consenttest

import (
	"context"
	"errors"

	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/consent"
)

// StaticAuth returns an Authenticator backed by username/password keys.
func StaticAuth(creds map[string]oauth.Subject) consent.Authenticator {
	return consent.AuthenticatorFunc(func(_ context.Context, username string, password string) (oauth.Subject, error) {
		subject, ok := creds[username+":"+password]
		if !ok {
			return oauth.Subject{}, errors.New("invalid credentials")
		}
		return subject, nil
	})
}

// DenyAll returns an Authenticator that rejects every login.
func DenyAll() consent.Authenticator {
	return consent.AuthenticatorFunc(func(context.Context, string, string) (oauth.Subject, error) {
		return oauth.Subject{}, errors.New("invalid credentials")
	})
}
