# Security Policy

## Supported versions

`mcp-kit` is pre-1.0. Only the latest tagged release is supported; there are no backports to earlier minor versions.
Consumers should track the latest tag and read [CHANGELOG.md](CHANGELOG.md) for breaking changes.

| Version | Supported |
|---|---|
| latest tag | Yes |
| anything older | No |

## Reporting a vulnerability

Report privately through [GitHub Security Advisories](https://github.com/haakco/mcp-kit/security/advisories/new) rather
than in a public issue.

Please include:

- the affected version and, where relevant, the Go and MCP SDK versions;
- the smallest reproduction you have, or the request/response that shows the problem;
- the impact you believe it has — data exposure, auth bypass, resource exhaustion, and so on.

We will acknowledge the report, confirm whether it is a real issue, and tell you the plan. This is a small team with
other responsibilities, so we do not promise a fixed response time; we do promise not to sit on a confirmed issue
silently.

## What the kit protects

The kit owns the transport envelope, bearer authentication, OAuth, discovery, and signing keys. Reports in these areas
are in scope:

- **Middleware order and bypasses.** `Origin → Bearer → Envelope → SDK handler`. Anything that reaches the SDK handler
  without authentication, or that leaks scope/identity across callers, is a bug.
- **Bearer and PAT handling.** Token validation, audience/resource-indicator enforcement, scope boundaries
  (`RequireScopeForTarget`), and the "auth disabled" sentinel. Inferring "auth disabled" from empty scopes is a
  privilege-escalation bug, not a style issue.
- **OAuth flows.** PKCE enforcement, `state` handling, redirect-URI validation, refresh-token rotation and replay,
  consent handling, and the RFC 9207 `iss` check.
- **Client ID Metadata Documents.** The server fetches a URL an unauthenticated caller supplies. Refusing loopback,
  private, link-local, multicast, and unspecified targets after DNS resolution, dialling the validated address, and
  bounding time, size, and redirects are all security controls. A report showing any of them can be bypassed is in
  scope.
- **Challenge and metadata correctness.** `WWW-Authenticate` quoting, `resource_metadata`, and protected-resource
  metadata. A malformed challenge can break client recovery or forge parameters.
- **Signing keys.** JWKS must publish public material only; rotation must not shorten the verification window it
  promises.

## What is out of scope

- **Consumer-owned surface.** Tools, resources, prompts, user tables, RBAC, audit storage, and CORS configuration belong
  to the consuming service. A vulnerable tool is the consumer's report.
- **Deliberately unimplemented features.** DPoP, token exchange, mTLS, tasks, and MCP Apps. Their absence is not a
  vulnerability; a report that the kit "does not support" one of them is not actionable.
- **The SDK and dependencies.** Report upstream, and tell us if the kit needs to pin or work around it. We run
  `govulncheck` in CI.
- **Deployments that disable protections**, such as `AllowUnauthenticated: true` in production, `AllowLoopback: true`
  with a public listener, or `DisableLocalhostProtection` on an exposed server.
- **Missing hardening with no concrete risk.** Reports that amount to "consider adding X" without a demonstrated impact
  will be closed with an explanation.

## How the kit is built to fail safely

- Auth fails closed: a provider with no introspector and no token validator denies by default.
- JWT client authentication fails closed (`ClientAssertionJWTValid` returns `ErrJTIKnown`) rather than silently
  disabling JTI replay protection.
- A Client ID Metadata Document that cannot be resolved reports an invalid client, and the fetch failure is not echoed
  to the caller.
- Input at the real trust boundary — client metadata documents, redirect URIs, token metadata — is validated where it
  enters, not where it is used.
