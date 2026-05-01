# mcp-kit Master Implementation Plan

**Status:** Draft. Created 2026-05-01. v0.1.0 spike landed in commit `5aedbba`.
**Goal:** Take `mcp-kit` from v0.1.0 skeleton to v1.0.0 stable, with three Go consumers (skills-mcp, vorrent, meridian) all building and shipping against it; capture the universal MCP-server methodology in shared HaakCo skills so non-Go servers (Laravel) reuse the patterns.

**Background:** HaakCo today has three Go services that need or already run MCP servers (`vorrent`, `skills-mcp`, `meridian`). Each shipped or planned its own ~3000 lines of OAuth + middleware + discovery + key rotation, with two of the three diverging in security posture (vorrent has no key rotation; skills-mcp has no JSON-RPC envelope rewriter). Without a shared library, each new server in any future Go project repeats the work and the implementations drift further. This plan extracts the cross-cutting concerns into a reusable library at `github.com/haakco/mcp-kit`, migrates the three servers, and shipss v1.0.0 with a battle-tested API surface.

**Architecture:** Single Go module at `github.com/haakco/mcp-kit` providing OAuth 2.1 + PKCE server, signing-key rotator with grace window, JSON-RPC envelope rewriter, Origin allowlist, OIDC/OAuth discovery endpoints, JWKS endpoint, Personal Access Token validator, CLI auth helper, and Ent schema mixins. Consumers implement three small interfaces (`UserStore`, `AuditEmitter`, `AuthzService`) wrapping their existing tables; the kit returns `http.Handler`s that consumers mount in any HTTP framework (stdlib mux, Echo, Chi, Gin). Universal MCP patterns (cycle methodology, tool naming, lessons-learned IDs) live in shared skills so HaakCo's Laravel-based MCP work reuses them.

**Tech Stack:**
- Go 1.26+
- Official MCP SDK: `github.com/modelcontextprotocol/go-sdk` v1.5.0+
- Fosite v0.49+ for OAuth 2.1
- go-jose/v3 for JWS
- Ent v0.14+ for storage (default; pluggable)
- Standard library `net/http` for HTTP layer (framework-agnostic)
- HTTP framework: NONE in the kit; consumers wrap in their preferred framework

**Parallel Work Model:** Phase-by-phase rollout with strict ordering between phases. Within a phase, sub-tasks may run concurrently. Migration order is deliberately serial across the three consumers to validate the kit API against one real consumer before the next adopts it. All three consumer migrations may use Same-Branch Concurrent agents per HaakCo's CLAUDE.md (no `git stash`, no `git reset`, scope-limited file ownership).

---

## Reference Directories

This plan references concrete code in five locations. All paths are absolute on the local workstation.

### The library itself

