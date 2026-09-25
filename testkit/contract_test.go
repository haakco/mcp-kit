package testkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/haakco/mcp-kit/mcpkit"
	"github.com/haakco/mcp-kit/testkit"
)

// The tests in this file pin the MCP 2026-07-28 transport and result contract
// as the kit composes it: the SDK's Streamable HTTP handler, stateless, behind
// the kit's Origin → Bearer → Envelope middleware. They assert the wire, not
// kit internals, because that is what clients and the conformance suite see.

func TestStatelessTransportServesPOSTAndRejectsOtherMethods(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")

	post := testkit.Post(t, server.MCPURL, token, "tools/list", nil)
	if post.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, want 200; body=%s", post.StatusCode, post.Body)
	}

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		wire := testkit.Do(t, server.MCPURL, token, method)
		if wire.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want 405", method, wire.StatusCode)
		}
		if allow := wire.Header.Get("Allow"); allow != http.MethodPost {
			t.Fatalf("%s Allow = %q, want POST", method, allow)
		}
	}
}

func TestStatelessTransportNeitherRequiresNorEmitsSessionID(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")

	wire := testkit.Post(t, server.MCPURL, token, "tools/list", nil)
	if sessionID := wire.Header.Get("Mcp-Session-Id"); sessionID != "" {
		t.Fatalf("Mcp-Session-Id = %q, want none in stateless mode", sessionID)
	}

	// A client that still sends a session header must be served on its merits,
	// not resumed into server state. Two calls with different bogus session IDs
	// must both succeed and be identical.
	first := testkit.PostHeaders(t, server.MCPURL, token, "tools/list", nil, map[string]string{"Mcp-Session-Id": "stale-session-a"})
	second := testkit.PostHeaders(t, server.MCPURL, token, "tools/list", nil, map[string]string{"Mcp-Session-Id": "stale-session-b"})
	if first.StatusCode != http.StatusOK || second.StatusCode != http.StatusOK {
		t.Fatalf("stale session statuses = %d, %d; want 200, 200", first.StatusCode, second.StatusCode)
	}
	if !bytes.Equal(first.JSON(), second.JSON()) {
		t.Fatalf("stale session changed the result:\n%s\n%s", first.JSON(), second.JSON())
	}
}

