# OAuth MCP Defaults Plan

**Status:** Implemented; downstream consumer upgrades remain separate.
**Goal:** Move reusable OAuth/MCP renewal and discovery fixes into `mcp-kit` so Skills, Vorrent, and future Go MCP servers do not carry app-local wrappers.
**Background:** Skills local dev reproduced a stale-token `/mcp` challenge that preserved `resource_metadata` but omitted `scope`, causing poorer client login UX than Linear. Linear's current OAuth docs publish 24-hour user access tokens with refresh-token rotation, while `mcp-kit` still defaulted access tokens to 1 hour and refresh tokens to 24 hours.
**Architecture:** Keep protocol-sensitive defaults in `oauth.Config`, bearer challenge behavior in `oauth.Bearer`, and protected-resource display metadata in OAuth/OIDC discovery helpers. Consumers may still override lifetimes and scopes explicitly.
**Tech Stack:** Go 1.26, Fosite, OAuth 2.1 draft-15, RFC 9728, MCP 2025-06-18 Streamable HTTP auth.
**Parallel Work Model:** Single shared-library patch. Downstream Skills/Vorrent adoption is separate so each consumer can update module versions and run its own integration gates.

---

## Current State (Verified)

Last verified: 2026-06-18

**Files examined:**
- `oauth/config.go` — default access token `1h`, refresh token `24h`.
- `oauth/provider.go` — Fosite receives configured lifetimes and composes refresh-token grant plus refresh rotation.
- `oauth/storage/storage.go` — refresh-token rotation invalidates old access and refresh token sessions.
- `oauth/middleware.go` — bearer challenges preserve `resource_metadata` but had no reusable configured scope hint.
- `oauth/metadata.go` and `oidc/discovery.go` — protected-resource metadata omitted RFC 9728 `resource_name`.
- Linear OAuth docs — user OAuth access tokens are valid for 24 hours and refresh responses return both a new access token and a new refresh token.

**Key findings:**
- The shared library is the correct owner for default lifetimes and challenge shape.
- Linear publishes access-token lifetime and rotation behavior, not a fixed refresh-token absolute lifetime. `mcp-kit` should match the 24-hour access token exactly and use a pragmatic 30-day refresh-token default with rotation.
- Scope hints must be available from `oauth.BearerConfig` so consumers do not need app-local wrappers around `/mcp`.
- `resource_name` should be available in protected-resource metadata helpers for better client display.

---

## Tasks

### Task 1: Linear-Style OAuth Defaults

**Files:**
- Modify: `oauth/config.go`
- Test: `oauth/config_test.go`

**Steps:**
1. Add failing defaults test.
2. Export `DefaultAccessTokenLifespan = 24h`.
3. Export `DefaultRefreshTokenLifespan = 30d`.
4. Use both in `Config.applyDefaults`.

### Task 2: Shared Bearer Scope Hints

**Files:**
- Modify: `oauth/middleware.go`
- Test: `oauth/middleware_test.go`

**Steps:**
1. Add failing tests for no-token and invalid-token challenges with scope hints.
2. Add `BearerConfig.RequiredScopes []string`.
3. Include `scope="..."` on 401 challenges while preserving `resource_metadata`.

### Task 3: Protected Resource Display Name

**Files:**
- Modify: `oauth/metadata.go`
- Modify: `oidc/discovery.go`
- Test: `oauth/metadata_test.go`
- Test: `oidc/discovery_test.go`

**Steps:**
1. Add failing tests for `resource_name`.
2. Add `ResourceName` fields to helper configs and metadata structs.
3. Preserve existing behavior when no resource name is configured.

---

## Validation

Run:

```bash
go test ./oauth ./oidc
go test ./...
go vet ./...
```

Expected:
- All tests pass.
- No vet issues.
- Downstream consumers can delete app-local scope wrappers after upgrading to the patched kit.

## Validation Evidence

Last verified: 2026-06-18

Commands:

```bash
go test ./...
go vet ./...
```

Results:
- All tests passed.
- `go vet ./...` passed.
