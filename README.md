# Agentic SDLC template

A minimal, reusable scaffold for building products with coding agents, where **the state of the
work lives in git** rather than in a chat session.

Copy this directory into a new repository and start. Everything here is markdown and four
dependency-free Python scripts.

## Quick start

```bash
cp -r /path/to/template/. ~/code/my-product/
cd ~/code/my-product
bash .sdlc/bin/init.sh "my-product"
```

`init.sh` initialises git if needed, generates the agent symlinks, installs the pre-commit hook,
removes the worked example, names the project, and runs all three validators. It is safe to
re-run and changes nothing that is already correct.

It then prints the two steps that need a human: filling in `docs/constitution.md`, and working
`chore-001` — committing the scaffold with `Increment: chore-001` in the trailer, which runs the
whole loop once on the workflow itself.

## What is in here

```
AGENTS.md                    entrypoint — tracks, rules and the map (CLAUDE.md symlinks to it)
docs/constitution.md         non-negotiable rules; changes only by human decision
docs/decisions/              ADRs — durable decisions and their rationale
docs/architecture.md         current state of the system, as built
specs/product.md             what we intend to build          ← write-product-spec
specs/assessments/           findings about existing code     ← assess-codebase
specs/increment-plan.md      THE LEDGER — state of all work   ← track-increment-state
specs/increments/            per-increment record + evidence
skills/                      seven skills, one per stage of the loop
.sdlc/templates/             increment, assessment, spec templates
.sdlc/bin/validate-plan.py   checks the ledger mechanically
.sdlc/bin/check-traceability.py  checks the ledger against git and the spec
.sdlc/bin/validate-skills.py checks skills against the Agent Skills spec
.sdlc/bin/new-increment.py   scaffolds an increment with the next id
.sdlc/bin/init.sh            one-command bootstrap for a new repo
.sdlc/bin/link-agents.sh     generates vendor symlinks from AGENTS.md and skills/
.sdlc/hooks/pre-commit       deterministic gate — runs all three checks
```

## Match ceremony to the work

Choose a track before starting. `AGENTS.md` states this first, before any other instruction,
because applying the full process to small work is the most common way this goes wrong:

| Track | When | Process |
|---|---|---|
| **Direct** | You could describe the diff in one sentence | Just do it. Commit. |
| **Increment** | A change with a testable outcome and blast radius. The default. | Ledger → build → verify |
| **Spec** | Multi-increment, user-facing, regulated, contractual | Spec → plan → increments |

## The loop

```
assess-codebase ─┐
                 ├─→ plan-increments ─→ [GATE 1] ─→ implement-increment ─→ verify-increment ─→ [GATE 2]
write-product-spec ┘                                        ↑                      │
                                                            └──── one at a time ────┘
```

Two human gates. Gate 1 approves the plan before code exists — the cheapest place to say no.
Gate 2 approves the ship. Agents can move work to `in-review`; only a human marks it `done`.

## The three planes

Separate them by lifetime, and never blur them. The repository is the single source of truth for
work state ([ADR-0002](docs/decisions/0002-repository-is-the-source-of-truth.md)) — if work is
also tracked on a board, the board is a projection, written second.

| Plane | File | Lifetime | Answers |
|---|---|---|---|
| Constitution | `docs/constitution.md` | Years | What are the rules? |
| Specs | `specs/product.md`, `specs/increments/` | Per feature | What are we building? |
| Ledger | `specs/increment-plan.md` | Continuous | Where are we? |

## The increment

The ledger holds one block per increment. This is the whole format:

```markdown
## sec-001: Parameterize the search query

- **Type:** security
- **Status:** approved
- **Tier:** 2 (High)
- **Traces:** SEC-007
- **Scope:** Parameterize the SQL query in `SearchService.search()`. No other changes.
- **Acceptance Criteria:**
  - [ ] WHEN a search query contains SQL metacharacters THE SYSTEM SHALL treat them as literal text
  - [ ] THE SYSTEM SHALL return identical results for the queries in `tests/fixtures/queries.json`
- **Test Strategy:**
  - Injection test: malicious input is safely escaped
  - Full regression suite
- **Dependencies:** none
- **Rollback Plan:** Revert `SearchService.search()` to the previous implementation
- **Risk:** Low — isolated change to one method
- **Evidence:** _(filled in at verify)_
```

