# OAuth Consent Helpers — Implementation Plan

**Goal:** Add a reusable, project-agnostic login + consent layer (`oauth/consent`) to mcp-kit so future MCP servers stop reimplementing the security-critical OAuth `/authorize` handler from scratch.

**Background:** The kit ships `Provider.AuthorizeHandler` documented as "consumers are responsible for authenticating the browser request and collecting consent" (see `oauth/handlers.go:13-16`). Two consumers — vorrent (`internal/api/mcp_oauth_*.go`) and meridian (`backend/internal/server/mcp_oauth_*.go`, commit `meridian@8e07600e3`) — independently ported the **same** pattern. Comparing the two files produced a near byte-for-byte match on ~60% of the code: HMAC-signed approval tokens, params digest, replay store, synthetic-GET trick, RFC 8707 strict-when-present validator, hidden-input form helper, redirect-URL stitching, and the RFC 6749 error JSON encoder. Meridian additionally hardened its port (single-use replay test, `kitaudit.Emitter` integration on approve+deny, server-derived resource URL, approval-secret length validation). Without an upstream, every new MCP server pays a ~2 engineering-day reverse-engineering cost and silently drifts.

**Architecture:** Ship a new `oauth/consent` Go package providing the security-critical primitives both consumers duplicated, plus a `consent.Handler` (`http.Handler`) that wires them together. Consumers plug in three things: a credential-checker (`Authenticator` interface that returns the existing `oauth.Subject`), an HTML renderer, and an optional `audit.Emitter`. The kit owns the protocol-correct token format, replay store, audience binding, and resource validation; the consumer owns user model and UI. Default-template and worked-example subpackages are deferred to a later release once a third consumer informs the design.

**Tech Stack:** Go 1.22+, `github.com/ory/fosite` (already a kit dep), `github.com/google/uuid` (already a kit dep transitively), `crypto/hmac`/`crypto/sha256` (stdlib), `net/url` (stdlib). NO `html/template` in the core — that lives in the deferred `oauth/consent/template` subpackage.

**Tech Stack (test):** Standard `testing` + `httptest`; the kit's existing `oauth/storage.MemoryStore` and `oauth/keys.Manager` testkit utilities; no new test dependencies.

**Parallel Work Model:**
- **Phase 1 (core)** is one team's work end-to-end — tasks are tightly coupled (each builds on the prior types) so concurrency inside Phase 1 is unsafe.
- **Phase 2 (test fixtures)** can start as soon as Phase 1 Task 1.6 lands (Authenticator interface + Handler types are stable from that point).
- **Phase 3 (docs)** can run in parallel with Phase 2 once Phase 1 is complete — the writer reads the implemented public API, not the implementation.
- **Phase 4 (BrowserAuthorizeHandler interface)** is a small additive change to `oauth/handlers.go` and `oauth/provider.go` and can run in parallel with Phase 3.
- All teams work on the same branch (TBD: e.g. `oauth-consent-helpers`). **Shared-branch rules apply**: never `git stash`, never `git reset` of any kind, never overwrite files outside assigned scope. Coordinator confirms branch state between phases.

---

## Open Decisions (Block Phase 1 — confirm before launching team)

The user pre-approved Decision Q1 (Option C: agnostic core ships now, default-template subpackage deferred). Decisions Q2–Q4 below remain open and are baked into this plan as the recommended path; flip them before kickoff if needed.

### Q2 — Subject contract

**Question:** What does `consent.Authenticator.Authenticate(...)` return?

| Option | Shape | Pros | Cons |
|---|---|---|---|
| **A** *(recommended)* | Return existing `oauth.Subject` | Kit stays user-model-agnostic. Consumers (vorrent ints, meridian UUIDs) stringify their IDs on the way in. Reuses the `Subject.GrantedScopes` field for scope filtering. | None material — this is the existing kit contract. |
| **B** | New `consent.User` struct with name/email/extra fields | Richer rendering data passed straight through to template | Forces kit to take a position on user-shape; meridian uses `firstName/lastName/email`, vorrent uses `email`-only. Drift back-pressure on the kit. |
| **C** | Generic `consent.Handler[T any]` parameterized over a user type | Maximum flexibility | Generics churn for no real gain — the only thing the kit needs is a string ID. |

**Recommendation:** **Option A.** Aligns with `README.md:23` ("It does not ship a user table"). Consumers who want richer template data put it in `Subject.Email` or extend their renderer.

### Q3 — Deprecation pace for `Provider.AuthorizeHandler`

**Question:** Do we rename the existing demo handler now, or wait?

| Option | Action | Pros | Cons |
|---|---|---|---|
| **A** *(recommended)* | Tighten the docstring on `Provider.AuthorizeHandler` to point to `oauth/consent` and `oauth/README.md`; keep the name. | Zero-churn for in-flight consumers (skills-mcp, vorrent, meridian). Documents the truth. | Future readers may still pick the demo path by skim. |
| **B** | Rename to `DemoAuthorizeHandler` immediately. Add `AuthorizeHandler` as a deprecated alias for one minor. | Forcing function — every consumer acknowledges they're using the demo. | Touches every consumer's wiring twice (alias rename now, alias removal later). |
| **C** | Remove the demo entirely; force consumers onto the new package. | Clean surface. | Pre-1.0 but consumers exist; needless break. |

**Recommendation:** **Option A**, with a note in `CHANGELOG.md` and a deprecation candidate flagged for the v1.0.0 release.

### Q4 — Audit emission inside `consent.Handler`

**Question:** Does the kit emit `oauth_authorize.{approved,denied}` audit events from `consent.Handler` when a `kitaudit.Emitter` is provided?

| Option | Action | Pros | Cons |
|---|---|---|---|
| **A** *(recommended)* | Optional `Emitter` field on `consent.Handler`; emit on approve/deny when set; `audit.Discard()` if nil. | Standardises a security-relevant event across projects. Matches the kit's existing `mcpkit.New(... AuditEmitter ...)` pattern (`README.md:62`). Picks up meridian's hardening for vorrent's next pass for free. | One more wire in the consumer's setup. |
| **B** | Consumer emits audits inside their `Authenticator.Authenticate` and from their renderer when they detect denied. | Maximum flexibility. | Drift restored — vorrent currently emits nothing, meridian emits both. The whole point of upstreaming is convergence. |

**Recommendation:** **Option A** — the kit already exposes `kitaudit.Emitter` as the canonical contract; `consent.Handler` should consume it.

---

## Current State (Verified)

**Files examined directly:**

