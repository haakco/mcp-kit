# MCP Lessons Learned

**Last verified:** 2026-05-01

This file collects reusable MCP implementation and QA lessons from HaakCo server cycles. Add new lessons when a failure mode would otherwise be rediscovered.

## Lesson ID Prefixes

| Prefix | Category |
|---|---|
| `FP` | False-positive disproofs |
| `PR` | Reusable probes |
| `OG` | OAuth gotchas |
| `JR` | JSON-RPC envelope traps |
| `TQ` | Transport quirks |
| `TG` | Tooling gotchas |
| `AG` | Authz gotchas |
| `CG` | Code-quality gotchas (project guardrails) |

## False Positives

### FP-01 - Rebuild before blaming SDK serialization

Live captures can come from stale binaries. If a protocol response appears to violate JSON encoding rules, rebuild the server from `HEAD` and re-capture before filing an SDK bug.

Expected confirmation:

- Fresh binary still reproduces the raw response bytes.
- A small standalone SDK reproduction behaves the same way, or the difference is understood.

## Reusable Probes

### PR-01 - Anonymous request is rejected

Purpose: catch accidental auth bypass regressions.

```bash
curl -i -X POST "$MCP_BASE_URL/mcp" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}'
```

Expected: HTTP `401` with a bearer challenge when auth is enabled. On MCP 2026-07-28 the first request a client makes
is `server/discover`; there is no `initialize` to probe with.

### PR-02 - Authenticated discovery and tools/list

Purpose: prove protocol, auth, and registry are all working together on the 2026-07-28 wire.

Setup: a valid bearer token from the OAuth token endpoint.

```bash
# 1. Discovery. No initialize, no notifications/initialized, no session.
curl -fsSi -X POST "$MCP_BASE_URL/mcp" \
  -H "Origin: $MCP_ORIGIN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Mcp-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: server/discover' \
  -d '{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}'

# 2. The real request. Each POST is self-contained.
curl -fsS -X POST "$MCP_BASE_URL/mcp" \
  -H "Origin: $MCP_ORIGIN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Mcp-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/list' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}'
```

Assert on the discover response:

- `supportedVersions` is exactly `["2026-07-28"]`.
- No `Mcp-Session-Id` response header appears.
- `resultType` is `complete`, and `ttlMs` / `cacheScope` are present.

`tools/list` must return the consumer's documented tool inventory.

Named methods additionally require `Mcp-Name` (`resources/read`, `prompts/get`, `tools/call`); omitting it returns
`-32020`.

### PR-03 - Origin allowlist enforcement

Purpose: catch browser-origin regressions.

Run the same authenticated `server/discover` request once with an allowed `Origin` and once with a disallowed
`Origin`.

Expected:

- Allowed origin returns a protocol response.
- Disallowed origin returns `403`.

### PR-04 - Dynamic client registration round trip

Purpose: verify compatibility with clients that register OAuth clients dynamically.

```bash
curl -fsS -X POST "$MCP_BASE_URL/mcp-oauth/register" \
  -H 'Content-Type: application/json' \
  -d '{"client_name":"manual-test","redirect_uris":["http://localhost:9999/cb"],"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"none"}' | jq .
```

Expected: JSON with `client_id` and `client_id_issued_at`; no client secret for public PKCE clients.

## OAuth Gotchas

### OG-01 - Protected-resource metadata lists issuer URLs

`/.well-known/oauth-protected-resource` must return authorization server issuer URLs, not metadata endpoint URLs.

### OG-02 - Dynamic registration is required for Claude Code-style clients

MCP clients may register dynamically before authorization. Missing or non-spec registration responses can make the client appear to hang.

### OG-03 - PKCE public clients must not send `client_secret`

For `token_endpoint_auth_method: none`, token requests include `code_verifier` and omit `client_secret`.

### OG-04 - PKCE base64url uses substitution, not stripping

Use URL-safe substitution for `+` and `/`, then remove padding.

