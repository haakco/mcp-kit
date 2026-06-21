# mcp-kit Plans Index

Active and archived implementation plans for `mcp-kit`.

## Active

| Plan | Status | Goal |
|---|---|---|
| [2026-05-01_mcp-kit_master_plan.md](2026-05-01_mcp-kit_master_plan.md) | Draft (v0.1.0 spike landed; awaiting review) | Take mcp-kit from v0.1.0 skeleton to v1.0.0 stable across three Go consumers (skills-mcp, vorrent, meridian) |

## Archive

| Plan | Completed | Result |
|---|---|---|
| [archive/2026-06-21-2026-06-18_oauth_mcp_defaults.md](archive/2026-06-21-2026-06-18_oauth_mcp_defaults.md) | 2026-06-21 | OAuth token defaults, bearer challenge hints, and protected-resource metadata moved into `mcp-kit`; Skills upgraded and rolled out on `v1.1.21`. |

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
