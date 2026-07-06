package oauth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/openid"

	"github.com/haakco/mcp-kit/oauth"
)

var errTestResolver = errors.New("resolve target")

type mockIntrospector struct {
	validTokens map[string]*fosite.AccessRequest
}

func (m *mockIntrospector) IntrospectToken(
	_ context.Context,
	token string,
	_ fosite.TokenType,
	_ fosite.Session,
	_ ...string,
) (fosite.TokenType, fosite.AccessRequester, error) {
	if request, ok := m.validTokens[token]; ok {
		return fosite.AccessToken, request, nil
	}
	return "", nil, fosite.ErrInvalidTokenFormat
}

type mockPATValidator struct {
	mu        sync.Mutex
	result    *oauth.PATAuthResult
	usedIDs   []string
	lastToken string
	used      chan string
}

func (m *mockPATValidator) ValidateAndResolve(_ context.Context, rawToken string) (*oauth.PATAuthResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastToken = rawToken
	if m.result == nil {
		return nil, fosite.ErrInvalidTokenFormat
	}
	return m.result, nil
}

func (m *mockPATValidator) RecordUsage(_ context.Context, tokenID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.usedIDs = append(m.usedIDs, tokenID)
	if m.used != nil {
		m.used <- tokenID
	}
}

func TestBearerRejects401WithWWWAuthenticate(t *testing.T) {
	middleware := oauth.Bearer(oauth.BearerConfig{
		Introspector:        &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{}},
		ResourceMetadataURL: "https://mcp.example.test/.well-known/oauth-protected-resource",
		RequiredScopes:      []string{"openid", "mcp.read", "mcp.write"},
	})

	handler := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler was called without a bearer token")
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	authHeader := response.Header().Get("WWW-Authenticate")
	if !strings.Contains(authHeader, "Bearer") {
		t.Fatalf("WWW-Authenticate = %q, want Bearer challenge", authHeader)
	}
	if !strings.Contains(authHeader, "resource_metadata=") {
		t.Fatalf("WWW-Authenticate = %q, want resource metadata URL", authHeader)
	}
	if strings.Contains(authHeader, `scope=`) {
		t.Fatalf("WWW-Authenticate = %q, did not want scope hint on invalid_token challenge", authHeader)
	}
	if !strings.Contains(authHeader, `error="invalid_token"`) {
		t.Fatalf("WWW-Authenticate = %q, want invalid_token error", authHeader)
	}
	if !strings.Contains(authHeader, `error_description="Missing or invalid access token"`) {
		t.Fatalf("WWW-Authenticate = %q, want error description", authHeader)
	}
}

func TestBearerInvalidTokenChallengeOmitsScopeHint(t *testing.T) {
	middleware := oauth.Bearer(oauth.BearerConfig{
		Introspector:        &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{}},
		ResourceMetadataURL: "https://mcp.example.test/.well-known/oauth-protected-resource",
		RequiredScopes:      []string{"mcp.read", "mcp.write"},
	})

	handler := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler was called with an invalid bearer token")
	}))

	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer stale-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	authHeader := response.Header().Get("WWW-Authenticate")
	if !strings.Contains(authHeader, "resource_metadata=") {
		t.Fatalf("WWW-Authenticate = %q, want resource metadata URL", authHeader)
	}
	if strings.Contains(authHeader, `scope=`) {
		t.Fatalf("WWW-Authenticate = %q, did not want scope hint on invalid_token challenge", authHeader)
	}
	if !strings.Contains(authHeader, `error="invalid_token"`) {
		t.Fatalf("WWW-Authenticate = %q, want invalid_token error", authHeader)
	}
	if !strings.Contains(authHeader, `error_description="Missing or invalid access token"`) {
		t.Fatalf("WWW-Authenticate = %q, want error description", authHeader)
	}
}

