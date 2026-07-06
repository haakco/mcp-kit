# AOA Security and Conformance Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Status:** Draft. Created 2026-07-06.

**Goal:** Borrow the useful security and conformance patterns from `github.com/0ndreu/aoa` and `github.com/0ndreu/aoa-conformance` without turning `mcp-kit` into an `aoa` wrapper or violating the kit/consumer boundary.

**Background:** `aoa` is a pre-1.0 Go package focused on MCP OAuth protected-resource behavior: RFC 9728 metadata, bearer-token challenge/validation, JWKS lookup, JWT hardening, DPoP, and token exchange. `mcp-kit` has a broader scope: it owns the OAuth issuer, Fosite-backed storage, key rotation, PAT support, discovery, JSON-RPC envelope middleware, Origin middleware, CLI auth, Ent mixins, and consumer testkit. The useful path is to copy the security probes, metadata validation rules, and conformance workflow, not to adopt `aoa` wholesale.

**Architecture:** Keep `mcp-kit` as the OAuth issuer plus resource-server middleware. Add small, focused helpers for RFC 9728 metadata path/validation and conformance execution; strengthen existing bearer/JWKS tests around the failure modes `aoa` calls out; document DPoP and token-exchange as future optional features unless a consumer needs them. `aoa-conformance` should be integrated as an external black-box smoke once its upstream CLI builds cleanly or via a pinned fork.

**Tech Stack:** Go 1.26.2, stdlib `net/http`, Ory Fosite, go-jose/v3, official MCP Go SDK, existing `just` recipes, `curl`, `jq`, optional external `github.com/0ndreu/aoa-conformance/cmd/aoa-conform`.

**Parallel Work Model:** Tasks 1 and 2 are independent and can run in parallel. Task 3 depends on Task 1. Task 4 depends on Task 2. Task 5 depends on Tasks 1-4. Task 6 is docs-only and can run after the behavior decisions in Tasks 1-4 are complete.

---

## Current State (Verified 2026-07-06)

**Local `mcp-kit` files opened:**

- `DESIGN.md` confirms the kit owns OAuth, middleware, key rotation, discovery, CLI auth, Ent mixins, and testkit, but does not own domain tools/resources/prompts, user tables, RBAC, or audit storage.
- `docs/lessons.md` already records OAuth/resource-server probes such as PR-01 through PR-04, OG-06 bearer recovery hints, and OG-07 token TTL behavior.
- `oidc/discovery.go` currently serves OAuth/OIDC metadata and protected-resource metadata at:
  - `/.well-known/openid-configuration`
  - `/.well-known/oauth-authorization-server`
  - `/.well-known/oauth-protected-resource`
  - `/.well-known/oauth-protected-resource/mcp`
  - `/.well-known/jwks.json`
- `oauth/metadata.go` has separate metadata handlers with `Cache-Control: no-store`, but less complete RFC 9728 validation/field coverage than `aoa`.
- `oauth/middleware.go` already emits `WWW-Authenticate` with `resource_metadata`, `error`, `error_description`, and `scope` hints for missing/invalid tokens.
- `_examples/minimal-server/main.go` exposes an auth-enabled demo server with OAuth metadata, JWKS, DCR, and `/mcp`.
- `justfile` has build/test/vet/lint/quality targets, but no conformance target.

**External `aoa` review notes:**

- `aoa` commit reviewed: `80ec8c6` dated 2026-06-15.
- Useful source files:
  - `metadata.go` for strict RFC 9728 validation, path rules, extension-field merge, and localhost dev exception.
  - `bearer.go` / `bearer_errors.go` for challenge construction and DPoP mode boundaries.
  - `jwks.go` / `verify.go` for JWKS cache behavior and alg-confusion defense.
  - `dpop.go` and related tests for future sender-constrained-token design.
