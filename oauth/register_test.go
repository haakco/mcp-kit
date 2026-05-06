package oauth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/storage"
)

func TestRegistrationHandlerAcceptsLogoURI(t *testing.T) {
	store := &capturingRegistrar{}
	handler := oauth.NewRegistrationHandler(oauth.RegistrationConfig{
		Store:                          store,
		AllowedScopes:                  []string{"openid", "mcp.read"},
		DefaultScopes:                  []string{"openid", "mcp.read"},
		Audience:                       "https://mcp.example.test/mcp",
		DefaultTokenEndpointAuthMethod: "none",
		DefaultGrantTypes:              []string{"authorization_code", "refresh_token"},
		DefaultResponseTypes:           []string{"code"},
		ClientIDPrefix:                 "mcp-",
	})

	response := postRegistration(t, handler, `{
		"client_name":"Inspector",
		"redirect_uris":["http://127.0.0.1:9999/callback"],
		"grant_types":["authorization_code","refresh_token"],
		"response_types":["code"],
		"token_endpoint_auth_method":"none",
		"scope":"openid mcp.read",
		"logo_uri":"https://assets.example.test/inspector.png"
	}`)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["logo_uri"]; got != "https://assets.example.test/inspector.png" {
		t.Fatalf("logo_uri = %#v, want registered logo", got)
	}
	if got := store.client.LogoURI; got != "https://assets.example.test/inspector.png" {
		t.Fatalf("stored LogoURI = %q, want registered logo", got)
	}
}

func TestRegistrationHandlerRejectsInvalidLogoURI(t *testing.T) {
	tests := []struct {
		name    string
		logoURI string
	}{
		{name: "non https", logoURI: "http://assets.example.test/inspector.png"},
		{name: "too long", logoURI: "https://assets.example.test/" + strings.Repeat("a", 2049)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &capturingRegistrar{}
			handler := oauth.NewRegistrationHandler(oauth.RegistrationConfig{
				Store:                          store,
				AllowedScopes:                  []string{"openid", "mcp.read"},
				DefaultScopes:                  []string{"openid", "mcp.read"},
				Audience:                       "https://mcp.example.test/mcp",
				DefaultTokenEndpointAuthMethod: "none",
				DefaultGrantTypes:              []string{"authorization_code"},
				DefaultResponseTypes:           []string{"code"},
			})

			response := postRegistration(t, handler, `{
				"client_name":"Inspector",
				"redirect_uris":["http://127.0.0.1:9999/callback"],
				"grant_types":["authorization_code"],
				"response_types":["code"],
				"token_endpoint_auth_method":"none",
				"scope":"openid mcp.read",
				"logo_uri":`+jsonQuote(tt.logoURI)+`
			}`)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
			}
			var payload map[string]string
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if got := payload["error"]; got != "invalid_client_metadata" {
				t.Fatalf("error = %q, want invalid_client_metadata", got)
			}
			if store.saved {
				t.Fatal("SaveClient called for invalid logo_uri")
			}
		})
	}
}

func postRegistration(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	return response
}

func jsonQuote(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

type capturingRegistrar struct {
	client storage.Client
	saved  bool
}

func (s *capturingRegistrar) SaveClient(_ context.Context, client storage.Client) error {
	s.client = client
	s.saved = true
	return nil
}
