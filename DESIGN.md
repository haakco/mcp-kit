# mcp-kit Design

**Status:** v0.1 design (2026-05-01). Reviewable in one sitting.
**Goal:** A reusable Go library that lets any HaakCo Go service expose a production-grade MCP endpoint with OAuth 2.1 + PKCE, key rotation, audit-ready middleware, and battle-tested test methodology, in under 200 lines of consumer-side glue code.

---

## Why this exists

Three HaakCo Go services already need or are about to need an MCP server:

| Service | Status |
|---|---|
| `vorrent` | Has MCP server, OAuth 2.1, no key rotation, no PAT, custom envelope middleware (the only repo with it) |
| `skills-mcp` | Has MCP server (mark3labs SDK), OAuth 2.1 with full surface (key rotation, PAT, OIDC, login/signup/reset), no envelope middleware |
| `meridian` | About to add MCP per `add_mcp.md`. No OAuth today. |

Without a shared kit, each service ships its own ~3000 lines of OAuth + middleware + discovery + key rotation + PAT + CLI auth helper, and the three implementations drift. With a shared kit, that ~3000 lines lives once; each service ships only its tools/resources/prompts plus ~200 lines of glue.

This is **only** for Go services. HaakCo also has Laravel-based MCP work — that uses a separate language ecosystem and its own libraries (Laravel Passport for OAuth, etc.). The kit is intentionally Go-specific. Universal concepts (cycle methodology, lessons-learned IDs, dispatch runbook patterns) are documented in shared skills and applied to every server regardless of language.

---

## Non-goals

The kit deliberately does **not**:

- **Ship tools, resources, or prompts.** Domain logic stays in the consumer.
- **Provide a user table or password store.** Consumers authenticate browser consent and map OAuth subjects.
- **Provide an authz/RBAC model.** Consumers enforce tool/resource authorization in their domain layer.
- **Provide an audit-log table.** Consumers map kit and domain events into their own audit system.
- **Lock to a single HTTP framework.** Returns `http.Handler`; consumer wraps in stdlib mux, Echo, Chi, Gin, whatever.
- **Lock to a single database.** Default Ent adapters ship; the storage layer is interface-based so a non-Ent app could plug in.
- **Cross language boundaries.** Laravel/Node MCP servers don't depend on this kit. The cycle methodology and design principles transfer; the code does not.

---

## Opinionated calls (pre-decided so the API stays small)

| Decision | Choice | Rationale |
|---|---|---|
| **MCP SDK** | `github.com/modelcontextprotocol/go-sdk` v1.5.0+ (official) | Anthropic-maintained, ecosystem direction, Vorrent's cycle-1 lessons (TQ-01..03, OG-04..05, FP-01) already proven against it. skills-mcp migrates from `mark3labs/mcp-go` as part of adopting the kit. |
| **OAuth library** | `github.com/ory/fosite` v0.49+ | Both reference servers already use it; PKCE + dynamic registration + refresh rotation supported out of the box. |
| **JWS library** | `github.com/go-jose/go-jose/v3` | Both reference servers use it. |
| **Database access** | `entgo.io/ent` v0.14+ | Both reference servers use it. Storage is behind an interface so non-Ent consumers can plug in, but the Ent adapter is the supported path. |
| **HTTP framework** | None (stdlib `http.Handler`) | Each consumer mounts in its own framework. |
| **Token format** | RS256-signed JWT | Both reference servers use it. Symmetric (HMAC) signing rejected because key rotation is harder. |
| **Default scope prefix** | None (consumer-supplied) | `vorrent.read`, `mcp.patient.summary`, `skills.write` — kit ships the prefix mechanism, not the names. |
| **Error envelope** | Canonical JSON-RPC 2.0 (-32700, -32600, -32601, -32602, -32000..-32099) | RFC-compliant; what real MCP clients (Inspector, Claude Code) expect. |

---

## Package layout

