package oauth

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/haakco/mcp-kit/oauth/keys"
	"github.com/haakco/mcp-kit/oauth/storage"
)

// Config holds settings for the OAuth/OIDC provider.
type Config struct {
	Secret   []byte //nolint:gosec // OAuth HMAC secret supplied by the consumer.
	Issuer   string
	Audience string
	Store    storage.Store

	KeyManager *keys.Manager

	AllowedScopes []string
	DefaultScopes []string

	AccessTokenLifespan   time.Duration
	RefreshTokenLifespan  time.Duration
	AuthorizeCodeLifespan time.Duration
	IDTokenLifespan       time.Duration
}

func (c *Config) applyDefaults() error {
	if c.AccessTokenLifespan == 0 {
		c.AccessTokenLifespan = time.Hour
	}
	if c.RefreshTokenLifespan == 0 {
		c.RefreshTokenLifespan = 24 * time.Hour
	}
	if c.AuthorizeCodeLifespan == 0 {
		c.AuthorizeCodeLifespan = 10 * time.Minute
	}
	if c.IDTokenLifespan == 0 {
		c.IDTokenLifespan = time.Hour
	}
	if len(c.Secret) == 0 {
		c.Secret = make([]byte, 32)
		if _, err := rand.Read(c.Secret); err != nil {
			return fmt.Errorf("generate OAuth secret: %w", err)
		}
	}
	if len(c.Secret) != 32 {
		return fmt.Errorf("oauth secret must be exactly 32 bytes, got %d", len(c.Secret))
	}
	if c.Issuer == "" {
		return fmt.Errorf("oauth issuer is required")
	}
	if c.Audience == "" {
		c.Audience = strings.TrimRight(c.Issuer, "/") + "/mcp"
	}
	if c.Store == nil {
		return fmt.Errorf("oauth store is required")
	}
	if c.KeyManager == nil {
		return fmt.Errorf("oauth key manager is required")
	}
	return nil
}
