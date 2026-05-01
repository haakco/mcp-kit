package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	request.Header.Set("Authorization", "Bearer example-token")
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
}