```
mcp-kit/
├── go.mod                              # module github.com/haakco/mcp-kit
├── README.md
├── DESIGN.md                           # this file
├── LICENSE
│
├── mcpkit/                             # Top-level convenience package
│   ├── server.go                       # mcpkit.New() + mcpkit.Config
│   ├── server_test.go
│   └── doc.go                          # Package-level docs + Quickstart
│
├── oauth/                              # OAuth 2.1 server
│   ├── config.go                       # Config struct + sane defaults
│   ├── provider.go                     # New() — constructs Fosite provider with our compose
│   ├── handlers.go                     # /authorize, /token, /register, /revoke as http.HandlerFuncs
│   ├── middleware.go                   # Bearer middleware (OAuth + PAT, canonical envelopes)
│   ├── token_validator.go              # TokenValidator interface + impl
│   ├── pat.go                          # PAT validator + interface
│   ├── login_classify.go               # Fixed-vocabulary login error classifier
│   ├── pkce.go                         # PKCE base64url helpers (substitution-correct)
│   └── *_test.go
│
├── oauth/keys/                         # Signing key management
│   ├── manager.go                      # EnsureSigningKey, ActiveJWKSet, RotateSigningKey, RetireExpiredKeys
│   ├── rotator.go                      # Background rotator goroutine (90d/48h grace)
│   └── *_test.go
│
├── oauth/storage/                      # Pluggable storage
│   ├── storage.go                      # Fosite storage interface
│   ├── ent.go                          # Default Ent-backed implementation
│   └── *_test.go
│
├── oidc/                               # OIDC + OAuth discovery endpoints
│   ├── discovery.go                    # /.well-known/oauth-authorization-server, openid-configuration
│   ├── protected_resource.go           # /.well-known/oauth-protected-resource
│   ├── jwks.go                         # /.well-known/jwks.json
│   └── *_test.go
│
├── mcpmw/                              # MCP-specific middleware
│   ├── envelope.go                     # JSON-RPC envelope rewriter (Vorrent's contribution)
│   ├── origin.go                       # Origin allowlist with explicit loopback allowance
│   ├── audit.go                        # Audit emitter wrapper for tool calls
│   └── *_test.go
│
├── cliauth/                            # CLI auth helper
│   ├── pkce.go                         # PKCE pair generation
│   ├── browser.go                      # Open system browser to authorize URL
│   ├── login.go                        # Loopback redirect listener + code exchange
│   ├── credstore.go                    # OS-appropriate credential storage
│   └── *_test.go
│
├── entschema/                          # Ent schema mixins for consumers
│   ├── oauth_client.go                 # Mixin for oauth_client table
│   ├── oauth_signing_key.go
│   ├── oauth_authorization_code.go
│   ├── oauth_access_token.go
│   ├── oauth_refresh_token.go
│   ├── personal_access_token.go
│   └── README.md                       # How consumers compose mixins into their schemas
│
├── audit/                              # Audit interface + redaction helpers
│   ├── emitter.go                      # AuditEmitter interface
│   └── *_test.go
│
├── userstore/                          # UserStore interface + reference helpers
│   ├── store.go                        # UserStore interface
│   └── *_test.go
│
├── authz/                              # Authz interface
│   └── authz.go                        # AuthzService interface (no implementation — consumer's responsibility)
│
├── testkit/                            # Test helpers for consumers
│   ├── server.go                       # Spin up a kit-backed MCP server in-memory
│   ├── token.go                        # Mint test tokens without going through the full OAuth flow
│   ├── handshake.go                    # Run the 3-step Streamable HTTP handshake
│   └── *_test.go
│
└── docs/                               # Long-form docs
    ├── cycle-methodology.md            # The phased E2E test cycle (universal, applies beyond Go)
    ├── lessons.md                      # TQ-01..03, OG-04..05, FP-01 — the whole catalog
    ├── dispatch-runbook-template.md    # Empty runbook with all phases stubbed
    └── migration/
        ├── skills-mcp.md               # Step-by-step migration
        ├── vorrent.md                  # Step-by-step migration
        └── new-server.md               # Bootstrapping a new server
```

**Total lines:** ~3500 of library code + ~1000 of docs. Consumers shed ~3000 each.

---

## Public API

### `mcpkit.Config` and `mcpkit.New()`

The top-level entry point. Consumers create the official Go SDK server,
convert it to an `http.Handler`, then pass that handler to the kit for
Origin, bearer-token, and JSON-RPC envelope middleware.

