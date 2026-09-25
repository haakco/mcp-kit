# Changelog

All notable changes to `mcp-kit` are documented here.

The module is pre-1.0. Breaking API changes are allowed between minor versions and must include migration notes.

## v0.6.0 - 2026-09-25

MCP revision **2026-07-28** migration. The kit is now modern-only: it targets a single protocol revision, serves
Streamable HTTP statelessly, and carries per-request metadata instead of the retired initialize handshake. This is a
breaking pre-1.0 release.

### Breaking changes

- **Go 1.27 is required** (toolchain `go1.27.1`, pinned in `mise.toml` and CI). Consumers must raise their own `go`
directive; the previous minimum was Go 1.26.
- **`mcpkit.ErrNotImplemented` was removed.** It was a v0.1.0 stub marker and is no longer referenced anywhere.
- **`mcpkit.Config.Implementation` and `mcpkit.Config.Instructions` were removed.** They were never read: consumers
already set identity and instructions on their own `mcp.Server`. Set them on `mcp.NewServer` instead.
- **`testkit.RunHandshake` and `testkit.RunHandshakeURL` were removed.** MCP 2026-07-28 has no handshake. Use
`testkit.Discover` / `testkit.DiscoverURL`, which return a `*mcp.DiscoverResult` instead of a session ID.
- **`testkit.ListTools` / `testkit.ListToolsURL` no longer take a session ID.**
- **`testkit.NewServer` now serves a real SDK server** restricted to `2026-07-28`, stateless, with explicit empty
capabilities and private caching. Its hand-written JSON-RPC fixture is gone. Tests that relied on `initialize`,
`notifications/initialized`, `ping`, or `Mcp-Session-Id` must be rewritten.
- **Authorization responses now carry `iss`** (RFC 9207). `cliauth` rejects a mismatched `iss` before redeeming the
code. An issuer that omits `iss` is still accepted, for compatibility with deployments that predate RFC 9207.
- **Client ID Metadata Documents are resolved by default.** A `client_id` that is an HTTPS URL and is absent from the
consumer's store is fetched and validated. Set `oauth.Config.ClientIDMetadata.Disabled` to keep registration-only
behaviour.
- **Bearer challenges now escape `resource_metadata`.** A metadata URL containing a quote, comma, or backslash no
longer breaks the `WWW-Authenticate` header structure.
- **Dependency majors:** `github.com/go-jose/go-jose/v3` → `v4`, `github.com/modelcontextprotocol/go-sdk` v1.6.1 →
v1.8.0, `golangci-lint` v2.12.2 → v2.14.0, plus current `golang.org/x/*`.
- **Cleared every open Dependabot alert on the module graph.** `google.golang.org/grpc` v1.82.1 → v1.83.1 fixes two
high-severity and one moderate alert; `go.opentelemetry.io/otel` and its OTLP/Zipkin exporters v1.43.0 → v1.46.0 fix
four low-severity alerts. `govulncheck` already reported no reachable vulnerabilities; this clears the alerts that
exist regardless of reachability.

### Migration

1. Raise the consumer module to Go 1.27 and upgrade `github.com/modelcontextprotocol/go-sdk` to v1.8.0.
2. Build the SDK server with `SupportedProtocolVersions: []string{mcpkit.ProtocolVersion}` and
   `Capabilities: &mcp.ServerCapabilities{}` (an explicit empty value stops the SDK advertising historical default
   logging).
3. Serve it with `mcp.NewStreamableHTTPHandler(..., &mcp.StreamableHTTPOptions{Stateless: true})`. Stateless is
   mandatory for 2026-07-28 over Streamable HTTP.
4. Set `SetCacheable: mcpkit.PrivateCache(mcpkit.DefaultCacheTTL)` unless a result is provably identical for every
   caller.
5. Allow `Mcp-Protocol-Version`, `Mcp-Method`, and `Mcp-Name` through CORS, and stop exposing `Mcp-Session-Id`.
6. Delete session/initialize assertions from tests; use `testkit.Discover` and `testkit.ListTools`.

### Added

- `mcpkit.ProtocolVersion` (`"2026-07-28"`), `mcpkit.DefaultCacheTTL`, and `mcpkit.PrivateCache(ttl)` — a ready-made
  `mcp.ServerOptions.SetCacheable` that marks cacheable results `private`. MCP defaults an absent `cacheScope` to
  `public`, which is wrong for any authenticated server.
- `testkit.Post` / `testkit.PostHeaders` / `testkit.Do` / `testkit.Wire` — raw wire-level helpers for asserting the
  transport contract. `Wire.JSON()` unwraps the SSE framing the transport uses by default.
- Server-side Client ID Metadata Document resolution (SEP-991) in `oauth`: `oauth.ClientIDMetadataConfig`,
  `oauth.ClientMetadataPolicy`, and `oauth.ClientIDMetadataFetcher`, plus `storage.ClientMetadataResolver` and
  `storage.Storage.WithClientMetadataResolver`. Fetches are bounded in time, size, and redirects; loopback, private,
  link-local, multicast, and unspecified targets are refused after DNS resolution; the validated address is dialled
  rather than re-resolved; documents are cached with HTTP cache semantics and a bounded in-memory cache.
- `oauth.Provider.AddIssuerParameter` records the RFC 9207 `iss` parameter on authorization responses.
- `oauth.AuthorizationServerMetadataConfig.ClientIDMetadataDocumentSupported` and
  `oidc.DiscoveryConfig.ClientIDMetadataDocumentSupported`, emitted as `client_id_metadata_document_supported`.
- `testkit.contract_test.go` pins the 2026-07-28 transport contract: POST-only stateless serving, absent session
  headers, `-32020`/`-32022`/`-32602` error codes, `resultType`/`ttlMs`/`cacheScope`, deterministic list ordering,
  SEP-2164 missing-resource handling, and draft 2020-12 schemas with local `$ref`.

### Changed

- `oauth/storage.Storage.GetClient` falls back to an optional client metadata resolver after a store miss, so
  registered clients always win. A metadata URL that cannot be resolved reports an invalid client rather than a server
  error, and the fetch failure is not echoed to the caller.
- Bearer challenges escape `resource_metadata` through the same quoting helper used for scope hints.

### Fixed

- `resource_metadata` in `WWW-Authenticate` is now a correctly quoted auth-param; a value containing a quote, comma,
  or backslash previously terminated the header structure early.

### Added

- Refresh-token rotation, replay, and failed-exchange audit events now include a
  non-reversible token fingerprint. Replay events also include rotation time,
  first replay time, replay age/count, configured window, and whether the
  cached response was returned, allowing consumers to measure stale-token use
  without recording token values.
- `oauth.DefaultAccessTokenLifespan` and `oauth.DefaultRefreshTokenLifespan` constants, now used by `oauth.Config` defaults. Access tokens default to 1 hour to limit stale-token windows; refresh tokens default to 30 days and continue to rotate on use.
- `oauth.BearerConfig.RequiredScopes`, which enforces required bearer scopes while preserving `resource_metadata` on 401 challenges and adding `scope="..."` only on 403 insufficient-scope challenges.
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
- Invalid-token bearer challenges now use the interoperable `Bearer realm="OAuth"` shape and omit `scope`, matching the RFC 6750 examples and keeping scope hints for `insufficient_scope`.

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
