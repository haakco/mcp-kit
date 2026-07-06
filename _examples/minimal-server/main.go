package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

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

	mcpServer, err := mcpkit.New(mcpkit.Config{
		Handler:        http.HandlerFunc(handleMCP),
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
	mux.Handle("/mcp", mcpServer.Handler())
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

func handleMCP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID     any    `json:"id"`
		Method string `json:"method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "malformed payload: invalid JSON", http.StatusBadRequest)
		return
	}
	if request.Method == "initialize" {
		w.Header().Set("Mcp-Session-Id", "minimal-example-session")
		writeJSON(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "minimal-server", "version": "0.0.0-example"},
			},
		})
		return
	}
	if request.Method == "notifications/initialized" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if request.Method != "tools/list" {
		http.Error(w, "JSON RPC not handled: "+request.Method, http.StatusBadRequest)
		return
	}

	writeJSON(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      request.ID,
		"result": map[string]any{
			"tools": []map[string]any{
				{
					"name":        "hello_world",
					"description": "Return a greeting.",
					"inputSchema": map[string]any{"type": "object"},
				},
			},
		},
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON: %v", err)
	}
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