```go
package mcpkit

import (
    "net/http"

    "github.com/haakco/mcp-kit/audit"
    "github.com/haakco/mcp-kit/oauth"
)

type Config struct {
    // Handler is the official Go SDK MCP HTTP handler before kit middleware.
    Handler http.Handler

    // Bearer authenticates bearer tokens before the SDK handler.
    Bearer BearerConfig

    // Validator is deprecated. Use Bearer.TokenValidator.
    Validator oauth.TokenValidator

    // AllowedOrigins is the Origin header allowlist for browser clients.
    AllowedOrigins []string

    // AllowLoopback permits Origin: http://127.0.0.1[:port], http://localhost[:port],
    // http://[::1][:port]. Default false. Set true in dev.
    AllowLoopback bool

    // AuditEmitter receives kit-owned audit events. Domain tool-call authz and
    // audit stay in the consumer-owned SDK tool handlers.
    // For tests, use audit.Discard.
    AuditEmitter audit.Emitter
}

type Server struct { /* unexported */ }

func New(cfg Config) (*Server, error)

// Handler returns the http.Handler to mount at /mcp.
// It composes (in outer-to-inner order): origin → bearer → envelope → MCP SDK.
func (s *Server) Handler() http.Handler

// Domain tools, resources, and prompts are registered on the consumer-owned
// official SDK server before passing its HTTP handler to mcpkit.New.
```

### `oauth.Config` and `oauth.New()`

The OAuth provider. Owns the entire flow: register → authorize → token → refresh → revoke.

```go
package oauth

type Config struct {
    // Issuer is the canonical URL of this server. Required.
    // MUST match the URL clients see — used in discovery + token aud claim.
    Issuer string

    // Store persists OAuth clients and grants.
    Store storage.Store

    // KeyManager owns signing keys and JWKS.
    KeyManager *keys.Manager

    // AccessTokenLifespan defaults to 1h.
    AccessTokenLifespan time.Duration
    // RefreshTokenLifespan defaults to 24h.
    RefreshTokenLifespan time.Duration
    // AuthorizationCodeLifespan defaults to 10m.
    AuthorizationCodeLifespan time.Duration
}

type Provider struct { /* unexported */ }

func New(cfg Config) (*Provider, error)

func (p *Provider) OAuth2Provider() fosite.OAuth2Provider
func (p *Provider) AuthorizeHandler(resolve SubjectResolver) http.Handler
func (p *Provider) TokenHandler() http.Handler
func (p *Provider) RegisterHandler() http.Handler
```

### Interfaces consumers implement

```go
// userstore.Store — consumers wrap their existing user table.
package userstore

type User interface {
    ID() uuid.UUID
    Email() string
    PasswordHash() []byte // bcrypt-hashed, or nil if user has no password
    IsActive() bool
}

type Store interface {
    // FindByEmail returns the user matching the email, or ErrNotFound.
    FindByEmail(ctx context.Context, email string) (User, error)
    // FindByID returns the user matching the ID, or ErrNotFound.
    FindByID(ctx context.Context, id uuid.UUID) (User, error)
}

var ErrNotFound = errors.New("user not found")

// Helper: VerifyPassword wraps bcrypt + constant-time comparison + error mapping.
// Returns (user, nil) on success, (nil, ErrInvalidCredentials) on failure.
func VerifyPassword(ctx context.Context, store Store, email, password string) (User, error)
```

```go
// audit.Emitter — consumers wrap their existing audit log.
package audit

type Event struct {
    EntityType  string         // "mcp_tool", "oauth_token", "oauth_key", ...
    EntityID    string         // tool name, jti, kid, ...
    Action      string         // "execute", "issued", "rotated", ...
    ActorUserID *uuid.UUID
    ClientID    string
    Scope       string
    PayloadHash string         // sha256 of redacted params, hex
    Metadata    map[string]any // free-form (after redaction)
    Timestamp   time.Time
}

type Emitter interface {
    Emit(ctx context.Context, event Event) error
}

// Helper
func Discard() Emitter // for tests
```

```go
// authz.Service — consumers wrap their existing RBAC.
package authz

type Service interface {
    // Check returns nil if userID has permission, ErrForbidden otherwise.
    Check(ctx context.Context, userID uuid.UUID, permission string) error
}

var ErrForbidden = errors.New("forbidden")
```

### `mcpmw` — middleware as primitives

Consumers can use the middleware standalone if they want non-default composition.

