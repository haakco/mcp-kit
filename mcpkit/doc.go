// Package mcpkit is the top-level convenience entry point for building MCP
// servers with mcp-kit.
//
// Most consumers only need the few public symbols here:
//   - mcpkit.New     — construct an MCP server with the kit's middleware composed
//   - mcpkit.Config  — the construction parameters
//   - mcpkit.Server  — the resulting server with Handler()
//
// The full public API and design rationale live in DESIGN.md at the repo root.
//
// # Quickstart
//
//	// sdkHandler is the consumer-owned official SDK handler. Register tools on
//	// that SDK server, and enforce domain authz/audit inside those tool handlers.
//	resourceURL := "https://app.example.com/mcp"
//	resourceMetadataURL, err := oauth.ProtectedResourceMetadataURLFor(resourceURL)
//	if err != nil { return err }
//
//	mcpServer, err := mcpkit.New(mcpkit.Config{
//	    Handler: sdkHandler,
//	    Bearer: mcpkit.BearerConfig{
//	        Introspector:        oauthProv.OAuth2Provider(),
//	        SessionFactory:      oauth.NewEmptySession,
//	        ResourceMetadataURL: resourceMetadataURL,
//	        RequiredScopes:      []string{"mcp.read"},
//	        ExpectedAudience:    resourceURL,
//	    },
//	    AllowedOrigins: []string{"https://app.example.com"},
//	    AllowLoopback:  isDev,
//	    AuditEmitter:   myAuditEmitter,
//	})
//	if err != nil { return err }
//
//	mux.Handle("/mcp", mcpServer.Handler())
//
// # Status
//
// v0.4.0 wraps a consumer-owned SDK HTTP handler with the kit's JSON-RPC
// envelope, bearer, and Origin middleware. Domain tools, resources, and prompts
// remain owned by the consumer's SDK server.
package mcpkit
