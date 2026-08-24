# 0005 — Where work lives: git or Canon

**Status:** proposed — needs a decision
**Date:** 2026-08-24
**Supersedes in part:** parts of [0003](0003-storage-history-and-federation.md), which chose the
storage model but left the transport unbuilt and did not say which end originates work.

## Context

Canon was built alongside an agentic SDLC template. The template keeps intent in
`specs/product.md`, work in `specs/increment-plan.md`, and evidence in `specs/increments/` — all in
git, with [ADR-0002](0002-repository-is-the-source-of-truth.md) stating that the repository is the
source of truth. Canon keeps issues in an event log.

Both were built. Then the ledger was imported into Canon, and the awkward question follows: if the
same work exists in both, which one is real?

## The measurement

Canon's own instance, holding this project's history:

```
347 events total
335 (96%)  reconstructible from specs/increment-plan.md + git log
 12  (3%)  not present in git in any form
```

The 335 are issues, transitions, team assignment, dependencies, hierarchy and commit links.
`scripts/import-ledger.py` reconstructs all of them, and `canon link` derives the commit links from
git directly. **The importer is the proof: a thing you can rebuild from git is a cache of git.**

The twelve that are not in git are six actor registrations, one role grant, four team memberships
and one token. That is identity, not work tracking.

So for a single repository, Canon is a queryable projection of the ledger. It is not redundant —
a projection you can query, aggregate and enforce against is worth having — but it is not the
source of truth, and building it as though it were is the error this ADR corrects.

## What genuinely cannot live in one repository's git

Being precise about this matters, because the answer decides what the central component is *for*.

1. **Identity.** Who exists, what they may do, what proves it. A repo cannot hold the org's actor
   registry, and forty repos holding forty copies is worse than not having one.

2. **Anything spanning repositories.** Cross-repo dependencies, and aggregate reporting. GitHub
   Issues has this exact limitation and it is a documented, unfixed failure — noted in ADR-0003 and
   then not acted on.

3. **Groupings that are orthogonal to containment.** A *ProductVersion* — the set of features
   presented to a customer this month — is not hierarchy. Epic > Feature > Story is containment; a
   version is a cross-cutting milestone that spans teams and repositories by nature. Canon's typed
   hierarchy (feat-020) cannot express it, and no single repo's ledger can own it.

4. **Work with no repository.** Support, design, operations. Real work that a repo-local ledger has
   nowhere to put.

5. **The schema itself.** One `canon.yaml` for the organisation is the product's entire argument.
   A rule enforced by a script copied into each repository is a rule that diverges — which is
   [ADR-0006](0006-distributing-the-template.md)'s subject, and the same disease from the other end.

Note what is *not* on that list: storing the issues. Points 1–5 are configuration, identity and
aggregation. None of them require the centre to be where work is authored.

## Options

### A. Keep Canon as the primary store; the template's ledger is a convenience

What we have. The repo ledger and Canon both hold the work, and `import-ledger.py` synchronises one
into the other, once, manually.

- **For:** already built; a single place to look; the UI, MCP surface and metrics all work today.
- **Against:** two sources of truth for the same facts and no reconciliation between them. The
  agent's loop is in git and the tracker is elsewhere, so every increment is written twice and can
  disagree. Contradicts ADR-0002, which we wrote.

### B. Delete Canon; the ledger in git is the tracker

Take the 96% seriously and stop there. `validate-plan.py` already enforces a schema, and the ledger
is already the thing the agent reads and writes.

- **For:** one source of truth; nothing to deploy; the agent loop is already this.
- **Against:** loses all five things above. No cross-repo anything, no identity, no ProductVersion,
  nothing for a person who is not in the repository, and the schema is a copied script that drifts.
  This is GitHub Issues' failure mode chosen deliberately.

### C. Git originates work; Canon owns schema, identity and the aggregate — *recommended*

The repository is where work is authored and where the agent loops, exactly as now. Canon becomes a
**federated aggregator**:

- Each repository keeps its ledger in git. It is the origin, and ADR-0002 stands unqualified.
- Canon holds the org-wide `canon.yaml` and **validates repo ledgers against it** rather than
  keeping a second editable copy.
- Repositories publish their event stream to Canon. Canon's log becomes a merge of federated logs,
  and its projection is the cross-repo view.
- Canon owns what no repository can: identity, ProductVersions, cross-repo dependencies, reporting,
  and issues that have no repository.

- **For:** one origin per fact. Uses the event log for what ADR-0003 designed it for. Makes the
  centre's job the job only a centre can do.
- **Against:** the transport does not exist. Ingest introduces conflict questions ADR-0003 raised
  and did not settle. Some of what is built moves or goes.

## Decision

**Option C.**

The deciding argument is that Option A requires believing two sources of truth can be kept
consistent by a script somebody remembers to run, and this project has already produced the
evidence against that: the ledger and Canon disagreed the moment either changed, and only stayed
aligned because one person re-ran the importer.

## Consequences

### What was built right, and stays

- **The event log, canonical CBOR and ULID ids.** ADR-0003 chose these *for* federation. In a
  federated design they are load-bearing rather than over-engineering.
- **Schema enforcement.** Moves from "enforced on write to Canon" to "enforced on ingest, and
  offered to repos as a check". The rules do not change.
- **Identity, roles, teams, tokens** (feat-014, 015, 030, 031). Central by nature; unaffected.
- **MCP and API parity, flow metrics, dependencies, the query language.** All aggregate concerns.

### What is in the wrong place

- **Canon as author of issues.** The create screen and `canon new` writing a local log. Under
  Option C a repo-scoped issue is authored in the repo.
- **`canon new`, `canon link`, `canon trace` writing SQLite directly.** Recorded for days as "the
  biggest gap"; under this model it reads as a symptom of building the centre as the origin.
- **`scripts/import-ledger.py` as a one-off.** Becomes the transport, run continuously, not a
  migration run by hand.

### What does not exist yet

- **Ingest.** Repo → centre. ADR-0003 named this and did not build it.
- **ProductVersion.** A cross-cutting grouping orthogonal to hierarchy.
- **Reconciliation.** What happens when the centre and a repo disagree — the centre must be
  derivable, so the repo wins, but nothing implements that.

### What this is not

**Not a rewrite.** Roughly four of the forty-five increments are in the wrong place, and two of
those are UI screens. The domain, the log, the schema layer, identity and the agent surface are
unaffected. The work is subtractive at the edges and additive in the middle: build the transport,
stop pretending the centre is the origin.

## Open questions for the decision

1. **Does a repo-scoped issue exist centrally before it is published?** If yes, ids must be
   allocated centrally and offline work breaks. If no, the centre cannot report on work in flight.
2. **What is the transport?** A push from CI on merge is simplest and requires no inbound access to
   the repo. A pull requires the centre to hold credentials for every repository.
3. **Does the centre ever write back?** A ProductVersion assignment is central by nature but is a
   fact about a repo-local issue. Something has to own that edge.
4. **Is `canon.yaml` distributed to repos, or do repos ask the centre?** Distribution reintroduces
   drift; asking introduces a dependency on the centre being reachable to commit.
