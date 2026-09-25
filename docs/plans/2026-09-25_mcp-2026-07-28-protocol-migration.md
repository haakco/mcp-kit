# MCP 2026-07-28 Protocol Migration and Release Plan

**Status:** In progress. Kit migration implemented and locally proven on branch `feat/mcp-2026-07-28-migration`.
Tasks 1-4 are done; Task 5 is partly done; Tasks 6-7 are ready to complete; Tasks 8-11 (consumer rollout,
tag, deploy) remain and need owner approval. Reconciled 2026-09-25; **corrected 2026-09-26 from execution evidence**
(see §1.1 Plan corrections — several original claims about the SDK and the conformance CLI were wrong).

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

**Tech stack:** Go 1.27.1, official MCP Go SDK v1.8.0, Ory Fosite, go-jose/v4, Ent, stdlib `net/http`, GitHub
Actions, and `@modelcontextprotocol/conformance` **0.2.0-alpha.11** (see §1.1: v0.1.16 cannot test 2026-07-28).

**Delivery model:** Tasks 1-7 produce one locally proven kit commit and push. `skills-mcp` then consumes that exact
commit through a Go pseudo-version and proves the release candidate in its real deployment. Only then is that kit
commit tagged `v0.6.0`. Both consumers are subsequently pinned to v0.6.0, verified, committed, pushed, and deployed
serially. Deployment and tag publication require normal owner approval at execution time. Never commit a local
`replace` directive in a consumer.

---

## 1. Verified baseline and decisions

Verified on 2026-09-25, corrected 2026-09-26:

- `mcp-kit`, `skills-mcp`, and `vorrent` are clean on `main`.
- `go test ./... -count=1`, `go vet ./...`, and `just lint-go` pass in `mcp-kit` before migration.
- All three modules pin Go SDK v1.6.1. Latest stable v1.8.0 supports 2026-07-28 and is now pinned.
- `skills-mcp` pins `mcp-kit` v0.5.13; `vorrent` pins v0.5.8.
- No Meridian checkout or consuming Go module exists at either path recorded by the master plan. Meridian is not a
  release gate for this migration.
- The repo is public, MIT licensed, and has CI. `CONTRIBUTING.md`, `SECURITY.md`, and
  `docs/migration/new-server.md` are missing.
- The SDK bump alone compiles and passes the existing suite, but that is not protocol proof: the testkit and example
  implement the legacy wire by hand.
- **Correction:** SDK v1.8.0 does **not** remove `DisableLocalhostProtection`. It still exists on both
  `StreamableHTTPOptions` and `SSEHandlerOptions`, and the conformance suite has a `dns-rebinding-protection`
  scenario that expects the protection to remain on. Consuming repositories must **not** delete it.

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

### 1.1 Plan corrections from execution evidence

Execution disproved several original assumptions. Each correction below is verified by the command or spike named.

