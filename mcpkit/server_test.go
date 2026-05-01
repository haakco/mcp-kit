package mcpkit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haakco/mcp-kit/mcpkit"
	"github.com/haakco/mcp-kit/oauth"
)

func TestNewComposesOriginBearerAndEnvelope(t *testing.T) {
	server, err := mcpkit.New(mcpkit.Config{
		Handler:        sdkBadRequestHandler(),
		AllowedOrigins: []string{"https://app.example.test"},
		Bearer: mcpkit.BearerConfig{
			TokenValidator: fakeTokenValidator{},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1}`))
	request.Header.Set("Origin", "https://app.example.test")
	request.Header.Set("Authorization", "Bearer valid-pat")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"jsonrpc":"2.0"`) {
		t.Fatalf("body = %q, want JSON-RPC envelope", response.Body.String())
	}
}

func TestNewRejectsOriginBeforeAuth(t *testing.T) {
	validator := &recordingTokenValidator{}
	server, err := mcpkit.New(mcpkit.Config{
		Handler:        http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("inner handler called") }),
		AllowedOrigins: []string{"https://app.example.test"},
		Bearer: mcpkit.BearerConfig{
			TokenValidator: validator,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Origin", "https://evil.example.test")
	request.Header.Set("Authorization", "Bearer valid-pat")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if validator.called {
		t.Fatal("token validator was called, want origin to reject before auth")
	}
}

func TestNewRejectsMissingBearerBeforeEnvelope(t *testing.T) {
	innerCalled := false
	server, err := mcpkit.New(mcpkit.Config{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			innerCalled = true
			http.Error(w, "malformed payload: missing required field", http.StatusBadRequest)
		}),
		Bearer: mcpkit.BearerConfig{
			TokenValidator: fakeTokenValidator{},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if !strings.Contains(response.Header().Get("WWW-Authenticate"), "Bearer") {
		t.Fatalf("WWW-Authenticate = %q, want Bearer", response.Header().Get("WWW-Authenticate"))
	}
	if innerCalled {
		t.Fatal("inner handler was called, want bearer to reject before envelope/SDK handler")
	}
	if strings.Contains(response.Body.String(), `"jsonrpc"`) {
		t.Fatalf("body = %q, want bearer JSON not JSON-RPC envelope", response.Body.String())
	}
}

func sdkBadRequestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "malformed payload: missing required field", http.StatusBadRequest)
	})
}

type fakeTokenValidator struct{}

func (fakeTokenValidator) ValidateAndResolve(context.Context, string) (*oauth.PATAuthResult, error) {
	return &oauth.PATAuthResult{
		UserID:  "user-1",
		TokenID: "token-1",
		Scopes:  []string{"mcp.read"},
	}, nil
}

func (fakeTokenValidator) RecordUsage(context.Context, string) {}

type recordingTokenValidator struct {
	called bool
}

func (v *recordingTokenValidator) ValidateAndResolve(context.Context, string) (*oauth.PATAuthResult, error) {
	v.called = true
	return (&fakeTokenValidator{}).ValidateAndResolve(context.Background(), "")
}

func (*recordingTokenValidator) RecordUsage(context.Context, string) {}