```bash
CODE_VERIFIER=$(openssl rand 96 | openssl base64 -A | tr '+/' '-_' | tr -d '=' | cut -c1-128)
CODE_CHALLENGE=$(echo -n "$CODE_VERIFIER" | openssl dgst -sha256 -binary | openssl base64 -A | tr '+/' '-_' | tr -d '=')
```

Deleting `+` or `/` silently changes the verifier bytes and can cause confusing `invalid_grant` failures.

### OG-05 - Fosite enforces meaningful `state`

Authorization requests should include a state value at least eight characters long. Missing or short state can fail before scope validation runs.

### OG-06 - Bearer challenges should include OAuth recovery hints

MCP clients recover best when every `/mcp` 401 challenge includes `resource_metadata`, OAuth `error` / `error_description`, and the required scope list. Do not only include these hints on no-token requests; stale or invalid bearer tokens need the same hint so clients can prompt for the correct login flow instead of surfacing a generic auth or transport failure.

### OG-07 - CLI-friendly defaults need refresh rotation plus a useful access TTL

Short access tokens are correct for OAuth-backed Streamable HTTP MCP because some clients do not refresh/retry when the server returns `401 invalid_token`; they only refresh from locally stored expiry metadata. Use a 1-hour access-token default to reduce stale-token windows, and rely on 30-day rotating refresh tokens for longer sessions. Keep both lifetimes overrideable per consumer.

### OG-08 - Resource metadata URLs must be derived from the resource URL

For a protected resource at `/mcp`, the canonical RFC 9728 metadata URL is `/.well-known/oauth-protected-resource/mcp`, not only the root `/.well-known/oauth-protected-resource`. Derive `resource_metadata` from the MCP resource URL and use the same value in 401 challenges and authorization-server metadata. Keep the root metadata route as a compatibility alias, but do not advertise a different URL than the path-specific route clients are expected to discover.

### OG-09 - Refresh-token storage needs refresh-token expiry

When storing Fosite sessions, persist the expiry that matches the session type. Refresh-token rows should use `fosite.RefreshToken`, not `fosite.AccessToken`, or stores that enforce row expiry can delete refresh sessions when the short access token expires. This makes "lower the access-token TTL so clients refresh sooner" backfire by removing the refresh path.

### OG-10 - Rotated refresh-token reuse needs correlation, not token logging

Clients can continue presenting a rotated refresh token long after a successful
exchange. Emit a truncated hash fingerprint for the presented token on rotation,
cached replay, and failed refresh events so consumers can reconstruct the full
timeline without storing token values. Audit every request, including followers
coalesced behind `singleflight`; otherwise concurrent stale-token use is silently
undercounted.

### OG-11 - Client ID Metadata Documents are a server-side fetch of caller-supplied URLs

MCP 2026-07-28 prefers Client ID Metadata Documents (SEP-991): the client's `client_id` is an HTTPS URL, and the
authorization server fetches that URL to learn the client's metadata. The fetch is the whole risk. A working
implementation needs, at minimum:

- an exact match between the document's `client_id` and the URL it was served from, or one host can serve metadata for
  another's identity;
- loopback, private, link-local, multicast, and unspecified targets refused **after** DNS resolution, with the resolved
  address dialled rather than re-resolved;
- bounded time, response size, and redirect count, each redirect re-validated;
- no credentials or fragment in the URL, and a non-root path;
- a scope claim intersected with what the server serves, so a document cannot widen its own access.

Correct alternatives include a single well-tested implementation reused by every server in the fleet, or a validated
third-party client. A hand-rolled fetch that skips the resolution check is an SSRF hole reachable by anyone who can
start an authorization flow.

`oauth.ClientIDMetadataFetcher` is the kit's implementation, and `oauth/cimd_test.go` covers each bound above.

### OG-12 - Authorization responses need `iss`

RFC 9207 adds `iss` to the authorization response so a client can confirm the response came from the issuer it started
the flow with before redeeming the code. Fosite does not emit it, so the kit adds it explicitly in both authorize
handlers. `cliauth` rejects a mismatched `iss` before token exchange, and accepts a missing one for compatibility with
issuers that predate the RFC.

### OG-13 - Bearer challenge auth-params must be quoted