- Do not copy:
  - `aoa`'s complete bearer middleware as a replacement for `oauth.Bearer`; `mcp-kit` already uses Fosite introspection plus PAT validation.
  - Token-exchange implementation until a concrete gateway/downstream delegation use case exists.
  - DPoP enforcement until a consumer commits to DPoP-bound access tokens and shared replay storage.

**External `aoa-conformance` review notes:**

- `aoa-conformance` commit reviewed: `cf6e357` dated 2026-06-14.
- Current upstream `@latest` does not compile because `conformance/checks_rfc8414.go` calls missing `probe.VerifyJWTWithJWKS`.
- A temporary local patch in `/tmp/aoa-conformance-review` restored that helper only to run discovery checks. Do not rely on that patch as repo state.
- Against local Skills deployment at `https://skill.dev.haak.co:8443/mcp`, patched conformance results were:
  - Core profile: 15 pass, 7 skip, 0 fail.
  - Full profile: 15 pass, 24 skip, 0 fail.
- Skips were expected for `signed_metadata`, token-presenting smoke, PKCE negative flow, resource-indicator token checks, DPoP, token exchange, introspection, revocation, and RFC 9207 auth-code callback checks.

## Scope Decisions

### Borrow Now

- Conformance smoke workflow against `_examples/minimal-server` and at least one real downstream service.
- RFC 9728 protected-resource metadata validation and path derivation tests.
- Bearer challenge formatting tests modeled on `aoa-conformance` checks.
- JWT/JWKS hardening test coverage where `mcp-kit` validates signed tokens or exposes JWKS.
- Lessons/doc updates explaining which `aoa` features are intentionally deferred.

### Defer

- DPoP runtime support.
- RFC 8693 token exchange.
- Full remote JWKS JWT verifier for third-party issuers unless a consumer needs resource-server-only mode.
- mTLS-bound access-token support.

### Do Not Do

- Do not add `github.com/0ndreu/aoa` as a production dependency.
- Do not replace Fosite introspection with a separate JWT verifier for the kit-issued OAuth path.
- Do not move user/session/login UI, RBAC, domain tools, or audit storage into the kit.

---

## Task 1: Add AOA-Inspired RFC 9728 Metadata Validation

**Files:**

- Create: `oidc/metadata_path.go`
- Create: `oidc/metadata_path_test.go`
- Modify: `oidc/discovery.go`
- Modify: `oidc/discovery_test.go`
- Modify: `oauth/metadata.go`
- Modify: `oauth/metadata_test.go`
- Modify: `docs/lessons.md`

- [ ] **Step 1: Write failing tests for pathful protected-resource metadata**

Add tests proving pathful MCP resource URLs get the correct RFC 9728 well-known path.

```go
func TestProtectedResourceMetadataPathFor(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		want     string
	}{
		{
			name:     "origin only",
			resource: "https://mcp.example.com",
			want:     "/.well-known/oauth-protected-resource",
		},
		{
			name:     "mcp path",
			resource: "https://mcp.example.com/mcp",
			want:     "/.well-known/oauth-protected-resource/mcp",
		},
		{
			name:     "nested path",
			resource: "https://mcp.example.com/api/mcp",
			want:     "/.well-known/oauth-protected-resource/api/mcp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProtectedResourceMetadataPathFor(tt.resource)
			if err != nil {
				t.Fatalf("ProtectedResourceMetadataPathFor() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("path = %q, want %q", got, tt.want)
			}
		})
	}
}
```

Run:

```bash
go test ./oidc -run TestProtectedResourceMetadataPathFor -count=1
```

Expected: FAIL because `ProtectedResourceMetadataPathFor` does not exist.

- [ ] **Step 2: Implement `ProtectedResourceMetadataPathFor`**

Create `oidc/metadata_path.go` with a small stdlib-only helper.

