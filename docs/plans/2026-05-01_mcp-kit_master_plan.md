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

## How to use this plan

1. **Read once cover-to-cover** to understand the kit's scope and the consumer migration order.
2. **Pick the next unstarted phase** from the Phase Map below.
3. **Open that phase's section** for files, source paths, sub-steps, and verify commands.
4. **Execute the steps verbatim** — verify commands are copy-paste ready.
5. **Tick the phase's verify checklist** before tagging the phase done.
6. **Update this plan** with the actual completion date and link the commit SHA in the phase's "Status" line.
7. **Move to the next phase** only after all verify items pass.

If a phase fails verification, do NOT advance. Stop, fix the underlying issue (which may be in the kit or in the consumer), and re-run verification.

---

## Phase Map (dependency graph)

```
Phase 1 (Skeleton) ✅ DONE
   │
   ▼
Phase 2 (v0.1.0 release + cycle docs port)
   │
   ▼
Phase 3 (OAuth core extraction)  ◄─── largest phase (6d)
   │
   ▼
Phase 4 (CLI auth helper)
   │
   ▼
Phase 5 (Test kit)  ◄─── tags v0.2.1; gate before consumer migrations
   │
   ▼
Phase 6 (skills-mcp migration)  ◄─── largest migration (7d, donates OAuth)
   │
   ▼
Phase 7 (vorrent migration)
   │
   ▼
Phase 8 (meridian plan rewrite — docs only)
   │
   ▼
Phase 9 (meridian implementation)  ◄─── tags v1.0.0
   │
   ├──► Phase 10 (Skills update — independent, can run in parallel with Phase 9)
   │
   ▼
Phase 11 (Migration docs)
   │
   ▼
Phase 12 (Public release + CI)  ◄─── final
```

**Critical-path phases** (block everything downstream): 1, 2, 3, 4, 5, 6, 7, 9, 12.
**Non-blocking phases** (can land anytime after their input): 8 (anytime after 7), 10 (anytime after 1), 11 (anytime after 9).

Phase 6 is the gate that proves the kit's API survives a real consumer. If Phase 6 surfaces breaking changes, those must land in the kit before Phase 7 starts.

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

**Detailed sub-steps:**

#### Sub-step 3.1: OAuth signing keys (1 day)

**Files (kit, create):**
- `mcp-kit/oauth/keys/manager.go` — RSA key gen, JWKS encoding, persistence
- `mcp-kit/oauth/keys/rotator.go` — background rotator goroutine
- `mcp-kit/oauth/keys/manager_test.go`, `rotator_test.go`

**Direct ports from:**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/keys.go` → `manager.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/rotator.go` → `rotator.go`

**Substitutions during port:**
- `service.AuditService` → `audit.Emitter` (kit interface)
- `auditpayload.KeyRotated{NewKID: ...}` → `audit.Event{EntityType: "oauth_key", Action: "rotated", EntityID: kid}`
- Imports: `github.com/haakco/skills-mcp/...` → `github.com/haakco/mcp-kit/...`

**Tests required (4 minimum, copied from skills-mcp's `rotator_test.go`):**
- `TestEnsureSigningKey_creates_when_absent`
- `TestRotateSigningKey_marks_prior_retired`
- `TestActiveJWKSet_includes_retired_within_grace`
- `TestRetireExpiredKeys_deletes_past_grace`

**Verify:**
```bash
cd /Users/timhaak/Dev/HaakCo/AiProjects/sharedLib/golang/mcp-kit
go test ./oauth/keys/... -count=1 -race
```
Expected: PASS, all 4+ tests green.

**Commit:** `feat(oauth/keys): RSA key manager + 90d/48h grace rotator`

#### Sub-step 3.2: Fosite storage (1 day)

**Files (kit, create):**
- `mcp-kit/oauth/storage/storage.go` — Fosite storage interface composition
- `mcp-kit/oauth/storage/ent.go` — Ent-backed implementation (default)
- `mcp-kit/oauth/storage/storage_test.go` — interface compliance tests
- `mcp-kit/oauth/storage/ent_test.go` — roundtrip tests

**Direct port from:**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/storage.go` → `ent.go`

