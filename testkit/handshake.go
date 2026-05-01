package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
)

// RunHandshake initializes a Streamable HTTP MCP session and returns Mcp-Session-Id.
func RunHandshake(t testing.TB, server *Server, token string) string {
	t.Helper()
	return RunHandshakeURL(t, server.MCPURL, token)
}

// RunHandshakeURL initializes a Streamable HTTP MCP session at mcpURL.
func RunHandshakeURL(t testing.TB, mcpURL string, token string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-kit-test-client", Version: "0.0.0-test"}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{},
	})
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             mcpURL,
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
		OAuthHandler:         staticOAuthHandler{token: token},
	}, nil)
	if err != nil {
		t.Fatalf("connect official MCP client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() }) //nolint:errcheck // test cleanup cannot recover from transport close failures
	sessionID := session.ID()
	if sessionID == "" {
		t.Fatal("official MCP client returned empty session ID")
	}
	return sessionID
}

type staticOAuthHandler struct {
	token string
}

func (h staticOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: h.token, TokenType: "Bearer"}), nil
}

func (staticOAuthHandler) Authorize(context.Context, *http.Request, *http.Response) error {
	return fmt.Errorf("test token was rejected")
}

func postMCP(t testing.TB, mcpURL string, token string, sessionID string, payload string) (string, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, mcpURL, bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("build MCP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", mcpURL, err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read MCP response: %v", err)
	}
	body := strings.TrimSpace(string(bodyBytes))
	if resp.StatusCode/100 != 2 {
		t.Fatalf("MCP request failed: %s: %s", resp.Status, body)
	}
	return resp.Header.Get("Mcp-Session-Id"), body
}

// ListTools calls tools/list over Streamable HTTP and returns tool names.
func ListTools(t testing.TB, server *Server, token string, sessionID string) []string {
	t.Helper()
	return ListToolsURL(t, server.MCPURL, token, sessionID)
}

// ListToolsURL calls tools/list at mcpURL and returns tool names.
func ListToolsURL(t testing.TB, mcpURL string, token string, sessionID string) []string {
	t.Helper()
	_, body := postMCP(t, mcpURL, token, sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var payload struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := jsonUnmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	tools := make([]string, 0, len(payload.Result.Tools))
	for _, tool := range payload.Result.Tools {
		tools = append(tools, tool.Name)
	}
	return tools
}

func jsonUnmarshal(data []byte, value any) error {
	if len(data) == 0 {
		return fmt.Errorf("empty response")
	}
	return json.Unmarshal(data, value)
}

var _ mcpauth.OAuthHandler = staticOAuthHandler{}