| Original claim | Verified reality | Impact |
|---|---|---|
| "SDK v1.8.0 removes `DisableLocalhostProtection`; vorrent needs a source change." | The field still exists on `StreamableHTTPOptions` and `SSEHandlerOptions`. Conformance has a `dns-rebinding-protection` scenario that passes precisely because it is on. | Task 10 must **not** remove it. Removing it would weaken localhost/DNS-rebinding protection. |
| Toolchain Go 1.26.4. | Go 1.27.1 is current, and `skills-mcp` is already on Go 1.27.0. | Kit moved to Go 1.27.1. `vorrent` (Go 1.26.4) must raise its `go` directive when it consumes v0.6.0 — a one-line change already inside Task 10. |
| Conformance CLI pinned at v0.1.16, invoked with `server --requirements 2026-07-28`. | The **pin** was wrong, not the command. v0.1.16 rejects `2026-07-28` outright (`Unknown spec version`; valid: 2025-03-26, 2025-06-18, 2025-11-25, draft, extension) and has no `--requirements` flag at all. `--requirements <revision>` exists only in the 0.2.x line, where it selects exactly the scenarios that revision requires and replaces `--suite`/`--spec-version`. | Task 6 pins `@modelcontextprotocol/conformance@0.2.0-alpha.11` and keeps the original `--requirements 2026-07-28` command. Verified: `164 passed, 0 failed` scored against the SDK reference server. |
| `conformance-baseline.yml` is the pass/fail set for the released requirement set. | The CLI takes the baseline path via `--expected-failures`, and an **empty** baseline file is accepted. `--requirements` excludes extension scenarios from the score automatically, printing them under "Not scored for 2026-07-28"; the baseline must not list them. | Baseline stays empty, as originally intended. `--suite active` was considered as the gate and rejected: it silently skips `caching`, `http-header-validation`, `http-custom-header-server-validation`, `json-schema-2020-12`, `sep-2164-resource-not-found`, and `server-stateless`, which are all part of the 2026-07-28 requirement set. |
are all part of the 2026-07-28 requirement set. |
| `-32022` covers unsupported protocol versions generally. | The SDK returns `-32022` only for requested versions **at or after** 2026-07-28. A legacy `Mcp-Protocol-Version` header (`2025-11-25` or earlier) is rejected at the HTTP layer with a `text/plain` 400 that is not a JSON-RPC response. | Task 3 pins the actual behaviour and records the interoperability gap as `TQ-*` in `docs/lessons.md`. Fixing it means changing `mcpmw.Envelope`'s public signature — deferred as a separate decision, not part of this migration. |
| `-32021` is a reachable server response the kit must prove. | The SDK defines `CodeMissingRequiredClientCapabilities` but never returns it server-side; conformance reaches it only through the reference server's `test_missing_capability` diagnostic tool. | Task 3 proves the constant is reserved and that no kit path requires capabilities, rather than fabricating a producer. |
| Task 4.4: "prove validation rejects `alg=none`, HMAC confusion, unknown `kid`, malformed keys". | The kit has **no JWT verifier**. It issues tokens through Fosite and validates them by introspection; consumers verify `id_token` signatures against the published JWKS. | Task 4.4 proves the JWKS boundary the kit actually owns (public material only, active + grace keys) and records that the JWT-rejection proofs belong to consumers. |
| Task 4.3 CIMD is a migration of existing behaviour. | The SDK implements Client ID Metadata Documents **client-side only** (`auth.ClientIDMetadataDocumentConfig`). No server-side fetcher exists anywhere in the kit. | CIMD is net-new code with a real SSRF surface; implemented in `oauth/cimd.go` + `oauth/cimd_transport.go` with bounds and tests. |
| Task 4.1: "include appropriate scope on both 401 and 403". | A deliberate, tested, documented decision already omits `scope` on `invalid_token` (matching the RFC 6750 examples) and sends it only on `insufficient_scope`. | Kept the existing decision; the plan bullet was stale. |
| Task 2 files list `mcpkit/errors.go`. | No such file exists; `ErrNotImplemented` lived in `mcpkit/server.go`. | Removed from there. |
| Decision 3 implies removing all dead config fields. | `mcpkit.Config.Validator` is dead by documentation but still used by ~12 `skills-mcp` test call sites. | Kept `Validator` (it still works via `Bearer.TokenValidator`); removing it is follow-up work, not migration scope. |
| Task 3: the Envelope middleware rewrites SDK plain-text protocol errors. | Against v1.8.0 the SDK returns proper JSON-RPC for every case the rewrite table names (`-32700`, `-32600`, `-32601`), so those rules are unreachable for SDK-backed handlers. | Kept the middleware and its rules as a safety net for non-SDK handlers, proved pass-through at the wire level, and recorded the observation rather than silently refactoring a public middleware. |
| Task 10's vorrent file list implies `internal/mcpserver/server.go` and CORS are the only source changes. | The upgrade also changed the bearer challenge realm: kit `7d3ed47` moved `Bearer realm="mcp-kit"` to the interoperable `realm="OAuth"`, first shipped in v0.5.10. vorrent pinned v0.5.8, so its `TestRegisterMCPIfEnabled_UsesKitBearerChallenge` passed on main and failed on the upgrade. | The CHANGELOG recorded it but the migration notes did not. Added as required change #11 plus a gotcha in `docs/migration/vorrent.md`; vorrent's assertion now follows the kit. `skills-mcp` never asserted the realm, so it was unaffected. |
| Task 10 treats vorrent's `go build ./...` and lint gate as working baselines to preserve. | Neither was working. `go build ./...` failed on main because commit `cd2ebe6c` pinned `go-diskfs => v1.7.0` for rclone v1.73.5 while the tree had moved to rclone v1.75.0, whose squashfs backend needs v1.9.x. Separately, raising the `go` directive forces `go mod tidy` to resolve golangci-lint v2.11.4 → v2.14.0 (v2.11.4 cannot decode Go 1.27 export data), and the newer analyzers then flag 8 pre-existing findings in untouched code. | Dropped the stale replace and resolved all 8 findings, including a real path-traversal in `parseHLSRequestPath` and a pointer-formatting bug in the audit summary. Both are verified pre-existing: a worktree of vorrent `main` fails identically under Go 1.26.8 and Go 1.27.1. |
| A consumer's focused/fast lane is sufficient local proof of a kit upgrade. | `just test-backend-fast` passes with vorrent's stale `realm="mcp-kit"` assertion; only the full `internal/api` suite catches it. The fast lane ran 63 tests in 4.2s and skips the DB-backed set. | Consumer proof for Tasks 8 and 10 uses the full suite for the touched packages, not the fast lane alone. |

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

