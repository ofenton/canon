# 0006 — Distributing the template

**Status:** proposed — needs a decision
**Date:** 2026-08-24

## Context

The agentic SDLC template is distributed by copying: *"Copy this directory into a new repository
and start."* Each project receives its own `AGENTS.md`, its own `skills/`, its own `.sdlc/bin/`
validators and its own hook.

Canon exists to argue that **per-project configuration diverges, and the divergence destroys the
one thing an org-wide system exists to provide**. The template's distribution model is per-project
configuration. We shipped the disease as the delivery mechanism for the cure.

## The measurement, and its honesty

After one week and one project, comparing this repository against the template it came from:

```
.sdlc/bin/validate-plan.py         identical
.sdlc/bin/check-traceability.py    identical
.sdlc/bin/check-tracked.py         identical
.sdlc/bin/link-agents.sh           identical
skills/  (6 of 8)                  identical
.sdlc/hooks/pre-commit             2 lines differ
AGENTS.md                          6 lines differ
skills/verify-increment            30 lines differ
.sdlc/bin/lint-workflows.py        exists here, absent from the template
```

**Being precise: the drift so far is small, and most of it is one-directional and deliberate** —
the template gained `design-architecture` and `check-architecture.py` today and this repository had
not yet received them. That is a template moving forward, not two copies rotting.

The one interesting case is `lint-workflows.py`. It was written here, in response to a real failure
— a malformed GitHub Actions file that failed remotely with no logs — and the template never got
it. **That is genuine divergence: an improvement made in a copy, invisible to every other copy.**
One instance after one week, by one person who wrote both.

The argument that this becomes serious is extrapolation, not measurement. It is the same
extrapolation Canon's own product thesis rests on — the 700th custom field is not the problem, the
mechanism that produced it is — so it would be inconsistent to accept it there and reject it here.
But it should be labelled as what it is: a prediction, with one data point.

## What "template" is actually doing

Three different things are bundled into one copied directory, and they have different needs:

| Part | What it is | Should it diverge? |
|---|---|---|
| `AGENTS.md`, `skills/` | How agents work | **No.** An improvement to `verify-increment` should reach every project. |
| `.sdlc/bin/*.py`, hooks | Mechanical enforcement | **No**, and worse: a project on an old validator enforces old rules while believing it enforces current ones. |
| `specs/`, `docs/constitution.md` | The project's own content | **Yes.** This *is* the project. |

The copy model gets the third right and the first two wrong, because it treats all three as one
artifact.

## Options

### A. Keep copying, and accept the drift

- **For:** zero infrastructure. A project can be modified freely and works offline. Forking a
  template is what every scaffolding tool does.
- **Against:** improvements do not propagate, `lint-workflows.py` being the first instance; a
  project cannot tell whether it is current; a rule fixed in the template stays broken everywhere
  else. And it contradicts the product this template was used to build.

### B. Distribute as a versioned dependency

`.sdlc/` and `skills/` come from a pinned release, fetched rather than copied. The project owns
only `specs/`, `docs/` and its own `AGENTS.md` additions.

- **For:** a project can state which version it is on; an improvement is a version bump; a fix
  reaches every project that upgrades. Drift becomes visible as a version number rather than
  invisible as a diff.
- **Against:** needs a distribution mechanism and a fetch step; offline work needs a vendored copy
  anyway; a version bump can break a project, so somebody has to own compatibility.

### C. Distribute the rules from Canon; keep the prose local

The mechanical rules — legal statuses, types, WIP limits, required fields — come from the org's
`canon.yaml` rather than being hardcoded in each copy of `validate-plan.py`. The validator becomes
a thin client of the schema. Skills and `AGENTS.md` are still copied, because prose that cannot be
locally adapted is prose people work around.

- **For:** puts the rules in exactly one place, which is the product's whole argument, applied to
  itself. `validate-plan.py` stops encoding policy in Python.
- **Against:** committing now depends on the schema being reachable, unless it is cached. Only
  covers the mechanical part; skills still drift. Couples the template to Canon, which is a
  significant thing to do to a tool whose selling point is that it needs nothing.

## Decision

**B for the mechanism, C for the rules, and the two compose.**

`.sdlc/` and `skills/` become a pinned, versioned artifact rather than a copy — so an improvement
propagates and a project can say what it is running. Within that, the *values* the validators
enforce come from the org's schema rather than from constants in Python, so policy lives where the
product says policy should live.

Deliberately **not** doing: making the hook require a network call. A schema fetched at commit time
is a commit that fails on a train. The schema is cached locally and refreshed explicitly, which
means a stale cache is possible and visible, rather than an outage being possible and surprising.

## Consequences

- `validate-plan.py` stops holding `STATUSES` and `TYPES` as literals and reads them from the
  schema, with the current values as the fallback when no schema is configured.
- The template gains a version, and a project can be asked what it is on.
- **This repository is the first case.** Canon uses the template *and* is the thing the template
  would depend on, which is circular and needs a straight answer before either half is built.
- Nothing here is urgent. One instance of real drift in one week is not a fire, and the honest
  sequence is to fix [ADR-0005](0005-where-work-lives-git-or-canon.md) first: if the centre's job
  changes, what the template should depend on changes with it.

## Open questions for the decision

1. **What is the artifact?** A git submodule, a released tarball, a package, or a `canon` subcommand
   that materialises `.sdlc/`. Each has a different failure mode when offline.
2. **Can a project override a rule?** If yes, this is the copy model with extra steps. If no, a
   project with a genuine exception is stuck — and "we have a genuine exception" is what every team
   in every Jira instance said.
3. **Does the template depend on Canon, or merely offer to?** A template that requires a running
   tracker to commit is a much heavier thing than the one we have.
4. **Who owns compatibility?** A version bump that changes the ledger format has to be somebody's
   problem before it is forty projects' problem.
