# 0001 — Vendor-neutral agent tooling

**Status:** accepted
**Date:** 2026-08-22

## Context

Every major vendor now ships an agentic development workflow, and each brings its own
directory layout: `.github/skills/` and Spec Kit's `constitution.md`/`spec.md`/`plan.md`,
`.kiro/steering/` and `.kiro/specs/<feature>/`, `.claude/skills/`, Antigravity's workspace.

Adopting one vendor's layout makes that vendor's tooling work out of the box and everything
else a translation problem. The pull toward doing so is real: an organisation standardising on
a single cloud gets a coherent story, and the vendor's defaults stop being decisions.

We expect to be multi-cloud. A layout that is only legible to one vendor's tools becomes a
migration cost precisely when we are least able to absorb it.

## Decision

Build on the open standards, and treat vendor paths as generated views.

- **`AGENTS.md`** is the single agent entrypoint. It is stewarded by the Agentic AI Foundation
  under the Linux Foundation, co-founded by Anthropic, OpenAI, Google, AWS and Microsoft, and
  read natively by Codex, Cursor, Copilot, Gemini CLI and Antigravity.
- **Agent Skills (`SKILL.md`)** is the single skill format. It is an open specification adopted
  across Claude Code, Copilot, Cursor, Codex and Gemini CLI.
- **Vendor paths are symlinks, never copies.** `.sdlc/bin/link-agents.sh` holds the full list;
  supporting another agent is one line, and dropping one is one line.
- **Enforcement is a git hook**, not an agent's configuration. `.sdlc/hooks/pre-commit` runs the
  validators. Agent-native hook configuration (`.claude/settings.json`) is a convenience that
  gives faster feedback, never the only thing standing between a bad ledger and the repository.
- **State is plain markdown in git**, readable without any tool at all.

## Consequences

**Good.** Any agent that reads `AGENTS.md` and `SKILL.md` works here on day one. Skills are
authored once. Switching or adding an agent is a symlink change, not a migration. State survives
every vendor in the chain, because it is markdown in git.

**Costs.** We do not get a vendor's spec workflow out of the box, and we maintain our own thin
scaffolding — currently four Python scripts with no dependencies. Where a vendor generates its
own layout by default, we either accept the mismatch or bridge it.

**Bridging.** If a specific vendor's tooling later becomes load-bearing, add a thin adapter
that points that vendor's expected paths at ours. That keeps this repository the source of truth
and confines the coupling to one file.

## Alternatives considered

**Adopt a single vendor's layout.** Rejected: it converts a future multi-cloud move into a
rewrite of everything agents read, for a convenience we can get from symlinks.

**Support every vendor by copying the files.** Rejected: duplicated instructions drift, and
nothing detects it. Symlinks make drift impossible rather than merely discouraged.