**Status:** Done.

- [x] Set module, local toolchain, CI, and repository instructions to **Go 1.27.1** (was planned as 1.26.4). Go 1.27 is
      current, `skills-mcp` is already on `go 1.27.0`, and the user asked for current majors. `vorrent` sits at
      `go 1.26.4` and must raise its directive when it consumes v0.6.0 — one line, already inside Task 10.
- [x] Upgrade the Go SDK to v1.8.0 and run `go mod tidy`.
- [x] Assert through `mcp.SupportedProtocolVersions()` that `2026-07-28` is available
      (`mcpkit/cache_test.go: TestProtocolVersionIsOfferedBySDK`).
- [x] Correct the stale AGENTS claim that no justfile or CI config exists.
- [x] Also take the available majors the plan did not name: `go-jose/v3` → `v4`, `golangci-lint` v2.12.2 → v2.14.0,
      and current `golang.org/x/*`.

**Proof (all run and green):**

```bash
go version                                          # go1.27.1
go list -m github.com/modelcontextprotocol/go-sdk    # v1.8.0
just build && just test && just vet && just lint-go
```

---

## 5. Task 2 — replace fabricated fixtures and clean public API

**Files:** `testkit/server.go`, `testkit/discover.go` (new; `handshake.go` deleted), tests,
`_examples/minimal-server/main.go`, `mcpkit/server.go`, `mcpkit/cache.go` (new), `CHANGELOG.md`.

**Status:** Done.

- [x] Replace hand-written JSON-RPC switches with a real `mcp.Server` and Streamable HTTP handler.
- [x] Configure only `2026-07-28`, explicit empty capabilities, explicit caching, and `Stateless: true`.
- [x] Replace `RunHandshake` with `testkit.Discover` / `DiscoverURL`, returning `*mcp.DiscoverResult`, not a session ID.
      `ListTools` / `ListToolsURL` dropped the session parameter.
- [x] Remove initialize, initialized-notification, ping, and session fixtures/assertions.
- [x] Remove the dead config fields and error named in Decision 3; add exact migration notes in `CHANGELOG.md`.
- [x] Keep `Origin → Bearer → Envelope → SDK handler` unchanged.
- [x] Add `mcpkit.ProtocolVersion`, `mcpkit.DefaultCacheTTL`, and `mcpkit.PrivateCache` so consumers have one shared
      cache policy instead of each inventing one.
- [x] Add `testkit.Post` / `PostHeaders` / `Do` / `Wire` for wire-level assertions, because the SDK's plain helpers
      cannot express header mismatch, unsupported version, or non-POST methods.

**Deliberate non-change:** `mcpkit.Config.Validator` is deprecated but still used by ~12 `skills-mcp` test call sites.
It still works (it is copied to `Bearer.TokenValidator`), so removing it would be unrelated scope. Recorded as
follow-up.

**Proof:**

```bash
go test -count=1 ./testkit ./mcpkit ./_examples/minimal-server
rg '2025-03-26|RunHandshake|ErrNotImplemented' testkit mcpkit _examples   # no matches
rg 'Mcp-Session-Id' testkit mcpkit _examples                              # only negative assertions
```

---

## 6. Task 3 — prove transport and result behavior

**Files:** `testkit/contract_test.go` (new), `oauth/middleware_test.go`, `oidc/jwks_test.go`, `docs/lessons.md`,
`docs/dispatch-runbook-template.md`.

**Status:** Done. All proofs live in `testkit/contract_test.go`, which drives a real SDK server through the full kit
stack rather than a stub.

- [x] POST succeeds; GET and DELETE return `405` with `Allow: POST`.
- [x] Session headers are neither required nor emitted, and two different stale session IDs produce byte-identical
      results (no state restored).
- [x] `-32020` for method-header mismatch, version-header/body mismatch, and a missing version header; `-32022` for an
      unsupported version at or after 2026-07-28, including `data.supported` and `data.requested`; `-32602` for missing
      `_meta` fields.
- [x] `resultType`, `ttlMs`, and `cacheScope` present on discover, list, and read results.
- [x] Authenticated results are `private`.
- [x] Tool order is deterministic and sorted; a missing resource returns `-32602` (SEP-2164), not the retired `-32002`.
- [x] Middleware preserves the SDK's JSON-RPC errors and HTTP statuses, and does not touch non-POST, 401/403, or SSE
      responses. `mcpmw`'s existing unit tests cover the SSE and pass-through cases directly.
