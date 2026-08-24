# 0009 — Canon as aggregator

**Status:** accepted
**Date:** 2026-08-24
**Settles:** the open questions in [0005](0005-where-work-lives-git-or-canon.md).

## Context

[ADR-0005](0005-where-work-lives-git-or-canon.md) established that 96% of Canon's data is
reconstructible from a repository's ledger and git log, and proposed that git originates work while
the centre owns schema, identity and aggregation. It left four questions open. This settles them by
going further than that ADR did: **the centre originates nothing.**

## Decision

**Canon is pointed at an organisation. It reads repositories that follow the template, derives
everything, and displays it. It authors no work of its own.**

- **Ingest, not authorship.** Canon clones or fetches, parses `specs/product.md` and
  `specs/increment-plan.md`, derives transitions, and reports. Nothing is typed into Canon that
  becomes the truth.
- **A catalogue of products.** One repository is one product ([ADR-0007](0007-components-and-composed-releases.md)),
  so the set of conforming repositories *is* the catalogue.
- **Intake writes to git.** A bug or request raised through Canon becomes a pull request adding a
  `planned` increment to that repository's ledger. Canon does not hold a second class of issue.

### Increment time comes from the ledger's own file history

`AGENTS.md` rule 4 requires that every status change is a commit. Therefore
`git log -p specs/increment-plan.md` **is** the transition log, exactly, with no heuristic.

Measured against this repository, 46 increments have a full transition history in git:

```
feat-025
  09:18:37  (new)     -> approved
  10:26:42  approved  -> in-review
  10:26:48  in-review -> done
```

This matters because the mechanism we built was materially wrong:

```
from the ledger's git history — 35 increments
  p50 4.3h · p85 15.4h · p95 17.6h · max 22.3h

scripts/import-ledger.py's approximation
  p50 9m  · p85 1.2h  · p95 9h
```

**Roughly thirty times out.** The importer spread each increment's route across the commits carrying
its trailer, and squashed merges collapsed those to near-simultaneous timestamps. The exact answer
was available the whole time, in one file's history.

The one honest limit: resolution is bounded by commit granularity. Two status changes in one commit
share a timestamp — visible above, where `in-progress` collapsed. That is exact about what git
recorded, which is the most any observer can claim.

## Schema enforcement, in this structure

The question changes shape. An aggregator cannot refuse a commit that has already happened.

**Enforcement stays at the edge; visibility becomes central.** `validate-plan.py` already runs in the
repository's pre-commit hook and CI, where it can refuse. Canon runs the same rules across every
repository it watches and reports **conformance**: which repositories follow the convention, which
have drifted, which have an increment claiming `done` with no evidence.

That is a genuine weakening of the original claim — "the schema is enforced on write" becomes "the
schema is enforced where writes happen, and reported centrally". It is also the only honest position
for something that reads other people's repositories.

### What is enforced

Exactly what the template already fixes, and nothing more:

```python
STATUSES = ["planned", "approved", "in-progress", "in-review", "done", "abandoned"]
TYPES    = {"feature", "fix", "security", "perf", "refactor", "chore", "docs"}
REQUIRED = [ ... Type, Status, Scope, Acceptance Criteria, ... ]
EVIDENCE_REQUIRED_FROM = {"in-review", "done"}
```

For `product.md`: that it exists, that requirements are identifiable, and that Must requirements are
traced. `check-traceability.py` already does this.

### `canon.yaml` almost disappears, and so does the original thesis

If the template fixes states, transitions, types and required fields, then `canon.yaml` has nothing
left to configure. What remains is a list of sources:

```yaml
sources:
  - org: acme
    include: ["*"]
```

**This changes the product's argument, and the change should be made consciously.** Canon was
positioned as *"your organisation's schema, centrally owned, so it cannot drift per project"*. It
becomes *"one fixed convention, aggregated"* — which is a **more** opinionated position, not a
weaker one: a schema with no configuration cannot drift at all. It is closer to Linear's answer than
to the one this project started with, and it gives up the claim that an organisation can be as
complex as it genuinely needs.

