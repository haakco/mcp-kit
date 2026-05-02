package consent_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ory/fosite"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/consent"
	"github.com/haakco/mcp-kit/oauth/keys"
	"github.com/haakco/mcp-kit/oauth/storage"
)

func TestHandlerGETRendersLoginPage(t *testing.T) {
	renderer := &capturingRenderer{}
	handler := newTestHandler(t, testHandlerConfig{renderer: renderer})

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+authorizeValues().Encode(), nil))

	if renderer.lastPage != consent.PageLogin {
		t.Fatalf("page = %v, want PageLogin", renderer.lastPage)
	}
	if got := hiddenInputValue(renderer.lastData.HiddenInputs, "client_id"); got != "client-id" {
		t.Fatalf("hidden client_id = %q", got)
	}
}

func TestHandlerPOSTLoginBadPasswordRendersLogin(t *testing.T) {
	renderer := &capturingRenderer{}
	handler := newTestHandler(t, testHandlerConfig{
		renderer: renderer,
		authenticator: consent.AuthenticatorFunc(func(context.Context, string, string) (oauth.Subject, error) {
			return oauth.Subject{}, errors.New("invalid")
		}),
	})

	form := authorizeValues()
	form.Set("action", "login")
	form.Set("username", "alice@example.com")
	form.Set("password", "wrong")
	handler.ServeHTTP(httptest.NewRecorder(), formRequest(form))

	if renderer.lastPage != consent.PageLogin || renderer.lastData.Error == "" {
		t.Fatalf("page/error = %v/%q, want login error", renderer.lastPage, renderer.lastData.Error)
	}
	if got := hiddenInputValue(renderer.lastData.HiddenInputs, "approval_token"); got != "" {
		t.Fatalf("approval token minted on bad password: %q", got)
	}
}

func TestHandlerPOSTLoginGoodPasswordRendersConsent(t *testing.T) {
	renderer := &capturingRenderer{}
	handler := newTestHandler(t, testHandlerConfig{renderer: renderer})

	token := loginAndApprovalToken(t, handler, renderer)

	if token == "" {
		t.Fatal("approval token hidden input is empty")
	}
	if renderer.lastPage != consent.PageConsent {
		t.Fatalf("page = %v, want PageConsent", renderer.lastPage)
	}
}

func TestHandlerPOSTApproveGoodTokenRedirectsWithCode(t *testing.T) {
	renderer := &capturingRenderer{}
	handler := newTestHandler(t, testHandlerConfig{renderer: renderer})
	token := loginAndApprovalToken(t, handler, renderer)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, approveRequest(token))

	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body=%s", response.Code, response.Body.String())
	}
	location := response.Header().Get("Location")
	if code := callbackCode(t, location); code == "" {
		t.Fatalf("redirect missing code: %s", location)
	}
}

func TestHandlerPOSTApproveExpiredTokenRendersLogin(t *testing.T) {
	now := testNow()
	renderer := &capturingRenderer{}
	handler := newTestHandler(t, testHandlerConfig{renderer: renderer, now: &now})
	token := loginAndApprovalToken(t, handler, renderer)
	now.Add(consent.ApprovalTokenTTL())
	now.Add(time.Second)

	handler.ServeHTTP(httptest.NewRecorder(), approveRequest(token))

	if renderer.lastPage != consent.PageLogin || !strings.Contains(renderer.lastData.Error, "expired") {
		t.Fatalf("page/error = %v/%q, want expired login", renderer.lastPage, renderer.lastData.Error)
	}
}

func TestHandlerPOSTDenyEmitsAccessDenied(t *testing.T) {
	emitter := &recordingEmitter{}
	handler := newTestHandler(t, testHandlerConfig{auditEmitter: emitter})
	form := authorizeValues()
	form.Set("action", "deny")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, formRequest(form))

	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want redirect with access_denied", response.Code)
	}
	if !strings.Contains(response.Header().Get("Location"), "error=access_denied") {
		t.Fatalf("location = %q, want access_denied", response.Header().Get("Location"))
	}
	if got := emitter.lastAction(); got != consent.ActionConsentDenied {
		t.Fatalf("audit action = %q", got)
	}
}

func TestHandlerPOSTApprovalReplayRejected(t *testing.T) {
	renderer := &capturingRenderer{}
	handler := newTestHandler(t, testHandlerConfig{renderer: renderer})
	token := loginAndApprovalToken(t, handler, renderer)

	handler.ServeHTTP(httptest.NewRecorder(), approveRequest(token))
	handler.ServeHTTP(httptest.NewRecorder(), approveRequest(token))

	if renderer.lastPage != consent.PageLogin || renderer.lastData.Error == "" {
		t.Fatalf("replay page/error = %v/%q, want login error", renderer.lastPage, renderer.lastData.Error)
	}
}

