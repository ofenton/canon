# feat-035: Ingest a repository

## Context

The first increment of Canon as an aggregator. Read a repository that follows the template and
derive everything from it: the spec, the ledger, and every increment's status history.

Additive by design. Nothing existing was removed, so `main` keeps working while the read path is
built underneath it and `cut-001` removes the write path later.

## Design notes

**Whole files at each commit, not diffs.** This is the decision the package turns on. A diff shows a
changed `Status:` line without reliably showing which increment it belongs to — the `## feat-nnn:`
heading is usually outside the hunk's three lines of context. A diff parser therefore guesses, and
guesses wrongly in exactly the case that matters: a long increment whose status sits far from its
heading. Reading the file at each commit and comparing parsed states is unambiguous, and costs one
parse per commit, which is nothing.

**Tolerant parsing.** This meets other people's markdown. A parser that refused a repository because
one increment was malformed would make Canon useless for precisely the repositories most worth
reporting on. What cannot be read is left out; `feat-037` makes that visible.

**A spec is optional, a ledger is not.** A repository with a ledger and no spec is mid-adoption and
worth reporting on. A repository with no ledger is not one this reads, and saying so is more useful
than returning an empty product.

**Determinism is asserted, not assumed.** The same head produces the same fingerprint, which is what
makes re-ingesting safe.

## The defect that real data found

The eight fixture tests passed. Ingesting **this** repository failed:

```
docs-001: transition 2 starts at "" but the previous ended at "in-review"
feat-016: transition 1 starts at "" but the previous ended at "approved"
feat-017, feat-018, feat-019: the same
```

The cause was real history, not a parsing bug: a `Revert "plan: hierarchy, dependencies, detail view
and richer fields"` commit removed five increments from the ledger, and a later commit put them
back. Derived naively, an increment that vanished and returned reads as having been **created
twice**.

An increment leaving the ledger is a real event with a real commit, so it is now recorded as one:

```
0                -> in-progress   26843be
1  in-progress   -> in-review     d94bc48
2  in-review     -> (removed)     a0ae48a
3  (removed)     -> in-review     5085867
4  in-review     -> done          8e8e6d6
```

The first fix was wrong in two ways and the second run showed both — a duplicate
`(removed) -> (removed)`, and then the id disappearing from tracking entirely so the reappearance
was still a creation. Both came from mutating the freshly parsed state to carry removals forward.
The working version keeps one `lastKnown` map that always holds the status most recently reported,
including `Removed`.

## Evidence

**Verified by:** implementing session, `inc/feat-035-ingest`

### AC: ingest without per-repository configuration

```
$ canon ingest .

Canon
  Work happens in repositories. Agents plan it, build it and record it there — in a spec, a ledger and

  head 602ec41 · git@github.com:ofenton/canon.git
  54 increments · 63 requirements

  approved          8
  in-review         3
  done             43

  191 status transitions, derived from specs/increment-plan.md
```

No configuration was supplied. `## Sequencing` and the prose after it are correctly not increments.

### AC: transitions derived from commit history, not approximated

`TestTransitionsComeFromCommitHistory` builds a real git repository with five commits at known times
and asserts each transition's from, to, timestamp and commit. `TestUnrelatedCommitsProduceNoTransitions`
asserts that two commits which touch the ledger without changing a status produce **no** transitions
— the specific failure of the mechanism this replaces, which produced one per commit carrying the
trailer and reported a p50 of nine minutes against a real four hours.

`TestIngestThisRepository` cross-checks against git itself rather than against the parser: every
commit cited by a derived transition must appear in `git log -- specs/increment-plan.md`. 54 of 54
increments carry a history, and every history is continuous.

### AC: ingesting twice produces the same result

`TestIngestIsDeterministic` — same head, same fingerprint.

### Tests

Ten in `internal/ingest`, all against **real git repositories** rather than fixtures of git's
output: reading the ledger, transitions from history, unrelated commits producing none, determinism,
two changes sharing a commit, a malformed entry not losing the others, a missing ledger refused, an
optional spec, removal and return, and the self-hosting test above.

Full suite green across twelve packages.

### Scope

`git diff --cached --stat main` — run. A new `internal/ingest` package, `canon ingest`, and the
usage text. Nothing removed.

### Not verified

**Only local paths.** `Repo()` takes a directory. Discovering repositories across an organisation
and fetching them is `feat-038`, and nothing here talks to a remote.

**Only the ledger's history is walked.** An increment's evidence file, or the spec's own history, is
not read. Transitions are the thing with a time; the rest is current state.

**No conformance reporting.** A repository whose ledger is half-parseable ingests quietly. Making
that visible is `feat-037`, and until then the tolerance that lets Canon read imperfect repositories
also lets it read them silently.

**The `(removed)` status is Canon's invention.** It is not in the template's status list, and a
consumer filtering on schema statuses will not expect it. It is parenthesised so it cannot collide
with a real status, but it is a value this package introduces rather than derives.
