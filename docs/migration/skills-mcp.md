# skills-mcp Migration Notes

**Last verified:** 2026-09-26
**Consumer repo:** `/Volumes/Dev/HaakCo/AiProjects/skills`
**MCP app:** `apps/skills-mcp`
**Kit gate:** `v0.6.0`

## v0.6.0 — MCP 2026-07-28 rollout

`v0.6.0` is the second breaking migration. It moves Skills onto MCP revision 2026-07-28, removes the session lifecycle,
and raises the Go floor to 1.27.

### Required source changes

1. **Go 1.27.** `apps/skills-mcp/go.mod` already declares `go 1.27.0`. Upgrade
   `github.com/modelcontextprotocol/go-sdk` to `v1.8.0` and `github.com/haakco/mcp-kit` to `v0.6.0`.
2. **Pin the revision and capabilities.** In `internal/server/server.go`, add to the `mcp.NewServer` options:
   `SupportedProtocolVersions: []string{mcpkit.ProtocolVersion}` and `Capabilities: &mcp.ServerCapabilities{}`. The
   explicit empty value stops the SDK advertising the deprecated default logging capability.
3. **Cache policy.** Add `SetCacheable: mcpkit.PrivateCache(mcpkit.DefaultCacheTTL)`. Every Skills result is
   authenticated and caller-varying. Do not use `public` to make a cache test pass.
4. **Stateless transport.** The handler built in `Server.ConfigureKitServer`
   (`mcp.NewStreamableHTTPHandler`) must set `&mcp.StreamableHTTPOptions{Stateless: true}`. Stateless is mandatory for
   2026-07-28 over HTTP; a stateful handler rejects the revision outright.
5. **`mcpkit.Config`.** `Implementation` and `Instructions` are gone. Skills already sets both on `mcp.NewServer`, so no
   replacement is needed. `Config.Validator` still works; migrating those call sites to `Bearer.TokenValidator` is
   optional cleanup.
6. **CORS.** In `internal/web/cors.go`, allow `Mcp-Protocol-Version`, `Mcp-Method`, and `Mcp-Name`; add `Mcp-Param-*`
   only if a tool annotates an input with `x-mcp-header`. Stop exposing `Mcp-Session-Id`.
7. **Tests.** Delete `initialize`, `notifications/initialized`, `ping`, and `Mcp-Session-Id` assertions. Replace
   `testkit.RunHandshake` with `testkit.Discover`, and drop the session argument from `testkit.ListTools`.
8. **CIMD.** Metadata documents are resolved by default. If Skills must stay registration-only, set
   `oauth.ClientIDMetadata.Disabled` and `oidc.DiscoveryConfig.ClientIDMetadataDocumentSupported = false` together, so
   discovery does not advertise a mechanism the server refuses.

### Rollout order

1. Land the kit `v0.6.0` change and prove it locally.
2. Consume the candidate through a pseudo-version (`go get github.com/haakco/mcp-kit@<sha>`) and prove it in the real
   deployment. Never commit a local `replace` directive.
3. Once proven, tag the exact kit commit `v0.6.0` and repin to the tag.
4. Re-run the boundary suites and conformance against the deployed server.

`docs/plans/2026-09-25_mcp-2026-07-28-protocol-migration.md` (kit) owns the sequencing and acceptance criteria.

## What Changed

`skills-mcp` was the donor consumer for most of the kit's OAuth surface. The migration moved reusable OAuth, discovery, CLI auth, PAT validation, key rotation, bearer middleware, and test helpers into `github.com/haakco/mcp-kit`.

The app keeps the pieces that are specific to Skills:

- tool, resource, and prompt behavior;
- the skill registry and materialization model;
- user, authz, and audit services;
- HTTP server ownership and deployment config.

## Migration Shape

Use thin adapters rather than copying app policy into the kit:

- OAuth storage/key adapters map Skills OAuth tables to the kit storage and key manager interfaces.
- Audit adapters map kit and domain MCP events into the existing Skills audit service.
- Authz adapters keep Skills permission checks in Skills-owned service code.
- OAuth route registration mounts kit handlers on the existing public paths.

The final route shape is kit-backed `/mcp`. Do not keep a temporary `/mcp-v2`,
compatibility route, or duplicate mount.

## SDK Migration

The Skills MCP surface migrated from `github.com/mark3labs/mcp-go` to the official `github.com/modelcontextprotocol/go-sdk`.

The migration should preserve client-facing contracts:

- tool names stay stable;
- input schemas stay stable unless intentionally changed;
- output text and structured content stay usable by existing clients;
- prompts continue to mention real tool names only.

After cutover, `go.mod` should include `github.com/haakco/mcp-kit` and should not include `github.com/mark3labs/mcp-go`.

## Verification

Minimum verification for a Skills migration closeout:

- `go test ./... -count=1` in `apps/skills-mcp`;
- `just verify-mcp-clients http://localhost:8892` against the kit-backed route;
- OAuth discovery, dynamic registration, authorization code, refresh rotation, and PAT validation;
- `server/discover`, `tools/list`, `resources/list`, and prompt retrieval on the 2026-07-28 wire;
- no `Mcp-Session-Id` on any response, and `GET`/`DELETE` answering `405` with `Allow: POST`;
- Inspector real-client gate;
- Claude Code real-client gate with one tool call against a known fixture.

## Gotchas

- Do not keep a mark3labs route, compatibility route, or duplicate mount.
- Do not move Skills-specific registry or materialization policy into the kit.
- Auth-enabled client verification must use a real bearer token; a disabled-auth pass is not equivalent coverage.
- Keep the hostname consistent through discovery, OAuth, and client verification. Do not mix `localhost` and `127.0.0.1` in the same auth-enabled flow.
- Run the cycle methodology against the rebuilt binary before filing SDK bugs.
- `v0.6.0`: a 2026-07-28 request against a stateful handler fails with `-32022`. Check `Stateless: true` first.
- `v0.6.0`: a rejected request often means a missing `Mcp-Method` or `Mcp-Name` header (`-32020`), not an auth problem.
