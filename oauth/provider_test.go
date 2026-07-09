package oauth_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/keys"
	"github.com/haakco/mcp-kit/oauth/storage"
)

func TestNewRejectsInvalidSecretLength(t *testing.T) {
	_, err := oauth.New(oauth.Config{
		Issuer:     "https://mcp.example.test",
		Secret:     []byte("too-short"),
		Store:      storage.NewMemoryStore(),
		KeyManager: keys.NewManager(newMemoryKeyStore(t)),
	})
	if err == nil {
		t.Fatal("New() error is nil, want invalid secret error")
	}
}

type memoryKeyStore struct {
	mu   sync.Mutex
	keys []keys.SigningKey
}

func newMemoryKeyStore(t *testing.T) *memoryKeyStore {
	t.Helper()

	return &memoryKeyStore{}
}

func (s *memoryKeyStore) FindActiveSigningKey(context.Context) (keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range s.keys {
		if key.IsActive {
			return key, nil
		}
	}
	return keys.SigningKey{}, keys.ErrNotFound
}

func (s *memoryKeyStore) EnsureSigningKey(_ context.Context, key keys.SigningKey) (keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.keys {
		if existing.IsActive {
			return existing, nil
		}
	}
	s.keys = append(s.keys, key)
	return key, nil
}

func (s *memoryKeyStore) RotateSigningKey(_ context.Context, replacement keys.SigningKey, retiredAt time.Time) (keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.keys {
		if s.keys[i].IsActive {
			s.keys[i].IsActive = false
			s.keys[i].RetiredAt = &retiredAt
		}
	}
	s.keys = append(s.keys, replacement)
	return replacement, nil
}

func (s *memoryKeyStore) ListVerifyingSigningKeys(_ context.Context, now time.Time) ([]keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []keys.SigningKey
	for _, key := range s.keys {
		if key.IsActive || (key.RetiredAt != nil && key.RetiredAt.After(now)) {
			result = append(result, key)
		}
	}
	return result, nil
}

func (s *memoryKeyStore) DeleteExpiredSigningKeys(_ context.Context, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var kept []keys.SigningKey
	deleted := 0
	for _, key := range s.keys {
		if !key.IsActive && key.RetiredAt != nil && key.RetiredAt.Before(now) {
			deleted++
			continue
		}
		kept = append(kept, key)
	}
	s.keys = kept
	return deleted, nil
}

func TestRegisterPublicClient(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)

	request := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{
		"client_name":"Inspector",
		"redirect_uris":["http://127.0.0.1:9999/callback"],
		"grant_types":["authorization_code","refresh_token"],
		"response_types":["code"],
		"token_endpoint_auth_method":"none",
		"scope":"openid mcp.read"
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	provider.RegisterHandler().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode registration response: %v", err)
	}
	clientID, _ := payload["client_id"].(string)
	if clientID == "" {
		t.Fatalf("response missing client_id: %#v", payload)
	}
	if _, ok := payload["client_secret"]; ok {
		t.Fatalf("public client response includes secret: %#v", payload)
	}
	if got := payload["token_endpoint_auth_method"]; got != "none" {
		t.Fatalf("auth method = %#v, want none", got)
	}
	if got := payload["scope"]; got != "openid mcp.read" {
		t.Fatalf("scope = %#v, want openid mcp.read", got)
	}

	client, err := store.GetClient(t.Context(), clientID)
	if err != nil {
		t.Fatalf("GetClient() error = %v", err)
	}
	if !client.IsPublic {
		t.Fatal("stored client IsPublic = false, want true")
	}
	if len(client.Audience) != 1 || client.Audience[0] != "https://mcp.example.test/mcp" {
		t.Fatalf("stored audience = %#v, want issuer /mcp", client.Audience)
	}
}