Three fields carry the weight:

- **Scope** ends with what does *not* change, which is what keeps an agent inside the increment.
- **Rollback Plan** must fit on one line. If it doesn't, split the increment.
- **Acceptance Criteria** use EARS — `WHEN <trigger> THE SYSTEM SHALL <observable response>`,
  or `WHILE …`, `IF … THEN …`, or a bare invariant. Each names a trigger and an observation, so
  it translates straight into a test.

Status moves `planned → approved → in-progress → in-review → done`, and each transition is its
own commit. Use `python3 .sdlc/bin/new-increment.py "<title>" --type <type> --tier <1-4>` to
scaffold one with the next id.

## The checks

The ledger is only useful if it is true. `validate-plan.py` enforces:

- unique, well-formed increment ids
- required fields present, statuses and types legal
- **at most one increment `in-progress`** — the WIP limit is the main brake on an agent
  half-finishing four things
- dependencies resolve, and no cycles
- an increment cannot be ahead of a dependency that is not `done`
- nothing reaches `done` with unticked criteria or empty evidence
- template placeholder text is never left unfilled

`check-traceability.py` covers the gap between the paperwork and the repository:

- every increment claimed `in-review` or `done` has commits carrying its `Increment: <id>` trailer
- every Must requirement in `specs/product.md` is traced by at least one increment
- no increment traces to a requirement or finding that does not exist
- no orphan commits claiming increments the ledger has never heard of

`validate-skills.py` checks skills against the Agent Skills spec: name/directory mismatch, a
description too vague to route on, an over-long body, a broken reference link.

All three exit non-zero and run from `.sdlc/hooks/pre-commit`, so a dishonest ledger cannot be
committed. Instructions in a markdown file are advisory; a hook is not.

## Verification runs in a fresh context

The agent that wrote the code does not grade it. `verify-increment` is meant to run in a new
session or a subagent that sees only the diff and the acceptance criteria — not the reasoning
that produced the change. It writes a **walkthrough**: a short, human-checkable account of the
evidence, with screenshots for anything user-facing. Reviewing a walkthrough is far faster than
reading raw logs, and far more reliable than trusting an assertion that it works.

The skill is also calibrated against over-reporting, since a reviewer told to find problems will
find some whether or not they exist.

## Portability

Deliberately vendor-neutral, for multi-cloud — see
[ADR-0001](docs/decisions/0001-vendor-neutral-agent-tooling.md).

There is **one copy of everything**: `AGENTS.md` is the only entrypoint, `skills/` the only skill
definitions. Both are open standards — `AGENTS.md` is stewarded by the Agentic AI Foundation
under the Linux Foundation (co-founded by Anthropic, OpenAI, Google, AWS and Microsoft), and
[Agent Skills](https://agentskills.io/specification) is adopted across Claude Code, Copilot,
Cursor, Codex and Gemini CLI.

Every vendor path is a **generated symlink** back to those, created by `link-agents.sh` and
gitignored:

| Agent | Reads | Points at |
|---|---|---|
| Claude Code | `CLAUDE.md`, `.claude/skills/` | `AGENTS.md`, `skills/` |
| GitHub Copilot | `.github/copilot-instructions.md`, `.github/skills/` | `AGENTS.md`, `skills/` |
| Gemini CLI | `GEMINI.md` | `AGENTS.md` |
| Codex, Cursor, Antigravity | `AGENTS.md` | — reads it directly |

Supporting another agent is one line in `link-agents.sh`. Dropping one is one line. Because they
are symlinks rather than copies, per-agent drift is impossible rather than merely discouraged.

Enforcement follows the same rule: `.sdlc/hooks/pre-commit` gates the repository for everyone,
and `.claude/settings.json` is a convenience for faster feedback in one editor — never the only
thing standing between a bad ledger and a commit.

## Extending it

Add a skill when a real failure justifies one, not in anticipation. Use `author-skill`, which
encodes the standard and the checks. Likely candidates: `resolve-tech-stack` (stack choices as
ADRs), `generate-tests` (if your test conventions are non-obvious), `run-e2e` (if browser
automation needs specific setup).

Do not add an assessment skill per lens. `assess-codebase` takes a lens and loads only that
lens's reference file, which is the same coverage at a fraction of the context.
