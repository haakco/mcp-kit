# skills-mcp Migration Notes

**Last verified:** 2026-05-02
**Consumer repo:** `/Users/timhaak/Dev/HaakCo/AiProjects/skills`
**MCP app:** `apps/skills-mcp`
**Kit gate:** `v0.3.0`

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
- MCP initialize, `tools/list`, `resources/list`, and prompt retrieval;
- Inspector real-client gate;
- Claude Code real-client gate with one tool call against a known fixture.

## Gotchas

- Do not keep a mark3labs route, compatibility route, or duplicate mount.
- Do not move Skills-specific registry or materialization policy into the kit.
- Auth-enabled client verification must use a real bearer token; a disabled-auth pass is not equivalent coverage.
- Keep the hostname consistent through discovery, OAuth, and client verification. Do not mix `localhost` and `127.0.0.1` in the same auth-enabled flow.
- Run the cycle methodology against the rebuilt binary before filing SDK bugs.
