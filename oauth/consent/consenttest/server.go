package consenttest

import (
	"context"
	"testing"

	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/keys"
	"github.com/haakco/mcp-kit/oauth/storage"
)

// Provider returns an OAuth provider backed by in-memory stores.
func Provider(t testing.TB, issuer string) (*oauth.Provider, *storage.MemoryStore) {
	t.Helper()
	store := storage.NewMemoryStore()
	manager := keys.NewManager(keys.NewMemoryStore())
	if _, err := manager.EnsureSigningKey(context.Background()); err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}
	provider, err := oauth.New(oauth.Config{
		Issuer:        issuer,
		Secret:        []byte("test-secret-must-be-32-bytes!!!!"),
		Store:         store,
		KeyManager:    manager,
		AllowedScopes: []string{"openid", "mcp.read", "mcp.write", "offline_access"},
		DefaultScopes: []string{"openid", "mcp.read"},
	})
	if err != nil {
		t.Fatalf("oauth.New() error = %v", err)
	}
	return provider, store
}

// ClientOptions configures RegisterClient.
type ClientOptions struct {
	ID           string
	Name         string
	RedirectURI  string
	Scopes       []string
	ResourceURL  string
	PublicClient bool
}

// RegisterClient saves a PKCE-capable authorization-code client.
func RegisterClient(t testing.TB, store *storage.MemoryStore, opts ClientOptions) {
	t.Helper()
	if opts.ID == "" {
		opts.ID = "client-id"
	}
	if opts.Name == "" {
		opts.Name = "Test Client"
	}
	if opts.RedirectURI == "" {
		opts.RedirectURI = "http://127.0.0.1/callback"
	}
	if len(opts.Scopes) == 0 {
		opts.Scopes = []string{"openid", "mcp.read"}
	}
	if opts.ResourceURL == "" {
		opts.ResourceURL = "https://app.example.com/mcp"
	}
	if err := store.SaveClient(context.Background(), storage.Client{
		ID:            opts.ID,
		Name:          opts.Name,
		RedirectURIs:  []string{opts.RedirectURI},
		GrantTypes:    []string{"authorization_code", "refresh_token"},
		ResponseTypes: []string{"code"},
		Scopes:        opts.Scopes,
		Audience:      []string{opts.ResourceURL},
		IsPublic:      true,
	}); err != nil {
		t.Fatalf("SaveClient() error = %v", err)
	}
}