func TestHeaderAndMetadataViolationsReturnProtocolErrors(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")

	tests := []struct {
		name       string
		method     string
		params     map[string]any
		headers    map[string]string
		wantCode   int
		wantStatus int
	}{
		{
			name:       "method header mismatch",
			method:     "tools/list",
			headers:    map[string]string{"Mcp-Method": "tools/call"},
			wantCode:   -32020,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "protocol version header disagreeing with body",
			method: "tools/list",
			params: map[string]any{"_meta": map[string]any{
				"io.modelcontextprotocol/protocolVersion":    mcpkit.ProtocolVersion,
				"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			}},
			headers:    map[string]string{"Mcp-Protocol-Version": "2027-01-01"},
			wantCode:   -32020,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "missing protocol version metadata",
			method: "tools/list",
			params: map[string]any{"_meta": map[string]any{
				"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			}},
			wantCode:   -32602,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "unsupported protocol version",
			method: "tools/list",
			params: map[string]any{"_meta": map[string]any{
				"io.modelcontextprotocol/protocolVersion":    "2027-01-01",
				"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			}},
			headers:    map[string]string{"Mcp-Protocol-Version": "2027-01-01"},
			wantCode:   -32022,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire := testkit.PostHeaders(t, server.MCPURL, token, tt.method, tt.params, tt.headers)
			if wire.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", wire.StatusCode, tt.wantStatus, wire.Body)
			}
			var payload struct {
				Error *struct {
					Code int             `json:"code"`
					Data json.RawMessage `json:"data"`
				} `json:"error"`
			}
			if err := json.Unmarshal(wire.JSON(), &payload); err != nil {
				t.Fatalf("decode error response %s: %v", wire.JSON(), err)
			}
			if payload.Error == nil {
				t.Fatalf("response has no JSON-RPC error: %s", wire.JSON())
			}
			if payload.Error.Code != tt.wantCode {
				t.Fatalf("error code = %d, want %d; body=%s", payload.Error.Code, tt.wantCode, wire.Body)
			}
			if tt.wantCode == -32022 {
				var data struct {
					Supported []string `json:"supported"`
					Requested string   `json:"requested"`
				}
				if err := json.Unmarshal(payload.Error.Data, &data); err != nil {
					t.Fatalf("decode unsupported-version data %s: %v", payload.Error.Data, err)
				}
				if len(data.Supported) != 1 || data.Supported[0] != mcpkit.ProtocolVersion {
					t.Fatalf("supported = %v, want [%s]", data.Supported, mcpkit.ProtocolVersion)
				}
				if data.Requested != "2027-01-01" {
					t.Fatalf("requested = %q, want 2027-01-01", data.Requested)
				}
			}
		})
	}
}

func TestLegacyProtocolVersionHeaderGetsPlainTextRejection(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")

	// The SDK rejects a request whose Mcp-Protocol-Version header names a
	// revision older than 2026-07-28 at the transport layer, before the
	// JSON-RPC layer runs, and answers with text/plain:
	//
	//     Bad Request: Unsupported protocol version (supported versions: 2026-07-28)
	//
	// That is not a JSON-RPC response, so a client cannot read the supported
	// version list and retry. Code -32022 is reserved for versions at or after
	// 2026-07-28, which the SDK does report properly. This test pins the current
	// behaviour so a future SDK or envelope change is visible rather than silent;
	// see docs/lessons.md TQ-* for the open interoperability gap.
	wire := testkit.PostHeaders(t, server.MCPURL, token, "tools/list", nil,
		map[string]string{"Mcp-Protocol-Version": "2025-11-25"})

	if wire.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", wire.StatusCode)
	}
	if contentType := wire.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain while the gap is open", contentType)
	}
	if !strings.Contains(string(wire.Body), "Unsupported protocol version") {
		t.Fatalf("body = %q, want an unsupported-version message", wire.Body)
	}
}

func TestUnknownMethodReturnsMethodNotFoundWith404(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")

	wire := testkit.Post(t, server.MCPURL, token, "no/such/method", nil)
	// SEP-2575: MethodNotFound MUST return HTTP 404, and the body stays a
	// JSON-RPC error so clients can distinguish it from a transport failure.
	if wire.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", wire.StatusCode, wire.Body)
	}
	var payload struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(wire.JSON(), &payload); err != nil {
		t.Fatalf("decode error response %s: %v", wire.JSON(), err)
	}
	if payload.Error == nil || payload.Error.Code != -32601 {
		t.Fatalf("error = %+v, want code -32601; body=%s", payload.Error, wire.Body)
	}
}

func TestDiscoverAndListResultsCarryResultTypeAndPrivateCache(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")

	for _, method := range []string{"server/discover", "tools/list"} {
		t.Run(method, func(t *testing.T) {
			wire := testkit.Post(t, server.MCPURL, token, method, nil)
			if wire.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", wire.StatusCode, wire.Body)
			}
			var payload struct {
				Result struct {
					ResultType string `json:"resultType"`
					TTLMs      int    `json:"ttlMs"`
					CacheScope string `json:"cacheScope"`
				} `json:"result"`
			}
			if err := json.Unmarshal(wire.JSON(), &payload); err != nil {
				t.Fatalf("decode %s result %s: %v", method, wire.JSON(), err)
			}
			if payload.Result.ResultType != "complete" {
				t.Fatalf("%s resultType = %q, want complete", method, payload.Result.ResultType)
			}
			if payload.Result.TTLMs <= 0 {
				t.Fatalf("%s ttlMs = %d, want a positive hint", method, payload.Result.TTLMs)
			}
			if payload.Result.CacheScope != "private" {
				t.Fatalf("%s cacheScope = %q, want private for authenticated results", method, payload.Result.CacheScope)
			}
		})
	}
}