func TestBearerAcceptsCaseInsensitiveAuthScheme(t *testing.T) {
	session := openid.NewDefaultSession()
	session.Subject = "user-123"
	session.ExpiresAt = map[fosite.TokenType]time.Time{fosite.AccessToken: time.Now().Add(time.Hour)}
	request := fosite.NewAccessRequest(session)
	request.GrantedScope = fosite.Arguments{"mcp.read"}
	request.GrantedAudience = fosite.Arguments{"https://mcp.example.test/mcp"}

	handler := oauth.Bearer(oauth.BearerConfig{
		Introspector:     &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{"valid-token": request}},
		ExpectedAudience: "https://mcp.example.test/mcp",
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "bearer valid-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

func TestBearerAcceptsValidJWT(t *testing.T) {
	session := openid.NewDefaultSession()
	session.Subject = "user-123"
	session.ExpiresAt = map[fosite.TokenType]time.Time{}
	session.ExpiresAt[fosite.AccessToken] = time.Now().Add(time.Hour)
	request := fosite.NewAccessRequest(session)
	request.GrantedScope = fosite.Arguments{"mcp.read"}
	request.GrantedAudience = fosite.Arguments{"https://mcp.example.test/mcp"}

	middleware := oauth.Bearer(oauth.BearerConfig{
		Introspector:     &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{"valid-token": request}},
		Now:              time.Now,
		ExpectedAudience: "https://mcp.example.test/mcp",
	})

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := oauth.GetUserID(r.Context()); got != "user-123" {
			t.Fatalf("GetUserID() = %q, want user-123", got)
		}
		if !oauth.GetScopes(r.Context()).Has("mcp.read") {
			t.Fatal("GetScopes() missing mcp.read")
		}
		if got := oauth.GetAuthSource(r.Context()); got != oauth.AuthSourceOAuth2 {
			t.Fatalf("GetAuthSource() = %q, want %q", got, oauth.AuthSourceOAuth2)
		}
		w.WriteHeader(http.StatusOK)
	}))

	requestHTTP := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	requestHTTP.Header.Set("Authorization", "Bearer valid-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestHTTP)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
}

func TestBearerAcceptsValidPAT(t *testing.T) {
	validator := &mockPATValidator{used: make(chan string, 1), result: &oauth.PATAuthResult{
		UserID:      "user-456",
		TokenID:     "token-789",
		ScopeType:   "workspace",
		ScopeTarget: "workspace-1",
		Scopes:      []string{"mcp.read", "mcp.write"},
	}}
	middleware := oauth.Bearer(oauth.BearerConfig{TokenValidator: validator})

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := oauth.GetUserID(r.Context()); got != "user-456" {
			t.Fatalf("GetUserID() = %q, want user-456", got)
		}
		if got := oauth.GetScopeType(r.Context()); got != "workspace" {
			t.Fatalf("GetScopeType() = %q, want workspace", got)
		}
		if got := oauth.GetScopeTarget(r.Context()); got != "workspace-1" {
			t.Fatalf("GetScopeTarget() = %q, want workspace-1", got)
		}
		if got := oauth.GetAuthSource(r.Context()); got != oauth.AuthSourcePAT {
			t.Fatalf("GetAuthSource() = %q, want %q", got, oauth.AuthSourcePAT)
		}
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer pat-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if validator.lastToken != "pat-secret" {
		t.Fatalf("ValidateAndResolve token = %q, want pat-secret", validator.lastToken)
	}
	select {
	case tokenID := <-validator.used:
		if tokenID != "token-789" {
			t.Fatalf("RecordUsage id = %q, want token-789", tokenID)
		}
	case <-time.After(time.Second):
		t.Fatal("RecordUsage was not called")
	}
}

func TestBearerRejectsInsufficientScope(t *testing.T) {
	middleware := oauth.RequireScope("mcp.write")
	handler := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler was called without required scope")
	}))

	ctx := oauth.WithScopes(context.Background(), fosite.Arguments{"mcp.read"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", nil).WithContext(ctx))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if !strings.Contains(response.Body.String(), "insufficient_scope") {
		t.Fatalf("body = %q, want insufficient_scope", response.Body.String())
	}
	authHeader := response.Header().Get("WWW-Authenticate")
	if !strings.Contains(authHeader, `scope="mcp.write"`) {
		t.Fatalf("WWW-Authenticate = %q, want required scope hint on insufficient_scope challenge", authHeader)
	}
	if !strings.Contains(authHeader, `error="insufficient_scope"`) {
		t.Fatalf("WWW-Authenticate = %q, want insufficient_scope error", authHeader)
	}
}

