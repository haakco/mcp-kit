# Stateless Streamable HTTP

MCP revision **2026-07-28** removes sessions. Serving Stateless HTTP is not a tuning choice; it is the only mode that
accepts the revision over HTTP.

## The setting

```go
handler := mcp.NewStreamableHTTPHandler(
    func(*http.Request) *mcp.Server { return sdkServer },
    &mcp.StreamableHTTPOptions{Stateless: true},
)
```

A non-stateless handler rejects a 2026-07-28 request outright — the SDK logs
`this server is stateful; set StreamableHTTPOptions.Stateless = true to accept it` and returns `-32022`.

Do **not** put this on `mcpkit.Config`. The kit wraps an `http.Handler` the consumer has already built, so it cannot
change how that handler was constructed. This is the consumer's line to write.

## What stateless means on the wire

| Behaviour | Stateless |
|---|---|
| `POST` | Served; each request is self-contained |
| `GET`, `DELETE` | `405`, `Allow: POST` |
| `Mcp-Session-Id` | Neither required nor emitted |
| Client sends a stale `Mcp-Session-Id` | Ignored; the request is served on its merits |
| Server → client requests | Impossible: there is no live session to answer |
| SSE resumability (`Last-Event-ID`) | Removed for 2026-07-28; a broken stream loses the in-flight request |

Each request carries its own `_meta`:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list",
  "params": {
    "_meta": {
      "io.modelcontextprotocol/protocolVersion": "2026-07-28",
      "io.modelcontextprotocol/clientCapabilities": {}
    }
  }
}
```

Over HTTP, mirror the version in `Mcp-Protocol-Version` and the method in `Mcp-Method`. `tools/call`,
`resources/read`, and `prompts/get` also require `Mcp-Name`. Any disagreement is `-32020`.

## Consequence: no server-initiated requests

Stateless mode cannot do sampling, elicitation, or roots — all three are server→client requests, and there is no session
for the reply to arrive on. A handler that tries is rejected immediately.

If you need input from the client mid-tool-call, that is **Multi Round-Trip Requests** (MRTR): the handler returns a
result with `resultType: "input_required"` plus the requests it needs, and the client re-issues the call with the
answers. MRTR is a consumer feature — this kit has no tools, so it has nothing to return `input-required` from and does
not implement it. If a consumer adopts MRTR, keep it in the tool handler and its request state; the kit's middleware
does not need to know.

## Middleware still applies

Stateless mode does not remove the kit's middleware, and the order still matters:

```text
Origin -> Bearer -> Envelope -> SDK handler
```

Origin rejects a disallowed browser origin before any token lookup. Bearer rejects an unauthenticated request before
the Envelope or the SDK see it. The Envelope only rewrites `400` + `text/plain` responses, so it never touches a JSON-RPC
error, an auth challenge, or a `404` — see [`TQ-06`](../lessons.md) and [`TQ-07`](../lessons.md).

## Distribution

Because no request depends on prior state, any instance can serve any request. That is what makes stateless mode
attractive for autoscaling and rolling deploys: no sticky routing, no session store, no drain window. Bear in mind that
`StreamableHTTPOptions.EventStore` is meaningless here, and the SDK's `SchemaCache` exists for deployments that build a
new `mcp.Server` per request.

## Caching

Stateless makes every request answerable by any instance, which makes caching tempting and dangerous. MCP defaults an
absent `cacheScope` to `public`. Set a policy:

```go
SetCacheable: mcpkit.PrivateCache(mcpkit.DefaultCacheTTL)
```

`private` for anything that varies by caller; `public` only with evidence that the result is identical and safe for
every caller. See [`OG-*`](../lessons.md) for the OAuth side of the same reasoning.

## Checking it

```bash
just conformance-against http://localhost:8080/mcp
```

The `server-stateless` scenario covers the transport contract directly. `testkit/contract_test.go` in this repository
asserts the same behaviours against the real SDK server through the kit stack.