**Substitutions:**
- All `*ent.Client` references stay (kit defaults to Ent)
- Schema-specific entity types (`ent.OauthClient` etc.) → consumer's Ent client; kit's `entschema/` mixins guarantee field shape
- Tests use in-memory SQLite Ent client

**Tests required (3 minimum):**
- `TestStorage_AuthCodeRoundtrip` — store → retrieve → invalidate
- `TestStorage_AccessTokenRoundtrip`
- `TestStorage_RefreshTokenRotation` — store → revoke → replacement issued

**Verify:**
```bash
cd /Users/timhaak/Dev/HaakCo/AiProjects/sharedLib/golang/mcp-kit
go test ./oauth/storage/... -count=1
```

**Commit:** `feat(oauth/storage): Fosite storage interface + Ent adapter`

#### Sub-step 3.3: OAuth provider + handlers (1.5 days)

**Files (kit, create):**
- `mcp-kit/oauth/config.go` — `Config` struct + `applyDefaults()`
- `mcp-kit/oauth/provider.go` — `New()`, Fosite compose
- `mcp-kit/oauth/handlers.go` — `/authorize`, `/token`, `/register`, `/revoke`
- `mcp-kit/oauth/handlers_test.go` — full OAuth flow integration

**Direct ports from:**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/provider.go` → `provider.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/handlers.go` → `handlers.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/register.go` → folded into `handlers.go`

**Substitutions:**
- `service.UserService.FindByEmail(...)` → `userstore.Store.FindByEmail(...)`
- `service.UserService.VerifyPassword(...)` → `userstore.VerifyPassword(...)` helper
- HTML login template stays in skills-mcp; kit's `/authorize` returns JSON-only (per Open Question 2 in DESIGN.md)
- Audience defaults to canonical `<issuer>/mcp` (lesson from Vorrent ISSUE-002)

**Tests required (8 minimum, mirror skills-mcp's `oauth_flow_test.go`):**
- `TestRegister_PublicClient` — POST `/register`, expect 201 + client_id
- `TestAuthorize_HappyPath` — full code flow with PKCE
- `TestAuthorize_RejectsBadState` — state < 8 chars (lesson OG-05)
- `TestAuthorize_RejectsBadPKCE` — invalid code_challenge
- `TestToken_ExchangesCode`
- `TestToken_RefreshRotation` — old refresh revoked after use
- `TestRevoke_IdempotentSuccess`
- `TestEnvelope_InvalidGrantOnPKCEFailure` — error envelope shape

**Verify:**
```bash
cd /Users/timhaak/Dev/HaakCo/AiProjects/sharedLib/golang/mcp-kit
go test ./oauth/ -count=1
```

**Commit:** `feat(oauth): provider + handlers extracted from skills-mcp`

#### Sub-step 3.4: Bearer + PAT middleware (1 day)

**Files (kit, create):**
- `mcp-kit/oauth/middleware.go` — `Bearer()` middleware
- `mcp-kit/oauth/token_validator.go` — `TokenValidator` interface + impl
- `mcp-kit/oauth/pat.go` — PAT validator
- `mcp-kit/oauth/verify.go` — bcrypt password verify helper
- All `*_test.go`

**Direct ports from:**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/middleware.go` → `middleware.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/pat_validator.go` → `pat.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/verify.go` → `verify.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/password.go` → folded into `verify.go`

**Substitutions:**
- `service.TokenService` → `pat.Validator` interface (kit-defined)
- `auth.PATServicer` → already an interface; rename to `pat.Servicer`
- Error responses use kit's canonical envelope helper

**Tests required (5 minimum):**
- `TestBearer_AcceptsValidJWT`
- `TestBearer_AcceptsValidPAT`
- `TestBearer_Rejects401WithWWWAuthenticate` — header set on 401
- `TestBearer_RejectsInsufficientScope`
- `TestPAT_AsyncLastUsedUpdate`

