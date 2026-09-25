# mcp-kit

A reusable Go library for building production-grade Model Context Protocol (MCP) servers with OAuth 2.1 authentication, key rotation, audit-ready middleware, and a battle-tested test methodology.

> **Status:** Pre-1.0 (v0.x). API may change. Pin a specific minor version.
>
> **Protocol:** MCP revision **2026-07-28**, modern-only. The kit serves one revision, statelessly, with per-request
> metadata instead of the retired `initialize` handshake. Requires Go 1.27 and MCP Go SDK v1.8.0.

## What it does

`mcp-kit` extracts the cross-cutting concerns common to every MCP server:

- **OAuth 2.1 server** with PKCE, dynamic client registration, Client ID Metadata Documents (SEP-991), refresh-token rotation, and 90-day signing-key rotation with grace window
- **Bearer middleware** that accepts both OAuth-issued JWTs and Personal Access Tokens
- **JSON-RPC envelope rewriter** so the plain-text protocol errors an SDK handler can still emit come out as canonical JSON-RPC envelopes
- **Private-by-default cache policy** (`mcpkit.PrivateCache`) so authenticated results are never marked `public`
- **Origin allowlist** with explicit loopback allowance for browser MCP clients
- **OIDC / OAuth discovery endpoints** (`/.well-known/openid-configuration`, `/.well-known/oauth-authorization-server`, `/.well-known/oauth-protected-resource`, path-specific protected-resource metadata, `/.well-known/jwks.json`)
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
        AllowedScopes: []string{"mcp.read", "mcp.write"},
        DefaultScopes: []string{"mcp.read"},
    })
    if err != nil { /* handle */ }

    resourceURL := "https://my-mcp.example.com/mcp"
    resourceMetadataURL, err := oauth.ProtectedResourceMetadataURLFor(resourceURL)
    if err != nil { /* handle */ }

    // 2. Build the SDK server and its handler first: the consumer owns identity,
    //    capabilities, and the transport.
    sdkServer := mcp.NewServer(
        &mcp.Implementation{Name: "my-server", Version: "1.0.0"},
        &mcp.ServerOptions{
            Instructions: "...",
            // Explicit empty capabilities: stops the SDK advertising the
            // deprecated default logging capability.
            Capabilities:              &mcp.ServerCapabilities{},
            SupportedProtocolVersions: []string{mcpkit.ProtocolVersion},
            // Authenticated results must not be cached as public.
            SetCacheable: mcpkit.PrivateCache(mcpkit.DefaultCacheTTL),
        },
    )
    // mcp.AddTool(sdkServer, ...)

    // 3. Wrap it with the kit. Middleware order is fixed:
    //    Origin -> Bearer -> Envelope -> SDK handler.
    //    The handler still owns domain authorization and audit: validate scopes,
    //    check RBAC, and emit audit events inside each tool/resource.
    mcpServer, err := mcpkit.New(mcpkit.Config{
        Handler: mcp.NewStreamableHTTPHandler(
            func(*http.Request) *mcp.Server { return sdkServer },
            // Mandatory for 2026-07-28 over Streamable HTTP.
            &mcp.StreamableHTTPOptions{Stateless: true},
        ),
        Bearer: mcpkit.BearerConfig{
            TokenValidator:      myapp.NewPATValidator(db),
            Introspector:        oauthProv.OAuth2Provider(),
            SessionFactory:      oauth.NewEmptySession,
            ResourceMetadataURL: resourceMetadataURL,
            RequiredScopes:      []string{"mcp.read"},
            ExpectedAudience:    resourceURL,
        },
        AllowedOrigins: []string{"https://my-mcp.example.com"},
        AllowLoopback:  isDev,
    })
    if err != nil { /* handle */ }

    // 4. Mount on your HTTP framework.
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

    discovery := oidc.NewDiscoveryConfig("https://my-mcp.example.com", []string{"mcp.read", "mcp.write"})
    discovery.RegisterRoutes(mux, oidc.RouteConfig{
        ResourceURL: resourceURL,
        JWKS:        oidc.JWKSHandler(myapp.NewOAuthKeyManager(db)),
    })

    http.ListenAndServe(":8080", mux)
}
```

Authentication is not authorization. `mcp-kit` validates bearer tokens, Origin,
metadata, and JSON-RPC envelope behavior. Consumers must still enforce
tool/resource permissions and audit every sensitive domain operation in their
handlers.

For the full walkthrough, including the exact server options the protocol
requires, see [docs/migration/new-server.md](docs/migration/new-server.md) and
[docs/recipes/stateless-http.md](docs/recipes/stateless-http.md).

## OAuth Token Lifetimes

By default, `oauth.Config` issues 1-hour access tokens and 30-day rotating refresh tokens. The short access-token lifetime limits the stale-token window when a client keeps sending a token that the server has already invalidated through revocation, database reset, or session cleanup.

For MCP clients using OAuth-backed Streamable HTTP, the standards-based recovery signal is the `WWW-Authenticate` bearer challenge on `401 Unauthorized`. `mcp-kit` includes `resource_metadata`, `error="invalid_token"`, and `error_description` on invalid-token challenges, and publishes protected-resource metadata with `authorization_servers`, `bearer_methods_supported=["header"]`, `resource_name`, and `scopes_supported` when configured. Scope hints are reserved for `403 insufficient_scope` challenges. Use `oauth.ProtectedResourceMetadataURLFor(resourceURL)` to keep the 401 challenge and discovery documents on the same RFC 9728 path, such as `/.well-known/oauth-protected-resource/mcp` for a `/mcp` resource. Some clients, including observed Codex/rmcp versions, still only refresh proactively from their local expiry timestamp and do not refresh/retry when the server returns `401 invalid_token`; lowering the access-token lifetime reduces that stale window but does not replace client-side 401 recovery.

## Documentation

- [DESIGN.md](DESIGN.md) — full design rationale, package layout, public API
- [docs/migration/new-server.md](docs/migration/new-server.md) — build a new 2026-07-28 server on the kit
- [docs/recipes/stateless-http.md](docs/recipes/stateless-http.md) — stateless Streamable HTTP, and what it rules out
- [docs/conformance.md](docs/conformance.md) — the official conformance gate
- [docs/migration/skills-mcp.md](docs/migration/skills-mcp.md) — migration notes from the donor Skills MCP server
- [docs/migration/vorrent.md](docs/migration/vorrent.md) — migration notes from Vorrent's kit-backed closeout
- [docs/migration/from-mark3labs.md](docs/migration/from-mark3labs.md) — moving a Go MCP server from mark3labs to the official Go SDK
- [docs/cycle-methodology.md](docs/cycle-methodology.md) — the E2E testing protocol
- [docs/lessons.md](docs/lessons.md) — reusable MCP OAuth, JSON-RPC, and transport lessons
- [docs/dispatch-runbook-template.md](docs/dispatch-runbook-template.md) — live-client runbook template for consumers
- [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md)

## Status

| Phase | Status |
|---|---|
| Design | ✅ — see [DESIGN.md](DESIGN.md) |
| v0.1.0 spike — package skeletons + envelope middleware | ✅ Complete |
| v0.2.0 — OAuth core extracted from skills-mcp | ✅ Complete |
| v0.3.0 — skills-mcp migrated to kit | ✅ Complete |
| v0.4.0 — Vorrent migrated to kit | ✅ Complete |
| v0.5.x — consent helpers, OAuth/PAT scope targeting, cache + discovery hardening | ✅ Complete |
| v0.6.0 — MCP 2026-07-28 migration (modern-only, stateless, CIMD, conformance gate) | 🔄 Kit complete; consumer rollout in progress |
| v1.0.0 — stable | Planned; deferred until the consumers have operated on the new wire |

## License

MIT — see [LICENSE](LICENSE).
