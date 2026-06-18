package oidc_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/haakco/mcp-kit/oidc"
)

func TestDiscoveryAuthorizationServersIsIssuerURL(t *testing.T) {
	cfg := oidc.NewDiscoveryConfig("https://auth.example.test/", []string{"mcp.read", "mcp.write"})
	doc := cfg.ProtectedResourceMetadata("https://auth.example.test/mcp")

	if !reflect.DeepEqual(doc.AuthorizationServers, []string{"https://auth.example.test"}) {
		t.Fatalf("authorization_servers = %#v, want issuer URL", doc.AuthorizationServers)
	}
}

func TestDiscoveryAllAdvertisedScopesPresent(t *testing.T) {
	scopes := []string{"openid", "mcp.read", "mcp.write", "offline_access"}
	cfg := oidc.NewDiscoveryConfig("https://auth.example.test", scopes)

	openidDoc := cfg.OpenIDConfiguration()
	if got := openidDoc["scopes_supported"]; !reflect.DeepEqual(got, scopes) {
		t.Fatalf("openid scopes_supported = %#v, want %#v", got, scopes)
	}

	resourceDoc := cfg.ProtectedResourceMetadata("https://auth.example.test/mcp")
	if !reflect.DeepEqual(resourceDoc.ScopesSupported, scopes) {
		t.Fatalf("protected resource scopes_supported = %#v, want %#v", resourceDoc.ScopesSupported, scopes)
	}
}

func TestDiscoveryProtectedResourceMetadataIncludesResourceName(t *testing.T) {
	cfg := oidc.NewDiscoveryConfig("https://auth.example.test", []string{"mcp.read"})
	cfg.ResourceName = "HaakCo MCP"

	doc := cfg.ProtectedResourceMetadata("https://auth.example.test/mcp")

	if doc.ResourceName != "HaakCo MCP" {
		t.Fatalf("resource_name = %q, want HaakCo MCP", doc.ResourceName)
	}
}

func TestDiscoveryRegisterRoutes(t *testing.T) {
	cfg := oidc.NewDiscoveryConfig("https://auth.example.test", []string{"mcp.read"})
	mux := http.NewServeMux()
	cfg.RegisterRoutes(mux, oidc.RouteConfig{
		ResourceURL:  "https://auth.example.test/custom-mcp",
		ResourceName: "Custom MCP",
	})

	assertJSONField(t, mux, "/.well-known/oauth-authorization-server", "issuer", "https://auth.example.test")
	assertJSONField(t, mux, "/.well-known/openid-configuration", "jwks_uri", "https://auth.example.test/.well-known/jwks.json")
	assertJSONField(t, mux, "/.well-known/oauth-protected-resource", "resource", "https://auth.example.test/custom-mcp")
	assertJSONField(t, mux, "/.well-known/oauth-protected-resource", "resource_name", "Custom MCP")
	assertJSONField(t, mux, "/.well-known/oauth-protected-resource/mcp", "resource", "https://auth.example.test/custom-mcp")
	assertJSONField(t, mux, "/.well-known/oauth-protected-resource/mcp", "resource_name", "Custom MCP")
}

func TestDiscoveryRejectsNonGET(t *testing.T) {
	cfg := oidc.NewDiscoveryConfig("https://auth.example.test", []string{"mcp.read"})
	response := httptest.NewRecorder()

	cfg.OpenIDConfigurationHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/.well-known/openid-configuration", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", response.Code)
	}
	if response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", response.Header().Get("Allow"))
	}
}

func assertJSONField(t *testing.T, mux *http.ServeMux, path string, key string, want string) {
	t.Helper()

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))

	if response.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200; body=%s", path, response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode %s response: %v", path, err)
	}
	if payload[key] != want {
		t.Fatalf("%s %s = %#v, want %q", path, key, payload[key], want)
	}
}