Whether that trade is right is the central question of this ADR, and it is not obviously yes.

## Identity

**Reads are open.** Anyone in the organisation can see anything. Actors, roles, teams, tokens,
team-scoped permissions and the `administer` verb all exist to protect writes that no longer happen.

**The one gate that matters already exists, and it is not Canon's.** A request raised by anybody
enters as a `planned` increment in a pull request. A Product Owner accepts it by approving and
merging. That is pull-request review — GitHub and Azure DevOps already do it, with better tooling
than Canon would ever build, and the org's existing access controls already decide who may approve.

So Canon needs **no approval concept**. It shows *"3 requests awaiting a decision"* by listing open
pull requests that add a `planned` increment. A read, derived, holding no state.

`feat-007`'s proposal machinery — agent proposes, human decides — was the right idea in the wrong
place. Pull request review is that mechanism, and it predates us.

## What gets deleted

12,890 non-test lines today. Best estimate of what survives:

| Package | Lines | Fate |
|---|---:|---|
| `internal/enforce` | 3,578 | **mostly deleted** — authz 173, auth 103, proposal 115, registry 92, board 54, backdate 38, checklist 106, dependency 136, link 79 all protect writes. Keep `usage.go` (125) as conformance reporting. |
| `internal/schema` | 1,363 | **mostly deleted** — roles 156, teams 57, webhooks 43, and most of schema.go. The template is the schema. Keep hierarchy validation if products ever nest. |
| `internal/api` | 2,130 | **halved** — every write route goes; the read surface and pagination stay. |
| `cmd/canon` | 1,686 | **halved** — `new`, `link`, `token`, `bootstrap` go. `serve`, `trace`, `usage`, `schema` stay; `ingest` is new. |
| `internal/event` | 838 | **demoted, see below** |
| `internal/projection` | 1,170 | **kept**, but rebuilt from ingest rather than from authored events |
| `internal/query` | 752 | **kept** — search and filter over the aggregate |
| `internal/metrics` | 401 | **kept**, fed by exact transitions instead of approximated ones |
| `internal/mcp` | 511 | **kept**, read-only |
| `internal/ui` | 211 | **kept**; the create screen inside it goes |
| `internal/webhook` | 250 | **kept** — notify on ingest rather than on write |

Roughly **5,000–6,000 lines removed**, and the `import-ledger.py` heuristic goes with them.

### The uncomfortable part: the event log is over-built

[ADR-0003](0003-storage-history-and-federation.md) chose an append-only log, immutability triggers
at the database level, and canonical CBOR for byte-stability, because Canon was the origin and its
log was the record.

If everything is derived from git and rebuildable at any time, the log is a **materialised cache**.
A cache does not need append-only triggers, and it does not need byte-stable encoding for a
signature nobody will apply. Ordinary tables would do.

It still works and there is no urgency to change it — but `feat-001` is the increment most likely to
look like engineering bought for a role the product no longer has, and that should be said before
anybody decides how much of the rest to keep.

## Consequences

- **The write path, and everything defending it, goes.** That is the bulk of the deletion and the
  bulk of the risk removed with it.
- **Canon can be pointed at an organisation and produce something immediately**, with no adoption
  step for teams already using the template.
- **Canon becomes unable to serve work that has no repository** — support, design, operations. That
  was previously possible and will not be. It is the largest capability given up here.
- **The product's argument narrows** from configurable-and-central to fixed-and-aggregated.

## Open questions for the decision

1. **Is "one fixed convention" the product you want?** It is more opinionated than where this
   started, and it forecloses the regulated-team-plus-support-team case the original spec used to
   justify configurability.
2. **What happens to work with no repository?** Either it is out of scope, or a repository is
   created for it, which is the convention taken to an uncomfortable place.
3. **How often does ingest run, and what does stale look like?** A dashboard that is quietly four
   hours behind is worse than one that says so.
4. **Is any of the existing code worth keeping, or is this a rewrite?** The estimate above says
   roughly half survives. That is a judgement made by the person who wrote it, and worth a second
   opinion.
