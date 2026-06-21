# mcp-kit

A reusable Go library for building production-grade Model Context Protocol (MCP) servers with OAuth 2.1 authentication, key rotation, audit-ready middleware, and a battle-tested test methodology.

> **Status:** Pre-1.0 (v0.x). API may change. Pin a specific minor version.

## What it does

`mcp-kit` extracts the cross-cutting concerns common to every MCP server:

- **OAuth 2.1 server** with PKCE, dynamic client registration, refresh-token rotation, and 90-day signing-key rotation with grace window
- **Bearer middleware** that accepts both OAuth-issued JWTs and Personal Access Tokens
- **JSON-RPC envelope rewriter** so SDK protocol errors come out as canonical JSON-RPC envelopes (not plain-text 400s)
- **Origin allowlist** with explicit loopback allowance for browser MCP clients
- **OIDC / OAuth discovery endpoints** (`/.well-known/openid-configuration`, `/.well-known/oauth-authorization-server`, `/.well-known/oauth-protected-resource`, `/.well-known/jwks.json`)
- **CLI auth helper** (`mcpkit/cliauth`) with browser-based PKCE flow and issuer-scoped 0600 file-backed token cache
- **Ent schema mixins** for OAuth tables (`oauth_client`, `oauth_signing_key`, `oauth_authorization_code`, `oauth_access_token`, `oauth_refresh_token`, `personal_access_token`)
- **E2E test methodology** templates with phased dispatch runbook, evidence captures, and lessons-learned IDs

It does **not** ship:

- Domain tools, resources, or prompts — those live in the consumer
- A user table or password store — consumers authenticate browser consent and map subjects themselves
- A permission/RBAC model — consumers enforce tool/resource authorization in their domain layer
- An audit log table — consumers map kit and domain events into their own audit system

## Quickstart

```go
import (
    "net/http"

    "github.com/haakco/mcp-kit/mcpkit"
    "github.com/haakco/mcp-kit/oauth"
    "github.com/haakco/mcp-kit/oauth/consent"
    "github.com/haakco/mcp-kit/oidc"
)

func main() {
    // 1. Construct the OAuth provider using your app's storage + key manager.
    oauthProv, err := oauth.New(oauth.Config{
        Issuer:        "https://my-mcp.example.com",
        Store:         myapp.NewOAuthStore(db),
        KeyManager:    myapp.NewOAuthKeyManager(db),
        AllowedScopes: []string{"mcp.read", "mcp.write", "offline_access"},
        DefaultScopes: []string{"mcp.read", "offline_access"},
    })
    if err != nil { /* handle */ }

    // 2. Wrap your official Go SDK MCP HTTP handler.
    // The handler still owns domain authorization and audit: validate scopes,
    // check RBAC, and emit audit events inside each tool/resource.
    mcpServer, err := mcpkit.New(mcpkit.Config{
        Handler: myapp.NewAuditedMCPHandler(myapp.MCPDeps{
            Authz: myapp.NewAuthz(db),
            Audit: myapp.NewAuditEmitter(db),
        }),
        Bearer: mcpkit.BearerConfig{
            TokenValidator: myapp.NewPATValidator(db),
            Introspector:   oauthProv.OAuth2Provider(),
            SessionFactory: oauth.NewEmptySession,
        },
        AllowedOrigins: []string{"https://app.example.com"},
        AllowLoopback:  isDev,
    })
    if err != nil { /* handle */ }

    // 3. Mount on your HTTP framework.
    mux := http.NewServeMux()
    mux.Handle("/mcp", mcpServer.Handler())
    authorize, err := consent.NewHandler(consent.Config{
        Provider:       oauthProv,
        Authenticator:  myapp.NewAuthenticator(db),
        Renderer:       myapp.NewConsentRenderer(),
        PublicURL:      "https://my-mcp.example.com",
        ApprovalSecret: myapp.OAuthApprovalSecret(), // exactly 32 bytes
        AuditEmitter:   myapp.NewAuditEmitter(db),
    })
    if err != nil { /* handle */ }
    mux.Handle("/oauth/authorize", authorize)
    mux.Handle("/oauth/token", oauthProv.TokenHandler())
    mux.Handle("/oauth/register", oauthProv.RegisterHandler())

    discovery := oidc.NewDiscoveryConfig("https://my-mcp.example.com", []string{"mcp.read", "mcp.write", "offline_access"})
    discovery.RegisterRoutes(mux, oidc.RouteConfig{
        ResourceURL: "https://my-mcp.example.com/mcp",
        JWKS:        oidc.JWKSHandler(myapp.NewOAuthKeyManager(db)),
    })

    http.ListenAndServe(":8080", mux)
}
```

Authentication is not authorization. `mcp-kit` validates bearer tokens, Origin,
metadata, and JSON-RPC envelope behavior. Consumers must still enforce
tool/resource permissions and audit every sensitive domain operation in their
handlers.

## OAuth Token Lifetimes

By default, `oauth.Config` issues 1-hour access tokens and 30-day rotating refresh tokens. The short access-token lifetime limits the stale-token window when a client keeps sending a token that the server has already invalidated through revocation, database reset, or session cleanup.

For MCP clients using OAuth-backed Streamable HTTP, the standards-based recovery signal is the `WWW-Authenticate` bearer challenge on `401 Unauthorized`. `mcp-kit` includes `resource_metadata`, `scope`, `error="invalid_token"`, and `error_description` on invalid-token challenges, and publishes protected-resource metadata with `authorization_servers`, `bearer_methods_supported=["header"]`, `resource_name`, and `scopes_supported` when configured. Some clients still only refresh proactively from their local expiry timestamp and do not refresh/retry when the server returns `401 invalid_token`; lowering the access-token lifetime reduces that stale window but does not replace client-side 401 recovery.

## Documentation

- [DESIGN.md](DESIGN.md) — full design rationale, package layout, public API
- [docs/migration/skills-mcp.md](docs/migration/skills-mcp.md) — migration notes from the donor Skills MCP server
- [docs/migration/vorrent.md](docs/migration/vorrent.md) — migration notes from Vorrent's kit-backed closeout
- [docs/migration/from-mark3labs.md](docs/migration/from-mark3labs.md) — moving a Go MCP server from mark3labs to the official Go SDK
- [docs/cycle-methodology.md](docs/cycle-methodology.md) — the E2E testing protocol
- [docs/lessons.md](docs/lessons.md) — reusable MCP OAuth, JSON-RPC, and transport lessons
- [docs/dispatch-runbook-template.md](docs/dispatch-runbook-template.md) — live-client runbook template for consumers

## Status

| Phase | Status |
|---|---|
| Design | ✅ — see [DESIGN.md](DESIGN.md) |
| v0.1.0 spike — package skeletons + envelope middleware | ✅ Complete |
| v0.2.0 — OAuth core extracted from skills-mcp | ✅ Complete |
| v0.3.0 — skills-mcp migrated to kit | ✅ Complete |
| v0.4.0 — Vorrent migrated to kit | ✅ Complete |
| v1.0.0 — Meridian on kit + cycle methodology shipped | Planned |

## License

MIT — see [LICENSE](LICENSE).