**Verify:**
```bash
go test ./oauth/ -run TestBearer -count=1
go test ./oauth/ -run TestPAT -count=1
```

**Commit:** `feat(oauth): bearer + PAT middleware`

#### Sub-step 3.5: Login classifier (2 hours)

**Files (kit, create):**
- `mcp-kit/oauth/login_classify.go` — fixed-vocabulary classifier
- `mcp-kit/oauth/login_classify_test.go`

**Direct port from:**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/auth/log_classify.go`

**Substitution:** `ent.IsNotFound(err)` → `errors.Is(err, userstore.ErrNotFound)`.

**Verify:**
```bash
go test ./oauth/ -run TestClassify -count=1
```

**Commit:** `feat(oauth): fixed-vocabulary login error classifier`

#### Sub-step 3.6: Discovery + JWKS (0.5 day)

**Files (kit, create):**
- `mcp-kit/oidc/discovery.go` — `/.well-known/oauth-authorization-server`
- `mcp-kit/oidc/openid.go` — `/.well-known/openid-configuration`
- `mcp-kit/oidc/protected_resource.go` — `/.well-known/oauth-protected-resource`
- `mcp-kit/oidc/jwks.go` — `/.well-known/jwks.json`
- `mcp-kit/oidc/discovery_test.go`

**Direct port from:**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/oidc/discovery.go`

**CRITICAL** (lesson from Vorrent ISSUE-002):
The `authorization_servers` field in `/.well-known/oauth-protected-resource` MUST be the issuer URL, NOT the metadata URL. Test asserts this exactly.

**Tests required:**
- `TestDiscovery_AuthorizationServersIsIssuerURL` — regression for ISSUE-002
- `TestDiscovery_AllAdvertisedScopesPresent`
- `TestJWKS_IncludesActiveAndRetiredKeys` — within grace window

**Verify:**
```bash
go test ./oidc/ -count=1
```

**Commit:** `feat(oidc): discovery + JWKS endpoints`

#### Sub-step 3.7: Ent schema mixins (0.5 day)

**Files (kit, create):**
- `mcp-kit/entschema/oauth_client.go`
- `mcp-kit/entschema/oauth_signing_key.go`
- `mcp-kit/entschema/oauth_authorization_code.go`
- `mcp-kit/entschema/oauth_access_token.go`
- `mcp-kit/entschema/oauth_refresh_token.go`
- `mcp-kit/entschema/personal_access_token.go`
- `mcp-kit/entschema/README.md` — composition guide for consumers
- `mcp-kit/entschema/_test/` — fixture Ent schema that composes mixins; `go generate` produces a runnable client; tests assert field shape

**Source schemas:**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/ent/schema/oauthclient.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/ent/schema/oauthsigningkey.go`
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/ent/schema/oauthsession.go` (split into auth_code + access_token + refresh_token)
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/ent/schema/personal_access_token.go`

**Conversion pattern:**
```go
// Before (skills-mcp inline schema):
type OAuthClient struct{ ent.Schema }
func (OAuthClient) Fields() []ent.Field { return []ent.Field{...} }

// After (kit mixin):
package entschema
type OAuthClient struct{ mixin.Schema }
func (OAuthClient) Fields() []ent.Field { return []ent.Field{...} }

