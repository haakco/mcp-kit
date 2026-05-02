# Recipe: Production OAuth consent

**Status:** Pattern plus kit helper package.

**Surfaced from:** vorrent, skills-mcp, and planned meridian OAuth authorize flows.

## Problem

Every MCP server needs the same browser authorization shape: validate the OAuth request, authenticate the user, show consent, bind the MCP resource audience, and issue an authorization code. Hand-rolled handlers drift on replay protection, RFC 8707 resource indicators, and audit events.

## Recommended Pattern

Use `oauth/consent.NewHandler` for `/oauth/authorize`. The consumer still owns the user model and HTML, but the kit owns the protocol-sensitive mechanics.

```go
authorize, err := consent.NewHandler(consent.Config{
    Provider:       oauthProv,
    Authenticator:  appAuthenticator,
    Renderer:       appConsentRenderer,
    PublicURL:      "https://my-mcp.example.com",
    ApprovalSecret: approvalSecret32Bytes,
    AuditEmitter:   auditEmitter,
})
if err != nil {
    return err
}
mux.Handle("/oauth/authorize", authorize)
```

## Backend Choice

Use `hmacstore` when you want a stateless approval token and can tolerate an in-process replay map.

Use `sessionstore` when your app already has a shared session backend. Its `Pull` operation must atomically delete and return entries.

## Scope Gates

Put domain-specific scope rules in `ConsentPolicy.ValidateScopes`. For example, admin scopes should be rejected before code issuance when the authenticated subject is not an admin.

Use `ConsentPolicy.AllowsSkip` only for first-party clients you can identify reliably.

## Related Kit Packages

- `oauth/README.md` — API overview and integration notes.
- `oauth/consent` — handler and interfaces.
- `oauth/consent/consenttest` — canonical test fixtures.

## Related Lessons

- `OG-04` — PKCE base64url substitution.
- `AG-03` — missing auth context must fail closed.
