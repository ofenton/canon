# chore-005: Take the template's own updates, and record the reconciliation

## Context

Two things at once, and they are the same subject from opposite ends.

Canon was built with the agentic SDLC template. Today the template gained a `design-architecture`
skill and a `check-architecture.py` check, and this repository had not received them — so the
project that proved the template was running an older version of it than the template itself.

At the same time a contradiction surfaced in review: the template keeps work in git and declares
the repository the source of truth, Canon keeps work in an event log, and the same work was in
both. ADR-0005 and ADR-0006 record the reconciliation.

## The drift, measured

```
.sdlc/bin/validate-plan.py         identical
.sdlc/bin/check-traceability.py    identical
.sdlc/bin/check-tracked.py         identical
.sdlc/bin/link-agents.sh           identical
skills/  (6 of 8)                  identical
.sdlc/hooks/pre-commit             2 lines differ
AGENTS.md                          6 lines differ
skills/verify-increment            30 lines differ
.sdlc/bin/lint-workflows.py        exists here, absent from the template
```

Most of that is the template moving forward today, which is a template working. The interesting
case is `lint-workflows.py`: written **here**, in response to a real failure — a malformed workflow
file that failed remotely with no logs — and never propagated back. One improvement, made in a
copy, invisible to every other copy, after one week.

That is one data point, and ADR-0006 says so rather than inflating it.

## The measurement behind ADR-0005

```
347 events total
335 (96%)  reconstructible from specs/increment-plan.md + git log
 12  (3%)  not in git at all — 6 actors, 1 role, 4 team memberships, 1 token
```

`scripts/import-ledger.py` rebuilds the 335. **The importer is the proof**: a thing you can rebuild
from git is a cache of git. The twelve that are not are identity, which is not work tracking.

## Evidence

**Verified by:** implementing session, `inc/chore-005-sync-template`

### In step with the template, both directions

```
.sdlc/hooks/pre-commit   identical
AGENTS.md                identical
verify-increment         identical
```

`design-architecture`, `check-architecture.py` and the updated `verify-increment` came in;
`lint-workflows.py` went back.

### The new check, against this repository's real architecture

```
skills: ok — 8 skills valid
docs/architecture.md: ok — 16 invariant(s), every named test exists
```

### Scope

`git diff --cached --stat main` — run. The skill, the check, the hook, `AGENTS.md`, the two ADRs
and this record. No product code.

### Not verified

**Neither ADR is decided.** Both are `proposed`, both carry open questions, and nothing has been
built or removed on the strength of them. That is the point: they exist to be reviewed.

**The circularity in ADR-0006 is unresolved.** Canon uses the template and is the thing the
template would depend on. That needs a straight answer before either half of that decision is
built.