`WWW-Authenticate` auth-params are quoted strings, and clients split them on commas. A `resource_metadata` URL
containing a comma, quote, or backslash breaks the header structure and can truncate or forge later parameters. Every
interpolated value goes through one quoting helper; the earlier code quoted the scope hint but not the metadata URL.

## JSON-RPC Envelope Traps

### JR-01 - Echo request IDs exactly

Clients correlate responses by `id`. The response must echo the request `id` exactly.

### JR-02 - Notifications do not get responses

Requests without `id` are notifications. Returning a response to a notification can confuse clients.

### JR-03 - Error codes must be canonical

Use JSON-RPC reserved codes for parse, invalid request, method, params, and internal errors. Use `-32000` to `-32099` for server-defined errors.

## Transport Quirks

### TQ-01 - Streamable HTTP responses may be SSE-framed

Clients must send:

```text
Accept: application/json, text/event-stream
```

When the response is SSE, parse the `data:` line before decoding JSON.

### TQ-02 - MCP 2026-07-28 has no handshake

The legacy sequence was `initialize` → `notifications/initialized` → first real request, all sharing an
`Mcp-Session-Id`. Under 2026-07-28 (SEP-2575) that is gone:

1. `server/discover` — one POST, no session, negotiated inline.
2. Any request, each self-contained.

Every request carries `_meta["io.modelcontextprotocol/protocolVersion"]` and
`_meta["io.modelcontextprotocol/clientCapabilities"]`, and HTTP requests mirror the version in
`Mcp-Protocol-Version` and the method in `Mcp-Method`. `tools/call`, `resources/read`, and `prompts/get` also need
`Mcp-Name`. A body/header disagreement is `-32020`.

Streamable HTTP must be served with `StreamableHTTPOptions{Stateless: true}`. A non-stateless handler rejects a
2026-07-28 request outright. In stateless mode GET and DELETE return `405` with `Allow: POST`, and `Mcp-Session-Id` is
neither required nor emitted.

`ping` is removed: the server answers `-32601`. Do not enable `ServerOptions.KeepAlive`.

### TQ-04 - Unsupported *legacy* protocol versions get plain text, not `-32022`

For a requested version **at or after** 2026-07-28 the SDK returns a proper JSON-RPC error:

```json
{"error":{"code":-32022,"message":"unsupported protocol version","data":{"supported":["2026-07-28"],"requested":"2027-01-01"}}}
```

For a requested version **before** 2026-07-28 the rejection happens during HTTP transport setup, before the JSON-RPC
layer, and the body is `text/plain`:

```text
Bad Request: Unsupported protocol version (supported versions: 2026-07-28)
```

A modern-only server therefore answers a stale legacy client with a body that client cannot parse, so it cannot learn
the supported versions and retry. `testkit/contract_test.go`
(`TestLegacyProtocolVersionHeaderGetsPlainTextRejection`) pins this so a fix is visible. Closing it means teaching
`mcpmw.Envelope` about supported revisions, which changes that middleware's public signature; treat it as its own
change rather than smuggling it into a migration.

### TQ-05 - `-32021` has no server-side producer in the Go SDK

`mcp.CodeMissingRequiredClientCapabilities` (`-32021`) and `MissingRequiredClientCapabilityData` exist, and the
conformance suite defines the behaviour, but the SDK never returns that code on its own: it is reached only through a
reference-server diagnostic tool. Do not claim `-32021` coverage from SDK behaviour alone.

### TQ-06 - The SDK now emits JSON-RPC for errors the kit's envelope used to rewrite

Against SDK v1.8.0 the cases `mcpmw.Envelope` was written for arrive as proper JSON-RPC, not plain text:

| Request | Response |
|---|---|
| malformed JSON body | `200` + `-32700` |
| missing `id` | `200` + `-32600` |
| unknown method | `404` + `-32601` (SEP-2575 requires 404) |

`Envelope` only rewrites `400` + `text/plain`, so those rules are unreachable for SDK-backed handlers and the
middleware is a no-op there. Keep it for non-SDK handlers and for the plain-text rejections that remain (an empty POST
body, and legacy protocol versions per TQ-04), and do not assume it is doing work it is not.