- `oauth/handlers.go` (148 LOC) — exposes `AuthorizeHandler`, `TokenHandler`, `RevokeHandler`, `RegisterRoutes`. The authorize handler is the *demo* (lines 13-16 docstring; the only check is state ≥ 8 + valid S256 challenge before grant). `grantSubjectScopes` and `grantDefaultAudience` helpers (122-147) are reusable as-is.
- `oauth/provider.go` (102 LOC) — `Provider` struct holds `oauth (fosite.OAuth2Provider)`, `audience`, `allowedScopes`. Exposes `OAuth2Provider()`, `RegisterHandler()`. Will gain `MountAuthorize(h BrowserAuthorizeHandler)` in Phase 4.
- `oauth/session.go` (38 LOC) — `Subject{ID, Email, GrantedScopes}` and `NewSession`/`NewEmptySession`. The `Subject` type is the right contract for `consent.Authenticator` (Q2 Option A).
- `oauth/config.go` (66 LOC) — note the `applyDefaults` pattern and especially the secret-length validation at lines 50-52 (`oauth secret must be exactly 32 bytes`). Phase 1 mirrors this for the approval secret.
- `oauth/error.go` (16 LOC) — has `writeOAuthErrorBody`. The new `consent.WriteAuthorizeError` is closely related; keep both for now (different shapes — `writeOAuthErrorBody` is a flat code/description map; the consent helper writes a fosite RFC 6749 envelope).
- `oauth/storage/memory.go`, `oauth/storage/storage.go` — `Store` interface + memory implementation. Used unchanged by Phase 2 fixtures.
- `audit/emitter.go` (66 LOC) — `Event` and `Emitter` interface. `EntityType` "oauth_authorize", `Action` "approved"/"denied" — exactly the shape meridian already uses; no audit-type seed required.
- `testkit/server.go` (109 LOC) — model for the `consenttest` package shape.
- `README.md` (113 LOC) — voice / framing reference. Quickstart section already mentions `oauthProv.AuthorizeHandler(myapp.ResolveSubject)` (line 70) — Phase 3 updates this snippet.
- `docs/plans/README.md` and `docs/plans/2026-05-01_mcp-kit_master_plan.md` exist; this plan is a focused chunk that fits inside that roadmap (the master plan tracks the v0.x → v1.0 arc; this delivers a slice).

**Reference consumers (do NOT modify in this plan):**

- `meridian/backend/internal/server/mcp_oauth_authorize.go` (621 LOC, commit `8e07600e3`) — the closer of the two ports to canonical; Phase 1 lifts almost every helper from here. Lines 399-485 = approval-token machinery; lines 487-621 = shared helpers (`oauthAuthorizeValues`, `cloneValues`, `resourcesFromValues`, `normalizeOAuthStringSlice`, `argumentsToStrings`, `clientNameFromRequester`, `authorizeRedirectURL`, `displayName`, `sessionForUser`, `writeOAuthAuthorizeRequestError`).
- `meridian/backend/internal/server/mcp_oauth_template.go` — meridian's HTML template (kept consumer-side since templating stays out of the kit core).
- `vorrent/internal/api/mcp_oauth_authorize.go` — older port; pattern source. Diverges only on user-model and audit emission.

**Key findings:**
- Both ports use `fmt.Sprintf("%d", expiresAt)` and `fmt.Sscanf(parts[1], "%d", &expiresAt)`. Phase 1 ships `strconv.FormatInt`/`strconv.ParseInt` instead — established kit idiom (`oauth/config.go` uses `strconv` already; the consumers are an idiom regression).
- Both ports use a `sync.Mutex` map keyed by token string for replay protection. This is correct but **unbounded** — neither port has a janitor sweep. Phase 1 documents the constraint and ships a lazy expiry-on-touch pattern (delete on either successful redemption or expired-redemption-attempt, but not on abandoned-after-login). A janitor goroutine is a follow-up.
- Meridian's `validateMCPResourceIndicators` (lines 314-330) accepts the resource param as optional but rejects mismatch when present. This is the right RFC 8707 posture and ships verbatim.
- Meridian's `cloneValues` helper (lines 507-513) is used exactly once in handlers; meridian is removing it during T2.5 cleanup. Phase 1 inlines the two-line copy at the call site, **does not** ship the helper.

**Surprises:**
- `oauth/config.go` already enforces `len(Secret) != 32` (line 50). The new `Config.ApprovalSecret` validation on `consent.Handler` should mirror this exactly: 32 bytes when explicit, auto-generate when empty, **never** silently accept a short operator-supplied value.
- The kit has `audit.Discard()` already — Phase 1's `consent.Handler` defaults to it when no emitter is provided, removing the nil-check boilerplate both consumers carry today.
- The kit's `_examples/minimal-server/` does not exercise OAuth at all; it shows PAT + bearer only. The deferred `_examples/oauth-consent-server/` (P2 from the gap analysis) is the right new home for an end-to-end demo.

**Parallelization opportunities (within this plan):**
- Phase 2 and Phase 3 can run concurrently after Phase 1 ends.
- Phase 4 can run concurrently with both Phase 2 and Phase 3 (touches different files: `handlers.go`, `provider.go`).

**Shared branch coordination:**
- All phases work on the same branch (`oauth-consent-helpers` or as the coordinator names it).
- Teams must not use `git stash`, `git reset`, or any automation that performs either.
- Teams must not modify files outside their assigned phase scope.
- If conflicting edits appear, the coordinator resolves via merge — never via reset.

---

## Phase 1: Agnostic Core (`oauth/consent` package)

**Goal:** Land the project-agnostic helpers and the `consent.Handler` that wires them.

**Owner:** Phase-1 team (one Change Agent → one Validation Agent → one Security Agent per the three-stage process in `~/.claude/CLAUDE.md`).

**Scope (files this team owns):**
- Create: `oauth/consent/doc.go`
- Create: `oauth/consent/config.go`
- Create: `oauth/consent/approval.go`
- Create: `oauth/consent/approval_test.go`
- Create: `oauth/consent/params.go`
- Create: `oauth/consent/params_test.go`
- Create: `oauth/consent/request.go`
- Create: `oauth/consent/request_test.go`
- Create: `oauth/consent/resource.go`
- Create: `oauth/consent/resource_test.go`
- Create: `oauth/consent/handler.go`
- Create: `oauth/consent/handler_test.go`
- Create: `oauth/consent/error.go`
- Create: `oauth/consent/redirect.go`
- Create: `oauth/consent/audit.go`

**Cannot run in parallel with:** other tasks within Phase 1 (each task builds on the prior).

---

### Task 1.1 — Package skeleton + doc.go

**Files:**
- Create: `oauth/consent/doc.go`

**Step 1.1.1 — Write doc.go**

```go
// Package consent provides reusable building blocks for an OAuth 2.1
// authorization-endpoint handler that performs browser-based login and
// explicit user consent before delegating to fosite.
//
// Why this package exists
//
// oauth.Provider.AuthorizeHandler is a "demo" handler — it grants requested
// scopes immediately to whatever subject SubjectResolver returns and is not
// safe for production. Production servers must (a) authenticate the browser
// session via a credential check, (b) collect explicit consent for the
// requested scopes, and (c) bind the canonical resource audience server-side
// per RFC 8707. consent.Handler is the kit's canonical answer.
//
// Layering
//
// The kit owns the protocol-correct mechanics — the HMAC-signed approval
// token format, the single-use replay store, the synthetic-GET trick fosite
// requires for POST flows, the resource-indicator validator, the RFC 6749
// error encoder, and the redirect-URL stitcher. The consumer owns the user
// model and the HTML renderer:
//
//   - Authenticator: a credential check that returns oauth.Subject.
//   - Renderer: an http.Handler-compatible function that writes the login or
//     consent page to the response.
//   - Optional audit.Emitter: when set, consent.Handler emits
//     oauth_authorize.{approved,denied} events automatically.
//
// See oauth/README.md "Replacing the demo authorize handler" for a worked
// integration sketch and the migration notes for both reference consumers.
package consent
```

