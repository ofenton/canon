# chore-003: Dogfood — run this project in Canon

## Context

The most convincing thing a tracker can demonstrate is that its authors use it. This imports the
increment ledger the project was actually built from.

## Design notes

**`deploy/canon.yaml` does not describe our process — it *is* our process.** The states are the
ones the ledger uses, the two human gates are the two transitions an agent may only propose, and
`done` requires evidence because that is the rule in `AGENTS.md`. If the file and the process ever
disagree, one of them is wrong, which is the point of putting a process somewhere a pull request
has to touch.

The agent role is written straight out of `AGENTS.md`: it may move work from `approved` to
`in_progress` to `in_review`, and it may propose Gate 1 and Gate 2 but not take them. That is
exactly the arrangement this project has been run under all week, now enforced by the product
rather than by an agent following instructions.

**History is reconstructed, not asserted.** An increment that is `done` did not teleport there; it
passed through approved, in_progress and in_review. The importer replays each transition so the
states and their order are real.

## Two defects found, which is what dogfooding is for

**1. `bootstrap` hardcoded the role name `admin`.** Pointing Canon at its own schema, where the
equivalent role is `maintainer`, failed immediately:

```
canon: canon.yaml defines no "admin" role; defined roles are agent, maintainer
```

The command now takes `-role`, and defaults to whichever role the schema grants everything to.
Getting that right needed a second pass: comparing grant *strings* does not work, because
`transition:*` and `transition:approved->in_progress` are different strings and the first subsumes
the second. It now evaluates every concrete operation through `Role.Decide`, which is the schema's
own semantics.

Verified across four cases: Canon's schema picks `maintainer`, the sample schema picks `admin`, an
explicit `-role agent` is honoured, and `-role wizard` is still refused.

**2. Backdating is designed for but not reachable.** `Event.At` may precede the append — built
into feat-001 for exactly this — but no API accepts a timestamp, so imported history lands at
import time. The consequence is visible in the metrics below: every cycle time is `0d`.

This is not cosmetic. R27 ("record the link with its original commit timestamp") needs it, and so
would any Jira import. The importer reads the real commit times from `Increment:` trailers and
currently discards them, with a comment saying why.

## Evidence

**Verified by:** implementing session, `inc/chore-003-dogfood`

### Every increment imported with its status and history

```
importing 20 increments as ollie
  chore-001  done         Adopt the increment workflow
  …
  chore-003  in-progress  Dogfood: run this project in Canon
  …
created 20 issues, applied 78 transitions
```

No problems reported. The board:

```
canon — grouped by state
  in_progress   1   chore-003
  done         19   chore-001, chore-002, docs-001, docs-002, feat-001, feat-002 … +13
```

One increment as Canon holds it:

```
feat-012  Meet the latency budget at 10,000 issues
state done   team canon
  kind      perf
  risk      Medium — if the projection shape is wrong this reveals it late…
  rollback  Remove the CI latency gate; correctness is unaffected
  tier      2
  traces    R17
transitions: approved → in_progress → in_review → done
```

Queries over our own work:

```
kind=security          0
tier=1                 13
kind=docs              2
state=in_progress      1
```

### Canon is where this project's work is tracked from here

`deploy/canon.yaml` and `scripts/import-ledger.py` are in the repository, so the import is
reproducible: start `canon serve -schema deploy/canon.yaml`, run the script, and the project is
in the tracker.

### Flow, and why it is empty

```
completed 19   in progress 1
cycle time   p50 0d  p85 0d  max 0d  (n=19)
```

All zero, because of defect 2 above. The *shape* is right — 19 completed, 1 in progress, computed
from replayed transitions — but the durations are not, and reporting them as though they were
would be worse than saying so.

### Scope

`git diff --cached --stat main` — run. The schema in `deploy/`, the importer in `scripts/`, and
the `bootstrap` fix in `cmd/canon`.

**The bootstrap change is a deviation** from "no product code changes". It was a hard blocker:
Canon could not be pointed at its own schema at all. Finding that is precisely what this increment
exists to do, and fixing it in a separate increment would have meant this one could not be done.

### Not verified

The ledger remains the source of truth for this project's process, per ADR-0002 — Canon holds a
projection of it, not the master copy. Making Canon authoritative would need the backdating fix
and a way to keep the two in step, and neither is built.

CI runs on the pull request.