func TestRegisterPublicClientIgnoresUnknownMetadata(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)

	request := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{
		"client_name":"Inspector",
		"redirect_uris":["http://127.0.0.1:9999/callback"],
		"grant_types":["authorization_code","refresh_token"],
		"response_types":["code"],
		"token_endpoint_auth_method":"none",
		"scope":"openid mcp.read",
		"client_uri":"http://localhost:6274",
		"contacts":["dev@example.com"],
		"software_id":"mcp-inspector",
		"software_version":"1.0.0",
		"application_type":"native"
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	provider.RegisterHandler().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "client_uri") {
		t.Fatalf("registration response leaked unsupported metadata: %s", response.Body.String())
	}
}

func TestRegisterDefaultsOmittedScopeToProviderDefaultScopes(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)

	request := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{
		"client_name":"Codex",
		"redirect_uris":["http://127.0.0.1:59771/callback"],
		"grant_types":["authorization_code","refresh_token"],
		"response_types":["code"],
		"token_endpoint_auth_method":"none"
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	provider.RegisterHandler().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode registration response: %v", err)
	}
	if got := payload["scope"]; got != "openid mcp.read offline_access" {
		t.Fatalf("scope = %#v, want provider default scopes", got)
	}

	clientID, _ := payload["client_id"].(string)
	client, err := store.GetClient(t.Context(), clientID)
	if err != nil {
		t.Fatalf("GetClient() error = %v", err)
	}
	if got := strings.Join(client.Scopes, " "); got != "openid mcp.read offline_access" {
		t.Fatalf("stored scopes = %q, want provider default scopes", got)
	}
}

func TestRegistrationHandlerCanDefaultPublicClientAuth(t *testing.T) {
	store := storage.NewMemoryStore()
	handler := oauth.NewRegistrationHandler(oauth.RegistrationConfig{
		Store:                          store,
		AllowedScopes:                  []string{"mcp.read", "mcp.write", "offline_access"},
		DefaultScopes:                  []string{"mcp.read", "mcp.write", "offline_access"},
		Audience:                       "https://mcp.example.test/mcp",
		DefaultTokenEndpointAuthMethod: "none",
		DefaultGrantTypes:              []string{"authorization_code", "refresh_token"},
		DefaultResponseTypes:           []string{"code"},
		ClientIDPrefix:                 "mcp-",
	})

	request := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{
		"redirect_uris":["http://127.0.0.1/callback"]
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["scope"]; got != "mcp.read mcp.write offline_access" {
		t.Fatalf("scope = %#v, want default scopes", got)
	}
	clientID, _ := payload["client_id"].(string)
	if !strings.HasPrefix(clientID, "mcp-") {
		t.Fatalf("client_id = %q, want mcp- prefix", clientID)
	}
	if got := payload["token_endpoint_auth_method"]; got != "none" {
		t.Fatalf("token_endpoint_auth_method = %#v, want none", got)
	}
	if got := stringSliceFromPayload(t, payload, "grant_types"); !slices.Equal(got, []string{"authorization_code", "refresh_token"}) {
		t.Fatalf("grant_types = %#v, want authorization_code refresh_token", got)
	}
	if got := stringSliceFromPayload(t, payload, "response_types"); !slices.Equal(got, []string{"code"}) {
		t.Fatalf("response_types = %#v, want code", got)
	}
	if _, ok := payload["client_secret"]; ok {
		t.Fatalf("public registration returned client_secret: %#v", payload)
	}
	client, err := store.GetClient(t.Context(), clientID)
	if err != nil {
		t.Fatalf("GetClient() error = %v", err)
	}
	if !client.IsPublic {
		t.Fatal("stored client IsPublic = false, want true")
	}
	if client.ClientSecretHash != "" {
		t.Fatalf("stored client secret hash = %q, want empty", client.ClientSecretHash)
	}
	if client.TokenAuthMethod != "none" {
		t.Fatalf("stored token auth method = %q, want none", client.TokenAuthMethod)
	}
}

func stringSliceFromPayload(t *testing.T, payload map[string]any, field string) []string {
	t.Helper()

	raw, ok := payload[field].([]any)
	if !ok {
		t.Fatalf("%s = %#v, want array", field, payload[field])
	}
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("%s contains %#v, want string", field, item)
		}
		values = append(values, value)
	}
	return values
}

