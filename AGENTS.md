# Agent instructions

This repository is developed by agents under a spec-anchored, increment-based workflow.
Read this file first; it tells you where everything lives and what the rules are.

Every path in this file is relative to the repository root, whichever filename you are reading
this under — `AGENTS.md`, `CLAUDE.md`, `GEMINI.md` and `.github/copilot-instructions.md` are the
same file.

## Match ceremony to the work — read this before anything else

The most common failure of this way of working is applying the full process to work that
does not need it. Choose a track deliberately:

| Track | When | Process |
|---|---|---|
| **Direct** | You could describe the diff in one sentence. Typos, log lines, renames, obvious one-line fixes. | Just do it. Commit. No increment, no spec. |
| **Increment** | A change with a testable outcome and some blast radius. **The default.** | One increment in the ledger → build → verify. |
| **Spec** | Multi-increment features, anything user-facing and new, anything regulated or contractual. | Product spec → plan → increments. |

If you are unsure, ask which track rather than defaulting to the heaviest. Generating a
thousand lines of markdown for a date-format change is a documented, measured failure mode,
not a sign of rigour.

## The three planes

| Plane | Lives in | Lifetime | Changes via |
|---|---|---|---|
| **Constitution** — non-negotiable rules | `docs/constitution.md` | Years | Human decision only |
| **Specs** — what we intend to build | `specs/product.md`, `specs/increments/` | Per feature | `write-product-spec`, `plan-increments` |
| **Ledger** — state of the work | `specs/increment-plan.md` | Continuous | `track-increment-state` only |

Never blur them. Rules do not go in the ledger; status does not go in the constitution.

## The loop

```
assess-codebase  →  write-product-spec  →  plan-increments  →  [GATE 1: human approves plan]
                                                                      ↓
                          [GATE 2: human approves ship]  ←  verify-increment  ←  implement-increment
                                                                      ↑                    │
                                                                      └────── one at a time ┘
```

Two human gates, deliberately. Gate 1 controls scope before any code exists (the cheapest place
to say no). Gate 2 controls release. Do not add a third gate — mid-review gates approve almost
everything and just cost time.

## Hard rules

1. **One increment in progress at a time.** `Status: in-progress` may appear at most once in the ledger.
2. **Implement from the increment, not from the conversation.** If it is not in the increment's Scope or
   Acceptance Criteria, it does not get built. Found something else? Add a new increment.
3. **The ledger is the truth.** Session memory is not. Before acting, read `specs/increment-plan.md`.
   After acting, update it and run `python3 .sdlc/bin/validate-plan.py`.
4. **Every status change is a commit.** State lives in git, not in your head.
5. **No increment is `done` without evidence** a human can check at a glance — test output, a diff,
   a screenshot — recorded in the increment file.
6. **Verification runs in a fresh context.** The agent that wrote the code does not grade it.

## Acceptance criteria use EARS

Write every acceptance criterion in Easy Approach to Requirements Syntax, because it is
unambiguous and translates directly into a test:

```
WHEN <trigger> THE SYSTEM SHALL <observable response>
WHILE <state> THE SYSTEM SHALL <observable response>
IF <condition> THEN THE SYSTEM SHALL <observable response>
THE SYSTEM SHALL <invariant>                      (unconditional)
```

`WHEN a search query contains a single quote THE SYSTEM SHALL return matching rows without
error` is testable. "Search handles special characters properly" is not.

## Git conventions

- Branch per increment: `inc/<id>-<slug>` (e.g. `inc/sec-001-parameterize-search`)
- Commit trailer: `Increment: <id>` — traceability tooling depends on this
- One increment per PR

## Skills

Skills live in `skills/` and follow the Agent Skills open standard. Run
`bash .sdlc/bin/link-agents.sh` after cloning — see Portability below.

| Skill | Use when |
|---|---|
| `assess-codebase` | You need findings about the current state (security, performance, quality, modernization) |
| `write-product-spec` | Turning an idea or brief into a reviewable product spec |
| `plan-increments` | Turning a spec or findings into ordered, shippable increments |
| `implement-increment` | Building exactly one increment |
| `verify-increment` | Proving an increment meets its acceptance criteria |
| `track-increment-state` | Any read or write of increment status — the ledger's only editor |
| `author-skill` | Adding or fixing a skill in this repo |

## Portability

This repository is deliberately vendor-neutral, because we expect to be multi-cloud
([ADR-0001](docs/decisions/0001-vendor-neutral-agent-tooling.md)).

There is one copy of everything: **`AGENTS.md`** is the only entrypoint and **`skills/`** the only
skill definitions. Every vendor-specific path — `CLAUDE.md`, `GEMINI.md`, `.claude/skills/`,
`.github/skills/`, `.github/copilot-instructions.md` — is a **generated symlink** back to those,
created by `.sdlc/bin/link-agents.sh` and gitignored.

Three rules follow, and they matter:

1. **Never edit a symlinked path.** Edit `AGENTS.md` or `skills/`. Editing `CLAUDE.md` edits
   `AGENTS.md` — which is fine — but writing a *new* file at a vendor path forks the instructions
   and nothing will detect the drift.
2. **Never author agent-specific instructions.** If a rule is worth stating, it is worth stating
   once, here. Agents that need a different path get a symlink, not a different document.
3. **Enforcement lives in git hooks**, not in any agent's configuration. `.claude/settings.json`
   exists as a convenience for faster feedback in one editor; `.sdlc/hooks/pre-commit` is what
   actually gates the repository, and works regardless of who is driving.

Supporting another agent is one line in `link-agents.sh`. Dropping one is one line.

## Checks

```bash
python3 .sdlc/bin/validate-plan.py       # ledger is well formed
python3 .sdlc/bin/validate-skills.py     # skills match the Agent Skills spec
python3 .sdlc/bin/check-traceability.py  # ledger matches what is actually in git
```

These are enforced by hooks, not by good intentions — see `.sdlc/hooks/`.