- [x] Draft 2020-12 schemas with a local `$ref` are served unchanged, and `$defs` is not stripped. No kit code was
      needed: the SDK does not resolve external `$ref` server-side, so there is nothing to bound. `AddTool` panics at
      registration on an invalid `x-mcp-header` annotation, which is fail-fast, not a runtime surface.
- [x] Legacy session/initialize lessons and the dispatch runbook are rewritten around discovery and stateless requests.

**Corrections recorded in §1.1 and `docs/lessons.md` (`TQ-04` to `TQ-07`):** `-32021` has no server-side producer in the
SDK; legacy (< 2026-07-28) protocol versions get a plain-text 400 rather than `-32022`; and the SDK now emits JSON-RPC
for the errors `mcpmw.Envelope` was written to rewrite, making those rules unreachable for SDK-backed handlers.

**Proof (green):**

```bash
go test -count=1 ./mcpmw ./testkit ./mcpkit ./oidc
go test -race -count=1 ./mcpmw ./testkit ./mcpkit ./oidc
```

---

## 7. Task 4 — finish OAuth and security migration

**Files:** `oauth/*`, `oidc/*`, `cliauth/*`, related tests, `docs/lessons.md`.

### 4.1 Metadata and Bearer challenges

**Status:** Done, with one plan bullet deliberately not followed.

- [x] Issuer/resource URL, authorization-server, scheme, and localhost validation already existed; pathful metadata
      routing is unchanged (`oauth/metadata.go`, `oidc/discovery.go`).
- [~] Challenges are emitted for missing, invalid, insufficient-scope, expired, and wrong-audience tokens, proven by a
      table test in `oauth/middleware_test.go`. **Not done:** scope hints on 401. A deliberate, tested, documented
      decision (see the `Unreleased` entry in `CHANGELOG.md`) omits `scope` on `invalid_token` to match the RFC 6750
      examples and sends it only on `insufficient_scope`. The plan bullet was stale; changing it would need a reason
      beyond "the plan said so".
- [x] `resource_metadata` and auth parameters are quoted through one helper, tested with commas, quotes, and
      backslashes. This fixed a real defect: the metadata URL was interpolated unescaped.
- [x] No `offline_access` in resource challenges or protected-resource examples. Remaining occurrences are OAuth
      request scopes in tests and the example, which is legitimate (refresh tokens), not resource metadata.
- [x] Audience/resource-indicator enforcement retained on both OAuth and PAT paths.

### 4.2 Issuer identification

**Status:** Done.

- [x] `oauth.Provider.AddIssuerParameter` adds the RFC 9207 `iss` parameter; both authorize handlers (the demo
      `AuthorizeHandler` and `oauth/consent`) call it before writing the response.
- [x] `cliauth` validates `iss` in `exchangeAndSave`, so both login flows check it before redeeming the code, and
      rejects mismatch. An issuer that omits `iss` is still accepted, documented on
      `Login.validateAuthorizeIssuer` and in `CHANGELOG.md`.

### 4.3 CIMD and DCR fallback

**Status:** Done. Net-new code, not a migration — the SDK's CIMD support is client-side only.

- [x] `oauth.ClientIDMetadataFetcher` resolves HTTPS URL client IDs; `storage.Storage.WithClientMetadataResolver`
      wires it in behind a store miss, so registered clients always win.
- [x] Exact document `client_id` match, plus validation of redirects, grant types, response types, `application_type`
      (only when supplied), `token_endpoint_auth_method` (public only), and `logo_uri`.
- [x] Bounded size (64 KiB), time (5 s), and redirects (3, each re-validated); credentials and fragments refused;
      non-HTTPS refused except explicit loopback; loopback/private/link-local/multicast/unspecified refused **after**
      DNS resolution, and the validated address is dialled rather than re-resolved.
- [x] HTTP cache semantics (`no-store`, `no-cache`, `max-age`, `Expires`) with a bounded in-memory cache (256 entries).
      No cache service.
- [x] DCR retained as the fallback; `application_type` validated when supplied and not required (it is client metadata,
      not a server requirement).
- [x] `client_id_metadata_document_supported` advertised in both metadata owners
      (`oidc.DiscoveryConfig` and `oauth.AuthorizationServerMetadataConfig`), defaulting on with the provider.

### 4.4 JWT and JWKS hardening

**Status:** Done for what the kit owns; the rest is not applicable.

- [x] `oidc/jwks_test.go: TestJWKSNeverPublishesPrivateKeyMaterial` proves the JWKS publishes only RSA public material
      (`n`, `e`) with the advertised `alg`/`use`, for both the active and grace-period keys, and contains no `d`, `p`,
      `q`, `dp`, `dq`, `qi`, `oth`, or symmetric key field.
