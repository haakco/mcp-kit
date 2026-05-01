package oauth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/token/jwt"

	"github.com/haakco/mcp-kit/oauth/storage"
)

// Provider owns OAuth provider state and HTTP handlers.
type Provider struct {
	oauth        fosite.OAuth2Provider
	store        storage.Store
	issuer       string
	allowedScope map[string]struct{}
}

// New constructs an OAuth provider backed by Fosite.
func New(cfg Config) (*Provider, error) {
	if err := cfg.applyDefaults(); err != nil {
		return nil, err
	}

	keyGetter := func(ctx context.Context) (any, error) {
		privateKey, _, err := cfg.KeyManager.GetActivePrivateKey(ctx)
		if err != nil {
			return nil, fmt.Errorf("get active signing key: %w", err)
		}
		return privateKey, nil
	}

	fositeConfig := &fosite.Config{
		GlobalSecret:                cfg.Secret,
		AccessTokenLifespan:         cfg.AccessTokenLifespan,
		RefreshTokenLifespan:        cfg.RefreshTokenLifespan,
		AuthorizeCodeLifespan:       cfg.AuthorizeCodeLifespan,
		IDTokenLifespan:             cfg.IDTokenLifespan,
		IDTokenIssuer:               cfg.Issuer,
		EnforcePKCEForPublicClients: true,
		MinParameterEntropy:         fosite.MinParameterEntropy,
		RefreshTokenScopes:          []string{},
	}
	strategy := &compose.CommonStrategy{
		CoreStrategy:               compose.NewOAuth2HMACStrategy(fositeConfig),
		OpenIDConnectTokenStrategy: compose.NewOpenIDConnectStrategy(keyGetter, fositeConfig),
		Signer:                     &jwt.DefaultSigner{GetPrivateKey: keyGetter},
	}
	fositeProvider := compose.Compose(
		fositeConfig,
		storage.New(cfg.Store),
		strategy,
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2RefreshTokenGrantFactory,
		compose.OpenIDConnectExplicitFactory,
		compose.OpenIDConnectRefreshFactory,
		compose.OAuth2TokenIntrospectionFactory,
		compose.OAuth2TokenRevocationFactory,
		compose.OAuth2PKCEFactory,
	)

	return &Provider{
		oauth:        fositeProvider,
		store:        cfg.Store,
		issuer:       cfg.Issuer,
		allowedScope: scopeSet(cfg.AllowedScopes),
	}, nil
}

// OAuth2Provider exposes the underlying Fosite provider for advanced consumers.
func (p *Provider) OAuth2Provider() fosite.OAuth2Provider {
	return p.oauth
}

// RegisterHandler returns the dynamic client registration handler.
func (p *Provider) RegisterHandler() http.Handler {
	return &RegistrationHandler{store: p.store, allowedScope: p.allowedScope}
}

func scopeSet(scopes []string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, scope := range scopes {
		result[scope] = struct{}{}
	}
	return result
}