func TestRegisterRejectsUnsafeRedirectSchemes(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)

	for _, redirectURI := range []string{"javascript:alert(1)", "data:text/html,hi", "file:///tmp/callback", "http://example.com/callback"} {
		t.Run(redirectURI, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{
				"client_name":"Unsafe",
				"redirect_uris":[`+strconv.Quote(redirectURI)+`],
				"grant_types":["authorization_code"],
				"response_types":["code"],
				"token_endpoint_auth_method":"none",
				"scope":"openid mcp.read"
			}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			provider.RegisterHandler().ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAuthorizationCodePKCEFlow(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)
	savePKCEClient(t, store)

	server := newOAuthTestServer(provider)
	defer server.Close()

	verifier := "test-code-verifier-1234567890-must-be-at-least-43-characters-long"
	code := authorizeCode(t, server, verifier, "state-123456")

	tokenResponse := exchangeCode(t, server, code, verifier)
	defer func() { _ = tokenResponse.Body.Close() }()
	if tokenResponse.StatusCode != http.StatusOK {
		t.Fatalf("token status = %d, want 200", tokenResponse.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(tokenResponse.Body).Decode(&payload); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	for _, field := range []string{"access_token", "refresh_token", "id_token"} {
		if payload[field] == "" {
			t.Fatalf("token response missing %s: %#v", field, payload)
		}
	}
}

func TestAuthorizeRejectsBadState(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)
	savePKCEClient(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	client := noRedirectClient()
	response, err := client.Get(authorizeURL(server.URL, "valid-verifier-1234567890-valid-verifier-1234567890", "short"))
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusSeeOther {
		t.Fatal("authorize accepted short state, want rejection")
	}
}

func TestAuthorizeRejectsBadPKCE(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)
	savePKCEClient(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	response, err := noRedirectClient().Get(server.URL + "/oauth/authorize?" + url.Values{
		"client_id":             {"pkce-client"},
		"redirect_uri":          {"http://127.0.0.1/callback"},
		"response_type":         {"code"},
		"scope":                 {"openid mcp.read"},
		"state":                 {"state-123456"},
		"code_challenge":        {"not-valid-base64url!!!!"},
		"code_challenge_method": {"S256"},
	}.Encode())
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusSeeOther {
		t.Fatal("authorize accepted invalid code_challenge, want rejection")
	}
}

func TestTokenRejectsBadPKCE(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)
	savePKCEClient(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	code := authorizeCode(t, server, "test-code-verifier-1234567890-must-be-at-least-43-characters-long", "state-123456")
	response := exchangeCode(t, server, code, "wrong-code-verifier-1234567890-must-be-at-least-43-characters-long")
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusOK {
		t.Fatal("token exchange accepted wrong PKCE verifier")
	}
}

func TestRefreshTokenRotation(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)
	savePKCEClient(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	code := authorizeCode(t, server, "test-code-verifier-1234567890-must-be-at-least-43-characters-long", "state-123456")
	tokenResponse := exchangeCode(t, server, code, "test-code-verifier-1234567890-must-be-at-least-43-characters-long")
	defer func() { _ = tokenResponse.Body.Close() }()
	refreshToken := decodeTokenField(t, tokenResponse, "refresh_token")

	rotated := refreshTokenRequest(t, server.URL, refreshToken)
	defer func() { _ = rotated.Body.Close() }()
	if rotated.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(rotated.Body)
		t.Fatalf("refresh status = %d, want 200; body=%s", rotated.StatusCode, string(body))
	}

	reused := refreshTokenRequest(t, server.URL, refreshToken)
	defer func() { _ = reused.Body.Close() }()
	if reused.StatusCode == http.StatusOK {
		t.Fatal("refresh token reuse succeeded, want rotation to invalidate old token")
	}
}

func TestRefreshTokenReplayWindowReturnsCachedRotation(t *testing.T) {
	store := storage.NewMemoryStore()
	now := time.Date(2026, 7, 8, 16, 0, 0, 0, time.UTC)
	provider := newTestProviderWithConfig(t, store, oauth.Config{
		RefreshReplayWindow: 5 * time.Minute,
		Now:                 func() time.Time { return now },
	})
	savePKCEClient(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	code := authorizeCode(t, server, "test-code-verifier-1234567890-must-be-at-least-43-characters-long", "state-123456")
	tokenResponse := exchangeCode(t, server, code, "test-code-verifier-1234567890-must-be-at-least-43-characters-long")
	defer func() { _ = tokenResponse.Body.Close() }()
	refreshToken := decodeTokenField(t, tokenResponse, "refresh_token")

	first := refreshTokenRequest(t, server.URL, refreshToken)
	defer func() { _ = first.Body.Close() }()
	firstBody := readResponseBody(t, first)
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first refresh status = %d, want 200; body=%s", first.StatusCode, firstBody)
	}

	second := refreshTokenRequest(t, server.URL, refreshToken)
	defer func() { _ = second.Body.Close() }()
	secondBody := readResponseBody(t, second)
	if second.StatusCode != http.StatusOK {
		t.Fatalf("second refresh status = %d, want replay 200; body=%s", second.StatusCode, secondBody)
	}
	if secondBody != firstBody {
		t.Fatalf("replayed body changed\nfirst:  %s\nsecond: %s", firstBody, secondBody)
	}
}

func TestRefreshTokenReplayWindowHandlesConcurrentReuse(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProviderWithConfig(t, store, oauth.Config{
		RefreshReplayWindow: 5 * time.Minute,
	})
	savePKCEClient(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	code := authorizeCode(t, server, "test-code-verifier-1234567890-must-be-at-least-43-characters-long", "state-123456")
	tokenResponse := exchangeCode(t, server, code, "test-code-verifier-1234567890-must-be-at-least-43-characters-long")
	defer func() { _ = tokenResponse.Body.Close() }()
	refreshToken := decodeTokenField(t, tokenResponse, "refresh_token")

	const attempts = 6
	start := make(chan struct{})
	var wg sync.WaitGroup
	bodies := make([]string, attempts)
	statuses := make([]int, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			response := refreshTokenRequest(t, server.URL, refreshToken)
			defer func() { _ = response.Body.Close() }()
			statuses[index] = response.StatusCode
			bodies[index] = readResponseBody(t, response)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("refresh attempt %d status = %d, want replay 200; body=%s", i, status, bodies[i])
		}
		if bodies[i] != bodies[0] {
			t.Fatalf("refresh attempt %d body changed\nfirst: %s\n got:  %s", i, bodies[0], bodies[i])
		}
	}
}

func TestRefreshTokenReplayWindowExpires(t *testing.T) {
	store := storage.NewMemoryStore()
	now := time.Date(2026, 7, 8, 16, 0, 0, 0, time.UTC)
	provider := newTestProviderWithConfig(t, store, oauth.Config{
		RefreshReplayWindow: 5 * time.Minute,
		Now:                 func() time.Time { return now },
	})
	savePKCEClient(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	code := authorizeCode(t, server, "test-code-verifier-1234567890-must-be-at-least-43-characters-long", "state-123456")
	tokenResponse := exchangeCode(t, server, code, "test-code-verifier-1234567890-must-be-at-least-43-characters-long")
	defer func() { _ = tokenResponse.Body.Close() }()
	refreshToken := decodeTokenField(t, tokenResponse, "refresh_token")

	first := refreshTokenRequest(t, server.URL, refreshToken)
	defer func() { _ = first.Body.Close() }()
	firstBody := readResponseBody(t, first)
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first refresh status = %d, want 200; body=%s", first.StatusCode, firstBody)
	}

	now = now.Add(5*time.Minute + time.Second)
	reused := refreshTokenRequest(t, server.URL, refreshToken)
	defer func() { _ = reused.Body.Close() }()
	if reused.StatusCode == http.StatusOK {
		t.Fatal("expired replay window accepted stale refresh token")
	}
}

func TestTokenEndpointEmitsAuditEvents(t *testing.T) {
	store := storage.NewMemoryStore()
	emitter := &recordingAuditEmitter{}
	provider := newTestProviderWithConfig(t, store, oauth.Config{AuditEmitter: emitter})
	savePKCEClient(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	code := authorizeCode(t, server, "test-code-verifier-1234567890-must-be-at-least-43-characters-long", "state-123456")
	tokenResponse := exchangeCode(t, server, code, "test-code-verifier-1234567890-must-be-at-least-43-characters-long")
	defer func() { _ = tokenResponse.Body.Close() }()
	refreshToken := decodeTokenField(t, tokenResponse, "refresh_token")

	rotated := refreshTokenRequest(t, server.URL, refreshToken)
	defer func() { _ = rotated.Body.Close() }()
	refreshBody := readResponseBody(t, rotated)
	if rotated.StatusCode != http.StatusOK {
		t.Fatalf("refresh status = %d, want 200; body=%s", rotated.StatusCode, refreshBody)
	}

	reused := refreshTokenRequest(t, server.URL, refreshToken)
	defer func() { _ = reused.Body.Close() }()
	reuseBody := readResponseBody(t, reused)
	if reused.StatusCode == http.StatusOK {
		t.Fatalf("reuse unexpectedly succeeded; body=%s", reuseBody)
	}

	assertAuditEvent(t, emitter.events, "oauth_token", "issued", "authorization_code")
	assertAuditEvent(t, emitter.events, "oauth_refresh", "rotated", "refresh_token")
	assertAuditEvent(t, emitter.events, "oauth_token_exchange", "failed", "refresh_token")
}

func TestTokenInvalidGrantEnvelopeOnPKCEFailure(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)
	savePKCEClient(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	code := authorizeCode(t, server, "test-code-verifier-1234567890-must-be-at-least-43-characters-long", "state-123456")
	response := exchangeCode(t, server, code, "wrong-code-verifier-1234567890-must-be-at-least-43-characters-long")
	defer func() { _ = response.Body.Close() }()

	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode token error response: %v", err)
	}
	if payload["error"] != "invalid_grant" {
		t.Fatalf("error = %#v, want invalid_grant; payload=%#v", payload["error"], payload)
	}
	if _, ok := payload["error_description"].(string); !ok {
		t.Fatalf("payload missing error_description: %#v", payload)
	}
}

func TestTokenHandlerRejectsOversizedForm(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	form := url.Values{
		"grant_type": {"client_credentials"},
		"client_id":  {strings.Repeat("a", 2<<20)},
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST token: %v", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusRequestEntityTooLarge {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want 413; body=%s", response.StatusCode, string(body))
	}
}

func TestRevokeIdempotentSuccess(t *testing.T) {
	store := storage.NewMemoryStore()
	provider := newTestProvider(t, store)
	savePKCEClient(t, store)
	server := newOAuthTestServer(provider)
	defer server.Close()

	response, err := http.PostForm(server.URL+"/oauth/revoke", url.Values{
		"client_id": {"pkce-client"},
		"token":     {"nonexistent-token"},
	})
	if err != nil {
		t.Fatalf("POST revoke: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d, want 200", response.StatusCode)
	}
}

func newTestProvider(t *testing.T, store storage.Store) *oauth.Provider {
	t.Helper()
	return newTestProviderWithConfig(t, store, oauth.Config{})
}

func newTestProviderWithConfig(t *testing.T, store storage.Store, cfg oauth.Config) *oauth.Provider {
	t.Helper()

	keyStore := newMemoryKeyStore(t)
	manager := keys.NewManager(keyStore)
	if _, err := manager.EnsureSigningKey(t.Context()); err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}

	cfg.Issuer = "https://mcp.example.test"
	cfg.Secret = []byte("test-secret-must-be-32-bytes!!!!")
	cfg.Store = store
	cfg.KeyManager = manager
	cfg.AllowedScopes = []string{"openid", "mcp.read", "mcp.write", "offline_access"}
	cfg.DefaultScopes = []string{"openid", "mcp.read", "offline_access"}
	provider, err := oauth.New(oauth.Config{
		Issuer:              cfg.Issuer,
		Secret:              cfg.Secret,
		Store:               cfg.Store,
		KeyManager:          cfg.KeyManager,
		AllowedScopes:       cfg.AllowedScopes,
		DefaultScopes:       cfg.DefaultScopes,
		AuditEmitter:        cfg.AuditEmitter,
		RefreshReplayWindow: cfg.RefreshReplayWindow,
		Now:                 cfg.Now,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return provider
}

func savePKCEClient(t *testing.T, store *storage.MemoryStore) {
	t.Helper()

	if err := store.SaveClient(t.Context(), storage.Client{
		ID:            "pkce-client",
		Name:          "PKCE Client",
		RedirectURIs:  []string{"http://127.0.0.1/callback"},
		GrantTypes:    []string{"authorization_code", "refresh_token"},
		ResponseTypes: []string{"code"},
		Scopes:        []string{"openid", "mcp.read"},
		Audience:      []string{"https://mcp.example.test/mcp"},
		IsPublic:      true,
	}); err != nil {
		t.Fatalf("SaveClient() error = %v", err)
	}
}

func newOAuthTestServer(provider *oauth.Provider) *httptest.Server {
	mux := http.NewServeMux()
	provider.RegisterRoutes(mux, "/oauth", func(*http.Request) (oauth.Subject, error) {
		return oauth.Subject{ID: "user-123", Email: "user@example.test", GrantedScopes: []string{"openid", "mcp.read"}}, nil
	})
	return httptest.NewServer(mux)
}

func authorizeCode(t *testing.T, server *httptest.Server, verifier string, state string) string {
	t.Helper()

	response, err := noRedirectClient().Get(authorizeURL(server.URL, verifier, state))
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("authorize status = %d, want 303", response.StatusCode)
	}

	callbackURL, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse callback URL: %v", err)
	}
	if callbackURL.Query().Get("state") != state {
		t.Fatalf("state = %q, want %q", callbackURL.Query().Get("state"), state)
	}
	code := callbackURL.Query().Get("code")
	if code == "" {
		t.Fatalf("missing code in callback URL: %s", callbackURL.String())
	}
	return code
}

