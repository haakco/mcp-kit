package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/haakco/mcp-kit/mcpkit"
	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/keys"
	"github.com/haakco/mcp-kit/oauth/storage"
	"github.com/haakco/mcp-kit/oidc"
)

const defaultIssuer = "http://localhost:8080"

func main() {
	handler, err := newHandler()
	if err != nil {
		log.Fatal(err)
	}
	addr := listenAddr()
	log.Println("listening on " + addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}

func newHandler() (http.Handler, error) {
	oauthProvider, keyManager, err := newExampleOAuth()
	if err != nil {
		return nil, err
	}
	resourceURL := issuerURL() + "/mcp"
	resourceMetadataURL, err := oauth.ProtectedResourceMetadataURLFor(resourceURL)
	if err != nil {
		return nil, err
	}

	// The kit supplies middleware only. The consumer owns the SDK server: its
	// identity, its transport options, and its cache policy.
	sdkServer := mcp.NewServer(
		&mcp.Implementation{Name: "minimal-server", Version: "0.0.0-example"},
		&mcp.ServerOptions{
			// Explicit empty capabilities: without this the SDK advertises the
			// historical default logging capability. Tool capabilities are still
			// inferred from the handlers registered below.
			Capabilities:              &mcp.ServerCapabilities{},
			SupportedProtocolVersions: []string{mcpkit.ProtocolVersion},
			// Results vary by caller because every request carries a bearer token.
			SetCacheable: mcpkit.PrivateCache(mcpkit.DefaultCacheTTL),
		},
	)
	mcp.AddTool(sdkServer, &mcp.Tool{
		Name:        "hello_world",
		Description: "Return a greeting.",
	}, func(context.Context, *mcp.CallToolRequest, any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "hello world"}},
		}, nil, nil
	})

	kitServer, err := mcpkit.New(mcpkit.Config{
		Handler: mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return sdkServer },
			// Stateless is mandatory for 2026-07-28 over Streamable HTTP.
			&mcp.StreamableHTTPOptions{Stateless: true},
		),
		AllowedOrigins: []string{"http://localhost:8080"},
		AllowLoopback:  true,
		Bearer: mcpkit.BearerConfig{
			Introspector:        oauthProvider.OAuth2Provider(),
			TokenValidator:      staticTokenValidator{},
			ResourceMetadataURL: resourceMetadataURL,
			RequiredScopes:      []string{"mcp.read"},
			ExpectedAudience:    resourceURL,
		},
	})
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	discovery := oidc.NewDiscoveryConfig(issuerURL(), []string{"mcp.read"})
	discovery.RegisterRoutes(mux, oidc.RouteConfig{
		ResourceURL: resourceURL,
		JWKS:        oidc.JWKSHandler(keyManager),
	})
	// Demo only: production servers must authenticate the browser session and
	// collect consent before granting requested scopes.
	oauthProvider.RegisterRoutes(mux, "/oauth", func(*http.Request) (oauth.Subject, error) {
		return oauth.Subject{
			ID:            "example-user",
			Email:         "example@example.com",
			GrantedScopes: []string{"openid", "mcp.read"},
		}, nil
	})
	mux.Handle("/mcp", kitServer.Handler())
	return mux, nil
}

func newExampleOAuth() (*oauth.Provider, *keys.Manager, error) {
	keyManager := keys.NewManager(keys.NewMemoryStore())
	if _, err := keyManager.EnsureSigningKey(context.Background()); err != nil {
		return nil, nil, err
	}
	provider, err := oauth.New(oauth.Config{
		Issuer:        issuerURL(),
		Audience:      issuerURL() + "/mcp",
		Store:         storage.NewMemoryStore(),
		KeyManager:    keyManager,
		AllowedScopes: []string{"openid", "mcp.read"},
	})
	if err != nil {
		return nil, nil, err
	}
	return provider, keyManager, nil
}

func issuerURL() string {
	if value := strings.TrimRight(os.Getenv("MCP_KIT_EXAMPLE_ISSUER"), "/"); value != "" {
		return value
	}
	return defaultIssuer
}

func listenAddr() string {
	if value := os.Getenv("MCP_KIT_EXAMPLE_ADDR"); value != "" {
		return value
	}
	return ":8080"
}

type staticTokenValidator struct{}

// ValidateAndResolve is demo-only PAT validation for the example server.
// Production servers should validate stored token hashes and authorization.
func (staticTokenValidator) ValidateAndResolve(_ context.Context, rawToken string) (*oauth.PATAuthResult, error) {
	if !strings.EqualFold(rawToken, "example-token") {
		return nil, errors.New("invalid token")
	}
	return &oauth.PATAuthResult{
		UserID:  "example-user",
		TokenID: "example-token",
		Scopes:  []string{"mcp.read"},
	}, nil
}

func (staticTokenValidator) RecordUsage(context.Context, string) {}
