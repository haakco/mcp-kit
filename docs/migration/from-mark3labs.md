# Migrating From mark3labs MCP Go To Official Go SDK

**Last verified:** 2026-05-02
**Source migration:** `skills-mcp` adopting `github.com/haakco/mcp-kit`

## Scope

This guide covers the MCP server API migration from `github.com/mark3labs/mcp-go` to `github.com/modelcontextprotocol/go-sdk` when adopting `mcp-kit`.

It does not cover OAuth or route wiring. For those, use the consumer migration notes.

## Core Differences

| Area | mark3labs | Official SDK with mcp-kit |
|---|---|---|
| Server construction | App-owned server wrapper | consumer-owned official SDK server wrapped by `mcpkit.New` |
| HTTP transport | Framework-specific glue | `mcpkit.Server.Handler()` mounted at `/mcp` |
| Middleware | App-owned | Kit-owned origin, bearer, and envelope middleware |
| Tool registration | mark3labs helper types | official SDK `mcp.AddTool` style registration |
| Auth context | App-specific middleware context | kit OAuth context plus consumer scope helpers |

## Migration Steps

1. Add `github.com/haakco/mcp-kit@v0.4.0` or a newer reviewed tag.
2. Build the kit-backed server and mount it on a temporary route only if parity testing needs it.
3. Migrate tools by domain, keeping names, descriptions, input schemas, and output contracts stable.
4. Move scope checks to read the kit-authenticated context.
5. Verify `tools/list`, `resources/list`, and `prompts/list` against both old and new routes during parity.
6. Cut over the canonical `/mcp` route to the kit handler.
7. Remove mark3labs imports and temporary route support.
8. Run the cycle methodology and real-client gates against the rebuilt binary.

## Tool Registration Pattern

Keep registration grouped by domain so the official SDK migration does not create a server god object:

```go
sdkServer := mcp.NewServer(&mcp.Implementation{Name: "skills-mcp", Version: version}, nil)
sdkHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
	return sdkServer
}, nil)
kitServer, err := mcpkit.New(mcpkit.Config{Handler: sdkHandler, Bearer: bearerConfig})
if err != nil {
	return err
}

type skillsHandler struct {
	deps Deps
}

func (h *skillsHandler) registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_skills",
		Description: "Search skills by keyword.",
	}, h.searchSkills)
}
```

## Verification Checklist

- [ ] No `github.com/mark3labs/mcp-go` dependency remains in `go.mod`.
- [ ] Every migrated tool appears in `tools/list` with the expected name.
- [ ] Existing client verification passes against the kit-backed route.
- [ ] Inspector connects and lists tools/resources/prompts.
- [ ] Claude Code connects and executes one known-safe tool.
- [ ] No temporary `/mcp-v2` or compatibility mount remains after cutover.

## Failure Modes

- Stale binaries can look like SDK serialization bugs. Rebuild before filing an SDK issue.
- Tool schema drift can hide in descriptions or optional fields; compare `tools/list` output, not only unit tests.
- Disabled-auth smoke tests do not prove hosted-client compatibility.
