# docs-005: Reframe the product spec

## Context

ADR-0009 decided Canon derives and does not author. The spec still described a tracker written into
directly, with 51 requirements about writes, permissions and proposals. A spec that contradicts the
accepted decisions is worse than none: it is what an agent reads first.

## What changed, and what did not

**Changed.** The problem statement is no longer "Jira's configuration is per-project and
unbounded". It is: work already happens in repositories, precisely recorded, and nothing shows it
together — and the common answer, a second tracker somebody types into, creates two sources of truth
that disagree by Wednesday.

**Kept.** Every superseded requirement, verbatim, under a heading that says so. 43 delivered
increments trace to them and `check-traceability.py` resolves those traces. Deleting them would tidy
the document and erase the record of how the product reached its current shape.

## The reframe in one line

From *"an issue tracker whose schema is versioned config"* to *"point it at an organisation and it
shows you what every team is building"*.

## What it gives up, stated in the spec rather than omitted

- **Authoring work in Canon.** Anything typed in that became truth recreates the two-sources problem.
- **Work with no repository.** Support and operations tickets have no home. This is the largest
  capability given up, and it is in the spec's Out of scope section rather than discovered later.
- **Configurability.** States, types and required fields are fixed by the template. More opinionated
  than this project started, deliberately.

## Evidence

**Verified by:** implementing session, `inc/docs-005-reframe`

### Traceability holds across the reframe

```
specs/increment-plan.md: ok — 53 increments trace cleanly
docs/architecture.md: ok — 16 invariant(s), every named test exists
```

Nine new Must requirements (R52–R60) and three Should (R61–R63), each claimed by one of six planned
increments.

### The loop refused the spec until the work was planned

Writing the requirements first produced:

```
✗ R59 is a Must requirement but no increment traces to it
✗ R60 is a Must requirement but no increment traces to it
```

`check-traceability.py` will not accept an agreed spec whose Must requirements nobody has planned.
That is the template working as intended, on its own author, and it is the reason the increments in
this commit exist rather than being deferred.

### Scope

`git diff --cached --stat main` — run. `specs/product.md`, the ledger, and this record. No code.

## Not verified

**The six planned increments are estimates of shape, not of effort.** `cut-001` claims roughly 5,000
lines removed; that number came from counting non-test lines per file in ADR-0009 and is the
author's judgement about which survive.

**Nothing is built.** This increment is a spec and a plan.
