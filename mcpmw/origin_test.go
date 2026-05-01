package mcpmw

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOrigin_NoOriginHeaderAllows(t *testing.T) {
	t.Parallel()

	called := false
	mw := Origin(OriginConfig{Allowed: []string{"https://app.example.com"}},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if !called {
		t.Errorf("non-browser client (no Origin) should pass through")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestOrigin_AllowedOriginPasses(t *testing.T) {
	t.Parallel()

	called := false
	mw := Origin(OriginConfig{Allowed: []string{"https://app.example.com"}},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if !called {
		t.Errorf("allowed origin should pass through")
	}
}

func TestOrigin_DisallowedOriginRejected(t *testing.T) {
	t.Parallel()

	called := false
	mw := Origin(OriginConfig{Allowed: []string{"https://app.example.com"}},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if called {
		t.Errorf("disallowed origin should not reach handler")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestOrigin_LoopbackAllowedWhenEnabled(t *testing.T) {
	t.Parallel()

	tests := []string{
		"http://127.0.0.1:8080",
		"http://localhost:3000",
		"http://[::1]:9000",
	}
	for _, origin := range tests {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()

			called := false
			mw := Origin(OriginConfig{AllowLoopback: true},
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			req.Header.Set("Origin", origin)
			rr := httptest.NewRecorder()
			mw.ServeHTTP(rr, req)

			if !called {
				t.Errorf("loopback origin %q should pass when AllowLoopback=true", origin)
			}
		})
	}
}

func TestOrigin_LoopbackRejectedWhenDisabled(t *testing.T) {
	t.Parallel()

	mw := Origin(OriginConfig{AllowLoopback: false},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("handler should not be called")
		}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestOrigin_OnDenyOverridesDefault(t *testing.T) {
	t.Parallel()

	gotOrigin := ""
	cfg := OriginConfig{
		Allowed: []string{"https://app.example.com"},
		OnDeny: func(w http.ResponseWriter, _ *http.Request, origin string) {
			gotOrigin = origin
			w.WriteHeader(http.StatusTeapot) // distinctive — proves we ran instead of default
		},
	}
	mw := Origin(cfg, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Errorf("OnDeny should have written its own response (got status %d)", rr.Code)
	}
	if gotOrigin != "https://evil.example.com" {
		t.Errorf("OnDeny got origin %q, want %q", gotOrigin, "https://evil.example.com")
	}
}