// Consumer composes:
package schema
type OAuthClient struct{ ent.Schema }
func (OAuthClient) Mixin() []ent.Mixin { return []ent.Mixin{kitschema.OAuthClient{}} }
```

**Verify:**
```bash
cd mcp-kit/entschema/_test
go generate ./...
go test ./...
```

**Commit:** `feat(entschema): Ent mixins for OAuth + PAT tables`

#### Sub-step 3.8: PKCE helpers (2 hours)

**Files (kit, create):**
- `mcp-kit/oauth/pkce.go` — verifier/challenge generation
- `mcp-kit/oauth/pkce_test.go`

**Direct port from:**
- `/Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp/internal/cliauth/pkce.go`

**CRITICAL** (lesson OG-04): Use `base64.RawURLEncoding` (substitution + no padding). Test asserts no `+`, `/`, or `=` characters in output.

**Verify:**
```bash
go test ./oauth/ -run TestPKCE -count=1
```

**Commit:** `feat(oauth): PKCE helpers (substitution-correct base64url)`

#### Sub-step 3.9: Bind kit `mcpkit.New()` (0.5 day)

**Files (kit, modify):**
- `mcp-kit/mcpkit/server.go` — replace `ErrNotImplemented` with real composition
- `mcp-kit/mcpkit/server_test.go` — integration test of composed handler

**Composition order (outer to inner):**
1. `mcpmw.Origin` (already exists)
2. `oauth.Bearer` (built in 3.4)
3. `mcpmw.Envelope` (already exists)
4. SDK MCP handler

**Verify:**
```bash
cd mcp-kit
go test ./mcpkit/ -count=1
```

**Commit:** `feat(mcpkit): compose middleware stack with OAuth + envelope + origin`

#### Sub-step 3.10: Reference example (0.5 day)

**Files (kit, create):**
- `mcp-kit/_examples/minimal-server/main.go` — full boot in <200 lines
- `mcp-kit/_examples/minimal-server/go.mod` (separate module to avoid forcing kit consumers to depend on example deps)
- `mcp-kit/_examples/minimal-server/users.go` — in-memory `userstore.Store` with one user (admin@example.com / `admin`)
- `mcp-kit/_examples/minimal-server/tools.go` — single `hello_world` tool
- `mcp-kit/_examples/minimal-server/scripts/smoke.sh` — full P0–P3 dispatch runbook against this server

**Verify:**
```bash
cd mcp-kit/_examples/minimal-server
go run . &              # background
sleep 2
./scripts/smoke.sh      # mints token, runs handshake, calls tools/list
kill %1
```
Expected: smoke.sh prints `PASS` for all phases.

**Commit:** `feat(examples): minimal-server reference + smoke runbook`

**Phase verify (all sub-steps):**
- `cd mcp-kit && go test ./... -count=1 -race` — all green.
- `cd mcp-kit/_examples/minimal-server && go run .` boots; `curl http://localhost:8080/.well-known/oauth-authorization-server` returns valid JSON; PKCE flow mints token; `tools/list` returns `[hello_world]`.
- `golangci-lint run ./...` zero warnings.

**Tag:** `v0.2.0-rc.1` after example server passes the runbook. Final `v0.2.0` after at least one external review of the API surface.

**Phase commit:** `feat(oauth): extract OAuth core from skills-mcp` (squash of sub-steps 3.1–3.10).

**Effort:** 6 days total across 10 sub-steps.

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

**Pre-flight checks:**

Before starting Phase 6, confirm:
- `mcp-kit` tagged `v0.2.1` and example server passing.
- `_examples/minimal-server/scripts/smoke.sh` green.
- skills-mcp's existing `verify-mcp-clients` test suite is green on `main` (baseline).
- skills-mcp branch created: `git checkout -b refactor/adopt-mcp-kit`.

**Detailed sub-steps:**

#### Sub-step 6.1: Add kit dependency (2 hours)

```bash
cd /Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp
go get github.com/haakco/mcp-kit@v0.2.1
go mod tidy
```

**Verify:**
```bash
go build ./...                # compiles unchanged
grep -n "mcp-kit" go.mod      # present
```

**Commit:** `chore: add mcp-kit dependency`

#### Sub-step 6.2: Build adapter layer (1 day)

**Files (skills-mcp, create):**
- `apps/skills-mcp/internal/kitwiring/userstore.go` — wraps `service.UserService` for `userstore.Store`
- `apps/skills-mcp/internal/kitwiring/audit.go` — wraps `service.AuditService` for `audit.Emitter`
- `apps/skills-mcp/internal/kitwiring/authz.go` — wraps `service.AuthzService` for `authz.Service`
- All `*_test.go`

