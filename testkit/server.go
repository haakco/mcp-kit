package testkit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/mcpkit"
)

const (
	// ServerName identifies the testkit MCP server.
	ServerName = "mcp-kit-test"
	// ServerVersion is the version the testkit MCP server reports.
	ServerVersion = "0.0.0-test"
	// HelloTool is the tool registered on every testkit server.
	HelloTool = "hello_world"
)

// Server is an in-memory mcp-kit server for tests.
//
// It mirrors the kit's production guidance: MCP 2026-07-28 only, stateless
// Streamable HTTP, explicit empty capabilities, and private caching. Tests that
// need different transport behavior should build their own mcp.Server rather
// than widen this fixture.
type Server struct {
	URL             string
	MCPURL          string
	HTTPServer      *httptest.Server
	MCP             *mcp.Server
	UserStore       *UserStore
	RegisteredTools []string
}

// NewServer starts an in-memory mcp-kit server with test-token auth.
//
// Register additional tools on the returned Server.MCP before the first
// request; the handler resolves the mcp.Server per request.
func NewServer(t testing.TB) *Server {
	t.Helper()
	users := NewUserStore(t)
	tools := []string{HelloTool}

	mcpServer := mcp.NewServer(
		&mcp.Implementation{Name: ServerName, Version: ServerVersion},
		&mcp.ServerOptions{
			Capabilities:              &mcp.ServerCapabilities{},
			SupportedProtocolVersions: []string{mcpkit.ProtocolVersion},
			SetCacheable:              mcpkit.PrivateCache(mcpkit.DefaultCacheTTL),
		},
	)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        HelloTool,
		Description: "Return a greeting.",
	}, helloWorld)

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)

	kit, err := mcpkit.New(mcpkit.Config{
		Handler: handler,
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
		MCP:             mcpServer,
		UserStore:       users,
		RegisteredTools: tools,
	}
}

func helloWorld(context.Context, *mcp.CallToolRequest, any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "hello world"}},
	}, nil, nil
}
