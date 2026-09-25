# mcp-kit Plans Index

Active and archived implementation plans for `mcp-kit`.

## Active

| Plan | Status | Goal |
|---|---|---|
| [2026-09-25_mcp-2026-07-28-protocol-migration.md](2026-09-25_mcp-2026-07-28-protocol-migration.md) | Draft | Move the kit onto MCP protocol revision 2026-07-28: go-sdk v1.8.0, stateless Streamable HTTP, `server/discover`, real SDK-backed test fixtures in place of the fabricated `2025-03-26` ones, the new error-code partition, and the OAuth changes (`iss`, `application_type`, Client ID Metadata Documents) |
| [2026-07-06_aoa_security_conformance_hardening.md](2026-07-06_aoa_security_conformance_hardening.md) | Draft | Borrow useful security and conformance patterns from `aoa`/`aoa-conformance` without replacing `mcp-kit`'s OAuth issuer and middleware architecture |
| [2026-05-01_mcp-kit_master_plan.md](2026-05-01_mcp-kit_master_plan.md) | In progress (phases 1-8 and 10 complete; 9, 11, 12 open) | Take mcp-kit from v0.1.0 skeleton to v1.0.0 stable across three Go consumers (skills-mcp, vorrent, meridian) |

## Archive

| Plan | Completed | Result |
|---|---|---|
| [archive/2026-06-21-2026-06-18_oauth_mcp_defaults.md](archive/2026-06-21-2026-06-18_oauth_mcp_defaults.md) | 2026-06-21 | OAuth token defaults, bearer challenge hints, and protected-resource metadata moved into `mcp-kit`; Skills upgraded and rolled out on `v1.1.21`. |
| [archive/2026-05-02-2026-05-02_oauth-consent-helpers.md](archive/2026-05-02-2026-05-02_oauth-consent-helpers.md) | 2026-05-02 | Shared `oauth/consent` authorize-endpoint handler landed with `hmacstore` and `sessionstore` approval-token backends plus `consenttest` fixtures; documented in `docs/recipes/oauth-consent.md`. |

## Plan format

All plans follow the HaakCo plan template from `~/.claude/CLAUDE.md`:

- **Goal** — one sentence
- **Background** — context
- **Architecture** — 2-3 sentences
- **Tech Stack** — key dependencies
- **Parallel Work Model** — concurrent teams + shared-branch rules
- **Current State (Verified)** — file paths actually opened, not assumed
- **Tasks** — bite-sized steps with verify commands

When a plan completes, move it to `archive/<archive_date>-<original_filename>.md` and update this index.
