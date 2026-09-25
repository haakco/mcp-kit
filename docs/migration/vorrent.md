# Vorrent Migration Notes

**Last verified:** 2026-09-26
**Consumer repo:** `/Volumes/Dev/HaakCo/AiProjects/vorrent`
**Vorrent migration commit:** `1d6b870d refactor: adopt shared mcp kit`
**Vorrent closeout commits:** `224c52c6 test: close mcp media blocker coverage`; `3c2596d3 fix: include offline subtitle sidecars`; `f51e78be docs: update offline subtitle package status`
**Kit version consumed:** `github.com/haakco/mcp-kit v0.6.0` (previous: `v0.3.1-0.20260501225920-376d0b3bc2da`)

## v0.6.0 — MCP 2026-07-28 rollout

`v0.6.0` moves Vorrent onto MCP revision 2026-07-28, removes the session lifecycle, raises the Go floor to 1.27, and
drops two `mcpkit.Config` fields.

### Required source changes

1. **Go 1.27.** Vorrent's `go.mod` declares `go 1.26.4`. Raise it to `go 1.27.0`; `v0.6.0` requires it. Keep the
   existing `replace github.com/jackc/pgx/v5 => ...` in place — it still guards the `ory/pop/v6` transitive range.
2. **Upgrade the SDK.** `github.com/modelcontextprotocol/go-sdk` v1.6.1 → `v1.8.0`, and `mcp-kit` → `v0.6.0`.
3. **`mcpkit.Config.Implementation` and `Config.Instructions` are gone.** `internal/api/http_server.go` does not set
   them, so no replacement is needed; Vorrent already sets both on `mcp.NewServer` in `internal/mcpserver/server.go`.
4. **Do not remove `DisableLocalhostProtection`.** An earlier plan revision claimed v1.8.0 dropped it. It did not: the
   field still exists on `StreamableHTTPOptions` and `SSEHandlerOptions`, and the conformance suite has a
   `dns-rebinding-protection` scenario that expects the protection on. Leave it as-is; removing it would weaken
   localhost/DNS-rebinding protection to make a test pass.
5. **Pin the revision and capabilities.** In `internal/mcpserver/server.go`, add
   `SupportedProtocolVersions: []string{mcpkit.ProtocolVersion}` and `Capabilities: &mcp.ServerCapabilities{}` to the
   `mcp.NewServer` options. The explicit empty value stops the SDK advertising the deprecated default logging
   capability.
6. **Cache policy.** Add `SetCacheable: mcpkit.PrivateCache(mcpkit.DefaultCacheTTL)`. Every Vorrent MCP result is
   authenticated and caller-varying.
7. **Stateless transport.** In `internal/api/http_server.go`, the SDK handler passed to `mcpkit.New` must be built with
   `&mcp.StreamableHTTPOptions{Stateless: true}`. Per-request construction (needed for the dynamic audience) is
   compatible with stateless mode; sessions are not.
8. **CORS.** Allow `Mcp-Protocol-Version`, `Mcp-Method`, and `Mcp-Name` in `internal/api/http_helpers.go`; add
   `Mcp-Param-*` only if a tool annotates an input with `x-mcp-header`. Stop exposing `Mcp-Session-Id`.
9. **Tests and probes.** Remove `initialize`, `notifications/initialized`, `ping`, and `Mcp-Session-Id` expectations.
   Rewrite the `PR-02`-style live probe around `server/discover` then `tools/list`.
10. **CIMD.** Metadata documents are resolved by default. Set `oauth.ClientIDMetadata.Disabled` and
    `oidc.DiscoveryConfig.ClientIDMetadataDocumentSupported = false` together if Vorrent must stay registration-only.

### Rollout order

Vorrent goes last because it depends on the tagged release:

1. Kit `v0.6.0` is tagged only after `skills-mcp` has proved the same commit in its real deployment.
2. Upgrade Vorrent to the tag, prove locally (boundary suites plus conformance), commit, push.
3. With owner approval, deploy and run the authenticated `server/discover` + `tools/list` probe and the OAuth recovery
   probe.

## What Changed

Vorrent now consumes `mcp-kit` from GitHub without a local `replace`.

The migration moved transport and OAuth edge behavior into the kit while keeping Vorrent's domain MCP server code in Vorrent:

- `/mcp` is wrapped by `mcpkit.New`.
- Kit bearer middleware owns the OAuth challenge and token introspection boundary.
- Kit origin middleware owns Origin validation.
- Kit envelope middleware replaces Vorrent's local JSON-RPC envelope rewriter.
- Kit OAuth metadata handlers serve the MCP protected-resource and authorization-server metadata.
- Kit dynamic client registration handler serves `/mcp-oauth/register`.
- Vorrent tools, resources, prompts, and service adapters stay in `internal/mcpserver`.