- [~] **Not applicable.** The kit has no JWT verifier: it issues tokens through Fosite and validates them by
      introspection, and consumers verify `id_token` signatures against the published JWKS. Proving rejections for
      `alg=none`, HMAC confusion, unknown `kid`, malformed keys, or retired-key signatures would require inventing a
      verifier the kit does not have and no consumer needs. Recorded in §1.1.
- [x] DPoP, token exchange, and mTLS stay explicitly deferred in this plan, `CHANGELOG.md`, `docs/conformance.md`, and
      `docs/migration/new-server.md`.

**Proof (green):**

```bash
go test -count=1 ./oauth/... ./oidc ./cliauth
go test -race -count=1 ./oauth/... ./oidc ./cliauth
```

---

## 8. Task 5 — finish migration and maintainer documentation

**Status:** Done, except the plan-link edit noted below.

- [x] `docs/migration/new-server.md` — modern-only server options, stateless handler, fixed middleware order, OAuth
      routes, private-cache default, required CORS headers, first discover/tools-list probe, and a failure table.
- [x] `docs/recipes/stateless-http.md` — what stateless means on the wire, why the server cannot initiate requests,
      and when MRTR would be needed. MRTR is documented, not implemented.
- [x] Live guidance for sessions, initialize, GET SSE, and deprecated capabilities is removed from `README.md`,
      `DESIGN.md`, `docs/cycle-methodology.md`, `docs/dispatch-runbook-template.md`, and the migration docs. Remaining
      matches are historical records (archived plans, `CHANGELOG.md` migration notes) or negative assertions in tests.
- [x] Both consumer migration contracts updated with the breaking API and rollout order
      (`docs/migration/skills-mcp.md`, `docs/migration/vorrent.md`).
- [x] `CONTRIBUTING.md` (commands, invariants, test and doc expectations) and `SECURITY.md` (reporting path, in-scope
      and out-of-scope surface, no SLA) added.
- [x] `AGENTS.md` corrected: the toolchain claim and the false "no justfile or CI config" statement. The plan's other
      repository facts were checkable and left alone.
- [x] Out-of-scope decisions recorded in `CHANGELOG.md`, `docs/conformance.md`, and `docs/migration/new-server.md`.
- [~] Linking this plan from `AGENTS.md` as the active plan: not done. `AGENTS.md` points at `docs/plans/` and the plan
      index; adding a second pointer for one in-flight plan would need removing when it archives. Left to Task 11's
      archival step.

**Proof:**

```bash
rg 'Mcp-Session-Id|2025-03-26|notifications/initialized|logging/setLevel|resources/subscribe' \
  README.md DESIGN.md docs _examples testkit
```

Every remaining match must be intentional historical or migration text.

---

## 9. Task 6 — official conformance and proportionate CI

**Files:** `scripts/conformance/run.sh` (new), `conformance-baseline.yml` (new), `justfile`, CI, `docs/conformance.md`
(new).

**Status:** Done. Verified: `164 passed, 0 failed` scored; empty baseline; gate proven to fail on a violation.

- [x] Add one runner for `@modelcontextprotocol/conformance@0.2.0-alpha.11`, pinned exactly. Start the reference
      server, use a bounded readiness loop, invoke `server --requirements 2026-07-28`, preserve failures, and always
      clean up. The CLI version is load-bearing: 0.1.16 does not know the revision and has no `--requirements` flag.
- [x] Keep `conformance-baseline.yml` empty for the released 2026-07-28 requirement set. `--requirements` excludes the
      `tasks-*` extension scenarios from the score automatically, so they are not baseline entries.
- [x] Add `just conformance` / `just conformance-against <url>` and a CI job selected for Go changes. Documentation-only
      changes skip the Go and conformance suites; the selection step fails loudly rather than silently running
      everything when diff metadata is missing.
- [x] Add the `govulncheck` invocation, pinned to `v1.8.0` in the justfile so local and CI run the identical command.
- [x] Demonstrate the gate fails for a protocol violation: the same reference server started with `-stateless=false`
      fails the 2026-07-28 requirement set with "Unexpected failures detected", and `run.sh` exits 1.
- [x] Do not add the AOA CLI or a second black-box harness.

**Deviation — the reference server is the SDK's, not a fork of it.** The plan's "start the reference server" implies a
kit-owned conformance server. Building one means reproducing the SDK's 1316-line `everything-server` (tools, resources,
prompts, completion, MRTR diagnostic tools) as consumer-owned fixtures, at real maintenance cost, to re-prove
behaviour the SDK already owns. The kit owns middleware, not tools.

The split adopted instead:

