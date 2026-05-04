# Changelog

All notable changes to `mcp-kit` are documented here.

The module is pre-1.0. Breaking API changes are allowed between minor versions and must include migration notes.

## Unreleased

### Added

- `docs/recipes/admin-gate.md` — pattern doc for enforcing admin-only mutations consistently across HTTP API + MCP surfaces. Sourced from skills-mcp Phase 1.5 / Phase 1 validation findings (privilege-escalation bug in the MCP `update_skill` tool path).
- `docs/migration/skills-mcp.md` and plan status updates now record the completed `skills-mcp` donor migration onto kit-owned OAuth/MCP wiring.
- `entschema.OAuthClient` now includes an `audience` JSON field so Ent-backed consumers persist the kit `storage.Client.Audience` contract used by default MCP resource indicators.
- `docs/lessons.md` — added `AG-*` (authz gotchas) and `CG-*` (code-quality gotchas) sections:
  - `AG-01` — cross-surface admin gates must live in the service layer or every handler must enforce.
  - `AG-02` — list/count parity for visibility-filtered endpoints (or pagination totals leak existence).
  - `AG-03` — "no scopes in context" must mean default-deny, not allow; use an explicit `WithAuthDisabled` sentinel.
  - `AG-04` — auth scopes belong on context, not in the actor struct (separates auth state from identity).
  - `CG-01` — silent-error annotations need a concrete reason, not "best-effort".
  - `CG-02` — methods-per-receiver caps catch god-classes early; prefer composition when the cap fires.
  - `CG-03` — atomic commit etiquette for pre-existing test fixes during feature work.

### Fixed

- Ent-backed dynamic OAuth clients can now whitelist and round-trip the default MCP audience needed by PKCE authorization.

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
