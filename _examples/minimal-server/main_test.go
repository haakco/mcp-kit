package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haakco/mcp-kit/mcpkit"
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
	mcpURL := server.URL + "/mcp"

	// MCP 2026-07-28 has no initialize handshake: discovery is the first request.
	discover := testkit.DiscoverURL(t, mcpURL, "example-token")
	if len(discover.SupportedVersions) != 1 || discover.SupportedVersions[0] != mcpkit.ProtocolVersion {
		t.Fatalf("supportedVersions = %v, want [%s]", discover.SupportedVersions, mcpkit.ProtocolVersion)
	}

	registered := testkit.ListToolsURL(t, mcpURL, "example-token")
	testkit.AssertChecklistCoverage(t, registered, []string{"hello_world"})

	wire := testkit.Post(t, mcpURL, "example-token", "tools/list", nil)
	if wire.StatusCode != http.StatusOK {
		t.Fatalf("tools/list status = %d, want 200; body=%s", wire.StatusCode, wire.Body)
	}
	if sessionID := wire.Header.Get("Mcp-Session-Id"); sessionID != "" {
		t.Fatalf("Mcp-Session-Id = %q, want no session header in stateless mode", sessionID)
	}
	var payload map[string]any
	if err := json.Unmarshal(wire.JSON(), &payload); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	result := payload["result"].(map[string]any)
	tools := result["tools"].([]any)
	tool := tools[0].(map[string]any)
	if tool["name"] != "hello_world" {
		t.Fatalf("tool name = %#v, want hello_world", tool["name"])
	}
	if result["cacheScope"] != "private" {
		t.Fatalf("cacheScope = %#v, want private", result["cacheScope"])
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
