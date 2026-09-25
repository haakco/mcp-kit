package oauth_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/storage"
)

// metadataServer serves a Client ID Metadata Document. It runs on loopback, so
// tests must enable AllowLoopback for the fetcher to reach it.
type metadataServer struct {
	server *httptest.Server
	hits   atomic.Int32
	// body overrides the document served. Empty means a valid document.
	body string
	// header overrides response headers.
	header map[string]string
	// status overrides the response status.
	status int
}

func newMetadataServer(t *testing.T) *metadataServer {
	t.Helper()
	m := &metadataServer{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.hits.Add(1)
		for name, value := range m.header {
			w.Header().Set(name, value)
		}
		if m.status != 0 {
			w.WriteHeader(m.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := m.body
		if body == "" {
			body = m.validDocument()
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(m.server.Close)
	return m
}

// clientID returns the URL used as the client_id for this server.
func (m *metadataServer) clientID() string { return m.server.URL + "/client-metadata.json" }

func (m *metadataServer) validDocument() string {
	return fmt.Sprintf(`{
		"client_id": %q,
		"client_name": "Test Client",
		"redirect_uris": ["http://127.0.0.1:8765/callback"],
		"grant_types": ["authorization_code", "refresh_token"],
		"response_types": ["code"],
		"token_endpoint_auth_method": "none",
		"scope": "mcp.read"
	}`, m.clientID())
}

func newTestFetcher(t *testing.T, cfg oauth.ClientIDMetadataConfig, now func() time.Time) *oauth.ClientIDMetadataFetcher {
	t.Helper()
	cfg.AllowLoopback = true
	if now == nil {
		now = time.Now
	}
	return oauth.NewClientIDMetadataFetcher(cfg, oauth.ClientMetadataPolicy{
		Audience:      "https://mcp.example.test/mcp",
		AllowedScopes: []string{"openid", "mcp.read", "mcp.write"},
		DefaultScopes: []string{"openid"},
	}, now)
}

func TestClientIDMetadataResolvesValidDocument(t *testing.T) {
	metadata := newMetadataServer(t)
	fetcher := newTestFetcher(t, oauth.ClientIDMetadataConfig{}, nil)

	client, err := fetcher.ResolveClient(t.Context(), metadata.clientID())
	if err != nil {
		t.Fatalf("ResolveClient: %v", err)
	}
	if client.ID != metadata.clientID() {
		t.Fatalf("client ID = %q, want the document URL", client.ID)
	}
	if client.Name != "Test Client" {
		t.Fatalf("client name = %q, want Test Client", client.Name)
	}
	if !client.IsPublic {
		t.Fatal("metadata-document clients must be public")
	}
	if client.TokenAuthMethod != "none" {
		t.Fatalf("token endpoint auth method = %q, want none", client.TokenAuthMethod)
	}
	if len(client.RedirectURIs) != 1 || client.RedirectURIs[0] != "http://127.0.0.1:8765/callback" {
		t.Fatalf("redirect URIs = %#v", client.RedirectURIs)
	}
	if strings.Join(client.Scopes, " ") != "mcp.read" {
		t.Fatalf("scopes = %#v, want [mcp.read]", client.Scopes)
	}
}

func TestClientIDMetadataRejectsNonURLClientID(t *testing.T) {
	fetcher := newTestFetcher(t, oauth.ClientIDMetadataConfig{}, nil)

	_, err := fetcher.ResolveClient(t.Context(), "registered-client-id")
	if !errors.Is(err, storage.ErrNotAClientMetadataURL) {
		t.Fatalf("error = %v, want storage.ErrNotAClientMetadataURL", err)
	}
}

func TestClientIDMetadataRejectsUnsafeClientIDsWithoutFetching(t *testing.T) {
	metadata := newMetadataServer(t)
	fetcher := newTestFetcher(t, oauth.ClientIDMetadataConfig{}, nil)

	unacceptable := []struct {
		name     string
		clientID string
	}{
		{"plain http off loopback", "http://example.test/client.json"},
		{"non-https scheme", "ftp://example.test/client.json"},
		{"root path", metadata.server.URL + "/"},
		{"no path", metadata.server.URL},
		{"fragment", metadata.clientID() + "#frag"},
		{"user info", strings.Replace(metadata.clientID(), "http://", "http://user:pass@", 1)},
	}

	for _, tt := range unacceptable {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fetcher.ResolveClient(t.Context(), tt.clientID)
			if !errors.Is(err, storage.ErrNotAClientMetadataURL) {
				t.Fatalf("error = %v, want storage.ErrNotAClientMetadataURL", err)
			}
		})
	}
	if hits := metadata.hits.Load(); hits != 0 {
		t.Fatalf("fetcher made %d HTTP requests for unusable client IDs, want 0", hits)
	}
}

func TestClientIDMetadataRequiresAllowLoopbackForLocalhost(t *testing.T) {
	metadata := newMetadataServer(t)
	fetcher := oauth.NewClientIDMetadataFetcher(
		oauth.ClientIDMetadataConfig{},
		oauth.ClientMetadataPolicy{AllowedScopes: []string{"mcp.read"}, DefaultScopes: []string{"mcp.read"}},
		time.Now,
	)

	_, err := fetcher.ResolveClient(t.Context(), metadata.clientID())
	if !errors.Is(err, storage.ErrNotAClientMetadataURL) {
		t.Fatalf("error = %v, want storage.ErrNotAClientMetadataURL when loopback is not allowed", err)
	}
	if hits := metadata.hits.Load(); hits != 0 {
		t.Fatalf("fetcher contacted loopback %d times without AllowLoopback", hits)
	}
}

func TestClientIDMetadataRejectsInvalidDocuments(t *testing.T) {
	metadata := newMetadataServer(t)
	fetcher := newTestFetcher(t, oauth.ClientIDMetadataConfig{}, nil)

	tests := []struct {
		name    string
		body    string
		status  int
		wantErr string
	}{
		{
			name:    "client_id does not match the URL",
			body:    `{"client_id":"https://other.example.test/client.json","redirect_uris":["http://127.0.0.1:8765/callback"]}`,
			wantErr: "does not match",
		},
		{
			name:    "missing redirect_uris",
			body:    fmt.Sprintf(`{"client_id":%q}`, metadata.clientID()),
			wantErr: "redirect_uris is required",
		},
		{
			name: "unsafe redirect uri",
			body: fmt.Sprintf(
				`{"client_id":%q,"redirect_uris":["http://example.test/callback"]}`, metadata.clientID()),
			wantErr: "loopback",
		},
		{
			name: "redirect uri with fragment",
			body: fmt.Sprintf(
				`{"client_id":%q,"redirect_uris":["http://127.0.0.1:8765/callback#x"]}`, metadata.clientID()),
			wantErr: "fragment",
		},
		{
			name: "unsupported grant type",
			body: fmt.Sprintf(
				`{"client_id":%q,"redirect_uris":["http://127.0.0.1:8765/callback"],"grant_types":["client_credentials"]}`,
				metadata.clientID()),
			wantErr: "grant type",
		},
		{
			name: "unsupported response type",
			body: fmt.Sprintf(
				`{"client_id":%q,"redirect_uris":["http://127.0.0.1:8765/callback"],"response_types":["token"]}`,
				metadata.clientID()),
			wantErr: "response type",
		},
		{
			name: "unsupported application type",
			body: fmt.Sprintf(
				`{"client_id":%q,"redirect_uris":["http://127.0.0.1:8765/callback"],"application_type":"desktop"}`,
				metadata.clientID()),
			wantErr: "application_type",
		},
		{
			name: "confidential auth method",
			body: fmt.Sprintf(
				`{"client_id":%q,"redirect_uris":["http://127.0.0.1:8765/callback"],"token_endpoint_auth_method":"client_secret_basic"}`,
				metadata.clientID()),
			wantErr: "client secret",
		},
		{
			name: "scope outside the server policy",
			body: fmt.Sprintf(
				`{"client_id":%q,"redirect_uris":["http://127.0.0.1:8765/callback"],"scope":"mcp.admin"}`,
				metadata.clientID()),
			wantErr: "not allowed",
		},
		{
			name:    "non-200 status",
			status:  http.StatusNotFound,
			wantErr: "status 404",
		},
		{
			name:    "malformed json",
			body:    `{`,
			wantErr: "decode",
		},
		{
			name:    "oversized document",
			body:    fmt.Sprintf(`{"client_id":%q,"client_name":%q}`, metadata.clientID(), strings.Repeat("a", 128<<10)),
			wantErr: "decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata.body = tt.body
			metadata.status = tt.status

			_, err := fetcher.ResolveClient(t.Context(), metadata.clientID())
			if err == nil {
				t.Fatalf("ResolveClient accepted an invalid document")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestClientIDMetadataCachesDocuments(t *testing.T) {
	metadata := newMetadataServer(t)
	now := time.Now()
	fetcher := newTestFetcher(t, oauth.ClientIDMetadataConfig{CacheTTL: time.Minute}, func() time.Time { return now })

	if _, err := fetcher.ResolveClient(t.Context(), metadata.clientID()); err != nil {
		t.Fatalf("first ResolveClient: %v", err)
	}
	if _, err := fetcher.ResolveClient(t.Context(), metadata.clientID()); err != nil {
		t.Fatalf("second ResolveClient: %v", err)
	}
	if hits := metadata.hits.Load(); hits != 1 {
		t.Fatalf("HTTP hits = %d, want 1 (second lookup served from cache)", hits)
	}

	now = now.Add(2 * time.Minute)
	if _, err := fetcher.ResolveClient(t.Context(), metadata.clientID()); err != nil {
		t.Fatalf("third ResolveClient: %v", err)
	}
	if hits := metadata.hits.Load(); hits != 2 {
		t.Fatalf("HTTP hits = %d, want 2 after the cache entry expired", hits)
	}
}

func TestClientIDMetadataHonoursCacheControlHeaders(t *testing.T) {
	tests := []struct {
		name       string
		header     map[string]string
		wantSecond int32
	}{
		{
			name:       "no-store refetches every time",
			header:     map[string]string{"Cache-Control": "no-store"},
			wantSecond: 2,
		},
		{
			name:       "max-age caches",
			header:     map[string]string{"Cache-Control": "max-age=300"},
			wantSecond: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := newMetadataServer(t)
			metadata.header = tt.header
			fetcher := newTestFetcher(t, oauth.ClientIDMetadataConfig{}, nil)

			for range 2 {
				if _, err := fetcher.ResolveClient(t.Context(), metadata.clientID()); err != nil {
					t.Fatalf("ResolveClient: %v", err)
				}
			}
			if hits := metadata.hits.Load(); hits != tt.wantSecond {
				t.Fatalf("HTTP hits = %d, want %d", hits, tt.wantSecond)
			}
		})
	}
}

// TestClientIDMetadataTransportBlocksPrivateTargets proves the default fetch
// client refuses loopback even when a document URL is shaped correctly, which
// is the SSRF boundary an authorization server cannot rely on callers to
// respect.
func TestClientIDMetadataTransportBlocksPrivateTargets(t *testing.T) {
	metadata := newMetadataServer(t)
	fetcher := oauth.NewClientIDMetadataFetcher(
		oauth.ClientIDMetadataConfig{AllowLoopback: false},
		oauth.ClientMetadataPolicy{AllowedScopes: []string{"mcp.read"}, DefaultScopes: []string{"mcp.read"}},
		time.Now,
	)

	// The URL shape check rejects http loopback before any dial happens, so the
	// request never reaches the server.
	if _, err := fetcher.ResolveClient(t.Context(), metadata.clientID()); !errors.Is(err, storage.ErrNotAClientMetadataURL) {
		t.Fatalf("error = %v, want ErrNotAClientMetadataURL", err)
	}
	if hits := metadata.hits.Load(); hits != 0 {
		t.Fatalf("HTTP hits = %d, want 0", hits)
	}
}

func TestStoragePrefersRegisteredClientOverMetadataDocument(t *testing.T) {
	store := storage.NewMemoryStore()
	if err := store.SaveClient(t.Context(), storage.Client{
		ID:           "registered",
		Name:         "Registered",
		RedirectURIs: []string{"https://app.example.test/callback"},
	}); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}

	metadata := newMetadataServer(t)
	fetcher := newTestFetcher(t, oauth.ClientIDMetadataConfig{}, nil)
	adapter := storage.New(store).WithClientMetadataResolver(fetcher)

	client, err := adapter.GetClient(t.Context(), "registered")
	if err != nil {
		t.Fatalf("GetClient(registered): %v", err)
	}
	if client.GetID() != "registered" {
		t.Fatalf("client ID = %q, want registered", client.GetID())
	}
	if hits := metadata.hits.Load(); hits != 0 {
		t.Fatalf("registered client lookup hit the network %d times", hits)
	}
}

func TestStorageResolvesUnknownMetadataClientThroughFetcher(t *testing.T) {
	metadata := newMetadataServer(t)
	fetcher := newTestFetcher(t, oauth.ClientIDMetadataConfig{}, nil)
	adapter := storage.New(storage.NewMemoryStore()).WithClientMetadataResolver(fetcher)

	client, err := adapter.GetClient(t.Context(), metadata.clientID())
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if client.GetID() != metadata.clientID() {
		t.Fatalf("client ID = %q, want the metadata URL", client.GetID())
	}
	if !client.IsPublic() {
		t.Fatal("metadata client must be public")
	}
}

func TestStorageReturnsNotFoundForUnresolvableMetadataClient(t *testing.T) {
	metadata := newMetadataServer(t)
	metadata.status = http.StatusInternalServerError
	fetcher := newTestFetcher(t, oauth.ClientIDMetadataConfig{}, nil)
	adapter := storage.New(storage.NewMemoryStore()).WithClientMetadataResolver(fetcher)

	// A fetch failure must not surface as a 500: an unusable client_id is an
	// invalid client, and the cause stays out of the response body.
	_, err := adapter.GetClient(t.Context(), metadata.clientID())
	if err == nil {
		t.Fatal("GetClient succeeded for a failing metadata document")
	}
	if strings.Contains(err.Error(), "status 500") {
		t.Fatalf("error leaked the fetch failure to the caller: %v", err)
	}
}

func TestStorageWithoutResolverFailsClosed(t *testing.T) {
	adapter := storage.New(storage.NewMemoryStore())

	_, err := adapter.GetClient(t.Context(), "https://client.example.test/metadata.json")
	if err == nil {
		t.Fatal("GetClient resolved a metadata URL without a resolver")
	}
}

// TestProviderRejectsUnknownClientWithMetadataDisabled covers the consumer
// opt-out path end to end through the authorize endpoint: with the feature off,
// a metadata URL is an unknown client and nothing is fetched.
func TestProviderRejectsUnknownClientWithMetadataDisabled(t *testing.T) {
	metadata := newMetadataServer(t)
	provider := newTestProviderWithConfig(t, storage.NewMemoryStore(), oauth.Config{
		ClientIDMetadata: oauth.ClientIDMetadataConfig{Disabled: true},
	})
	server := newOAuthTestServer(provider)
	defer server.Close()

	response, err := noRedirectClient().Get(metadataAuthorizeURL(server.URL, metadata.clientID()))
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusSeeOther {
		t.Fatal("authorize accepted a metadata client while the feature is disabled")
	}
	if hits := metadata.hits.Load(); hits != 0 {
		t.Fatalf("provider fetched the metadata document %d times while disabled", hits)
	}
}

// TestProviderResolvesMetadataClientByDefault guards the spec preference: MCP
// 2026-07-28 puts Client ID Metadata Documents ahead of dynamic registration.
func TestProviderResolvesMetadataClientByDefault(t *testing.T) {
	metadata := newMetadataServer(t)
	provider := newTestProviderWithConfig(t, storage.NewMemoryStore(), oauth.Config{
		ClientIDMetadata: oauth.ClientIDMetadataConfig{AllowLoopback: true},
	})
	server := newOAuthTestServer(provider)
	defer server.Close()

	response, err := noRedirectClient().Get(metadataAuthorizeURL(server.URL, metadata.clientID()))
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("authorize status = %d, want 303; body=%s", response.StatusCode, response.Body)
	}
	callbackURL, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse callback URL: %v", err)
	}
	if callbackURL.Query().Get("code") == "" {
		t.Fatalf("callback has no code: %s", callbackURL)
	}
	if callbackURL.Query().Get("iss") != "https://mcp.example.test" {
		t.Fatalf("iss = %q, want the configured issuer", callbackURL.Query().Get("iss"))
	}
}

// metadataAuthorizeURL builds a PKCE authorize request whose client_id is a
// Client ID Metadata Document URL.
func metadataAuthorizeURL(baseURL string, clientID string) string {
	const verifier = "test-code-verifier-1234567890-must-be-at-least-43-characters-long"
	return baseURL + "/oauth/authorize?" + url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {"http://127.0.0.1:8765/callback"},
		"response_type":         {"code"},
		"scope":                 {"mcp.read"},
		"state":                 {"state-123456"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}.Encode()
}