**Adapter shape (illustrative):**
```go
package kitwiring

type UserStore struct { svc *service.UserService }

func (u *UserStore) FindByEmail(ctx context.Context, email string) (userstore.User, error) {
    user, err := u.svc.FindByEmail(ctx, email)
    if ent.IsNotFound(err) { return nil, userstore.ErrNotFound }
    if err != nil { return nil, err }
    return userAdapter{user}, nil
}

type userAdapter struct{ inner *ent.User }
func (u userAdapter) ID() uuid.UUID         { return u.inner.UUID }
func (u userAdapter) Email() string         { return u.inner.Email }
func (u userAdapter) PasswordHash() []byte  { return u.inner.PasswordHash }
func (u userAdapter) IsActive() bool        { return u.inner.IsActive }
```

**Tests required:**
- `TestUserStore_FindByEmail_NotFoundMapsToErrNotFound`
- `TestUserStore_FindByID_HappyPath`
- `TestAudit_EmitForwardsToService`
- `TestAuthz_CheckForwardsAndMapsForbidden`

**Verify:**
```bash
go test ./internal/kitwiring/... -count=1
```

**Commit:** `feat(kitwiring): adapters for UserStore, AuditEmitter, AuthzService`

#### Sub-step 6.3: Schema mixin migration (1 day)

