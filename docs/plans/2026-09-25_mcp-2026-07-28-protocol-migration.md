# MCP 2026-07-28 Protocol Migration and Release Plan

**Status:** Ready for execution. Reconciled 2026-09-25.

**Goal:** Make `mcp-kit` and its two verified Go consumers conform to MCP revision **2026-07-28**, release the
breaking library change as `v0.6.0`, and leave one current, executable plan in place of the unfinished master and
AOA plans.

**Background:** The checkout pins `github.com/modelcontextprotocol/go-sdk` v1.6.1 and its testkit fabricates the
retired session/initialize protocol. The 2026-07-28 specification removes sessions and initialization, requires
`server/discover`, introduces required request metadata and cache declarations, and changes OAuth client discovery.
The official Go SDK v1.8.0 implements that revision. Work that remains valid from the master and AOA plans is folded
in here; completed or superseded work is recorded in the supersession ledger.

**Architecture:** Consumers continue to own the SDK `mcp.Server`, domain tools/resources/prompts, user data, RBAC,
and audit storage. The kit continues to own OAuth, discovery, key rotation, middleware, CLI auth, Ent mixins, and
shared test support. The kit must not add configuration it cannot apply: consumers configure their stateless SDK
HTTP handler and server capabilities, while the kit supplies tested middleware and exact guidance.

**Tech stack:** Go 1.26.4, official MCP Go SDK v1.8.0, Ory Fosite, go-jose/v3, Ent, stdlib `net/http`, GitHub
Actions, and `@modelcontextprotocol/conformance` v0.1.16.

**Delivery model:** Tasks 1-7 produce one locally proven kit commit and push. `skills-mcp` then consumes that exact
commit through a Go pseudo-version and proves the release candidate in its real deployment. Only then is that kit
commit tagged `v0.6.0`. Both consumers are subsequently pinned to v0.6.0, verified, committed, pushed, and deployed
serially. Deployment and tag publication require normal owner approval at execution time. Never commit a local
`replace` directive in a consumer.

---

## 1. Verified baseline and decisions

Verified on 2026-09-25:

- `mcp-kit`, `skills-mcp`, and `vorrent` are clean on `main`.
- `go test ./... -count=1`, `go vet ./...`, and `just lint-go` pass in `mcp-kit` before migration.
- All three modules pin Go SDK v1.6.1. Latest stable v1.8.0 supports 2026-07-28.
- `skills-mcp` pins `mcp-kit` v0.5.13; `vorrent` pins v0.5.8.
- No Meridian checkout or consuming Go module exists at either path recorded by the master plan. Meridian is not a
  release gate for this migration.
- The repo is public, MIT licensed, and has CI. `CONTRIBUTING.md`, `SECURITY.md`, and
  `docs/migration/new-server.md` are missing.
- The SDK bump alone compiles and passes the existing suite, but that is not protocol proof: the testkit and example
  implement the legacy wire by hand.
- SDK v1.8.0 removes `DisableLocalhostProtection`; vorrent currently sets it and needs a source change.

Primary authorities:

