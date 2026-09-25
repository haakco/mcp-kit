# MCP 2026-07-28 Protocol Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Status:** Draft. Created 2026-09-25.

**Goal:** Make `mcp-kit` serve protocol revision **2026-07-28** — the revision that makes MCP stateless, requires
`server/discover`, and moves the OAuth surface to Client ID Metadata Documents — without changing the kit/consumer
boundary or absorbing any domain code.

**Background:** `mcp-kit` pins the official Go SDK at **v1.6.1**. That SDK's newest negotiated revision is
`2025-11-25`, so nothing the kit wraps can speak 2026-07-28 today. Upstream `go-sdk` **v1.8.0** negotiates
`2026-07-28` as `latestProtocolVersion` and carries `server/discover`, stateless per-request `_meta`, `resultType`,
`SetCacheable`, `SchemaCache`, and the new error-code partition. The spec changes that matter to this kit:
the `initialize` handshake and `Mcp-Session-Id` are gone (`subscriptions/listen` replaces `resources/subscribe` and
the GET SSE stream); `server/discover` is mandatory; `ping`, `logging/setLevel`, and
`notifications/roots/list_changed` are removed; logging, roots, and sampling are deprecated (SEP-2577); `ttlMs` and
`cacheScope` are required on `server/discover`, the four list methods, and `resources/read`; error codes
`-32020`–`-32099` are reserved for the spec and `-32002` is forbidden; and dynamic client registration is deprecated
in favour of Client ID Metadata Documents.

**Architecture:** The kit keeps owning transport middleware, OAuth issuer, key rotation, discovery, CLI auth, Ent
mixins, and testkit. This plan upgrades the pinned SDK, replaces the kit's fabricated protocol fixtures with a real
SDK-backed reference server, and extends the OAuth and middleware layers for the 2026-07-28 wire. No domain tools,
resources, prompts, user tables, RBAC, or audit storage move into the kit.

**Tech Stack:** Go (currently 1.26.2; consumers are on 1.27.x), stdlib `net/http`, Ory Fosite, go-jose/v3,
`github.com/modelcontextprotocol/go-sdk` v1.6.1 → **v1.8.0**, Ory Ent, existing `just` recipes, GitHub Actions
(`.github/workflows/ci.yml`, self-hosted `haakco-build`), and the official conformance CLI
`@modelcontextprotocol/conformance` pinned at **0.1.16**.

**Parallel Work Model:** Task 1 is a prerequisite for every other task and must land first. Tasks 2, 4, and 6 are
independent of each other and may run concurrently on non-overlapping files. Task 3 depends on Task 2. Task 5 depends
on Tasks 2 and 3. Task 7 depends on Tasks 2–6. Task 9 depends on Tasks 2–7 and must run **before** the v0.6.0 tag, per
this repo's own rule that transport-affecting changes are re-verified on at least one downstream consumer before
tagging. Task 8 drafts the release material once the behaviour decisions are final and finalizes it after Task 9
passes. Per repo policy there is no `git stash` and no `git reset` at any point.

---

## Current State (Verified 2026-09-25)

### Environment probe: the SDK bump is a no-op mechanically

The migration was probed in a throwaway copy, leaving this repo untouched:

```bash
rsync -a --exclude '.git' ~/Dev/HaakCo/AiProjects/sharedLib/golang/mcp-kit/ /tmp/mcp-kit-probe/
cd /tmp/mcp-kit-probe
go mod edit -require=github.com/modelcontextprotocol/go-sdk@v1.8.0
GOFLAGS=-mod=mod go mod tidy
go build ./...      # clean, zero errors
go vet ./...        # clean, zero findings
go test ./...       # OK across cliauth, entschema, mcpkit, mcpmw, oauth (+6 subpackages), oidc, testkit
```

So there is **no mechanical migration to perform**. That is the single most important finding here, because it means
every remaining task is behavioural and none of it is covered by the existing green suite. Treat a green suite as
evidence of nothing in this plan.