```go
package mcpmw

// Envelope rewrites SDK plain-text protocol errors as canonical JSON-RPC envelopes.
// Passes through SSE responses (Content-Type: text/event-stream) unchanged.
// Passes through 401/403 (auth boundary stays untouched).
func Envelope(next http.Handler) http.Handler

// Origin enforces an allowlist on the Origin header.
// Empty origin (non-browser client) → allow. Allowed → allow. Loopback if AllowLoopback → allow. Else 403.
type OriginConfig struct {
    Allowed       []string
    AllowLoopback bool
}
func Origin(cfg OriginConfig, next http.Handler) http.Handler

// Tool-call audit belongs in the consumer's SDK tool handlers because only the
// consumer knows which domain fields are sensitive and which RBAC decision was
// made. Kit audit primitives are available for consumer adapters and future
// kit-owned events.
```

### `entschema` — opt-in mixins

Consumers add the kit's tables to their Ent schema via mixins. They keep their existing user/audit/role schemas untouched.

```go
// In consumer's ent/schema/oauth_client.go:
package schema

import (
    "entgo.io/ent"
    kitschema "github.com/haakco/mcp-kit/entschema"
)

type OAuthClient struct{ ent.Schema }

func (OAuthClient) Mixin() []ent.Mixin {
    return []ent.Mixin{
        kitschema.OAuthClient{}, // provides all standard fields + indices
    }
}

// Consumer can add their own edges/fields here.
func (OAuthClient) Edges() []ent.Edge { return nil }
```

The mixin defines fields, indices, and the canonical schema name. Consumers can extend but should not redefine the kit's fields.

---

## How a consumer mounts everything

Reference glue (~120 lines) in a fresh service:

```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"

    "entgo.io/ent/dialect"
    entsql "entgo.io/ent/dialect/sql"

    "github.com/haakco/mcp-kit/audit"
    "github.com/haakco/mcp-kit/mcpkit"
    "github.com/haakco/mcp-kit/oauth"
    "github.com/haakco/mcp-kit/oidc"
    "github.com/modelcontextprotocol/go-sdk/mcp"

    "myapp/ent"
    "myapp/internal/myauth"
    "myapp/internal/mytools"
)

func main() {
    logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
    db := openDB()
    drv := entsql.OpenDB(dialect.SQLite, db)
    entClient := ent.NewClient(ent.Driver(drv))

    // Wrap the consumer's audit log.
    auditEmit := myauth.NewAuditEmitter(entClient)

    // Construct the OAuth provider.
    oauthProv, err := oauth.New(oauth.Config{
        Issuer:        "https://my-mcp.example.com",
        Store:         myapp.NewOAuthStore(entClient),
        KeyManager:    myapp.NewOAuthKeyManager(entClient),
        AllowedScopes: []string{"mcp.read", "mcp.write", "offline_access"},
        DefaultScopes: []string{"mcp.read", "offline_access"},
    })
    if err != nil { panic(err) }

    // Construct the consumer-owned SDK server and wrap its handler with the kit.
    sdkServer := mcp.NewServer(&mcp.Implementation{Name: "my-mcp", Version: "0.1.0"}, nil)
    sdkHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
        return sdkServer
    }, nil)
    mcpServer, err := mcpkit.New(mcpkit.Config{
        Handler: sdkHandler,
        Bearer: mcpkit.BearerConfig{
            Introspector:    oauthProv.OAuth2Provider(),
            SessionFactory: oauth.NewEmptySession,
        },
        AllowedOrigins: []string{"https://app.example.com"},
        AllowLoopback:  isDev(),
        AuditEmitter:   auditEmit,
    })
    if err != nil { panic(err) }

    // Register the consumer's tools/resources/prompts on the consumer-owned SDK server.
    mytools.Register(sdkServer, entClient, users)

    // Start the key rotator.
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    rotator := keys.NewRotator(myapp.NewOAuthKeyManager(entClient), keys.RotationConfig{}, logger)
    go func() {
        rotator.Run(ctx)
    }()

    // Mount on stdlib mux.
    mux := http.NewServeMux()
    mux.Handle("/mcp", mcpServer.Handler())
    oauthProv.RegisterRoutes(mux, "/oauth", myauth.ResolveSubject)
    discovery := oidc.NewDiscoveryConfig("https://my-mcp.example.com", []string{"mcp.read", "mcp.write", "offline_access"})
    discovery.RegisterRoutes(mux, oidc.RouteConfig{
        ResourceURL: "https://my-mcp.example.com/mcp",
        JWKS:        oidc.JWKSHandler(myapp.NewOAuthKeyManager(entClient)),
    })
    mux.HandleFunc("/healthz", healthHandler)

    if err := http.ListenAndServe(":8080", mux); err != nil { panic(err) }
}
```

