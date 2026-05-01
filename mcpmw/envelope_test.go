package mcpmw

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnvelope_RewritesPlainText400AsParseError(t *testing.T) {
	t.Parallel()

	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		http.Error(w, "malformed payload: unexpected EOF", http.StatusBadRequest)
	})
	mw := Envelope(upstream)

	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":42,"method":"tools/list"}`))
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", got)
	}

	var got struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    string `json:"data"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if got.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", got.JSONRPC)
	}
	if !equalAny(got.ID, float64(42)) {
		t.Errorf("id = %v, want 42", got.ID)
	}
	if got.Error.Code != codeParseError {
		t.Errorf("code = %d, want %d", got.Error.Code, codeParseError)
	}
	if got.Error.Message != "Parse error" {
		t.Errorf("message = %q, want Parse error", got.Error.Message)
	}
	if !strings.Contains(got.Error.Data, "malformed payload") {
		t.Errorf("data = %q, want it to preserve original error", got.Error.Data)
	}
}

func TestEnvelope_RewritesMethodNotFound(t *testing.T) {
	t.Parallel()

	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		http.Error(w, "JSON RPC not handled: unknown_method", http.StatusBadRequest)
	})
	mw := Envelope(upstream)

	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":"abc","method":"unknown_method"}`))
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	var got struct {
		ID    string `json:"id"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Error.Code != codeMethodNotFound {
		t.Errorf("code = %d, want %d", got.Error.Code, codeMethodNotFound)
	}
	if got.ID != "abc" {
		t.Errorf("id = %q, want abc — string id should round-trip", got.ID)
	}
}

func TestEnvelope_PassesThroughSSE(t *testing.T) {
	t.Parallel()

	const sseBody = "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n"

	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, sseBody)
	})
	mw := Envelope(upstream)

	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if rr.Body.String() != sseBody {
		t.Errorf("SSE body was modified by middleware:\n got: %q\nwant: %q", rr.Body.String(), sseBody)
	}
}

func TestEnvelope_PassesThrough401WithWWWAuthenticate(t *testing.T) {
	t.Parallel()

	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		w.Header().Set("Content-Type", "text/plain")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	mw := Envelope(upstream)

	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (auth boundary must survive)", rr.Code)
	}
	if got := rr.Header().Get("WWW-Authenticate"); got == "" {
		t.Errorf("WWW-Authenticate header was stripped — OAuth discovery will break")
	}
}

func TestEnvelope_PassesThroughGET(t *testing.T) {
	t.Parallel()

	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		http.Error(w, "malformed payload: GET should not rewrite", http.StatusBadRequest)
	})
	mw := Envelope(upstream)

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (GET must not be rewritten)", rr.Code)
	}
}

func TestEnvelope_NullIDForMissingOrInvalidJSON(t *testing.T) {
	t.Parallel()

	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		http.Error(w, "malformed payload: not json", http.StatusBadRequest)
	})
	mw := Envelope(upstream)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("not json at all"))
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	var got struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got.ID) != "null" {
		t.Errorf("id = %s, want null", string(got.ID))
	}
}

func equalAny(a, b any) bool {
	return a == b
}
