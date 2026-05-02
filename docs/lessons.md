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

### PR-01 - Anonymous initialize is rejected

Purpose: catch accidental auth bypass regressions.

```bash
curl -i -X POST "$MCP_BASE_URL/mcp" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize"}'
```

Expected: HTTP `401` with a bearer challenge when auth is enabled.

### PR-02 - Authenticated handshake to tools/list

Purpose: prove protocol, auth, and registry are all working together.

Setup: a valid bearer token from the OAuth token endpoint.

```bash
curl -fsSi -X POST "$MCP_BASE_URL/mcp" \
  -H "Origin: $MCP_ORIGIN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"manual-test","version":"0"}}}' \
  > /tmp/mcp-init.txt

SESSION=$(grep -i '^mcp-session-id:' /tmp/mcp-init.txt | awk '{print $2}' | tr -d '\r')

curl -fsS -X POST "$MCP_BASE_URL/mcp" \
  -H "Origin: $MCP_ORIGIN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $TOKEN" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'

curl -fsS -X POST "$MCP_BASE_URL/mcp" \
  -H "Origin: $MCP_ORIGIN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $TOKEN" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
```

Expected: `tools/list` returns the consumer's documented tool inventory.

### PR-03 - Origin allowlist enforcement

Purpose: catch browser-origin regressions.

Run the same authenticated `initialize` request once with an allowed `Origin` and once with a disallowed `Origin`.

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

### TQ-02 - The MCP handshake is three steps

The canonical sequence is:

1. `initialize`
2. `notifications/initialized`
3. First real request, such as `tools/list`

Use the same `Mcp-Session-Id` for steps 2 and 3.

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