### Why the suite is green: the kit never exercises the MCP protocol

- **Nothing in the repo constructs the SDK's HTTP handler.** `grep -rn 'StreamableHTTPHandler\|StreamableHTTPOptions'
  --include='*.go' .` returns nothing. The consumer passes its own handler into `mcpkit.Config.Handler`; the kit wraps
  it with `Origin → Bearer → Envelope` and returns it.
- **`testkit/server.go` fabricates an MCP server.** `handleMCP` decodes a request and switches on the method,
  answering `initialize`, `notifications/initialized`, `ping`, and `tools/list` from a hand-written map. It reports
  `"protocolVersion": "2025-03-26"` and sets `Mcp-Session-Id`.
- **`_examples/minimal-server/main.go` fabricates the same server again** (`handleMCP`, line 109 onward), with the
  same hardcoded `"2025-03-26"` and the same session header.
- **`testkit.RunHandshake` uses a real SDK client** (`mcp.NewClient` + `mcp.StreamableClientTransport`) against that
  fabricated server, then returns `session.ID()`. Under SDK v1.8.0 the client probes `server/discover` first, the
  fabricated server answers HTTP 400 for an unknown method, and the client falls back to the legacy `initialize`
  handshake — which is why the test passes and why it proves nothing about 2026-07-28.
- The stale revision literal appears in four places: `testkit/server.go:71`, `_examples/minimal-server/main.go:124`,
  `docs/dispatch-runbook-template.md:92`, and `docs/lessons.md:57`.

### `mcpkit.Config` accepts fields it silently drops

`mcpkit/server.go` declares `Implementation any` ("mcp.Implementation in v0.2.0; any to keep v0.1.0 dep-free") and
`Instructions string`. `mcpkit.New` reads neither. It validates `Handler`, copies the deprecated `Validator` into
`Bearer.TokenValidator`, composes the three middlewares, and returns. 2026-07-28 promotes `instructions` to a
spec-visible field of `DiscoverResult`, so a value a consumer sets and the kit discards is now a user-visible
omission rather than an internal detail. The `any`-for-dep-freedom rationale is also stale: the module already
depends on the SDK through `testkit`.

### OAuth surface today

- `oauth/register.go` implements **dynamic client registration** and treats it as the primary registration path.
  2026-07-28 deprecates RFC 7591 DCR in favour of Client ID Metadata Documents.
- No `iss` parameter validation on authorization responses (RFC 9207, new requirement).
- No `application_type` handling in DCR (new requirement, avoids OIDC redirect-URI conflicts).
- Bearer challenge and RFC 9728 protected-resource metadata exist and were hardened on 2026-06-21 and are being
  extended by the `aoa` plan. `docs/plans/2026-07-06_aoa_security_conformance_hardening.md` **owns** RFC 9728 metadata
  validation, bearer challenge contract tests, JWKS hardening, and the external conformance smoke. This plan must not
  duplicate that work; it references it.

### Middleware

- `mcpmw/envelope.go` rewrites plain-text HTTP errors from the SDK handler into canonical JSON-RPC envelopes, keyed on
  known SDK error prefixes, with 401/403/404/415 deliberately passing through so `WWW-Authenticate` and Origin
  denials survive.
- `mcpmw/origin.go` enforces an Origin allowlist with an explicit loopback allowance.
- Neither has been checked against the 2026-07-28 error-code partition (`-32020`–`-32099` reserved; `-32002`
  forbidden) or the newly required `Mcp-Method`, `Mcp-Name`, and `Mcp-Protocol-Version` request headers.

### Repo hygiene facts that contradict `AGENTS.md`

`AGENTS.md` states "This is a single Go module with no Makefile, justfile, or CI config — use the `go` toolchain
directly" and "Go 1.26 (toolchain `go1.26.2`) is required". Both are stale: `justfile` exists with `build`, `test`,
`test-race`, `vet`, `lint-go`, `lint-go-deep`, `lint-go-structural`, `quality`, `tidy`, and `format` recipes;
`mise.toml` exists; `.github/workflows/ci.yml` runs on the self-hosted `haakco-build` runner with
`go-version: "1.26.2"`; and `.github/actionlint.yaml` exists. Fix this in Task 8.

---

## Scope Decisions

### Decided by evidence (no sign-off needed)

1. **Task 1 is the SDK bump alone, landed first.** The probe proves it is risk-free at compile and test time, so it
   should not be entangled with behaviour changes that need review.
2. **The fabricated protocol fixtures must go.** A testkit that answers `initialize` with `2025-03-26` cannot validate
   any revision claim, and it actively misleads consumers who copy it. Replace with a real SDK-backed reference
   server.
3. **`ping` must stop being answered.** It is removed at 2026-07-28.
4. **The kit does not own MCP header validation.** The SDK's transport validates `Mcp-Method`, `Mcp-Name`, and
   `Mcp-Protocol-Version` and returns `-32020` on mismatch. The kit's job is to not mask that, which is a test, not
   an implementation.
5. **D1 — modern-only. `SupportedProtocolVersions` is narrowed to `["2026-07-28"]`.** Decided by the owner on
   2026-09-25, over this plan's original dual-era recommendation. The kit serves one wire era: no `initialize`
   handshake, no `Mcp-Session-Id`, no legacy fallback, and the testkit teaches only the modern path. Consequences
   carried through every task below:

   - `RunHandshake` loses its meaning entirely, so it is **replaced**, not kept alongside a modern helper. That is a
     breaking API change for `skills-mcp` and `vorrent`, and it needs a migration note in `CHANGELOG.md` plus an
     update to both `docs/migration/*.md` contracts.
   - **A client that speaks only the legacy handshake can no longer connect.** The spec's guidance is that a
     modern-only server should name the versions it supports in any error it returns to `initialize`, because legacy
     clients have no fall-forward mechanism. The SDK answers an excluded version with `2025-11-25` so the client
     disconnects cleanly rather than misreading the reply — verify that observed behaviour, do not assume it.
   - Conformance is scored against `2026-07-28` only.
   - Each consumer must confirm its clients are modern before its own upgrade, because the break lands in their
     deployment, not in this repo.

### Still needs owner sign-off

| # | Decision | Recommendation | Reason |
|---|---|---|---|
| **D2** | Keep `Implementation any`, or take the SDK type? | **Take `*mcp.Implementation`** and actually apply it plus `Instructions`. | The module already depends on the SDK via `testkit`, so `any` buys nothing, and 2026-07-28 makes `instructions` spec-visible through `server/discover`. Silently dropping it is now a visible defect. Watch the ~12 method-per-receiver cap (`CG-02`). |
| **D3** | Replace DCR with CIMD, or add CIMD alongside? | **Add CIMD; demote DCR to the documented fallback.** | The spec deprecates DCR, but real clients still use it. Removing it would break the consumers' current onboarding for a mechanism the spec keeps working during the deprecation window. |
| **D4** | Who validates the MCP transport headers? | **The SDK**, with kit tests asserting the kit does not interfere. | Header validation is transport behaviour the SDK owns. Re-implementing it in `mcpmw` would duplicate and drift. |

### Explicitly not in scope

- Domain tools, resources, prompts, user tables, RBAC, or audit storage. Unchanged boundary.
- The `io.modelcontextprotocol/tasks` and MCP Apps extensions. Optional by definition, not required for any purpose
  here, and no consumer has asked.
- Adopting `mark3labs/mcp-go`. `docs/migration/from-mark3labs.md` moves servers *off* it.
- DPoP and token exchange. Already deferred by the `aoa` plan; keep that decision.
- RFC 9728 protected-resource metadata validation. Owned by the `aoa` plan.

---

## Task 1: Bump the SDK and the toolchain

**Files:**

- Modify: `go.mod`, `go.sum`
- Modify: `mise.toml`
- Modify: `.github/workflows/ci.yml`
- Modify: `AGENTS.md` (Go version sentence only; the CI/justfile contradiction is Task 8)

- [ ] Bump the SDK: `go get github.com/modelcontextprotocol/go-sdk@v1.8.0 && go mod tidy`.
- [ ] Align the Go toolchain with the consumers. Decide 1.26.x or 1.27.x and apply it in **one** pass to `go.mod`
      (`go` and `toolchain`), `mise.toml`, and the `go-version` in `.github/workflows/ci.yml`. Do not leave
      `mise.toml` and CI disagreeing — that is how a local pass becomes a CI failure.
- [ ] Confirm the SDK's protocol ceiling moved: `go list -m github.com/modelcontextprotocol/go-sdk` reports v1.8.0,
      and a temporary assertion that `mcp.LatestProtocolVersion` (or the SDK's exported equivalent) is `2026-07-28`
      passes.
- [ ] Leave `docs/lessons.md` prefixes and `CHANGELOG.md` untouched in this task; Task 8 owns the release notes.

**Verify:**

```bash
just vet
just test
just test-race
just lint-go
```

**Note:** the probe above already ran `go build ./...`, `go vet ./...`, and `go test ./...` clean at v1.8.0, so an
unexpected failure here means something else changed in the tree, not that the bump is risky.

---

## Task 2: Support stateless Streamable HTTP

This is the actual blocker. At 2026-07-28 the SDK's Streamable HTTP handler **rejects** requests at that revision
unless `StreamableHTTPOptions.Stateless` is true, so a consumer following the kit's current guidance cannot serve the
revision at all.

**Files:**

- Modify: `mcpkit/server.go` (config surface), `mcpkit/doc.go` (usage)
- Modify: `_examples/minimal-server/main.go` (wire the real handler)
- Add: `docs/recipes/stateless-http.md`

- [ ] Add an explicit config knob for the transport mode (for example `StatelessHTTP bool`) rather than inferring it,
      and document that it must be paired with `StreamableHTTPOptions{Stateless: true}` on the consumer's handler.
- [ ] Do **not** implement the SDK handler inside the kit. The consumer still constructs it; the kit supplies the
      setting, the middleware, and the guidance. Keeping handler construction in the consumer is what preserves the
      kit/consumer boundary.
- [ ] Record the consequence plainly in `doc.go` and the recipe: in stateless mode the server cannot make
      server-initiated requests, because there is no reverse channel from a temporary session. Consumers that need
      elicitation, sampling, or roots must either stay on a legacy revision or use Multi Round-Trip Requests.
- [ ] Write `docs/recipes/stateless-http.md` in the style of `docs/recipes/admin-gate.md`: when to use it, the exact
      handler configuration, the middleware order, and the failure a consumer actually sees when they forget the
      flag.

**Verify:**

```bash
just vet
just test
```

Plus the Task 3 reference-server test, which is where this behaviour is really proven.

---

## Task 3: Replace the fabricated protocol fixtures with a real SDK-backed server

**Files:**

- Modify: `testkit/server.go` (delete `handleMCP`; serve the real SDK handler)
- Modify: `testkit/handshake.go` (replace the session-returning handshake helper with a modern discovery helper)
- Modify: `testkit/token.go` if the server fixture changes its auth shape
- Add: `testkit/SDK_VERSION`-style guard or an assertion in `testkit/server_test.go` pinning the advertised revision
- Modify: `testkit/testkit_test.go`, `mcpkit/server_test.go`

- [ ] Build a consumer-shaped `mcp.Server` from the SDK inside the testkit: register one or two throwaway tools, set
      instructions and server identity, and serve it through `mcp.StreamableHTTPHandler` with the option the Task 2
      knob selects.
- [ ] Delete the hand-written `handleMCP` switch entirely. A fixture that answers `ping` and `initialize` by hand is
      the reason the suite could not see this migration.
- [ ] Make the advertised revision come from the SDK, never a literal. Delete `"2025-03-26"` from
      `testkit/server.go` and `_examples/minimal-server/main.go`.
- [ ] Replace `RunHandshake` with a modern-only helper (for example `RunDiscovery`). The current signature returns
      `session.ID()` and callers send `Mcp-Session-Id`; at 2026-07-28 neither exists. Delete the session plumbing
      rather than keeping a legacy variant beside it, and record the removal as a breaking change.
- [ ] Add a test asserting the modern path works with **no** `initialize` and **no** `Mcp-Session-Id`, and that
      `server/discover` answers `supportedVersions: ["2026-07-28"]` exactly.
- [ ] Add a test asserting a legacy `initialize` request does **not** negotiate a legacy session — capture the actual
      response and record it, so the disconnect behaviour is documented rather than assumed.
- [ ] Keep the throwaway tools read-only and clearly marked as fixtures; do not ship anything resembling a domain
      tool.

**Verify:**

```bash
just test
go test -run TestDiscovery -v ./testkit/...
go test -run TestLegacyInitializeRejected -v ./testkit/...
```

---

## Task 4: Stop dropping identity and instructions

**Files:**

- Modify: `mcpkit/server.go`
- Modify: `mcpkit/doc.go`
- Modify: `testkit/server.go` (assert the values survive to `server/discover`)

- [ ] Change `Config.Implementation` from `any` to `*mcp.Implementation` (D2) and apply it, plus `Instructions`, to
      the server the consumer builds. If the kit cannot apply them because the consumer owns the `mcp.Server`
      instance, then expose them the other way — a documented helper the consumer passes them through — rather than
      accepting and discarding them.
- [ ] Add a test that sets a distinctive name, version, and instructions and asserts they appear in the
      `server/discover` response (`supportedVersions`, `capabilities`, `_meta['io.modelcontextprotocol/serverInfo']`,
      `instructions`).
- [ ] Decide and document the `ErrNotImplemented` status of any field still stubbed. `mcpkit.ErrNotImplemented` is a
      real return value in this repo; do not leave a field that is neither implemented nor flagged.

**Verify:**

```bash
go test ./mcpkit/... ./testkit/...
go test -run TestDiscoverReportsIdentity -v ./testkit/...
```

---

## Task 5: Prove the middleware against the 2026-07-28 wire

**Files:**

- Modify: `mcpmw/envelope.go`, `mcpmw/envelope_test.go`
- Modify: `mcpmw/origin.go`, `mcpmw/origin_test.go`
- Modify: `mcpkit/server_test.go`
- Modify: `docs/lessons.md` (new lessons, see below)

- [ ] Assert the envelope rewriter never re-codes or swallows the spec-reserved errors: `-32020` `HeaderMismatch`,
      `-32021` `MissingRequiredClientCapability`, `-32022` `UnsupportedProtocolVersion`. Each must arrive at the
      client as a JSON-RPC error with its code intact.
- [ ] Assert the forbidden `-32002` is never emitted (resource-not-found is `-32602` at this revision).
- [ ] Confirm the existing pass-through rule still holds now that the transport validates headers: 401 and 403 bodies
      must keep `WWW-Authenticate` intact, and Origin denials must precede any auth challenge. This ordering is
      load-bearing per `AGENTS.md` and the `docs/lessons.md` OG/JR records.
- [ ] Test the streaming path: a successful `text/event-stream` response must still pass through untouched, and a
      `subscriptions/listen` stream must not be buffered by the envelope middleware.
- [ ] Test against a **real SDK server** (Task 3) rather than `httptest` hand-shaped bodies, so the prefix matching
      is validated against the error text the SDK actually produces at v1.8.0. If the SDK's plain-text prefixes
      changed, the rewriter silently stops matching and this is the only test that would notice.
- [ ] Add lessons: an `JR-*` entry for "reserved MCP error codes must pass through the envelope rewriter", and a
      `TQ-*` entry for "the SDK owns transport header validation; middleware must not mask `HeaderMismatch`". Update
      any existing lesson whose encoded behaviour this changes.

**Verify:**

```bash
go test ./mcpmw/... ./mcpkit/...
just lint-go-deep
```

---

## Task 6: OAuth surface for 2026-07-28

Coordinate with `docs/plans/2026-07-06_aoa_security_conformance_hardening.md`, which owns RFC 9728 metadata
validation and the bearer/JWKS hardening. This task owns the *revision-driven* OAuth changes only.

**Files:**

- Modify: `oauth/register.go`, `oauth/register_test.go` (`application_type`, CIMD)
- Add: `oauth/cimd.go`, `oauth/cimd_test.go` (Client ID Metadata Documents)
- Modify: `oauth/provider.go`, `oauth/config.go` (`iss` validation)
- Modify: `oauth/consent/*` only if scope-consent behaviour needs the incremental form
- Modify: `oidc/discovery.go` + `oidc/discovery_test.go` if metadata gains fields
- Modify: `docs/migration/skills-mcp.md`, `docs/migration/vorrent.md`, `docs/migration/from-mark3labs.md`

- [ ] Add `iss` (RFC 9207) to authorization responses and **validate it on redemption** against the recorded issuer.
      Do not emit `iss` without validating it; a present-but-unchecked `iss` is worse than absent.
- [ ] Require `application_type` in DCR so OpenID Connect redirect-URI conflicts cannot arise.
- [ ] Implement Client ID Metadata Documents and demote DCR to the documented fallback (D3). Record which clients use
      which path, so the fallback can eventually be retired on evidence rather than guesswork.
- [ ] Key persisted client credentials by issuer and reject reuse across a different authorization server.
- [ ] If incremental scope consent is adopted, express it through `WWW-Authenticate` per SEP-835 rather than a
      bespoke consent error. If it is not adopted, say so in `DESIGN.md` as a decision, not an omission.
- [ ] Update the three migration docs in the same change: they are contracts real migrations follow, and a public API
      change without them makes consumers drift.

**Verify:**

```bash
go test ./oauth/... ./oidc/...
go test -run TestIssValidation -v ./oauth/...
go test -run TestApplicationType -v ./oauth/...
```

---

## Task 7: Make revision support provable, then prove it

Without this task, every revision claim in the README, `DESIGN.md`, and Task 8's release notes is an assertion.

**Files:**

- Add: `scripts/conformance/run.sh` (or a `just conformance` recipe)
- Add: `conformance-baseline.yml`
- Modify: `.github/workflows/ci.yml`
- Modify: `docs/cycle-methodology.md`

- [ ] Pin the official CLI exactly: `npx --yes @modelcontextprotocol/conformance@0.1.16`.
- [ ] Run the frozen per-revision requirement set, not the growing suite: `--requirements 2026-07-28`. Do not add a
      `2025-11-25` run — D1 made the kit modern-only, so a legacy-scored suite would measure an era the kit
      deliberately does not serve.
- [ ] Run it against the Task 3 SDK-backed reference server (that is the only server in this repo that speaks the
      protocol), and document the consumer-facing command in `docs/cycle-methodology.md` so each consumer runs it
      against its own deployment.
- [ ] Use per-check baseline entries (`<scenario>:<check-id>`) for anything not yet passing, never whole-scenario
      entries. `server-stateless` alone is over twenty checks; excusing the scenario hides the other nineteen.
- [ ] Wire the run into CI on the existing self-hosted `haakco-build` runner. Do not add a new workflow.
- [ ] Cross-check the `aoa` plan's external smoke and this suite. They answer different questions (library-level
      metadata/security probes vs protocol conformance); keep both and say so, so neither is deleted as a duplicate.

**Verify:**

```bash
just conformance
# expected: scored scenarios pass for 2026-07-28; any baseline entry names a specific check and a reason
```

**Show it can fail:** deliberately advertise `logging` or drop a tool annotation and confirm a scored check fails.
A conformance run that has never failed is not evidence.

---

## Task 8: Contracts, docs, and release notes

**Files:**

- Modify: `CHANGELOG.md`
- Modify: `DESIGN.md`
- Modify: `AGENTS.md`
- Modify: `README.md`
- Modify: `docs/lessons.md` (final pass)
- Modify: `docs/plans/README.md` (move this plan to the archive table when complete)

- [ ] Release as **v0.6.0** with a `CHANGELOG.md` entry per repo policy: pre-1.0 allows breaking changes between
      minors, but they must be documented with migration notes. Note explicitly that the SDK bump alone is
      behaviourally invisible, and that the real changes are the stateless requirement, the fixture replacement, the
      identity/instructions fix, and the OAuth additions.
- [ ] Correct the stale `AGENTS.md` claims: there *is* a justfile, a `mise.toml`, and CI, and the Go version is
      whatever Task 1 settled on. List the real recipes so the next reader uses `just` instead of raw `go`.
- [ ] Record D1–D4 in `DESIGN.md`'s decision log: modern-only (D1), the `*mcp.Implementation` change (D2), and the
      DCR-as-fallback rationale (D3). `DESIGN.md` is what the next maintainer reads before changing public API.
- [ ] State the consequence of modern-only in `README.md` in the consumer's terms: the kit serves one wire era, a
      legacy-only client cannot connect, and the migration note names the helper that was replaced and the
      `SupportedProtocolVersions` value to set. Do not soften this into "dual-era available on request" — the code
      does not offer it.
- [ ] Hand the break to the consumers explicitly. `skills-mcp` and `vorrent` each need their own change; a note in
      this repo's `CHANGELOG.md` is not notification.
- [ ] State the deprecation-window position: logging, roots, and sampling are deprecated at 2026-07-28 and remain
      functional for at least twelve months, so consumers should migrate off roots/sampling guidance now and expect
      removal later. This is guidance for consumers, not new kit code.
- [ ] Do not touch `docs/plans/2026-07-06_aoa_security_conformance_hardening.md` or the master plan beyond adding a
      cross-reference; they own their own scope.

**Verify:**

```bash
just quality
bash -n scripts/conformance/run.sh
```

---

## Task 9: Verify on a downstream consumer, then roll out

**Files:**

- Modify: `docs/cycle-methodology.md` (record the modern-only verification steps)
- Modify: `CHANGELOG.md` (finalize only after this task passes)
- No behaviour changes in this repo; the consumer-side edits happen in `skills-mcp` and `vorrent`

This task is not optional polish. `AGENTS.md` already requires that kit changes affecting transport, auth, or
discovery be re-verified with the bootstrap probe (`PR-02`) on at least one downstream consumer before a release is
tagged, and this migration touches all three. The plan has no downstream evidence without it.

- [ ] Upgrade **one** consumer first, not both. The master plan's migration order is deliberately serial so the kit
      API is validated against one real consumer before the next adopts it. Keep that property.
- [ ] Per consumer, in that consumer's own repo and its own plan:
  1. Confirm the consumer's clients speak `2026-07-28`. Probe with `server/discover`. A legacy-only client cannot
     connect at all, and this is the check that prevents an outage dressed up as a library upgrade.
  2. Bump `mcp-kit` to the released version and `go-sdk` to v1.8.0.
  3. Narrow to `SupportedProtocolVersions: ["2026-07-28"]`, or inherit the kit's modern-only default.
  4. Replace the `RunHandshake` call site with the new discovery helper.
  5. Turn on stateless HTTP: Task 2's config knob plus `StreamableHTTPOptions{Stateless: true}` on the consumer's
     handler.
  6. Run the frozen conformance suite against the **consumer's** deployment, not the kit's reference server.
  7. Update the consumer's own migration and release notes. A line in this repo's `CHANGELOG.md` is not notification.
- [ ] Run the bootstrap probe (`PR-02`) and record the result in `docs/lessons.md` as this migration's downstream
      evidence. If it fails, the fix belongs here, before the tag, not in the consumer.
- [ ] Only then finalize `CHANGELOG.md` and tag v0.6.0.
- [ ] Upgrade the second consumer after the first is verified in its own deployment, not simultaneously.

**Verify:**

```bash
just quality
just conformance
# then, against the first upgraded consumer deployment:
#   PR-02 bootstrap probe, plus
#   npx @modelcontextprotocol/conformance server --url <consumer-url> --requirements 2026-07-28
```

**If a consumer cannot confirm modern clients:** stop, and do not tag. A modern-only library that cannot be adopted is
worse than an unupgraded one. The honest options are to wait, or to revisit D1 as a deliberate decision with that
evidence — never to ship and find out in a consumer's production.

---

## Final Verification

```bash
# Full local gate.
just quality

# Revision support at 2026-07-28 — the only revision the kit serves.
just conformance

# Prove the modern path is genuinely modern.
go test -run TestNoHandshakeNoSession -v ./testkit/...
go test -run TestDiscoverReportsIdentity -v ./testkit/...
go test -run TestLegacyInitializeRejected -v ./testkit/...

# Downstream verification, required before the tag (repo rule for transport changes).
#   PR-02 bootstrap probe + the conformance suite against one consumer deployment.
```

## Completion Criteria

1. `go.mod` pins go-sdk **v1.8.0**; `mise.toml`, `go.mod`, and CI agree on one Go version; `just quality` is green.
2. The kit's test fixtures serve the **real SDK handler**. No hardcoded protocol revision remains outside
   `CHANGELOG.md` and migration docs.
3. A modern path is proven with no `initialize`, no `Mcp-Session-Id`, `server/discover` answering
   `supportedVersions: ["2026-07-28"]`, and `resultType` on results. A legacy `initialize` is confirmed to be
   rejected rather than silently negotiated.
4. `Config.Implementation` and `Config.Instructions` either take effect or are documented as unimplemented; neither is
   silently discarded.
5. The envelope rewriter is proven not to mask `-32020`/`-32021`/`-32022`, never emits `-32002`, and still passes 401/
   403 and streaming responses through untouched.
6. `iss` is emitted **and** validated; `application_type` is required in DCR; CIMD is implemented with DCR documented
   as the fallback.
7. The official conformance suite passes the frozen `2026-07-28` requirement set, with any baseline entry naming a
   specific check and a reason, and a demonstrated failure proving the gate can fail.
8. `CHANGELOG.md` v0.6.0 carries migration notes; `DESIGN.md` records D1–D4; `AGENTS.md` no longer contradicts the
   repo; the three `docs/migration/*.md` contracts are updated.
9. At least one consumer is verified end to end on `2026-07-28` — modern clients confirmed, conformance green against
   its own deployment, bootstrap probe (`PR-02`) recorded — **before** v0.6.0 is tagged. Without this the migration's
   downstream evidence does not exist, and the repo's own rule for transport changes is unmet.

## What This Plan Deliberately Does Not Do

| Not doing | Why |
|---|---|
| Dual-era support | Decided against 2026-09-25. The kit serves `2026-07-28` only; a legacy-only client cannot connect, and each consumer confirms its clients are modern before upgrading. Adding a second wire era back is a deliberate future decision, not a default. |
| Building the SDK handler inside the kit | The consumer owns the `mcp.Server` and its tools. The kit supplies configuration and middleware. Moving handler construction into the kit would make it a framework and start absorbing domain concerns. |
| Re-implementing transport header validation | The SDK owns it and returns `-32020`. Duplicating it in `mcpmw` creates two authorities that will diverge. |
| SQL/DDL, or any change to Ent mixin shape | Not required by this revision. If a Task 6 field needs storage, raise it as a separate scoped change with its own migration. |
| Adding tasks/MCP Apps extensions | Optional by definition, unrequested, and each adds a maintained surface with no consumer. |