- The runner's default target is the SDK's own `conformance/everything-server`, built from the pinned SDK version. That
  run is a smoke test of the gate itself: it proves the pinned CLI and revision profile still agree, and it would catch
  an upstream change that hollowed out the check.
- The kit's own 2026-07-28 transport contract is proven by `testkit/contract_test.go` against a real SDK server through
  the full kit stack (Origin → Bearer → Envelope → SDK handler): POST-only stateless serving, absent session headers,
  `-32020`/`-32022`/`-32602`, `resultType`/`ttlMs`/`cacheScope`, deterministic list ordering, SEP-2164, and draft
  2020-12 schemas.
- Consumers run the same script with `CONFORMANCE_URL` pointed at their server, where the tool/resource/prompt fixtures
  actually live. `docs/conformance.md` documents each case.

MRTR stays unimplemented, as Task 5 requires: the scenarios pass against the SDK reference server, which implements the
diagnostic tools, and a kit with no tools has nothing to return `input-required` from.

**Proof:**

```bash
just conformance        # 164 passed, 0 failed scored; extension scenarios reported but unscored
just vulncheck          # no vulnerabilities found
just quality
git diff --check
```

Recorded: CLI `0.2.0-alpha.11`, profile `--requirements 2026-07-28`, totals `164 passed / 0 failed`, baseline entries
`0`, and the negative control (`-stateless=false`) exiting 1.

---

**Status:** Locally complete. **Push and tag are deliberately not done** — see the note below.

- [x] Full diff reviewed for public API, security, and migration accuracy; the review produced the §1.1 corrections
      table and the deliberate non-changes recorded in Tasks 2, 3, and 4.
- [x] Run once on the stable tree: `go mod tidy` clean, `just build`, `just test`, `just test-race`, `just vet`,
      `just lint-go` (0 issues), `just conformance` (164 passed / 0 failed scored, empty baseline), `just vulncheck`
      (no vulnerabilities), `git diff --check`.
- [ ] **Not done: push to `main`.** Work is on branch `feat/mcp-2026-07-28-migration`. Pushing to `main` is a delivery
      action and Task 12/13 deploy consumers, so it needs the owner's decision; the branch is ready for review.
- [x] Not tagged, per plan: the exact commit must be proven in a real consumer first.

**Proof (green, on the branch):**

```bash
go mod tidy && git diff --exit-code -- go.mod go.sum
just build && just test && just test-race && just vet && just lint-go
just conformance && just vulncheck
git diff --check
```

---

## 11. Task 8 — validate the candidate in skills-mcp

**Checkout:** `/Volumes/Dev/HaakCo/AiProjects/skills`

**Likely files:** `apps/skills-mcp/go.mod`, `go.sum`, `internal/server/server.go`, server tests,
`internal/web/cors.go`, auth/API helpers, and live MCP docs.

- [x] Re-read nearest AGENTS files and verify a clean worktree. Branch `feat/mcp-2026-07-28-migration` from `f8624a5d`.
- [x] Use `go get github.com/haakco/mcp-kit@<task-7-sha>` to obtain the exact pseudo-version. Never commit a local
      filesystem `replace`. Upgrade the direct SDK dependency to v1.8.0. Done: `v0.5.14-0.20260925102941-07980ed9c9da`
      and SDK `v1.8.0`; the only `replace` in `go.mod` is the pre-existing unrelated `clipperhouse/displaywidth` pin.
- [x] Restrict HTTP and stdio servers to 2026-07-28; set explicit empty capabilities and cache policy. Use `private`
      for authenticated/caller-varying results. Done in `internal/server/server.go` and `cmd/skills/mcp_server.go`:
      `SupportedProtocolVersions`, `Capabilities: &mcp.ServerCapabilities{}`, `SetCacheable: PrivateCache(DefaultCacheTTL)`.
- [x] Set HTTP `Stateless: true`; remove session/initialize assumptions from tests and helpers. Done. The helpers were
      worse than assumed: `postMCP` sent no per-request `_meta` and used a 2025-11-25 header, so the suite was
      exercising the SDK legacy compatibility path. Both request helpers now send `_meta` plus `Mcp-Protocol-Version`,
      `Mcp-Method`, and `Mcp-Name`; the session idiom is gone from 21 helpers across 9 files, and `mcpInit` now proves
      `server/discover` returns exactly `[2026-07-28]`.
- [x] Allow `Mcp-Protocol-Version`, `Mcp-Method`, and `Mcp-Name` through CORS; allow `Mcp-Param-*` only if used. Stop
      exposing `Mcp-Session-Id`. Done in `internal/web/cors.go` (`Mcp-Param-*` unused, so not added).
