# Changelog

All notable changes to `mcp-kit` are documented here.

The module is pre-1.0. Breaking API changes are allowed between minor versions and must include migration notes.

## Unreleased

### Added

- `oauth.DefaultAccessTokenLifespan` and `oauth.DefaultRefreshTokenLifespan` constants, now used by `oauth.Config` defaults. Access tokens default to 1 hour to limit stale-token windows; refresh tokens default to 30 days and continue to rotate on use.
- `oauth.BearerConfig.RequiredScopes`, which adds a `scope="..."` hint to 401 bearer challenges while preserving `resource_metadata`.
- `resource_name` support in OAuth/OIDC protected-resource metadata helpers.
- `oauth.ProtectedResourceMetadataPathFor` / `oauth.ProtectedResourceMetadataURLFor` plus `oidc` re-exports for deriving RFC 9728 path-specific protected-resource metadata URLs from the canonical MCP resource URL.
- `oauth.AuthorizationServerMetadataConfig.Resource` and `ResourceMetadataURL`, emitted as `resource` and `resource_metadata` in OAuth authorization-server metadata when configured.
- `oauth/consent` — production-oriented authorization endpoint helper shared across Go MCP servers, with `Authenticator`, `Renderer`, `ApprovalTokenStore`, `ConsentPolicy`, and `ChallengeProvider` interfaces.
- `oauth/consent/hmacstore` and `oauth/consent/sessionstore` — stock approval-token backends for stateless and session-backed consent flows.
- `oauth/consent/consenttest` — fixtures for in-memory provider setup and canonical consent-handler tests.
- `oauth.Subject.Extra map[string]any` — additive field propagated to OIDC session claims via `oauth.NewSession`.
- `oauth.consent.approved` and `oauth.consent.denied` audit event names for consent decisions.
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

### Changed

- `Provider.AuthorizeHandler` docstring now calls out that it is demo-oriented and points production consumers to `oauth/consent`. Behavior is unchanged.
- OIDC/OAuth discovery metadata now includes `resource`, `resource_metadata`, and `response_modes_supported=["query"]` when route registration knows the protected resource URL.
- `oidc.DiscoveryConfig.RegisterRoutes` now mounts the RFC 9728 path-specific protected-resource metadata route derived from `RouteConfig.ResourceURL`, while keeping the root `/.well-known/oauth-protected-resource` route for compatibility.

### Fixed

- Bearer challenges now include OAuth `error` and `error_description` auth-params in `WWW-Authenticate`, so clients can classify unauthenticated MCP startup as a login-required OAuth flow instead of a generic transport failure.
- Ent-backed dynamic OAuth clients can now whitelist and round-trip the default MCP audience needed by PKCE authorization.
- OAuth storage now persists refresh-token session rows with the refresh-token expiry instead of reusing the access-token expiry for every session type.

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
