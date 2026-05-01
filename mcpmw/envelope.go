// Package mcpmw provides MCP-specific HTTP middleware: JSON-RPC envelope
// rewriting, Origin allowlist, and (in later versions) audit emitter wrapping.
//
// Each middleware is a standard `func(http.Handler) http.Handler` so it
// composes with any HTTP framework that can wrap handlers.
package mcpmw

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"strings"
)

// JSON-RPC 2.0 protocol error codes used when rewriting SDK plain-text errors.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
)

// Envelope rewrites plain-text HTTP error responses on the MCP transport
// into canonical JSON-RPC 2.0 error envelopes.
//
// The Streamable HTTP handler from the upstream Go MCP SDK uses http.Error for
// protocol-level rejections (parse errors, unknown methods, missing params).
// Those escape the SDK as text/plain bodies that strict MCP clients
// (Inspector, Claude Code) misreport as transport-level failures rather than
// the actual JSON-RPC -326xx errors they really are.
//
// Behaviour:
//   - Only intercepts POST. Other methods pass through untouched.
//   - Buffers the response in memory until the first byte (or end of handler).
//     If the SDK starts streaming an SSE response (Content-Type:
//     text/event-stream) or returns a non-error status (< 400), the buffered
//     state is flushed verbatim and the writer falls back to pass-through —
//     so successful streaming responses are unaffected.
//   - For 4xx text/plain responses with a known SDK error prefix, rewrites
//     them as a canonical JSON-RPC envelope with HTTP 200.
//   - 401 / 403 / 404 / 415 / other transport-level codes pass through
//     untouched so WWW-Authenticate, Origin enforcement, etc. survive.
//
// The original SDK error message is preserved in the envelope's `error.data`
// field so callers can debug. Per MCP transport guidance, JSON-RPC error
// responses use HTTP 200; the JSON `error` object is the truth.
//
// Originally extracted from vorrent/internal/mcpserver/jsonrpc_envelope.go.
func Envelope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope(w, r, next)
	})
}

func envelope(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if r.Method != http.MethodPost {
		next.ServeHTTP(w, r)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		next.ServeHTTP(w, r)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	brw := &bufferedResponseWriter{underlying: w}
	next.ServeHTTP(brw, r)

	if !shouldRewrite(brw) {
		brw.flush()
		return
	}

	rawMessage := strings.TrimSpace(brw.body.String())
	code, msg, ok := mapSDKError(rawMessage)
	if !ok {
		brw.flush()
		return
	}

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      extractID(body),
		"error": map[string]any{
			"code":    code,
			"message": msg,
			"data":    rawMessage,
		},
	})
	if err != nil {
		brw.flush()
		return
	}

	h := w.Header()
	h.Del("Content-Type")
	h.Del("Content-Length")
	h.Del("X-Content-Type-Options")
	h.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

// shouldRewrite returns true only for HTTP 400 + text/plain — the SDK uses 400
// for parse / method / params protocol errors. Other 4xx codes (401, 403, 404,
// 405, 409, 415) carry transport semantics clients depend on and must survive
// untouched.
func shouldRewrite(brw *bufferedResponseWriter) bool {
	if brw.passthrough {
		return false
	}
	if brw.statusCode != http.StatusBadRequest {
		return false
	}
	return strings.HasPrefix(brw.header.Get("Content-Type"), "text/plain")
}

func mapSDKError(text string) (int, string, bool) {
	switch {
	case strings.HasPrefix(text, "malformed payload:"):
		return codeParseError, "Parse error", true
	case strings.Contains(text, "JSON RPC not handled"):
		return codeMethodNotFound, "Method not found", true
	case strings.Contains(text, "missing required"),
		strings.Contains(text, "missing id"),
		strings.Contains(text, "unexpected id"):
		return codeInvalidRequest, "Invalid request", true
	default:
		return 0, "", false
	}
}

func extractID(body []byte) json.RawMessage {
	var probe struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || len(probe.ID) == 0 {
		return json.RawMessage("null")
	}
	return probe.ID
}

type bufferedResponseWriter struct {
	underlying  http.ResponseWriter
	header      http.Header
	statusCode  int
	body        bytes.Buffer
	passthrough bool
}

func (b *bufferedResponseWriter) Header() http.Header {
	if b.passthrough {
		return b.underlying.Header()
	}
	if b.header == nil {
		b.header = make(http.Header)
	}
	return b.header
}

func (b *bufferedResponseWriter) WriteHeader(statusCode int) {
	if b.passthrough {
		b.underlying.WriteHeader(statusCode)
		return
	}
	if b.statusCode != 0 {
		return
	}
	b.statusCode = statusCode

	ct := b.header.Get("Content-Type")
	if statusCode < http.StatusBadRequest || strings.HasPrefix(ct, "text/event-stream") {
		b.flush()
	}
}

func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	if b.passthrough {
		return b.underlying.Write(p)
	}
	if b.statusCode == 0 {
		b.WriteHeader(http.StatusOK)
		if b.passthrough {
			return b.underlying.Write(p)
		}
	}
	return b.body.Write(p)
}

func (b *bufferedResponseWriter) Flush() {
	if b.passthrough {
		if f, ok := b.underlying.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func (b *bufferedResponseWriter) flush() {
	if b.passthrough {
		return
	}
	b.passthrough = true
	maps.Copy(b.underlying.Header(), b.header)
	if b.statusCode != 0 {
		b.underlying.WriteHeader(b.statusCode)
	}
	if b.body.Len() > 0 {
		_, _ = b.underlying.Write(b.body.Bytes())
		b.body.Reset()
	}
}
