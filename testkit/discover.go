package testkit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/haakco/mcp-kit/mcpkit"
)

// Wire is a raw HTTP response from an MCP endpoint. Tests use it to assert the
// transport contract — status, headers, and body — directly.
type Wire struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// JSON returns the JSON-RPC payload carried by the response.
//
// The Streamable HTTP transport frames responses as a single SSE message by
// default, so a byte-wise comparison against the body is a transport assertion
// rather than a payload one. JSON unwraps that framing and is a no-op for
// application/json responses.
func (w Wire) JSON() []byte {
	if !strings.HasPrefix(w.Header.Get("Content-Type"), "text/event-stream") {
		return w.Body
	}
	var data []string
	for _, line := range strings.Split(string(w.Body), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimSpace(value))
		}
	}
	return []byte(strings.Join(data, "\n"))
}

// Post sends one MCP 2026-07-28 JSON-RPC request to mcpURL and returns the raw
// response. It does not assert on the status, so callers can check failures.
//
// The request carries the headers and per-request metadata that MCP revision
// 2026-07-28 requires. Pass a nil params map for methods that take none.
func Post(t testing.TB, mcpURL, token, method string, params map[string]any) Wire {
	t.Helper()
	return PostHeaders(t, mcpURL, token, method, params, nil)
}

// PostHeaders is Post with extra request headers, so tests can drive mismatch
// and unsupported-version cases.
func PostHeaders(t testing.TB, mcpURL, token, method string, params map[string]any, extra map[string]string) Wire {
	t.Helper()

	if params == nil {
		params = map[string]any{}
	}
	if _, ok := params["_meta"]; !ok {
		params["_meta"] = map[string]any{
			"io.modelcontextprotocol/protocolVersion":    mcpkit.ProtocolVersion,
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		}
	}

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		t.Fatalf("marshal %s request: %v", method, err)
	}

	headers := map[string]string{
		"Content-Type":         "application/json",
		"Mcp-Protocol-Version": mcpkit.ProtocolVersion,
		"Mcp-Method":           method,
	}
	for name, value := range extra {
		headers[name] = value
	}
	return send(t, mcpURL, token, http.MethodPost, bytes.NewReader(body), headers)
}

// Do sends a bodiless request so tests can assert how the transport answers
// methods that carry no JSON-RPC message, such as GET and DELETE in stateless
// mode.
func Do(t testing.TB, mcpURL, token, method string) Wire {
	t.Helper()
	return send(t, mcpURL, token, method, nil, nil)
}

func send(t testing.TB, mcpURL, token, method string, body io.Reader, headers map[string]string) Wire {
	t.Helper()

	req, err := http.NewRequest(method, mcpURL, body)
	if err != nil {
		t.Fatalf("build %s request: %v", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Accept", "application/json, text/event-stream")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, mcpURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s response: %v", method, err)
	}
	return Wire{StatusCode: resp.StatusCode, Header: resp.Header, Body: bytes.TrimSpace(respBody)}
}

// Discover performs server/discover against a testkit server and returns the
// decoded result. MCP 2026-07-28 has no handshake, so this is the first request
// a client makes.
func Discover(t testing.TB, server *Server, token string) *mcp.DiscoverResult {
	t.Helper()
	return DiscoverURL(t, server.MCPURL, token)
}

// DiscoverURL performs server/discover at mcpURL and returns the decoded result.
func DiscoverURL(t testing.TB, mcpURL, token string) *mcp.DiscoverResult {
	t.Helper()
	var result mcp.DiscoverResult
	decodeResult(t, Post(t, mcpURL, token, "server/discover", nil), "server/discover", &result)
	return &result
}

// ListTools performs tools/list against a testkit server and returns tool names.
func ListTools(t testing.TB, server *Server, token string) []string {
	t.Helper()
	return ListToolsURL(t, server.MCPURL, token)
}

// ListToolsURL performs tools/list at mcpURL and returns tool names.
func ListToolsURL(t testing.TB, mcpURL, token string) []string {
	t.Helper()
	var result mcp.ListToolsResult
	decodeResult(t, Post(t, mcpURL, token, "tools/list", nil), "tools/list", &result)
	tools := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		tools = append(tools, tool.Name)
	}
	return tools
}

// decodeResult asserts a successful JSON-RPC response and decodes its result.
func decodeResult(t testing.TB, wire Wire, method string, out any) {
	t.Helper()
	body := wire.JSON()
	if wire.StatusCode/100 != 2 {
		t.Fatalf("%s: status %d, body %s", method, wire.StatusCode, body)
	}
	var payload struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("%s: decode response %s: %v", method, body, err)
	}
	if payload.Error != nil {
		t.Fatalf("%s: JSON-RPC error %d: %s", method, payload.Error.Code, payload.Error.Message)
	}
	if len(payload.Result) == 0 {
		t.Fatalf("%s: response has no result: %s", method, body)
	}
	if err := json.Unmarshal(payload.Result, out); err != nil {
		t.Fatalf("%s: decode result %s: %v", method, payload.Result, err)
	}
}
