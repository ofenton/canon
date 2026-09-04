# 0006 — Distributing the template

**Status:** accepted
**Date:** 2026-08-24, decided 2026-08-26
**Amended by:** [0009](0009-canon-as-aggregator.md), which removed the schema option C depended on.

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

**B for the mechanism. C is withdrawn — the thing it distributed no longer exists.**

### What changed since this was proposed

[ADR-0009](0009-canon-as-aggregator.md) deleted `canon.yaml` two days later. There is no org
schema, because the template *is* the schema: states, types and required fields are fixed by the
convention rather than chosen per organisation, and `/api/schema` reports them as
`"not configurable"`.

So option C — "the validators read the rules from the org's schema" — has nothing to read from.
And its consequence, *"`validate-plan.py` stops holding `STATUSES` and `TYPES` as literals"*, is
now exactly backwards: those literals are correct. They are the convention, in the file that
enforces it, with no indirection to a service that might be unreachable.

This is worth stating rather than quietly dropping. C was the more elegant option and the one that
applied the product's argument to itself. It died because the product's argument moved.

### The mechanism

`.sdlc/` and `skills/` are a versioned artifact rather than a copy — realised not as a package or a
submodule but as **a classified install**. The template already contained the classification, in
this ADR's own table asking "should this diverge?" of each part. `adopt.sh` executes that table:

| Class | Files | On every run |
|---|---|---|
| **managed** | `.sdlc/`, `skills/` | Replaced. An improvement that does not reach a project is an improvement that project does not have. |
| **seeded** | `AGENTS.md`, `specs/`, `docs/` | Written only when absent. Once it exists it is the project's, and drift is **reported, never fixed**. |
| **merged** | `.gitignore` | Template rules appended in a marked block; everything else untouched. |
| **skipped** | `README.md`, the worked example | The project's front door, and content that would appear in a real ledger as work nobody did. |

A file the project adds *inside* a managed directory is an extension, not divergence: left alone
and reported. Deleting somebody's skill to enforce a rule about skills would be the wrong trade
every time.

`.sdlc/VERSION` records the last commit that touched the template — not the repository's HEAD,
since the template lives alongside unrelated projects and a version that moves when something else
commits tells an adopting project nothing.

### Reporting rather than enforcing

Seeded files that have diverged are named on every run and left alone. That is deliberately the
same posture Canon takes to conformance, and for the same reason: the tool cannot know whether a
project's `AGENTS.md` differs because it is stale or because that project needed something. What it
*can* do is make the difference visible, which is the thing a copy could never do.

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

## The open questions, answered

1. **What is the artifact?** A directory at a git ref, installed by a script that classifies it.
   No submodule, no package, no registry. Offline behaviour is the best available: once installed,
   nothing fetches — not at commit time, not ever — and re-running the installer is the only thing
   that touches the network, if the template is remote at all.

2. **Can a project override a rule?** For managed files, no: edit a validator and the next run
   restores it. For seeded files, yes, and the difference is reported. The line is drawn at
   *mechanical enforcement* versus *prose and content*, which is the same line the table above
   draws. A project with a genuine exception to a validator has to argue it into the template,
   where every project gets it — which is the point.

3. **Does the template depend on Canon?** **No.** ADR-0009 settled this from the other side: Canon
   reads repositories and writes nothing to them. A template that required a running tracker to
   commit would invert that relationship. The template does not know Canon exists; Canon recognises
   the template by one file.

4. **Who owns compatibility?** Not answered, and deliberately deferred. With two projects it is not
   yet a real question, and `.sdlc/VERSION` makes it an answerable one when it becomes one: a
   project can be asked what it is on, and an upgrade is an explicit act with a diff. Guessing at a
   compatibility policy for forty projects while running two would be inventing process for a
   problem nobody has.

## Evidence

Exercised against a repository that already existed rather than reasoned about: **Puzzlo**, a
SwiftUI iOS app with an AWS backend — 109 files, 60 commits, on the App Store. It has a README
somebody wrote and a `.gitignore` tuned to Xcode, which are exactly the two files a copy destroys.

- `README.md` untouched; `.gitignore` gained five rules in a marked block and kept its own.
- All five validators pass unmodified against Swift, Python and Terraform. The template makes no
  assumption about language or toolchain.
- Running the installer twice changes nothing.
- Three bugs found by testing rather than by reading, recorded in `docs-008`.
