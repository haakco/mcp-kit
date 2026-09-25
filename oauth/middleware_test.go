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
// TestBearerChallengeShapePerRejectionReason pins the RFC 6750 challenge each
// rejection path emits. Clients parse these headers by splitting on commas and
// unescaping quoted strings, so the shape is part of the contract.
func TestBearerChallengeShapePerRejectionReason(t *testing.T) {
	t.Parallel()

	const metadataURL = "https://mcp.example.test/.well-known/oauth-protected-resource/mcp"

	expiredSession := openid.NewDefaultSession()
	expiredSession.Subject = "user-expired"
	expiredSession.ExpiresAt = map[fosite.TokenType]time.Time{}
	expiredSession.ExpiresAt[fosite.AccessToken] = time.Now().Add(-time.Hour)
	expiredRequest := fosite.NewAccessRequest(expiredSession)
	expiredRequest.GrantedAudience = fosite.Arguments{"https://mcp.example.test/mcp"}

	wrongAudienceSession := openid.NewDefaultSession()
	wrongAudienceSession.Subject = "user-wrong-aud"
	wrongAudienceSession.ExpiresAt = map[fosite.TokenType]time.Time{}
	wrongAudienceSession.ExpiresAt[fosite.AccessToken] = time.Now().Add(time.Hour)
	wrongAudienceRequest := fosite.NewAccessRequest(wrongAudienceSession)
	wrongAudienceRequest.GrantedAudience = fosite.Arguments{"https://other.example.test/mcp"}

	tests := []struct {
		name          string
		cfg           oauth.BearerConfig
		withScope     string
		token         string
		contextScopes fosite.Arguments
		wantCode      int
		wantError     string
		wantScopeHint string
	}{
		{
			name: "missing token",
			cfg: oauth.BearerConfig{
				Introspector:        &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{}},
				ResourceMetadataURL: metadataURL,
			},
			wantCode:  http.StatusUnauthorized,
			wantError: "invalid_token",
		},
		{
			name: "invalid token",
			cfg: oauth.BearerConfig{
				Introspector:        &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{}},
				ResourceMetadataURL: metadataURL,
			},
			token:     "not-a-token",
			wantCode:  http.StatusUnauthorized,
			wantError: "invalid_token",
		},
		{
			name: "expired token",
			cfg: oauth.BearerConfig{
				Introspector:        &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{"expired": expiredRequest}},
				ResourceMetadataURL: metadataURL,
				Now:                 time.Now,
			},
			token:     "expired",
			wantCode:  http.StatusUnauthorized,
			wantError: "invalid_token",
		},
		{
			name: "wrong audience",
			cfg: oauth.BearerConfig{
				Introspector:        &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{"wrong-aud": wrongAudienceRequest}},
				ResourceMetadataURL: metadataURL,
				Now:                 time.Now,
				ExpectedAudience:    "https://mcp.example.test/mcp",
			},
			token:     "wrong-aud",
			wantCode:  http.StatusUnauthorized,
			wantError: "invalid_token",
		},
		{
			name:          "authenticated without any scope",
			cfg:           oauth.BearerConfig{AllowUnauthenticated: true},
			withScope:     "mcp.write",
			wantCode:      http.StatusUnauthorized,
			wantError:     "invalid_token",
			wantScopeHint: "",
		},
		{
			name:          "insufficient scope",
			cfg:           oauth.BearerConfig{AllowUnauthenticated: true},
			withScope:     "mcp.write",
			contextScopes: fosite.Arguments{"mcp.read"},
			wantCode:      http.StatusForbidden,
			wantError:     "insufficient_scope",
			wantScopeHint: "mcp.write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := oauth.Bearer(tt.cfg)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("handler was called for a rejected request")
			}))
			if tt.withScope != "" {
				handler = oauth.RequireScope(tt.withScope)(handler)
			}

			request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			if len(tt.contextScopes) > 0 {
				request = request.WithContext(oauth.WithScopes(request.Context(), tt.contextScopes))
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; challenge=%q", response.Code, tt.wantCode, response.Header().Get("WWW-Authenticate"))
			}
			challenge := response.Header().Get("WWW-Authenticate")
			if !strings.HasPrefix(challenge, `Bearer realm="OAuth"`) {
				t.Fatalf("WWW-Authenticate = %q, want a Bearer realm=OAuth challenge", challenge)
			}
			if !strings.Contains(challenge, `error="`+tt.wantError+`"`) {
				t.Fatalf("WWW-Authenticate = %q, want error=%s", challenge, tt.wantError)
			}
			if tt.cfg.ResourceMetadataURL != "" && !strings.Contains(challenge, "resource_metadata=") {
				t.Fatalf("WWW-Authenticate = %q, want resource_metadata so clients can recover", challenge)
			}
			hasScopeHint := strings.Contains(challenge, `scope="`)
			if tt.wantScopeHint == "" && hasScopeHint {
				t.Fatalf("WWW-Authenticate = %q, want no scope hint on invalid_token", challenge)
			}
			if tt.wantScopeHint != "" && !strings.Contains(challenge, `scope="`+tt.wantScopeHint+`"`) {
				t.Fatalf("WWW-Authenticate = %q, want scope=%q on insufficient_scope", challenge, tt.wantScopeHint)
			}
		})
	}
}

func TestBearerChallengeEscapesQuotedAuthParams(t *testing.T) {
	t.Parallel()

	// A quote, comma, and backslash in the metadata URL must not be able to
	// terminate the quoted string or start a new auth-param.
	const hostileURL = `https://mcp.example.test/a,b"c\d`

	middleware := oauth.Bearer(oauth.BearerConfig{
		Introspector:        &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{}},
		ResourceMetadataURL: hostileURL,
	})
	handler := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler was called without a token")
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	challenge := response.Header().Get("WWW-Authenticate")
	if !strings.Contains(challenge, `resource_metadata="`+`https://mcp.example.test/a,b\"c\\d`+`"`) {
		t.Fatalf("WWW-Authenticate = %q, want the metadata URL quoted with quotes and backslashes escaped", challenge)
	}
}
