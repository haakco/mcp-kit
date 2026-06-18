// Package oidc serves OAuth/OIDC discovery documents for MCP servers.
package oidc

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

const signingAlgorithm = "RS256"

// DiscoveryConfig holds values needed to build discovery documents.
type DiscoveryConfig struct {
	Issuer                string
	ResourceName          string
	AuthorizationEndpoint string
	TokenEndpoint         string
	JWKSEndpoint          string
	RevocationEndpoint    string
	RegistrationEndpoint  string
	ScopesSupported       []string
}

// ProtectedResourceMetadata is RFC 9728 protected resource metadata.
type ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	ResourceName           string   `json:"resource_name,omitempty"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
}

// RouteConfig configures mounted discovery routes.
type RouteConfig struct {
	ResourceURL  string
	ResourceName string
	JWKS         http.Handler
}

// NewDiscoveryConfig builds a discovery config from an issuer URL.
func NewDiscoveryConfig(issuerURL string, scopes []string) DiscoveryConfig {
	issuer := strings.TrimRight(issuerURL, "/")
	return DiscoveryConfig{
		Issuer:                issuer,
		AuthorizationEndpoint: issuer + "/oauth/authorize",
		TokenEndpoint:         issuer + "/oauth/token",
		JWKSEndpoint:          issuer + "/.well-known/jwks.json",
		RevocationEndpoint:    issuer + "/oauth/revoke",
		RegistrationEndpoint:  issuer + "/oauth/register",
		ScopesSupported:       append([]string{}, scopes...),
	}
}

// OpenIDConfiguration returns the OIDC discovery document. The same shape is
// also valid OAuth authorization server metadata for the kit's supported flows.
func (d DiscoveryConfig) OpenIDConfiguration() map[string]any {
	return map[string]any{
		"issuer":                                d.Issuer,
		"authorization_endpoint":                d.AuthorizationEndpoint,
		"token_endpoint":                        d.TokenEndpoint,
		"jwks_uri":                              d.JWKSEndpoint,
		"revocation_endpoint":                   d.RevocationEndpoint,
		"registration_endpoint":                 d.RegistrationEndpoint,
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{signingAlgorithm},
		"scopes_supported":                      append([]string{}, d.ScopesSupported...),
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_basic", "client_secret_post"},
		"code_challenge_methods_supported":      []string{"S256"},
	}
}

// AuthorizationServerMetadata returns RFC 8414 metadata.
func (d DiscoveryConfig) AuthorizationServerMetadata() map[string]any {
	return d.OpenIDConfiguration()
}

// ProtectedResourceMetadata returns metadata for an MCP protected resource.
func (d DiscoveryConfig) ProtectedResourceMetadata(resourceURL string) ProtectedResourceMetadata {
	return ProtectedResourceMetadata{
		Resource:               strings.TrimRight(resourceURL, "/"),
		ResourceName:           d.ResourceName,
		AuthorizationServers:   []string{d.Issuer},
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        append([]string{}, d.ScopesSupported...),
	}
}

// OpenIDConfigurationHandler returns a handler for /.well-known/openid-configuration.
func (d DiscoveryConfig) OpenIDConfigurationHandler() http.Handler {
	return jsonHandler(func() any { return d.OpenIDConfiguration() })
}

// AuthorizationServerHandler returns a handler for /.well-known/oauth-authorization-server.
func (d DiscoveryConfig) AuthorizationServerHandler() http.Handler {
	return jsonHandler(func() any { return d.AuthorizationServerMetadata() })
}

// ProtectedResourceHandler returns a handler for /.well-known/oauth-protected-resource.
func (d DiscoveryConfig) ProtectedResourceHandler(resourceURL string) http.Handler {
	return jsonHandler(func() any {
		return d.ProtectedResourceMetadata(resourceURL)
	})
}

// RegisterRoutes mounts discovery routes. If ResourceURL is empty, it uses issuer + "/mcp".
func (d DiscoveryConfig) RegisterRoutes(mux *http.ServeMux, cfg RouteConfig) {
	resourceURL := cfg.ResourceURL
	if resourceURL == "" {
		resourceURL = d.Issuer + "/mcp"
	}
	if cfg.ResourceName != "" {
		d.ResourceName = cfg.ResourceName
	}
	mux.Handle("/.well-known/openid-configuration", d.OpenIDConfigurationHandler())
	mux.Handle("/.well-known/oauth-authorization-server", d.AuthorizationServerHandler())
	protectedResourceHandler := d.ProtectedResourceHandler(resourceURL)
	mux.Handle("/.well-known/oauth-protected-resource", protectedResourceHandler)
	mux.Handle("/.well-known/oauth-protected-resource/mcp", protectedResourceHandler)
	if cfg.JWKS != nil {
		mux.Handle("/.well-known/jwks.json", cfg.JWKS)
	}
}

func jsonHandler(build func() any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(build()); err != nil {
			slog.Error("write discovery response", "error", err)
		}
	})
}