- **Library root:** `/Users/timhaak/Dev/HaakCo/AiProjects/sharedLib/golang/mcp-kit/`
- **Git remote:** `git@github.com:haakco/mcp-kit.git` (https://github.com/haakco/mcp-kit)
- **Module path:** `github.com/haakco/mcp-kit`
- **Design doc:** `DESIGN.md` (in repo root)
- **This plan:** `docs/plans/2026-05-01_mcp-kit_master_plan.md`

### Consumer 1 — skills-mcp (largest migration)

- **Repo root:** `/Users/timhaak/Dev/HaakCo/AiProjects/skills/`
- **MCP server:** `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/`
- **OAuth/OIDC code (will donate to kit):**
  - `apps/skills-mcp/internal/oidc/` — `discovery.go`, `handlers.go`, `keys.go`, `provider.go`, `register.go`, `rotator.go`, `session_store.go`, `storage.go`, `form.go`
  - `apps/skills-mcp/internal/auth/` — `middleware.go`, `pat_validator.go`, `log_classify.go`, `verify.go`, `password.go`, `session.go`, `scope_context.go`
  - `apps/skills-mcp/internal/cliauth/` — `pkce.go`, `browser.go`, `login.go`, `login_http.go`, `credstore.go`
- **MCP server core:** `apps/skills-mcp/internal/server/server.go`, `registry.go`, `registry_*.go`
- **Ent schemas (informing kit's `entschema/` mixins):**
  - `apps/skills-mcp/ent/schema/oauthclient.go`
  - `apps/skills-mcp/ent/schema/oauthsigningkey.go`
  - `apps/skills-mcp/ent/schema/oauthsession.go`
  - `apps/skills-mcp/ent/schema/personal_access_token.go`
- **Stack:** Go 1.26.2, mark3labs/mcp-go v0.49.0, Fosite v0.49.0, Ent v0.14.6, Koanf v2.3, go-jose v3, urfave/cli v3
- **Special property:** Has the largest and cleanest OAuth surface — donates ~95% of the kit's `oauth/` and `oauth/keys/` packages.
- **Migration cost:** ~7 days (largest delta — also migrates SDK from mark3labs to official).

### Consumer 2 — vorrent

- **Repo root:** `/Volumes/Dev/HaakCo/AiProjects/vorrent/` (also symlinked at `/Users/timhaak/Dev/HaakCo/AiProjects/vorrent/`)
- **MCP server:** `/Volumes/Dev/HaakCo/AiProjects/vorrent/internal/mcpserver/`
- **MCP code (will donate to kit):**
  - `internal/mcpserver/jsonrpc_envelope.go` — already ported into kit's `mcpmw/envelope.go` in v0.1.0
  - `internal/mcpserver/origin.go` — already ported into kit's `mcpmw/origin.go` in v0.1.0
  - `internal/mcpserver/server.go`, `dynamic_bearer_auth.go`, `tools.go`, `resources.go`, `prompts.go`
- **OAuth code (will be replaced by kit's):**
  - `internal/api/mcp_oauth.go`, `mcp_oauth_register.go`
  - `internal/oauth/` (entire package)
- **Cycle 1 outputs (drive kit's docs):**
  - `docs/plans/mcp/lessons_learned.md` — TQ-01..03, OG-04..05, FP-01, EG-01..06
  - `docs/plans/mcp/dispatch_runbook.md` — phased curl recipes
  - `docs/plans/mcp/cycle_summary.md` — cycle 1 closeout
  - `docs/plans/mcp/sub-plans/SP-MCP-NN-*.md` — phase sub-plans
  - `docs/plans/mcp/cross_repo_recommendations.md` — the source of this plan
- **Stack:** Go 1.26.2, modelcontextprotocol/go-sdk v1.5.0, Fosite v0.49.0, Ent v0.14.6, Koanf v2.3, Wails v3 alpha
- **Special property:** Already on the official Go SDK + has cycle 1 lessons baked in. Donates the JSON-RPC envelope middleware + origin allowlist + cycle methodology to the kit.
- **Migration cost:** ~3 days (smallest delta).

### Consumer 3 — meridian (greenfield adoption)

- **Repo root:** `/Users/timhaak/Dev/mairin/meridian/`
- **Backend:** `/Users/timhaak/Dev/mairin/meridian/backend/`
- **MCP plan:** `/Users/timhaak/Dev/mairin/meridian/docs/plans/add_mcp.md` — to be rewritten in Phase 9 of this plan to depend on kit
- **Existing auth (kit must coexist with):**
  - `backend/internal/auth/jwt/` — JWT auth for GraphQL clients
  - `backend/internal/auth/middleware.go` — bearer extraction
  - `backend/internal/audit/` — existing audit logger
  - `backend/internal/authz/` — existing RBAC
- **Server entry:** `backend/cmd/api/main.go` → `backend/internal/server/server.go`
- **Stack:** Go 1.26.1, Echo v5, Ent v0.14.6, gqlgen v0.17.89, JWT v5, Koanf v2.3, Sentry, sqlite/postgres
- **Special property:** Greenfield — no existing OAuth code. Validates the kit's API surface against a consumer that has nothing to migrate from.
- **Migration cost:** ~4 days (greenfield adoption, healthcare-data sensitivity review adds time).

### Universal patterns home — HaakCo skills

- **Skills index:** `/Volumes/Dev/HaakCo/AiProjects/vorrent/.skills/skills-index.yaml` (per-project sync target)
- **Canonical skill source:** `/Users/timhaak/Dev/HaakCo/AiProjects/skills/skills/` (git-versioned skills)
- **Affected skills:**
  - `haakco-mcp-server-design` — production patterns for building MCP servers (Go + universal)
  - `haakco-mcp-plugins` — MCP client/runner reference (mostly universal)
- **Skills sync workflow:** From canonical repo run `just sync-pull <project>` after editing any skill.

---

## Current State (Verified)

**Files I opened to write this plan:**

- `/Volumes/Dev/HaakCo/AiProjects/vorrent/internal/mcpserver/jsonrpc_envelope.go` — confirmed envelope middleware is self-contained, no Vorrent-specific deps. Direct port to `mcp-kit/mcpmw/envelope.go` worked first try.
- `/Volumes/Dev/HaakCo/AiProjects/vorrent/go.mod` — confirmed Vorrent on `modelcontextprotocol/go-sdk v1.5.0`.
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/go.mod` — confirmed skills-mcp on `mark3labs/mcp-go v0.49.0` (different SDK; will migrate as part of kit adoption).
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/rotator.go` — full key-rotation impl with 90d/48h grace, audit-emit, clock injection, error-tolerant. Direct port candidate for `mcp-kit/oauth/keys/rotator.go`.
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/pat_validator.go` — interface-based PAT validator with async last_used_at update. Direct port to `mcp-kit/oauth/pat.go`.
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/log_classify.go` — fixed-vocabulary login error classifier. Direct port to `mcp-kit/oauth/login_classify.go`.
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/cliauth/pkce.go` — RFC 7636 conformant PKCE pair gen. Direct port.
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/server/server.go` — server bundle pattern + reflection-based registry snapshot. Selected pieces inform kit's `mcpkit/server.go`.
- `/Users/timhaak/Dev/mairin/meridian/backend/cmd/api/main.go` — confirmed Echo v5 wiring, identified injection points for kit (`server.New()`).
- `/Users/timhaak/Dev/mairin/meridian/backend/internal/auth/jwt/jwt.go` — confirmed JWT-only auth today; kit OAuth coexists at separate path (`/mcp-oauth/*`).

**v0.1.0 already shipped (commit `5aedbba`):**

- `mcp-kit/.gitignore`, `LICENSE` (MIT), `README.md`
- `mcp-kit/DESIGN.md` — full design with package layout, public API, migration paths
- `mcp-kit/go.mod` — module declaration, go 1.26
- `mcp-kit/mcpmw/envelope.go` + `_test.go` — JSON-RPC envelope rewriter ported from Vorrent (6 tests passing)
- `mcp-kit/mcpmw/origin.go` + `_test.go` — Origin allowlist with loopback fallback (6 tests passing)
- `mcp-kit/audit/emitter.go` — `Emitter` interface + `Discard()` helper
- `mcp-kit/userstore/store.go` — `User` + `Store` interfaces, `ErrNotFound`, `ErrInvalidCredentials`
- `mcp-kit/authz/authz.go` — `Service` interface, `ErrForbidden`, `AlwaysAllow()` for tests
- `mcp-kit/mcpkit/doc.go` + `server.go` — top-level `New()` + `Config` (returns `ErrNotImplemented` until v0.2.0)

**Build + test status:** `go build ./...` clean, `go test ./...` 9/9 passing.

---

## Migration order rationale

| Order | Consumer | Why |
|---|---|---|
| 1st | skills-mcp | Donates ~95% of OAuth code; migrates first because the kit IS skills-mcp's OAuth lifted out. Validates the API against the most complex existing consumer. Also forces the SDK migration (mark3labs → official) — most painful step done first. |
| 2nd | vorrent | Already on official SDK. Migration is mostly "swap our OAuth for kit's, delete our envelope middleware (now in kit)". Validates that the API works for a consumer that didn't donate the OAuth code. |
| 3rd | meridian | Greenfield. Validates that a consumer with zero existing OAuth code can adopt the kit cleanly without inheriting any technical debt. Also surfaces healthcare-data sensitivity requirements that may inform the kit's defaults. |

---

## Phases

Twelve phases. v0.1.0 (skeleton) is complete. Remaining phases ship as v0.2.0 → v1.0.0.

### Phase 1: Skeleton (v0.1.0) ✅ COMPLETE

**Files (already shipped in commit `5aedbba`):**
- `mcp-kit/{go.mod,DESIGN.md,README.md,LICENSE,.gitignore}`
- `mcp-kit/mcpmw/envelope.go` + tests (port from `vorrent/internal/mcpserver/jsonrpc_envelope.go`)
- `mcp-kit/mcpmw/origin.go` + tests
- `mcp-kit/{audit,userstore,authz,mcpkit}/` — interface stubs

**Status:** Done. Tagged `v0.1.0` in next phase.

### Phase 2: v0.1.0 release + cycle docs port

**Files (kit):**
- Create: `mcp-kit/CHANGELOG.md` — initial release notes
- Create: `mcp-kit/docs/cycle-methodology.md` — port from `vorrent/docs/plans/mcp/cycle_reset.md` + `dispatch_runbook.md` outline
- Create: `mcp-kit/docs/lessons.md` — port `vorrent/docs/plans/mcp/lessons_learned.md` (TQ-01..03, OG-04..05, FP-01, EG-01..06, PR-01..NN)
- Create: `mcp-kit/docs/dispatch-runbook-template.md` — empty template with all phases stubbed

**Source files in vorrent (read-only references):**
- `/Volumes/Dev/HaakCo/AiProjects/vorrent/docs/plans/mcp/lessons_learned.md`
- `/Volumes/Dev/HaakCo/AiProjects/vorrent/docs/plans/mcp/cycle_reset.md`
- `/Volumes/Dev/HaakCo/AiProjects/vorrent/docs/plans/mcp/dispatch_runbook.md`
- `/Volumes/Dev/HaakCo/AiProjects/vorrent/docs/plans/mcp/cycle_summary.md`

**Steps:**
1. Read each source file in vorrent. Strip Vorrent-specific tool names; keep universal patterns.
2. Author the three doc files in the kit.
3. Tag `v0.1.0` (the spike already shipped is the v0.1.0 surface).

**Verify:** `git tag --list | grep v0.1.0` — present. `cd mcp-kit && go test ./...` — 9/9 passing.

**Commit:** `docs: port cycle methodology + lessons learned from vorrent`

**Effort:** 1 day.

---

### Phase 3: OAuth core extraction (v0.2.0-pre)

**Files (kit, all created):**
- `mcp-kit/oauth/config.go` — `Config` struct, defaults
- `mcp-kit/oauth/provider.go` — `New()`, Fosite compose
- `mcp-kit/oauth/handlers.go` — `/authorize`, `/token`, `/register`, `/revoke`
- `mcp-kit/oauth/middleware.go` — `Bearer()` middleware
- `mcp-kit/oauth/token_validator.go` — `TokenValidator` interface + impl
- `mcp-kit/oauth/pat.go` — Personal Access Token validator
- `mcp-kit/oauth/login_classify.go` — fixed-vocabulary classifier
- `mcp-kit/oauth/pkce.go` — PKCE base64url helpers (substitution-correct)
- `mcp-kit/oauth/keys/manager.go` — RSA key gen, JWKS, rotation primitives
- `mcp-kit/oauth/keys/rotator.go` — background rotator
- `mcp-kit/oauth/storage/storage.go` — Fosite storage interface
- `mcp-kit/oauth/storage/ent.go` — default Ent-backed implementation
- `mcp-kit/oidc/discovery.go` — `/.well-known/oauth-authorization-server`
- `mcp-kit/oidc/openid.go` — `/.well-known/openid-configuration`
- `mcp-kit/oidc/protected_resource.go` — `/.well-known/oauth-protected-resource`
- `mcp-kit/oidc/jwks.go` — `/.well-known/jwks.json`
- `mcp-kit/entschema/oauth_client.go` — Ent mixin
- `mcp-kit/entschema/oauth_signing_key.go`, `oauth_authorization_code.go`, `oauth_access_token.go`, `oauth_refresh_token.go`, `personal_access_token.go`
- All `*_test.go` siblings

**Source files (skills-mcp, read-only — direct ports with package + import path edits):**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/discovery.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/handlers.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/keys.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/provider.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/register.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/rotator.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/storage.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/middleware.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/pat_validator.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/log_classify.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/verify.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/password.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/ent/schema/oauthclient.go` (informs `entschema/oauth_client.go`)
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/ent/schema/oauthsigningkey.go`

**Steps:**
1. **Sub-step 3.1: OAuth keys** — port `keys.go` + `rotator.go`. Tests must include rotation, grace window, retired-key cleanup, audit emit. Use the kit's `audit.Emitter` interface (not skills-mcp's concrete `service.AuditService`).
2. **Sub-step 3.2: Storage** — port `storage.go`. Extract Fosite storage interface; provide Ent-backed impl behind it. Tests roundtrip auth-code → access-token → refresh-token.
3. **Sub-step 3.3: Provider + handlers** — port `provider.go`, `handlers.go`, `register.go`. Use kit's `userstore.Store` interface for user lookup (not skills-mcp's concrete `service.UserService`).
4. **Sub-step 3.4: Bearer + PAT middleware** — port `middleware.go`, `pat_validator.go`, `verify.go`, `password.go`. Use kit's interfaces.
5. **Sub-step 3.5: Login classifier** — port `log_classify.go` verbatim (no consumer-specific dependencies).
6. **Sub-step 3.6: Discovery** — port `discovery.go` to `mcp-kit/oidc/discovery.go` + `openid.go`. **CRITICAL:** `authorization_servers` field MUST be the issuer URL (lesson from Vorrent ISSUE-002).
7. **Sub-step 3.7: Ent mixins** — convert each schema in `apps/skills-mcp/ent/schema/oauth*.go` to a mixin under `entschema/`. Tests via a fixture project that composes the mixin.
8. **Sub-step 3.8: PKCE helpers** — port `apps/skills-mcp/internal/cliauth/pkce.go` to `oauth/pkce.go`. Substitution-correct (`base64.RawURLEncoding`, lesson OG-04).
9. **Sub-step 3.9: Bind kit-level `mcpkit.New()` to the OAuth core.** Replace `ErrNotImplemented` with real composition: Origin → Bearer → Envelope → SDK handler.
10. **Sub-step 3.10: Reference example** — create `mcp-kit/_examples/minimal-server/` that boots kit + a single in-memory user + Discard audit + `mcp.read` scope tool, exposes `/mcp` + `/mcp-oauth/*` + `/.well-known/*`. Smoke-tests via shell script.

**Verify:**
- `cd mcp-kit && go test ./... -count=1` — all packages green.
- `cd mcp-kit/_examples/minimal-server && go run .` boots, `/healthz` 200.
- Run cycle 1 dispatch runbook P0–P3 against the example server. Token mints; `tools/list` returns the example tool.

**Commit:** `feat(oauth): extract OAuth core from skills-mcp`

**Tag:** `v0.2.0-rc.1` after example server passes the runbook.

**Effort:** 6 days.

---

### Phase 4: CLI auth helper (v0.2.0)

**Files (kit, all created):**
- `mcp-kit/cliauth/pkce.go` — PKCE pair gen (already mostly in `oauth/pkce.go`; CLI-specific helpers here)
- `mcp-kit/cliauth/browser.go` — Open system browser
- `mcp-kit/cliauth/login.go` — Loopback redirect listener + code exchange
- `mcp-kit/cliauth/login_http.go` — HTTP server for the redirect callback
- `mcp-kit/cliauth/credstore.go` — OS-appropriate credential storage
- All `*_test.go` siblings

**Source files (skills-mcp, direct ports):**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/cliauth/browser.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/cliauth/login.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/cliauth/login_http.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/cliauth/credstore.go`

**Steps:**
1. Port each file; adjust imports to kit packages.
2. Replace skills-mcp's hardcoded credstore path (`~/.skills-mcp/credentials.json`) with a configurable default keyed by issuer URL: `~/.config/mcp-kit/<sha256-of-issuer>/credentials.json`.
3. Add a `cliauth.LoginOptions` struct so consumers can override paths and browser-open behavior.

**Verify:** Boot the example server; run `go run ./_examples/minimal-server/cliauth-test` to mint a token via browser flow. Token persists; second run uses cached token.

**Commit:** `feat(cliauth): browser-based PKCE login flow`

**Tag:** `v0.2.0`

**Effort:** 1.5 days.

---

### Phase 5: Test kit (v0.2.1)

**Files (kit, all created):**
- `mcp-kit/testkit/server.go` — `NewServer(t)` spins up an in-memory mcp-kit server with a fake user store + discard audit
- `mcp-kit/testkit/token.go` — `MintToken(t, scopes...)` issues a test bearer token without going through the OAuth flow
- `mcp-kit/testkit/handshake.go` — `RunHandshake(t, server, token)` does the 3-step Streamable HTTP init; returns `Mcp-Session-Id`
- `mcp-kit/testkit/coverage.go` — `AssertChecklistCoverage(t, registered, checklist)` matches EG-01 from Vorrent's lessons

**Steps:**
1. Build the in-memory user store (single user, configurable scopes).
2. Build the token minter (signs with the kit's test key without going through `/authorize` + `/token`).
3. Build the handshake helper using the official Go SDK as a client.
4. Build the coverage assertion.

**Verify:** Update `_examples/minimal-server` tests to use `testkit/`; tests now run in <500ms.

**Commit:** `feat(testkit): test helpers for kit consumers`

**Tag:** `v0.2.1`

**Effort:** 2 days.

---

### Phase 6: Migrate skills-mcp to kit (v0.3.0 gate)

**Repo:** `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/`

**Files (skills-mcp, modified or deleted):**
- Delete: `apps/skills-mcp/internal/oidc/{discovery.go,handlers.go,keys.go,provider.go,register.go,rotator.go,session_store.go,storage.go}`
- Delete: `apps/skills-mcp/internal/auth/{middleware.go,pat_validator.go,log_classify.go,verify.go,password.go,session.go,scope_context.go}`
- Delete: `apps/skills-mcp/internal/cliauth/` (entire package)
- Create: `apps/skills-mcp/internal/kitwiring/userstore.go` — adapter from existing `service.UserService` to `userstore.Store`
- Create: `apps/skills-mcp/internal/kitwiring/audit.go` — adapter from existing `service.AuditService` to `audit.Emitter`
- Create: `apps/skills-mcp/internal/kitwiring/authz.go` — adapter from existing `service.AuthzService` to `authz.Service`
- Modify: `apps/skills-mcp/internal/server/server.go` — replace `mcpserver.NewMCPServer` (mark3labs) with `mcpkit.New` (official SDK)
- Modify: `apps/skills-mcp/internal/server/registry*.go` — migrate tool registration calls to official SDK syntax
- Modify: `apps/skills-mcp/cmd/skills-mcp/main.go` — wire kit at startup
- Modify: `apps/skills-mcp/ent/schema/oauthclient.go` etc — replace inline schema with kit mixin composition
- Modify: `apps/skills-mcp/go.mod` — add `github.com/haakco/mcp-kit`, remove `github.com/mark3labs/mcp-go`

**Steps:**
1. **Sub-step 6.1: Add kit dependency.** `go get github.com/haakco/mcp-kit@v0.2.1`. Don't delete anything yet.
2. **Sub-step 6.2: Build adapter layer.** `internal/kitwiring/{userstore,audit,authz}.go`. Three small adapters wrapping existing services. Tests prove the adapters satisfy the kit interfaces.
3. **Sub-step 6.3: Schema mixin migration.** For each `oauth*` and `personal_access_token` schema, compose the kit mixin instead of redeclaring fields. Run `go generate ./ent/...`. Migration must be empty diff — same DDL.
4. **Sub-step 6.4: Wire kit in main.** Construct `oauth.New()` → `mcpkit.New()` next to existing server bootstrap. Mount on a different path (`/mcp-v2`, `/mcp-oauth-v2`) for parallel testing.
5. **Sub-step 6.5: Migrate tool registration.** For each `registry_*.go`, replace mark3labs `mcp.NewTool(name, opts...)` with the official SDK's tool registration. Tests must continue to pass.
6. **Sub-step 6.6: Cut over.** Move `/mcp-v2` to `/mcp` and remove the legacy `/mcp` mount. Delete legacy OAuth code (the files listed above).
7. **Sub-step 6.7: Run skills-mcp's verify-mcp-clients suite.** Must pass against the kit-backed binary.
8. **Sub-step 6.8: Delete `internal/cliauth/`.** Replace `skills-mcp auth login` subcommand with a thin wrapper around `cliauth.Login()` from the kit.

**Verify:**
- `cd apps/skills-mcp && go test ./... -count=1` — green.
- `just verify-mcp-clients http://127.0.0.1:8892` — green (with token if auth enabled).
- Run cycle 1 dispatch runbook P0–P10 (adapted to skills-mcp's surface) against kit-backed binary.
- Diff `go.mod`: `mark3labs/mcp-go` gone; `mcp-kit` present.
- LOC delta: net deletion of ~3000 lines.

**Commit (in skills-mcp):** `refactor(mcp): adopt mcp-kit; remove vendored OAuth + middleware`

**Kit tag:** `v0.3.0` (after skills-mcp validation passes).

**Effort:** 7 days.

---

### Phase 7: Migrate vorrent to kit (v0.4.0 gate)

**Repo:** `/Volumes/Dev/HaakCo/AiProjects/vorrent/`

**Files (vorrent, modified or deleted):**
- Delete: `internal/api/mcp_oauth.go`, `mcp_oauth_register.go`
- Delete: `internal/oauth/` (entire package)
- Delete: `internal/mcpserver/jsonrpc_envelope.go` (now in kit's `mcpmw/envelope.go`)
- Delete: `internal/mcpserver/origin.go` (now in kit's `mcpmw/origin.go`)
- Delete: `internal/mcpserver/dynamic_bearer_auth.go` (now in kit's `oauth/middleware.go`)
- Create: `internal/kitwiring/userstore.go` — adapter from existing `internal/user.Service` to `userstore.Store`
- Create: `internal/kitwiring/audit.go` — adapter from existing audit emitter
- Create: `internal/kitwiring/authz.go` — adapter from existing permission service
- Modify: `internal/mcpserver/server.go` — replace direct SDK construction with `mcpkit.New()`
- Modify: `internal/api/http_server.go` — replace direct envelope/origin middleware with kit composition (already done by `mcpkit.New()`)
- Modify: `cmd/<entry>/main.go` (or `main.go`) — wire kit
- Modify: `ent/schema/` — replace inline OAuth schemas with kit mixins
- Modify: `go.mod`

**Steps:**
1. Add kit dep.
2. Build `internal/kitwiring/` adapters (~3 files, small).
3. Schema mixin migration (empty DDL diff required).
4. Replace `internal/oauth/` with `oauth.New()`.
5. Replace `internal/mcpserver/` core construction (`server.go`) with `mcpkit.New()`. Keep `tools.go`, `resources.go`, `prompts.go` — those are domain code.
6. Delete the now-replaced files.
7. Run cycle 2 (the deferred real-client gates from cycle 1) against kit-backed binary.

**Verify:**
- `go build ./...` clean.
- `go test ./... -count=1` green (24 unit tests in internal/mcpserver).
- Cycle 2 dispatch runbook P0–P10 green, including Inspector + Claude Code real-client gates.
- LOC delta: net deletion of ~1500 lines.
- Kit gains: key rotation + PAT (vorrent didn't have these before).

**Commit (in vorrent):** `refactor(mcp): adopt mcp-kit; remove vendored OAuth + middleware`

**Kit tag:** `v0.4.0`.

**Effort:** 3 days.

---

### Phase 8: Update meridian add_mcp.md (assumes kit exists)

**Repo:** `/Users/timhaak/Dev/mairin/meridian/`

**Files (meridian, modified):**
- Modify: `docs/plans/add_mcp.md` — rewrite phases 1–6 to depend on `mcp-kit@v0.4.0` instead of "port from skills-mcp" / "port from vorrent". Phase 7 (production hardening) unchanged.

**Steps:**
1. Read current `add_mcp.md`.
2. Collapse phases:
   - Old Phase 2 (OAuth core) → "Adopt kit"
   - Old Phase 3 (MCP server core) → "Wire kit into Echo"
   - Old Phase 4 (Tools) → unchanged (consumer's domain)
   - Old Phase 5 (CLI auth helper) → "Use kit's `cliauth` package"
   - Old Phase 6 (E2E test cycle) → "Apply kit's `docs/cycle-methodology.md`"
3. Update tech stack section: add `github.com/haakco/mcp-kit v0.4.0+`.
4. Update healthcare-data sensitivity checklist: reference kit's interfaces (`UserStore`, `AuditEmitter`, `AuthzService`) for where consumer integrates.
5. Effort: 7d → ~4d.

**Verify:** Plan reviewable in one sitting; no "port from X" language remains.

**Commit (in meridian):** `docs(plans): rewrite add_mcp.md to adopt mcp-kit`

**Effort:** 0.5 days.

---

### Phase 9: meridian implementation (v1.0.0 gate)

**Repo:** `/Users/timhaak/Dev/mairin/meridian/`

Per the rewritten add_mcp.md from Phase 8.

**Files (meridian, all created):**
- Ent schemas via kit mixins (`backend/ent/schema/oauth*.go`, `personal_access_token.go`)
- `backend/internal/kitwiring/userstore.go`
- `backend/internal/kitwiring/audit.go`
- `backend/internal/kitwiring/authz.go`
- `backend/internal/mcp/server.go` — wires kit into meridian
- `backend/internal/mcp/tools_reference.go` — `search_medications`, `get_medication`, `search_icd10`, `get_icd10`
- `backend/internal/mcp/tools_patient_summary.go` — `get_my_appointments`, `get_my_profile`
- `backend/internal/mcp/tools_audit.go` — `summarize_audit_events`
- `backend/internal/mcp/resources.go`, `prompts.go`
- All `*_test.go` siblings

**Modified:**
- `backend/cmd/api/main.go` — construct kit + register tools
- `backend/internal/server/server.go` — mount kit handler at `/mcp` via `e.Any(echo.WrapHandler(...))`

**Steps:** Per add_mcp.md phases 1–7 (rewritten). Healthcare sensitivity checklist gates Phase 4 commit.

**Verify:**
- `task test` green (existing 100% + new MCP tests).
- `task lint` zero warnings.
- Compliance review sign-off recorded.
- Run kit's cycle methodology against meridian's MCP surface (P0–P10).
- Inspector + Claude Code real-client gates green.

**Commit (in meridian):** `feat(mcp): MCP server with OAuth 2.1 via mcp-kit`

**Kit tag:** `v1.0.0` after meridian validates the kit's API survives a third consumer with no further breaking changes.

**Effort:** 4 days.

---

### Phase 10: Update HaakCo skills with universal patterns

**Files (canonical skills repo):**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/skills/versions/haakco-mcp-server-design/000N/` — bump version, add Go-specific section + universal section
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/skills/versions/haakco-mcp-plugins/000N/` — bump version, add reference to mcp-kit as canonical Go server foundation

**Steps:**
1. From canonical skills repo: `just sync-pull <project>` to pull current state.
2. Add to `haakco-mcp-server-design/overview.md`:
   - "Go servers: use `github.com/haakco/mcp-kit`" subsection with kit ref
   - "Other servers (Laravel, Node): apply universal patterns below" subsection
3. Add to `haakco-mcp-server-design/workflow.md`:
   - Cycle methodology phases (universal)
   - Lessons-learned IDs (TQ-, OG-, FP-, PR-, EG-) and what each category means
   - "Rebuild before filing SDK bugs" (universal)
   - "PKCE base64url substitution" (universal)
   - "OAuth `authorization_servers` MUST be issuer URL" (universal)
   - "Streamable HTTP 3-step handshake" (universal)
4. Add to `haakco-mcp-server-design/validation.md`:
   - Checklist items that apply across languages
5. Add to `haakco-mcp-plugins/overview.md`:
   - "Building a new Go MCP server: see mcp-kit"
   - Keep client/runner content unchanged
6. Bump versions, lastUpdatedAt; push from canonical: `just sync-push <project>`.
7. From a project (e.g. vorrent): `just sync-pull` to confirm new versions land.

**Verify:** `cat /Volumes/Dev/HaakCo/AiProjects/vorrent/.skills/haakco-mcp-server-design/metadata.yaml` shows new version + lastUpdatedAt.

**Commit (in canonical skills):** `chore(skills): mcp-kit references + universal patterns from cycle 1`

**Effort:** 1 day.

---

### Phase 11: Migration documentation (v1.0.x)

**Files (kit, all created):**
- `mcp-kit/docs/migration/skills-mcp.md` — captures Phase 6 actually-executed steps + gotchas
- `mcp-kit/docs/migration/vorrent.md` — captures Phase 7
- `mcp-kit/docs/migration/new-server.md` — generic guide based on meridian Phase 9
- `mcp-kit/docs/migration/from-mark3labs.md` — extracted SDK migration steps from Phase 6

**Steps:**
1. After each migration phase (6, 7, 9), capture friction + fixes in real-time. Don't write these docs from theory — write them from actual completion.
2. Cross-link from `README.md` and `DESIGN.md`.

**Verify:** Each doc is reviewable in 15 minutes by someone unfamiliar with the kit.

**Commit:** `docs: migration guides for new and existing MCP server consumers`

**Tag:** `v1.0.1` (docs-only patch).

**Effort:** 1 day.

---

### Phase 12: Public release + maintenance posture

**Files:**
- `mcp-kit/CONTRIBUTING.md` — how to propose API changes, where breaks are tolerated, where they aren't
- `mcp-kit/SECURITY.md` — vulnerability disclosure, security-fix release cadence
- `mcp-kit/.github/workflows/ci.yml` — lint + test on push + PR

**Steps:**
1. Write CONTRIBUTING + SECURITY.
2. Set up CI — golangci-lint, `go test ./... -race -count=1`, vulncheck.
3. Tag `v1.0.0` on the commit where all three consumers are green.
4. Announce internally; offer cross-pollination support to any future Go MCP server.

**Verify:** GitHub Actions pipeline green on a fresh PR. SemVer commitment in README.

**Commit:** `chore: CI + contributing + security policy`

**Tag:** `v1.0.0`.

**Effort:** 1 day.

---

## Total effort summary

| Phase | Effort | Cumulative |
|---|---|---|
| 1. Skeleton (v0.1.0) | ✅ 1 day (done) | 1 |
| 2. v0.1.0 release + cycle docs | 1 | 2 |
| 3. OAuth core extraction | 6 | 8 |
| 4. CLI auth helper | 1.5 | 9.5 |
| 5. Test kit | 2 | 11.5 |
| 6. skills-mcp migration | 7 | 18.5 |
| 7. vorrent migration | 3 | 21.5 |
| 8. meridian plan rewrite | 0.5 | 22 |
| 9. meridian implementation | 4 | 26 |
| 10. Skills update | 1 | 27 |
| 11. Migration docs | 1 | 28 |
| 12. Public release + CI | 1 | 29 |
| **Total** | | **~29 working days** |

Allowing for review cycles, integration debugging, and 50% slack: **~6 calendar weeks** if executed serially by one engineer. **~3 calendar weeks** if Phase 6 (skills-mcp) and Phase 11 (migration docs from real experience) are partially parallelized after Phase 5 ships v0.2.1.

---

## Risks + mitigations

| Risk | Mitigation |
|---|---|
| Kit API needs breaking changes during Phase 6 (skills-mcp migration) | Tag pre-release versions (`v0.2.0-rc.1`); only tag `v0.2.0` after Phase 5 example server passes runbook. Kit is pre-1.0 until Phase 9 — breaks tolerated. |
| skills-mcp's mark3labs → official SDK migration introduces tool-shape drift | Run `verify-mcp-clients` against both legacy and kit-backed binaries during Phase 6.4 cutover. Don't delete legacy until parity confirmed. |
| Vorrent's cycle 2 surfaces a bug in the kit | Phase 7 includes cycle 2 dispatch runbook execution; any bug is fixed in the kit and re-validated before Phase 7 closes. |
| Meridian's healthcare sensitivity surfaces requirements not in the kit | Phase 9 may require kit changes (e.g. tighter audit hooks, mandatory IP allowlist mode). Kit is pre-1.0 through Phase 8 — breaks still tolerated. |
| Ent regen produces non-empty migration diff in Phase 6.3 / 7 | Compare DDL pre/post mixin migration. If diff is non-empty, mixins are wrong; fix kit before consumer. |
| Three consumers diverge on what they need from the kit | Coordinator owns the API. Sub-team output must integrate cleanly. If team A needs a feature team B doesn't, it goes through the kit's coordinator before landing. |

---

## Rollback per phase

Every phase has a clean rollback path:

| Phase | Rollback |
|---|---|
| 1–5 (kit-only) | Revert tag; the kit doesn't ship yet, no consumer is affected |
| 6 (skills-mcp migration) | Revert the cutover commit; legacy `/mcp` was kept until 6.6 explicitly removed it |
| 7 (vorrent migration) | Revert the migration commit; vorrent's `internal/oauth/` and `internal/mcpserver/` files exist in git history |
| 8 (meridian plan rewrite) | Restore from git; no code changes |
| 9 (meridian implementation) | Disable `MCPEnabled` config flag (default off in prod per the plan) |
| 10–12 (docs + CI) | Revert; no functional impact |

---

## Cycle methodology to apply

Every consumer migration (Phases 6, 7, 9) MUST run the cycle methodology against the kit-backed binary before the migration is considered done:

1. **P0** Bootstrap: mint a token via the kit's OAuth flow (curl recipe per `dispatch-runbook-template.md`).
2. **P1** Initialize handshake (TQ-02 verified).
3. **P2** OAuth deep coverage (refresh rotation, error envelopes per OG-01..05).
4. **P3** Auth + origin + scope.
5. **P4–P9** Tool / resource / prompt phases (consumer-specific).
6. **P10** Envelope conformance + instructions accuracy.

Real-client gates (Inspector for P1/P8/P9; Claude Code for P4/P10) are **mandatory before tagging `v0.4.0` and `v1.0.0`** — they may be deferred during pre-1.0 work but must close out before stable release.

The runbook template ships in Phase 2 (`mcp-kit/docs/dispatch-runbook-template.md`); each consumer copies + customizes for its tool surface.

---

## Universal vs Go-specific split

To support HaakCo's non-Go MCP work (Laravel servers, etc.), this plan deliberately separates:

| Goes in `mcp-kit/` (Go-only) | Goes in shared skills (universal) |
|---|---|
| All Go code | Cycle methodology (phases, real-client gates, audit tiers) |
| Fosite-based OAuth provider | Lessons-learned ID scheme (TQ-, OG-, FP-, PR-, EG-) |
| Ent schema mixins | Tool naming conventions (action_object, snake_case) |
| Go SDK adapter | Stable response contract (`{success, message, nextAction}`) |
| Bearer middleware | OAuth `authorization_servers` MUST be issuer URL |
| Key rotation goroutine | Streamable HTTP 3-step handshake |
| | PKCE base64url substitution |
| | Rebuild-before-filing-SDK-bugs (FP-01) |
| | OAuth signing-key rotation cadence (90d/48h) — concept, not impl |

Phase 10 lands the universal content into `haakco-mcp-server-design` and `haakco-mcp-plugins` skills. Laravel servers reference the skills, not the kit.

---

## Completion checklist (v1.0.0)

Per `~/.claude/CLAUDE.md` Completion Checklist + MCP-specific:

- [ ] **Readable** — new Go developer can wire kit into a fresh server in under 30 minutes following `docs/migration/new-server.md`.
- [ ] **Linted** — `golangci-lint run ./...` zero warnings on kit.
- [ ] **Tested** — `go test ./... -race -count=1` green on kit + all three consumers.
- [ ] **All problems fixed** — no broken state in any consumer; cycle 2 real-client gates closed for all three.
- [ ] **Consistent** — kit follows official SDK conventions; consumers consume kit identically through `kitwiring/` adapter pattern.
- [ ] **Simple** — no abstractions for hypothetical fourth consumer; storage interface only because two consumers were already on Ent.
- [ ] **Named well** — public API names are stable (`mcpkit.New`, `oauth.New`, `oidc.RegisterRoutes`); no `MgrV2Final` or similar.
- [ ] **No hacks** — no `MCPGODEBUG=jsonescaping=1` anywhere; FP-01 disproof recipe passes against all three consumers.
- [ ] **Documented** — DESIGN.md current, migration docs reflect actual experience, skills updated, README's status table accurate.
- [ ] **System working** — three consumers in production (or staging-ready) on `mcp-kit@v1.0.0`. Inspector + Claude Code connect to all three.
- [ ] **Public release** — repo public on GitHub, MIT licensed, CI green, SECURITY.md present.

---

## Out of scope (tracked for v1.x or v2)

| Item | Why deferred | Target version |
|---|---|---|
| Multi-tenancy / tenant-scoping interface | None of the three consumers needs it today | v1.x when meridian's multitenancy phase 2 lands |
| HTML login pages | Each consumer renders its own UI | Maybe never; consumers retain control |
| Non-Ent storage adapter | YAGNI until a fourth consumer demands it | v1.x on real demand |
| Client-credentials grant | Limited demand; security review needed | v1.x |
| Subscriptions / streaming tool results | SDK supports it; no consumer needs it | TBD |
| Laravel/Node companion library | Different language ecosystems; universal patterns in skills suffice for now | v2 if cross-language code reuse becomes valuable |
| MCP server SDK migration helpers | mark3labs → official is a one-time event | Not a kit concern after Phase 6 |

---

## References

- **MCP spec:** https://spec.modelcontextprotocol.io/specification/
- **Go MCP SDK:** https://github.com/modelcontextprotocol/go-sdk
- **Fosite:** https://github.com/ory/fosite
- **OAuth 2.1 draft:** https://datatracker.ietf.org/doc/draft-ietf-oauth-v2-1/
- **Reference servers:**
  - skills-mcp: `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/`
  - vorrent: `/Volumes/Dev/HaakCo/AiProjects/vorrent/internal/mcpserver/`
  - meridian (planned): `/Users/timhaak/Dev/mairin/meridian/backend/internal/mcp/`
- **Vorrent cycle 1 outputs:** `/Volumes/Dev/HaakCo/AiProjects/vorrent/docs/plans/mcp/`
- **Cross-pollination analysis:** `/Volumes/Dev/HaakCo/AiProjects/vorrent/docs/plans/mcp/cross_repo_recommendations.md`
- **Meridian add_mcp.md (pre-rewrite):** `/Users/timhaak/Dev/mairin/meridian/docs/plans/add_mcp.md`
- **Skills repo (canonical):** `/Users/timhaak/Dev/HaakCo/AiProjects/skills/skills/`
