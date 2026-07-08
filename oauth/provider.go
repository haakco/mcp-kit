package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/haakco/mcp-kit/audit"
	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/token/jwt"
	"golang.org/x/sync/singleflight"

	"github.com/haakco/mcp-kit/oauth/storage"
)

// Provider owns OAuth provider state and HTTP handlers.
type Provider struct {
	oauth         fosite.OAuth2Provider
	store         storage.Store
	issuer        string
	audience      string
	allowedScopes []string
	defaultScopes []string
	allowedScope  map[string]struct{}
	auditEmitter  audit.Emitter
	replayWindow  time.Duration
	now           func() time.Time
	replayCache   *refreshReplayCache
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
		oauth:         fositeProvider,
		store:         cfg.Store,
		issuer:        cfg.Issuer,
		audience:      cfg.Audience,
		allowedScopes: append([]string{}, cfg.AllowedScopes...),
		defaultScopes: append([]string{}, cfg.DefaultScopes...),
		allowedScope:  scopeSet(cfg.AllowedScopes),
		auditEmitter:  cfg.AuditEmitter,
		replayWindow:  cfg.RefreshReplayWindow,
		now:           cfg.Now,
		replayCache:   newRefreshReplayCache(),
	}, nil
}

// OAuth2Provider exposes the underlying Fosite provider for advanced consumers.
func (p *Provider) OAuth2Provider() fosite.OAuth2Provider {
	return p.oauth
}

// RegisterHandler returns the dynamic client registration handler.
func (p *Provider) RegisterHandler() http.Handler {
	return NewRegistrationHandler(RegistrationConfig{
		Store:         p.store,
		AllowedScopes: p.allowedScopes,
		DefaultScopes: p.defaultScopes,
		Audience:      p.audience,
	})
}

func scopeSet(scopes []string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, scope := range scopes {
		result[scope] = struct{}{}
	}
	return result
}

type replayedTokenResponse struct {
	body      []byte
	header    http.Header
	status    int
	expiresAt time.Time
}

type refreshReplayCache struct {
	mu      sync.Mutex
	entries map[string]replayedTokenResponse
	group   singleflight.Group
}

func newRefreshReplayCache() *refreshReplayCache {
	return &refreshReplayCache{entries: map[string]replayedTokenResponse{}}
}

func refreshReplayKey(clientID string, refreshToken string) string {
	sum := sha256.Sum256([]byte(clientID + "\x00" + refreshToken))
	return hex.EncodeToString(sum[:])
}

func (c *refreshReplayCache) get(key string, now time.Time) (replayedTokenResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return replayedTokenResponse{}, false
	}
	if !entry.expiresAt.After(now) {
		delete(c.entries, key)
		return replayedTokenResponse{}, false
	}
	return cloneReplayedResponse(entry), true
}

func (c *refreshReplayCache) set(key string, entry replayedTokenResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cloneReplayedResponse(entry)
}

func cloneReplayedResponse(entry replayedTokenResponse) replayedTokenResponse {
	entry.body = append([]byte{}, entry.body...)
	entry.header = entry.header.Clone()
	return entry
}