```go
package oidc

import (
	"errors"
	"net/url"
	"strings"
)

const protectedResourceWellKnownPath = "/.well-known/oauth-protected-resource"

// ProtectedResourceMetadataPathFor returns the RFC 9728 well-known path for a resource URL.
func ProtectedResourceMetadataPathFor(resourceURL string) (string, error) {
	u, err := url.Parse(resourceURL)
	if err != nil || !u.IsAbs() {
		return "", errors.New("resource URL must be absolute")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("resource URL must not contain query or fragment")
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	if path == "" {
		return protectedResourceWellKnownPath, nil
	}
	return protectedResourceWellKnownPath + path, nil
}
```

Run:

```bash
go test ./oidc -run TestProtectedResourceMetadataPathFor -count=1
```

Expected: PASS.

- [ ] **Step 3: Add validation tests for protected-resource metadata**

Add tests covering absolute URL, HTTPS requirement, no fragments, and localhost dev exception.

```go
func TestProtectedResourceMetadataValidate(t *testing.T) {
	tests := []struct {
		name    string
		meta    ProtectedResourceMetadata
		opts    MetadataValidateOptions
		wantErr bool
	}{
		{
			name: "https resource valid",
			meta: ProtectedResourceMetadata{
				Resource:             "https://mcp.example.com/mcp",
				AuthorizationServers: []string{"https://idp.example.com"},
			},
		},
		{
			name: "http non-local rejected",
			meta: ProtectedResourceMetadata{Resource: "http://mcp.example.com/mcp"},
			wantErr: true,
		},
		{
			name: "localhost http allowed for dev",
			meta: ProtectedResourceMetadata{Resource: "http://localhost:8080/mcp"},
			opts: MetadataValidateOptions{AllowInsecureLocalhost: true},
		},
		{
			name: "fragment rejected",
			meta: ProtectedResourceMetadata{Resource: "https://mcp.example.com/mcp#frag"},
			wantErr: true,
		},
		{
			name: "authorization server must be https",
			meta: ProtectedResourceMetadata{
				Resource:             "https://mcp.example.com/mcp",
				AuthorizationServers: []string{"http://idp.example.com"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.meta.ValidateWithOptions(tt.opts)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
```

Run:

```bash
go test ./oidc -run TestProtectedResourceMetadataValidate -count=1
```

Expected: FAIL because validation methods/options do not exist.

- [ ] **Step 4: Implement metadata validation without widening package responsibility**

Add `MetadataValidateOptions`, `Validate`, and `ValidateWithOptions` methods to the existing `oidc.ProtectedResourceMetadata`. Keep this in `oidc`; do not introduce an `aoa` dependency.

The implementation must:

- Require `Resource` to be an absolute URI.
- Reject query and fragment on `Resource`.
- Require HTTPS except `http://localhost`, `http://127.0.0.1`, and `http://[::1]` when `AllowInsecureLocalhost` is true.
- Require each `AuthorizationServers` value to be absolute HTTPS with no query or fragment.

Run:

