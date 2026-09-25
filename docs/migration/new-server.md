# Building a new server on `mcp-kit`

This is the shortest path to a working MCP server on revision **2026-07-28**. It assumes the kit owns transport,
OAuth, discovery, and key rotation, and that you own the SDK `mcp.Server`, its tools, and your user data.

The kit is modern-only: one protocol revision, no sessions, no `initialize` handshake. If you need the legacy
lifecycle, this kit is the wrong starting point.

## 1. Add the dependency

```bash
go get github.com/haakco/mcp-kit@v0.6.0
go get github.com/modelcontextprotocol/go-sdk@v1.8.0
```

Go 1.27 or newer is required.

## 2. Build the SDK server

The consumer owns the `mcp.Server`. Two settings are not optional for 2026-07-28:

```go
sdkServer := mcp.NewServer(
    &mcp.Implementation{Name: "my-server", Version: "1.0.0"},
    &mcp.ServerOptions{
        Instructions: "...",
        // Explicit empty capabilities. Without this the SDK advertises the
        // historical default logging capability, which 2026-07-28 deprecated.
        // Tool/resource/prompt capabilities are still inferred from handlers.
        Capabilities: &mcp.ServerCapabilities{},
        // Serve exactly one revision. An empty list serves every legacy
        // revision too, which you do not want and cannot test.
        SupportedProtocolVersions: []string{mcpkit.ProtocolVersion},
        // MCP defaults an absent cacheScope to "public". That is wrong for a
        // server that authenticates callers.
        SetCacheable: mcpkit.PrivateCache(mcpkit.DefaultCacheTTL),
    },
)
mcp.AddTool(sdkServer, &mcp.Tool{Name: "hello_world", Description: "Greet."}, handleHello)
```

Only override the cache policy for a result you can prove is identical and safe for every caller. `public` on an
authenticated list method leaks one user's view to another.

## 3. Serve it statelessly

```go
handler := mcp.NewStreamableHTTPHandler(
    func(*http.Request) *mcp.Server { return sdkServer },
    &mcp.StreamableHTTPOptions{
        // Mandatory: a non-stateless handler rejects 2026-07-28 outright.
        Stateless: true,
    },
)
```

With `Stateless: true`:

- `POST` is the only accepted method; `GET` and `DELETE` return `405` with `Allow: POST`.
- `Mcp-Session-Id` is neither required nor emitted.
- The server cannot make client requests (sampling, elicitation, roots) — there is no live session to answer.

## 4. Wrap it with the kit

Middleware order is fixed: **Origin → Bearer → Envelope → SDK handler**. Origin runs first so a rejected browser origin
never triggers a token lookup, and the Envelope never sees a 401 or 403.

```go
kitServer, err := mcpkit.New(mcpkit.Config{
    Handler:        handler,
    AllowedOrigins: []string{"https://app.example.com"},
    AllowLoopback:  !isProduction, // http://localhost:* and http://127.0.0.1:* origins
    Bearer: mcpkit.BearerConfig{
        Introspector:        provider.OAuth2Provider(),
        TokenValidator:      patValidator,
        ResourceMetadataURL: protectedResourceMetadataURL,
        RequiredScopes:      []string{"mcp.read"},
        ExpectedAudience:    resourceURL,
    },
})
if err != nil {
    return err
}
mux.Handle("/mcp", kitServer.Handler())
```

Never pass `AllowUnauthenticated: true` in production. When it, and only it, disables auth, the kit marks the request
with an explicit sentinel (`oauth.WithAuthDisabled`) rather than leaving scopes empty — the distinction is what keeps
[`AG-03`](../lessons.md) from recurring.

## 5. Mount OAuth and discovery

```go
provider.RegisterRoutes(mux, "/oauth", resolveSubject)

discovery := oidc.NewDiscoveryConfig(issuerURL, []string{"mcp.read"})
discovery.RegisterRoutes(mux, oidc.RouteConfig{
    ResourceURL: resourceURL,
    JWKS:        oidc.JWKSHandler(keyManager),
})
```

`oauth/consent` owns the authorize endpoint's user-facing half: authenticate the browser session, show consent, then
render the redirect. The bare `Provider.AuthorizeHandler` is a demo that grants whatever the resolver returns.

## 6. CORS

If a browser client talks to `/mcp`, allow the MCP request headers. Missing any of these breaks the request before your
handler sees it:

```text
Authorization, Content-Type, Accept,
Mcp-Protocol-Version, Mcp-Method, Mcp-Name
```

Add `Mcp-Param-*` only if a tool annotates an input property with `x-mcp-header`. Do **not** expose `Mcp-Session-Id`:
there are no sessions, and advertising the header invites clients to depend on it.

## 7. First probe

Prove discovery and the tool list before anything else. See
[`PR-02`](../lessons.md) for the exact commands. In short: POST `server/discover`, assert `supportedVersions` is exactly
`["2026-07-28"]` and no `Mcp-Session-Id` response header appears; then POST `tools/list` and check your inventory.

Common failures:

| Symptom | Cause |
|---|---|
| `400` with plain-text `Unsupported protocol version` | You sent a pre-2026-07-28 `Mcp-Protocol-Version`. See [`TQ-04`](../lessons.md). |
| `405` on a request you expected to work | You used `GET` or `DELETE`; stateless serves `POST` only. |
| `-32020` | `Mcp-Method`, `Mcp-Name`, or `Mcp-Protocol-Version` disagrees with the body. |
| `-32602` | `_meta` is missing `protocolVersion` or `clientCapabilities`. |
| `404` with `-32601` | The method was removed (`ping`, `logging/setLevel`, `resources/subscribe`) or is unknown. |
| `cacheScope` is `public` on authenticated results | `SetCacheable` is not set. |

## 8. Conformance

```bash
just conformance-against http://localhost:8080/mcp
```

See [conformance](../conformance.md) for what the suite covers and why the baseline is empty.

## Not in this kit

DPoP, token exchange, mTLS, tasks, and MCP Apps are not implemented and not planned without a concrete consumer need.
Roots, sampling, and logging are deprecated by 2026-07-28; the kit does not advertise them. Multi Round-Trip Requests
are a consumer feature — the kit has no tools to return `input-required` from.
