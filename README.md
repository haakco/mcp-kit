# mcp-kit

A reusable Go library for building production-grade Model Context Protocol (MCP) servers with OAuth 2.1 authentication, key rotation, audit-ready middleware, and a battle-tested test methodology.

> **Status:** Pre-1.0 (v0.x). API may change. Pin a specific minor version.

## What it does

`mcp-kit` extracts the cross-cutting concerns common to every MCP server:

- **OAuth 2.1 server** with PKCE, dynamic client registration, refresh-token rotation, and 90-day signing-key rotation with grace window
- **Bearer middleware** that accepts both OAuth-issued JWTs and Personal Access Tokens
- **JSON-RPC envelope rewriter** so SDK protocol errors come out as canonical JSON-RPC envelopes (not plain-text 400s)
- **Origin allowlist** with explicit loopback fallback for browser MCP clients
- **OIDC / OAuth discovery endpoints** (`/.well-known/openid-configuration`, `/.well-known/oauth-authorization-server`, `/.well-known/oauth-protected-resource`, `/.well-known/jwks.json`)
- **CLI auth helper** (`mcpkit/cliauth`) with browser-based PKCE flow and OS credential storage
- **Ent schema mixins** for OAuth tables (`oauth_client`, `oauth_signing_key`, `oauth_authorization_code`, `oauth_access_token`, `oauth_refresh_token`, `personal_access_token`)
- **E2E test methodology** templates with phased dispatch runbook, evidence captures, and lessons-learned IDs

It does **not** ship:

- Domain tools, resources, or prompts — those live in the consumer
- A user table or password store — consumer provides via `UserStore` interface
- A permission/RBAC model — consumer provides via `AuthzService` interface
- An audit log table — consumer provides via `AuditEmitter` interface

## Quickstart

```go
import (
    "github.com/haakco/mcp-kit/mcpkit"
    "github.com/haakco/mcp-kit/mcpkit/oauth"
    "github.com/haakco/mcp-kit/mcpkit/oidc"
)

func main() {
    // 1. Provide your app's user/authz/audit implementations.
    userStore := myapp.NewUserStore(db)
    authz := myapp.NewAuthz(db)
    audit := myapp.NewAuditEmitter(db)

    // 2. Construct the OAuth provider.
    oauthProv, err := oauth.New(oauth.Config{
        Issuer:        "https://my-mcp.example.com",
        EntClient:     entClient,
        UserStore:     userStore,
        AuditEmitter:  audit,
        KeyRotation:   oauth.DefaultKeyRotation, // 90d / 48h grace
    })
    if err != nil { /* handle */ }

    // 3. Construct the MCP server.
    mcpServer := mcpkit.New(mcpkit.Config{
        Implementation: mcp.Implementation{Name: "my-mcp", Version: "0.1.0"},
        Validator:      oauthProv.TokenValidator(),
        AllowedOrigins: []string{"https://app.example.com"},
        AllowLoopback:  isDev,
    })

    // 4. Register your tools/resources/prompts.
    myapp.RegisterTools(mcpServer, deps)

    // 5. Mount on your HTTP framework.
    mux := http.NewServeMux()
    mux.Handle("/mcp", mcpServer.Handler())
    oauthProv.RegisterRoutes(mux)
    oidc.RegisterRoutes(mux, oauthProv.DiscoveryConfig())

    http.ListenAndServe(":8080", mux)
}
```

## Documentation

- [DESIGN.md](DESIGN.md) — full design rationale, package layout, public API
- [docs/migration/skills-mcp.md](docs/migration/skills-mcp.md) — *(coming with v0.2.0)* migrating from skills-mcp's vendored OAuth
- [docs/migration/vorrent.md](docs/migration/vorrent.md) — *(coming with v0.2.0)* migrating from Vorrent's vendored MCP server
- [docs/cycle-methodology.md](docs/cycle-methodology.md) — the E2E testing protocol
- [docs/lessons.md](docs/lessons.md) — reusable MCP OAuth, JSON-RPC, and transport lessons
- [docs/dispatch-runbook-template.md](docs/dispatch-runbook-template.md) — live-client runbook template for consumers

## Status

| Phase | Status |
|---|---|
| Design | ✅ — see [DESIGN.md](DESIGN.md) |
| v0.1.0 spike — package skeletons + envelope middleware | ✅ Complete |
| v0.2.0 — OAuth core extracted from skills-mcp | Planned |
| v0.3.0 — skills-mcp migrated to kit | Planned |
| v0.4.0 — Vorrent migrated to kit | Planned |
| v1.0.0 — Meridian on kit + cycle methodology shipped | Planned |

## License

MIT — see [LICENSE](LICENSE).
