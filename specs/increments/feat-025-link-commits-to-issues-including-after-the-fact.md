# feat-025: Link commits to issues, including after the fact

## Context

The `NOJIRA` problem is not carelessness. Policy demands a ticket for work that does not warrant
one, creating a ticket is expensive, and linking after the fact is impossible — so a placeholder
is the only move left. This increment removes the third constraint: a commit can be linked to an
issue at any time, carrying the timestamp it actually had, so the record can be made true later
instead of having to be true up front.

## Design notes

**The link is an event, not a field.** Who linked what, and when, is precisely what an audit asks
about; a field would keep only the answer. It also means a link is subject to the same actor
provenance as every other write.

**A repeat is a no-op, not an error.** The natural caller is a sweep over a commit range, which
re-sees everything it has already linked every time it runs. Making that an error would force
every caller to track its own history; the second-worst outcome — a log full of duplicate links —
is worse than doing nothing.

**Abbreviated and full hashes are the same commit.** Git abbreviates freely, so a sweep that used
short hashes last week must not re-link the same work with full ones today.

**Linking an old commit needs the `backdate` grant.** A commit's author time is nearly always in
the past, so a link is the ordinary case of a backdated write and goes through feat-023's check.
Without that, `canon link -range` over an old range would have been a way around the grant.

**`link` is its own verb, and deliberately cheap to grant.** It changes no state and gates nothing.
Member, agent and admin all hold it in the shipped schemas — a permission that made linking hard
would recreate the problem this exists to solve.

**Only a hash is required.** A link with nothing but a hash is still true and useful. Demanding
more would be the same mistake as demanding twelve fields to create an issue.

**Only the commit subject is stored.** Bodies are sometimes enormous and the log is not the place
for a second copy of the repository.

**Shelling out to git.** Everything here depends on real commit metadata; reimplementing object
parsing would be a much larger surface to get subtly wrong.

## Evidence

**Verified by:** implementing session, `inc/feat-025-link-commits`

### A sweep over Canon's own history

Twelve of Canon's own commits, linked into a fresh instance from the real repository:

```
$ canon link -actor ollie -repo ../tracker -range HEAD~12..HEAD

  no issue  99a4f1e  plan: build feat-025 before feat-024
  no issue  33c955c  plan: sequence the remaining Should requirements
  no issue  45f043d  Merge pull request #25 from ofenton/inc/feat-019-rich-fields
  no issue  2df57e4  Merge pull request #24 from ofenton/inc/feat-018-detail-view
    2599acb  feat-023: mark done at Gate 2  → FEAT-023
    51e1d79  feat-023: Backdated writes with an explicit timestamp (#27)  → FEAT-023
    e754ea2  feat-022: mark done at Gate 2  → FEAT-022
    8ed2e10  feat-022: Render checklists and multi-value fields (#26)  → FEAT-022
    194cdcb  plan: render checklists and multi-value fields (feat-022)  → FEAT-019
    8dbee8a  feat-019: merged in PR #25, mark done  → FEAT-019
    ...
linked 12 commit(s); 4 carry no issue reference
```

The four it could not place are named rather than dropped silently.

**It also caught a real mistake of mine.** `194cdcb`, whose subject says feat-022, resolved to
FEAT-019 — because the commit's trailer genuinely reads `Increment: feat-019`. I mislabelled that
commit when planning feat-022. The tool is right and the history was wrong, which is the entire
point of being able to see this.

### Original timestamps survive

```
$ curl .../api/issues/FEAT-019/commits

  2026-08-24T06:39:34  c452053  state: feat-019 -> in-progress
  2026-08-24T06:45:33  b5b760e  feat-019: checklist and multi-value fields
  2026-08-24T06:56:00  8dbee8a  feat-019: merged in PR #25, mark done
  2026-08-24T06:56:03  194cdcb  plan: render checklists and multi-value fields (feat-022)
```

Each carries the commit's own author time, not the moment of linking, and they read oldest first
regardless of the order `git log` handed them over in.

### Repeating a sweep

```
$ canon link -actor ollie -repo ../tracker -range HEAD~12..HEAD
  = 9f655f2  state: feat-018 -> in-progress  → FEAT-018
  = 7f5fa1e  feat-021: merged in PR #23, mark done  → FEAT-021

linked 0 commit(s); 12 already linked; 4 carry no issue reference
```

FEAT-019 still has exactly four commits.

The first version of this printed `linked 12` on the second run, having counted commits it
considered rather than events it wrote. `LinkCommit` now reports whether it wrote, the CLI counts
that, and a test asserts the second and third calls report no write. A sweep that says it did
twelve things when it did nothing is worse than no output.

### Tests

Twenty new tests. Ten in `internal/enforce` covering the decision — idempotence, abbreviated
hashes, the backdate interaction, what is and is not a commit id. Four in `cmd/canon` running the
real CLI against **real temporary git repositories with real author dates**, including a dry run
that must write nothing. One route-contract entry and the MCP descriptions, both of which the
existing structural tests demanded before they would pass.

Full suite green across all ten packages.

### Scope

`git diff --cached --stat main` — run. The link event and its enforcement, the projection, two API
routes, the MCP entries, the `canon link` command, and `link` added to the schema's closed verb
list and granted in both shipped schemas.

### Not verified

**There is no unlink.** A wrong link is a wrong fact in the log and there is currently no way to
retract it. The event model supports a counter-event; nothing emits one. This is the gap I would
close first.

**The inline id pattern is a guess at other people's conventions.** `CANON-12: fix the thing` is
matched because plenty of teams write that, but it is a heuristic — `Bump to v1.2-3` is excluded by
a test, and there will be conventions it reads wrongly. The explicit trailer always wins over an id
mentioned in passing, which is the case that matters.

**`canon link` writes to the log directly**, so it must not run against a database a server holds
open for writing. Every other write path is HTTP; this one is not, and that inconsistency is worth
revisiting when there is a remote client.
