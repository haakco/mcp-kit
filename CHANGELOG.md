# Changelog

All notable changes to `mcp-kit` are documented here.

The module is pre-1.0. Breaking API changes are allowed between minor versions and must include migration notes.

## v0.1.0 - 2026-05-01

Initial reusable MCP server kit surface.

### Added

- JSON-RPC envelope middleware for normalizing SDK protocol errors into canonical JSON-RPC error responses.
- Origin allowlist middleware with explicit loopback development support.
- Initial `audit.Emitter`, `authz.Service`, and `userstore.Store` interfaces for consumer-provided integrations.
- Initial `mcpkit.New` configuration shell for the future Streamable HTTP + OAuth server wrapper.
- Cycle methodology docs, lessons learned, and a dispatch runbook template for real-client MCP validation.

### Notes

- OAuth, CLI auth, Ent mixins, and the testkit are planned for v0.2.x and v0.3.x.
- The current `mcpkit.New` server constructor is intentionally incomplete and returns `ErrNotImplemented`.
