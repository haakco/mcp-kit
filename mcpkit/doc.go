// Package mcpkit is the top-level convenience entry point for building MCP
// servers with mcp-kit.
//
// Most consumers only need the few public symbols here:
//   - mcpkit.New     — construct an MCP server with the kit's middleware composed
//   - mcpkit.Config  — the construction parameters
//   - mcpkit.Server  — the resulting server with Handler() and SDK() methods
//
// The full public API and design rationale live in DESIGN.md at the repo root.
//
// # Quickstart
//
//	mcpServer, err := mcpkit.New(mcpkit.Config{
//	    Implementation: mcp.Implementation{Name: "my-mcp", Version: "0.1.0"},
//	    Validator:      oauthProv.TokenValidator(),
//	    AllowedOrigins: []string{"https://app.example.com"},
//	    AllowLoopback:  isDev,
//	    AuditEmitter:   audit.Discard(),
//	})
//	if err != nil { return err }
//
//	mux.Handle("/mcp", mcpServer.Handler())
//
// # Status
//
// v0.1.0 is a skeleton. mcpkit.New currently returns ErrNotImplemented; the
// JSON-RPC envelope middleware (mcpmw.Envelope) and Origin allowlist
// (mcpmw.Origin) ARE working and tested — consumers wanting just those two
// can use them standalone. v0.2.0 will land the full OAuth core extracted
// from skills-mcp.
package mcpkit
