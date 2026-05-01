package oauth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/haakco/mcp-kit/oauth"
)

func TestProtectedResourceMetadataHandler(t *testing.T) {
	handler := oauth.ProtectedResourceMetadataHandler(oauth.ProtectedResourceMetadataConfig{
		Resource:             "https://vorrent.example.test/mcp",
		AuthorizationServers: []string{"https://vorrent.example.test"},
		ScopesSupported:      []string{"mcp.read", "mcp.write", "offline_access"},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Resource               string   `json:"resource"`
		AuthorizationServers   []string `json:"authorization_servers"`
		BearerMethodsSupported []string `json:"bearer_methods_supported"`
		ScopesSupported        []string `json:"scopes_supported"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if body.Resource != "https://vorrent.example.test/mcp" {
		t.Fatalf("resource = %q", body.Resource)
	}
	if !slices.Equal(body.AuthorizationServers, []string{"https://vorrent.example.test"}) {
		t.Fatalf("authorization_servers = %#v", body.AuthorizationServers)
	}
	if !slices.Equal(body.BearerMethodsSupported, []string{"header"}) {
		t.Fatalf("bearer_methods_supported = %#v", body.BearerMethodsSupported)
	}
	if !slices.Equal(body.ScopesSupported, []string{"mcp.read", "mcp.write", "offline_access"}) {
		t.Fatalf("scopes_supported = %#v", body.ScopesSupported)
	}
}

func TestAuthorizationServerMetadataHandler(t *testing.T) {
	handler := oauth.AuthorizationServerMetadataHandler(oauth.AuthorizationServerMetadataConfig{
		Issuer:                "https://vorrent.example.test",
		AuthorizationEndpoint: "https://vorrent.example.test/mcp-oauth/authorize",
		TokenEndpoint:         "https://vorrent.example.test/mcp-oauth/token",
		RegistrationEndpoint:  "https://vorrent.example.test/mcp-oauth/register",
		ScopesSupported:       []string{"mcp.read", "mcp.write", "offline_access"},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Issuer                        string   `json:"issuer"`
		AuthorizationEndpoint         string   `json:"authorization_endpoint"`
		TokenEndpoint                 string   `json:"token_endpoint"`
		RegistrationEndpoint          string   `json:"registration_endpoint"`
		GrantTypesSupported           []string `json:"grant_types_supported"`
		ResponseTypesSupported        []string `json:"response_types_supported"`
		CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
		TokenEndpointAuthMethods      []string `json:"token_endpoint_auth_methods_supported"`
		ScopesSupported               []string `json:"scopes_supported"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if body.Issuer != "https://vorrent.example.test" {
		t.Fatalf("issuer = %q", body.Issuer)
	}
	if body.AuthorizationEndpoint != "https://vorrent.example.test/mcp-oauth/authorize" {
		t.Fatalf("authorization_endpoint = %q", body.AuthorizationEndpoint)
	}
	if body.TokenEndpoint != "https://vorrent.example.test/mcp-oauth/token" {
		t.Fatalf("token_endpoint = %q", body.TokenEndpoint)
	}
	if body.RegistrationEndpoint != "https://vorrent.example.test/mcp-oauth/register" {
		t.Fatalf("registration_endpoint = %q", body.RegistrationEndpoint)
	}
	if !slices.Equal(body.GrantTypesSupported, []string{"authorization_code", "refresh_token"}) {
		t.Fatalf("grant_types_supported = %#v", body.GrantTypesSupported)
	}
	if !slices.Equal(body.ResponseTypesSupported, []string{"code"}) {
		t.Fatalf("response_types_supported = %#v", body.ResponseTypesSupported)
	}
	if !slices.Equal(body.CodeChallengeMethodsSupported, []string{"S256"}) {
		t.Fatalf("code_challenge_methods_supported = %#v", body.CodeChallengeMethodsSupported)
	}
	if !slices.Equal(body.TokenEndpointAuthMethods, []string{"none", "client_secret_basic", "client_secret_post"}) {
		t.Fatalf("token_endpoint_auth_methods_supported = %#v", body.TokenEndpointAuthMethods)
	}
	if !slices.Equal(body.ScopesSupported, []string{"mcp.read", "mcp.write", "offline_access"}) {
		t.Fatalf("scopes_supported = %#v", body.ScopesSupported)
	}
}
