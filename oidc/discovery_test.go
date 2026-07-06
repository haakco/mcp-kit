package oidc_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/haakco/mcp-kit/oidc"
)

func TestProtectedResourceMetadataURLFor(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		want     string
	}{
		{
			name:     "origin only",
			resource: "https://mcp.example.test",
			want:     "https://mcp.example.test/.well-known/oauth-protected-resource",
		},
		{
			name:     "mcp path",
			resource: "https://mcp.example.test/mcp",
			want:     "https://mcp.example.test/.well-known/oauth-protected-resource/mcp",
		},
		{
			name:     "nested path",
			resource: "https://mcp.example.test/api/mcp",
			want:     "https://mcp.example.test/.well-known/oauth-protected-resource/api/mcp",
		},
		{
			name:     "escaped path",
			resource: "https://mcp.example.test/api/%E2%9C%93",
			want:     "https://mcp.example.test/.well-known/oauth-protected-resource/api/%E2%9C%93",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := oidc.ProtectedResourceMetadataURLFor(tt.resource)
			if err != nil {
				t.Fatalf("ProtectedResourceMetadataURLFor() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("url = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProtectedResourceMetadataURLForRejectsQueryAndFragment(t *testing.T) {
	for _, resource := range []string{
		"https://mcp.example.test/mcp?tenant=a",
		"https://mcp.example.test/mcp#fragment",
	} {
		t.Run(resource, func(t *testing.T) {
			if _, err := oidc.ProtectedResourceMetadataURLFor(resource); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

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
	assertJSONField(t, mux, "/.well-known/oauth-authorization-server", "resource", "https://auth.example.test/custom-mcp")
	assertJSONField(t, mux, "/.well-known/oauth-authorization-server", "resource_metadata", "https://auth.example.test/.well-known/oauth-protected-resource/custom-mcp")
	assertJSONField(t, mux, "/.well-known/openid-configuration", "jwks_uri", "https://auth.example.test/.well-known/jwks.json")
	assertJSONField(t, mux, "/.well-known/openid-configuration", "resource", "https://auth.example.test/custom-mcp")
	assertJSONField(t, mux, "/.well-known/openid-configuration", "resource_metadata", "https://auth.example.test/.well-known/oauth-protected-resource/custom-mcp")
	assertJSONField(t, mux, "/.well-known/oauth-protected-resource", "resource", "https://auth.example.test/custom-mcp")
	assertJSONField(t, mux, "/.well-known/oauth-protected-resource", "resource_name", "Custom MCP")
	assertJSONField(t, mux, "/.well-known/oauth-protected-resource/custom-mcp", "resource", "https://auth.example.test/custom-mcp")
	assertJSONField(t, mux, "/.well-known/oauth-protected-resource/custom-mcp", "resource_name", "Custom MCP")
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