- [MCP 2026-07-28 specification](https://modelcontextprotocol.io/specification/2026-07-28)
- [MCP 2026-07-28 key changes](https://modelcontextprotocol.io/specification/2026-07-28/changelog)
- [Official Go SDK v1.8.0 release](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0)
- [Official MCP conformance framework](https://github.com/modelcontextprotocol/conformance)

Decisions:

1. **Modern-only protocol.** Restrict every server to `2026-07-28`. Remove initialize fallback, sessions, GET SSE,
   and legacy test helpers.
2. **No dead transport knob.** Do not add `StatelessHTTP` to `mcpkit.Config`; the kit cannot apply it to a handler
   the consumer has already built. Consumers set `StreamableHTTPOptions{Stateless: true}`.
3. **Remove dead public fields.** Remove `mcpkit.Config.Implementation`, `Config.Instructions`, and unused
   `mcpkit.ErrNotImplemented`. Consumers already set identity/instructions on their SDK server. Document the
   pre-1.0 breaking cleanup.
4. **CIMD plus DCR fallback.** Prefer Client ID Metadata Documents. Retain Dynamic Client Registration as deprecated
   compatibility fallback and validate `application_type` when supplied.
5. **Official conformance owns black-box checking.** Do not build an AOA runner. Preserve useful AOA-derived RFC
   9728, challenge, JWT, and JWKS unit probes.
6. **Conservative caching.** Authenticated or caller-varying results are `private`; use `public` only when results
   are proven identical and safe across callers. Do not rely on the SDK's public default.
7. **No deprecated capabilities by default.** Use an explicit empty `ServerCapabilities` so SDK v1.8.0 does not
   advertise historical default logging. Tool/resource/prompt capabilities remain inferred from handlers.
8. **Release v0.6.0, not v1.0.0.** This is a breaking pre-1.0 migration. Stable v1 remains deferred until the real
   consumers have operated on the new wire.

Not in scope: domain tools/resources/prompts; tasks and MCP Apps extensions; deprecated roots, sampling, or logging;
DPoP; token exchange; mTLS; or migration back to `mark3labs/mcp-go`.

---

## 2. Specification contract to prove

Use the official 2026-07-28 specification as authority. Evidence must cover:

- `server/discover` works without initialization and the reference client uses it first; initialize and
  `notifications/initialized` are absent.
- Streamable HTTP is stateless: POST only, no `Mcp-Session-Id`, DELETE termination, GET SSE, or `Last-Event-ID`.
- Requests carry per-request `_meta`; HTTP requests carry `Mcp-Protocol-Version` and `Mcp-Method`, plus `Mcp-Name`
  for named tools/resources/prompts. Header/body disagreement returns `-32020`.
- Every result has `resultType`. Discover, four list methods, and resource reads have `ttlMs` and `cacheScope`.
- `-32020`, `-32021`, and `-32022` retain protocol meanings; `-32020` through `-32099` remain reserved. Missing
  resources use invalid params (`-32602`), not retired `-32002`.
- `subscriptions/listen` replaces the GET stream and resource subscribe/unsubscribe. The kit need not advertise
  subscriptions, but middleware must not buffer a future stream.
- JSON Schema draft 2020-12 works. External references are disabled unless explicitly bounded and enabled; schema
  traversal has deterministic resource limits.
- Protected Resource Metadata names the authorization server. Bearer challenges include `resource_metadata` and an
  appropriate `scope`; `offline_access` is not a resource requirement.
- Authorization responses include `iss`; CLI clients validate it when present before code exchange.
- Client ID Metadata Documents use exact HTTPS URL client IDs, validated redirects, bounded safe fetches, and HTTP
  cache semantics. DCR remains fallback.
- Tokens stay resource/audience bound. JWKS never exposes private key material; validation rejects `none`, HMAC
  confusion, and algorithms outside policy.

---

## 3. Dependency map

```text
Task 1 SDK/toolchain baseline
  ├─ Task 2 real reference server and API cleanup
  ├─ Task 3 transport/result contract
  └─ Task 4 OAuth/security contract
Tasks 2-4 ── Task 5 docs/policy ── Task 6 conformance/CI
Tasks 1-6 ── Task 7 kit candidate commit/push
Task 7 ── Task 8 skills-mcp candidate rollout
Task 8 ── Task 9 v0.6.0 tag and skills-mcp repin
Task 9 ── Task 10 vorrent rollout ── Task 11 closeout
```

Tasks 3 and 4 may run in parallel after Task 1 only with non-overlapping file ownership. The coordinator owns
integration, review, and final verification. Do not archive this plan before Task 11.

---

## 4. Task 1 — upgrade SDK and align toolchain

**Files:** `go.mod`, `go.sum`, `mise.toml`, `.github/workflows/ci.yml`, `AGENTS.md`.

- [ ] Set module, local toolchain, CI, and repository instructions to Go 1.26.4. This meets SDK v1.8.0 without
      forcing vorrent onto Go 1.27.
- [ ] Upgrade the Go SDK to v1.8.0 and run `go mod tidy`.
- [ ] Assert through `mcp.SupportedProtocolVersions()` that `2026-07-28` is available.
- [ ] Correct the stale AGENTS claim that no justfile or CI config exists.

**Proof:**

```bash
go version
go list -m github.com/modelcontextprotocol/go-sdk
just build
just test
just vet
just lint-go
```

---

## 5. Task 2 — replace fabricated fixtures and clean public API

**Files:** `testkit/server.go`, `testkit/handshake.go`, tests, `_examples/minimal-server/main.go`,
`mcpkit/server.go`, `mcpkit/errors.go`, `mcpkit/doc.go`, `CHANGELOG.md`.

- [ ] Replace hand-written JSON-RPC switches with a real `mcp.Server` and Streamable HTTP handler.
- [ ] Configure only `2026-07-28`, explicit empty capabilities, explicit caching, and `Stateless: true`.
- [ ] Replace `RunHandshake` with a discovery helper returning discover data, not a session ID. Update `ListTools`
      and callers for stateless requests.
- [ ] Remove initialize, initialized-notification, ping, and session fixtures/assertions.
- [ ] Remove the dead config fields and error named in Decision 3; add exact migration notes.
- [ ] Keep `Origin → Bearer → Envelope → SDK handler` unchanged.

**Proof:**

```bash
go test -v ./testkit ./mcpkit ./_examples/minimal-server
rg '2025-03-26|Mcp-Session-Id|RunHandshake|ErrNotImplemented' testkit mcpkit _examples
```

No live legacy implementation may remain.

---

## 6. Task 3 — prove transport and result behavior

**Files:** `mcpmw/*`, `testkit/*`, focused integration tests, `docs/lessons.md`.

- [ ] Prove POST succeeds and GET/DELETE fail with documented status and `Allow` behavior.
- [ ] Prove session headers are neither required nor emitted and old session headers do not restore state.
- [ ] Prove protocol/method/name headers and `_meta`, including `-32020` mismatch, `-32021` capability, and
      `-32022` unsupported-version failures.
- [ ] Prove discover/list/read results expose `resultType`, `ttlMs`, and `cacheScope`.
- [ ] Prove authenticated/user-varying handlers use `private`; add a `public` fixture only for a genuinely invariant
      result.
- [ ] Prove deterministic list ordering and `-32602` for a missing resource.
- [ ] Prove middleware preserves SDK protocol errors, does not buffer SSE/subscription bodies, and rewrites only the
      known plain-text errors it owns.
- [ ] Exercise draft 2020-12 schemas, local `$ref`, rejected external `$ref`, and complexity bounds through SDK
      behavior. Add kit code only if the SDK lacks a required boundary.
- [ ] Rewrite legacy session/initialize lessons and dispatch runbooks around discovery and stateless requests.

**Proof:**

```bash
go test -v ./mcpmw ./testkit ./mcpkit
go test -race ./mcpmw ./testkit ./mcpkit
```

---

## 7. Task 4 — finish OAuth and security migration

**Files:** `oauth/*`, `oidc/*`, `cliauth/*`, related tests, `docs/lessons.md`.

### 4.1 Metadata and Bearer challenges

- [ ] Add strict validation for issuer/resource URLs, authorization servers, supported schemes, and explicit
      localhost development exceptions. Preserve existing pathful metadata routing.
- [ ] Emit exact, safely escaped challenges for missing, invalid, insufficient-scope, expired, and wrong-audience
      tokens. Include appropriate scope on both 401 and 403.
- [ ] Quote `resource_metadata` and auth parameters correctly; test commas, quotes, and backslashes.
- [ ] Remove `offline_access` from resource challenges and protected-resource `scopes_supported` examples.
- [ ] Retain audience/resource-indicator enforcement for OAuth and PAT paths.

### 4.2 Issuer identification

- [ ] Add `iss` to successful authorization responses.
- [ ] In `cliauth`, validate `iss` against the configured issuer when present and reject mismatch before token
      exchange. Document compatibility behavior for a legacy issuer that omits it.

### 4.3 CIMD and DCR fallback

- [ ] Resolve HTTPS URL client IDs through a small injectable Client ID Metadata Document fetcher.
- [ ] Require exact document `client_id`; validate shape, required fields, redirects, response types, and grants.
- [ ] Bound size, time, redirects, and DNS/IP targets; reject credentials, fragments, non-HTTPS production URLs,
      loopback/private/link-local targets after resolution, and redirect escapes. Keep localhost exceptions explicit.
- [ ] Respect HTTP cache headers with a bounded provider-owned in-memory cache; do not add a cache service.
- [ ] Keep DCR as deprecated fallback. Parse and validate `application_type` when supplied; do not misstate the
      client-side requirement as a server requirement.
- [ ] Advertise supported registration mechanisms in authorization-server metadata per the final SDK/spec shape.

### 4.4 JWT and JWKS hardening

- [ ] Prove JWKS publishes active and grace-period public keys but never private RSA parameters.
- [ ] Prove validation rejects `alg=none`, HMAC confusion, unknown `kid`, malformed keys, algorithms outside policy,
      and signatures made with expired retired keys.
- [ ] Keep DPoP, token exchange, and mTLS explicitly deferred; the core spec does not require them and no verified
      consumer needs them.

**Proof:**

```bash
go test -v ./oauth/... ./oidc ./cliauth
go test -race ./oauth/... ./oidc ./cliauth
```

---

## 8. Task 5 — finish migration and maintainer documentation

**Files:** `README.md`, `DESIGN.md`, `CHANGELOG.md`, `docs/cycle-methodology.md`, `docs/lessons.md`, migration docs,
`docs/migration/new-server.md` (new), `docs/recipes/stateless-http.md` (new), `CONTRIBUTING.md` (new),
`SECURITY.md` (new), `AGENTS.md`.

- [ ] Add a new-server guide with exact modern-only server options, stateless handler, middleware order, OAuth
      routes, private-cache default, required CORS headers, and first discover/tools-list probe.
- [ ] Add a focused stateless recipe. Explain the absence of server-initiated requests and when Multi Round-Trip
      Requests would be needed; do not implement unused MRTR infrastructure.
- [ ] Remove live guidance for sessions, initialize, GET SSE, deprecated capabilities, and `offline_access` resource
      metadata.
- [ ] Update both consumer migration contracts with the breaking API and rollout steps below.
- [ ] Add concise contribution commands and security reporting/supported-version policy without an unsustainable SLA.
- [ ] Correct repository/toolchain facts in AGENTS and link this as the active plan.
- [ ] Record why DPoP, token exchange, mTLS, and deprecated capabilities remain out of scope.

**Proof:**

```bash
rg 'Mcp-Session-Id|2025-03-26|notifications/initialized|logging/setLevel|resources/subscribe' \
  README.md DESIGN.md docs _examples testkit
```

Every remaining match must be intentional historical or migration text.

---

## 9. Task 6 — official conformance and proportionate CI

**Files:** `scripts/conformance/run.sh` (new), `conformance-baseline.yml` (new), `justfile`, CI, conformance docs.

- [ ] Add one runner for `@modelcontextprotocol/conformance@0.1.16`, pinned exactly. Start the reference server,
      use a bounded readiness loop, invoke `server --requirements 2026-07-28`, preserve failures, and always clean
      up.
- [ ] Keep `conformance-baseline.yml` empty for the released 2026-07-28 requirement set. If a temporary expected
      failure is needed while implementing, require a reason, owner, and removal condition; any remaining required
      failure blocks Task 7.
- [ ] Add `just conformance` and a CI job selected for transport/OAuth/reference-server changes. Documentation-only
      changes must not run unrelated Go or conformance suites.
- [ ] Add the ecosystem-approved `govulncheck` invocation and document the identical local command.
- [ ] Demonstrate the gate fails for one temporary protocol violation before restoring the passing implementation.
- [ ] Do not add the AOA CLI or a second black-box harness.

**Proof:**

```bash
just conformance
just quality
git diff --check
```

Record CLI version, profile, totals, baseline entries, and duration in the evidence log.

---

## 10. Task 7 — commit and push the kit release candidate

- [ ] Review the full diff for public API, security, and migration accuracy.
- [ ] Run once on the stable tree:

```bash
go mod tidy
git diff --exit-code -- go.mod go.sum
just build
just test
just test-race
just vet
just lint-go
just conformance
git diff --check
```

- [ ] Commit the coherent kit migration, push `main`, record the SHA, and confirm `origin/main` equals it.
- [ ] Do not tag yet; first prove this exact commit in a real consumer.

---

## 11. Task 8 — validate the candidate in skills-mcp

**Checkout:** `/Volumes/Dev/HaakCo/AiProjects/skills`

**Likely files:** `apps/skills-mcp/go.mod`, `go.sum`, `internal/server/server.go`, server tests,
`internal/web/cors.go`, auth/API helpers, and live MCP docs.

- [ ] Re-read nearest AGENTS files and verify a clean worktree.
- [ ] Use `go get github.com/haakco/mcp-kit@<task-7-sha>` to obtain the exact pseudo-version. Never commit a local
      filesystem `replace`. Upgrade the direct SDK dependency to v1.8.0.
- [ ] Restrict HTTP and stdio servers to 2026-07-28; set explicit empty capabilities and cache policy. Use `private`
      for authenticated/caller-varying results.
- [ ] Set HTTP `Stateless: true`; remove session/initialize assumptions from tests and helpers.
- [ ] Allow `Mcp-Protocol-Version`, `Mcp-Method`, and `Mcp-Name` through CORS; allow `Mcp-Param-*` only if used. Stop
      exposing `Mcp-Session-Id`.
- [ ] Update challenge expectations, metadata, live runbook, and migration notes.
- [ ] Run repository-defined focused/boundary suites plus official conformance.
- [ ] With owner approval, deploy and run authenticated `server/discover` plus `tools/list`. Redefine PR-02 around
      this stateless path.
- [ ] Commit and push only after local proof. Record commit, CI, deployment, and hosted probe separately.

**Gate:** SDK reports 2026-07-28, no session header appears, discover/tools-list conform, invalid tokens recover via
advertised metadata, and the hosted service remains healthy.

---

## 12. Task 9 — publish v0.6.0 and repin skills-mcp

- [ ] Confirm the deployed candidate used exactly the Task 7 kit commit and all gates passed.
- [ ] With owner approval, tag that exact commit `v0.6.0` and push the tag. Do not create a release-only source
      commit after consumer proof.
- [ ] Confirm the public tag resolves to the proven commit and the module proxy/checksum path fetches it.
- [ ] Replace the skills-mcp pseudo-version with v0.6.0, tidy, rerun focused dependency/build/tests, commit, push,
      and redeploy with owner approval.
- [ ] Re-run hosted discover/tools-list and record final version evidence.

---

## 13. Task 10 — update and verify vorrent

**Checkout:** `/Volumes/Dev/HaakCo/AiProjects/vorrent`

**Likely files:** `go.mod`, `go.sum`, `internal/mcpserver/server.go`, `internal/api/http_helpers.go`,
`internal/api/http_server.go`, MCP tests, and live docs.

- [ ] Re-read nearest AGENTS files and verify a clean worktree.
- [ ] Upgrade `mcp-kit` to v0.6.0 and SDK to v1.8.0.
- [ ] Remove deleted `DisableLocalhostProtection`. Preserve the kit's explicit Origin policy; do not weaken
      localhost/DNS-rebinding protection to pass tests.
- [ ] Apply modern-only options, stateless handler, explicit capabilities/cache policy, CORS headers, and
      session-free tests as in skills-mcp.
- [ ] Update live docs and PR-02-style probes; preserve archived plans as history.
- [ ] Run repository-defined focused/boundary suites and official conformance.
- [ ] Commit and push after local proof. With owner approval, deploy and run authenticated discover/tools-list and
      OAuth recovery. Record commit, CI, deployment, and hosted evidence separately.

---

## 14. Task 11 — final acceptance and closeout

- [ ] `mcp-kit` main and v0.6.0 resolve to the same proven commit.
- [ ] Kit build, unit, race, vet, lint, vulnerability, and official conformance checks pass.
- [ ] The 2026-07-28 required conformance set has no expected-failure baseline entries.
- [ ] Skills-mcp and vorrent pin v0.6.0; commits are pushed and CI is green.
- [ ] Both hosted consumers pass authenticated 2026-07-28 discover/tools-list and OAuth recovery probes.
- [ ] No live docs/fixtures teach initialize, sessions, GET SSE, deprecated capabilities, or DCR as primary.
- [ ] README, DESIGN, changelog, migration guides, policies, lessons, and cycle methodology agree.
- [ ] Old master and AOA plans remain marked superseded; all valid remainder is represented here.
- [ ] Only then move this file to
      `docs/plans/archive/<completion-date>-2026-09-25_mcp-2026-07-28-protocol-migration.md` and update the index.

Stable v1.0.0 is a separate future decision based on operating evidence, not an inherited unchecked box.

---

## 15. Supersession ledger

| Prior item | Disposition |
|---|---|
| Master phase 9: migrate Meridian | Superseded as v0.6 gate: no checkout or consuming module exists. Plan when a real consumer and owner exist. |
| Master phase 11: reusable/new-server docs | Retained in Task 5 and updated for the modern protocol. |
| Master phase 12: public repo, licence, CI | Already complete and verified. |
| Master phase 12: contribution/security policy | Retained in Task 5. |
| Master phase 12: v1.0.0 | Superseded here by v0.6.0; reconsider after operating evidence. |
| AOA task 1: RFC 9728 path/metadata | Path support complete; strict validation remains in Task 4.1. |
| AOA tasks 2 and 5: AOA runner/smoke | Superseded by the official pinned conformance CLI in Task 6. |
| AOA task 3: challenge contract | Retained; latest spec expects scope on the initial challenge. |
| AOA task 4: JWKS/JWT probes | Retained in Task 4.4; reuse existing rotation coverage. |
| AOA task 6: DPoP/token-exchange decision | Documented deferral retained in Task 5. |

---

## 16. Execution evidence log

Fill this during delivery; never turn planned commands into claimed evidence.

| Boundary | Commit/version | Local proof | CI | Hosted/deployed proof |
|---|---|---|---|---|
| mcp-kit release candidate | — | — | — | n/a |
| skills-mcp candidate | — | — | — | — |
| mcp-kit v0.6.0 | — | — | — | module/tag fetch — |
| skills-mcp v0.6.0 | — | — | — | — |
| vorrent v0.6.0 | — | — | — | — |

Record blockers with the exact failed command or external state. A local pass does not substitute for CI, tag,
dependency resolution, deployment, or hosted acceptance.
