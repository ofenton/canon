# chore-006: Delete what the rewrite left behind

**Traces:** R58

## What was actually there

`cut-001` deleted the event store, the schema loader and `canon.yaml` a month ago. It did
not delete the things that pointed at them, so the repository has been contradicting its
own README since:

| | |
|---|---|
| `deploy/canon.yaml` | 3,059 bytes, tracked, the entire old schema — states, roles, grants. No Go file references it. `CHANGELOG.md` already announced "**BREAKING:** `canon.yaml` is gone." |
| `.gitignore` `*.db`, `*.db-wal`, `*.db-shm` | Rules for a writer that no longer exists |
| `.gitignore` `/canon.yaml` | Anchored, with a comment explaining that the unanchored form would exclude `internal/schema/testdata/canon.yaml` — a file, and a package, that were deleted |
| `.gitignore` `/.demotmp/`, `/.seedtmp/` | Nothing creates them |
| `canon.db`, `deploy/canon.db*` | 2.2MB of local WAL, untracked, last written 23 August |

README line 12 says Canon has "no database, and nothing to configure". That was true of the
program and false of the repository.

## What was not there

I looked for dead UI code and did not find any. `cut-001` rewrote `index.html` rather than
editing it, so there is nothing left over: every function is called, and the only
unreferenced CSS selectors a first pass reported — `.note` and `.warning` — are set at
line 466 as `class="sev ${f.severity}"`, from the three `conform.Severity` values. They are
live, and deleting them would have removed two of the three severities' colours from the
conformance screen.

Reporting no cleanup is the honest outcome there, so no UI code was changed.

## The test

`TestTheRepositoryHoldsNoStateOrConfiguration` reads `git ls-files` and fails on any
tracked `.db`/`.sqlite`, and on any `.yaml`/`.yml` outside `.github/`. The claim is now
checked rather than written down.

It found its own defect immediately. `go test` runs in the package directory, so
`git ls-files` listed six files and every rule passed by seeing nothing — caught only
because the test asserts a minimum tracked-file count before it starts. It now resolves
`--show-toplevel` first. A structural test that cannot see the thing it guards is the
failure mode this repository keeps producing, and this one shipped with the guard rather
than after it.

Mutation: adding `probe.db` and `probe.yaml` to the index fails both rules.

**Caveat.** Go caches test results by package contents, and git state is not part of that.
A developer who adds a database and re-runs `go test ./...` gets a cached pass; `-count=1`
or CI's fresh checkout does not. This is a property of the test cache, not of the test.

## Where the tracked repositories are stored

Nowhere, and that is the design. `catalogue.Discover(root)` reads one directory, one level
deep, and takes anything with `specs/increment-plan.md` and a `.git`. There is no list, no
registration and nothing to keep in sync — adopting Canon is committing a file, and
dropping out is deleting it. Discovery by artifact is what ADR-0009 chose over a registry.

The known shortfall against R52 is unchanged: discovery is local paths only, so an
organisation on a remote host has to clone first. That is recorded, not fixed here.

## One more thing the ledger was wrong about

`docs-003` has been `in-review` since PR #43 merged, sixteen increments ago. Its work is on
main, all three criteria are ticked and its evidence file exists — the status was simply
never advanced. Flipped to `done` here.

Nothing caught it: `check-traceability.py` asks whether an in-review increment has a commit
carrying its trailer, which this one does. Nobody asks whether an increment has been
in-review for a fortnight. That is a conformance rule Canon could report and does not —
worth a finding of its own, not worth inventing inside a cleanup increment.

## Evidence

- `deploy/` removed entirely; 2.2MB of local database deleted
- `.gitignore` down to build output, agent-generated views and compiled test binaries
- `go test ./...` — all packages pass; `e2e/urls.mjs` and `e2e/keyboard.mjs` — all checks pass
- `docs/architecture.md` — 29 invariants, every named test exists