That's it. The consumer's domain code (`myauth`, `mytools`) is the part they actually have to write.

---

## Consumer migration paths

### skills-mcp (largest migration)

Currently uses `mark3labs/mcp-go`; kit uses official SDK.

| Step | Action | Effort |
|---|---|---|
| 1 | Adopt kit at v0.1.0 — vendor + import; **don't replace anything yet** | 0.5d |
| 2 | Wrap existing `internal/auth/PATServicer` in `userstore.Store` | 0.5d |
| 3 | Wrap existing `internal/auth/audit` in `audit.Emitter` | 2h |
| 4 | Replace `internal/oidc/keys.go`, `rotator.go` with kit's `oauth/keys/` | 0.5d (mostly delete) |
| 5 | Replace `internal/oidc/handlers.go`, `provider.go`, `register.go`, `storage.go` with kit's `oauth/` | 1d |
| 6 | Replace `internal/auth/middleware.go`, `pat_validator.go`, `log_classify.go`, `verify.go` with kit's `oauth.Bearer` | 1d |
| 7 | Replace `internal/cliauth/` with kit's `cliauth/` | 0.5d |
| 8 | Migrate tool registration from mark3labs to official SDK syntax | 1.5d |
| 9 | Run `verify-mcp-clients` against kit-backed server; fix drift | 1d |
| 10 | Delete the replaced files | 0.5d |
| **Total** | | **~7 days** |

Net: skills-mcp deletes ~3000 lines, gains the JSON-RPC envelope middleware + origin allowlist they didn't have, ships on the official SDK.

### vorrent (smaller migration)

Already on official SDK. Just swaps in the OAuth core.

| Step | Action | Effort |
|---|---|---|
| 1 | Adopt kit | 2h |
| 2 | Wrap existing user/audit/authz | 0.5d |
| 3 | Replace `internal/api/mcp_oauth_*.go`, `internal/oauth/*` with kit's `oauth/` | 1d |
| 4 | Replace `internal/mcpserver/{server.go, dynamic_bearer_auth.go}` with kit's `mcpkit.New()` | 0.5d |
| 5 | Keep `internal/mcpserver/{tools.go, resources.go, prompts.go, jsonrpc_envelope.go, origin.go}` — these are domain or already-extracted-into-kit | n/a |
| 6 | Delete envelope middleware + origin allowlist source files (now in kit) | 1h |
| 7 | Run cycle 2 against kit-backed server | 0.5d |
| **Total** | | **~3 days** |

Net: vorrent deletes ~1500 lines, gains key rotation + PAT it didn't have.

### meridian (greenfield adoption)

Per `add_mcp.md` rewritten for kit (Task 40 below): collapses 7 phases to ~4.

| Step | Action | Effort |
|---|---|---|
| 1 | Adopt kit | 2h |
| 2 | Compose Ent schemas via mixins | 0.5d |
| 3 | Wrap user/audit/authz | 0.5d |
| 4 | Wire kit into `server.New()` (Echo wrap of `http.Handler`) | 0.5d |
| 5 | Register 7 v1 tools (medications, ICD-10, self-only patient summary, audit) | 2d |
| 6 | Healthcare sensitivity audit + compliance sign-off | 0.5d |
| **Total** | | **~4 days** |

Net: meridian writes ~600 lines of glue + tool registration, gets the full OAuth surface for free.

---

## Versioning + release plan

