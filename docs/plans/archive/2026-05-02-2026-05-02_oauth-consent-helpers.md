# OAuth Consent Helpers — Implementation Plan (v2)

> **For agentic workers:** Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land a single, opinionated `oauth/consent` package in `mcp-kit` that **replaces every hand-rolled `/oauth/authorize` handler in HaakCo's Go MCP servers with one shared shape**, follows the latest MCP authorization spec strictly by default, and preserves consumer flexibility through five small interfaces (one handler, two pluggable approval-token backends, one consent policy, one optional challenge provider).

**Background:** Today three Go MCP servers ship divergent authorize handlers:

| Consumer | Pattern | Storage | RFC 8707? | Audit on consent? |
|---|---|---|---|---|
| `vorrent` | Stateless HMAC approval token + params digest | In-memory map | Strict-when-present | No |
| `skills-mcp` | Session cookie + CSRF cookie + redirect to `/login` | Session backend (DB) | No | No (audits at token endpoint only) |
| `meridian` (planned) | Same shape as vorrent (independent port) | In-memory map | Strict-when-present | Yes (approve + deny) |

The Laravel side (`cb`, `tlm` via Passport) uses a third mechanism — session-stored opaque `auth_token` `pull`'d on consume — but converges on the same security properties (continuity, replay, audience-binding). All four mechanisms solve the same threats; convergence is achievable.

This plan picks **one** Go-side `consent.Handler` shape that all three Go servers will call into via identical wiring. Two `ApprovalTokenStore` implementations ship: an `hmacstore` (vorrent-style, default) and a `sessionstore` (skills-mcp / Laravel-equivalent). Consumers swap one constructor call to choose; the rest of the API is identical.

**Architecture:**

```
                 ┌──────────────────────────────────┐
                 │ consent.Handler (one shape)      │
                 │   ServeHTTP — login, approve,    │
                 │   deny, redirect-bridge          │
                 └────────────────┬─────────────────┘
                                  │ uses
        ┌───────────────┬─────────┼─────────┬──────────────┐
        ▼               ▼         ▼         ▼              ▼
  Authenticator    Renderer  ApprovalToken ConsentPolicy  ChallengeProvider
  (consumer)       (consumer)    Store      (default:      (default: nil;
                                 ▲          AlwaysAsk)     for future 2FA)
                                 │
                       ┌─────────┴─────────┐
                       │                   │
                   hmacstore         sessionstore
                   (default,         (opt-in for
                   stateless)        consumers with
                                     session backend)
```

**Tech Stack:** Go 1.26+, `github.com/ory/fosite v0.49`, `github.com/google/uuid v1.6` (already direct deps), `crypto/hmac`/`crypto/sha256`/`net/url` (stdlib). No `html/template` in core.

**Tech Stack (test):** Standard `testing` + `httptest`; the kit's existing `oauth/storage.NewMemoryStore` and `keys.NewManager(keys.NewMemoryStore())`; `go test -race` mandatory.

---

## Backwards Compatibility Guarantee (Non-Negotiable)

Two production-grade Go services depend on this kit today: **vorrent** (`v0.4.0`) and **skills-mcp** (`v0.3.0`). Neither uses `oauth/consent` (it does not exist yet). After this plan ships, both must continue to:

- Compile against the new kit SHA without source changes (`go build ./...`).
- Pass their own test suites (`go test ./... -race`).
- Run their own E2E probes (`PR-01`, `PR-02`, `PR-03` in `docs/lessons.md`) unchanged.

The plan achieves this by being **100% additive** to the kit's public surface:

| Kit area | Change | Risk to consumers |
|---|---|---|
| `oauth/consent/*` | Whole new package | None — consumers don't import it yet. |
| `oauth.Subject` | Add `Extra map[string]any` field | None — additive struct field; existing literal initializers `Subject{ID:..., Email:..., GrantedScopes:...}` still compile (named fields), and even positional ones reading the first three fields still work because Go doesn't allow positional struct literals across packages. |
| `oauth/handlers.go` `AuthorizeHandler` | Tighten docstring only; **behavior unchanged** | None — doc-only. |
| `audit.Event` | No change | None. |
| `mcpkit.Config` | No change | None. |
| `oauth.Provider` | No change in Phase 1–6. (Phase 4 originally proposed `MountAuthorize` and `BrowserAuthorizeHandler`; **dropped** — `http.Handler` is sufficient and the interface added zero type safety.) | None. |

**Verification gates** at the end of every phase:

1. `go build ./...` in mcp-kit — green.
2. `go test ./... -race -count=1` in mcp-kit — green.
3. `go vet ./...` in mcp-kit — clean.
4. **Cross-consumer build check (phase end only)**: in a scratch directory, clone vorrent and skills-mcp at their current main, point each `go.mod` at the kit branch via `replace`, run `go build ./...` in each. Must succeed without source changes. (One-shot; just confirms additive.)

If any gate fails, the phase is rejected. No skipping. No "we'll fix it next phase."

---

## Open Decisions (locked before launch)

The user has confirmed the architectural direction. The four decisions below are now committed; flip them only by user request.

### Q1 — Agnostic core ships now; default template deferred (LOCKED — Option C)

Rationale: with `N=2` actual templates today (vorrent's two-page, skills-mcp's session-redirect), there is not enough signal to design a default. Revisit when a third Go consumer hand-rolls one.

### Q2 — Subject contract (LOCKED — Option A + Extra)

`Authenticator.Authenticate` returns the existing `oauth.Subject`, with one additive field — `Extra map[string]any` — so consumers like vorrent (who stuff `role`/`user_uuid` into the fosite session) can preserve those fields without forcing the kit to take a position on user shape. The default consent flow ignores `Extra`; consumers who care wire it through their own `SessionFor(Subject)` hook (Phase 1, Task 1.4).

### Q3 — Deprecation pace for `Provider.AuthorizeHandler` (LOCKED — Option A)

Tighten the docstring to call out the demo nature and point to `oauth/consent` + `oauth/README.md`. **Keep the name.** No rename in this plan; revisit at v1.0.0 deprecation cycle. Zero-churn for in-flight consumers.

### Q4 — Audit emission inside `consent.Handler` (LOCKED — Option A)

`consent.Handler` accepts an optional `audit.Emitter` and emits `oauth.consent.approved` / `oauth.consent.denied` events on every approve/deny path with the cb-aligned payload (`{user_id, client_id, client_name, scopes[], decision, ip, user_agent}` plus `actor_user_id` if subject parses as UUID). Default is `audit.Discard()` so the kit never crashes on a nil emitter. Event names use the dotted convention to match Laravel's existing `mcp.oauth.token.issued` family for cross-language log correlation.

---

## Current State (Verified)

**Files examined directly (verified line numbers, 2026-05-02):**

- `oauth/handlers.go` (148 LOC) — `AuthorizeHandler` lines 13-53; current docstring (lines 13-15) does *not* call out the demo nature. `grantSubjectScopes` lines **134-147**; `grantDefaultAudience` lines **121-132**. `RegisterRoutes` lines 111-119.
- `oauth/provider.go` (102 LOC) — `Provider` struct lines 16-24 holds `oauth fosite.OAuth2Provider`, `store storage.Store`, `issuer string`, `audience string`, `allowedScopes []string`, `defaultScopes []string`, `allowedScope map[string]struct{}`. Methods: `OAuth2Provider()` line 81, `RegisterHandler()` line 86. `New(cfg Config)` line 27.
- `oauth/session.go` (39 LOC) — `Subject{ID, Email, GrantedScopes}` lines 12-16; `NewSession` line 19; `NewEmptySession` line 36. **This file gains one additive field** (`Extra map[string]any`) in Phase 1, Task 1.3.
- `oauth/config.go` (66 LOC) — `applyDefaults` enforces `len(Secret) != 32` at lines 50-52 (`oauth secret must be exactly 32 bytes, got %d`). Phase 1 mirrors this rule on `consent.Config.ApprovalSecret`.
- `oauth/error.go` (16 LOC) — exposes private `writeOAuthErrorBody`. Phase 4 ships a separate public `consent.WriteAuthorizeError` for the RFC 6749 envelope (different shape, different Cache-Control headers, JSON body); both stay.
- `oauth/middleware.go:250-264` — `writeBearerChallenge` already emits `WWW-Authenticate: Bearer realm="mcp-kit", resource_metadata="..."` on 401. **Spec-compliant for MCP 2025-06-18.** Phase 0 confirms only.
- `oauth/storage/memory.go` — `NewMemoryStore()` line 17. `oauth/storage/storage.go` — `Store` interface line 67.
- `audit/emitter.go` (66 LOC) — `Event` struct fields: `EntityType`, `EntityID`, `Action`, `ActorUserID *uuid.UUID`, `ClientID`, `Scope`, `PayloadHash`, `Metadata map[string]any`, `Timestamp`. `Discard()` returns nil-safe emitter at line 61.
- `oauth/keys/manager.go` — `NewManager(store Store, opts ...ManagerOption) *Manager` at line 71. `oauth/keys/memory.go` — `NewMemoryStore()` at line 16. **No `NewMemoryManager` convenience constructor exists**; tests compose `keys.NewManager(keys.NewMemoryStore())`.
- `testkit/server.go` (109 LOC) — `NewServer(t testing.TB)` constructor wiring `mcpkit.New` with `BearerConfig`, `AllowLoopback`, `AuditEmitter: audit.Discard()`. Model for `consenttest`'s `Provider(t, issuer)` in Phase 5.
- `_examples/minimal-server/main.go` — already exercises full OAuth (`AuthorizeHandler` line 60+, `OAuth2Provider()` introspector). The plan does **not** modify this example; a separate `_examples/oauth-consent-server/` is a deferred follow-up.
- `README.md:23` — "A user table or password store — consumers authenticate browser consent and map subjects themselves" (this is the boundary the plan respects).
- `README.md:55` — `myapp.NewAuditEmitter(db)` shown wired into a domain handler.
- `README.md:70` — `mux.Handle("/oauth/authorize", oauthProv.AuthorizeHandler(myapp.ResolveSubject))` — the line Phase 6 replaces with a `consent.NewHandler` example.
- `go.mod:3` — `go 1.26` with `toolchain go1.26.2`. `github.com/google/uuid v1.6.0` is a direct dep. `github.com/ory/fosite v0.49.0` is a direct dep.

**Reference consumers (do NOT modify in this plan):**

- `vorrent/internal/api/mcp_oauth_authorize.go` (**481 LOC**, locally accessible). Phase 4 lifts almost every helper from here:
  - Approval-token machinery: lines 350-419 (`newApprovalToken`, `consumeApprovalToken`, `consumeStoredApprovalToken`, `approvalParamsDigest`, `signApprovalPayload`).
  - Synthetic-GET: `buildAuthorizeRequest` lines 290-313.
  - RFC 8707 validator: `validateMCPResourceIndicators` lines 437-455.
  - Form scrubber: `oauthAuthorizeValues` lines 336-348.
  - Hidden-input + redirect helpers: `clientNameFromRequester` lines 271-288, `authorizeRedirectURL` lines 253-269, `argumentsToStrings` lines 474-481, `normalizeOAuthStringSlice` lines 457-472, `resourcesFromValues` lines 421-427.
  - RFC 6749 error encoder: `writeOAuthAuthorizeRequestError` lines 69-81.
