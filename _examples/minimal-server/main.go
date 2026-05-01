package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/haakco/mcp-kit/mcpkit"
	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oidc"
)

const issuer = "http://localhost:8080"

func main() {
	handler, err := newHandler()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

func newHandler() (http.Handler, error) {
	mcpServer, err := mcpkit.New(mcpkit.Config{
		Handler:        http.HandlerFunc(handleMCP),
		AllowedOrigins: []string{"http://localhost:8080"},
		AllowLoopback:  true,
		Bearer: mcpkit.BearerConfig{
			TokenValidator:      staticTokenValidator{},
			ResourceMetadataURL: issuer + "/.well-known/oauth-protected-resource",
		},
	})
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	discovery := oidc.NewDiscoveryConfig(issuer, []string{"mcp.read"})
	discovery.RegisterRoutes(mux, oidc.RouteConfig{ResourceURL: issuer + "/mcp"})
	mux.Handle("/mcp", mcpServer.Handler())
	return mux, nil
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
