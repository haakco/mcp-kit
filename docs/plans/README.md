# mcp-kit Plans Index

Active and archived implementation plans for `mcp-kit`.

## Active

| Plan | Status | Goal |
|---|---|---|
| [2026-09-25_mcp-2026-07-28-protocol-migration.md](2026-09-25_mcp-2026-07-28-protocol-migration.md) | In progress — kit tasks 1-7 locally proven on `feat/mcp-2026-07-28-migration`; consumer rollout (tasks 8-11) pending | Move the kit and both verified consumers onto MCP 2026-07-28, prove official conformance, and release v0.6.0 |

## Archive

| Plan | Closed | Result |
|---|---|---|
| [archive/2026-09-25-2026-07-06_aoa_security_conformance_hardening.md](archive/2026-09-25-2026-07-06_aoa_security_conformance_hardening.md) | Superseded 2026-09-25 | Official MCP conformance replaced the AOA runner; applicable security work moved to the active plan. |
| [archive/2026-09-25-2026-05-01_mcp-kit_master_plan.md](archive/2026-09-25-2026-05-01_mcp-kit_master_plan.md) | Superseded 2026-09-25 | Consumer inventory, protocol contract, and release target changed; applicable work moved to the active plan. |
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

When a plan completes or is superseded, move it to `archive/<archive_date>-<original_filename>.md` and update this
index with the truthful closeout state.
