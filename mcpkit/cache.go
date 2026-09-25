package mcpkit

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProtocolVersion is the MCP revision mcp-kit targets.
//
// The kit is modern-only. Servers restrict themselves to this revision, serve
// Streamable HTTP statelessly, and carry per-request metadata instead of the
// retired initialize handshake. Pass it to
// mcp.ServerOptions.SupportedProtocolVersions so a server advertises exactly
// one revision.
const ProtocolVersion = "2026-07-28"

// DefaultCacheTTL is the freshness window PrivateCache applies when the caller
// has no more specific requirement.
const DefaultCacheTTL = time.Minute

// cacheScopePrivate marks a cacheable MCP result as usable only by the
// requesting user's client. MCP revision 2026-07-28 defines "public" and
// "private"; the SDK does not export constants for them.
const cacheScopePrivate = "private"

// PrivateCache returns an mcp.ServerOptions.SetCacheable function that marks
// every cacheable result private.
//
// MCP defaults an absent cacheScope to "public". That default is wrong for a
// server that authenticates callers: server/discover, the four list methods,
// and resources/read can all vary by caller, so a shared cache would serve one
// user's view to another. Set this on every server that checks a token.
//
// A handler whose result genuinely cannot vary between callers may overwrite
// the value it receives, but only with evidence that the result is identical
// and safe for every caller.
func PrivateCache(ttl time.Duration) func(context.Context, mcp.Request, *mcp.Cacheable) {
	return func(_ context.Context, _ mcp.Request, c *mcp.Cacheable) {
		c.TTLMs = int(ttl.Milliseconds())
		c.CacheScope = cacheScopePrivate
	}
}