func TestHandlerRFC8707RejectsMismatchedResource(t *testing.T) {
	handler := newTestHandler(t, testHandlerConfig{})
	form := authorizeValues()
	form.Set("resource", "https://evil.example.com/mcp")
	form.Set("action", "login")
	form.Set("username", "alice@example.com")
	form.Set("password", "password")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, formRequest(form))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if !strings.Contains(response.Body.String(), "current MCP resource") {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestHandlerAuditEventsApprovedAndDenied(t *testing.T) {
	emitter := &recordingEmitter{}
	renderer := &capturingRenderer{}
	handler := newTestHandler(t, testHandlerConfig{renderer: renderer, auditEmitter: emitter})
	token := loginAndApprovalToken(t, handler, renderer)

	handler.ServeHTTP(httptest.NewRecorder(), approveRequest(token))
	form := authorizeValues()
	form.Set("action", "deny")
	handler.ServeHTTP(httptest.NewRecorder(), formRequest(form))

	if !emitter.hasAction(consent.ActionConsentApproved) {
		t.Fatal("missing approved audit event")
	}
	if !emitter.hasAction(consent.ActionConsentDenied) {
		t.Fatal("missing denied audit event")
	}
	if clientID := emitter.events[0].ClientID; clientID != "client-id" {
		t.Fatalf("client_id = %q", clientID)
	}
}

func TestHandlerRejectsShortApprovalSecret(t *testing.T) {
	_, err := consent.NewHandler(consent.Config{
		Provider:       testProvider(t, storage.NewMemoryStore()),
		Authenticator:  staticAuthenticator(),
		Renderer:       consent.RendererFunc(func(http.ResponseWriter, consent.Page, consent.PageData) {}),
		PublicURL:      "https://app.example.com",
		ApprovalSecret: []byte{0x01},
	})
	if err == nil {
		t.Fatal("expected NewHandler to reject 1-byte ApprovalSecret")
	}
	if !strings.Contains(err.Error(), "exactly 32 bytes") {
		t.Fatalf("error does not mention 32-byte requirement: %v", err)
	}
}

func TestHandlerConsentPolicyAllowsSkip(t *testing.T) {
	renderer := &capturingRenderer{}
	handler := newTestHandler(t, testHandlerConfig{renderer: renderer, policy: skipPolicy{}})
	form := authorizeValues()
	form.Set("action", "login")
	form.Set("username", "alice@example.com")
	form.Set("password", "password")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, formRequest(form))

	if renderer.seen(consent.PageConsent) {
		t.Fatal("PageConsent rendered despite skip policy")
	}
	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", response.Code)
	}
}

type testHandlerConfig struct {
	renderer      *capturingRenderer
	authenticator consent.Authenticator
	auditEmitter  audit.Emitter
	policy        consent.ConsentPolicy
	now           *testClock
}