## Vorrent Adapters

Vorrent did not need a separate `internal/kitwiring` package for this pass. It already had OAuth persistence and route ownership in `internal/api`, so the integration uses thin adapters at the API boundary:

- `kitMCPHandler` in `internal/api/http_server.go` builds kit config per request so the OAuth resource/audience matches the request host.
- `strictMCPAudienceIntrospector` in `internal/api/http_server.go` preserves Vorrent's exact `<baseURL>/mcp` audience requirement.
- `vorrentMCPClientRegistrar` in `internal/api/mcp_oauth_register.go` persists kit-generated dynamic clients into Vorrent's existing Ent OAuth client table.
- `internal/mcpserver/auth.go` reads scopes from `github.com/haakco/mcp-kit/oauth`.

The app-specific OAuth route files remain active as thin glue. They do not provide a compatibility path; they mount kit handlers and adapt Vorrent's existing persistence/audit model.

## Kit Changes Required By Vorrent

The Vorrent pass required these kit commits after `v0.3.0`:

- `d7214ed feat: expose oauth metadata handlers`
- `ba1b961 fix: persist oauth token auth method`
- `376d0b3 feat: configure registration defaults`

These covered metadata handler reuse, dynamic registration defaults for omitted `scope` / grant / response / auth-method fields, and persistence of `token_endpoint_auth_method`.

## Removed Vorrent Code

Vorrent deleted local middleware files instead of keeping compatibility paths:

- `internal/mcpserver/jsonrpc_envelope.go`
- `internal/mcpserver/jsonrpc_envelope_test.go`
- `internal/mcpserver/origin.go`
- `internal/mcpserver/origin_test.go`

The old dynamic-client read-only scope repair path was removed from Vorrent's OAuth service because kit registration now stores the canonical provider defaults at creation time.

## Verification Completed

Vorrent verification that passed during the migration:

- `go test ./internal/mcpserver/... -count=1 -race`
- `go test ./internal/api/... -run='Mcp' -count=1`
- `go test ./... -count=1`
- `go build ./...`
- `just qa-critical`
- Live OAuth metadata and dynamic client registration against a running kit-backed Vorrent server.
- Authorization-code grant and refresh-token grant.
- Unauthenticated `/mcp` kit bearer challenge.
- MCP `server/discover` and `tools/list`, `resources/list`, `prompts/list` on the 2026-07-28 wire;
- Safe tool calls, resource reads, prompt reads, error envelopes, and read-only token write denial.
- MCP Inspector CLI rendered all 14 tool schemas.
- Codex `mcp login vorrent-mcp` completed through browser OAuth approval driven by Playwright MCP.
- Claude Code real-client gate passed against `vorrent-mcp`.
- Full destructive/fixture-heavy final blocker passed against the rebuilt kit-backed Vorrent binary on 2026-05-02:
  - disposable Big Buck Bunny download deleted with `delete_files=true`;
  - fixture-backed `start_transcode_job` completed and produced a probed MP4 output;
  - disposable `cancel_transcode_job` returned canceled status.
- Security follow-up: Vorrent now rejects MCP transcode input/output paths outside app-owned media storage before queueing; a live rerun rejected an outside `/tmp` input and completed an allowed disposable fixture under the resolved Vorrent media/download root.

## v0.4.0 Tag Status

No Vorrent destructive/fixture-heavy migration blocker remains open. The final Vorrent closeout SHA is recorded above and in the master plan. `v0.4.0` was tagged on 2026-05-02.

## Gotchas

- Vorrent computes the MCP OAuth resource dynamically from the request host. Do not mix `localhost` and `127.0.0.1` in the same OAuth token flow unless the audience/resource is intentionally changed.
- Vorrent's real user database may have no users. For local OAuth testing, create or reset a local admin user before opening the authorization URL.
- Project-scoped `.mcp.json` should not include shared bearer-token placeholders. Vorrent removed the project `skills-http` entry that referenced `${SKILLS_BEARER_TOKEN}`; keep secret-bearing Skills HTTP config in local user config.
- `v0.6.0`: a 2026-07-28 request against a stateful handler fails with `-32022`. Check `Stateless: true` first.
- `v0.6.0`: `-32020` means a `Mcp-Method` / `Mcp-Name` / `Mcp-Protocol-Version` header disagrees with the body, not
  that auth failed.