**Step 1.1.2 — Commit**

```
git add oauth/consent/doc.go
git commit -m "docs(oauth/consent): package skeleton + design rationale"
```

---

### Task 1.2 — `Config` + minimum-length secret validation

**Files:**
- Create: `oauth/consent/config.go`
- (Tests for Config land in `handler_test.go` Task 1.10 — `Config` is exercised through `NewHandler`.)

**Step 1.2.1 — Write the failing test in `handler_test.go` (will be created in Task 1.10; for now jot the test as a comment in `config.go` and remove it when 1.10 lands)**

The actual TDD red commit happens in Task 1.10 where the constructor is wired. For Task 1.2 itself, the validator is pure data plus pure validation — write it, but the test that proves rejection of a 1-byte operator-supplied secret comes in Task 1.10 alongside the `NewHandler` test.

**Step 1.2.2 — Write `oauth/consent/config.go`**

```go
package consent

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/oauth"
)

// approvalSecretLength is the required length in bytes for the HMAC key used
// to sign approval tokens. Matches oauth.Config.Secret length so operators
// who already manage one cryptographically-strong 32-byte key can reuse the
// same posture. Shorter operator-supplied values are rejected outright; an
// empty value triggers a one-shot in-memory random key.
const approvalSecretLength = 32

// Config wires a consent.Handler.
//
// All fields except Renderer and Provider have sensible zero-value behaviour
// (auto-generated approval key, audit.Discard, a derived resource URL).
type Config struct {
	// Provider is the kit OAuth provider whose fosite OAuth2Provider is used
	// to build authorize requests, mint authorization codes, and write
	// errors. Required.
	Provider *oauth.Provider

	// Authenticator verifies username/password and returns an oauth.Subject.
	// Required.
	Authenticator Authenticator

	// Renderer renders the login form, consent page, and post-approve
	// redirect bridge. Required.
	Renderer Renderer

	// PublicURL is the externally-reachable origin of the OAuth server, with
	// no trailing slash, e.g. "https://app.example.com". Used to build the
	// synthetic GET request fosite reads from. Required.
	PublicURL string

	// FormPath is the path the consent form POSTs back to, e.g.
	// "/oauth/authorize". Defaults to "/oauth/authorize" when empty.
	FormPath string

	// ResourceURL is the canonical MCP resource URL bound server-side to
	// every issued token (RFC 8707). Defaults to PublicURL + "/mcp" when
	// empty.
	ResourceURL string

	// ApprovalSecret is the HMAC key used to sign approval tokens. When
	// empty, a 32-byte random key is generated at construction (and a
	// warning is logged via Logger). When non-empty, it MUST be exactly 32
	// bytes — shorter values are rejected.
	ApprovalSecret []byte

	// AuditEmitter receives oauth_authorize.{approved,denied} events. When
	// nil, audit.Discard is used. The kit always emits whichever events it
	// guarantees; consumers wanting more granular audits should compose
	// emitters at their domain layer.
	AuditEmitter audit.Emitter

	// FormBodyLimit caps the POST body size for the authorize endpoint.
	// Defaults to 64 KiB when zero. Set to a smaller value for hardened
	// deployments.
	FormBodyLimit int64

	// Now overrides time.Now for tests. Defaults to time.Now when nil.
	Now func() time.Time

	// Logger receives non-blocking diagnostic events (auto-generated secret,
	// resource-mismatch warnings). Defaults to slog.Default when nil.
	Logger *slog.Logger
}

// applyDefaults fills zero-valued fields and validates the configuration.
// Returns an error if a required field is missing or an explicit field is
// invalid.
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
	if c.ResourceURL == "" {
		c.ResourceURL = c.PublicURL + "/mcp"
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
	if len(c.ApprovalSecret) == 0 {
		c.ApprovalSecret = make([]byte, approvalSecretLength)
		if _, err := rand.Read(c.ApprovalSecret); err != nil {
			return fmt.Errorf("consent: generate approval secret: %w", err)
		}
		c.Logger.Warn("consent: approval secret not configured; generated ephemeral secret. Set Config.ApprovalSecret to persist across restarts.")
		return nil
	}
	if len(c.ApprovalSecret) != approvalSecretLength {
		return fmt.Errorf("consent: ApprovalSecret must be exactly %d bytes, got %d", approvalSecretLength, len(c.ApprovalSecret))
	}
	return nil
}

// Authenticator verifies a (username, password) pair and returns the
// resulting oauth.Subject on success. On failure the returned error is shown
// to the user as a generic "invalid email or password" — the kit does not
// distinguish between unknown user, locked account, or wrong password to
// avoid enumeration leaks. Implementations may emit their own audit
// events for failed attempts.
type Authenticator interface {
	Authenticate(ctx context.Context, username, password string) (oauth.Subject, error)
}

// Renderer writes one of three page types to the response writer:
//
//   - PageLogin — first GET, after a failed login attempt, or after an
//     expired approval-token retry.
//   - PageConsent — after a successful login; includes the approval_token
//     hidden input the kit will consume on POST action=approve.
//   - PageRedirectBridge — after a successful approve, a tiny meta-refresh
//     page that bounces the browser to the OAuth redirect_uri with the
//     authorization code. Optional — when nil, the kit performs a 302
//     redirect directly.
//
// The kit hands the renderer a populated PageData; the renderer owns HTML.
type Renderer interface {
	Render(w http.ResponseWriter, page Page, data PageData)
}
```

**Why import `time`/`slog`/`context`/`http`/`url` here:** Most of these are referenced from later tasks; declaring them in `config.go` now keeps the package compileable per-task instead of cascading import errors.

**Step 1.2.3 — Run `go vet` and `gofmt`**

```
cd /home/timhaak/Dev/HaakCo/AiProjects/sharedLib/golang/mcp-kit
gofmt -w oauth/consent
go vet ./oauth/consent/...
```

Expected: clean (no unresolved references yet — `Page`, `PageData`, `Authenticator`, `Renderer` get fleshed out in later tasks; declarations alone compile).

**Step 1.2.4 — Commit**

```
git add oauth/consent/config.go
git commit -m "feat(oauth/consent): Config with applyDefaults + Authenticator/Renderer interfaces"
```

---

### Task 1.3 — `ApprovalToken` (HMAC + replay store)

**Files:**
- Create: `oauth/consent/approval.go`
- Create: `oauth/consent/approval_test.go`

**Step 1.3.1 — Write the failing tests first**

