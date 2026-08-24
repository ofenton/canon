# fix-001: Resolve issue references case-insensitively

## Context

Found by starting the thing up. Importing Canon's own ledger and then linking its own commits
failed on the first commit:

```
canon: linking 45d2e42 to FEAT-026: unknown issue FEAT-026
```

Canon's ledger uses lower-case ids (`feat-026`); `issueFrom` upper-cased every trailer match on the
assumption that ids look like `CANON-12`. Nothing linked at all — 0 of 105.

**Worse than the failure: the two commands disagreed.** `canon trace` reported 92.8% tracked for
exactly the commits `canon link` refused as unknown, because `knownIssues` upper-cased both sides
while `link` compared the raw string against the projection. One tool said the work was tracked and
the other said the reference named nothing.

## Design notes

**Neither command guesses now; both ask.** `issueFrom` returns the reference as written — which id
that names is the resolver's business, not the parser's — and `knownIssues` returns a map keyed by
upper case and valued with the id as Canon actually holds it. `trace` classifies through it and
`link` writes through it, so they cannot disagree by construction.

**An unresolvable reference is reported as written**, so a reader can find it in the commit message
rather than a normalised form of it.

## Evidence

**Verified by:** implementing session, `inc/fix-001-case-insensitive-refs`

### The failing case, before and after

```
before:  canon: linking 45d2e42 to FEAT-026: unknown issue FEAT-026     (0 linked)
after:   linked 105 commit(s); 33 carry no issue reference
```

Against Canon's own repository and its own imported ledger.

### Tests

```
PASS  TestReferencesResolveWhateverTheirCase   — canon-1 and CANON-1 both link
PASS  TestUnknownReferenceIsReportedAsWritten  — feat-999 shown as feat-999
```

The first asserts both halves that used to disagree: `trace` calls both castings tracked, and
`link` actually links both. The existing 15 trace and link tests pass unchanged apart from two
`issueFrom` expectations, which now assert the reference is returned as written — that is the
behaviour change, so the assertion had to move with it.

Full suite green across all ten packages.

### Scope

`git diff --cached --stat main` — run. `issueFrom` stops upper-casing, `knownIssues` returns
upper→actual, `classify` and `canon link` both resolve through it.

### Not verified

**Two issues differing only in case would collide** in the resolution map, and the later id in
`IssueIDs()` order would win. Canon generates ids itself so this cannot arise from normal use, but
an import that created both `CANON-1` and `canon-1` would resolve one of them silently. Refusing
that at creation is the real fix and belongs with id generation, which has a separate known problem
(ids are `CANON-<n>` over a count of live issues, so a deletion can collide).