- [x] Update challenge expectations, metadata, live runbook, and migration notes. `internal/web/cors_test.go` was
      rewritten from the session-header assertion into three modern-wire assertions; `TestAPI_InitializeCapabilities`
      became `TestAPI_DiscoverCapabilities`, which also pins `cacheScope: private` and a positive `ttlMs`.
- [x] Run repository-defined focused/boundary suites plus official conformance. Suites: `check-go-structure.sh` clean,
      `gofmt` clean, and **full** `SKILLS_TEST_POSTGRES_URL=... go test -p 1 ./... -count=1` green. Postgres 18 was
      provisioned locally on `127.0.0.1:15432`, the port `apps/skills-mcp/mise.toml` documents. This matters: without
      a database the DB-backed `internal/server` suite skips and reports a meaningless pass. Conformance against
      skills-mcp was **not** run (see the note below the checklist).
- [ ] With owner approval, deploy and run authenticated `server/discover` plus `tools/list`. Redefine PR-02 around
      this stateless path. **Blocked on owner approval to deploy.**
- [x] Commit and push only after local proof. Record commit, CI, deployment, and hosted probe separately. Pushed
      `83ee190a` to `feat/mcp-2026-07-28-migration`. CI not yet observed for this branch.

Two environment facts worth recording, because both cost real time to rediscover:

- The repo pins `go = "1.26"` and `golangci-lint = "2.11.3"` in `mise.toml`, but `apps/skills-mcp/go.mod` declares
  `go 1.27.0` and has done since before this migration. The mise-pinned linter therefore cannot lint this module at
  all. `golangci-lint 2.14.0` works and reports 7 gosec findings (`G710` open redirect, `G124` cookie attributes) in
  files this migration does not touch, so they are pre-existing and were reported rather than folded in.
- `task` is not installed in this environment, so `just lint-go` cannot run; `golangci-lint run` and
  `tooling/scripts/check-go-structure.sh .` were run directly instead.

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

- [x] Re-read nearest AGENTS files and verify a clean worktree. Branch `feat/mcp-2026-07-28-migration` from `0e58e3c3`.
- [x] Upgrade `mcp-kit` to v0.6.0 and SDK to v1.8.0. Done with the same release candidate as skills-mcp
      (`v0.5.14-0.20260925102941-07980ed9c9da`), no local `replace`, plus Go `1.26.4` → `1.27.0`. The v0.6.0 tag is
      still gated on Task 9, so both consumers are repinned together after the tag lands.
- [x] Remove deleted `DisableLocalhostProtection`. Preserve the kit's explicit Origin policy; do not weaken
      localhost/DNS-rebinding protection to pass tests. Confirmed the field was never deleted; it was kept and now
      carries a comment explaining why, so the next reader does not repeat the plan's original mistake.
- [x] Apply modern-only options, stateless handler, explicit capabilities/cache policy, CORS headers, and
      session-free tests as in skills-mcp. Options and `Stateless: true` in `internal/mcpserver/server.go`; CORS
      allow-list extended in `internal/api/http_helpers.go`, which wraps the `/mcp` route. vorrent had no
      session-header exposure to remove and no session-threading test helpers, so its test surface needed only the
      realm assertion below.
- [x] Update live docs and PR-02-style probes; preserve archived plans as history. Kit-side
      `docs/migration/vorrent.md` gained required change #11 plus a gotcha for the challenge realm.
- [x] Run repository-defined focused/boundary suites and official conformance. `go build ./...`, `just lint-go`
      (0 issues), full `go test ./internal/api/... ./internal/config/... ./internal/discovery/... ./internal/torrent/service/... -count=1`,
      and `just test-backend-fast` all green. Conformance against vorrent was **not** run.
- [x] Commit and push after local proof. Pushed `1941c55c` to
      `feat/mcp-2026-07-28-migration`. CI not yet observed for this branch.
- [ ] With owner approval, deploy and run authenticated discover/tools-list and OAuth recovery. **Blocked on owner
      approval to deploy.**

Vorrent's build and lint gate were both broken on `main` before this migration, so the branch also repairs them:

- `go build ./...` failed on `main` because `cd2ebe6c` pinned `go-diskfs => v1.7.0` for rclone v1.73.5 and the tree had
  since moved to rclone v1.75.0, whose squashfs backend needs go-diskfs v1.9.x. Dropping the stale replace fixes it.
- Raising the `go` directive forces `go mod tidy` to resolve golangci-lint v2.11.4 → v2.14.0, because v2.11.4 cannot
  decode Go 1.27 export data. The newer analyzers then flag 8 pre-existing findings; all 8 are resolved, including a
  real path-traversal in `parseHLSRequestPath` and a pointer-formatting bug in the audit diagnostic.