| Version | Scope | Gate |
|---|---|---|
| **v0.1.0** | Skeleton: package layout, public API stubs, JSON-RPC envelope middleware + origin allowlist (Vorrent's contributions) ported with tests. Other packages return `ErrNotImplemented`. | API-shape feedback from a second pair of eyes. |
| **v0.2.0** | OAuth core extracted from skills-mcp: provider, storage (Ent), discovery, JWKS, key rotation, PAT, login classifier. Consumer interfaces implemented and tested with in-memory fixtures. | All `oauth/*` tests pass; example consumer in `_examples/minimal-server/` boots and serves `tools/list` against a kit-issued token. |
| **v0.3.0** | skills-mcp migrated. Kit's API survives a real consumer. Cycle methodology + lessons-learned ported into kit docs. | skills-mcp's `verify-mcp-clients` green against kit-backed binary. |
| **v0.4.0** | vorrent migrated. Kit serves two consumers. Any drift between vorrent's needs and skills-mcp's needs is reconciled. | vorrent cycle 2 (real-client gates against kit binary) green. |
| **v1.0.0** | meridian on kit. Three consumers, stable API, full docs. Public release. | All three consumers' test suites green; SemVer commitment from this point. |

Pre-1.0 releases are **breaking-change permitted between minor versions** but every break is documented in CHANGELOG with migration notes.

---

## What goes in shared skills (universal, not Go-specific)

The kit is Go-only, but several patterns transfer to Laravel and any future MCP server:

| Pattern | Lives in | Universal? |
|---|---|---|
| Cycle methodology (phased E2E, audit tiers, real-client gates) | `mcp-kit/docs/cycle-methodology.md` and `haakco-mcp-server-design` skill | ✅ Yes — Laravel servers should follow the same cycle |
| Lessons-learned IDs (TQ-, OG-, FP-, PR-, EG-) | `mcp-kit/docs/lessons.md` and `haakco-mcp-server-design` skill | ✅ Yes — categories transfer; specifics are server-specific |
| Tool naming conventions (action-oriented snake_case, scope prefixes) | `haakco-mcp-server-design` skill | ✅ Yes |
| Stable response contract (`{success, message, nextAction}`) | `haakco-mcp-server-design` skill | ✅ Yes |
| Discovery via prompts and tools | `haakco-mcp-server-design` skill | ✅ Yes |
| OAuth signing-key rotation cadence (90d/48h) | `mcp-kit/oauth/keys/` and `haakco-mcp-server-design` skill | ✅ Concept yes, implementation Go-only |
| PKCE base64url substitution rule | `mcp-kit/cliauth/pkce.go` and `haakco-mcp-server-design` skill | ✅ Yes — same rule in any language |
| Streamable HTTP 3-step handshake | `haakco-mcp-server-design` skill, `haakco-mcp-plugins` skill | ✅ Yes |
| `authorization_servers` = issuer URL (not metadata URL) | `haakco-mcp-server-design` skill | ✅ Yes |
| Rebuild-before-filing-SDK-bugs | `haakco-mcp-server-design` skill | ✅ Yes |

The skills get the universal content. The kit gets the Go implementation. Laravel servers reference the skills, not the kit.

---

## Open questions

1. **Storage abstraction depth.** Default Ent adapter is fine for the three known consumers. If a fourth consumer wants Postgres-with-sqlc or pure `database/sql`, the storage interface needs to be slightly more abstract. Defer until a real demand surfaces — YAGNI.
2. **Login form.** skills-mcp ships HTML login/signup/reset/verify pages. Should the kit ship those, or just the OAuth endpoints and let the consumer provide pages? **Tentative answer:** kit ships *no* HTML — it provides JSON endpoints and lets the consumer render whatever UI they want. skills-mcp keeps its existing HTML in-app. Reduces kit scope and lets each consumer match their own design system.
3. **OIDC vs OAuth-only.** skills-mcp serves `/.well-known/openid-configuration` (full OIDC); Vorrent serves `/.well-known/oauth-authorization-server` (OAuth-only). MCP protocol officially needs only the OAuth-only endpoint. **Tentative answer:** kit ships both endpoints serving the same metadata. Real OIDC features (id_token issuance, userinfo) are out of scope for v1.
4. **Multi-tenancy.** None of the three consumers is multi-tenant today. If meridian's eventual multi-tenancy phase 2 lands first, the kit may need a tenant-scoping interface. Defer until that plan is in flight.

---

## Decision log

| Date | Decision | Rationale |
|---|---|---|
| 2026-05-01 | Use official SDK, not mark3labs | Anthropic-maintained; ecosystem direction; Vorrent's lessons already proven against it |
| 2026-05-01 | Storage interface (with default Ent impl), not Ent-locked | Flexibility for future non-Ent consumer at low cost |
| 2026-05-01 | No HTML login pages in kit | Out of scope; each consumer renders their own UI |
| 2026-05-01 | RS256-signed JWTs, not opaque tokens | Both reference servers; key rotation works cleanly with JWS |
| 2026-05-01 | Skills-mcp migrates first (largest delta) | Most of the OAuth gravity originates there; proves the kit against a complex consumer |
| 2026-05-01 | Cycle methodology in skills + kit docs (universal); kit code Go-only | HaakCo runs MCP servers in non-Go languages too (Laravel) |
