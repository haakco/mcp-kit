# OAuth

Last verified: 2026-05-02

The `oauth` package owns the OAuth 2.1 provider, token handlers, bearer context, and the production consent-helper surface used by browser authorization flows.

## Replacing The Demo Authorize Handler

`Provider.AuthorizeHandler` is intentionally small and demo-oriented. Production servers should mount `consent.NewHandler`:

```go
authorize, err := consent.NewHandler(consent.Config{
    Provider:       oauthProv,
    Authenticator:  myapp.NewAuthenticator(db),
    Renderer:       myapp.NewConsentRenderer(),
    PublicURL:      "https://my-mcp.example.com",
    ApprovalSecret: myapp.OAuthApprovalSecret(), // exactly 32 bytes
    AuditEmitter:   myapp.NewAuditEmitter(db),
})
if err != nil {
    return err
}
mux.Handle("/oauth/authorize", authorize)
```

The handler owns protocol mechanics: canonical POST-to-fosite request rebuilding, RFC 8707 strict-when-present resource validation, server-side audience binding, single-use approval tokens, and `oauth.consent.approved` / `oauth.consent.denied` audit events.

## Collaborators

- `Authenticator` checks credentials and returns `oauth.Subject`. Use `Subject.Extra` for consumer-specific claim data such as role or organization.
- `Renderer` writes the login, consent, and redirect-bridge pages. It must include `PageData.HiddenInputs` in forms.
- `ApprovalTokenStore` binds a successful login to one authorize request. Use `hmacstore` for stateless flows, or `sessionstore` when you already have a shared session backend.
- `ConsentPolicy` gates scopes and can skip consent for trusted first-party clients.
- `ChallengeProvider` is defined for future second-factor flows; no stock implementation ships yet.

## Approval Token Backends

`hmacstore.New(secret, time.Now)` signs an opaque token and tracks replay in memory. It is the best default for single-process deployments. The map is bounded by approval-token TTL times abandoned login rate; wrap it with your own janitor if adversarial mint-but-never-consume behavior is realistic.

`sessionstore.New(backend, time.Now)` stores an opaque token in a consumer backend. `SessionBackend.Pull` must atomically delete and return the entry, or replay protection is broken.

## Audit

Approve and deny paths emit:

- `EntityType`: `oauth_authorize`
- `Action`: `oauth.consent.approved` or `oauth.consent.denied`
- `ClientID`, `Scope`, and metadata for `client_name`, `decision`, `ip`, `user_agent`, and `resources`

Audit emission is best-effort. Wrap your emitter if consent decisions must be durably queued before authorize completes.

## Testing

Use `oauth/consent/consenttest` to construct an in-memory provider, register a PKCE client, and run the canonical handler suite:

```go
consenttest.RunCanonicalSuite(t, handler, consenttest.SuiteOptions{
    Renderer:        renderer,
    AuthorizeValues: values,
})
```
