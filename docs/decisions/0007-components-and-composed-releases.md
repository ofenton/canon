# 0007 — Components and composed releases

**Status:** accepted
**Date:** 2026-08-24
**Builds on:** [0005](0005-where-work-lives-git-or-canon.md) — git originates work, the centre owns
schema, identity and the aggregate.

## Context

The agentic SDLC template gives each repository one `specs/product.md`. That works while one
repository produces one thing a customer buys. It stops working the moment a customer-facing product
is assembled from several repositories — a storefront, an orders API, a recommendations data
product, and a design system shared with three other applications.

Two questions were tangled together and are separable:

- **What does one repository contain?**
- **How is a customer-facing release expressed?**

## Decision

**A repository holds one component, and a release composes component versions.**

### A repository holds one component

`specs/product.md` describes **the deliverable this repository produces**. That may be a UI, an API,
a data product, or a shared library. It is the unit that builds, versions and deploys on its own
cadence.

A *business* product — the thing a customer buys — is a composition of components, and it is
therefore not a repository-local concept. It lives in the centre.

The template's shape does not change. `product.md` already meant "the thing this repository
produces"; this makes that explicit rather than leaving it to be inferred.

### A release composes component versions

Each component versions itself, in its own repository, on its own cadence. The centre composes:

```
Release 2026-08
  storefront    @ 2.4.0
  orders-api    @ 1.9.1
  recommendations @ 0.7.2
```

A release therefore has two kinds of member: the **component versions** it pins, and the **issues**
those versions contain. The second is derivable from the first.

## Alternatives rejected

**One repository per business product (a monorepo).** Everything becomes repository-local and a
release is a field — genuinely the simplest answer, and correct for a single-repo product. Rejected
because it does not survive a shared component: a design system used by four products either lives
inside one product's repository, which is wrong, or the monorepo expands until it is the whole
organisation. It also asks an organisation with fifty repositories to restructure before it can use
the tool.

**Unconstrained — a repository is whatever a team made it.** Rejected because nothing can then be
aggregated: "product" means something different in each repository, which is the divergence this
product exists to refuse, one level up.

**A release as an org-wide enum field.** Demonstrated working today, with no new code:

```yaml
fields:
  - {name: version, type: enum, values: ["2026-08", "2026-09"]}
```

Cross-repository grouping falls out of the shared schema for free, and an undeclared version is
refused. Rejected as the end state, not as a starting point — see the sequencing note below. It
cannot answer *"which build of the orders API is in the August release"*, which is an incident
question, and it cannot carry a release date, a status or notes.

**Git tags and platform releases only.** Rejected because it cannot aggregate across repositories,
which is the one thing a centre exists to do. Choosing it is choosing not to have a centre.

## Consequences

- **The centre gains a composition concept.** A release pins component versions. This is the first
  entity in Canon that is not an issue, and that is a real cost against a product whose thesis is
  one entity — accepted because a release genuinely has facts of its own, which an issue field
  cannot carry.
- **Components must version.** Not every team does. A component with no version cannot be pinned,
  so this imposes a discipline that did not previously exist.
- **Cross-repository dependencies become checkable.** *"2026-08 contains `orders-api@1.9.1`, which
  waits on a storefront change that is not in 2026-08"* is a question the centre can now answer, and
  no repository can.
- **"Product" carries two meanings** — the component a repository produces, and the thing a customer
  buys. This is a real cost that cannot be designed away without renaming `product.md`, which would
  churn the template, the skill and every existing project for a naming improvement. Documented
  rather than fixed.

## Sequencing

**Ship the enum field first.** It works today, costs nothing, and gets cross-repository grouping
free. It answers "what is in the August release" at the issue level immediately.

**Promote to composed releases when a release needs facts of its own** — a ship date, a
planned/released status, generated notes, or a pinned component build. That is the threshold. It is
not repository boundaries, and it is not team size.

Building the composition before anything needs pinning would be a new entity earning nothing.