```go
// oauth/consent/approval_test.go
package consent

import (
	"crypto/rand"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestApprovalToken_RoundTrip(t *testing.T) {
	key := mustRandKey(t)
	store := newApprovalStore(key, time.Now)
	subject := uuid.New().String()
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}

	token, err := store.issue(subject, params)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := store.consume(token, params)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if got != subject {
		t.Fatalf("subject mismatch: got %q want %q", got, subject)
	}
}

func TestApprovalToken_RejectsReplay(t *testing.T) {
	key := mustRandKey(t)
	store := newApprovalStore(key, time.Now)
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	token, _ := store.issue(uuid.New().String(), params)

	if _, err := store.consume(token, params); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := store.consume(token, params); err == nil {
		t.Fatalf("second consume succeeded; expected replay rejection")
	}
}

func TestApprovalToken_RejectsExpired(t *testing.T) {
	key := mustRandKey(t)
	now := time.Now()
	clock := func() time.Time { return now }
	store := newApprovalStore(key, clock)
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	token, _ := store.issue(uuid.New().String(), params)

	now = now.Add(approvalTokenTTL + time.Second)

	if _, err := store.consume(token, params); err == nil {
		t.Fatalf("consume of expired token succeeded")
	}
}

func TestApprovalToken_RejectsParamMismatch(t *testing.T) {
	key := mustRandKey(t)
	store := newApprovalStore(key, time.Now)
	original := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	tampered := url.Values{"client_id": {"DIFFERENT"}, "state": {"xxxxxxxx"}}
	token, _ := store.issue(uuid.New().String(), original)

	if _, err := store.consume(token, tampered); err == nil {
		t.Fatalf("consume with tampered params succeeded")
	}
}

func TestApprovalToken_RejectsForgedSignature(t *testing.T) {
	keyA := mustRandKey(t)
	keyB := mustRandKey(t)
	store := newApprovalStore(keyA, time.Now)
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	token, _ := store.issue(uuid.New().String(), params)

	rogue := newApprovalStore(keyB, time.Now)
	if _, err := rogue.consume(token, params); err == nil {
		t.Fatalf("consume with rogue key succeeded")
	}
}

func mustRandKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, approvalSecretLength)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return key
}
```

**Step 1.3.2 — Verify tests fail**

```
cd /home/timhaak/Dev/HaakCo/AiProjects/sharedLib/golang/mcp-kit
go test ./oauth/consent/ -run TestApprovalToken
```

Expected: `undefined: newApprovalStore` — fails to compile, which is a valid red.

**Step 1.3.3 — Implement `oauth/consent/approval.go`**

```go
package consent

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// approvalTokenTTL bounds how long a logged-in user has to click
// Approve/Deny before the token is rejected. Keeps the consent step bound
// to the intent of the original sign-in.
const approvalTokenTTL = 5 * time.Minute

// errApprovalToken is the sentinel returned by approvalStore.consume on any
// failure mode. Callers translate to a user-facing message and a fosite
// error; the kit never distinguishes between expired/forged/replayed at the
// user-visible layer.
var errApprovalToken = errors.New("consent: approval token invalid")

// approvalStore issues and consumes single-use HMAC-signed approval tokens.
// The map ensures replay protection that the HMAC alone cannot provide:
// even if a token decodes and verifies, the second redemption attempt
// finds an empty slot.
//
// The map is bounded only by approvalTokenTTL — abandoned successful
// logins remain until their stale token is re-presented or until the
// process restarts. Operators concerned about memory in adversarial
// scenarios should restart the process after extended uptime, or add a
// janitor goroutine in a later release. (Tracked: see CHANGELOG note for
// this version.)
type approvalStore struct {
	key []byte
	now func() time.Time

	mu     sync.Mutex
	issued map[string]time.Time
}

func newApprovalStore(key []byte, now func() time.Time) *approvalStore {
	return &approvalStore{
		key:    key,
		now:    now,
		issued: make(map[string]time.Time),
	}
}

// issue builds a token of the form base64url(subject|expiresAt|paramsDigest|hmac)
// and records it in the in-memory map. Stable subject strings are required
// — the kit does not validate format.
func (s *approvalStore) issue(subject string, params url.Values) (string, error) {
	expiresAt := s.now().Add(approvalTokenTTL).Unix()
	payload := subject + "|" + strconv.FormatInt(expiresAt, 10) + "|" + paramsDigest(params)
	signature := s.sign(payload)
	token := base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + signature))

	s.mu.Lock()
	s.issued[token] = time.Unix(expiresAt, 0)
	s.mu.Unlock()
	return token, nil
}

// consume verifies, expires, and removes a token. Returns the embedded
// subject string on success.
func (s *approvalStore) consume(token string, params url.Values) (string, error) {
	if token == "" {
		return "", errApprovalToken
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", errApprovalToken
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 4 {
		return "", errApprovalToken
	}
	payload := strings.Join(parts[:3], "|")
	want := s.sign(payload)
	if !hmac.Equal([]byte(parts[3]), []byte(want)) {
		return "", errApprovalToken
	}
	expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", errApprovalToken
	}
	if s.now().Unix() > expiresAt {
		s.mu.Lock()
		delete(s.issued, token)
		s.mu.Unlock()
		return "", errApprovalToken
	}
	if parts[2] != paramsDigest(params) {
		return "", errApprovalToken
	}
	if !s.consumeStored(token) {
		return "", errApprovalToken
	}
	return parts[0], nil
}

func (s *approvalStore) consumeStored(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.issued[token]
	if !ok {
		return false
	}
	delete(s.issued, token)
	return s.now().Before(expiresAt)
}

func (s *approvalStore) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
```

**Step 1.3.4 — Verify tests pass**

```
go test ./oauth/consent/ -run TestApprovalToken -v
```

Expected: 5/5 PASS.

**Step 1.3.5 — Commit**

```
git add oauth/consent/approval.go oauth/consent/approval_test.go
git commit -m "feat(oauth/consent): HMAC-signed approval token store with single-use replay protection"
```

---

### Task 1.4 — `paramsDigest` + `oauthAuthorizeValues` (param scrubber)

**Files:**
- Create: `oauth/consent/params.go`
- Create: `oauth/consent/params_test.go`

**Step 1.4.1 — Write tests**

```go
// oauth/consent/params_test.go
package consent

import (
	"net/url"
	"testing"
)

func TestOAuthAuthorizeValues_StripsCredentialFields(t *testing.T) {
	in := url.Values{
		"client_id":       {"abc"},
		"username":        {"alice@example.com"},
		"password":        {"hunter2"},
		"action":          {"login"},
		"approval_token":  {"deadbeef"},
		"state":           {"xxxxxxxx"},
		"code_challenge":  {"sha256-base64"},
		"resource":        {"https://example.com/mcp"},
	}
	out := oauthAuthorizeValues(in)
	for _, banned := range []string{"username", "password", "action", "approval_token"} {
		if _, has := out[banned]; has {
			t.Errorf("%q should be stripped", banned)
		}
	}
	for _, kept := range []string{"client_id", "state", "code_challenge", "resource"} {
		if _, has := out[kept]; !has {
			t.Errorf("%q should be retained", kept)
		}
	}
}

func TestParamsDigest_Stable_ForLogicallyEqualValues(t *testing.T) {
	a := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}, "username": {"a"}}
	b := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}, "password": {"b"}}
	if paramsDigest(a) != paramsDigest(b) {
		t.Fatalf("digest must ignore credential fields")
	}
}

func TestParamsDigest_DiffersOnCanonicalChange(t *testing.T) {
	a := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	b := url.Values{"client_id": {"DIFFERENT"}, "state": {"xxxxxxxx"}}
	if paramsDigest(a) == paramsDigest(b) {
		t.Fatalf("digest must change when canonical params change")
	}
}
```

**Step 1.4.2 — Verify red, then write `oauth/consent/params.go`**