### TQ-07 - HTTP status is part of the contract, not decoration

SEP-2575 pins statuses clients depend on: `404` for unknown or removed methods, and `400` for `-32020`, `-32021`,
`-32022`, and `-32602`. Middleware that normalises error bodies must not also rewrite the status, or a client cannot
tell a protocol rejection from a transport failure. `testkit/contract_test.go` asserts both.

### TQ-03 - Rebuild before filing binary-behavior bugs

If live behavior contradicts tests or standalone probes, rebuild the server and retry before investigating dependencies.

## Tooling Gotchas

### TG-01 - Curl probes should include the same headers as real clients

For transport probes, include `Origin`, `Content-Type`, `Accept`, `Authorization`, and `Mcp-Session-Id` where applicable. Missing headers can test a different path than a real client uses.

## Engineering Gaps

### EG-01 - Checklist coverage should be testable

Every registered tool, resource, and prompt should have a corresponding checklist row in the active MCP cycle docs. Add a coverage test that fails when a registered surface has no documented probe.

### EG-02 - CI needs a live HTTP smoke

Unit tests can pass while real Streamable HTTP behavior fails. CI should include a small live smoke that boots the server, mints or injects a test token, calls `tools/list`, and asserts the protocol shape.

### EG-03 - Inspector smoke should be automated

Manual Inspector checks are easy to defer. Add a fixture-backed Inspector run or equivalent schema-render smoke so schema warnings are caught before release.

### EG-04 - Server instructions must reference real surfaces

If server instructions name tools, resources, or prompts, test that each named surface exists in the registered set. Stale instructions are a client-facing bug.

### EG-05 - Scope mapping should live in code

Each surface's required scopes should be declared in a registry that tests can inspect. For each protected surface, verify a token missing the required scope receives `insufficient_scope`.

### EG-06 - Every tool needs dispatch coverage

Each tool should have at least one dispatch-level test that proves request decoding, handler execution, and response encoding work together.

## Authz Gotchas

### AG-01 - Cross-surface admin gates need to live in the service layer, not the handler

