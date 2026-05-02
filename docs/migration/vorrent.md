# Vorrent Migration Notes

**Last verified:** 2026-05-02
**Consumer repo:** `/Volumes/Dev/HaakCo/AiProjects/vorrent`
**Vorrent commit:** `1d6b870d refactor: adopt shared mcp kit`
**Kit version consumed:** `github.com/haakco/mcp-kit v0.3.1-0.20260501225920-376d0b3bc2da`

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
- MCP initialize and `tools/list`, `resources/list`, `prompts/list`.
- Safe tool calls, resource reads, prompt reads, error envelopes, and read-only token write denial.
- MCP Inspector CLI rendered all 14 tool schemas.
- Codex `mcp login vorrent-mcp` completed through browser OAuth approval driven by Playwright MCP.

## Remaining Before Tagging v0.4.0

Do not tag `v0.4.0` until these are closed:

- Full Vorrent MCP cycle P0-P10, including destructive/fixture-heavy rows, against the kit-backed binary.
- Claude Code real-client gate against `vorrent-mcp`.

## Gotchas

- Vorrent computes the MCP OAuth resource dynamically from the request host. Do not mix `localhost` and `127.0.0.1` in the same OAuth token flow unless the audience/resource is intentionally changed.
- Vorrent's real user database may have no users. For local OAuth testing, create or reset a local admin user before opening the authorization URL.
- Project-scoped `.mcp.json` should not include shared bearer-token placeholders. Vorrent removed the project `skills-http` entry that referenced `${SKILLS_BEARER_TOKEN}`; keep secret-bearing Skills HTTP config in local user config.
