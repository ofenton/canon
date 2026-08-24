# feat-026: Untracked work as a counted category

## Context

`NOJIRA` is a symptom of a policy that demands a ticket for every commit and offers no way to say
"this one does not need one". People comply in the only way left to them: they type something
ticket-shaped that means nothing. Stricter enforcement produces junk tickets *on top of* the
placeholders.

feat-024 made a ticket cheap to create; feat-025 made the record correctable afterwards. This makes
the number visible, so an organisation can decide what it will tolerate instead of pretending it is
zero.

## Design notes

**Five categories, not two.** A single "has a ticket?" check flattens things that ask for different
responses:

| | Means | What to do |
|---|---|---|
| tracked | reference resolves to a real issue | nothing |
| declared untracked | `Untracked: <reason>` | nothing — this is the sanctioned form |
| placeholder | `NOJIRA` and friends | the policy is not working |
| unknown issue | reference resolves to nothing | looks fine to any grep, and is not |
| unexplained | nothing at all | link it, or declare it |

**`Untracked: <reason>` is the honest replacement for `NOJIRA`.** A reason is required — a bare
`Untracked:` is `NOJIRA` with extra characters, and a test asserts it falls through to unexplained.

**Declared work is still counted as untracked.** It is separated in the headline, not excused. The
point is a true number, and "we decided not to track 12% of our commits" is a legitimate answer that
an org should be able to see and defend.

**`unknown issue` is its own category** because it is the case a regex-based check gets wrong: a
commit naming `CANON-999` looks tracked to any grep and tracks nothing. Deciding this requires
Canon's knowledge, not git's.

**The gate is on unexplained work only.** `-max-untracked-pct` ignores declared work, because
declaring is the behaviour this is trying to encourage and gating it would push people straight back
to placeholders.

**No log is not an error.** A missing database means references are taken at face value and the
proportions still print, so this is usable in CI on a fresh checkout.

## The defect running it for real found

The first version counted merge commits, and produced this against Canon's own history:

```
  tracked               15   11.0%
  unexplained           33   24.3%
```

Twenty-five of those 33 were `Merge pull request #NN from ofenton/inc/...`. A merge commit is not
work — the commits it joins are, and they are already in the range. A report that says a quarter of
your work is unexplained when most of that is merges is a number people learn to ignore, which is
worse than no number. `--no-merges` is now the default, with `-merges` to include them, and a test
covers both.

## Evidence

**Verified by:** implementing session, `inc/feat-026-untracked-report`

### Against Canon's own 109 commits

Seeded from the real ledger's 34 increment ids:

```
$ canon trace -repo ../tracker -range HEAD

109 commits in HEAD

  tracked              101   92.7%
  unexplained            8    7.3%

  carrying no working issue reference: 7.3%

unexplained
  f342e6f  ignore local runtime state, and check nothing ignored is tracked
  44f3a59  fix check-tracked output parsing for NUL-separated fields
  f725d32  document the proposal routes in the README
  ab06187  describe the board routes for MCP, broken by merging #12 and #13 together
  4e147be  remove a stale not-built claim that survived merging #14 and #15
  fd29ac0  Revert "plan: hierarchy, dependencies, detail view and richer fields"
  33c955c  plan: sequence the remaining Should requirements
  99a4f1e  plan: build feat-025 before feat-024
  → canon link -issue <id> -commit <sha>, or say why: Untracked: <reason>
```

**This independently agrees with the SDLC template's own validator**, which reported "8 of 108
commits (7%) carry no Increment trailer" from a completely separate implementation. Two tools
built for different purposes arriving at the same eight commits is the strongest evidence here.

All eight are genuine Direct-track work — the template's own category for changes too small to
warrant an increment. They are exactly what `Untracked: <reason>` exists for.

### Tests

Eight new tests in `cmd/canon`, over real temporary git repositories including one with a real merge
commit:

```
PASS  TestTraceReportsTheProportion                  — 60.0% of a five-commit fixture
PASS  TestTraceSeparatesDeclaredFromUnexplained      — (20.0% declared, 40.0% not)
PASS  TestTraceNamesTheUnexplainedCommits
PASS  TestTraceCatchesReferencesToIssuesThatDoNotExist
PASS  TestTraceGateCountsOnlyUnexplainedWork         — passes 25%, fails 10%
PASS  TestTraceWorksWithoutALog
PASS  TestTraceIgnoresMergeCommitsByDefault
PASS  TestClassifyPrefersTheMostSpecificReading      — 9 precedence cases
```

Full suite green across all ten packages.

### Scope

`git diff --cached --stat main` — run. The `canon trace` command and its tests, `readCommits`
extended to take git flags, and the usage text.

## The API route was dropped, deliberately

The plan said "exposed as `canon trace` and an API route". I did not build the route, because it
cannot answer the question. R28 asks for the proportion of commits carrying no reference — the
denominator is every commit in a range, and **the server has no repository to read.** It can only
see commits somebody already told it about, which is the numerator. An API route would have
reported a proportion of the tracked set against itself and always been near 100%: a number that
looks like an answer and is not.

The honest options are a client that sends the commit list (in which case the client did the git
work and the route is a formatting service) or a server-side checkout (a different product). Both
are worse than a CLI, so the ledger entry is corrected rather than the route faked.

### Not verified

**Declared and placeholder counts are zero in Canon's own history**, so those two paths are proven
by fixtures rather than by real data. The fixture covers each, but no real repository has yet been
run through them.

**`n/a` is in the placeholder pattern** and is the loosest of the five — a commit legitimately
containing "n/a" in prose would be misread. The narrower patterns (`NOJIRA`, `NO-TICKET`,
`NO-ISSUE`) carry the argument; this one is a judgement call I would drop if it produced noise.

**`canon trace` reads the log directly**, the same remote-server gap as `canon new` and `canon link`.
