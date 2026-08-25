# feat-037: Conformance, reported not enforced

## Context

Under ADR-0009 enforcement stays at the edge and visibility becomes central. An aggregator cannot
decline a commit that has already happened; `validate-plan.py` refuses in the repository's hook and
CI, where refusing works. This runs the same rules everywhere and says who is failing them.

## Design notes

**Nothing is fatal.** A repository with fifty findings still ingests. A report that stops at the
first problem is a report nobody can act on, and the repositories most worth reporting on are
precisely the ones that fail.

**Three severities, and the distinction is load-bearing.** An illegal status is an `error`. Data that
is misleading rather than malformed is a `warning`. The untracked-commit ratio is a `note` — work
that genuinely needs no increment is the Direct track and is normal, and forcing an increment for
every typo is what produces placeholder references in the first place. A report where everything is
an error is a report people learn to ignore.

**`canon conform` exits non-zero only on an error**, so it is usable in CI without a note about
commit discipline failing somebody's build. `-strict` includes warnings.

**The rules are a second copy, and this says so.** Canon reads repositories it does not control and
cannot assume a current copy of the template's Python is present. ADR-0006 proposes distributing
these from one place; until then the duplication is named in the source rather than hidden.

## The rule no repository can check for itself

```
warning  —  cycle time understates the work: 6 of 26 increments record
            in-progress within 2m0s of in-review, so it measures two
            commits rather than the work. Set in-progress before
            starting, not alongside the result
```

This is a property of how a team runs the loop, not of any one commit, so no local check can see
it. It came out of feat-036, where cycle time read p50 14m against a lead time of 3.1h, and the
cause was `in-progress` being committed alongside the finished code — which `implement-increment`
tells you not to do at step 2 of its workflow.

An aggregator that can say *"your cycle times are meaningless and here is why"* is doing something
the repository cannot do for itself. That is the argument for the centre existing, in one finding.

## The defect running it found

First run against this repository:

```
error    54
error    chore-001    has no test strategy
error    chore-002    has no test strategy
error    feat-001     has no test strategy
...
```

Every increment in this repository has a Test Strategy. They are written as indented sub-lists under
an empty inline value — as Test Strategy and Acceptance Criteria always are — and the parser was
reading only the inline part.

**Fifty-four false errors.** A conformance report that cries wolf on every increment is worse than
no report, because the first thing anybody does with it is stop reading. The parser now captures
continuation lines, and `TestMultiLineFieldValuesAreCaptured` and `TestMultiLineFieldsCountAsPresent`
hold both ends of it.

## Evidence

**Verified by:** implementing session, `inc/feat-037-conformance`

### AC: a failing rule is named with its increment, and the others continue

```
$ canon conform .

Canon
  54 increments · 63 requirements · head b37d9ef

  warning  1
  note     1

  warning  —  cycle time understates the work: 6 of 26 increments ...
  note     —  9 of 143 commits (6%) carry no increment reference and
              were not declared untracked
```

Zero errors, and the two findings are both real. `TestOneBadIncrementDoesNotStopTheReport` asserts
that two bad increments in a ledger are both reported rather than the first ending the run.

### AC: a trace to a requirement that does not exist is reported

`TestDanglingTraceIsReported` — R99 reported, R1 not. A dangling trace looks fine to any grep and
traces nothing.

### AC: the proportion of commits carrying no reference, per repository

```
9 of 143 commits (6%)
```

Cross-checks against the template's own `check-traceability.py`, which independently reports
"11 of 143 commits (8%)" — the difference being the two commits this counts as *declared* untracked
via the `Untracked:` trailer, which that check does not yet distinguish. Two implementations, one
number, and the discrepancy explained rather than averaged.

### Tests

Eight in `internal/conform`: a bad increment not stopping the report, dangling traces, reference
discipline as a note, the cycle-time warning firing and correctly *not* firing, the WIP limit, a
clean repository reporting nothing, and multi-line fields counting as present. Plus one in
`internal/ingest` for the parser fix.

Full suite green across thirteen packages.

### Scope

`git diff --cached --stat main` — run. `internal/conform`, `ingest.Commits`, the parser's
continuation handling, `canon conform`, and tests.

### Not verified

**One repository at a time.** `canon conform <path>` takes a directory; reporting across an
organisation is `feat-038`.

**No cycle detection in dependencies.** `validate-plan.py` refuses dependency cycles; this reports
only dependencies that do not resolve. A cycle would pass conformance here and fail locally, which
is a gap between the two copies of the rules.

**Placeholder text is not detected.** `validate-plan.py` catches unfilled template text —
`<angle brackets>` and unchosen `a | b | c` options. Not ported, so an increment full of template
boilerplate conforms.

**The duplication is real.** These rules exist twice, in Python and in Go, and nothing keeps them in
step. ADR-0006 is the answer and is not built.