```bash
go test ./oidc -run 'TestProtectedResourceMetadata(PathFor|Validate)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Use the path helper in route registration**

Update `DiscoveryConfig.RegisterRoutes` so it mounts the path returned by `ProtectedResourceMetadataPathFor(resourceURL)` in addition to the legacy `/.well-known/oauth-protected-resource` path when they differ.

Verification test:

```go
func TestRegisterRoutesMountsPathfulProtectedResourceMetadata(t *testing.T) {
	mux := http.NewServeMux()
	discovery := NewDiscoveryConfig("https://issuer.example.com", []string{"mcp.read"})
	discovery.RegisterRoutes(mux, RouteConfig{ResourceURL: "https://issuer.example.com/api/mcp"})

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/api/mcp", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", res.Code, res.Body.String())
	}
}
```

Run:

```bash
go test ./oidc -run TestRegisterRoutesMountsPathfulProtectedResourceMetadata -count=1
```

Expected: PASS.

- [ ] **Step 6: Update lessons**

Add a lesson to `docs/lessons.md`:

````markdown
### OG-08 - Protected-resource metadata path follows the resource path

For a resource URL with a path, RFC 9728 clients may fetch
`/.well-known/oauth-protected-resource/<resource-path>`. Mount both the root
well-known path and the pathful variant when the MCP endpoint is not at the
origin root. Keep the resource URI query- and fragment-free.
````

Run:

```bash
go test ./oidc ./oauth -count=1
```

Expected: PASS.

Commit:

```bash
git add oidc/metadata_path.go oidc/metadata_path_test.go oidc/discovery.go oidc/discovery_test.go oauth/metadata.go oauth/metadata_test.go docs/lessons.md
git commit -m "feat(oidc): validate protected resource metadata paths"
```

---

## Task 2: Add AOA-Conformance Smoke Target

**Files:**

- Modify: `justfile`
- Create: `tooling/aoa-conformance/README.md`
- Create: `tooling/aoa-conformance/run.sh`
- Modify: `_examples/minimal-server/main_test.go`
- Modify: `README.md`
- Modify: `docs/lessons.md`

- [ ] **Step 1: Add a script that refuses the broken upstream build clearly**

Create `tooling/aoa-conformance/run.sh`.

```bash
#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-http://127.0.0.1:18080/mcp}"
PROFILE="${AOA_CONFORMANCE_PROFILE:-core}"
FORMAT="${AOA_CONFORMANCE_FORMAT:-json}"
PKG="${AOA_CONFORMANCE_PKG:-github.com/0ndreu/aoa-conformance/cmd/aoa-conform@latest}"

tmp_report="$(mktemp "${TMPDIR:-/tmp}/mcp-kit-aoa-conformance.XXXXXX.json")"
trap 'rm -f "$tmp_report"' EXIT

if ! go run "$PKG" \
  --target "$TARGET" \
  --profile "$PROFILE" \
  --format "$FORMAT" \
  >"$tmp_report"; then
  echo "aoa-conformance failed or did not compile for package: $PKG" >&2
  echo "If upstream still references missing probe.VerifyJWTWithJWKS, pin a fixed fork via AOA_CONFORMANCE_PKG." >&2
  exit 1
fi

if command -v jq >/dev/null 2>&1; then
  jq '{target, counts: ([.entries[].result.status] | group_by(.) | map({status: .[0], count: length})), failures: [.entries[] | select(.result.status=="fail" or .result.status=="error") | {id:.check.id, severity:.check.severity, rfc:.check.rfc, message:.result.message}]}' "$tmp_report"
else
  cat "$tmp_report"
fi
```

Run:

```bash
chmod +x tooling/aoa-conformance/run.sh
tooling/aoa-conformance/run.sh http://127.0.0.1:18080/mcp
```

Expected today if upstream is still broken: FAIL with a clear message naming `probe.VerifyJWTWithJWKS`.

- [ ] **Step 2: Add a local server-backed Just recipe**

Add to `justfile`:

```just
# Run AOA conformance against the minimal example server. Requires a compiling aoa-conformance package.
conformance-aoa package="github.com/0ndreu/aoa-conformance/cmd/aoa-conform@latest":
    @bash -lc 'set -euo pipefail; port="$${MCP_KIT_AOA_PORT:-18080}"; base="http://127.0.0.1:$${port}"; MCP_KIT_EXAMPLE_ADDR="127.0.0.1:$${port}" MCP_KIT_EXAMPLE_ISSUER="$${base}" go run ./_examples/minimal-server >/tmp/mcp-kit-minimal-server.log 2>&1 & pid=$$!; cleanup() { kill "$$pid" >/dev/null 2>&1 || true; }; trap cleanup EXIT; for _ in $$(seq 1 40); do if curl -fsS "$${base}/.well-known/oauth-protected-resource" >/dev/null 2>&1; then break; fi; sleep 0.25; done; AOA_CONFORMANCE_PKG="{{package}}" tooling/aoa-conformance/run.sh "$${base}/mcp"'