func TestResourceReadCarriesCacheContractAndMissingResourceIsInvalidParams(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")
	server.MCP.AddResource(&mcp.Resource{URI: "file:///known", Name: "known"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{URI: "file:///known", Text: "hi"}},
		}, nil
	})

	read := func(uri string) testkit.Wire {
		return testkit.PostHeaders(t, server.MCPURL, token, "resources/read",
			map[string]any{"uri": uri}, map[string]string{"Mcp-Name": uri})
	}

	known := read("file:///known")
	if known.StatusCode != http.StatusOK {
		t.Fatalf("known resource status = %d, want 200; body=%s", known.StatusCode, known.Body)
	}
	var payload struct {
		Result struct {
			ResultType string `json:"resultType"`
			TTLMs      int    `json:"ttlMs"`
			CacheScope string `json:"cacheScope"`
		} `json:"result"`
	}
	if err := json.Unmarshal(known.JSON(), &payload); err != nil {
		t.Fatalf("decode resources/read result %s: %v", known.JSON(), err)
	}
	if payload.Result.ResultType != "complete" || payload.Result.CacheScope != "private" {
		t.Fatalf("resources/read result = %+v, want complete/private", payload.Result)
	}

	// SEP-2164: a missing resource is invalid params (-32602), not the retired
	// -32002 "Resource not found" code.
	missing := read("file:///missing")
	var missingPayload struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(missing.JSON(), &missingPayload); err != nil {
		t.Fatalf("decode missing-resource response %s: %v", missing.JSON(), err)
	}
	if missingPayload.Error == nil || missingPayload.Error.Code != -32602 {
		t.Fatalf("missing-resource error = %+v, want code -32602; body=%s", missingPayload.Error, missing.Body)
	}
}

func TestToolListOrderIsDeterministic(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")
	for _, name := range []string{"zulu_tool", "alpha_tool", "mike_tool"} {
		server.MCP.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}},
			func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{}, nil
			})
	}

	first := testkit.ListTools(t, server, token)
	second := testkit.ListTools(t, server, token)
	if len(first) != len(second) {
		t.Fatalf("tool counts differ between calls: %v vs %v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("tool order is not stable: %v vs %v", first, second)
		}
	}
	if !sort.StringsAreSorted(first) {
		t.Fatalf("tool order = %v, want sorted for deterministic pagination", first)
	}
}

func TestDraft202012SchemaWithLocalRefIsServedUnchanged(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")
	server.MCP.AddTool(&mcp.Tool{
		Name: "schema_2020",
		InputSchema: map[string]any{
			"$schema":    "https://json-schema.org/draft/2020-12/schema",
			"type":       "object",
			"$defs":      map[string]any{"name": map[string]any{"type": "string"}},
			"properties": map[string]any{"a": map[string]any{"$ref": "#/$defs/name"}},
		},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})

	wire := testkit.Post(t, server.MCPURL, token, "tools/list", nil)
	if wire.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", wire.StatusCode, wire.Body)
	}
	var payload struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(wire.JSON(), &payload); err != nil {
		t.Fatalf("decode tools/list %s: %v", wire.JSON(), err)
	}
	for _, tool := range payload.Result.Tools {
		if tool.Name != "schema_2020" {
			continue
		}
		if tool.InputSchema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("$schema = %v, want draft 2020-12", tool.InputSchema["$schema"])
		}
		properties, ok := tool.InputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("properties = %#v, want an object", tool.InputSchema["properties"])
		}
		ref, ok := properties["a"].(map[string]any)["$ref"]
		if !ok || ref != "#/$defs/name" {
			t.Fatalf("local $ref = %#v, want #/$defs/name", ref)
		}
		// The server must not resolve external references on the client's
		// behalf; a schema it cannot fetch is not a reason to fetch anything.
		if _, ok := tool.InputSchema["$defs"]; !ok {
			t.Fatal("$defs was dropped; local references would no longer resolve")
		}
		return
	}
	t.Fatal("schema_2020 tool not found in tools/list")
}