One kit behavior change surfaced here that the original plan did not anticipate: the bearer challenge realm moved from
`"mcp-kit"` to `"OAuth"` in kit `v0.5.10`. vorrent pinned `v0.5.8`, so its test passed on `main` and failed on the
upgrade. `just test-backend-fast` does **not** catch it — only the full `internal/api` suite does.

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
| mcp-kit release candidate | `8101108` (migration) + `9973cf3` (CI fix) on `feat/mcp-2026-07-28-migration`; PR [#5](https://github.com/haakco/mcp-kit/pull/5) | `just quality` (build, vet, deep lint 0 issues, structural lint 0 issues, race — 13 packages), `just vulncheck` (no vulnerabilities), `just conformance` (CLI `0.2.0-alpha.11`, `--requirements 2026-07-28`, 164 passed / 0 failed scored, 0 baseline entries), negative control `-stateless=false` exits 1, `actionlint` clean, suite-selection logic verified under `/bin/dash`, `git diff --check` clean | **Green** on run [36123660998](https://github.com/haakco/mcp-kit/actions/runs/36123660998): Detect changes pass, Quality pass (4m55s), Conformance pass (1m54s), GitGuardian pass. First run failed and correctly skipped the suites; root cause was `set -o pipefail` under dash, fixed in `9973cf3`. | n/a |
| skills-mcp candidate | `83ee190a` on `feat/mcp-2026-07-28-migration` (base `f8624a5d`) | `go build ./...`, `go vet ./...`, `gofmt` clean, `tooling/scripts/check-go-structure.sh apps/skills-mcp` clean, and **full** `SKILLS_TEST_POSTGRES_URL='postgres://skills:skills@127.0.0.1:15432/skills?sslmode=disable' go test -p 1 ./... -count=1` green (Postgres 18 provided locally; `internal/server` alone 62.8s). Helper rewrite proved by the suite itself: `postMCP` now sends real 2026-07-28 `_meta` + `Mcp-Method`/`Mcp-Name` headers, so a wrong header or missing `_meta` fails as `-32020` instead of passing silently. `TestAPI_DiscoverCapabilities` pins `supportedVersions == [2026-07-28]`, `cacheScope: private`, positive `ttlMs`. | Not yet observed for this branch. | Pending owner approval to deploy. |
| vorrent candidate | `1941c55c` on `feat/mcp-2026-07-28-migration` (base `0e58e3c3`) | `go build ./...` clean (was **broken** on `main`), `just lint-go` `0 issues.` (was 8), `go test ./internal/api/... ./internal/config/... ./internal/discovery/... ./internal/torrent/service/... -count=1` green, `just test-backend-fast` green, new `TestParseHLSRequestPath` covers sibling-directory traversal. `main`'s breakage independently confirmed pre-existing by reproducing it in a worktree of `main` under both Go 1.26.8 and Go 1.27.1. | Not yet observed for this branch. | Pending owner approval to deploy. |
| kit migration-doc follow-up | `95e4962` on `feat/mcp-2026-07-28-migration` | Doc-only: adds the `realm="mcp-kit"` → `"OAuth"` change to `docs/migration/vorrent.md` as required change #11 and a gotcha, after that change broke a vorrent test. | Pending. | n/a |
| mcp-kit v0.6.0 | — | — | — | Not tagged. Gated on owner approval after the skills-mcp deployment proves the same commit (Task 9). |
| skills-mcp v0.6.0 | — | — | — | Not repinned. Pending the tag and the deployment probe. |
| vorrent v0.6.0 | — | — | — | Not repinned. Pending the tag and the deployment probe. |

Dependabot: all 7 open alerts on `main` (2 high, 1 moderate, 4 low) are addressed on the branch — `grpc`
v1.82.1 → v1.83.1 and `otel` v1.43.0 → v1.46.0. GitHub re-evaluates the default branch only after the change lands,
so confirmation is pending merge.

Dependency versions resolved for the candidate: Go `1.27.1`, MCP Go SDK `v1.8.0`,
`github.com/go-jose/go-jose/v4 v4.1.5`, `golangci-lint v2.14.0`, `google.golang.org/grpc v1.83.1`,
`go.opentelemetry.io/otel v1.46.0`, `golang.org/x/crypto v0.57.0`, `golang.org/x/oauth2 v0.37.0`,
`golang.org/x/sync v0.23.0`, `govulncheck v1.8.0`, conformance CLI `0.2.0-alpha.11`.
`entgo.io/ent` and `github.com/ory/fosite` were already at their latest releases.

Note for local setup: editing `mise.toml` makes mise treat the config as untrusted until `mise trust` is run once.

Record blockers with the exact failed command or external state. A local pass does not substitute for CI, tag,
dependency resolution, deployment, or hosted acceptance.
