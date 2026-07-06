package oauth

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ProtectedResourceMetadataConfig configures RFC 9728 protected resource metadata.
type ProtectedResourceMetadataConfig struct {
	Resource             string
	ResourceName         string
	AuthorizationServers []string
	ScopesSupported      []string
}

// AuthorizationServerMetadataConfig configures OAuth authorization server metadata.
type AuthorizationServerMetadataConfig struct {
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	RegistrationEndpoint  string
	Resource              string
	ResourceMetadataURL   string
	ScopesSupported       []string
}

type protectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	ResourceName           string   `json:"resource_name,omitempty"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
}

type authorizationServerMetadata struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	RegistrationEndpoint          string   `json:"registration_endpoint,omitempty"`
	Resource                      string   `json:"resource,omitempty"`
	ResourceMetadataURL           string   `json:"resource_metadata,omitempty"`
	GrantTypesSupported           []string `json:"grant_types_supported"`
	ResponseTypesSupported        []string `json:"response_types_supported"`
	ResponseModesSupported        []string `json:"response_modes_supported"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethods      []string `json:"token_endpoint_auth_methods_supported"`
	ScopesSupported               []string `json:"scopes_supported,omitempty"`
}

// ProtectedResourceMetadataHandler returns an RFC 9728 metadata handler.
func ProtectedResourceMetadataHandler(cfg ProtectedResourceMetadataConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMetadataError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeMetadataJSON(w, protectedResourceMetadata{
			Resource:               cfg.Resource,
			ResourceName:           cfg.ResourceName,
			AuthorizationServers:   append([]string{}, cfg.AuthorizationServers...),
			BearerMethodsSupported: []string{"header"},
			ScopesSupported:        append([]string{}, cfg.ScopesSupported...),
		})
	})
}

// AuthorizationServerMetadataHandler returns OAuth authorization server metadata.
func AuthorizationServerMetadataHandler(cfg AuthorizationServerMetadataConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMetadataError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeMetadataJSON(w, authorizationServerMetadata{
			Issuer:                        cfg.Issuer,
			AuthorizationEndpoint:         cfg.AuthorizationEndpoint,
			TokenEndpoint:                 cfg.TokenEndpoint,
			RegistrationEndpoint:          cfg.RegistrationEndpoint,
			Resource:                      cfg.Resource,
			ResourceMetadataURL:           cfg.ResourceMetadataURL,
			GrantTypesSupported:           []string{"authorization_code", "refresh_token"},
			ResponseTypesSupported:        []string{"code"},
			ResponseModesSupported:        []string{"query"},
			CodeChallengeMethodsSupported: []string{"S256"},
			TokenEndpointAuthMethods:      []string{authMethodNone, authMethodClientSecretBasic, authMethodClientSecretPost},
			ScopesSupported:               append([]string{}, cfg.ScopesSupported...),
		})
	})
}

func writeMetadataJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("write oauth metadata response", "error", err)
	}
}

func writeMetadataError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(message)); err != nil {
		slog.Error("write oauth metadata error", "error", err)
	}
}