When a privileged mutation (e.g. changing a skill's visibility, archiving a workspace, rotating a signing key) is exposed via **both** the HTTP API (cookie/session auth) and the MCP tool path (bearer-token auth), the admin gate must be enforced at every surface — or, better, pushed into the service mutator so neither caller has to remember.

Surfaced in skills-mcp 2026-05-02: `PUT /api/v1/skills/{id}` correctly required admin to flip `visibility`, but the matching `update_skill` MCP tool did not, so any bearer-authed caller could promote a workspace skill to public. The fix added an `AuthzService.IsAdmin(ctx, userID)` probe and gated both surfaces — but the systemic answer is to move the check into `service.UpdateSkill` so the rule cannot be missed when a future surface is added.

Probe to add to your contract suite:

- For each privileged field (visibility, role, scope, etc.), exercise the mutation via every public surface with a non-admin token. Each call must reject with a clear error.
- Bonus: register the gated fields in a single registry and have a test iterate it; new fields auto-wire into the suite.

### AG-02 - List/Count parity for visibility-filtered endpoints

If you ship a `ListVisibleX` method, also ship a `CountVisibleX` (and use it for pagination totals). Calling a vanilla `CountX` from a handler that returned visibility-filtered rows leaks the existence of private records via the `total` field, even if the rows themselves are filtered. Easy to miss because the list output looks correct.

### AG-03 - "No scopes in context" must mean default-deny, not allow

When auth is opt-in (issuer URL + public URL drive whether OAuth is configured), it's tempting to write `if scopes == nil { allow }` as the dev-mode shortcut in service-layer authz. **Don't.** That branch fires in two situations: (a) auth is genuinely off at startup; (b) auth IS configured but a code path reached the service without going through bearer middleware. Case (a) is fine; case (b) is a privilege-escalation channel that's invisible until it bites.

Fix: add an explicit `WithAuthDisabled(ctx)` sentinel that bearer middleware sets only when `Introspector == nil && TokenValidator == nil`. Service-layer code reads that sentinel — never `scopes == nil` directly — so a forgotten middleware wrapper defaults to anonymous (only public records visible) instead of full passthrough.

Surfaced in skills-mcp 2026-05-02: three sites in `service/skill_visibility.go` and `service/authz_scope.go::Authorize` were using `auth.GetScopes(ctx) == nil` as the dev-mode shortcut. Replacing with `auth.IsAuthDisabled(ctx)` closed the bypass for any caller that fails to set scopes (e.g. cookie-session API paths, internal callers, test fixtures). The pre-existing `RequireScope` middleware already used the right sentinel — service layer just hadn't matched it.

Probe to add to your contract suite:

- For every public read/write entry point on the service layer, exercise it with `context.Background()` (no scopes, no auth-disabled). The expected outcome is "anonymous" (UserID=0) — only public/global records visible — not "everything".
- Then exercise it with `WithAuthDisabled(ctx)` to confirm the dev-mode passthrough still works.

### AG-04 - Auth scopes belong on context, not in the actor struct

When you need to make decisions like "is this caller authenticated at all?" or "what scopes does this token grant?", read them from `context.Context` via typed helpers (`auth.GetScopes(ctx)`, `auth.IsAuthDisabled(ctx)`) — not from a side-channel `Actor` parameter. Reasons:

- The middleware that establishes auth state writes to context once; downstream callers read it without extra plumbing.
- A handler that builds an Actor from a session can forget to populate scopes; an Actor-only model has no defense.
- Tests can construct contexts with exactly the scope state under test (`context.WithValue(ctx, ContextKeyScopes, ...)`) and have them flow through unchanged.

Surfaced in skills-mcp 2026-05-02: the visibility filter took both `ctx` and `actor` and was tempted to use `actor.UserID == 0` as the dev-mode signal. That conflated "anonymous user" with "auth not configured". Splitting them — auth state on context, identity on the actor — kept each concern in its own channel.

## Code-Quality Gotchas

### CG-01 - Silent errors need a *concrete* reason, not "best-effort"

Every `_ = funcCall(...)` site in production code should have `// nolint:errcheck // <reason>` where the reason cites either a stdlib invariant ("hash.Hash.Write never returns an error"), a documented intent ("best-effort revoke during logout-all — caller already aborted"), or a structural justification ("response already partial; can't recover"). "Best-effort" alone is not enough — it tends to cover for genuine handling gaps.

Lint rule: `errcheck` with a fail-loud CI gate. Pair with `revive` to flag bare `_ = ...` without an annotation comment within ±1 line.

### CG-02 - Methods-per-receiver caps catch god-classes early

Set a cap of ~12 methods per receiver and fail-loud in CI. The cap doesn't have to be perfect; what matters is the conversation it forces: when a 13th method wants to be added, the right move is usually a separate handler/embedder type (e.g. `*skillsWriteHandler` in skills-mcp) rather than padding `*Server`. Without the cap, server types reliably grow into 30-method god classes.

Surfaced in skills-mcp 2026-05-02: a planned `Server.callerIsAdmin` helper would have pushed `*Server` to 13 methods. Moving it to `*skillsWriteHandler` (which embeds `*Server`) kept the cap intact and put the logic next to the only caller. This is the structural answer the cap was meant to surface.

### CG-03 - When fixing pre-existing failures, keep them in separate commits

If your feature work uncovers test files that were already broken (e.g. a missing `ConfigureKitServer` call from an upstream rename), fix them — that's the ownership rule. But land the fix in its own commit before the feature commit, not bundled inside it. Otherwise reviewers conflate the two scopes and `git revert` of the feature also reverts the unrelated test fix.

Surfaced in skills-mcp 2026-04-30: `internal/server/api_test.go`, `auth_flow_test.go`, and `oidc/oauth_flow_test.go` test fixes were bundled into Task 1.8's search commit. The reviewer flagged the non-atomic boundary and tracked it as a process note for future PRs.