```

Run:

```bash
just --list | rg conformance-aoa
```

Expected: recipe listed.

- [ ] **Step 3: Document the upstream build blocker and fork escape hatch**

Create `tooling/aoa-conformance/README.md`.

````markdown
# AOA Conformance

This folder contains a thin runner for `github.com/0ndreu/aoa-conformance`.

As of 2026-07-06, upstream `@latest` does not compile because
`conformance/checks_rfc8414.go` references a missing `probe.VerifyJWTWithJWKS`.
Keep this target non-gating until upstream fixes that build or HaakCo pins a
small fork.

Run against the minimal example:

```bash
just conformance-aoa
```

Run against a fixed fork:

```bash
just conformance-aoa github.com/haakco/aoa-conformance/cmd/aoa-conform@<commit>
```

Run against a downstream deployment:

```bash
AOA_CONFORMANCE_PKG=github.com/haakco/aoa-conformance/cmd/aoa-conform@<commit> \
  tooling/aoa-conformance/run.sh https://skill.dev.haak.co:8443/mcp
```
````

Run:

```bash
sed -n '1,120p' tooling/aoa-conformance/README.md
```

Expected: file documents the upstream blocker and fork override.

- [ ] **Step 4: Add an optional CI note but keep quality gate unchanged**

Update `README.md` documentation section with:

````markdown
- `just conformance-aoa` — optional external AOA conformance smoke against `_examples/minimal-server`; non-gating until upstream `aoa-conformance` publishes a compiling version.
````

Do not add this to `just quality` yet.

Run:

```bash
go test ./_examples/minimal-server
```

Expected: PASS.

Commit:

```bash
git add justfile tooling/aoa-conformance/README.md tooling/aoa-conformance/run.sh README.md _examples/minimal-server/main_test.go docs/lessons.md
git commit -m "test: add optional aoa conformance smoke"
```

---

## Task 3: Strengthen Bearer Challenge Contract Tests

**Files:**

- Modify: `oauth/middleware_test.go`
- Modify: `oauth/middleware.go`
- Modify: `docs/lessons.md`

- [ ] **Step 1: Add exact challenge tests for no token and invalid token**

Add table-driven tests that parse the `WWW-Authenticate` header instead of only using `strings.Contains`.

```go
func TestBearerChallengeIncludesRecoveryHints(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		wantStatus    int
		wantError     string
		wantScope     string
		wantResource  string
	}{
		{
			name:         "missing token",
			wantStatus:   http.StatusUnauthorized,
			wantError:    "invalid_token",
			wantScope:    "openid mcp.read",
			wantResource: "https://mcp.example.test/.well-known/oauth-protected-resource",
		},
		{
			name:         "invalid token",
			token:        "stale-token",
			wantStatus:   http.StatusUnauthorized,
			wantError:    "invalid_token",
			wantScope:    "openid mcp.read",
			wantResource: "https://mcp.example.test/.well-known/oauth-protected-resource",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := oauth.Bearer(oauth.BearerConfig{
				Introspector:        &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{}},
				ResourceMetadataURL: tt.wantResource,
				RequiredScopes:      []string{"openid", "mcp.read"},
			})
			handler := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("handler should not be called")
			}))
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.Code, tt.wantStatus)
			}
			params := parseAuthParamsForTest(t, res.Header().Get("WWW-Authenticate"))
			if params["resource_metadata"] != tt.wantResource {
				t.Fatalf("resource_metadata = %q, want %q", params["resource_metadata"], tt.wantResource)
			}
			if params["scope"] != tt.wantScope {
				t.Fatalf("scope = %q, want %q", params["scope"], tt.wantScope)
			}
			if params["error"] != tt.wantError {
				t.Fatalf("error = %q, want %q", params["error"], tt.wantError)
			}
			if params["error_description"] == "" {
				t.Fatal("error_description is empty")
			}
		})
	}
}
```

Run:

```bash
go test ./oauth -run TestBearerChallengeIncludesRecoveryHints -count=1
```

Expected: FAIL until `parseAuthParamsForTest` exists.

- [ ] **Step 2: Add a local test parser**

Add this helper to `oauth/middleware_test.go`.

```go
func parseAuthParamsForTest(t *testing.T, header string) map[string]string {
	t.Helper()
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		t.Fatalf("WWW-Authenticate = %q, want Bearer challenge", header)
	}
	out := map[string]string{}
	for _, part := range strings.Split(strings.TrimPrefix(header, prefix), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		out[key] = strings.Trim(value, `"`)
	}
	return out
}
```

Run:

```bash
go test ./oauth -run TestBearerChallengeIncludesRecoveryHints -count=1
```

Expected: PASS.

- [ ] **Step 3: Add quote/escaping regression tests**

Add tests for scopes or metadata URLs that contain characters requiring escaping. This locks down `quoteAuthParam` behavior.

```go
func TestBearerChallengeEscapesAuthParams(t *testing.T) {
	middleware := oauth.Bearer(oauth.BearerConfig{
		Introspector:        &mockIntrospector{validTokens: map[string]*fosite.AccessRequest{}},
		ResourceMetadataURL: `https://mcp.example.test/.well-known/oauth-protected-resource?bad="quote"`,
		RequiredScopes:      []string{`mcp.read`, `custom"scope`},
	})
	handler := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not be called")
	}))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	authHeader := res.Header().Get("WWW-Authenticate")
	if strings.Contains(authHeader, `?bad="quote"`) {
		t.Fatalf("WWW-Authenticate did not escape quote: %q", authHeader)
	}
	if !strings.Contains(authHeader, `custom\"scope`) {
		t.Fatalf("WWW-Authenticate missing escaped scope: %q", authHeader)
	}
}
```

Run:

```bash
go test ./oauth -run TestBearerChallengeEscapesAuthParams -count=1
```

Expected: PASS or a focused FAIL that points at `writeBearerChallenge` escaping.

- [ ] **Step 4: Fix escaping only if the new test fails**

If needed, route all auth-param values through `quoteAuthParam` before adding them to the challenge string. Do not change status codes or body shape.

Run:

```bash
go test ./oauth ./mcpkit ./_examples/minimal-server -count=1
```

Expected: PASS.

Commit:

```bash
git add oauth/middleware.go oauth/middleware_test.go docs/lessons.md
git commit -m "test(oauth): harden bearer challenge contract"
```

---

## Task 4: Add JWT/JWKS Hardening Probes Where They Fit MCP-Kit

**Files:**

- Modify: `oauth/keys/manager_test.go`
- Modify: `oidc/jwks_test.go`
- Modify: `oauth/provider_test.go`
- Modify: `docs/lessons.md`

- [ ] **Step 1: Add JWKS shape tests for active and retired signing keys**

Verify `oidc.JWKSHandler` exposes only appropriate public keys and never leaks private material.

```go
func TestJWKSHandlerDoesNotExposePrivateMaterial(t *testing.T) {
	manager := keys.NewManager(keys.NewMemoryStore())
	if _, err := manager.EnsureSigningKey(context.Background()); err != nil {
		t.Fatalf("EnsureSigningKey: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	res := httptest.NewRecorder()
	oidc.JWKSHandler(manager).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, forbidden := range []string{`"d"`, `"p"`, `"q"`, `"dp"`, `"dq"`, `"qi"`} {
		if strings.Contains(body, forbidden+":") {
			t.Fatalf("JWKS leaked private key field %s in %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"kid"`) {
		t.Fatalf("JWKS missing kid: %s", body)
	}
}
```

Run:

```bash
go test ./oidc -run TestJWKSHandlerDoesNotExposePrivateMaterial -count=1
```

Expected: PASS or focused FAIL in `oidc.JWKSHandler`.

- [ ] **Step 2: Add token-signing algorithm tests**

Verify kit-issued access tokens are signed with the intended asymmetric algorithm and cannot be confused with `none` or `HS*` in local verification paths.

Use existing token issuance helpers in `oauth/provider_test.go`; if no helper exists, add a package-private helper that completes client registration and token issuance through the provider. Assert the protected header contains `alg=RS256`.

Run:

```bash
go test ./oauth -run 'Test.*AccessToken.*RS256|Test.*Rejects.*None|Test.*Rejects.*HS256' -count=1
```

Expected: PASS after the helper/test names are real.

- [ ] **Step 3: Add documentation for the boundary**

Add a lesson:

```markdown
### OG-09 - Do not replace issuer introspection with ad hoc JWT parsing

When `mcp-kit` issues the access token, validate it through the Fosite provider
or the kit-owned validation interface so revocation, storage state, expiry, and
audience checks stay coupled. JWT/JWKS hardening tests still matter for the
issuer and JWKS endpoint, but a standalone remote-JWKS resource-server verifier
is a separate feature.
```

Run:

```bash
go test ./oauth ./oauth/keys ./oidc -count=1
```

Expected: PASS.

Commit:

```bash
git add oauth/provider_test.go oauth/keys/manager_test.go oidc/jwks_test.go docs/lessons.md
git commit -m "test(oauth): add jwt and jwks hardening probes"
```

---

## Task 5: Add Downstream Conformance Runbook for Skills

**Files:**

- Create: `docs/recipes/aoa-conformance.md`
- Modify: `docs/cycle-methodology.md`
- Modify: `docs/lessons.md`

- [ ] **Step 1: Write the recipe**

Create `docs/recipes/aoa-conformance.md`.

````markdown
# AOA Conformance Smoke

Use this recipe to run external OAuth/MCP conformance checks against a
kit-backed server. It is a black-box smoke, not a replacement for unit tests.

## Current upstream status

As of 2026-07-06, `github.com/0ndreu/aoa-conformance@latest` does not compile
because it references a missing `probe.VerifyJWTWithJWKS`. Use this only after
upstream publishes a fixed version or after pinning an internal fork.

## Local Skills example

The local Skills stack normally listens at:

```bash
https://skill.dev.haak.co:8443/mcp
```

Expected unauthenticated discovery baseline from the patched 2026-07-06 run:

```text
core: 15 pass, 7 skip, 0 fail
full: 15 pass, 24 skip, 0 fail
```

The skips are expected unless the target advertises or supplies:

- `signed_metadata`
- DPoP-bound tokens
- RFC 8693 token exchange
- RFC 7662 introspection
- RFC 7009 revocation checks with credentials
- interactive auth-code credentials
- a subject token for token presentation

## Commands

```bash
AOA_CONFORMANCE_PKG=github.com/haakco/aoa-conformance/cmd/aoa-conform@<fixed-commit> \
  tooling/aoa-conformance/run.sh https://skill.dev.haak.co:8443/mcp
```

If the target uses local self-signed TLS, run the conformance CLI directly with
its `--insecure-skip-verify` flag or add CA support to the local runner.
````

Run:

```bash
sed -n '1,180p' docs/recipes/aoa-conformance.md
```

Expected: recipe captures the current blocker and expected Skills baseline.

- [ ] **Step 2: Link recipe from cycle methodology**

Add an optional probe section to `docs/cycle-methodology.md` after the bootstrap/discovery probe:

```markdown
### Optional external conformance smoke

When `aoa-conformance` has a compiling pinned version, run
`docs/recipes/aoa-conformance.md` against at least one kit-backed downstream
server before tagging transport/auth releases. Treat failures as release
blockers only after the harness version is pinned and known-good for the target
profile.
```

Run:

```bash
rg -n "Optional external conformance smoke|aoa-conformance" docs/cycle-methodology.md docs/recipes/aoa-conformance.md
```

Expected: both docs contain the conformance recipe reference.

- [ ] **Step 3: Add a probe lesson**

Add to `docs/lessons.md`:

```markdown
### PR-05 - External conformance is a black-box smoke, not a unit-test substitute

Run external conformance against a real mounted server after local unit tests
pass. Keep the harness version pinned. If the harness itself fails to compile,
record that as a tooling blocker and do not infer target compliance from it.
```

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

Commit:

```bash
git add docs/recipes/aoa-conformance.md docs/cycle-methodology.md docs/lessons.md
git commit -m "docs: add aoa conformance recipe"
```

---

## Task 6: Document Deferred DPoP and Token Exchange Decisions

**Files:**

- Modify: `DESIGN.md`
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Modify: `docs/lessons.md`

- [ ] **Step 1: Add design notes for deferred features**

Add a short subsection to `DESIGN.md` under non-goals or opinionated calls:

```markdown
### Deferred OAuth extensions

`mcp-kit` does not currently implement DPoP (RFC 9449), OAuth Token Exchange
(RFC 8693), or mTLS-bound access tokens (RFC 8705). These are valid future
extensions, but each needs a concrete consumer requirement:

- DPoP needs client key management, proof verification, clock/nonce policy, and
  a distributed replay cache for multi-instance deployments.
- Token Exchange needs a gateway/downstream delegation model and clear scope
  downscoping rules.
- mTLS-bound access tokens need deployment-level client certificate handling.

Until then, keep bearer-token recovery, issuer metadata, audience checks, token
lifetimes, revocation, and PAT boundaries as the supported production path.
```

Run:

```bash
rg -n "Deferred OAuth extensions|DPoP|Token Exchange" DESIGN.md
```

Expected: section present.

- [ ] **Step 2: Add README status note**

Add to `README.md` near the feature list:

```markdown
Not currently implemented: DPoP, OAuth Token Exchange, and mTLS-bound access
tokens. The design leaves room for them, but they are deferred until a consumer
needs them.
```

Run:

```bash
rg -n "Not currently implemented: DPoP" README.md
```

Expected: note present.

- [ ] **Step 3: Add changelog entry**

Add an unreleased entry to `CHANGELOG.md`:

```markdown
## Unreleased

- Documented the AOA review outcome: borrow conformance and hardening probes now; defer DPoP, token exchange, and mTLS-bound token runtime support until a consumer needs them.
```

If `CHANGELOG.md` already has an `Unreleased` section, append the bullet there instead of adding a duplicate heading.

Run:

```bash
rg -n "AOA review|DPoP|token exchange" CHANGELOG.md
```

Expected: changelog note present.

- [ ] **Step 4: Verify docs and tests**

Run:

```bash
go test ./... -count=1
go vet ./...
```

Expected: PASS.

Commit:

```bash
git add DESIGN.md README.md CHANGELOG.md docs/lessons.md
git commit -m "docs: record deferred oauth extension decisions"
```

---

## Final Verification

Run from the repo root:

```bash
go build ./...
go test ./...
go vet ./...
just lint-go
```

Expected:

- Build passes.
- Unit tests pass.
- Vet passes.
- Existing lint suite passes.

Optional after `aoa-conformance` is fixed or pinned:

```bash
just conformance-aoa
```

Expected after a fixed harness:

- Exit 0 for the core profile.
- No `fail` or `error` result entries.
- Skips only for capabilities not advertised by `_examples/minimal-server`.

## Completion Criteria

- `mcp-kit` has RFC 9728 metadata validation/path coverage inspired by `aoa`.
- Bearer challenge contract tests assert machine-readable recovery hints, not only loose substrings.
- JWKS/token-signing tests cover the useful `aoa` hardening themes that apply to kit-owned tokens.
- External AOA conformance is available as a non-gating target with clear upstream-build handling.
- Docs explain why DPoP and Token Exchange are deferred rather than missing by accident.
- `docs/plans/README.md` links this active plan.