```go
package consent

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
)

// oauthAuthorizeValues returns a copy of values with the credential, action,
// and approval-token fields stripped. The result is what the kit feeds to
// fosite (which expects the canonical OAuth params only) and what
// paramsDigest hashes (so the digest is stable across the login → approve
// transition).
func oauthAuthorizeValues(values url.Values) url.Values {
	clean := make(url.Values, len(values))
	for key, vals := range values {
		switch key {
		case "username", "password", "action", "approval_token":
			continue
		}
		for _, value := range vals {
			clean.Add(key, value)
		}
	}
	return clean
}

// paramsDigest is a base64url-encoded SHA-256 of the canonical
// (credential-free) OAuth params. Used inside the approval token so a
// tampered approve POST is rejected even when the HMAC verifies.
func paramsDigest(values url.Values) string {
	clean := oauthAuthorizeValues(values)
	sum := sha256.Sum256([]byte(clean.Encode()))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// normalizeOAuthStringSlice trims, deduplicates, and drops empty values
// from a slice. Used for the resource indicator slice and any other
// repeated OAuth parameter where order does not matter.
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

**Step 1.4.3 — Verify green, commit**

```
go test ./oauth/consent/ -run "TestOAuthAuthorizeValues|TestParamsDigest" -v
git add oauth/consent/params.go oauth/consent/params_test.go
git commit -m "feat(oauth/consent): credential-stripping param scrubber + stable digest"
```

---

### Task 1.5 — `BuildAuthorizeRequest` (synthetic GET helper)

**Files:**
- Create: `oauth/consent/request.go`
- Create: `oauth/consent/request_test.go`

**Step 1.5.1 — Write tests**

The test exercises that a POST form correctly round-trips through fosite by being rebuilt as a synthetic GET. Use `oauth/storage.MemoryStore` and `oauth/keys.NewMemoryManager` (already in the kit) to build a real `oauth.Provider`. Reference `oauth/provider_test.go` for the recipe.

```go
// oauth/consent/request_test.go
package consent

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/haakco/mcp-kit/oauth"
)

func TestBuildAuthorizeRequest_RebuildsAsSyntheticGET(t *testing.T) {
	// (helper testProvider is defined in handler_test.go — Task 1.10)
	prov := testProvider(t, "https://app.example.com")
	registerTestClient(t, prov, "client-1", "https://app.example.com/callback")

	form := url.Values{
		"client_id":             {"client-1"},
		"response_type":         {"code"},
		"redirect_uri":          {"https://app.example.com/callback"},
		"state":                 {"xxxxxxxx"},
		"code_challenge":        {strings.Repeat("a", 43)},
		"code_challenge_method": {"S256"},
		"username":              {"alice@example.com"},
		"password":              {"hunter2"},
	}
	rawBody := strings.NewReader(form.Encode())
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", rawBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	clean := oauthAuthorizeValues(form)
	got, err := buildAuthorizeRequest(req.Context(), prov, "https://app.example.com", "/oauth/authorize", clean, req)
	if err != nil {
		t.Fatalf("buildAuthorizeRequest: %v", err)
	}
	if got.GetClient() == nil || got.GetClient().GetID() != "client-1" {
		t.Fatalf("client_id not propagated; got %+v", got.GetClient())
	}
}
```

**Step 1.5.2 — Implement `oauth/consent/request.go`**

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
// publicURL must be the externally-reachable origin (no trailing slash);
// formPath is the path the canonical request appears to live at.
//
// The original request's headers are preserved (minus Content-Type and
// Content-Length, which would mislead fosite about the synthetic request's
// body). This keeps middleware-derived state — e.g. an Origin header
// previously validated by mcp-kit's origin allowlist — available to
// downstream handlers if they consult it.
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

**Step 1.5.3 — Verify, commit**

```
go test ./oauth/consent/ -run TestBuildAuthorizeRequest -v
git add oauth/consent/request.go oauth/consent/request_test.go
git commit -m "feat(oauth/consent): synthetic-GET helper for fosite-compatible POST flows"
```

---

### Task 1.6 — `validateResourceIndicators` (RFC 8707 strict-when-present)

**Files:**
- Create: `oauth/consent/resource.go`
- Create: `oauth/consent/resource_test.go`

Test cases (write the test file first, verify red, then implement):

| Input | Expected |
|---|---|
| no `resource` param | accepted |
| single `resource` matching expected | accepted |
| single `resource` mismatching expected | rejected with `fosite.ErrInvalidRequest` |
| two `resource` params, both matching | rejected (kit allows exactly one) |
| `resource` empty / whitespace-only | accepted (treated as absent after `normalizeOAuthStringSlice`) |

Implementation lifts meridian's `validateMCPResourceIndicators` (lines 314-330) verbatim, parameterised by `expected string`. Logger calls go via `Config.Logger`.

```
git commit -m "feat(oauth/consent): RFC 8707 strict-when-present resource indicator validator"
```

---

### Task 1.7 — `WriteAuthorizeError` (RFC 6749 JSON envelope)

**Files:**
- Create: `oauth/consent/error.go`
- (Tests folded into `handler_test.go`.)

Lift `writeOAuthAuthorizeRequestError` from `meridian/backend/internal/server/mcp_oauth_authorize.go:610-621` verbatim, with `fosite.ErrorToRFC6749Error` handling. Function signature:

```go
func WriteAuthorizeError(w http.ResponseWriter, err error)
```

Public (capitalised) because consumers may want to call it from custom error paths in their renderer. Document the contract: writes `Cache-Control`, `Pragma`, `Content-Type`, status code derived from the fosite error, and a JSON body.

```
git commit -m "feat(oauth/consent): RFC 6749 error envelope encoder"
```

---

### Task 1.8 — `RedirectURLFromResponder` + `HiddenInputs` + `ClientNameFromRequester`

**Files:**
- Create: `oauth/consent/redirect.go`

Three pure functions, no state. Lift verbatim from meridian:

```go
func HiddenInputs(values url.Values) []HiddenInput     // for template iteration
func ClientNameFromRequester(req fosite.AuthorizeRequester) string
func RedirectURLFromResponder(req fosite.AuthorizeRequester, resp fosite.AuthorizeResponder) string
```

`HiddenInput` is `struct { Name, Value string }` so renderers iterate without depending on `url.Values`.

Tests live in `handler_test.go` — these functions exercised end-to-end by the handler tests in Task 1.10.

```
git commit -m "feat(oauth/consent): redirect + form helpers"
```

---

### Task 1.9 — `Page` + `PageData` types

**Files:**
- Modify: `oauth/consent/config.go` (add types alongside `Renderer` interface) **OR** create `oauth/consent/page.go` if config.go is getting long.

Recommend a separate file for clarity. Define:

```go
package consent

type Page int

const (
	PageLogin Page = iota
	PageConsent
	PageRedirectBridge
)

