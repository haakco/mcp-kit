package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/haakco/mcp-kit/oauth/storage"
)

const (
	authMethodNone              = "none"
	authMethodClientSecretBasic = "client_secret_basic"
	authMethodClientSecretPost  = "client_secret_post"
	maxRegistrationBodyBytes    = 16 * 1024
	clientSecretRandomBytes     = 32
	defaultClientName           = "Dynamic Client"
	maxClientNameLen            = 200
)

var (
	supportedGrantTypes = map[string]struct{}{
		"authorization_code": {},
		"refresh_token":      {},
	}
	supportedResponseTypes = map[string]struct{}{
		"code": {},
	}
	supportedAuthMethods = map[string]struct{}{
		authMethodNone:              {},
		authMethodClientSecretBasic: {},
		authMethodClientSecretPost:  {},
	}
)

// RegistrationHandler implements RFC 7591 dynamic client registration.
type RegistrationHandler struct {
	store                          ClientRegistrar
	allowedScopes                  []string
	allowedScope                   map[string]struct{}
	audience                       string
	defaultTokenEndpointAuthMethod string
	clientIDPrefix                 string
}

// ClientRegistrar persists dynamically registered OAuth clients.
type ClientRegistrar interface {
	SaveClient(context.Context, storage.Client) error
}

// RegistrationConfig configures a dynamic client registration handler.
type RegistrationConfig struct {
	Store                          ClientRegistrar
	AllowedScopes                  []string
	Audience                       string
	DefaultTokenEndpointAuthMethod string
	ClientIDPrefix                 string
}

// NewRegistrationHandler creates an RFC 7591 dynamic client registration handler.
func NewRegistrationHandler(cfg RegistrationConfig) http.Handler {
	defaultAuthMethod := strings.TrimSpace(cfg.DefaultTokenEndpointAuthMethod)
	if defaultAuthMethod == "" {
		defaultAuthMethod = authMethodClientSecretBasic
	}
	return &RegistrationHandler{
		store:                          cfg.Store,
		allowedScopes:                  append([]string{}, cfg.AllowedScopes...),
		allowedScope:                   scopeSet(cfg.AllowedScopes),
		audience:                       cfg.Audience,
		defaultTokenEndpointAuthMethod: defaultAuthMethod,
		clientIDPrefix:                 cfg.ClientIDPrefix,
	}
}

func (h *RegistrationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeRegistrationError(w, http.StatusMethodNotAllowed, "invalid_request", "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRegistrationBodyBytes)
	defer func() { _ = r.Body.Close() }()

	var req registrationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeRegistrationError(w, http.StatusBadRequest, "invalid_client_metadata", "request body is not valid JSON")
		return
	}

	resp, status, code, description := h.process(r, req)
	if code != "" {
		writeRegistrationError(w, status, code, description)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("write registration response", "error", err)
	}
}

func (h *RegistrationHandler) process(r *http.Request, req registrationRequest) (*registrationResponse, int, string, string) {
	if len(req.RedirectURIs) == 0 {
		return nil, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris is required"
	}
	for _, uri := range req.RedirectURIs {
		if err := validateRedirectURI(uri); err != nil {
			return nil, http.StatusBadRequest, "invalid_redirect_uri", err.Error()
		}
	}

	grantTypes, status, code, description := normalizeGrantTypes(req.GrantTypes)
	if code != "" {
		return nil, status, code, description
	}
	responseTypes, status, code, description := normalizeResponseTypes(req.ResponseTypes)
	if code != "" {
		return nil, status, code, description
	}
	authMethod, status, code, description := h.normalizeAuthMethod(req.TokenEndpointAuthMethod)
	if code != "" {
		return nil, status, code, description
	}
	scopes, scopeString, err := normalizeScopesWithDefault(req.Scope, h.allowedScopes, h.allowedScope)
	if err != nil {
		return nil, http.StatusBadRequest, "invalid_client_metadata", err.Error()
	}

	clientName := strings.TrimSpace(req.ClientName)
	if clientName == "" {
		clientName = defaultClientName
	}
	if len(clientName) > maxClientNameLen {
		clientName = clientName[:maxClientNameLen]
	}

	clientID := h.clientIDPrefix + generateRandomURLValue(24)
	isPublic := authMethod == authMethodNone
	var rawSecret string
	var secretHash string
	if !isPublic {
		rawSecret = generateRandomURLValue(clientSecretRandomBytes)
		var err error
		secretHash, err = HashSecret(rawSecret)
		if err != nil {
			slog.Error("hash oauth client secret", "error", err, "client_id", clientID)
			return nil, http.StatusInternalServerError, "server_error", "failed to create client secret"
		}
	}

	client := storage.Client{
		ID:               clientID,
		Name:             clientName,
		ClientSecretHash: secretHash,
		RedirectURIs:     append([]string{}, req.RedirectURIs...),
		GrantTypes:       grantTypes,
		ResponseTypes:    responseTypes,
		Scopes:           scopes,
		Audience:         []string{h.audience},
		IsPublic:         isPublic,
	}
	if err := h.store.SaveClient(r.Context(), client); err != nil {
		slog.Error("persist oauth client", "error", err, "client_id", clientID)
		return nil, http.StatusInternalServerError, "server_error", "failed to persist client"
	}

	resp := &registrationResponse{
		ClientID:                clientID,
		ClientName:              clientName,
		ClientIDIssuedAt:        time.Now().UTC().Unix(),
		RedirectURIs:            append([]string{}, req.RedirectURIs...),
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		TokenEndpointAuthMethod: authMethod,
		Scope:                   scopeString,
	}
	if rawSecret != "" {
		resp.ClientSecret = rawSecret
		resp.ClientSecretExpiresAt = 0
	}
	return resp, http.StatusCreated, "", ""
}

type registrationRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

type registrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientSecretExpiresAt   int64    `json:"client_secret_expires_at,omitempty"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope,omitempty"`
}

func normalizeGrantTypes(requested []string) ([]string, int, string, string) {
	if len(requested) == 0 {
		requested = []string{"authorization_code"}
	}
	for _, grantType := range requested {
		if _, ok := supportedGrantTypes[grantType]; !ok {
			return nil, http.StatusBadRequest, "invalid_client_metadata", fmt.Sprintf("grant_type %q is not supported", grantType)
		}
	}
	return append([]string{}, requested...), 0, "", ""
}

func normalizeResponseTypes(requested []string) ([]string, int, string, string) {
	if len(requested) == 0 {
		requested = []string{"code"}
	}
	for _, responseType := range requested {
		if _, ok := supportedResponseTypes[responseType]; !ok {
			return nil, http.StatusBadRequest, "invalid_client_metadata", fmt.Sprintf("response_type %q is not supported", responseType)
		}
	}
	return append([]string{}, requested...), 0, "", ""
}

func (h *RegistrationHandler) normalizeAuthMethod(requested string) (string, int, string, string) {
	if requested == "" {
		requested = h.defaultTokenEndpointAuthMethod
	}
	if _, ok := supportedAuthMethods[requested]; !ok {
		return "", http.StatusBadRequest, "invalid_client_metadata", fmt.Sprintf("token_endpoint_auth_method %q is not supported", requested)
	}
	return requested, 0, "", ""
}

func normalizeScopes(scope string, allowed map[string]struct{}) ([]string, string, error) {
	scopes := strings.Fields(scope)
	for _, requested := range scopes {
		if _, ok := allowed[requested]; !ok {
			return nil, "", fmt.Errorf("scope %q is not allowed", requested)
		}
	}
	return scopes, strings.Join(scopes, " "), nil
}

func normalizeScopesWithDefault(scope string, allowedScopes []string, allowed map[string]struct{}) ([]string, string, error) {
	if strings.TrimSpace(scope) != "" {
		return normalizeScopes(scope, allowed)
	}
	scopes := make([]string, 0, len(allowedScopes))
	seen := make(map[string]struct{}, len(allowedScopes))
	for _, allowedScope := range allowedScopes {
		allowedScope = strings.TrimSpace(allowedScope)
		if allowedScope == "" {
			continue
		}
		if _, duplicate := seen[allowedScope]; duplicate {
			continue
		}
		seen[allowedScope] = struct{}{}
		scopes = append(scopes, allowedScope)
	}
	return scopes, strings.Join(scopes, " "), nil
}

func validateRedirectURI(raw string) error {
	if raw == "" {
		return fmt.Errorf("redirect_uri is empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("redirect_uri %q: %w", raw, err)
	}
	if !parsed.IsAbs() {
		return fmt.Errorf("redirect_uri %q must be absolute", raw)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("redirect_uri %q must not contain a fragment", raw)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("redirect_uri %q: scheme %q is not supported", raw, parsed.Scheme)
	}
	return fmt.Errorf("redirect_uri %q: http is only allowed for loopback hosts", raw)
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func generateRandomURLValue(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("generate random value: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func writeRegistrationError(w http.ResponseWriter, status int, code string, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": description,
	}); err != nil {
		slog.Error("write registration error", "error", err)
	}
}