**Files (skills-mcp, modify):**
- `apps/skills-mcp/ent/schema/oauthclient.go`
- `apps/skills-mcp/ent/schema/oauthsigningkey.go`
- `apps/skills-mcp/ent/schema/oauthsession.go` (or split into auth_code/access/refresh per kit's split)
- `apps/skills-mcp/ent/schema/personal_access_token.go`

**Pattern:**
```go
// Before:
type OAuthClient struct{ ent.Schema }
func (OAuthClient) Fields() []ent.Field { return []ent.Field{...} }
func (OAuthClient) Edges() []ent.Edge   { return []ent.Edge{...} }

// After:
type OAuthClient struct{ ent.Schema }
func (OAuthClient) Mixin() []ent.Mixin {
    return []ent.Mixin{kitschema.OAuthClient{}}
}
func (OAuthClient) Edges() []ent.Edge { return []ent.Edge{...} }  // kept; mixin doesn't define edges
```

**CRITICAL:** Resulting Ent schema diff must be empty. Run:
```bash
cd apps/skills-mcp
go generate ./ent/...
git diff ent/migrate/migrations/*.sql
```
Expected: no SQL change. If non-empty, kit mixin is wrong; fix kit first.

**Verify:**
```bash
go build ./...
go test ./ent/... -count=1
```

**Commit:** `refactor(ent): compose kit mixins for OAuth + PAT schemas`

#### Sub-step 6.4: Wire kit in main (parallel mount) (1 day)

**Files (skills-mcp, modify):**
- `apps/skills-mcp/cmd/skills-mcp/main.go` — add kit construction
- `apps/skills-mcp/internal/server/server.go` — add `ServeKitHTTP()` method that mounts at `/mcp-v2`

**Pattern:**
```go
// In main.go after existing server setup:
oauthProv, _ := oauth.New(oauth.Config{
    Issuer:       cfg.PublicURL,
    EntClient:    entClient,
    UserStore:    kitwiring.NewUserStore(userSvc),
    AuditEmitter: kitwiring.NewAuditEmitter(auditSvc),
})
mcpKitServer, _ := mcpkit.New(mcpkit.Config{
    Implementation: mcp.Implementation{Name: "skills-mcp", Version: cfg.Version},
    Validator:      oauthProv.TokenValidator(),
    AllowedOrigins: cfg.AllowedOrigins,
    AllowLoopback:  cfg.IsDev,
    AuditEmitter:   kitwiring.NewAuditEmitter(auditSvc),
})

mux.Handle("/mcp-v2", mcpKitServer.Handler())
oauthProv.RegisterRoutes(mux, oauth.WithPathPrefix("/mcp-oauth-v2"))
```

**Both mounts active.** Legacy `/mcp` keeps mark3labs; new `/mcp-v2` runs kit.

**Verify:**
```bash
./bin/skills-mcp serve
curl http://localhost:8892/.well-known/oauth-authorization-server  # legacy
curl http://localhost:8892/.well-known/oauth-authorization-server  # check kit's endpoints conflict-free; if so, kit endpoints under /v2 prefix
curl http://localhost:8892/mcp-v2 -X POST ...                       # new mount responds
```

**Commit:** `feat(server): parallel mount kit-backed /mcp-v2 endpoint`

#### Sub-step 6.5: Migrate tool registration (2 days)

**Files (skills-mcp, modify):**
- `apps/skills-mcp/internal/server/registry.go`
- `apps/skills-mcp/internal/server/registry_skills_read.go`
- `apps/skills-mcp/internal/server/registry_skills_write.go`
- `apps/skills-mcp/internal/server/registry_clients.go`
- `apps/skills-mcp/internal/server/registry_subscriptions.go`
- `apps/skills-mcp/internal/server/registry_tree.go`

**Pattern (mark3labs → official SDK):**
```go
// Before (mark3labs):
import (
    "github.com/mark3labs/mcp-go/mcp"
    mcpserver "github.com/mark3labs/mcp-go/server"
)
mcpServer.AddTool(
    mcp.NewTool("search_skills",
        mcp.WithDescription("..."),
        mcp.WithString("query", mcp.Required(), mcp.Description("..."))),
    handlerFunc,
)

// After (official):
import (
    "github.com/modelcontextprotocol/go-sdk/mcp"
)
mcp.AddTool(sdkServer, &mcp.Tool{
    Name:        "search_skills",
    Description: "...",
}, handlerFunc)  // typed handler signature
```

**Migrate one tool family per commit** (skills_read, skills_write, clients, subscriptions, tree). Tests must continue to pass after each.

**Verify per family:**
```bash
go test ./internal/server/... -run TestRegistry -count=1
```

**Commit (per family):**
- `refactor(mcp): migrate skills_read tools to official SDK`
- `refactor(mcp): migrate skills_write tools to official SDK`
- `refactor(mcp): migrate clients tools to official SDK`
- `refactor(mcp): migrate subscriptions tools to official SDK`
- `refactor(mcp): migrate tree tools to official SDK`

#### Sub-step 6.6: Cut over (0.5 day)

**Files (skills-mcp, modify):**
- `apps/skills-mcp/cmd/skills-mcp/main.go`
- `apps/skills-mcp/internal/server/server.go`

**Steps:**
1. Move `/mcp-v2` mount to `/mcp`.
2. Move `/mcp-oauth-v2/*` mounts to `/mcp-oauth/*`.
3. Delete legacy mark3labs construction code.

**Verify:**
```bash
./bin/skills-mcp serve
just verify-mcp-clients http://127.0.0.1:8892
```
Expected: full suite green against kit-backed binary.

**Commit:** `refactor(server): cut over /mcp to kit; remove mark3labs mount`

#### Sub-step 6.7: Delete legacy code (0.5 day)

**Files (skills-mcp, delete):**
- `apps/skills-mcp/internal/oidc/discovery.go`
- `apps/skills-mcp/internal/oidc/handlers.go`
- `apps/skills-mcp/internal/oidc/keys.go`
- `apps/skills-mcp/internal/oidc/provider.go`
- `apps/skills-mcp/internal/oidc/register.go`
- `apps/skills-mcp/internal/oidc/rotator.go`
- `apps/skills-mcp/internal/oidc/session_store.go`
- `apps/skills-mcp/internal/oidc/storage.go`
- `apps/skills-mcp/internal/oidc/form.go` (if unused after migration)
- `apps/skills-mcp/internal/auth/middleware.go`
- `apps/skills-mcp/internal/auth/pat_validator.go`
- `apps/skills-mcp/internal/auth/log_classify.go`
- `apps/skills-mcp/internal/auth/verify.go`
- `apps/skills-mcp/internal/auth/password.go`
- `apps/skills-mcp/internal/auth/scope_context.go` (if unused — verify no consumers in HTML handlers)
- `apps/skills-mcp/internal/cliauth/` (entire package)
- All corresponding `*_test.go`

**Files (skills-mcp, modify):**
- `apps/skills-mcp/cmd/skills-mcp/auth.go` — `auth login` subcommand becomes thin wrapper around `cliauth.Login()` from kit
- `apps/skills-mcp/go.mod` — remove `github.com/mark3labs/mcp-go`

**Verify:**
```bash
go mod tidy
go build ./...
go test ./... -count=1 -race
just verify-mcp-clients http://127.0.0.1:8892
```

**Commit:** `chore: delete vendored OAuth + cliauth (now in mcp-kit)`

#### Sub-step 6.8: Cycle methodology validation (1 day)

Run the cycle dispatch runbook (template from kit's `docs/dispatch-runbook-template.md`) against skills-mcp's kit-backed binary.

**Phases to execute:**
- P0 Bootstrap — mint token via kit's OAuth flow
- P1 Initialize handshake (TQ-02)
- P2 OAuth deep coverage (refresh rotation, error envelopes)
- P3 Auth + origin + scope
- P4 Tool surface (skills/clients/subs/tree)
- P5 PAT validation
- P6 Discovery accuracy
- P7 Envelope conformance + instructions accuracy

**Real-client gates (mandatory before kit `v0.3.0` tag):**
- MCP Inspector connects + lists tools
- Claude Code connects + executes one tool

**Verify:** All phases green. Captures saved under `apps/skills-mcp/docs/cycle-evidence/2026-MM-DD/`.

**Commit:** `test(mcp): cycle dispatch validation against kit-backed binary`

**Phase verify (all sub-steps):**
- LOC delta: `git diff --stat main...refactor/adopt-mcp-kit` shows ~3000 net deleted lines.
- `go.mod` diff: `mark3labs/mcp-go` removed; `mcp-kit` present.
- `verify-mcp-clients` green.
- Cycle methodology P0–P7 + real-client gates green.
- All consumer-facing tool names + behaviors unchanged (no client breakage).

**Phase commit:** `refactor(mcp): adopt mcp-kit; remove vendored OAuth + middleware`

**Kit tag:** `v0.3.0` after sub-step 6.8 closes.

**Effort:** 7 days total across 8 sub-steps.

---

### Phase 7: Migrate vorrent to kit (v0.4.0 gate)

**Repo:** `/Volumes/Dev/HaakCo/AiProjects/vorrent/`
**Status:** Kit-backed Vorrent migration committed and pushed as `1d6b870d refactor: adopt shared mcp kit` on 2026-05-02. `v0.4.0` is not tagged yet because the full destructive/fixture-heavy P0-P10 cycle and Claude Code real-client gate remain unproven in the migration pass.

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

**Commit (in vorrent):** `1d6b870d refactor: adopt shared mcp kit`

**Kit tag:** pending `v0.4.0` after the remaining live-client gates close.

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

---

## Appendix A: Verify-commands cheatsheet

Copy-paste recipes for the most common verification points. All run from the indicated working directory.

### Kit build + test (any phase)

```bash
cd /Users/timhaak/Dev/HaakCo/AiProjects/sharedLib/golang/mcp-kit
unset GOROOT  # mise/Go interaction guard
go build ./...
go test ./... -count=1 -race
golangci-lint run ./...
```

### Smoke the example server (Phase 3 sub-step 3.10 onwards)

```bash
cd /Users/timhaak/Dev/HaakCo/AiProjects/sharedLib/golang/mcp-kit/_examples/minimal-server
go run . &
sleep 2
./scripts/smoke.sh
kill %1
```

### skills-mcp (Phase 6)

```bash
cd /Users/timhaak/Dev/HaakCo/AiProjects/skills/apps/skills-mcp
unset GOROOT
go build ./...
go test ./... -count=1
just verify-mcp-clients http://127.0.0.1:8892        # client contract
git diff --stat main...HEAD                           # LOC delta check
grep -c "mark3labs/mcp-go" go.mod || echo "removed"   # confirm SDK swap
```

### vorrent (Phase 7)

```bash
cd /Volumes/Dev/HaakCo/AiProjects/vorrent
unset GOROOT
go build ./...
go test ./internal/mcpserver/... -count=1
git diff --stat main...HEAD                            # LOC delta check
```

### meridian (Phase 9)

```bash
cd /Users/timhaak/Dev/mairin/meridian/backend
task build
task test
task lint
```

### Cycle dispatch (any consumer)

```bash
# 1. Mint token (P0)
TOKEN=$(./scripts/mcp-test-token.sh)

# 2. Three-step handshake (P1)
SESSION=$(curl -i -s -X POST $URL/mcp \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}' \
  | grep -i '^Mcp-Session-Id' | awk '{print $2}' | tr -d '\r')

curl -X POST $URL/mcp -H "Authorization: Bearer $TOKEN" -H "Mcp-Session-Id: $SESSION" \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'  # expect 202

# 3. tools/list (P4)
curl -X POST $URL/mcp -H "Authorization: Bearer $TOKEN" -H "Mcp-Session-Id: $SESSION" \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | xxd | head -25
# Verify: bytes show '5c 6e' (escaped \n) at every newline, NOT '0a' (raw LF) — FP-01 disproof
```

### Real-client gates (Phase 6, 7, 9)

- **Inspector:** Open https://inspector.modelcontextprotocol.io, paste server URL + token, click Connect. Verify tools/resources/prompts list populates without errors. Take screenshot.
- **Claude Code:** Add server to `~/.claude.json` `mcpServers`, restart, run `/mcp` to list. Verify tool execution against a known fixture.

---

## Appendix B: Why this plan is serial across consumers

A natural objection: "Why not migrate all three consumers in parallel?" Three reasons:

1. **The kit's API is unproven until at least one consumer ships against it.** Phase 6 is the gate that catches API mistakes. If Phase 7 or 9 ran simultaneously and the kit needed a breaking change, all three would block while it lands.
2. **skills-mcp donates the OAuth code.** Until it's lifted out, vorrent and meridian can't migrate. Migration order is dictated by code donation order.
3. **Pre-1.0 versioning works only if API breaks land between consumers, not within.** Tagging `v0.3.0` after skills-mcp validates the API; `v0.4.0` after vorrent re-validates; `v1.0.0` after meridian. Three checkpoints, three opportunities to break + fix before stable.

Phase 10 (Skills update) and Phase 11 (Migration docs) CAN run in parallel with their predecessors — they're documentation and don't gate any code path.

---

## Appendix C: Glossary

| Term | Meaning |
|---|---|
| **kit** | This library — `github.com/haakco/mcp-kit` |
| **consumer** | A Go service that depends on the kit (skills-mcp, vorrent, meridian) |
| **donor** | Consumer whose existing code is the source for a kit package (skills-mcp donates OAuth; vorrent donates middleware) |
| **kitwiring** | Adapter layer in each consumer that wraps existing services in kit interfaces (`UserStore`, `AuditEmitter`, `AuthzService`) |
| **cycle** | A phased E2E test session against a running MCP server (P0..P10), defined in `mcp-kit/docs/cycle-methodology.md` |
| **real-client gate** | Mandatory cycle phase that uses Inspector or Claude Code, not curl |
| **lessons-learned ID** | Stable reference for a known failure mode: TQ- (transport), OG- (OAuth), FP- (false positive), PR- (procedural), EG- (engineering gap) |
| **dispatch runbook** | Per-cycle copy-paste recipe for executing each phase (template ships in kit) |
| **same-branch concurrent** | Multi-agent execution model from HaakCo CLAUDE.md where teams share a branch but own non-overlapping file sets |