// PageData is the data the kit hands to Renderer. Renderers may ignore
// fields they don't display (e.g. ConsentDisplayName on PageLogin).
type PageData struct {
	// Authenticated is true on PageConsent and PageRedirectBridge.
	Authenticated bool

	// DisplayName is the resolved-friendly name of the authenticated user
	// (e.g. "Alice Smith"). Empty on PageLogin. Renderers fall back to the
	// Subject.Email or Subject.ID if they want.
	DisplayName string

	// ClientName is the human-readable name of the OAuth client requesting
	// authorization. Defaults to "your MCP client" when no name is
	// available.
	ClientName string

	// Scopes is the list of scopes the client requested. Renderers
	// typically render these as a checklist.
	Scopes []string

	// Resources is the set of canonical resources the token will be
	// audience-bound to (RFC 8707). Empty when the client did not include
	// `resource`.
	Resources []string

	// HiddenInputs are name/value pairs the renderer must include as
	// <input type="hidden"> in the consent form so the kit can re-derive
	// the original request on POST. Includes approval_token on
	// PageConsent.
	HiddenInputs []HiddenInput

	// FormAction is the path the form must POST back to (typically
	// "/oauth/authorize").
	FormAction string

	// Error is a human-readable message when re-rendering after a failed
	// login attempt or expired token. Empty on the happy path.
	Error string

	// RedirectURL is set on PageRedirectBridge — the URL the renderer
	// should bounce the browser to (meta-refresh or JS).
	RedirectURL string
}
```

```
git commit -m "feat(oauth/consent): Page + PageData types for renderers"
```

---

### Task 1.10 — `Handler` + `NewHandler` (the wire-up)

**Files:**
- Create: `oauth/consent/handler.go`
- Create: `oauth/consent/audit.go`
- Create: `oauth/consent/handler_test.go`

This is the largest task. Approach:

**Step 1.10.1 — Write the failing test suite first (TDD red)**

Six required test cases (mirror meridian, keep meridian's bonus replay test):

```go
// oauth/consent/handler_test.go
package consent

import ( /* ... */ )

func TestHandler_GET_RendersLoginPage(t *testing.T) { /* ... */ }
func TestHandler_POST_LoginBadPassword(t *testing.T) { /* ... */ }
func TestHandler_POST_LoginGoodPassword(t *testing.T) { /* ... */ }
func TestHandler_POST_ApproveGoodToken(t *testing.T) { /* ... */ }
func TestHandler_POST_ApproveExpiredToken(t *testing.T) { /* ... */ }
func TestHandler_POST_Deny(t *testing.T) { /* ... */ }
func TestHandler_POST_ApprovalReplayRejected(t *testing.T) { /* ... */ }
func TestHandler_AuditEmittedOnApproveAndDeny(t *testing.T) { /* ... */ }
func TestHandler_RejectsShortOperatorSecret(t *testing.T) {
	// Confirms NewHandler errors when ApprovalSecret is non-empty but != 32 bytes.
	_, err := NewHandler(Config{
		Provider:       testProvider(t, "https://app.example.com"),
		Authenticator:  fakeAuth(map[string]string{}),
		Renderer:       fakeRenderer{},
		PublicURL:      "https://app.example.com",
		ApprovalSecret: []byte{0x01},
	})
	if err == nil {
		t.Fatal("expected NewHandler to reject 1-byte approval secret")
	}
}
func TestHandler_AcceptsEmptyOperatorSecret_GeneratesEphemeral(t *testing.T) { /* ... */ }
```

Test helpers in the same file:
- `testProvider(t, issuer string) *oauth.Provider` — wraps `oauth.New` with `oauth/storage.MemoryStore` and a memory `keys.Manager`.
- `registerTestClient(t, prov, clientID, redirectURI)` — pushes a client into the store.
- `fakeAuth(creds map[string]string) Authenticator` — returns a `consent.Authenticator` impl that checks the supplied map.
- `fakeRenderer` — captures `Page` + `PageData` for assertions.
- `fakeEmitter` — captures `audit.Event` slice for assertions.
- `s256Challenge(verifier string) string` — copy from meridian.

These helpers move into `oauth/consent/consenttest` in Phase 2.

**Step 1.10.2 — Verify all tests fail, then implement `oauth/consent/handler.go`**

Sketch (follows meridian's structure with the duplicated ports replaced by package-internal calls):

```go
package consent

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/ory/fosite"

	"github.com/haakco/mcp-kit/oauth"
)

type Handler struct {
	cfg   Config
	store *approvalStore
}

func NewHandler(cfg Config) (*Handler, error) {
	if err := cfg.applyDefaults(); err != nil {
		return nil, err
	}
	return &Handler{cfg: cfg, store: newApprovalStore(cfg.ApprovalSecret, cfg.Now)}, nil
}

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
	authorizeRequest, err := buildAuthorizeRequest(r.Context(), h.cfg.Provider, h.cfg.PublicURL, h.cfg.FormPath, cleanParams, r)
	if err != nil {
		WriteAuthorizeError(w, err)
		return
	}
	if err := validateResourceIndicators(cleanParams, h.cfg.ResourceURL, h.cfg.Logger); err != nil {
		WriteAuthorizeError(w, err)
		return
	}
	switch r.PostForm.Get("action") {
	case "login":
		h.handleLogin(w, r, authorizeRequest, cleanParams)
	case "approve":
		h.handleApprove(w, r, authorizeRequest, cleanParams)
	case "deny":
		_, _ = h.store.consume(r.PostForm.Get("approval_token"), cleanParams)
		h.emitAudit(r.Context(), "denied", authorizeRequest, oauth.Subject{})
		h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, authorizeRequest, fosite.ErrAccessDenied)
	default:
		h.renderLogin(w, r, cleanParams, "unknown authorization action")
	}
}

// handleLogin, handleApprove, renderLogin, renderConsent, renderRedirectBridge,
// emitAudit — port verbatim from meridian/backend/internal/server/mcp_oauth_authorize.go,
// substituting:
//   - s.client.User.Query()...        →  Authenticator.Authenticate(ctx, username, password)
//   - sessionForUser(u)               →  oauth.NewSession(subject)
//   - s.writeHTML(w, tmpl, data)      →  h.cfg.Renderer.Render(w, page, pageData)
//   - kitaudit.Discard fallback       →  cfg.applyDefaults already set it
```

**Step 1.10.3 — Implement `oauth/consent/audit.go`**

```go
package consent

import (
	"context"
	"strings"
	"time"

	"github.com/ory/fosite"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/oauth"
)