func TestRequireScopeForTargetRejectsPATOutsideBoundary(t *testing.T) {
	validator := &mockPATValidator{result: &oauth.PATAuthResult{
		UserID:      "user-456",
		TokenID:     "token-789",
		ScopeType:   "workspace",
		ScopeTarget: "workspace-1",
		Scopes:      []string{"mcp.write"},
	}}
	handler := oauth.Bearer(oauth.BearerConfig{TokenValidator: validator})(
		oauth.RequireScopeForTarget("mcp.write", func(*http.Request) (oauth.Target, error) {
			return oauth.Target{ScopeType: "workspace", Target: "workspace-2"}, nil
		})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("handler was called outside PAT boundary")
		})),
	)

	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer pat-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestRequireScopeForTargetAllowsPATInsideBoundary(t *testing.T) {
	validator := &mockPATValidator{result: &oauth.PATAuthResult{
		UserID:      "user-456",
		TokenID:     "token-789",
		ScopeType:   "workspace",
		ScopeTarget: "workspace-1",
		Scopes:      []string{"mcp.write"},
	}}
	handler := oauth.Bearer(oauth.BearerConfig{TokenValidator: validator})(
		oauth.RequireScopeForTarget("mcp.write", func(*http.Request) (oauth.Target, error) {
			return oauth.Target{ScopeType: "workspace", Target: "workspace-1"}, nil
		})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})),
	)

	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer pat-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

func TestRequireScopeForTargetAllowsOAuthByScope(t *testing.T) {
	session := openid.NewDefaultSession()
	session.Subject = "user-123"
	session.ExpiresAt = map[fosite.TokenType]time.Time{fosite.AccessToken: time.Now().Add(time.Hour)}
	request := fosite.NewAccessRequest(session)
	request.GrantedScope = fosite.Arguments{"mcp.write"}
	request.GrantedAudience = fosite.Arguments{"https://mcp.example.test/mcp"}
	handler := oauth.Bearer(oauth.BearerConfig{
		Introspector:     &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{"valid-token": request}},
		ExpectedAudience: "https://mcp.example.test/mcp",
	})(oauth.RequireScopeForTarget("mcp.write", func(*http.Request) (oauth.Target, error) {
		return oauth.Target{ScopeType: "workspace", Target: "workspace-1"}, nil
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

func TestRequireScopeForTargetRejectsResolverError(t *testing.T) {
	validator := &mockPATValidator{result: &oauth.PATAuthResult{
		UserID:      "user-456",
		TokenID:     "token-789",
		ScopeType:   "workspace",
		ScopeTarget: "workspace-1",
		Scopes:      []string{"mcp.write"},
	}}
	handler := oauth.Bearer(oauth.BearerConfig{TokenValidator: validator})(
		oauth.RequireScopeForTarget("mcp.write", func(*http.Request) (oauth.Target, error) {
			return oauth.Target{}, errTestResolver
		})(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler was called after resolver error")
		})),
	)

	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer pat-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestBearerAcceptsOAuthWithoutExpectedAudience(t *testing.T) {
	session := openid.NewDefaultSession()
	session.Subject = "user-123"
	session.ExpiresAt = map[fosite.TokenType]time.Time{fosite.AccessToken: time.Now().Add(time.Hour)}
	request := fosite.NewAccessRequest(session)
	request.GrantedScope = fosite.Arguments{"mcp.read"}
	handler := oauth.Bearer(oauth.BearerConfig{
		Introspector: &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{"valid-token": request}},
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

func TestRequireScopeForTargetAllowsUnscopedPAT(t *testing.T) {
	validator := &mockPATValidator{result: &oauth.PATAuthResult{
		UserID:  "user-456",
		TokenID: "token-789",
		Scopes:  []string{"mcp.write"},
	}}
	handler := oauth.Bearer(oauth.BearerConfig{TokenValidator: validator})(
		oauth.RequireScopeForTarget("mcp.write", func(*http.Request) (oauth.Target, error) {
			return oauth.Target{ScopeType: "workspace", Target: "workspace-2"}, nil
		})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})),
	)

	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer pat-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

func TestBearerDeniesMissingAuthConfigByDefault(t *testing.T) {
	handler := oauth.Bearer(oauth.BearerConfig{})(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			t.Fatal("handler was called with missing auth config")
		},
	))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestBearerAllowsWhenAuthDisabledExplicitly(t *testing.T) {
	handler := oauth.Bearer(oauth.BearerConfig{AllowUnauthenticated: true})(oauth.RequireScope("mcp.read")(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	)))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}
