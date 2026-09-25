# Conformance

`mcp-kit` ships middleware, not tools. That split decides how conformance is used and what each half is responsible
for proving.

| Owner | Owns | Proved by |
|---|---|---|
| `mcp-kit` | Streamable HTTP envelope, Origin allowlist, bearer auth, OAuth, discovery, key rotation | `testkit/contract_test.go`, `oauth/*_test.go`, and the `dns-rebinding-protection` scenario |
| Consumer server | Tools, resources, prompts, their schemas, and their results | The full 2026-07-28 requirement set against a deployed server |

## One command

```bash
just conformance                                   # SDK reference server, proves the gate itself
just conformance-against http://localhost:8080/mcp # a real server
```

Equivalent, without `just`:

```bash
./scripts/conformance/run.sh
CONFORMANCE_URL=http://localhost:8080/mcp ./scripts/conformance/run.sh
```

With `CONFORMANCE_URL` unset the runner builds the MCP Go SDK's own reference server, starts it stateless, and runs
the suite against it. That run is a smoke test of the gate: it proves the pinned CLI and revision profile still agree
with each other, so a silent upstream change cannot hollow out the check.

## The pinned CLI

`CONFORMANCE_VERSION` defaults to `0.2.0-alpha.11`.

The version is not cosmetic. **`0.1.16` does not know the `2026-07-28` revision at all** and answers
`Unknown spec version: 2026-07-28`. Only the `0.2.x` line implements it, and only the `0.2.x` line accepts
`--requirements`. Do not downgrade the pin without re-checking both facts.

```bash
# What the runner executes, in full:
npx --yes @modelcontextprotocol/conformance@0.2.0-alpha.11 server \
  --url "$TARGET" \
  --requirements 2026-07-28 \
  --expected-failures conformance-baseline.yml
```

`--requirements 2026-07-28` selects exactly the scenarios that revision requires, frozen at its release, and replaces
`--suite`/`--spec-version`. Use the latter only to explore:

```bash
npx --yes @modelcontextprotocol/conformance@0.2.0-alpha.11 list --server --spec-version 2026-07-28
npx --yes @modelcontextprotocol/conformance@0.2.0-alpha.11 server --url "$TARGET" --spec-version 2026-07-28 --suite active
```

The CLI separates suites: `active` (default), `pending`, `draft`, and `all`. `caching`, `http-header-validation`,
`http-custom-header-server-validation`, `json-schema-2020-12`, `sep-2164-resource-not-found`, `server-stateless`, and
every `input-required-result-*` (Multi Round-Trip Request) scenario are `pending` but **are** part of the 2026-07-28
requirement set, so `--requirements` includes them. Running `--suite active` alone silently skips them.

## Baseline policy

`conformance-baseline.yml` is passed as `--expected-failures` and is **empty**.

Extension scenarios are not the baseline's business: the CLI excludes the `tasks-*` family from the 2026-07-28 score
automatically and prints them under "Not scored for 2026-07-28". They must not be listed.

Adding an entry is a deliberate act. Every entry needs a reason, an owner, and the condition under which it is removed.
An unexplained baseline entry is how a conformance gate stops meaning anything.

Verify a new entry actually belongs there:

```bash
# Should fail loudly, naming the unexpected scenarios:
/tmp/stateful-server -http=127.0.0.1:9000 -stateless=false &
CONFORMANCE_URL=http://127.0.0.1:9000 ./scripts/conformance/run.sh   # exit 1
```

## What the kit does not implement

- **Multi Round-Trip Requests** (`input-required-result-*`). MRTR lets a tool handler ask the client for more input
  mid-call. It is a consumer feature — a kit with no tools has nothing to return `input-required` for. The scenarios
  pass against the SDK reference server, which implements the diagnostic tools.
- **Tasks** (`tasks-*`). A separate extension, not part of the 2026-07-28 requirement set.
- **DPoP, token exchange, and mTLS.** Deferred; no verified consumer needs them, and the core revision does not
  require them.

## CI

`.github/workflows/ci.yml` runs the gate on changes that touch Go files, and uploads the raw results as an artifact so
a failure can be diagnosed from the log and the JSON alone. Documentation-only changes skip the Go and conformance
suites.

## Related

- `docs/lessons.md` — `TQ-02`, `TQ-04` through `TQ-07` record the transport behaviours the suite pins.
- `docs/migration/new-server.md` — how to build a server that passes.