func (h *Handler) emitAudit(ctx context.Context, action string, req fosite.AuthorizeRequester, subject oauth.Subject) {
	clientID := ""
	if req != nil && req.GetClient() != nil {
		clientID = req.GetClient().GetID()
	}
	scope := ""
	if req != nil {
		scope = strings.Join(req.GetRequestedScopes(), " ")
	}
	event := audit.Event{
		EntityType: "oauth_authorize",
		EntityID:   clientID,
		Action:     action,
		ClientID:   clientID,
		Scope:      scope,
		Timestamp:  time.Now().UTC(),
	}
	if subject.ID != "" {
		// Consumers stringify their own user-ID type to subject.ID; the
		// kit-side Event.ActorUserID is uuid.UUID to match the existing
		// audit.Event shape. When the consumer's ID is not a UUID,
		// ActorUserID stays nil and the consumer adds context via their
		// emitter wrapper.
		if u, err := uuid.Parse(subject.ID); err == nil {
			event.ActorUserID = &u
		}
	}
	_ = h.cfg.AuditEmitter.Emit(ctx, event)
}
```

**Note on ActorUserID:** The kit's `audit.Event.ActorUserID` is `*uuid.UUID`, but `oauth.Subject.ID` is a string (Q2 Option A). Pragmatic resolution: parse the subject ID as UUID; if it parses, populate `ActorUserID`; otherwise leave nil and let the consumer's emitter wrap the call to add their own actor-id field. Documents in the package doc.

**Step 1.10.4 — Verify all 10 tests pass**

```
go test ./oauth/consent/ -count=1 -v
```

**Step 1.10.5 — Commit**

```
git add oauth/consent/handler.go oauth/consent/audit.go oauth/consent/handler_test.go
git commit -m "feat(oauth/consent): Handler wiring + audit emission for approve/deny"
```

---

### Task 1.11 — Update `Provider.AuthorizeHandler` docstring

**Files:**
- Modify: `oauth/handlers.go:13-16`

**Step 1.11.1 — Tighten the docstring**

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
// The handler is retained for tests, examples, and incremental migration —
// it MAY be removed or renamed in v1.0.0.
func (p *Provider) AuthorizeHandler(resolve SubjectResolver) http.Handler {
```

**Step 1.11.2 — Run kit-wide tests**

```
go test ./...
```

Expected: green. Doc-only change.

**Step 1.11.3 — Commit**

```
git add oauth/handlers.go
git commit -m "docs(oauth): warn AuthorizeHandler is demo-only; point to oauth/consent"
```

---

### Phase 1 Sign-off Gate (coordinator)

Before launching Phases 2–4, the coordinator runs the **three-stage review** (`~/.claude/CLAUDE.md` "Three-Stage Review Process"):

1. **Spec compliance reviewer** — verifies every required artifact (Tasks 1.1–1.11) exists and matches the plan. No missing helpers, no extra features.
2. **Code quality reviewer** — reads the diff, focuses on naming, idioms, surgical scope, error handling, locking.
3. **Security reviewer** — REQUIRED here (auth changes). Verifies HMAC key handling, replay store correctness under concurrent load (`go test -race ./oauth/consent/...`), no secret material in logs, no info leak in error messages, RFC 8707 enforcement is correct.

If any stage finds issues, the implementer fixes and the same reviewer re-reviews before moving on.

---

## Phase 2: Test Fixtures (`oauth/consent/consenttest`)

**Goal:** Saving ~150 LOC of test boilerplate per consumer, codifying the canonical six-test contract.

**Owner:** Phase-2 team. Can start once Phase 1 ends.

**Files:**
- Create: `oauth/consent/consenttest/doc.go`
- Create: `oauth/consent/consenttest/server.go`
- Create: `oauth/consent/consenttest/auth.go`
- Create: `oauth/consent/consenttest/render.go`
- Create: `oauth/consent/consenttest/transport.go`
- Create: `oauth/consent/consenttest/checks.go`
- Create: `oauth/consent/consenttest/consenttest_test.go` (smoke test)

### Task 2.1 — `Provider(t)` test harness

Wrap `oauth.New` with `MemoryStore` + memory `keys.Manager`. Mirror `testkit/server.go:23-53` style. Returns `*oauth.Provider`.

```go
package consenttest

func Provider(t testing.TB, issuer string) *oauth.Provider { ... }
```

### Task 2.2 — `RegisterClient`

```go
func RegisterClient(t testing.TB, p *oauth.Provider, opts ClientOptions) // ClientID, RedirectURI, Scopes
```

### Task 2.3 — `Authenticator` fakes

```go
func StaticAuth(creds map[string]oauth.Subject) consent.Authenticator      // username → Subject
func DenyAll() consent.Authenticator                                       // always returns errors
```

### Task 2.4 — `Renderer` capture fake

```go
type CapturingRenderer struct { /* records last (Page, PageData) */ }
func NewCapturingRenderer() *CapturingRenderer
```

### Task 2.5 — Transport helpers

```go
func NoFollowClient() *http.Client                          // disables redirects so tests can read 302 Location
func S256Challenge(verifier string) string                  // PKCE challenge helper
func HiddenInputValue(body, name string) string             // parse form HTML for hidden input
func RedirectOrContinue(t testing.TB, resp *http.Response) string  // returns Location or "" depending on status
```

### Task 2.6 — `RunCanonicalSuite`

A single function consumers call to assert the six canonical test cases pass against their `consent.Handler` configuration. Internally calls each test as a `t.Run` subtest.

```go
func RunCanonicalSuite(t *testing.T, h *consent.Handler, opts SuiteOptions) {
	t.Helper()
	t.Run("GET_RendersLoginForm",          func(t *testing.T) { ... })
	t.Run("POST_LoginBadPassword",         func(t *testing.T) { ... })
	t.Run("POST_LoginGoodPassword",        func(t *testing.T) { ... })
	t.Run("POST_ApproveGoodToken",         func(t *testing.T) { ... })
	t.Run("POST_ApproveExpiredToken",      func(t *testing.T) { ... })
	t.Run("POST_Deny",                     func(t *testing.T) { ... })
	t.Run("POST_ApprovalReplayRejected",   func(t *testing.T) { ... })
}
```

### Task 2.7 — Self-test

Smoke test in `consenttest_test.go` runs `RunCanonicalSuite` against an in-memory `consent.Handler`. Validates the package itself.

```
git commit -m "test(oauth/consent): consenttest fixture package + canonical suite"
```

---

## Phase 3: Documentation (`oauth/README.md` + Quickstart update)

**Goal:** Future projects find the right path in 30 minutes, not 2 days.

**Owner:** Phase-3 team. Can start once Phase 1 ends; runs in parallel with Phase 2 and Phase 4.

**Files:**
- Create: `oauth/README.md`
- Modify: `README.md` (kit root) — Quickstart code block
- Modify: `CHANGELOG.md` — add entry for v0.5.0 (or whatever the next version is)

### Task 3.1 — `oauth/README.md`

Sections:
1. **Overview** — what the package does, what each subpackage owns.
2. **Replacing the demo authorize handler** — step-by-step, with a 50-line worked example using `consent.NewHandler`.
3. **Authenticator contract** — what to return, what errors look like to the user, no enumeration leaks.
4. **Renderer contract** — three pages, fields you must render, fields you may ignore.
5. **Audit events** — `oauth_authorize.{approved,denied}`, when emitted, what fields are populated.
6. **RFC 8707 enforcement** — strict-when-present semantics, why.
7. **Operator considerations** — `ApprovalSecret` (32 bytes), bounded map growth, why no janitor.
8. **Migration from `Provider.AuthorizeHandler`** — what changes in the consumer.
9. **Testing your handler** — points at `oauth/consent/consenttest.RunCanonicalSuite`.

### Task 3.2 — Update root `README.md` Quickstart

Replace the line:

```go
mux.Handle("/oauth/authorize", oauthProv.AuthorizeHandler(myapp.ResolveSubject))
```

with:

```go
authorize, err := consent.NewHandler(consent.Config{
    Provider:      oauthProv,
    Authenticator: myapp.NewAuthenticator(db),  // your credential check
    Renderer:      myapp.NewConsentRenderer(),  // your HTML
    PublicURL:     "https://my-mcp.example.com",
    AuditEmitter:  myapp.NewAuditEmitter(db),
})
if err != nil { /* handle */ }
mux.Handle("/oauth/authorize", authorize)
```

