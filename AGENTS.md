# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

`mcp-kit` is a **reusable Go library** (module `github.com/haakco/mcp-kit`) for building Model Context Protocol (MCP) servers that consumers mount into their own Go services. It ships infrastructure — OAuth 2.1, key rotation, JSON-RPC envelope middleware, Origin allowlist, OIDC discovery, CLI auth, Ent schema mixins, a testkit — but **does not** ship domain tools, resources, prompts, user tables, RBAC, or audit storage. Three consumers drive the design: `skills-mcp`, `vorrent`, `meridian`.

Status is **pre-1.0**. Breaking changes between minor versions are allowed and must be documented in `CHANGELOG.md` with migration notes. Read [DESIGN.md](DESIGN.md) before changing public API.

## Common commands

This is a single Go module with no Makefile, justfile, or CI config — use the `go` toolchain directly. Go 1.26 (toolchain `go1.26.2`) is required.

```bash
# Build (compile-check) every package in the module.
go build ./...

# Run all unit tests in the module (excludes _examples and entschema/_test by Go convention).
go test ./...

# Run a single package's tests with verbose output.
go test -v ./oauth/...
go test -v ./mcpmw

# Run one test by name.
go test -run TestBearerRejectsAnonymous ./oauth

# Race detector + coverage for a package.
go test -race -cover ./oauth/keys

# The minimal example is a Go program in _examples/minimal-server/. The leading
# underscore makes Go skip it for ./... — invoke it directly.
go test ./_examples/minimal-server
go run ./_examples/minimal-server   # boots issuer at http://localhost:8080

# Static checks (no formal lint config — use go vet at minimum).
go vet ./...

# Module hygiene.
go mod tidy
```

The Ent test fixtures in `entschema/_test/` are generated Ent code used to verify mixin composition. They live behind the `_` prefix so they don't pollute `./...`. To regenerate after editing mixins:

```bash
cd entschema/_test && go run -mod=mod entgo.io/ent/cmd/ent generate ./schema
```

## High-level architecture

### The kit/consumer boundary

The kit owns transport, OAuth, key rotation, and middleware. The consumer owns:

- **Tools/resources/prompts** registered on a consumer-owned `mcp.Server` from `github.com/modelcontextprotocol/go-sdk`. The consumer passes the SDK's `http.Handler` into `mcpkit.New(...)`.
- **User store** — implements `userstore.Store` over the consumer's existing user table.
- **Audit emitter** — implements `audit.Emitter` over the consumer's audit pipeline.
- **Authz service** — implements `authz.Service` over the consumer's RBAC (kit ships `authz.AlwaysAllow()` for tests only).
- **OAuth subject resolver** — a `oauth.SubjectResolver` that authenticates the browser session at `/oauth/authorize` and decides which scopes to grant.
- **OAuth storage backend** — implements `oauth/storage.Store` (`oauth/storage` ships an in-memory `NewMemoryStore`; consumers typically back it with their database, often via the Ent mixins in `entschema/`).

`mcpkit.New` composes middleware in this order, outer to inner: `Origin → Bearer → Envelope → SDK handler`. Reorder this and you break the lessons in `docs/lessons.md` (origin denials must precede auth challenges; envelope rewriting must not see 401/403; SSE responses must pass through untouched).

### Package map (production code)

