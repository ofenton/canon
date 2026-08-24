# feat-024: Create an issue from a repository in one command

## Context

feat-025 made the record correctable after the fact. This makes it cheap to be correct in the
first place. A ticket that costs one command and one argument is a ticket people will actually
create; `NOJIRA` exists because the alternative costs a browser, a project picker and twelve
fields.

```
$ canon new "Search is slow" -actor ollie

CANON-1  Search is slow
  on main in https://github.com/acme/widgets.git
  linked fb08756  Cache the query plan

put this in your commit message:

  Increment: CANON-1
```

## Design notes

**The title comes first**, so the common case reads like a sentence. Flags may still come first
for anyone who prefers it, in which case the leftovers are the title — a command usable only one
way is a command people get wrong.

**It prints the trailer ready to paste.** That is the point of the whole thing: the next commit
carries the reference, and nothing has to be typed twice or remembered.

**Origin is recorded as a link, not as fields.** An org's `canon.yaml` is unlikely to define
`branch` or `repository`, and adding them per project would be exactly the accretion this product
refuses. The commit link from feat-025 is already the right place, which is why this increment was
resequenced to build after it rather than before.

**Outside a repository it still works.** No commits, no git at all, wrong directory — the issue is
created and the origin fields are simply empty. Refusing to file a ticket because the shell was in
the wrong place would be a silly reason to send somebody back to a browser.

**A failed link warns rather than fails.** The issue exists, which is the thing that had to
succeed. Failing at that point would leave a developer unsure whether to run the command again.

**Ids and default types moved into the domain.** `nextID` and `defaultIssueType` were methods on
the HTTP server; `canon new` needed the same answers. Two implementations of "what is the next id"
is precisely the drift this product argues against, so both now ask `Enforcer`.

## The defect this increment found

feat-023 refused any write dated before its issue was created, on the reasoning that an issue's
history cannot begin before the issue does. That is right for the issue's own events and wrong for
a commit link — **a commit routinely predates the ticket that tracks it, which is the entire NOJIRA
case.** Folding both rules into one check made recording real history impossible.

The check is now two:

- `AuthoriseBackdate` — the future check and the `backdate` permission, applied to every backdated
  write including links.
- `CheckNotBeforeCreation` — the creation floor, applied only to an issue's own events by the API's
  write path.

`TestACommitMayPredateTheIssueThatTracksIt` asserts both halves: the link is accepted, and the
issue's own history is still fenced.

## Evidence

**Verified by:** implementing session, `inc/feat-024-canon-new`

### The three acceptance criteria

Six new tests in `cmd/canon`, driving the real CLI against real temporary git repositories:

```
PASS  TestNewNeedsOnlyATitle              — id, title and trailer printed
PASS  TestNewRecordsWhereItWasRun         — branch, remote and commit recorded
PASS  TestNewWorksOutsideAGitRepository   — created, with nothing linked
PASS  TestNewWithoutATitleSaysHowToCallIt — error shows the command's shape
PASS  TestNewIssuesGetSequentialIDs       — CANON-1, CANON-2, CANON-3
PASS  TestNewAcceptsFlagsBeforeTheTitle
```

`TestNewRecordsWhereItWasRun` asserts the link carries `2026-03-02T10:00:00Z` — the commit's own
author time, five months before the issue was created. That assertion fails against the old
one-check rule, which is how the defect above was found rather than assumed.

Full suite green across all ten packages.

### Scope

`git diff --cached --stat main` — run. The `canon new` command and its tests, the backdating check
split in two with its test, `NextIssueID`/`DefaultIssueType` moved into `enforce` with the API
delegating, and the usage text.

### Not verified

**`canon new` writes to the log directly**, like `canon link`, so it must not run against a
database a server holds open for writing. For a developer in a product repository the tracker will
usually be remote, and neither command can talk to one yet. This is the most significant gap in
both increments and it is not planned work — a `-server` mode is what would make these commands
usable as designed.

**Ids are `CANON-<n>` with the prefix hardcoded**, and `n` is the count of live issues, so deleting
an issue can make the next id collide with one already used. That predates this increment; moving
the function did not fix it, and it is worth an increment of its own.

**`-parent` is accepted but only lightly exercised** — one path, no test asserting hierarchy rules
are applied through this command. They are applied by `ReparentAs`, which is tested elsewhere.
