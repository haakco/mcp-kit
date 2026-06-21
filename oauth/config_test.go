package oauth

import (
	"testing"
	"time"

	"github.com/haakco/mcp-kit/oauth/keys"
	"github.com/haakco/mcp-kit/oauth/storage"
)

func TestConfigApplyDefaultsUsesShortAccessTokenLifetime(t *testing.T) {
	cfg := Config{
		Issuer:     "https://mcp.example.test",
		Store:      storage.NewMemoryStore(),
		KeyManager: keys.NewManager(nil),
	}

	if err := cfg.applyDefaults(); err != nil {
		t.Fatalf("applyDefaults() error = %v", err)
	}

	if cfg.AccessTokenLifespan != time.Hour {
		t.Fatalf("AccessTokenLifespan = %s, want 1h", cfg.AccessTokenLifespan)
	}
	if cfg.RefreshTokenLifespan != 30*24*time.Hour {
		t.Fatalf("RefreshTokenLifespan = %s, want 30d", cfg.RefreshTokenLifespan)
	}
}