- `vorrent/internal/api/mcp_oauth_template.go` — `hiddenInputsFromValues` (vorrent's HTML template helpers; rendering stays consumer-side).
- `skills-mcp/apps/skills-mcp/internal/oidc/handlers.go` (364 LOC) — **different architecture** (session cookie + CSRF, redirect to `/login`). Phase 5 ships `sessionstore` so this consumer can adopt `consent.Handler` without rewriting its session model.

**Key findings (from cross-consumer survey):**

- Both Go consumers use `strconv` already (not `fmt.Sprintf`/`fmt.Sscanf`); the plan does NOT need to "fix" this.
- Vorrent's `approvalTokens` map has **no janitor** — bounded only by token TTL. Plan ships the same lazy-expiry pattern (delete on consume, delete on expired-redemption-attempt) and documents in `oauth/README.md` that operators expecting adversarial mint-but-never-consume behavior should add a janitor in their wrapper.
- Vorrent's resource URL is **dynamic per-request** (`s.resourceURL(r)` derived from request host) — supports multi-tenant deployments. Plan uses `ResourceURLFunc(*http.Request) (string, error)`, not a static string. A `consent.StaticResourceURL("https://...")` helper covers single-host deployments.
- Vorrent has domain-specific scope-vs-role validation (`validateMCPAdminScopesForUser`) — admin scopes require admin user. The kit cannot bake this in (it's domain-specific) but exposes a `ConsentPolicy.ValidateScopes(ctx, subject, scopes)` hook so consumers fail authorize before token issuance when forbidden.
- cb's Laravel implementation supports skip-consent for first-party clients (`OAuthPassportClient::skipsAuthorization`). Plan exposes `ConsentPolicy.AllowsSkip(ctx, client, subject, scopes) bool` so future Go consumers can match.
- cb's audit payload (`OAuthConsentAuditData{userId, clientId, clientName, scopes[], decision, ip, userAgent}`) and event names (`oauth.consent.approved`/`oauth.consent.denied`) are the cross-language target. Plan adopts both verbatim.
- cb has 2FA between credential check and code issuance. Plan defines the `ChallengeProvider` interface but ships only a nil-safe wiring — actual 2FA support is a future follow-up.

**Surprises:**
- `oauth/middleware.go:253` already emits `resource_metadata=` on 401 — the kit is already MCP 2025-06-18 spec-compliant for the bearer challenge. Phase 0 confirms by running the existing `middleware_test.go:87-89` test. No fix needed.
- `oauth.Subject` is referenced by name in vorrent's kit-imported scope code (`internal/mcpserver/auth.go`); confirming via `grep` that adding a field doesn't shadow anything in vorrent is part of the cross-consumer build check.

**Parallelization opportunities:**
- Phase 2 (`hmacstore`) and Phase 3 (`sessionstore`) both depend only on Phase 1 (interfaces). They can run in parallel.
- Phase 5 (`consenttest`) needs Phase 4 (`Handler`) green.
- Phase 6 (docs) can run in parallel with Phase 5.

**Shared branch coordination:**
- All phases work on the same branch (suggested name: `oauth-consent-helpers`).
- Teams must NOT use `git stash`, `git reset` (any flavor), or any automation that performs either.
- Teams must NOT modify files outside their assigned phase scope.
- If conflicting edits appear, the coordinator resolves via merge — never via reset.

---

## Phase 0: Spec Audit + Baseline

**Goal:** Confirm the kit is already at MCP 2025-06-18 spec compliance for everything outside the consent surface, and lock the baseline so any phase that breaks an existing test is caught immediately.

**Owner:** Coordinator. ~15 minutes total.

**Files:**
- (No files modified.)

### Task 0.1 — Confirm `WWW-Authenticate: ..., resource_metadata="<url>"` on 401

- [ ] **Step 0.1.1 — Run the existing test that proves the header is emitted**

  ```bash
  cd /home/timhaak/Dev/HaakCo/AiProjects/sharedLib/golang/mcp-kit
  go test -run "TestBearer.*WWW.*Authenticate|TestBearerChallenge|TestBearer" ./oauth/... -v
  ```

  Expected: tests in `oauth/middleware_test.go` lines 83-89 pass; the assertion that the header contains `resource_metadata=` succeeds.

  If the test does NOT exist or fails, **stop**. The kit is not spec-compliant for the bearer challenge. File a separate bug; do not start Phase 1 — `consent.Handler` cannot ship on top of a broken bearer story.

### Task 0.2 — Confirm `oauth/consent/` does not exist

- [ ] **Step 0.2.1 — Verify no collision**

  ```bash
  ls oauth/consent/ 2>&1 | head -1
  ```

  Expected: `ls: cannot access 'oauth/consent/': No such file or directory`.

  If the directory exists, stop and surface to the coordinator. Either it's stale work to clean up (with user approval) or another team is mid-implementation — do not overwrite.

### Task 0.3 — Lock the baseline

- [ ] **Step 0.3.1 — Capture green-state tests + lint**

  ```bash
  go build ./... 2>&1 | tee /tmp/mcp-kit-phase0-build.log
  go test ./... -race -count=1 2>&1 | tee /tmp/mcp-kit-phase0-test.log
  go vet ./... 2>&1 | tee /tmp/mcp-kit-phase0-vet.log
  ```

  Expected: all three logs end clean. If anything is red on `main`, the project has a pre-existing problem — fix it first per the ownership rule in `~/.claude/CLAUDE.md`. **Do not start Phase 1 on a red baseline.**

### Task 0.4 — Cross-consumer baseline

- [ ] **Step 0.4.1 — Confirm vorrent + skills-mcp build against `main`**

  ```bash
  cd /tmp
  rm -rf phase0-vorrent phase0-skills
  git clone --depth 1 /home/timhaak/Dev/HaakCo/AiProjects/vorrent phase0-vorrent
  git clone --depth 1 /home/timhaak/Dev/HaakCo/AiProjects/skills phase0-skills
  cd phase0-vorrent && go build ./... 2>&1 | tail -20
  cd /tmp/phase0-skills/apps/skills-mcp && go build ./... 2>&1 | tail -20
  ```

  Expected: both builds succeed. Captures the "before" state. Phase 6's verification re-runs this with the kit pointed at the new branch.

---

## Phase 1: Core Types + Interfaces

**Goal:** Land the type/interface skeleton that every other phase builds on. Five interfaces, one struct (`Config`), supporting types (`Page`, `PageData`, `HiddenInput`, `ResourceURLFunc`).

**Owner:** Phase-1 team (one Change Agent → one Validation Agent → one Security Agent per the three-stage process in `~/.claude/CLAUDE.md`).

**Scope (files this team owns):**
- Create: `oauth/consent/doc.go`
- Create: `oauth/consent/page.go`
- Create: `oauth/consent/authenticator.go`
- Create: `oauth/consent/renderer.go`
- Create: `oauth/consent/store.go`
- Create: `oauth/consent/policy.go`
- Create: `oauth/consent/challenge.go`
- Create: `oauth/consent/resource.go` (just the `ResourceURLFunc` type + helpers; the validator function lands in Phase 4)
- Create: `oauth/consent/config.go`
- Modify: `oauth/session.go` (add `Extra map[string]any` to `Subject`)
- Modify: `oauth/session_test.go` (add a small test that `Extra` round-trips through `NewSession` if used)

**Cannot run in parallel with:** any other Phase 1 task — each builds on the prior. Phases 2–6 are blocked until Phase 1 lands.

---

### Task 1.1 — Package skeleton

**Files:**
- Create: `oauth/consent/doc.go`

- [ ] **Step 1.1.1 — Write `doc.go`**

  ```go
  // Package consent provides one shared OAuth 2.1 authorization-endpoint
  // handler that performs browser-based login, optional 2FA challenge, explicit
  // user consent, and produces a fosite authorization response with the
  // canonical MCP audience bound server-side per RFC 8707.
  //
  // # Why this package exists
  //
  // oauth.Provider.AuthorizeHandler is a demo handler — it grants requested
  // scopes immediately to whatever subject SubjectResolver returns, does not
  // authenticate the browser, does not collect explicit consent, and does
  // not enforce resource indicators. Production servers must do all three.
  // consent.Handler is the kit's canonical answer.
  //
  // # Design — five interfaces, one Handler
  //
  // Consumers wire five collaborators. The kit owns the protocol-correct
  // mechanics; the consumer owns user model and UI:
  //
  //   - Authenticator       — credential check, returns oauth.Subject
  //   - Renderer            — writes login/consent/redirect-bridge HTML
  //   - ApprovalTokenStore  — issues + consumes approval tokens; ships in
  //                            two stock impls (hmacstore, sessionstore)
  //   - ConsentPolicy       — per-client/scope skip-consent + scope-grant
  //                            validation; default AlwaysAsk()
  //   - ChallengeProvider   — optional second-factor; nil-safe (no challenge)
  //
  // # Spec posture (defaults)
  //
  //   - PKCE S256 (handled by fosite via oauth.Provider).
  //   - RFC 8707 strict-when-present resource indicators.
  //   - Server-side audience binding regardless of client RFC 8707 support.
  //   - RFC 6749 JSON error envelope on hard failures.
  //   - Single-use approval tokens.
  //   - Audit events oauth.consent.approved / oauth.consent.denied
  //     (cross-language naming aligned with Laravel Passport implementations).
  //
  // # Layering
  //
  // See oauth/README.md "Replacing the demo authorize handler" for a worked
  // integration sketch and per-consumer migration notes.
  package consent
  ```

- [ ] **Step 1.1.2 — Verify the package compiles in isolation**

  ```bash
  go build ./oauth/consent/...
  ```

  Expected: `go: no Go files in .../oauth/consent/...` becomes "compiled cleanly" once the .go file exists. Any Go error ends the step.

- [ ] **Step 1.1.3 — Commit**

  ```bash
  git add oauth/consent/doc.go
  git commit -m "feat(oauth/consent): package skeleton + design rationale"
  ```

---

### Task 1.2 — `Page`, `PageData`, `HiddenInput`

**Files:**
- Create: `oauth/consent/page.go`

- [ ] **Step 1.2.1 — Write `oauth/consent/page.go`**

  ```go
  package consent

  // Page identifies which page the Renderer should render.
  type Page int

  const (
      // PageLogin is the initial form (GET) and the re-render after a failed
      // login or expired approval token.
      PageLogin Page = iota

      // PageConsent is rendered after a successful credential check (and an
      // optional successful Challenge). Includes the approval_token hidden
      // input the kit consumes on the approve POST.
      PageConsent

      // PageRedirectBridge is rendered after a successful approve. The renderer
      // writes a small HTML doc that bounces the browser to the OAuth
      // redirect_uri (typically meta-refresh + a manual link). When a renderer
      // returns nil for this page, the kit falls back to a 302 redirect.
      PageRedirectBridge
  )

  // HiddenInput is one <input type="hidden"> the renderer must emit verbatim
  // so the kit can re-derive the original OAuth request on POST.
  type HiddenInput struct {
      Name  string
      Value string
  }

  // PageData is the data the kit hands the Renderer. Renderers ignore fields
  // they don't display (e.g. Authenticated is meaningless on PageRedirectBridge).
  type PageData struct {
      // Authenticated is true on PageConsent and PageRedirectBridge.
      Authenticated bool

      // DisplayName is the resolved-friendly name of the authenticated user
      // (e.g. "Alice Smith"). Empty on PageLogin. Renderers may fall back to
      // Subject.Email or Subject.ID.
      DisplayName string

      // ClientName is the human-readable name of the OAuth client requesting
      // authorization. Defaults to "your MCP client" when no name is available.
      ClientName string

      // Scopes is the list of scopes the client requested. Renderers typically
      // render these as a checklist. The user's scope choices are not honored
      // on POST — fosite grants what was requested unless ConsentPolicy
      // narrows it.
      Scopes []string

      // Resources is the set of canonical resources the token will be
      // audience-bound to (RFC 8707). Empty when the client did not include
      // `resource`; the kit always binds the canonical resource server-side
      // regardless.
      Resources []string

      // HiddenInputs are name/value pairs the renderer must include as
      // <input type="hidden"> in the consent form so the kit can re-derive
      // the original request on POST. Includes approval_token on PageConsent.
      HiddenInputs []HiddenInput

      // FormAction is the path the form must POST back to (typically
      // "/oauth/authorize").
      FormAction string

      // Error is a human-readable message when re-rendering after a failed
      // login attempt or expired token. Empty on the happy path.
      Error string

      // RedirectURL is set on PageRedirectBridge — the URL the renderer should
      // bounce the browser to (meta-refresh or JS).
      RedirectURL string
  }
  ```

- [ ] **Step 1.2.2 — Verify and commit**

  ```bash
  go build ./oauth/consent/
  git add oauth/consent/page.go
  git commit -m "feat(oauth/consent): Page + PageData + HiddenInput types"
  ```

---

### Task 1.3 — Add `Subject.Extra` (additive)

**Files:**
- Modify: `oauth/session.go` lines 12-16

- [ ] **Step 1.3.1 — Write the failing test in `oauth/session_test.go`**

  ```go
  func TestSubjectExtra_RoundTripsViaNewSession(t *testing.T) {
      subject := Subject{
          ID:    "11111111-1111-1111-1111-111111111111",
          Email: "alice@example.com",
          Extra: map[string]any{"role": "admin"},
      }
      sess := NewSession(subject)
      role, ok := sess.Claims.Extra["role"].(string)
      if !ok || role != "admin" {
          t.Fatalf("Extra[role] not propagated to session claims: %#v", sess.Claims.Extra)
      }
  }
  ```

  *Whether `oauth/session_test.go` exists already:* check first. If absent, this is the file's first test and it needs `package oauth` + the standard imports.

- [ ] **Step 1.3.2 — Verify red**

  ```bash
  go test ./oauth/ -run TestSubjectExtra
  ```

  Expected: compile error (`Extra` is not a field of `Subject`) or test failure.

- [ ] **Step 1.3.3 — Add the field + propagate through `NewSession`**

  Edit `oauth/session.go`:

  ```go
  // Subject is the authenticated resource owner authorizing an OAuth client.
  type Subject struct {
      ID            string
      Email         string
      GrantedScopes []string

      // Extra carries consumer-specific session data (e.g. role, organization,
      // user_uuid as separate from ID). The kit copies Extra into the
      // OIDC session's Extra map verbatim. Keys must be JSON-serializable —
      // values are passed straight through to the JWT claims.
      Extra map[string]any
  }

  // NewSession creates an OIDC session for subject.
  func NewSession(subject Subject) *openid.DefaultSession {
      extra := map[string]any{
          "email": subject.Email,
      }
      for k, v := range subject.Extra {
          extra[k] = v
      }
      return &openid.DefaultSession{
          Claims: &jwt.IDTokenClaims{
              Subject:     subject.ID,
              RequestedAt: time.Now().UTC(),
              Extra:       extra,
          },
          Headers:   &jwt.Headers{},
          Subject:   subject.ID,
          Username:  subject.Email,
          ExpiresAt: map[fosite.TokenType]time.Time{},
      }
  }
  ```

  Note the order of inserts: `email` is set first so a consumer-supplied `Extra["email"]` (rare) wins — this matches what a consumer would expect if they explicitly override.

- [ ] **Step 1.3.4 — Verify green**

  ```bash
  go test ./oauth/ -run TestSubjectExtra -race
  go test ./oauth/... -count=1 -race  # full kit-side oauth tests stay green
  ```

  Expected: all green. Subject's existing tests don't reference `Extra` so they're unaffected.

- [ ] **Step 1.3.5 — Verify backwards compatibility**

  ```bash
  cd /tmp/phase0-vorrent && go build ./...
  cd /tmp/phase0-skills/apps/skills-mcp && go build ./...
  ```

  Expected: both succeed. Vorrent and skills-mcp construct `Subject` literals with named fields (`Subject{ID:..., Email:..., GrantedScopes:...}`); the new field defaults to `nil` and is ignored.

- [ ] **Step 1.3.6 — Commit**

  ```bash
  git add oauth/session.go oauth/session_test.go
  git commit -m "feat(oauth): add additive Subject.Extra field for consumer session data"
  ```

---

### Task 1.4 — `Authenticator` interface

**Files:**
- Create: `oauth/consent/authenticator.go`

- [ ] **Step 1.4.1 — Write `authenticator.go`**

  ```go
  package consent

  import (
      "context"

      "github.com/haakco/mcp-kit/oauth"
  )

  // Authenticator verifies a (username, password) pair and returns the
  // resulting oauth.Subject on success.
  //
  // # Error semantics
  //
  // On failure the returned error is shown to the user as a generic "invalid
  // email or password" — the kit never distinguishes between unknown user,
  // locked account, or wrong password to avoid enumeration leaks.
  // Implementations may emit their own audit events for failed attempts;
  // consent.Handler emits oauth.consent.denied only on explicit user-clicked
  // deny, not on credential failure.
  //
  // # Subject.Extra
  //
  // Implementations populate Subject.Extra with any consumer-specific session
  // data (role, organization, alternative ID forms). The kit propagates Extra
  // into the issued token's claims via oauth.NewSession.
  //
  // # Context
  //
  // Implementations must respect ctx cancellation; consent.Handler passes
  // r.Context() unchanged so request-scoped timeouts apply.
  type Authenticator interface {
      Authenticate(ctx context.Context, username, password string) (oauth.Subject, error)
  }

  // AuthenticatorFunc adapts a function to the Authenticator interface.
  type AuthenticatorFunc func(ctx context.Context, username, password string) (oauth.Subject, error)

  // Authenticate calls f.
  func (f AuthenticatorFunc) Authenticate(ctx context.Context, username, password string) (oauth.Subject, error) {
      return f(ctx, username, password)
  }
  ```

- [ ] **Step 1.4.2 — Verify and commit**

  ```bash
  go build ./oauth/consent/
  git add oauth/consent/authenticator.go
  git commit -m "feat(oauth/consent): Authenticator interface + Func adapter"
  ```

---

### Task 1.5 — `Renderer` interface

**Files:**
- Create: `oauth/consent/renderer.go`

- [ ] **Step 1.5.1 — Write `renderer.go`**

  ```go
  package consent

  import "net/http"

  // Renderer writes one of three pages to the response. The kit hands the
  // renderer a populated PageData; the renderer owns HTML.
  //
  // The kit calls Render exactly once per request. After Render returns,
  // the kit assumes the response is complete and does not write further.
  // (Exception: when the renderer returns ErrNoBridge from PageRedirectBridge,
  // the kit falls back to a 302 redirect to PageData.RedirectURL.)
  type Renderer interface {
      Render(w http.ResponseWriter, page Page, data PageData)
  }

  // RendererFunc adapts a function to the Renderer interface.
  type RendererFunc func(w http.ResponseWriter, page Page, data PageData)

  // Render calls f.
  func (f RendererFunc) Render(w http.ResponseWriter, page Page, data PageData) {
      f(w, page, data)
  }
  ```

  Note: `ErrNoBridge` is mentioned in the docstring but not defined — the kit's redirect-bridge path is rare enough that we don't expose a sentinel. If a renderer can't render `PageRedirectBridge`, it should write nothing and the kit will detect zero bytes written via a `responseWriterTracker` (Phase 4, Task 4.5). This keeps the API surface smaller.

- [ ] **Step 1.5.2 — Strike the `ErrNoBridge` reference and use plain language**

  Edit the docstring to drop the `ErrNoBridge` sentence and replace with:

  > When a renderer needs to skip the bridge entirely, it should write zero bytes; the kit detects that and falls back to a direct 302 redirect to `PageData.RedirectURL`.

- [ ] **Step 1.5.3 — Verify and commit**

  ```bash
  go build ./oauth/consent/
  git add oauth/consent/renderer.go
  git commit -m "feat(oauth/consent): Renderer interface + Func adapter"
  ```

---

### Task 1.6 — `ApprovalTokenStore` interface

**Files:**
- Create: `oauth/consent/store.go`

- [ ] **Step 1.6.1 — Write `store.go`**

  ```go
  package consent

  import (
      "context"
      "errors"
      "net/url"

      "github.com/haakco/mcp-kit/oauth"
  )

  // ErrApprovalTokenInvalid is returned by ApprovalTokenStore.Consume on any
  // failure mode (expired, forged, replayed, params mismatch). Callers
  // translate to a user-facing message and a fosite error; the kit never
  // distinguishes between failure modes at the user-visible layer.
  var ErrApprovalTokenInvalid = errors.New("consent: approval token invalid")

  // ApprovalTokenStore mints and consumes single-use approval tokens that
  // bind a successful login to a specific OAuth authorize request, with
  // tamper detection on the canonical OAuth params.
  //
  // # Implementations
  //
  // The kit ships two:
  //
  //   - hmacstore — stateless HMAC-signed payload + in-memory replay map.
  //                  Default. Works without any session backend. Memory-bound
  //                  by approvalTokenTTL × abandonment-rate.
  //   - sessionstore — opaque token stored in a consumer-supplied session
  //                     backend; consumed via SessionBackend.Pull (single-use
  //                     enforced by the backend). Mirrors Laravel Passport.
  //
  // # Lifetime
  //
  // Issued tokens MUST be consumed within approvalTokenTTL (5 minutes) or
  // Consume returns ErrApprovalTokenInvalid. Implementations enforce this
  // either by embedding an expiry in the token (hmacstore) or by setting a
  // TTL on the storage entry (sessionstore).
  //
  // # Tamper detection
  //
  // params is the credential-stripped OAuth request as filtered by
  // oauthAuthorizeValues. Implementations bind the params on Issue and
  // verify on Consume; a request whose canonical params changed between
  // login and approve must fail.
  type ApprovalTokenStore interface {
      // Issue mints a single-use approval token bound to subject and the
      // canonical params. The token is returned as an opaque string the
      // renderer embeds as <input type="hidden" name="approval_token">.
      Issue(ctx context.Context, subject oauth.Subject, params url.Values) (string, error)

      // Consume verifies, expires, and removes the token, returning the
      // embedded subject. params must match what was passed to Issue (after
      // credential-field stripping). Returns ErrApprovalTokenInvalid on any
      // failure.
      Consume(ctx context.Context, token string, params url.Values) (oauth.Subject, error)
  }
  ```

- [ ] **Step 1.6.2 — Verify and commit**

  ```bash
  go build ./oauth/consent/
  git add oauth/consent/store.go
  git commit -m "feat(oauth/consent): ApprovalTokenStore interface + ErrApprovalTokenInvalid"
  ```

---

### Task 1.7 — `ConsentPolicy` interface + `AlwaysAsk`

**Files:**
- Create: `oauth/consent/policy.go`

- [ ] **Step 1.7.1 — Write `policy.go`**

  ```go
  package consent

  import (
      "context"

      "github.com/ory/fosite"

      "github.com/haakco/mcp-kit/oauth"
  )

  // ConsentPolicy decides per-request whether the consent step may be skipped
  // and validates that the requested scopes are grantable to the subject.
  //
  // # When to implement
  //
  // Most consumers use AlwaysAsk(). Implement a custom policy when:
  //
  //   - The OAuth client is a first-party app the user has already trusted
  //     (skip consent — analogous to cb's OAuthPassportClient::skipsAuthorization).
  //   - Some scopes require role/permission gating that should fail authorize
  //     rather than render the consent form (e.g. vorrent's
  //     validateMCPAdminScopesForUser — admin scopes require admin user).
  type ConsentPolicy interface {
      // AllowsSkip returns true when the consent step should be bypassed for
      // this (client, subject, scopes) tuple. On true, consent.Handler skips
      // PageConsent and proceeds directly to issue the authorization code.
      // The renderer is never invoked for PageConsent.
      AllowsSkip(ctx context.Context, client fosite.Client, subject oauth.Subject, scopes []string) bool

      // ValidateScopes returns nil when subject may grant all scopes. Returns
      // a non-nil error to fail authorize before token issuance — the kit
      // wraps the error in fosite.ErrAccessDenied with the error message as
      // the hint. Use this to enforce role-based scope gates server-side.
      ValidateScopes(ctx context.Context, subject oauth.Subject, scopes []string) error
  }

  // AlwaysAsk returns a ConsentPolicy that never skips consent and never
  // rejects scopes. Default when Config.ConsentPolicy is nil.
  func AlwaysAsk() ConsentPolicy { return alwaysAskPolicy{} }

  type alwaysAskPolicy struct{}

  func (alwaysAskPolicy) AllowsSkip(context.Context, fosite.Client, oauth.Subject, []string) bool {
      return false
  }

  func (alwaysAskPolicy) ValidateScopes(context.Context, oauth.Subject, []string) error {
      return nil
  }
  ```

- [ ] **Step 1.7.2 — Verify and commit**

  ```bash
  go build ./oauth/consent/
  git add oauth/consent/policy.go
  git commit -m "feat(oauth/consent): ConsentPolicy interface + AlwaysAsk default"
  ```

---

### Task 1.8 — `ChallengeProvider` interface (interface only — no impl)

**Files:**
- Create: `oauth/consent/challenge.go`

- [ ] **Step 1.8.1 — Write `challenge.go`**

  ```go
  package consent

  import (
      "context"
      "errors"

      "github.com/haakco/mcp-kit/oauth"
  )

  // ErrChallengeRequired is returned by ChallengeProvider.Begin when an
  // additional credential check (e.g. TOTP, WebAuthn) must complete before
  // consent.Handler proceeds to PageConsent.
  var ErrChallengeRequired = errors.New("consent: additional challenge required")

  // ChallengeProvider runs an optional second-factor step between successful
  // credential check and PageConsent. Default in Config is nil — no challenge.
  //
  // The interface is defined now so future 2FA support can land without a
  // breaking API change. No kit-side implementation ships in this plan;
  // consumers wanting 2FA today must implement ChallengeProvider themselves.
  //
  // # Flow
  //
  // 1. Authenticator returns a Subject.
  // 2. consent.Handler calls Begin(ctx, subject). If Begin returns
  //    ErrChallengeRequired, Handler renders PageLogin with a challenge prompt
  //    (renderer-defined; kit hands PageData.Error="challenge required" plus
  //    PageData.HiddenInputs=[{Name: "challenge_id", Value: <id>}]).
  // 3. The browser POSTs back with action=challenge and a challenge response.
  //    Handler calls Verify(ctx, challengeID, response) and either proceeds
  //    or re-renders PageLogin.
  //
  // The plan ships steps 1 and 3's wiring as no-ops when Config.Challenge is
  // nil — every code path is nil-safe. Adding a real ChallengeProvider in a
  // future plan touches only the implementation.
  type ChallengeProvider interface {
      // Begin returns a non-nil error to halt the flow at PageLogin. When
      // err is ErrChallengeRequired, the kit renders PageLogin with the
      // returned challengeID embedded as a hidden input. Other errors are
      // surfaced as fosite.ErrServerError.
      Begin(ctx context.Context, subject oauth.Subject) (challengeID string, err error)

      // Verify validates a challenge response. nil err on success, error
      // (typically fmt.Errorf("invalid code")) on failure — the kit renders
      // PageLogin with PageData.Error set verbatim.
      Verify(ctx context.Context, challengeID, response string) error
  }
  ```

- [ ] **Step 1.8.2 — Verify and commit**

  ```bash
  go build ./oauth/consent/
  git add oauth/consent/challenge.go
  git commit -m "feat(oauth/consent): ChallengeProvider interface (defined for future 2FA)"
  ```

---

### Task 1.9 — `ResourceURLFunc` + `StaticResourceURL` helper

**Files:**
- Create: `oauth/consent/resource.go`

- [ ] **Step 1.9.1 — Write `resource.go` (type + helper only; the validator function is Phase 4)**

  ```go
  package consent

  import "net/http"

  // ResourceURLFunc derives the canonical MCP resource URL for an inbound
  // request. Vorrent-style multi-tenant deployments derive the URL from the
  // request host so the same binary serves multiple OAuth audiences.
  // Single-host deployments use StaticResourceURL.
  //
  // The returned URL is what the kit binds as the canonical audience on
  // every issued token (via fosite GrantAudience), and what
  // validateResourceIndicators expects when the client sends `resource=`.
  type ResourceURLFunc func(r *http.Request) (string, error)

  // StaticResourceURL returns a ResourceURLFunc that always returns url.
  // Use for single-host deployments. The supplied url is returned without
  // normalization — callers should pass the canonical form
  // (e.g. "https://my-mcp.example.com/mcp", no trailing slash).
  func StaticResourceURL(url string) ResourceURLFunc {
      return func(*http.Request) (string, error) { return url, nil }
  }
  ```

- [ ] **Step 1.9.2 — Verify and commit**

  ```bash
  go build ./oauth/consent/
  git add oauth/consent/resource.go
  git commit -m "feat(oauth/consent): ResourceURLFunc + StaticResourceURL helper"
  ```

---

### Task 1.10 — `Config` + `applyDefaults`

**Files:**
- Create: `oauth/consent/config.go`

- [ ] **Step 1.10.1 — Write `config.go`**

  ```go
  package consent

  import (
      "errors"
      "fmt"
      "log/slog"
      "time"

      "github.com/haakco/mcp-kit/audit"
      "github.com/haakco/mcp-kit/oauth"
  )

  // approvalSecretLength matches oauth.Config.Secret length so operators
  // who already manage a 32-byte cryptographically-strong key for the kit's
  // OAuth provider can use the same key-management posture for the consent
  // approval secret.
  const approvalSecretLength = 32

  // approvalTokenTTL bounds how long a logged-in user has to click
  // Approve/Deny before the token is rejected. Keeps the consent step bound
  // to the intent of the original sign-in.
  const approvalTokenTTL = 5 * time.Minute

  // Config wires consent.NewHandler. All required fields are flagged as such;
  // the rest have safe defaults (audit.Discard, AlwaysAsk policy, 64 KiB
  // form-body limit, slog.Default logger).
  type Config struct {
      // Provider is the kit OAuth provider whose fosite OAuth2Provider is
      // used to build authorize requests, mint authorization codes, and
      // write fosite errors. Required.
      Provider *oauth.Provider

      // Authenticator verifies username/password and returns an oauth.Subject.
      // Required.
      Authenticator Authenticator

      // Renderer renders PageLogin / PageConsent / PageRedirectBridge.
      // Required.
      Renderer Renderer

      // ApprovalStore mints and consumes approval tokens. When nil, an
      // hmacstore.New is constructed using ApprovalSecret.
      ApprovalStore ApprovalTokenStore

      // ApprovalSecret is the HMAC key used by the default hmacstore. When
      // empty AND ApprovalStore is nil, a 32-byte random key is generated
      // at construction (and a warning is logged via Logger). When non-empty
      // it MUST be exactly 32 bytes — shorter values are rejected. Ignored
      // when ApprovalStore is non-nil.
      ApprovalSecret []byte

      // ConsentPolicy decides skip-consent + scope validation. When nil,
      // AlwaysAsk() is used.
      ConsentPolicy ConsentPolicy

      // Challenge is the optional second-factor step between login and
      // consent. nil = no challenge. Future-proofs the API; current
      // consumers leave nil.
      Challenge ChallengeProvider

      // ResourceURL derives the canonical MCP resource URL per request.
      // When nil, the kit uses StaticResourceURL(PublicURL + "/mcp").
      ResourceURL ResourceURLFunc

      // PublicURL is the externally-reachable origin of the OAuth server
      // with no trailing slash, e.g. "https://app.example.com". Used to
      // build the synthetic GET request fosite reads from. Required.
      PublicURL string

      // FormPath is the path the consent form POSTs back to.
      // Defaults to "/oauth/authorize".
      FormPath string

      // AuditEmitter receives oauth.consent.{approved,denied} events. When
      // nil, audit.Discard is used.
      AuditEmitter audit.Emitter

      // FormBodyLimit caps the POST body size for the authorize endpoint.
      // Defaults to 64 KiB. Set smaller for hardened deployments.
      FormBodyLimit int64

      // Now overrides time.Now for tests. Defaults to time.Now.
      Now func() time.Time

      // Logger receives diagnostic events (auto-generated secret, resource
      // mismatches). Defaults to slog.Default.
      Logger *slog.Logger
  }

  // applyDefaults fills zero-valued fields and validates the configuration.
  // Returns an error if a required field is missing or an explicit field is
  // invalid. Mutates c in place.
  func (c *Config) applyDefaults() error {
      if c.Provider == nil {
          return errors.New("consent: Provider is required")
      }
      if c.Authenticator == nil {
          return errors.New("consent: Authenticator is required")
      }
      if c.Renderer == nil {
          return errors.New("consent: Renderer is required")
      }
      if c.PublicURL == "" {
          return errors.New("consent: PublicURL is required")
      }
      if c.FormPath == "" {
          c.FormPath = "/oauth/authorize"
      }
      if c.ResourceURL == nil {
          c.ResourceURL = StaticResourceURL(c.PublicURL + "/mcp")
      }
      if c.ConsentPolicy == nil {
          c.ConsentPolicy = AlwaysAsk()
      }
      if c.AuditEmitter == nil {
          c.AuditEmitter = audit.Discard()
      }
      if c.FormBodyLimit == 0 {
          c.FormBodyLimit = 64 << 10
      }
      if c.Now == nil {
          c.Now = time.Now
      }
      if c.Logger == nil {
          c.Logger = slog.Default()
      }
      // ApprovalStore default depends on ApprovalSecret. The constructor
      // (NewHandler) wires this after applyDefaults returns so the
      // hmacstore package is not imported here.
      return nil
  }

  // ValidateApprovalSecret enforces the 32-byte rule on operator-supplied
  // secrets. Called by NewHandler when ApprovalStore is nil and
  // ApprovalSecret is non-empty. Public so consumers building their own
  // ApprovalTokenStore from Config.ApprovalSecret can reuse the rule.
  func ValidateApprovalSecret(secret []byte) error {
      if len(secret) != approvalSecretLength {
          return fmt.Errorf("consent: ApprovalSecret must be exactly %d bytes, got %d", approvalSecretLength, len(secret))
      }
      return nil
  }

  // ApprovalTokenTTL returns the bound on approval-token lifetime. Tests
  // and operators inspecting policy use this rather than hard-coding.
  // Exposed so subpackage ApprovalTokenStore implementations (hmacstore,
  // sessionstore) compute matching expiry without duplicating the constant.
  func ApprovalTokenTTL() time.Duration { return approvalTokenTTL }
  ```

- [ ] **Step 1.10.2 — Verify and commit**

  ```bash
  go build ./oauth/consent/
  go vet ./oauth/consent/...
  git add oauth/consent/config.go
  git commit -m "feat(oauth/consent): Config + applyDefaults + ValidateApprovalSecret"
  ```

---

### Phase 1 Sign-off Gate

Before launching Phases 2–6, the coordinator runs the **three-stage review** (`~/.claude/CLAUDE.md`):

1. **Spec compliance reviewer** — every interface from this plan exists with the signature documented; `Subject.Extra` round-trips through `NewSession`; nil-safety on optional fields confirmed.
2. **Code quality reviewer** — naming matches kit idioms (`Func` adapters, `Static*` helpers), godocs explain *why* not *what*, no premature abstractions.
3. **Security reviewer** — `ValidateApprovalSecret` enforces 32-byte rule; nil-safe ChallengeProvider does not leak through to the auth path; no secret material in logs (`Logger.Warn` for auto-generated secret only mentions "ephemeral", never the bytes).

Cross-consumer build check at the end of Phase 1:

```bash
cd /tmp/phase0-vorrent && go build ./...        # must succeed
cd /tmp/phase0-skills/apps/skills-mcp && go build ./...  # must succeed
go test ./... -race -count=1                    # in mcp-kit; must succeed
```

---

## Phase 2: hmacstore (default ApprovalTokenStore)

**Goal:** Land the stateless HMAC-signed approval-token implementation. Lifts ~110 LOC from `vorrent/internal/api/mcp_oauth_authorize.go:336-419` with one structural change (param-stripping moved out of the digest's path so the same digest works for sessionstore too).

**Owner:** Phase-2 team. May run in parallel with Phase 3 (different files).

**Scope:**
- Create: `oauth/consent/hmacstore/doc.go`
- Create: `oauth/consent/hmacstore/store.go`
- Create: `oauth/consent/hmacstore/store_test.go`
- Create: `oauth/consent/params.go` (shared between hmacstore and sessionstore — lifted to parent package)
- Create: `oauth/consent/params_test.go`

---

### Task 2.1 — `ParamsDigest` + `oauthAuthorizeValues` + `normalizeOAuthStringSlice`

**Files:**
- Create: `oauth/consent/params.go`
- Create: `oauth/consent/params_test.go`

These three helpers are shared between `hmacstore` and `sessionstore`; both back-ends need to compute a stable canonical digest of the credential-stripped params.

- [ ] **Step 2.1.1 — Write the failing tests first**

  ```go
  // oauth/consent/params_test.go
  package consent

  import (
      "net/url"
      "testing"
  )

  // TestOAuthAuthorizeValues_StripsCredentialFields proves we never feed
  // username/password back to fosite or include them in the params digest.
  // Failure mode: a regression here would let a tampered approve POST
  // succeed by changing client_id while keeping the username constant.
  func TestOAuthAuthorizeValues_StripsCredentialFields(t *testing.T) {
      in := url.Values{
          "client_id":      {"abc"},
          "username":       {"alice@example.com"},
          "password":       {"hunter2"},
          "action":         {"login"},
          "approval_token": {"deadbeef"},
          "state":          {"xxxxxxxx"},
          "code_challenge": {"sha256-base64"},
          "resource":       {"https://example.com/mcp"},
      }
      out := oauthAuthorizeValues(in)
      for _, banned := range []string{"username", "password", "action", "approval_token"} {
          if _, has := out[banned]; has {
              t.Errorf("%q must be stripped", banned)
          }
      }
      for _, kept := range []string{"client_id", "state", "code_challenge", "resource"} {
          if _, has := out[kept]; !has {
              t.Errorf("%q must be retained", kept)
          }
      }
  }

  // TestParamsDigest_StableAcrossLoginAndApprove proves the digest is
  // computed only from canonical OAuth params, so the GET-after-login
  // (no credentials) and POST-on-approve (different action/approval_token)
  // produce the same digest for the same canonical request. Failure mode:
  // a regression here would invalidate every approval token immediately.
  func TestParamsDigest_StableAcrossLoginAndApprove(t *testing.T) {
      login := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}, "username": {"a"}}
      approve := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}, "approval_token": {"deadbeef"}}
      if ParamsDigest(login) != ParamsDigest(approve) {
          t.Fatalf("digest must be stable across credential / token field churn")
      }
  }

  // TestParamsDigest_DiffersOnCanonicalChange proves a tampered client_id
  // breaks the digest. Failure mode: a regression here would let an
  // attacker swap client_id mid-flow.
  func TestParamsDigest_DiffersOnCanonicalChange(t *testing.T) {
      a := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
      b := url.Values{"client_id": {"DIFFERENT"}, "state": {"xxxxxxxx"}}
      if ParamsDigest(a) == ParamsDigest(b) {
          t.Fatal("digest must change when canonical params change")
      }
  }

  // TestNormalizeOAuthStringSlice_DropsBlanksAndDupes proves we don't
  // smuggle empty resources past the RFC 8707 validator. Failure mode:
  // an attacker submits resource=&resource=https://evil/ and the second
  // value gets accepted because the empty was treated as a valid resource.
  func TestNormalizeOAuthStringSlice_DropsBlanksAndDupes(t *testing.T) {
      out := normalizeOAuthStringSlice([]string{"https://x/", "", "  ", "https://x/", "https://y/"})
      if len(out) != 2 || out[0] != "https://x/" || out[1] != "https://y/" {
          t.Fatalf("got %#v", out)
      }
  }
  ```

  Note: each test has a comment explaining the failure mode it guards. The plan rejects tests that "increase coverage" without naming a documented failure mode (see Testing Philosophy section).

- [ ] **Step 2.1.2 — Verify red, then implement `oauth/consent/params.go`**

  ```go
  package consent

  import (
      "crypto/sha256"
      "encoding/base64"
      "net/url"
      "strings"
  )

  // oauthAuthorizeValues returns a copy of values with credential, action,
  // and approval-token fields stripped. The result is what the kit feeds to
  // fosite (which expects canonical OAuth params only) and what ParamsDigest
  // hashes (so the digest is stable across the login → approve transition).
  func oauthAuthorizeValues(values url.Values) url.Values {
      clean := make(url.Values, len(values))
      for key, vals := range values {
          switch key {
          case "username", "password", "action", "approval_token", "challenge_id", "challenge_response":
              continue
          }
          for _, value := range vals {
              clean.Add(key, value)
          }
      }
      return clean
  }

  // ParamsDigest is a base64url-encoded SHA-256 of the canonical
  // (credential-free) OAuth params. Used inside the approval token so a
  // tampered approve POST is rejected even when the HMAC verifies.
  // Public so subpackage ApprovalTokenStore impls (hmacstore, sessionstore)
  // can compute a matching digest without re-implementing param scrubbing.
  func ParamsDigest(values url.Values) string {
      clean := oauthAuthorizeValues(values)
      sum := sha256.Sum256([]byte(clean.Encode()))
      return base64.RawURLEncoding.EncodeToString(sum[:])
  }

  // normalizeOAuthStringSlice trims, deduplicates, and drops empty values.
  // Used for resource indicator slice and any other repeated OAuth param
  // where order does not matter.
  func normalizeOAuthStringSlice(values []string) []string {
      out := make([]string, 0, len(values))
      seen := make(map[string]struct{}, len(values))
      for _, value := range values {
          trimmed := strings.TrimSpace(value)
          if trimmed == "" {
              continue
          }
          if _, ok := seen[trimmed]; ok {
              continue
          }
          seen[trimmed] = struct{}{}
          out = append(out, trimmed)
      }
      return out
  }
  ```

  Note: `challenge_id` and `challenge_response` are added to the strip list now even though Phase 1's ChallengeProvider doesn't ship a real impl — defining the canonical field names up-front prevents a future 2FA migration from changing the digest contract.

- [ ] **Step 2.1.3 — Verify green, commit**

  ```bash
  go test ./oauth/consent/ -run "TestOAuthAuthorizeValues|TestParamsDigest|TestNormalizeOAuthStringSlice" -race -v
  git add oauth/consent/params.go oauth/consent/params_test.go
  git commit -m "feat(oauth/consent): credential-stripping param scrubber + stable digest"
  ```

---

### Task 2.2 — `hmacstore.Store`

**Files:**
- Create: `oauth/consent/hmacstore/doc.go`
- Create: `oauth/consent/hmacstore/store.go`
- Create: `oauth/consent/hmacstore/store_test.go`

- [ ] **Step 2.2.1 — Write `doc.go`**

  ```go
  // Package hmacstore implements consent.ApprovalTokenStore using a
  // stateless HMAC-signed payload + an in-memory replay map.
  //
  // Token format:
  //
  //   base64url( subject_id "|" expires_at "|" params_digest "|" hmac )
  //
  // - subject_id      — string-form subject ID (oauth.Subject.ID).
  // - expires_at      — unix seconds.
  // - params_digest   — base64url(sha256(canonical OAuth params)).
  // - hmac            — base64url(HMAC-SHA256(key, payload-without-hmac)).
  //
  // The replay map ensures single-use redemption: even if a token decodes
  // and verifies, the second redemption attempt finds an empty slot.
  //
  // # Memory bound
  //
  // The map is bounded only by approvalTokenTTL × abandonment-rate.
  // Abandoned successful logins (user closes the tab between login and
  // approve) remain in the map until either the stale token is replayed
  // or the process restarts. For long-running deployments expecting
  // adversarial mint-but-never-consume behavior, a janitor goroutine
  // sweeping the map on a timer is a reasonable wrapper. The kit ships
  // none — N=0 production reports of OOM from this map.
  package hmacstore
  ```

- [ ] **Step 2.2.2 — Write the failing tests**

  ```go
  // oauth/consent/hmacstore/store_test.go
  package hmacstore

  import (
      "context"
      "crypto/rand"
      "net/url"
      "testing"
      "time"

      "github.com/google/uuid"

      "github.com/haakco/mcp-kit/oauth"
      "github.com/haakco/mcp-kit/oauth/consent"
  )

  // TestStore_RoundTrip is the happy path. Failure mode: any regression in
  // sign/verify symmetry breaks every consent flow.
  func TestStore_RoundTrip(t *testing.T) {
      key := mustRandKey(t)
      store := New(key, time.Now)
      sub := oauth.Subject{ID: uuid.NewString(), Email: "alice@example.com"}
      params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}

      tok, err := store.Issue(context.Background(), sub, params)
      if err != nil {
          t.Fatalf("issue: %v", err)
      }
      got, err := store.Consume(context.Background(), tok, params)
      if err != nil {
          t.Fatalf("consume: %v", err)
      }
      if got.ID != sub.ID {
          t.Fatalf("subject ID mismatch: got %q want %q", got.ID, sub.ID)
      }
  }

  // TestStore_RejectsReplay proves a second redemption fails. Failure mode:
  // a regression here means an attacker who steals an approval token can
  // mint multiple authorization codes from a single login.
  func TestStore_RejectsReplay(t *testing.T) {
      key := mustRandKey(t)
      store := New(key, time.Now)
      sub := oauth.Subject{ID: uuid.NewString()}
      params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
      tok, _ := store.Issue(context.Background(), sub, params)

      if _, err := store.Consume(context.Background(), tok, params); err != nil {
          t.Fatalf("first consume: %v", err)
      }
      if _, err := store.Consume(context.Background(), tok, params); err != consent.ErrApprovalTokenInvalid {
          t.Fatalf("second consume err = %v, want ErrApprovalTokenInvalid", err)
      }
  }

  // TestStore_RejectsExpired proves expiry is enforced. Failure mode: if
  // expiry is silently extended on Consume, a stolen token from yesterday
  // works today.
  func TestStore_RejectsExpired(t *testing.T) {
      key := mustRandKey(t)
      now := time.Now()
      clock := func() time.Time { return now }
      store := New(key, clock)
      sub := oauth.Subject{ID: uuid.NewString()}
      params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
      tok, _ := store.Issue(context.Background(), sub, params)

      now = now.Add(consent.ApprovalTokenTTL() + time.Second)

      if _, err := store.Consume(context.Background(), tok, params); err != consent.ErrApprovalTokenInvalid {
          t.Fatalf("expired consume err = %v, want ErrApprovalTokenInvalid", err)
      }
  }

  // TestStore_RejectsParamMismatch proves the params digest defends against
  // post-login client_id swap. Failure mode: an attacker who gets a victim
  // to log in for client A could otherwise redirect the consent for client B.
  func TestStore_RejectsParamMismatch(t *testing.T) {
      key := mustRandKey(t)
      store := New(key, time.Now)
      sub := oauth.Subject{ID: uuid.NewString()}
      original := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
      tampered := url.Values{"client_id": {"DIFFERENT"}, "state": {"xxxxxxxx"}}
      tok, _ := store.Issue(context.Background(), sub, original)

      if _, err := store.Consume(context.Background(), tok, tampered); err != consent.ErrApprovalTokenInvalid {
          t.Fatalf("mismatched consume err = %v, want ErrApprovalTokenInvalid", err)
      }
  }

  // TestStore_RejectsForgedSignature proves an attacker who knows the
  // payload format but not the HMAC key cannot mint tokens. Failure mode:
  // forged tokens accepted = full auth bypass.
  func TestStore_RejectsForgedSignature(t *testing.T) {
      keyA := mustRandKey(t)
      keyB := mustRandKey(t)
      store := New(keyA, time.Now)
      sub := oauth.Subject{ID: uuid.NewString()}
      params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
      tok, _ := store.Issue(context.Background(), sub, params)

      rogue := New(keyB, time.Now)
      if _, err := rogue.Consume(context.Background(), tok, params); err != consent.ErrApprovalTokenInvalid {
          t.Fatalf("forged-key consume err = %v, want ErrApprovalTokenInvalid", err)
      }
  }

  // TestStore_ConcurrentConsume proves -race cleanliness under concurrent
  // redemption attempts on the same token. Failure mode: a TOCTOU bug
  // could let two parallel consumers both succeed.
  func TestStore_ConcurrentConsume(t *testing.T) {
      key := mustRandKey(t)
      store := New(key, time.Now)
      sub := oauth.Subject{ID: uuid.NewString()}
      params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
      tok, _ := store.Issue(context.Background(), sub, params)

      results := make(chan error, 8)
      for i := 0; i < 8; i++ {
          go func() {
              _, err := store.Consume(context.Background(), tok, params)
              results <- err
          }()
      }

      successes, failures := 0, 0
      for i := 0; i < 8; i++ {
          if err := <-results; err == nil {
              successes++
          } else {
              failures++
          }
      }
      if successes != 1 || failures != 7 {
          t.Fatalf("got %d successes / %d failures; want exactly 1 success", successes, failures)
      }
  }

  func mustRandKey(t *testing.T) []byte {
      t.Helper()
      key := make([]byte, 32)
      if _, err := rand.Read(key); err != nil {
          t.Fatalf("rand: %v", err)
      }
      return key
  }
  ```

  Note: the tests reference `consent.ApprovalTokenTTL()` (a getter we expose so tests don't depend on the unexported constant). Add it to `oauth/consent/config.go`:

  ```go
  // ApprovalTokenTTL returns the bound on approval-token lifetime. Tests
  // and operators inspecting policy use this rather than hard-coding.
  func ApprovalTokenTTL() time.Duration { return approvalTokenTTL }
  ```

- [ ] **Step 2.2.3 — Write `oauth/consent/hmacstore/store.go`**

  ```go
  package hmacstore

  import (
      "context"
      "crypto/hmac"
      "crypto/sha256"
      "encoding/base64"
      "net/url"
      "strconv"
      "strings"
      "sync"
      "time"

      "github.com/haakco/mcp-kit/oauth"
      "github.com/haakco/mcp-kit/oauth/consent"
  )

  // Store is a stateless HMAC + in-memory replay map implementation of
  // consent.ApprovalTokenStore.
  type Store struct {
      key []byte
      now func() time.Time

      mu     sync.Mutex
      issued map[string]time.Time
  }

  // New constructs a Store with the supplied HMAC key (must be 32 bytes;
  // the kit's consent.NewHandler validates this before constructing) and
  // clock. Pass time.Now in production.
  func New(key []byte, now func() time.Time) *Store {
      if now == nil {
          now = time.Now
      }
      return &Store{key: key, now: now, issued: make(map[string]time.Time)}
  }

  // Issue implements consent.ApprovalTokenStore.
  func (s *Store) Issue(_ context.Context, subject oauth.Subject, params url.Values) (string, error) {
      expiresAt := s.now().Add(consent.ApprovalTokenTTL()).Unix()
      payload := subject.ID + "|" + strconv.FormatInt(expiresAt, 10) + "|" + consent.ParamsDigest(params)
      signature := s.sign(payload)
      token := base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + signature))

      s.mu.Lock()
      s.issued[token] = time.Unix(expiresAt, 0)
      s.mu.Unlock()
      return token, nil
  }

  // Consume implements consent.ApprovalTokenStore.
  func (s *Store) Consume(_ context.Context, token string, params url.Values) (oauth.Subject, error) {
      if token == "" {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      decoded, err := base64.RawURLEncoding.DecodeString(token)
      if err != nil {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      parts := strings.Split(string(decoded), "|")
      if len(parts) != 4 {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      payload := strings.Join(parts[:3], "|")
      want := s.sign(payload)
      if !hmac.Equal([]byte(parts[3]), []byte(want)) {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
      if err != nil {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      if s.now().Unix() > expiresAt {
          s.mu.Lock()
          delete(s.issued, token)
          s.mu.Unlock()
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      if parts[2] != consent.ParamsDigest(params) {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      if !s.consumeStored(token) {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      // Subject reconstructed from token has only ID; consumers re-resolve
      // additional fields after Consume by calling Authenticator-side
      // logic in a future flow if needed. The Handler does NOT re-fetch —
      // the approval token is the trust anchor for the subject.
      return oauth.Subject{ID: parts[0]}, nil
  }

  func (s *Store) consumeStored(token string) bool {
      s.mu.Lock()
      defer s.mu.Unlock()
      expiresAt, ok := s.issued[token]
      if !ok {
          return false
      }
      delete(s.issued, token)
      return s.now().Before(expiresAt)
  }

  func (s *Store) sign(payload string) string {
      mac := hmac.New(sha256.New, s.key)
      _, _ = mac.Write([]byte(payload))
      return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
  }
  ```

  Note: `consent.ParamsDigest` (uppercase, public) is defined in Task 2.1 above. The other helpers (`oauthAuthorizeValues`, `normalizeOAuthStringSlice`) stay private — they're used only by the kit-internal Handler.

- [ ] **Step 2.2.4 — Verify**

  ```bash
  go test ./oauth/consent/hmacstore/ -race -v
  go test ./oauth/consent/ -race -v
  go vet ./oauth/consent/...
  ```

  Expected: all green, no `-race` reports.

- [ ] **Step 2.2.5 — Commit**

  ```bash
  git add oauth/consent/hmacstore oauth/consent/params.go oauth/consent/params_test.go oauth/consent/config.go
  git commit -m "feat(oauth/consent/hmacstore): stateless HMAC ApprovalTokenStore + replay map"
  ```

---

## Phase 3: sessionstore (opt-in ApprovalTokenStore for skills-mcp / Laravel-style consumers)

**Goal:** Land the session-backed implementation. Mirrors Laravel Passport's `pull from session` pattern. Skills-mcp will adopt this in its consumer migration plan.

**Owner:** Phase-3 team. May run in parallel with Phase 2 (different files).

**Scope:**
- Create: `oauth/consent/sessionstore/doc.go`
- Create: `oauth/consent/sessionstore/backend.go` (the consumer-supplied interface + `MemoryBackend` for tests)
- Create: `oauth/consent/sessionstore/store.go`
- Create: `oauth/consent/sessionstore/store_test.go`

---

### Task 3.1 — `SessionBackend` interface

- [ ] **Step 3.1.1 — Write `oauth/consent/sessionstore/backend.go`**

  ```go
  // Package sessionstore implements consent.ApprovalTokenStore using a
  // consumer-supplied session backend. The OAuth params + subject are
  // stored under a random opaque key; on Consume the key is "pulled" —
  // single-use is enforced by the backend, not by an in-process map.
  //
  // This pattern matches Laravel Passport's session-stored auth_token and
  // is the right choice for consumers who already have a session backend
  // (e.g. skills-mcp's apps/skills-mcp/internal/auth/session.go). For
  // stateless / multi-process / serverless deployments, prefer hmacstore.
  package sessionstore

  import (
      "context"
      "errors"
      "sync"
      "time"
  )

  // ErrNotFound is returned by SessionBackend.Pull when the key is not in
  // storage (or has expired, or has been pulled previously).
  var ErrNotFound = errors.New("sessionstore: not found")

  // Entry is the payload sessionstore puts into the backend. The backend
  // serializes/deserializes it however it wants — JSON in a database row,
  // a serialized session struct, etc.
  //
  // Backends MUST treat Entry as a black box: do not introspect or modify
  // fields, do not log values (Params and SubjectID may include client
  // identifiers, but never credentials — those are stripped before Put).
  type Entry struct {
      // SubjectID is oauth.Subject.ID at issue time.
      SubjectID string

      // SubjectExtra is oauth.Subject.Extra at issue time. May be nil.
      // Backends serializing must handle empty maps; on deserialization,
      // nil and empty map are equivalent.
      SubjectExtra map[string]any

      // Params is the credential-stripped OAuth request as filtered by
      // consent.oauthAuthorizeValues.
      Params map[string][]string

      // ExpiresAt is when the entry must be treated as expired regardless
      // of backend TTL. The backend SHOULD also enforce its own TTL.
      ExpiresAt time.Time
  }

  // SessionBackend is implemented by the consumer. Methods must be safe
  // for concurrent use.
  //
  // Single-use semantics: Pull MUST atomically delete-and-return. A backend
  // that returns the Entry but leaves it in storage breaks the kit's replay
  // protection.
  type SessionBackend interface {
      // Put stores entry under key for at most ttl. Returns nil on success.
      Put(ctx context.Context, key string, entry Entry, ttl time.Duration) error

      // Pull atomically deletes-and-returns the entry under key. Returns
      // ErrNotFound when the key is absent, expired, or already pulled.
      Pull(ctx context.Context, key string) (Entry, error)
  }

  // MemoryBackend is an in-process SessionBackend for tests and simple
  // single-instance deployments. Production multi-instance servers MUST
  // implement a backend backed by their session store (DB, Redis, etc.).
  type MemoryBackend struct {
      mu    sync.Mutex
      items map[string]memoryItem
      now   func() time.Time
  }

  type memoryItem struct {
      entry     Entry
      expiresAt time.Time
  }

  // NewMemoryBackend returns a MemoryBackend. Pass time.Now in production.
  func NewMemoryBackend(now func() time.Time) *MemoryBackend {
      if now == nil {
          now = time.Now
      }
      return &MemoryBackend{items: make(map[string]memoryItem), now: now}
  }

  func (b *MemoryBackend) Put(_ context.Context, key string, entry Entry, ttl time.Duration) error {
      b.mu.Lock()
      defer b.mu.Unlock()
      b.items[key] = memoryItem{entry: entry, expiresAt: b.now().Add(ttl)}
      return nil
  }

  func (b *MemoryBackend) Pull(_ context.Context, key string) (Entry, error) {
      b.mu.Lock()
      defer b.mu.Unlock()
      item, ok := b.items[key]
      if !ok {
          return Entry{}, ErrNotFound
      }
      delete(b.items, key)
      if b.now().After(item.expiresAt) {
          return Entry{}, ErrNotFound
      }
      return item.entry, nil
  }
  ```

- [ ] **Step 3.1.2 — Verify and commit**

  ```bash
  go build ./oauth/consent/sessionstore/
  git add oauth/consent/sessionstore/backend.go
  git commit -m "feat(oauth/consent/sessionstore): SessionBackend interface + MemoryBackend"
  ```

---

### Task 3.2 — `sessionstore.Store`

- [ ] **Step 3.2.1 — Write the failing tests**

  ```go
  // oauth/consent/sessionstore/store_test.go
  package sessionstore

  import (
      "context"
      "net/url"
      "reflect"
      "testing"
      "time"

      "github.com/google/uuid"

      "github.com/haakco/mcp-kit/oauth"
      "github.com/haakco/mcp-kit/oauth/consent"
  )

  func TestStore_RoundTrip(t *testing.T) {
      backend := NewMemoryBackend(time.Now)
      store := New(backend, time.Now)
      sub := oauth.Subject{
          ID:    uuid.NewString(),
          Email: "alice@example.com",
          Extra: map[string]any{"role": "admin"},
      }
      params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}

      tok, err := store.Issue(context.Background(), sub, params)
      if err != nil {
          t.Fatalf("issue: %v", err)
      }
      got, err := store.Consume(context.Background(), tok, params)
      if err != nil {
          t.Fatalf("consume: %v", err)
      }
      if got.ID != sub.ID || got.Email != sub.Email {
          t.Fatalf("subject mismatch: got %+v want %+v", got, sub)
      }
      if !reflect.DeepEqual(got.Extra, sub.Extra) {
          t.Fatalf("Extra mismatch: got %+v want %+v", got.Extra, sub.Extra)
      }
  }

  // TestStore_RejectsReplay proves the backend's Pull is single-use.
  // Failure mode: a backend that returns-but-doesn't-delete breaks replay
  // protection.
  func TestStore_RejectsReplay(t *testing.T) {
      backend := NewMemoryBackend(time.Now)
      store := New(backend, time.Now)
      sub := oauth.Subject{ID: uuid.NewString()}
      params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
      tok, _ := store.Issue(context.Background(), sub, params)

      if _, err := store.Consume(context.Background(), tok, params); err != nil {
          t.Fatalf("first consume: %v", err)
      }
      if _, err := store.Consume(context.Background(), tok, params); err != consent.ErrApprovalTokenInvalid {
          t.Fatalf("second consume err = %v, want ErrApprovalTokenInvalid", err)
      }
  }

  // TestStore_RejectsParamMismatch proves the params digest stored in the
  // backend entry detects post-issue tampering.
  func TestStore_RejectsParamMismatch(t *testing.T) {
      backend := NewMemoryBackend(time.Now)
      store := New(backend, time.Now)
      sub := oauth.Subject{ID: uuid.NewString()}
      original := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
      tampered := url.Values{"client_id": {"DIFFERENT"}, "state": {"xxxxxxxx"}}
      tok, _ := store.Issue(context.Background(), sub, original)

      if _, err := store.Consume(context.Background(), tok, tampered); err != consent.ErrApprovalTokenInvalid {
          t.Fatalf("mismatched consume err = %v, want ErrApprovalTokenInvalid", err)
      }
  }

  // TestStore_RejectsExpired proves entries past ExpiresAt are rejected
  // even if the backend hasn't expunged them yet.
  func TestStore_RejectsExpired(t *testing.T) {
      now := time.Now()
      clock := func() time.Time { return now }
      backend := NewMemoryBackend(clock)
      store := New(backend, clock)
      sub := oauth.Subject{ID: uuid.NewString()}
      params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
      tok, _ := store.Issue(context.Background(), sub, params)

      now = now.Add(consent.ApprovalTokenTTL() + time.Second)

      if _, err := store.Consume(context.Background(), tok, params); err != consent.ErrApprovalTokenInvalid {
          t.Fatalf("expired consume err = %v, want ErrApprovalTokenInvalid", err)
      }
  }
  ```

- [ ] **Step 3.2.2 — Implement `oauth/consent/sessionstore/store.go`**

  ```go
  package sessionstore

  import (
      "context"
      "crypto/rand"
      "encoding/base64"
      "errors"
      "net/url"
      "time"

      "github.com/haakco/mcp-kit/oauth"
      "github.com/haakco/mcp-kit/oauth/consent"
  )

  // Store is a session-backed implementation of consent.ApprovalTokenStore.
  type Store struct {
      backend SessionBackend
      now     func() time.Time
  }

  // New constructs a Store backed by backend.
  func New(backend SessionBackend, now func() time.Time) *Store {
      if now == nil {
          now = time.Now
      }
      return &Store{backend: backend, now: now}
  }

  // Issue stores a new entry under a 32-byte random key, returns the key.
  func (s *Store) Issue(ctx context.Context, subject oauth.Subject, params url.Values) (string, error) {
      key, err := newKey()
      if err != nil {
          return "", err
      }
      entry := Entry{
          SubjectID:    subject.ID,
          SubjectExtra: subject.Extra,
          Params:       cloneValues(params),
          ExpiresAt:    s.now().Add(consent.ApprovalTokenTTL()),
      }
      if err := s.backend.Put(ctx, key, entry, consent.ApprovalTokenTTL()); err != nil {
          return "", err
      }
      // Stash the params digest with the token so Consume validates the
      // canonical params even if the backend leaks the entry contents.
      // (Not strictly needed since we re-encode and compare on consume,
      // but keeps the consume path symmetric with hmacstore.)
      return key + "." + consent.ParamsDigest(params), nil
  }

  // Consume validates the digest, pulls the entry, validates expiry +
  // params, and returns the stored subject.
  func (s *Store) Consume(ctx context.Context, token string, params url.Values) (oauth.Subject, error) {
      key, digest, ok := splitToken(token)
      if !ok {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      if digest != consent.ParamsDigest(params) {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      entry, err := s.backend.Pull(ctx, key)
      if errors.Is(err, ErrNotFound) {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      if err != nil {
          return oauth.Subject{}, err
      }
      if s.now().After(entry.ExpiresAt) {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      // Re-validate digest against the stored params just in case the
      // backend was tampered with. Compute the digest from
      // url.Values(entry.Params).
      stored := url.Values(entry.Params)
      if consent.ParamsDigest(stored) != digest {
          return oauth.Subject{}, consent.ErrApprovalTokenInvalid
      }
      // Reconstruct the Subject — backend stores ID + Extra; Email is not
      // round-tripped (consumers refresh from their user table after
      // approval if they need it).
      return oauth.Subject{
          ID:    entry.SubjectID,
          Extra: entry.SubjectExtra,
      }, nil
  }

  func newKey() (string, error) {
      buf := make([]byte, 32)
      if _, err := rand.Read(buf); err != nil {
          return "", err
      }
      return base64.RawURLEncoding.EncodeToString(buf), nil
  }

  func splitToken(token string) (key, digest string, ok bool) {
      idx := indexLastByte(token, '.')
      if idx < 0 {
          return "", "", false
      }
      return token[:idx], token[idx+1:], true
  }

  func indexLastByte(s string, b byte) int {
      for i := len(s) - 1; i >= 0; i-- {
          if s[i] == b {
              return i
          }
      }
      return -1
  }

  func cloneValues(in url.Values) map[string][]string {
      out := make(map[string][]string, len(in))
      for k, v := range in {
          dup := make([]string, len(v))
          copy(dup, v)
          out[k] = dup
      }
      return out
  }
  ```

- [ ] **Step 3.2.3 — Verify, commit**

  ```bash
  go test ./oauth/consent/sessionstore/ -race -v
  git add oauth/consent/sessionstore
  git commit -m "feat(oauth/consent/sessionstore): session-backed ApprovalTokenStore for Passport-style consumers"
  ```

---

## Phase 4: Handler

**Goal:** Land `consent.Handler` and `consent.NewHandler`. End-to-end: GET renders login, POST(login) validates credentials and renders consent, POST(approve) issues code, POST(deny) emits ErrAccessDenied. Audit fires on approve/deny. RFC 8707 enforced. Synthetic-GET works against real fosite.

**Owner:** Phase-4 team. Cannot run in parallel with itself; depends on Phases 1, 2, 3.

**Scope:**
- Create: `oauth/consent/request.go` (synthetic-GET)
- Create: `oauth/consent/validate.go` (resource indicator validator)
- Create: `oauth/consent/error.go` (RFC 6749 envelope)
- Create: `oauth/consent/redirect.go` (RedirectURLFromResponder, ClientNameFromRequester, HiddenInputs)
- Create: `oauth/consent/audit.go` (event emission)
- Create: `oauth/consent/handler.go` (the wire-up)
- Create: `oauth/consent/handler_test.go` (the canonical seven-test suite + 4 extras)
- Modify: `oauth/handlers.go` lines 13-15 (docstring tighten)

---

### Task 4.1 — `BuildAuthorizeRequest` (synthetic-GET)

- [ ] **Step 4.1.1 — Write `oauth/consent/request.go`**

  Lifted from `vorrent/internal/api/mcp_oauth_authorize.go:290-313`.

  ```go
  package consent

  import (
      "context"
      "net/http"
      "net/url"

      "github.com/ory/fosite"

      "github.com/haakco/mcp-kit/oauth"
  )

  // buildAuthorizeRequest re-creates the canonical OAuth request fosite
  // expects. fosite reads r.URL.Query() regardless of HTTP method, so for
  // POST flows we build a synthetic GET request using the form params and
  // the configured public URL.
  //
  // The original request's headers are preserved (minus Content-Type and
  // Content-Length, which would mislead fosite about the synthetic
  // request's body). This keeps middleware-derived state — e.g. an Origin
  // header previously validated by mcp-kit's origin allowlist — available
  // to downstream handlers if they consult it.
  func buildAuthorizeRequest(
      ctx context.Context,
      provider *oauth.Provider,
      publicURL string,
      formPath string,
      cleanParams url.Values,
      original *http.Request,
  ) (fosite.AuthorizeRequester, error) {
      rawURL := publicURL + formPath + "?" + cleanParams.Encode()
      clone, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
      if err != nil {
          return nil, fosite.ErrServerError.WithWrap(err)
      }
      if original != nil {
          clone.Header = original.Header.Clone()
          clone.Header.Del("Content-Type")
          clone.Header.Del("Content-Length")
      }
      return provider.OAuth2Provider().NewAuthorizeRequest(ctx, clone)
  }
  ```

- [ ] **Step 4.1.2 — Commit**

  ```bash
  git add oauth/consent/request.go
  git commit -m "feat(oauth/consent): synthetic-GET helper for fosite-compatible POST flows"
  ```

---

### Task 4.2 — Resource indicator validator (RFC 8707)

- [ ] **Step 4.2.1 — Write `oauth/consent/validate.go`**

  Lifted from `vorrent/internal/api/mcp_oauth_authorize.go:437-455`, parameterized by a `ResourceURLFunc`.

  ```go
  package consent

  import (
      "log/slog"
      "net/http"
      "net/url"

      "github.com/ory/fosite"
  )

  // validateResourceIndicators enforces RFC 8707 strict-when-present:
  //
  //   - No `resource` param  → accept (older clients, bind canonical URL
  //                            server-side later).
  //   - One `resource` param → must equal the canonical resource URL for
  //                            this request; otherwise reject.
  //   - Multiple `resource`  → reject (kit allows exactly one).
  //
  // The strict-when-present posture matches MCP 2025-06-18 guidance and
  // is what cb's OAuthResourceIndicatorMiddleware enforces.
  func validateResourceIndicators(
      r *http.Request,
      params url.Values,
      resourceURL ResourceURLFunc,
      logger *slog.Logger,
  ) error {
      resources := normalizeOAuthStringSlice(params["resource"])
      if len(resources) == 0 {
          return nil
      }
      expected, err := resourceURL(r)
      if err != nil {
          return fosite.ErrInvalidRequest.WithHint("invalid mcp host")
      }
      if len(resources) != 1 || resources[0] != expected {
          logger.Warn("consent: authorize rejected: resource indicator mismatch",
              "client_id", params.Get("client_id"),
              "provided_resources", resources,
              "expected_resource", expected,
          )
          return fosite.ErrInvalidRequest.WithHint("authorization request must target the current MCP resource")
      }
      return nil
  }
  ```

- [ ] **Step 4.2.2 — Commit**

  ```bash
  git add oauth/consent/validate.go
  git commit -m "feat(oauth/consent): RFC 8707 strict-when-present resource indicator validator"
  ```

---

### Task 4.3 — Redirect + form helpers

- [ ] **Step 4.3.1 — Write `oauth/consent/redirect.go`**

  ```go
  package consent

  import (
      "net/url"
      "strings"

      "github.com/ory/fosite"
  )

  // ClientNameFromRequester returns the client's display name, falling back
  // to the client_id, falling back to "your MCP client".
  func ClientNameFromRequester(req fosite.AuthorizeRequester) string {
      if req == nil || req.GetClient() == nil {
          return "your MCP client"
      }
      client := req.GetClient()
      if provider, ok := client.(interface{ GetName() string }); ok {
          if name := strings.TrimSpace(provider.GetName()); name != "" {
              return name
          }
      }
      if id := strings.TrimSpace(client.GetID()); id != "" {
          return id
      }
      return "your MCP client"
  }

  // RedirectURLFromResponder builds the final redirect URL the browser
  // bounces to: the client's registered redirect_uri with the OAuth
  // response parameters layered onto it.
  //
  // Returns "" when the requester has no redirect URI (caller falls back
  // to fosite's WriteAuthorizeResponse).
  func RedirectURLFromResponder(req fosite.AuthorizeRequester, resp fosite.AuthorizeResponder) string {
      redirectURI := req.GetRedirectURI()
      if redirectURI == nil {
          return ""
      }
      out := *redirectURI
      query := out.Query()
      for key, values := range resp.GetParameters() {
          delete(query, key)
          for _, value := range values {
              query.Add(key, value)
          }
      }
      out.RawQuery = query.Encode()
      return out.String()
  }

  // HiddenInputs is the kit-side helper for templates: maps url.Values to
  // []HiddenInput so renderers iterate without depending on url.Values.
  func HiddenInputs(values url.Values) []HiddenInput {
      out := make([]HiddenInput, 0, len(values))
      for key, vals := range values {
          for _, value := range vals {
              out = append(out, HiddenInput{Name: key, Value: value})
          }
      }
      return out
  }

  // ResourcesFromValues is a convenience for renderers: returns the
  // normalized + deduped resource params, or nil.
  func ResourcesFromValues(values url.Values) []string {
      out := normalizeOAuthStringSlice(values["resource"])
      if len(out) == 0 {
          return nil
      }
      return out
  }

  // ArgumentsToStrings copies a fosite.Arguments to []string.
  func ArgumentsToStrings(args fosite.Arguments) []string {
      if len(args) == 0 {
          return nil
      }
      out := make([]string, len(args))
      copy(out, args)
      return out
  }
  ```

- [ ] **Step 4.3.2 — Commit**

  ```bash
  git add oauth/consent/redirect.go
  git commit -m "feat(oauth/consent): redirect + form + scope helpers"
  ```

---

### Task 4.4 — `WriteAuthorizeError` (RFC 6749 JSON envelope)

- [ ] **Step 4.4.1 — Write `oauth/consent/error.go`**

  Lifted from `vorrent/internal/api/mcp_oauth_authorize.go:69-81`.

  ```go
  package consent

  import (
      "encoding/json"
      "net/http"

      "github.com/ory/fosite"
  )

  // WriteAuthorizeError writes an RFC 6749 JSON error envelope to w. Used
  // for hard failures before fosite has a redirect_uri to redirect to
  // (synthetic GET construction failures, RFC 8707 mismatches, malformed
  // form bodies). For redirectable errors, callers use
  // provider.OAuth2Provider().WriteAuthorizeError instead.
  //
  // Cache-Control: no-store and Pragma: no-cache are set per RFC 6749
  // section 5.1 to prevent intermediaries from caching error responses.
  //
  // Public so consumer-implemented error paths inside Renderer can call it.
  func WriteAuthorizeError(w http.ResponseWriter, err error) {
      rfc := fosite.ErrorToRFC6749Error(err)
      status := rfc.StatusCode()
      if status == 0 {
          status = http.StatusBadRequest
      }
      w.Header().Set("Cache-Control", "no-store")
      w.Header().Set("Pragma", "no-cache")
      w.Header().Set("Content-Type", "application/json;charset=UTF-8")
      w.WriteHeader(status)
      _ = json.NewEncoder(w).Encode(rfc)
  }
  ```

- [ ] **Step 4.4.2 — Commit**

  ```bash
  git add oauth/consent/error.go
  git commit -m "feat(oauth/consent): RFC 6749 error envelope encoder"
  ```

---

### Task 4.5 — Audit emission

- [ ] **Step 4.5.1 — Write `oauth/consent/audit.go`**

  ```go
  package consent

  import (
      "context"
      "net/http"
      "strings"
      "time"

      "github.com/google/uuid"
      "github.com/ory/fosite"

      "github.com/haakco/mcp-kit/audit"
      "github.com/haakco/mcp-kit/oauth"
  )

  // emitConsentEvent fires oauth.consent.{approved,denied} on
  // h.cfg.AuditEmitter. Naming + payload matches Laravel cb's
  // OAuthConsentAuditData for cross-language log correlation:
  //
  //   - EntityType  = "oauth_authorize"
  //   - EntityID    = client_id
  //   - Action      = "oauth.consent.approved" | "oauth.consent.denied"
  //   - ActorUserID = subject.ID parsed as UUID, nil otherwise
  //   - ClientID    = client_id (also in Metadata as "client_name")
  //   - Scope       = space-joined granted scopes
  //   - Metadata    = {"client_name", "decision", "ip", "user_agent",
  //                    "resources" (RFC 8707 if any)}
  //
  // Errors from emitter.Emit are silently dropped (audit is best-effort —
  // a failed audit emission does not fail authorize). Operators concerned
  // about audit reliability wrap their emitter in a retry shim.
  func (h *Handler) emitConsentEvent(
      ctx context.Context,
      r *http.Request,
      action string,
      requester fosite.AuthorizeRequester,
      subject oauth.Subject,
      decision string,
      resources []string,
  ) {
      clientID := ""
      clientName := ""
      if requester != nil && requester.GetClient() != nil {
          clientID = requester.GetClient().GetID()
          clientName = ClientNameFromRequester(requester)
      }
      scope := ""
      if requester != nil {
          scope = strings.Join(ArgumentsToStrings(requester.GetRequestedScopes()), " ")
      }

      event := audit.Event{
          EntityType: "oauth_authorize",
          EntityID:   clientID,
          Action:     action,
          ClientID:   clientID,
          Scope:      scope,
          Timestamp:  h.cfg.Now().UTC(),
          Metadata: map[string]any{
              "client_name": clientName,
              "decision":    decision,
              "ip":          clientIP(r),
              "user_agent":  r.UserAgent(),
              "resources":   resources,
          },
      }
      if subject.ID != "" {
          if u, err := uuid.Parse(subject.ID); err == nil {
              event.ActorUserID = &u
          }
      }
      _ = h.cfg.AuditEmitter.Emit(ctx, event)
  }

  // clientIP returns the request's perceived client IP (best-effort, no
  // X-Forwarded-For parsing — operators behind proxies wrap their emitter
  // to enrich).
  func clientIP(r *http.Request) string {
      if r == nil {
          return ""
      }
      // RemoteAddr is "ip:port" — strip the port. Don't parse XFF here:
      // every proxy chain is different, and trusting headers blindly is a
      // spoofing risk. Consumers that want XFF wrap their emitter.
      if idx := strings.LastIndex(r.RemoteAddr, ":"); idx > 0 {
          return r.RemoteAddr[:idx]
      }
      return r.RemoteAddr
  }

  // Action constants — exposed so consumer-side audit pipelines can match.
  const (
      ActionConsentApproved = "oauth.consent.approved"
      ActionConsentDenied   = "oauth.consent.denied"
  )
  ```

  Note: this file imports `time`, `strings`, `audit`, `oauth`, `fosite`, `uuid`, `http`, `context` — all used by `emitConsentEvent` and `clientIP`. No defensive `var _ = ...` placeholders.

- [ ] **Step 4.5.2 — Commit**

  ```bash
  git add oauth/consent/audit.go
  git commit -m "feat(oauth/consent): audit emission with cb-aligned event names + payload"
  ```

---

### Task 4.6 — `Handler` + `NewHandler`

This is the largest task. Approach: write the failing test suite first (TDD red), then implement.

- [ ] **Step 4.6.1 — Write `oauth/consent/handler_test.go` (canonical seven + four extras)**

  Each test names the failure mode it guards. No "constructor doesn't panic" test, no "field exists" test.

  ```go
  // oauth/consent/handler_test.go
  package consent_test

  import (
      "context"
      "io"
      "net/http"
      "net/http/httptest"
      "net/url"
      "strings"
      "sync/atomic"
      "testing"
      "time"

      "github.com/google/uuid"
      "github.com/ory/fosite"

      "github.com/haakco/mcp-kit/audit"
      "github.com/haakco/mcp-kit/oauth"
      "github.com/haakco/mcp-kit/oauth/consent"
      "github.com/haakco/mcp-kit/oauth/consent/hmacstore"
  )

  // The test suite uses external test package (consent_test) so it exercises
  // only the public API. Helpers below.

  // --- 7 canonical tests every consumer should pass ---

  // TestHandler_GET_RendersLoginPage proves the kit's GET path delegates to
  // the renderer with PageLogin and the OAuth params propagated.
  // Failure mode: a GET that doesn't render login = users see fosite's
  // raw error page.
  func TestHandler_GET_RendersLoginPage(t *testing.T) { /* ... */ }

  // TestHandler_POST_LoginBadPassword proves a wrong password re-renders
  // PageLogin with an error and does NOT mint an approval token.
  // Failure mode: a regression that mints a token on bad credentials =
  // full auth bypass.
  func TestHandler_POST_LoginBadPassword(t *testing.T) { /* ... */ }

  // TestHandler_POST_LoginGoodPassword_RendersConsent proves a successful
  // login renders PageConsent with an approval_token hidden input.
  // Failure mode: skipping consent = users grant scopes they didn't see.
  func TestHandler_POST_LoginGoodPassword_RendersConsent(t *testing.T) { /* ... */ }

  // TestHandler_POST_ApproveGoodToken_RedirectsWithCode proves the approve
  // path exchanges an approval token for a fosite authorization code.
  // Failure mode: a regression here means the kit cannot complete a single
  // OAuth flow.
  func TestHandler_POST_ApproveGoodToken_RedirectsWithCode(t *testing.T) { /* ... */ }

  // TestHandler_POST_ApproveExpiredToken_RendersLogin proves an expired
  // token re-renders PageLogin with an error, not a 500.
  // Failure mode: a 500 on expiry = users hit a wall.
  func TestHandler_POST_ApproveExpiredToken_RendersLogin(t *testing.T) { /* ... */ }

  // TestHandler_POST_Deny_EmitsErrAccessDenied proves explicit deny short-
  // circuits to fosite.ErrAccessDenied and emits oauth.consent.denied.
  // Failure mode: a deny that issues a code = ignored user choice = trust
  // failure.
  func TestHandler_POST_Deny_EmitsErrAccessDenied(t *testing.T) { /* ... */ }

  // TestHandler_POST_ApprovalReplay_Rejected proves an approval token can
  // only be redeemed once. Failure mode: stolen approval token = unlimited
  // tokens minted from one login.
  func TestHandler_POST_ApprovalReplay_Rejected(t *testing.T) { /* ... */ }

  // --- 4 extras that guard against regressions surfaced in vorrent /
  // skills-mcp / cb / tlm ---

  // TestHandler_RFC8707_RejectsMismatchedResource proves resource= must
  // match the canonical URL when the client sends one.
  // Failure mode: cross-audience token confusion.
  func TestHandler_RFC8707_RejectsMismatchedResource(t *testing.T) { /* ... */ }

  // TestHandler_AuditEvents_Approved_Denied proves audit events fire with
  // the cb-aligned payload. Failure mode: no audit on consent decision =
  // post-incident forensics blocked.
  func TestHandler_AuditEvents_Approved_Denied(t *testing.T) { /* ... */ }

  // TestHandler_RejectsShortApprovalSecret proves NewHandler refuses to
  // start with a 1-byte operator-supplied secret. Failure mode: weak HMAC
  // = forge approval tokens trivially.
  func TestHandler_RejectsShortApprovalSecret(t *testing.T) {
      _, err := consent.NewHandler(consent.Config{
          Provider:       testProvider(t, "https://app.example.com"),
          Authenticator:  consent.AuthenticatorFunc(func(context.Context, string, string) (oauth.Subject, error) { return oauth.Subject{}, nil }),
          Renderer:       consent.RendererFunc(func(http.ResponseWriter, consent.Page, consent.PageData) {}),
          PublicURL:      "https://app.example.com",
          ApprovalSecret: []byte{0x01},
      })
      if err == nil {
          t.Fatal("expected NewHandler to reject 1-byte ApprovalSecret")
      }
      if !strings.Contains(err.Error(), "exactly 32 bytes") {
          t.Fatalf("error does not mention 32-byte requirement: %v", err)
      }
  }

  // TestHandler_ConsentPolicy_AllowsSkip proves a skip-consent ConsentPolicy
  // is honored — PageConsent is never rendered, the code is issued
  // immediately after login. Failure mode: kit ignoring policy = first-
  // party clients show consent screens unnecessarily.
  func TestHandler_ConsentPolicy_AllowsSkip(t *testing.T) { /* ... */ }

  // --- helpers ---

  func testProvider(t testing.TB, issuer string) *oauth.Provider { /* ... */ }
  func registerTestClient(t testing.TB, p *oauth.Provider, clientID, redirect string) { /* ... */ }
  func s256Challenge(verifier string) string { /* ... */ }
  ```

  **Implementer note:** the test bodies are sketched in the canonical-suite list above. Full code blocks for each test live in `oauth/consent/consenttest/RunCanonicalSuite` (Phase 5). Phase 4 ships them inline first, then Phase 5 lifts them into the fixture package and the kit's own tests delegate to `consenttest.RunCanonicalSuite(t, h, opts)`.

  **DO NOT** add a `TestHandler_NewHandler_DoesNotPanic` or `TestHandler_FieldsExist` — those test the wrong thing. If the constructor errors, that's caught by the next test that calls it. If a field doesn't exist, `go build` fails before tests run.

- [ ] **Step 4.6.2 — Verify red, then implement `oauth/consent/handler.go`**

  ```go
  package consent

  import (
      "context"
      "crypto/rand"
      "errors"
      "net/http"
      "net/url"

      "github.com/ory/fosite"

      "github.com/haakco/mcp-kit/oauth"
      "github.com/haakco/mcp-kit/oauth/consent/hmacstore"
  )

  // Handler is the kit's canonical /oauth/authorize handler. Wires
  // Authenticator, Renderer, ApprovalTokenStore, ConsentPolicy, and
  // optional ChallengeProvider to produce a fosite authorization code.
  type Handler struct {
      cfg Config
  }

  // NewHandler validates cfg, wires defaults, and returns a Handler. Errors
  // when a required field is missing or an explicit ApprovalSecret is the
  // wrong length.
  func NewHandler(cfg Config) (*Handler, error) {
      if err := cfg.applyDefaults(); err != nil {
          return nil, err
      }
      if cfg.ApprovalStore == nil {
          if len(cfg.ApprovalSecret) == 0 {
              cfg.ApprovalSecret = make([]byte, approvalSecretLength)
              if _, err := rand.Read(cfg.ApprovalSecret); err != nil {
                  return nil, err
              }
              cfg.Logger.Warn("consent: approval secret not configured; generated ephemeral secret. Set Config.ApprovalSecret to persist across restarts.")
          } else if err := ValidateApprovalSecret(cfg.ApprovalSecret); err != nil {
              return nil, err
          }
          cfg.ApprovalStore = hmacstore.New(cfg.ApprovalSecret, cfg.Now)
      }
      return &Handler{cfg: cfg}, nil
  }

  // ServeHTTP implements http.Handler.
  func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
      switch r.Method {
      case http.MethodGet:
          h.renderLogin(w, r, r.URL.Query(), "")
      case http.MethodPost:
          h.handlePost(w, r)
      default:
          http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
      }
  }

  func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request) {
      r.Body = http.MaxBytesReader(w, r.Body, h.cfg.FormBodyLimit)
      if err := r.ParseForm(); err != nil {
          http.Error(w, "invalid form body", http.StatusBadRequest)
          return
      }
      cleanParams := oauthAuthorizeValues(r.PostForm)
      requester, err := buildAuthorizeRequest(r.Context(), h.cfg.Provider, h.cfg.PublicURL, h.cfg.FormPath, cleanParams, r)
      if err != nil {
          WriteAuthorizeError(w, err)
          return
      }
      if err := validateResourceIndicators(r, cleanParams, h.cfg.ResourceURL, h.cfg.Logger); err != nil {
          WriteAuthorizeError(w, err)
          return
      }
      switch r.PostForm.Get("action") {
      case "login":
          h.handleLogin(w, r, requester, cleanParams)
      case "approve":
          h.handleApprove(w, r, requester, cleanParams)
      case "deny":
          // best-effort cleanup; the deny response does not depend on
          // approval-token validity (a stale or missing token still denies).
          _, _ = h.cfg.ApprovalStore.Consume(r.Context(), r.PostForm.Get("approval_token"), cleanParams)
          h.emitConsentEvent(r.Context(), r, ActionConsentDenied, requester, oauth.Subject{}, "denied", ResourcesFromValues(cleanParams))
          h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, requester, fosite.ErrAccessDenied)
      default:
          h.renderLogin(w, r, cleanParams, "unknown authorization action")
      }
  }

  func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request, requester fosite.AuthorizeRequester, cleanParams url.Values) {
      username := r.PostForm.Get("username")
      password := r.PostForm.Get("password")
      if username == "" || password == "" {
          h.renderLogin(w, r, cleanParams, "username and password are required")
          return
      }
      subject, err := h.cfg.Authenticator.Authenticate(r.Context(), username, password)
      if err != nil {
          h.renderLogin(w, r, cleanParams, "invalid email or password")
          return
      }
      if err := h.cfg.ConsentPolicy.ValidateScopes(r.Context(), subject, ArgumentsToStrings(requester.GetRequestedScopes())); err != nil {
          h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, requester, fosite.ErrAccessDenied.WithHint(err.Error()))
          return
      }
      // Skip-consent path: ConsentPolicy says first-party. Issue immediately.
      if h.cfg.ConsentPolicy.AllowsSkip(r.Context(), requester.GetClient(), subject, ArgumentsToStrings(requester.GetRequestedScopes())) {
          h.completeApprove(w, r, requester, cleanParams, subject)
          return
      }
      // Optional challenge step.
      if h.cfg.Challenge != nil {
          challengeID, err := h.cfg.Challenge.Begin(r.Context(), subject)
          if errors.Is(err, ErrChallengeRequired) {
              h.renderLoginWithChallenge(w, r, cleanParams, challengeID)
              return
          }
          if err != nil {
              WriteAuthorizeError(w, fosite.ErrServerError.WithWrap(err))
              return
          }
      }
      // Render PageConsent with an approval token.
      token, err := h.cfg.ApprovalStore.Issue(r.Context(), subject, cleanParams)
      if err != nil {
          WriteAuthorizeError(w, fosite.ErrServerError.WithWrap(err))
          return
      }
      withToken := cloneAndSet(cleanParams, "approval_token", token)
      h.cfg.Renderer.Render(w, PageConsent, PageData{
          Authenticated: true,
          DisplayName:   subject.Email,
          ClientName:    ClientNameFromRequester(requester),
          Scopes:        ArgumentsToStrings(requester.GetRequestedScopes()),
          Resources:     ResourcesFromValues(withToken),
          HiddenInputs:  HiddenInputs(withToken),
          FormAction:    h.cfg.FormPath,
      })
  }

  func (h *Handler) handleApprove(w http.ResponseWriter, r *http.Request, requester fosite.AuthorizeRequester, cleanParams url.Values) {
      subject, err := h.cfg.ApprovalStore.Consume(r.Context(), r.PostForm.Get("approval_token"), cleanParams)
      if err != nil {
          h.renderLogin(w, r, cleanParams, "approval expired; sign in again")
          return
      }
      h.completeApprove(w, r, requester, cleanParams, subject)
  }

  func (h *Handler) completeApprove(w http.ResponseWriter, r *http.Request, requester fosite.AuthorizeRequester, cleanParams url.Values, subject oauth.Subject) {
      requestedScopes := ArgumentsToStrings(requester.GetRequestedScopes())
      if err := h.cfg.ConsentPolicy.ValidateScopes(r.Context(), subject, requestedScopes); err != nil {
          h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, requester, fosite.ErrAccessDenied.WithHint(err.Error()))
          return
      }
      for _, scope := range requestedScopes {
          requester.GrantScope(scope)
      }

      // Bind canonical resource server-side regardless of client RFC 8707
      // support — every issued token is correctly audience-bound.
      expectedResource, err := h.cfg.ResourceURL(r)
      if err != nil {
          h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, requester, fosite.ErrInvalidRequest.WithHint("invalid mcp host"))
          return
      }
      requester.GrantAudience(expectedResource)

      session := oauth.NewSession(subject)
      responder, err := h.cfg.Provider.OAuth2Provider().NewAuthorizeResponse(r.Context(), requester, session)
      if err != nil {
          h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, requester, err)
          return
      }
      h.emitConsentEvent(r.Context(), r, ActionConsentApproved, requester, subject, "approved", ResourcesFromValues(cleanParams))

      redirectURL := RedirectURLFromResponder(requester, responder)
      if redirectURL == "" {
          h.cfg.Provider.OAuth2Provider().WriteAuthorizeResponse(r.Context(), w, requester, responder)
          return
      }
      // Renderer owns the bridge HTML. If it writes nothing, fall back to a
      // 302 — never leave the browser hanging.
      tracker := &trackingWriter{ResponseWriter: w}
      h.cfg.Renderer.Render(tracker, PageRedirectBridge, PageData{
          ClientName:  ClientNameFromRequester(requester),
          RedirectURL: redirectURL,
      })
      if !tracker.wrote {
          http.Redirect(w, r, redirectURL, http.StatusFound)
      }
  }

  func (h *Handler) renderLogin(w http.ResponseWriter, r *http.Request, params url.Values, msg string) {
      h.cfg.Renderer.Render(w, PageLogin, PageData{
          ClientName:   "your MCP client", // re-resolved from cleanParams.client_id by future enrichment if needed
          Scopes:       nil,
          Resources:    ResourcesFromValues(params),
          HiddenInputs: HiddenInputs(params),
          FormAction:   h.cfg.FormPath,
          Error:        msg,
      })
  }

  func (h *Handler) renderLoginWithChallenge(w http.ResponseWriter, r *http.Request, params url.Values, challengeID string) {
      withChallenge := cloneAndSet(params, "challenge_id", challengeID)
      h.cfg.Renderer.Render(w, PageLogin, PageData{
          HiddenInputs: HiddenInputs(withChallenge),
          FormAction:   h.cfg.FormPath,
          Error:        "additional verification required",
      })
  }

  func cloneAndSet(in url.Values, key, value string) url.Values {
      out := make(url.Values, len(in)+1)
      for k, v := range in {
          dup := make([]string, len(v))
          copy(dup, v)
          out[k] = dup
      }
      out.Set(key, value)
      return out
  }

  // trackingWriter remembers whether anything was written. Used to detect a
  // renderer that returned without writing PageRedirectBridge content so
  // the kit falls back to a 302.
  type trackingWriter struct {
      http.ResponseWriter
      wrote bool
  }

  func (t *trackingWriter) Write(p []byte) (int, error) {
      t.wrote = t.wrote || len(p) > 0
      return t.ResponseWriter.Write(p)
  }

  func (t *trackingWriter) WriteHeader(code int) {
      t.wrote = true
      t.ResponseWriter.WriteHeader(code)
  }
  ```

- [ ] **Step 4.6.3 — Verify all 11 tests pass with `-race`**

  ```bash
  go test ./oauth/consent/... -race -count=1 -v
  ```

  Expected: 11/11 PASS. Any flake = blocker.

- [ ] **Step 4.6.4 — Commit**

  ```bash
  git add oauth/consent/handler.go oauth/consent/handler_test.go oauth/consent/audit.go
  git commit -m "feat(oauth/consent): Handler + NewHandler + canonical 11-test suite"
  ```

---

### Task 4.7 — Update `Provider.AuthorizeHandler` docstring

**Files:**
- Modify: `oauth/handlers.go` lines 13-15

- [ ] **Step 4.7.1 — Tighten the docstring**

  Replace lines 13-15 with:

  ```go
  // AuthorizeHandler returns an OAuth authorization endpoint that grants
  // requested scopes immediately to whatever oauth.Subject SubjectResolver
  // returns.
  //
  // This is a DEMO handler. It does not authenticate the browser session,
  // does not collect explicit user consent, and does not bind the canonical
  // resource audience server-side per RFC 8707. Production servers MUST
  // replace it with a handler that does all three. See the oauth/consent
  // package and oauth/README.md "Replacing the demo authorize handler" for
  // the kit's recommended path.
  //
  // The handler is retained for tests, examples, and incremental migration.
  // It MAY be renamed in v1.0.0; consumers should plan to migrate to
  // oauth/consent.NewHandler.
  ```

  Function signature, body, behavior — all unchanged.

- [ ] **Step 4.7.2 — Verify no behavior change**

  ```bash
  go test ./oauth/... -count=1 -race
  ```

  Expected: green; the existing `oauth/handlers_test.go` tests pass unchanged.

- [ ] **Step 4.7.3 — Commit**

  ```bash
  git add oauth/handlers.go
  git commit -m "docs(oauth): warn AuthorizeHandler is demo-only; point to oauth/consent"
  ```

---

### Phase 4 Sign-off Gate

End-of-phase verification:

```bash
go build ./...
go vet ./...
go test ./... -race -count=1
cd /tmp/phase0-vorrent && go build ./...
cd /tmp/phase0-skills/apps/skills-mcp && go build ./...
```

All five must succeed. The two cross-consumer builds confirm the additive `Subject.Extra` and the doc-only `AuthorizeHandler` change did not break vorrent or skills-mcp.

Three-stage review:
1. Spec compliance — all 11 tests guard a documented failure mode; canonical seven match the cross-consumer audit.
2. Code quality — `cloneAndSet` and `trackingWriter` are minimal, single-responsibility; no premature abstractions.
3. Security — `-race` is clean; `ApprovalSecret` length is enforced; no log lines contain secret material; nil ChallengeProvider does not leak through.

---

## Phase 5: Test Fixtures (`oauth/consent/consenttest`)

**Goal:** Save ~150 LOC of test boilerplate per consumer. Codify the canonical seven-test contract as a single function consumers call.

**Owner:** Phase-5 team. May run in parallel with Phase 6.

**Files:**
- Create: `oauth/consent/consenttest/doc.go`
- Create: `oauth/consent/consenttest/server.go`
- Create: `oauth/consent/consenttest/auth.go`
- Create: `oauth/consent/consenttest/render.go`
- Create: `oauth/consent/consenttest/transport.go`
- Create: `oauth/consent/consenttest/suite.go`
- Create: `oauth/consent/consenttest/consenttest_test.go`

(Detailed per-task steps follow the same pattern as Phase 1–4: write the failing test or signature, implement, verify, commit. Brief outline:)

| Task | What it ships | Why |
|---|---|---|
| 5.1 | `Provider(t, issuer) *oauth.Provider` | One-liner kit-provider construction backed by `oauth/storage.NewMemoryStore` + `keys.NewManager(keys.NewMemoryStore())`. |
| 5.2 | `RegisterClient(t, p, opts ClientOptions)` | Push a fosite client into the memory store. |
| 5.3 | `StaticAuth(creds map[string]oauth.Subject)` and `DenyAll()` | Authenticator fakes — explicit, no hidden state. |
| 5.4 | `CapturingRenderer` | Records last `(Page, PageData)` pair for assertions. |
| 5.5 | Transport helpers — `NoFollowClient`, `S256Challenge`, `HiddenInputValue`, `RedirectOrContinue` | The ~5 lines every test would otherwise duplicate. |
| 5.6 | `RunCanonicalSuite(t, h *consent.Handler, opts SuiteOptions)` | Runs the canonical seven tests via `t.Run` subtests. Consumer calls one function, gets 7 sub-test names in their CI output. |
| 5.7 | `consenttest_test.go` self-test | Builds an in-memory `Handler`, runs `RunCanonicalSuite`, must pass. Validates the fixture itself. |

Each task ends with `git add` + `git commit -m "test(oauth/consent): ..."`. Final commit:

```bash
git commit -m "test(oauth/consent): consenttest fixture package + RunCanonicalSuite"
```

---

## Phase 6: Documentation

**Goal:** Future Go MCP servers find the right path in 30 minutes, not 2 days. Both reference consumers (vorrent, skills-mcp) get a one-page migration sketch.

**Owner:** Phase-6 team. May run in parallel with Phase 5.

**Files:**
- Create: `oauth/README.md`
- Modify: `README.md` (kit root) — Quickstart code block at line 70
- Modify: `CHANGELOG.md` — add entry under `## Unreleased`
- Create: `docs/recipes/oauth-consent.md` — pattern doc (matches `docs/recipes/admin-gate.md`'s style)

### Task 6.1 — `oauth/README.md`

Sections:
1. **Overview** — what the package owns, what each subpackage does (`hmacstore`, `sessionstore`, `consenttest`).
2. **Replacing the demo authorize handler** — 50-line worked example using `consent.NewHandler` with `hmacstore`.
3. **Authenticator contract** — return `oauth.Subject` (with `Extra` for role/etc.); error mapping; no enumeration leaks.
4. **Renderer contract** — three pages, fields you must render, fields you may ignore, the zero-bytes-→-302 fallback.
5. **ApprovalTokenStore — picking a backend** — decision flowchart: stateless? hmacstore. Already have a session backend? sessionstore. Side-by-side latency/storage/memory tradeoff.
6. **ConsentPolicy** — when to skip consent (first-party clients), when to gate scopes (admin scopes for non-admin users); concrete vorrent + cb examples.
7. **ChallengeProvider** — interface defined for future 2FA; not implemented in v0.5; consumer wraps Authenticator if they need it today.
8. **Audit events** — `oauth.consent.{approved,denied}` payload, when emitted, no PII in event.
9. **RFC 8707 enforcement** — strict-when-present semantics, why server-side audience binding is always-on.
10. **Operator considerations** — `ApprovalSecret` 32-byte rule, hmacstore memory bound + janitor wrapper sketch, sessionstore TTL contract.
11. **Migration sketches:**
    - From vorrent's hand-rolled `mcp_oauth_authorize.go` → `consent.NewHandler` + `hmacstore.New(secret, time.Now)`. Code diff: ~480 LOC removed, ~30 added.
    - From skills-mcp's session-cookie + redirect-to-login → `consent.NewHandler` + `sessionstore.New(skillsSessionBackend, time.Now)`. Code diff: ~360 LOC removed, ~50 added.
12. **Testing your handler** — `consenttest.RunCanonicalSuite(t, h, opts)` plus when to write your own additional tests.

### Task 6.2 — Update root `README.md` Quickstart

Replace line 70:

```go
mux.Handle("/oauth/authorize", oauthProv.AuthorizeHandler(myapp.ResolveSubject))
```

with:

```go
authorize, err := consent.NewHandler(consent.Config{
    Provider:      oauthProv,
    Authenticator: myapp.NewAuthenticator(db),         // your credential check
    Renderer:      myapp.NewConsentRenderer(),         // your HTML
    PublicURL:     "https://my-mcp.example.com",
    ApprovalSecret: myapp.OAuthApprovalSecret(),       // 32 bytes
    AuditEmitter:  myapp.NewAuditEmitter(db),
})
if err != nil { /* handle */ }
mux.Handle("/oauth/authorize", authorize)
```

Add an import line for `"github.com/haakco/mcp-kit/oauth/consent"`.

### Task 6.3 — `CHANGELOG.md` entry under `## Unreleased`

```markdown
### Added

- `oauth/consent` — opinionated authorization-endpoint handler shared across
  Go MCP servers. One `Handler` shape, five interfaces (`Authenticator`,
  `Renderer`, `ApprovalTokenStore`, `ConsentPolicy`, `ChallengeProvider`),
  two stock `ApprovalTokenStore` impls (`hmacstore` for stateless flows,
  `sessionstore` for consumers with an existing session backend).
- `oauth/consent/consenttest` — `RunCanonicalSuite` runs the seven canonical
  tests every consumer should pass.
- `oauth.Subject.Extra map[string]any` — additive field, propagated to OIDC
  session claims via `oauth.NewSession`. Existing consumers unaffected.
- `oauth.consent.{approved,denied}` audit event names — cross-language
  alignment with the Laravel cb / tlm Passport implementations.

### Changed

- `Provider.AuthorizeHandler` docstring tightened to call out demo nature
  and point at `oauth/consent`. Behavior unchanged. Marked as a candidate
  for rename / removal in v1.0.0.

### Notes

- Vorrent (v0.4.0) and skills-mcp (v0.3.0) continue to compile without
  source changes; their migrations to `consent.NewHandler` are tracked as
  separate per-consumer plans.
- `oauth/middleware.go` already emits `WWW-Authenticate: ..., resource_metadata="..."`
  on 401 (verified in this release; spec-compliant for MCP 2025-06-18).
```

### Task 6.4 — `docs/recipes/oauth-consent.md`

Follow the style of `docs/recipes/admin-gate.md`. Cross-link to:
- `oauth/README.md` for the API.
- `docs/lessons.md` `OG-04` (PKCE base64url substitution) and any new lessons surfaced during Phases 1–5.

Final commit for Phase 6:

```bash
git add oauth/README.md README.md CHANGELOG.md docs/recipes/oauth-consent.md
git commit -m "docs(oauth/consent): replace-the-demo guide + Quickstart + CHANGELOG + recipe"
```

---

## Phase 6 Sign-off Gate (final)

End-of-plan verification:

```bash
# 1. Kit-side
go build ./... && go vet ./... && go test ./... -race -count=1

# 2. Cross-consumer additive check
cd /tmp/phase0-vorrent && go build ./...
cd /tmp/phase0-skills/apps/skills-mcp && go build ./...

# 3. End-to-end smoke against in-memory provider
go test ./oauth/consent/consenttest -run TestConsenttest_SelfTest -race -count=3 -v

# 4. Doc accuracy
test -f oauth/README.md
test -f docs/recipes/oauth-consent.md
grep -q "consent.NewHandler" README.md
```

All four gates must pass. If any fails, the plan is not done — fix and re-verify per the ownership rule.

---

## Testing Philosophy (Non-Negotiable)

The user's directive: tests must verify real functionality, not "tests for testing sake". Every test in this plan adheres to:

**Required**

1. **Each test names the failure mode it guards.** Comment at the top of every test function: "Failure mode: ...". If you can't name a real operator-visible failure, the test should not exist.
2. **Tests use real fosite, real HMAC, real `httptest.ResponseRecorder`, real `httpguard.NewRequest`.** No mocking the kit's internal collaborators (fosite, audit, oauth.Provider).
3. **`-race` clean.** Every package with concurrent state has a concurrent-access test (`TestStore_ConcurrentConsume` is the template).
4. **End-to-end coverage at least once.** `TestHandler_POST_ApproveGoodToken_RedirectsWithCode` exercises Authenticator → ApprovalStore → Handler → fosite → redirect URL in one test. No path is exercised only at unit granularity.
5. **Public-API tests live in `package_test` packages.** `oauth/consent/handler_test.go` uses `package consent_test`. Forces the test to use only the exported surface — catches accidental API gaps.

**Forbidden**

1. **No "constructor doesn't panic" tests.** Constructor errors are caught by the next test that calls it; a panicking constructor blocks the entire suite anyway.
2. **No "field exists" or "method exists" tests.** `go build` catches missing symbols.
3. **No tests that re-test fosite's responsibilities** (PKCE verification, code single-use, refresh-token rotation). Those belong in fosite's own test suite.
4. **No mocks of internal kit packages.** A mock that drifts is worse than no mock — it tests the mock, not the kit. If you can't write the test against the real type, the type is wrong.
5. **No tests added "to bump coverage."** Coverage is a side-effect of testing real failure modes, not a goal.

**The 11 Phase-4 tests + 6 hmacstore tests + 4 sessionstore tests = 21 tests total** for the entire `oauth/consent` package. Each names a failure mode. Each survives the next refactor. None tests an implementation detail.

---

## Project-wide Improvement Punch List (out of scope; tracked for follow-up)

The user asked for "what other improvements we can do for this project." The plan above ships `oauth/consent`. The items below surfaced during the cross-consumer survey but are out of scope for this plan — each becomes a separate plan tracked by the master plan.

1. **`golangci-lint` config** — the kit ships no lint config; CLAUDE.md notes "no formal lint config." `~/.claude/CLAUDE.md` rules on errcheck + revive should land as a `.golangci.yml`. Effort: 1h.
2. **CI smoke test** (per `docs/lessons.md` EG-02) — boot the kit's `_examples/minimal-server`, run a curl-based authenticated handshake (`PR-02`), assert protocol shape. Currently manual. Effort: 4h.
3. **Rate-limit middleware** — skills-mcp ships its own (`apps/skills-mcp/internal/oidc/handlers.go:77-96`); the kit doesn't. Three consumers need it; promote to kit. Effort: 1d.
4. **Error-code redaction logger helper** — skills-mcp's `logging.RedactFositeError` (referenced in handlers.go) prevents PII leaks in logs. Reusable across consumers. Effort: 0.5d.
5. **Janitor goroutine for `hmacstore`** — only if a real consumer reports memory bloat from abandoned approvals. Skip until that signal arrives.
6. **`_examples/oauth-consent-server/`** — end-to-end demo of `consent.NewHandler` wired against an in-memory user table + a one-file Renderer. Useful but doc-only. Effort: 0.5d.
7. **CORS wrapping for OAuth endpoints** — skills-mcp wraps with explicit method allowlists; the kit relies on the consumer doing this. Document the requirement OR ship a `cors.WrapOAuth(provider)` helper. Effort: depends on path; document-only is 1h.
8. **`docs/migration/meridian.md`** — third Go consumer on the kit's roadmap (master plan Phase 9). Drafted from vorrent's migration template once the meridian port lands.
9. **2FA `ChallengeProvider` implementation** — TOTP + recovery codes, modeled on cb. Defer until a Go consumer needs it.

These are NOT in this plan. Adding them mid-phase = scope creep. They get separate plans.

---

## Coordination Notes

- **Branch creation requires explicit user approval.** Coordinator confirms before `git checkout -b`. Suggested name: `oauth-consent-helpers`.
- **One implementer subagent per phase.** Each implementer self-reviews before reporting. The coordinator dispatches review subagents (spec → quality → security) per `~/.claude/CLAUDE.md`.
- **Never push.** Final commits land on the local branch only. User reviews and pushes manually.
- **Surgical scope.** Each task lists its files. Implementers must not touch files outside the listed set.
- **No `git stash`. No `git reset` of any kind.** No exceptions.
- **Cross-consumer build check at every phase end.** Vorrent and skills-mcp must continue to compile.
- **Questions go up, not sideways.** Implementer → coordinator → user. No implementer-to-implementer chat.

---

## Estimated Effort

| Phase | Tasks | Implementer | Review |
|---|---|---|---|
| Phase 0 — spec audit + baseline | 4 | 0.25h | 0.25h |
| Phase 1 — core types + interfaces | 10 | 4h | 2h |
| Phase 2 — hmacstore | 2 | 3h | 1h |
| Phase 3 — sessionstore | 2 | 3h | 1h |
| Phase 4 — Handler + docstring | 7 | 5h | 2h |
| Phase 5 — consenttest fixtures | 7 | 4h | 1h |
| Phase 6 — docs | 4 | 3h | 1h |
| **Total** | **36** | **~22h implementer** | **~8h review** |

Phases 2 and 3 in parallel after Phase 1; Phases 5 and 6 in parallel after Phase 4. Wall-clock with three implementers + the review gauntlet: **~14–18 hours**.

---

## Completion Checklist (Non-Negotiable)

Per `~/.claude/CLAUDE.md`:

- [ ] **Readable** — A new contributor reads `oauth/README.md` + `oauth/consent/doc.go` and writes a working `consent.NewHandler` call in under 15 minutes.
- [ ] **Linted** — `go vet ./...` clean; once a `.golangci.yml` ships (separate plan), `golangci-lint run ./...` clean.
- [ ] **Code quality** — Zero duplication between `oauth/consent` and the kit's existing `oauth` package internals.
- [ ] **Tested** — All 21 Phase-1–4 tests pass with `-race`. `consenttest.RunCanonicalSuite` self-test green.
- [ ] **All problems fixed** — No TODOs left in the package. No "will fix later" comments. No defensive `var _ = ...` import-keepers.
- [ ] **Consistent** — Naming matches kit conventions (`oauth.Provider`, `audit.Emitter`, `consent.Authenticator`).
- [ ] **Simple** — `oauth/consent` total LOC < 900 across handler + 5 helper files + hmacstore + sessionstore + consenttest.
- [ ] **Named well** — `Authenticator`, `Renderer`, `ApprovalTokenStore`, `ConsentPolicy`, `ChallengeProvider`, `Page`, `PageData` — all reveal intent without comments.
- [ ] **No hacks** — No silent fallbacks. Operator-supplied short approval secret returns `error`, does not auto-fix.
- [ ] **Documented** — `oauth/README.md`, `docs/recipes/oauth-consent.md`, `CHANGELOG.md` entry, package + symbol godocs.
- [ ] **System working** — `go test ./...` green at the kit root. **Vorrent + skills-mcp builds green** against the new kit branch.

If any check fails: stop, update this plan with the failure mode + correct long-term fix, execute the update, re-run the full checklist. Repeat until all items pass.