func newTestHandler(t *testing.T, cfg testHandlerConfig) *consent.Handler {
	t.Helper()
	store := storage.NewMemoryStore()
	registerTestClient(t, store)
	renderer := cfg.renderer
	if renderer == nil {
		renderer = &capturingRenderer{}
	}
	authenticator := cfg.authenticator
	if authenticator == nil {
		authenticator = staticAuthenticator()
	}
	defaultNow := testNow()
	nowFunc := defaultNow.Now
	if cfg.now != nil {
		nowFunc = cfg.now.Now
	}
	handler, err := consent.NewHandler(consent.Config{
		Provider:       testProvider(t, store),
		Authenticator:  authenticator,
		Renderer:       renderer,
		PublicURL:      "https://app.example.com",
		ApprovalSecret: []byte("approval-secret-32-byte-value!!!"),
		ResourceURL:    consent.StaticResourceURL("https://app.example.com/mcp"),
		AuditEmitter:   cfg.auditEmitter,
		ConsentPolicy:  cfg.policy,
		Now:            nowFunc,
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

type testClock struct{ v int64 }

func testNow() testClock { return testClock{} }

func (c *testClock) Now() time.Time {
	return time.Unix(c.v, 0).UTC()
}

func (c *testClock) Add(d time.Duration) {
	c.v += int64(d.Seconds())
}

func testProvider(t testing.TB, store *storage.MemoryStore) *oauth.Provider {
	t.Helper()
	manager := keys.NewManager(keys.NewMemoryStore())
	if _, err := manager.EnsureSigningKey(context.Background()); err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}
	provider, err := oauth.New(oauth.Config{
		Issuer:        "https://app.example.com",
		Secret:        []byte("test-secret-must-be-32-bytes!!!!"),
		Store:         store,
		KeyManager:    manager,
		AllowedScopes: []string{"openid", "mcp.read", "offline_access"},
		DefaultScopes: []string{"openid", "mcp.read"},
	})
	if err != nil {
		t.Fatalf("oauth.New() error = %v", err)
	}
	return provider
}

func registerTestClient(t testing.TB, store *storage.MemoryStore) {
	t.Helper()
	if err := store.SaveClient(context.Background(), storage.Client{
		ID:            "client-id",
		Name:          "Test Client",
		RedirectURIs:  []string{"http://127.0.0.1/callback"},
		GrantTypes:    []string{"authorization_code", "refresh_token"},
		ResponseTypes: []string{"code"},
		Scopes:        []string{"openid", "mcp.read", "offline_access"},
		Audience:      []string{"https://app.example.com/mcp"},
		IsPublic:      true,
	}); err != nil {
		t.Fatalf("SaveClient() error = %v", err)
	}
}

func staticAuthenticator() consent.Authenticator {
	return consent.AuthenticatorFunc(func(_ context.Context, username string, password string) (oauth.Subject, error) {
		if username != "alice@example.com" || password != "password" {
			return oauth.Subject{}, errors.New("invalid")
		}
		return oauth.Subject{ID: uuid.NewString(), Email: username}, nil
	})
}

func authorizeValues() url.Values {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {"client-id"},
		"redirect_uri":          {"http://127.0.0.1/callback"},
		"scope":                 {"openid mcp.read"},
		"state":                 {"state-123456"},
		"code_challenge":        {s256Challenge("verifier-123456789012345678901234567890")},
		"code_challenge_method": {"S256"},
		"resource":              {"https://app.example.com/mcp"},
	}
}

func formRequest(values url.Values) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return request
}

func approveRequest(token string) *http.Request {
	form := authorizeValues()
	form.Set("action", "approve")
	form.Set("approval_token", token)
	return formRequest(form)
}

func loginAndApprovalToken(t *testing.T, handler *consent.Handler, renderer *capturingRenderer) string {
	t.Helper()
	form := authorizeValues()
	form.Set("action", "login")
	form.Set("username", "alice@example.com")
	form.Set("password", "password")
	handler.ServeHTTP(httptest.NewRecorder(), formRequest(form))
	return hiddenInputValue(renderer.lastData.HiddenInputs, "approval_token")
}

func hiddenInputValue(inputs []consent.HiddenInput, name string) string {
	for _, input := range inputs {
		if input.Name == name {
			return input.Value
		}
	}
	return ""
}

func callbackCode(t *testing.T, location string) string {
	t.Helper()
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect location: %v", err)
	}
	return parsed.Query().Get("code")
}

func s256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type capturingRenderer struct {
	mu       sync.Mutex
	lastPage consent.Page
	lastData consent.PageData
	pages    []consent.Page
}

func (r *capturingRenderer) Render(w http.ResponseWriter, page consent.Page, data consent.PageData) {
	r.mu.Lock()
	r.lastPage = page
	r.lastData = data
	r.pages = append(r.pages, page)
	r.mu.Unlock()
	if page != consent.PageRedirectBridge {
		w.WriteHeader(http.StatusOK)
	}
}

func (r *capturingRenderer) seen(page consent.Page) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, got := range r.pages {
		if got == page {
			return true
		}
	}
	return false
}

type recordingEmitter struct {
	mu     sync.Mutex
	events []audit.Event
}

func (e *recordingEmitter) Emit(_ context.Context, event audit.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
	return nil
}

func (e *recordingEmitter) lastAction() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.events) == 0 {
		return ""
	}
	return e.events[len(e.events)-1].Action
}

func (e *recordingEmitter) hasAction(action string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, event := range e.events {
		if event.Action == action {
			return true
		}
	}
	return false
}

type skipPolicy struct{}

func (skipPolicy) AllowsSkip(context.Context, fosite.Client, oauth.Subject, []string) bool {
	return true
}
func (skipPolicy) ValidateScopes(context.Context, oauth.Subject, []string) error { return nil }
