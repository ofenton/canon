# 0008 — The meta-repo and the product track

**Status:** proposed — needs a decision
**Date:** 2026-08-24
**Builds on:** [0007](0007-components-and-composed-releases.md) — a repository holds one component.

## Context

[ADR-0007](0007-components-and-composed-releases.md) decided that a repository holds one component
and a business product is a composition of them. That leaves a gap the template cannot fill:

- A Product Owner starting a new product does not know what a component is, let alone which
  repositories one needs. The template's loop begins with `write-product-spec` in *a* repository,
  and nothing says which.
- Once components exist in separate repositories, an agent begins every session with no knowledge of
  the others. Different test commands, different CI, different conventions, rediscovered each time.
- `check-traceability.py` verifies the ledger against **the local git**. It cannot see another
  repository, so a component increment cannot be checked against the product increment it serves.

## Prior art, and how established it actually is

Worth being precise, because the two halves have very different standing.

**The mechanism is a decade old.** [`meta`](https://github.com/mateodelnorte/meta) by Matt Walker —
2.2k stars, 297 commits — turns many repositories into one "meta repo" with a `.meta` JSON manifest,
so `meta git clone` fetches every child. Google's `repo` tool for Android is earlier prior art for
the same idea. The tagline is the argument: *"why choose many repos or a monolithic repo, when you
can have both?"*

**The agent-oriented variant is practitioner-led and 2026-vintage, not a vendor standard.** No major
vendor ships it. What exists is convergence across independent practitioners, which is weaker
evidence than a specification but stronger than one blog post:

| Who | Scale | Their name for it |
|---|---|---|
| Owen Zanzal (Shoplogix, formerly GameChanger) | 35 repositories | "virtual monorepo" |
| Bernd Kampl (Anyline) | six mobile SDKs | "agents meta-repository" |
| Matthew Groff (Umbrage, a Bain & Company studio) | client work | "context engineering" |
| Dean Sharon | — | built **Mars**, an open-source tool formalising it |

Also called "spine pattern", "polyrepo synthesis" and "repo-of-repos". The proliferation of names is
itself the signal: several people arrived at the same shape separately and none of them agreed on
what to call it.

**One component of it *is* standardised.** `AGENTS.md` is used by 60,000+ repositories and stewarded
by the Linux Foundation's Agentic AI Foundation — which this template already adopted in
[ADR-0001](0001-vendor-neutral-agent-tooling.md).

The common finding across all of them, in Groff's words: *"Every session with Claude Code or OpenCode
starts stateless. No memory of your conventions. The only thing that reliably onboards each new
session is your rules file."*

## Decision

**A product gets a meta-repository holding no application code, and the loop gains a fourth track
that runs there and stops before implementation.**

### What the meta-repo holds

```
specs/product.md          the business product — customers, outcomes, requirements
docs/architecture.md      components, their dependencies, and the invariants across them
repos.yaml                the component registry: name, repository, build, test, version file
specs/increment-plan.md   product-level increments, which decompose into component increments
docs/decisions/           decisions that bind more than one component
```

`repos.yaml` is the artifact that does not exist today and is the reason to do this at all. Everything
else is the template as it stands, in a repository that happens to have no code.

### The product track

The template currently matches ceremony to work with three tracks — Direct, Increment, Spec. This
adds a fourth:

| Track | Where | Runs |
|---|---|---|
| **Direct** | any repo | just do it |
| **Increment** | component repo | ledger → build → verify |
| **Spec** | component repo | spec → architecture → plan → build → verify |
| **Product** | meta-repo | spec → architecture → plan → **stop** |

**The product track never implements.** `implement-increment` and `verify-increment` have nothing to
act on in a repository with no code, and an agent that runs them there will do something incoherent.
Saying so explicitly is the point of naming the track.

### How the loop forks

```
meta-repo:     write-product-spec → design-architecture → plan-increments → [GATE 1]
                                          │                      │
                                    repos.yaml             product increments
                                          │                      │
                                          ▼                      ▼
component repo:                    (repos exist)  →  plan-increments → implement → verify → [GATE 2]
```

Three changes to existing skills follow:

1. **`design-architecture` gains an output**: the component decomposition and `repos.yaml`. This is
   the same question it already asks — what are the components and what depends on what — with
   "and where does each live" appended.

2. **`plan-increments` gains a proposing mode.** In the meta-repo it produces product increments and
   *proposes* the component increments they decompose into. It does not write into another
   repository.

3. **`Traces:` gains a cross-repository form.** A component increment traces to a product increment
   that is not in its own git history.

### Repositories are proposed, never created by an agent

Creating a repository is an organisational act — naming, access control, CI, branch protection,
cost. `design-architecture` proposes a layout; a human creates them, and Gate 1 already sits at
exactly that moment. An agent may be granted this deliberately; it must not acquire it by default.

## Consequences

- **Cross-repository traceability cannot be checked locally.** `check-traceability.py` sees one git.
  Verifying that a component increment serves a real product increment needs something that can see
  both — which is precisely the aggregation job [ADR-0005](0005-where-work-lives-git-or-canon.md)
  gives the centre. The meta-repo provides local orientation; Canon provides verification across
  repositories. They are complementary, and neither replaces the other.

- **A third artifact that can rot.** The practitioner write-ups are unanimous that the meta-repo
  requires active maintenance and that agents cannot keep it synchronised by themselves. That is
  [ADR-0006](0006-distributing-the-template.md)'s drift problem at a third level, and it should be
  answered the same way it was for `architecture.md`: make `repos.yaml` machine-checked. Does every
  listed repository exist? Does each carry the files the registry claims? A registry nobody verifies
  is a map that sends agents to the wrong place, which is worse than no map.

- **`component.md` is deliberately not introduced.** A component repository already has
  `product.md`, and under ADR-0007 that *is* the component. A second document type means a second
  skill, a second template, and an ambiguity about which one an increment traces to. What is
  genuinely missing is one line — a reference upward to the product — not a new document. If
  component specs turn out to need consistently different sections, split then; splitting on the
  prediction produces two documents that are eighty per cent identical and drift.

- **A product with one component does not need any of this.** The meta-repo is overhead until there
  are at least two repositories to orient between. Below that it is a directory of indirection.

## Open questions for the decision

1. **Does the meta-repo hold a ledger, or only a spec?** A two-level ledger means a product
   increment is done only when its component increments are, and nothing expresses that today.
2. **Is `repos.yaml` the source of truth, or a projection of the centre's registry?** Two registries
   is the mistake ADR-0005 was written to stop making.
3. **What creates a component repository from the proposal?** A human, a script, or an agent granted
   the permission — and if the last, under what gate.
4. **Does `AGENTS.md` change now, or when this is accepted?** It should change when accepted. A loop
   documented but not decided teaches agents a process nobody agreed to.
