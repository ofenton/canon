# feat-028: Full-text search

## Context

The query language could already filter — `team=platform`, `title~slow` — but there was no way to
just *search*. Typing a word into the box returned `term "reindex" has no value; write key=value`,
which is the tracker asking to be taught its own grammar before it will help.

## Design notes

**A bare word is a search.** No new syntax, no `search:` prefix, no second box. Somebody typing
`reindex` means "find reindex", and everything else in the language stays exactly as it was.

**A bare word that names a key is refused, and says both things it could mean:**

```
term "team" has no value; write team=value to filter, or "team" to search for the word
```

Typing `team` is almost certainly a half-written filter, not a search for the string "team".
Quoting says you meant the word. Without this, a mistyped filter would silently become a text
search returning plausible-looking nonsense — worse than an error.

**Search covers text, not structure.** Title, and every value the issue carries: fields,
multi-valued fields, checklist items. Not state, type or team — those are exactly addressable, and
a bare word matching them would make `done` an unusable search term.

**The id matches whole, not as a substring.** Every id in an instance shares a prefix, so substring
matching made `on` return every issue in `CANON-*`. Searching an id means "take me to this one",
which only ever wants the whole thing.

**Case-insensitive without allocating.** `strings.ToLower` on every value of every issue was the
entire cost of the scan; comparing folded bytes against an already-folded needle costs nothing.

## Evidence

**Verified by:** implementing session, `inc/feat-028-search`

### Against Canon's own instance

```
q=backdate               1 — feat-023
q=backdate state=done    1 — feat-023          search composes with filters
q=checklist              2 — feat-019, feat-022
q=team                   refused: term "team" has no value; write team=value to filter,
                                  or "team" to search for the word
q="team"                 2 — feat-014, feat-015
```

`backdate` and `checklist` appear in no title — they are matched from the `scope` field, which is
the "and bodies" half of the requirement.

### AC: under 200ms at p95 with 10,000 issues

Added to the existing latency budget, which fails CI on regression:

```
dataset: 10000 issues, 30003 events

  full-text search           p50     0.8ms   p95     0.9ms   ok
  search, no matches         p50     0.8ms   p95     0.9ms   ok
  search plus a filter       p50     0.8ms   p95     0.9ms   ok
```

Well inside the 200ms budget. The no-match case is the honest measurement: a search that matches
nothing cannot short-circuit and has to read every value on every issue, and it costs the same as
one that hits immediately.

### Tests

Four in `internal/query`: matching across titles, non-title fields and ids; composition with
filters and negation; the bare-key-name refusal and its quoted escape; and structural attributes
*not* matching. Plus three benchmark rows.

Full suite green across all ten packages.

### Scope

`git diff --cached --stat main` — run. `searchKey`, `searchHit`, `containsFold`/`equalFold` in
`internal/query`, the bare-word branch in `Parse`, four tests, three benchmark rows, and the UI's
`/` prompt renamed from "Filter" to "Search".

### The operator trap this run exposed

Not this increment's defect, but found by it. Restarting the instance after feat-030 failed:

```
canon: schema does not fit the existing log:
  removing team "Marketting" would strand 1 issue(s): CANON-39
```

Correct — I had created that issue earlier, while demonstrating that teams were unvalidated. But
**the only way to fix the data is through a server that will not start.** I got out by temporarily
declaring the typo team, deleting the issue, and removing the declaration again.

That works and is discoverable from the error, but it is a bad experience for something an
operator hits exactly when they are already in trouble. `canon repair` — an offline command to
reassign or delete stranded issues — is the missing tool. Recorded here rather than built, because
it is a different increment.

### Not verified

**No ranking.** Results come back in id order; a title match does not outrank a match buried in a
scope field. Fine at Canon's size and wrong at a large one.

**No word boundaries.** `on` matches "reindex **on** write" and also "rec**on**cile". Substring
matching is what `title~` already did and is predictable, but it is not what most people mean by
search.

**Single term only.** `reindex slow` is two search terms ANDed, which is probably right, but there
is no phrase search — `"reindex on write"` searches for the literal string including spaces only
because the shell passes it as one word; the query parser splits on whitespace and would treat it
as three terms. Quoted phrases are unimplemented.