| Package | Role |
|---|---|
| `mcpkit/` | Top-level entry. `mcpkit.New(Config) → *Server` glues SDK handler + middleware. |
| `oauth/` | Fosite-backed OAuth 2.1 provider: authorize, token, revoke, dynamic register. Bearer middleware (OAuth + PAT). PKCE helpers. Login error classifier. |
| `oauth/keys/` | RS256 signing-key manager + rotator (90-day cadence, 48-hour grace). |
| `oauth/storage/` | Fosite storage adapter interface + in-memory impl. Consumers plug their DB in here. |
| `oidc/` | `/.well-known/oauth-authorization-server`, `/openid-configuration`, `/oauth-protected-resource`, `/jwks.json` handlers. |
| `mcpmw/` | Standalone middleware: `Origin`, `Envelope` (JSON-RPC error rewriter for the SDK's plain-text protocol errors). |
| `cliauth/` | CLI helper: PKCE + browser launch + loopback redirect listener + 0600 file-backed cred store. |
| `entschema/` | Opt-in Ent mixins for `oauth_client`, `oauth_signing_key`, `oauth_*_token`, `personal_access_token`. Composition-only; consumers add their own edges/timestamps. |
| `audit/`, `authz/`, `userstore/` | **Interfaces only**, plus tiny test helpers (`audit.Discard()`, `authz.AlwaysAllow()`). The kit never owns these tables. |
| `testkit/` | In-memory `Server`, deterministic `TokenValidator`, `RunHandshake`, `ListTools` — for both kit tests and downstream consumer tests. |

### Auth context model (read this before changing auth code)

Auth state lives on `context.Context`, not on an actor struct. The kit exposes typed accessors in `oauth/middleware.go`:

- `oauth.GetUserID(ctx)`, `GetScopes(ctx)`, `GetScopeType(ctx)`, `GetScopeTarget(ctx)`, `GetAuthSource(ctx)`.
- `oauth.WithAuthDisabled(ctx)` / `IsAuthDisabled(ctx)` — explicit sentinel for "auth is off at startup". **Never** infer "auth disabled" from `scopes == nil`; that conflates "auth off" with "request bypassed middleware" and has shipped privilege-escalation bugs (see `AG-03` in `docs/lessons.md`).
- `BearerConfig.AllowUnauthenticated` only takes effect when both `Introspector` and `TokenValidator` are nil.

PAT and OAuth bearer paths share the same context shape; `GetAuthSource(ctx)` distinguishes them (`AuthSourcePAT`, `AuthSourceOAuth2`). PATs may carry a `(scope_type, scope_target)` boundary; use `RequireScopeForTarget` rather than reimplementing the check.

## Working norms specific to this repo

- **Lessons drive design.** Before adding/removing middleware or changing OAuth behavior, scan `docs/lessons.md` for the relevant prefix (`OG-*` OAuth, `JR-*` JSON-RPC envelopes, `TQ-*` transport, `AG-*` authz, `CG-*` code quality, `EG-*` engineering gaps, `FP-*`/`PR-*` reusable probes/false positives). Lessons are reproducible; if you change behavior that one of them encodes, update the lesson in the same change.
- **Recipes for cross-cutting patterns.** `docs/recipes/admin-gate.md` is the canonical pattern for "same privileged mutation reachable from HTTP API and MCP." When adding a similar pattern, add a recipe rather than burying it in a tool handler.
- **Migration docs are contracts.** `docs/migration/{skills-mcp,vorrent,from-mark3labs}.md` are followed by real migrations. Changing the public API means updating the relevant migration doc in the same PR — otherwise downstream consumers drift.
- **The cycle methodology is the QA loop.** `docs/cycle-methodology.md` describes the phased E2E protocol consumers run against their kit-backed servers. Kit changes that affect transport, auth, or discovery should be re-verified with the bootstrap probe (`PR-02` in `docs/lessons.md`) on at least one downstream consumer before tagging a release.
- **`mcpkit.ErrNotImplemented` is a real return value.** Some symbols (notably parts of `mcpkit.Config`) are intentionally stubbed for future versions. Don't paper over them — implement properly or leave them as `ErrNotImplemented` and call them out in `CHANGELOG.md`.
- **Method-per-receiver cap (~12).** `CG-02` in `docs/lessons.md` describes the rationale: when a 13th method wants to land on `*Server` or `*Provider`, prefer a separate handler/embedder type over padding the receiver. Apply the cap in code review, not via lint config (none exists yet).
- **Plans live in `docs/plans/`.** The active plan is `docs/plans/2026-05-01_mcp-kit_master_plan.md`. New plans follow the template in `docs/plans/README.md` and are dated `YYYY-MM-DD_<topic>.md`.

## What not to do

- **Don't depend on `mark3labs/mcp-go`.** The kit is on the official `github.com/modelcontextprotocol/go-sdk` SDK. `docs/migration/from-mark3labs.md` exists to move servers off mark3labs, not back to it.
- **Don't ship login HTML or RBAC tables in the kit.** That decision is recorded in `DESIGN.md` (open question 2 + decision log entry 2026-05-01). Consumers render their own UI and own their own roles.
- **Don't `git stash` or `git reset` while working in this repo.** Per `~/.claude/CLAUDE.md`, those are forbidden across the org and especially dangerous on a shared library.
- **Don't add domain code (tools, resources, prompts, user models) to the kit.** It belongs in the consumer. If a pattern keeps recurring across consumers, document it as a recipe in `docs/recipes/` first; promote to code only after a third consumer needs it.