### Task 3.3 — `CHANGELOG.md` entry

Under the new version heading, summarise: new `oauth/consent` package, new `oauth/consent/consenttest` fixtures, `Provider.AuthorizeHandler` docstring update, deferred items (default template, `BrowserAuthorizeHandler` rename in v1.0.0).

```
git commit -m "docs(oauth): replace-the-demo guide + Quickstart + CHANGELOG"
```

---

## Phase 4: `BrowserAuthorizeHandler` interface + `Provider.MountAuthorize`

**Goal:** Type the contract `Provider.AuthorizeHandler` was always meant to be a stand-in for, so future consumers can't silently use the demo path.

**Owner:** Phase-4 team. Can run in parallel with Phases 2 and 3 (different files).

**Files:**
- Modify: `oauth/handlers.go` — add interface + `MountAuthorize`
- Modify: `oauth/provider.go` — add method
- Modify: `oauth/handlers_test.go` (or `provider_test.go` if that's where mount tests live) — interface satisfaction test

### Task 4.1 — Interface + Mount method

**Step 4.1.1 — Write the failing test**

```go
func TestProvider_MountAuthorize_RejectsNilHandler(t *testing.T) { ... }
func TestProvider_MountAuthorize_RegistersHandler(t *testing.T) { ... }

// Compile-time check that consent.Handler satisfies BrowserAuthorizeHandler.
var _ oauth.BrowserAuthorizeHandler = (*consent.Handler)(nil)
```

**Step 4.1.2 — Implement**

```go
// oauth/handlers.go (additions)

// BrowserAuthorizeHandler is the contract a production /oauth/authorize
// handler must satisfy. http.Handler is the only real method, but the
// named type makes the contract explicit at the call site of
// Provider.MountAuthorize and prevents passing a generic http.Handler that
// is not actually safe for OAuth.
type BrowserAuthorizeHandler interface {
	http.Handler
}

// MountAuthorize mounts a production-ready /authorize handler at
// prefix+"/authorize" on mux. token, revoke, and register handlers are
// mounted via RegisterRoutes; this method exists to type-check that the
// authorize handler is not the demo variant.
func (p *Provider) MountAuthorize(mux *http.ServeMux, prefix string, h BrowserAuthorizeHandler) {
	if prefix == "" {
		prefix = "/oauth"
	}
	if h == nil {
		panic("oauth: MountAuthorize: handler is nil; pass a consent.Handler or your own BrowserAuthorizeHandler")
	}
	mux.Handle(prefix+"/authorize", h)
}
```

`RegisterRoutes` stays as-is (backwards compatible), but its docstring gains a sentence pointing at `MountAuthorize` for production use.

**Step 4.1.3 — Verify, commit**

```
go test ./...
git commit -m "feat(oauth): BrowserAuthorizeHandler interface + Provider.MountAuthorize"
```

---

## Deferred (NOT in this plan)

These came up in the gap analysis but are intentionally out of scope. Track separately:

1. **Default consent HTML template** (`oauth/consent/template` subpackage) — N=2 templates today is not enough signal to pick a default. Revisit after a third consumer.
2. **Renaming `Provider.AuthorizeHandler` → `DemoAuthorizeHandler`** — defer to v1.0.0 deprecation cycle.
3. **`_examples/oauth-consent-server/` worked example** — useful but doc-only follow-up.
4. **Janitor goroutine for `approvalStore.issued` map** — current size bound is one approval-token-per-abandoned-login, which is small in practice. Add only if a real consumer hits it.
5. **Consumer migrations** — vorrent and meridian both keep their hand-rolled handlers running today. Migrating each to `oauth/consent` is a separate per-project plan: it's a refactor with no behavior change, with the bonus that vorrent picks up meridian's hardenings.

---

## Completion Checklist (Phase-by-Phase)

Per `~/.claude/CLAUDE.md` "Completion Checklist (Non-Negotiable)":

- [ ] **Readable** — A new contributor reads `oauth/README.md` and `oauth/consent/doc.go` and writes a working `consent.NewHandler` call in under 15 minutes.
- [ ] **Linted** — `golangci-lint run ./oauth/consent/... ./oauth/...` green; no new findings introduced.
- [ ] **Code quality** — Zero duplication between `oauth/consent` and the kit's existing `oauth` package internals.
- [ ] **Tested** — All Task 1.x test cases pass with `-race` clean. `consenttest.RunCanonicalSuite` self-test green.
- [ ] **All problems fixed** — No TODOs left in the package. No "will fix later" comments.
- [ ] **Consistent** — Naming matches the kit's existing conventions (`oauth.Provider`, `audit.Emitter`, `consent.Authenticator`).
- [ ] **Simple** — `oauth/consent` total LOC < 800 across handler + helpers (the meridian port is 621 LOC; we should be slightly smaller because the consumer-specific user-model code drops out).
- [ ] **Named well** — `Authenticator`, `Renderer`, `Page`, `PageData`, `Handler`, `BrowserAuthorizeHandler` — all reveal intent without comments.
- [ ] **No hacks** — No silent fallbacks. Operator-supplied short approval secret returns `error`, does not auto-fix.
- [ ] **Documented** — Package doc, public-symbol docs, `oauth/README.md`, CHANGELOG entry.
- [ ] **System working** — `go test ./...` green at the kit root.

If any check fails, do **not** ship. Update this plan with the failure mode, fix the underlying issue (no band-aid), and re-run the checklist.

---

## Coordination Notes for the Implementer Team

- **Branch creation requires explicit user approval.** Coordinator confirms with user before `git checkout -b`. Suggested name: `oauth-consent-helpers`.
- **One implementer subagent per phase.** Each implementer self-reviews before reporting. The coordinator dispatches review subagents (spec → quality → security) per `~/.claude/CLAUDE.md`.
- **Never push.** Final commits land on the local branch only. User reviews and pushes manually.
- **Surgical scope.** Each task lists its files. Implementers must not touch files outside the listed set. If an unrelated improvement seems necessary, surface to the coordinator — do not ship it.
- **No `git stash`. No `git reset` of any kind.** No exceptions. If multiple agents end up touching the same file (rare given phase scoping), coordinator resolves via merge.
- **Questions go up, not sideways.** Implementers ask the coordinator; coordinator asks the user. No implementer-to-implementer chat (they don't share context).

---

## Estimated Effort

| Phase | Tasks | Estimate |
|---|---|---|
| Phase 1 — agnostic core | 11 tasks | 6–8 hours implementer + 2 hours review (3 stages) |
| Phase 2 — test fixtures | 7 tasks | 3–4 hours |
| Phase 3 — docs | 3 tasks | 2–3 hours |
| Phase 4 — interface | 1 task | 1 hour |
| **Total** | **22 tasks** | **~12–16 hours of implementer work + ~3 hours of review across 3 stages** |

Phases 2/3/4 in parallel after Phase 1: total wall-clock is Phase 1 (~6–8h) + max(Phase 2, Phase 3, Phase 4) (~3–4h) = **~10–12 hours wall-clock with three implementers and the review gauntlet.**
