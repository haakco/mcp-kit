# Contributing to `mcp-kit`

`mcp-kit` is a reusable library. Consumers mount it into their own Go services, so a public API change reaches every
one of them. Read [DESIGN.md](DESIGN.md) before changing anything exported.

## Setup

Go 1.27 (the toolchain is pinned in `mise.toml`, `go.mod`, and CI) plus [`just`](https://github.com/casey/just).

```bash
mise install        # or install Go 1.27 another way
just                # list recipes
```

## Commands

```bash
just build                  # compile every package
just test                   # go test ./...
just test-race              # go test ./... -race -count=1
just vet                    # go vet ./...
just lint-go                # fast lint suite (what you run while editing)
just lint-go-deep           # full lint suite
just lint-go-structural     # duplication, complexity, length, nesting
just vulncheck              # govulncheck, pinned in the justfile
just conformance            # MCP 2026-07-28 conformance against the SDK reference server
just quality                # build + vet + deep lint + structural lint + race
```

`just quality` is the gate CI runs. Run it once the tree is stable — not repeatedly while editing.

Useful focused runs:

```bash
go test -race -cover ./oauth/keys
go test -run TestBearerRejectsAnonymous ./oauth
go test ./_examples/minimal-server      # underscore dirs are skipped by ./...
```

## Invariants

- **Middleware order is load-bearing.** `Origin → Bearer → Envelope → SDK handler`. Origin denials must precede auth
  challenges, and the Envelope must not see a 401 or 403.
- **Never infer "auth disabled" from empty scopes.** Use the explicit
  `oauth.WithAuthDisabled` sentinel. Conflating the two has shipped a privilege-escalation bug before
  ([`AG-03`](docs/lessons.md)).
- **No domain code in the kit.** Tools, resources, prompts, user tables, and RBAC belong to consumers. If a pattern
  recurs across consumers, write a recipe in `docs/recipes/` first; promote it to code only when a third consumer needs
  it.
- **Method-per-receiver cap (~12).** When a 13th method wants to land on `*Server` or `*Provider`, prefer a separate
  handler or embedder type over padding the receiver ([`CG-02`](docs/lessons.md)).
- **Delete rather than maintain.** An unused path costs a future maintainer more than it saves.

## Tests

Test the behaviour the change is about, at the level where it lives. Prefer one fast test that proves three related
behaviours clearly over three tests that each repeat the same setup. Do not add a test that fails for the same reason as
an existing one, and do not assert implementation details — they break on every refactor and train people to delete
tests.

For the wire, use `testkit`: `testkit.Post` for raw requests, `testkit.Discover` and `testkit.ListTools` for decoded
results. Test against the real SDK handler, not a hand-written JSON-RPC stub. The kit's one exception used to be exactly
that stub; it fabricated the retired lifecycle and hid real protocol behaviour.

`docs/lessons.md` is the record of behaviours that cost someone real time. If a change makes a lesson wrong, fix the
lesson in the same change. Prefixes: `OG` OAuth, `JR` JSON-RPC envelope, `TQ` transport, `AG` authz, `CG` code quality,
`TG` tooling, `FP`/`PR` probes.

## Documentation

- `README.md` — what the kit is and how to start.
- `DESIGN.md` — the kit/consumer boundary and recorded decisions.
- `CHANGELOG.md` — every notable change, with migration notes for breaking ones.
- `docs/migration/*.md` — contracts real migrations follow. Changing the public API means updating the relevant
  migration doc in the same change.
- `docs/plans/` — dated, indexed plans. Follow `docs/plans/README.md`.

## Commits and branches

Work on a branch; do not push to `main` without review. Never `git stash`, `git reset`, or force-push `main` — inspect
the workspace and preserve unrelated changes. One coherent commit per usable boundary, with a message that says what
changed and why.

## Reporting a bug

Include the kit version, Go version, SDK version, the exact request or config, and what you expected. If it is a
protocol behaviour, say which revision and which client. `docs/lessons.md` explains why this level of detail is the
difference between a fix and a guess.