func authorizeURL(baseURL string, verifier string, state string) string {
	return baseURL + "/oauth/authorize?" + url.Values{
		"client_id":             {"pkce-client"},
		"redirect_uri":          {"http://127.0.0.1/callback"},
		"response_type":         {"code"},
		"scope":                 {"openid mcp.read"},
		"state":                 {state},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}.Encode()
}

func exchangeCode(t *testing.T, server *httptest.Server, code string, verifier string) *http.Response {
	t.Helper()

	response, err := http.PostForm(server.URL+"/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {"pkce-client"},
		"code":          {code},
		"redirect_uri":  {"http://127.0.0.1/callback"},
		"code_verifier": {verifier},
	})
	if err != nil {
		t.Fatalf("POST token: %v", err)
	}
	return response
}

func refreshTokenRequest(t *testing.T, serverURL string, refreshToken string) *http.Response {
	t.Helper()

	response, err := http.PostForm(serverURL+"/oauth/token", url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {"pkce-client"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		t.Fatalf("POST refresh token: %v", err)
	}
	return response
}

func decodeTokenField(t *testing.T, response *http.Response, field string) string {
	t.Helper()
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("token status = %d, want 200; body=%s", response.StatusCode, string(body))
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	value, _ := payload[field].(string)
	if value == "" {
		t.Fatalf("token response missing %s: %#v", field, payload)
	}
	return value
}

func readResponseBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}

type recordingAuditEmitter struct {
	events []audit.Event
}

func (r *recordingAuditEmitter) Emit(_ context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}

func assertAuditEvent(t *testing.T, events []audit.Event, entityType string, action string, grantType string) {
	t.Helper()
	for _, event := range events {
		if event.EntityType != entityType || event.Action != action {
			continue
		}
		if event.Metadata["grant_type"] != grantType {
			t.Fatalf("event %s/%s grant_type = %#v, want %q", entityType, action, event.Metadata["grant_type"], grantType)
		}
		return
	}
	t.Fatalf("missing audit event %s/%s grant_type=%s; events=%#v", entityType, action, grantType, events)
}

func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
