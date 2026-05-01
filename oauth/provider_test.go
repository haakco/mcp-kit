package oauth_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

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
	if !strings.Contains(response.Body.String(), `"client_id"`) {
		t.Fatalf("response missing client_id: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "client_secret") {
		t.Fatalf("public client response includes secret: %s", response.Body.String())
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
	defer tokenResponse.Body.Close()
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
	defer response.Body.Close()
	if response.StatusCode == http.StatusSeeOther {
		t.Fatal("authorize accepted short state, want rejection")
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
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		t.Fatal("token exchange accepted wrong PKCE verifier")
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
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d, want 200", response.StatusCode)
	}
}

func newTestProvider(t *testing.T, store storage.Store) *oauth.Provider {
	t.Helper()

	keyStore := newMemoryKeyStore(t)
	manager := keys.NewManager(keyStore)
	if _, err := manager.EnsureSigningKey(t.Context()); err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}

	provider, err := oauth.New(oauth.Config{
		Issuer:        "https://mcp.example.test",
		Secret:        []byte("test-secret-must-be-32-bytes!!!!"),
		Store:         store,
		KeyManager:    manager,
		AllowedScopes: []string{"openid", "mcp.read", "mcp.write", "offline_access"},
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
		IsPublic:      true,
	}); err != nil {
		t.Fatalf("SaveClient() error = %v", err)
	}
}

func newOAuthTestServer(provider *oauth.Provider) *httptest.Server {
	mux := http.NewServeMux()
	provider.RegisterRoutes(mux, "/oauth", func(*http.Request) (oauth.Subject, error) {
		return oauth.Subject{ID: "user-123", Email: "user@example.test"}, nil
	})
	return httptest.NewServer(mux)
}

func authorizeCode(t *testing.T, server *httptest.Server, verifier string, state string) string {
	t.Helper()

	response, err := noRedirectClient().Get(authorizeURL(server.URL, verifier, state))
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer response.Body.Close()
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

func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
