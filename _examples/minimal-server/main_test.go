package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haakco/mcp-kit/testkit"
)

func TestMinimalServerDiscoveryAndToolsList(t *testing.T) {
	handler, err := newHandler()
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	discovery := httptest.NewRecorder()
	handler.ServeHTTP(discovery, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	if discovery.Code != http.StatusOK {
		t.Fatalf("discovery status = %d, want 200", discovery.Code)
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	sessionID := testkit.RunHandshakeURL(t, server.URL+"/mcp", "example-token")

	registered := testkit.ListToolsURL(t, server.URL+"/mcp", "example-token", sessionID)
	testkit.AssertChecklistCoverage(t, registered, []string{"hello_world"})

	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	request.Header.Set("Authorization", "Bearer example-token")
	request.Header.Set("Mcp-Session-Id", sessionID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	result := payload["result"].(map[string]any)
	tools := result["tools"].([]any)
	tool := tools[0].(map[string]any)
	if tool["name"] != "hello_world" {
		t.Fatalf("tool name = %#v, want hello_world", tool["name"])
	}
}

func TestMinimalServerRejectsMissingToken(t *testing.T) {
	handler, err := newHandler()
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1}`)))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	authHeader := response.Header().Get("WWW-Authenticate")
	if strings.Contains(authHeader, `scope=`) {
		t.Fatalf("WWW-Authenticate = %q, did not want scope hint on invalid_token challenge", authHeader)
	}
	if !strings.Contains(authHeader, `resource_metadata="http://localhost:8080/.well-known/oauth-protected-resource/mcp"`) {
		t.Fatalf("WWW-Authenticate = %q, want path-specific resource metadata URL", authHeader)
	}

	metadata := httptest.NewRecorder()
	handler.ServeHTTP(metadata, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp", nil))
	if metadata.Code != http.StatusOK {
		t.Fatalf("protected resource metadata status = %d, want 200", metadata.Code)
	}
	var payload struct {
		ScopesSupported []string `json:"scopes_supported"`
	}
	if err := json.Unmarshal(metadata.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode protected resource metadata: %v", err)
	}
	if strings.Join(payload.ScopesSupported, " ") != "mcp.read" {
		t.Fatalf("scopes_supported = %v, want [mcp.read]", payload.ScopesSupported)
	}
}
