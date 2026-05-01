package testkit

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/mcpkit"
)

// Server is an in-memory mcp-kit server for tests.
type Server struct {
	URL             string
	MCPURL          string
	HTTPServer      *httptest.Server
	UserStore       *UserStore
	RegisteredTools []string
}

// NewServer starts an in-memory mcp-kit server with test-token auth.
func NewServer(t testing.TB) *Server {
	t.Helper()
	users := NewUserStore(t)
	tools := []string{"hello_world"}
	sessionID := randomID(t)
	kit, err := mcpkit.New(mcpkit.Config{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleMCP(w, r, tools, sessionID)
		}),
		Bearer: mcpkit.BearerConfig{
			TokenValidator: TokenValidator(t),
		},
		AllowLoopback: true,
		AuditEmitter:  audit.Discard(),
	})
	if err != nil {
		t.Fatalf("create mcp-kit server: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", kit.Handler())
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	return &Server{
		URL:             httpServer.URL,
		MCPURL:          httpServer.URL + "/mcp",
		HTTPServer:      httpServer,
		UserStore:       users,
		RegisteredTools: tools,
	}
}

func handleMCP(w http.ResponseWriter, r *http.Request, tools []string, sessionID string) {
	var request struct {
		ID     any    `json:"id"`
		Method string `json:"method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "malformed payload: invalid JSON", http.StatusBadRequest)
		return
	}
	switch request.Method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", sessionID)
		writeJSON(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "mcp-kit-test", "version": "0.0.0-test"},
			},
		})
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
	case "ping":
		writeJSON(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  map[string]any{},
		})
	case "tools/list":
		items := make([]map[string]any, 0, len(tools))
		for _, tool := range tools {
			items = append(items, map[string]any{
				"name":        tool,
				"description": "Test tool.",
				"inputSchema": map[string]any{"type": "object"},
			})
		}
		writeJSON(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  map[string]any{"tools": items},
		})
	default:
		http.Error(w, "JSON RPC not handled: "+request.Method, http.StatusBadRequest)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("testkit: write JSON", "error", err)
	}
}
