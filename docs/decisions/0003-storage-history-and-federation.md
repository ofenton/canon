# 0003 — Storage, history and federation

**Status:** proposed — needs a decision at Gate 1
**Date:** 2026-08-22

## Context

Three requirements arrived together, and it turns out they have one answer:

1. The store must **preserve history** — who changed what, when, and why.
2. It must be **performant now and evolvable to scale**, with real backups.
3. Tickets might live **local to each repo, in git**, federated across repos with a cache — which
   would compose well with the increment ledger and a decomposable SDLC.

## Prior art, and what it cost people to learn

| Project | Approach | Outcome |
|---|---|---|
| **Bugs Everywhere** | Issues as files in the working tree | Merge conflicts on every concurrent edit. Abandoned. |
| **ticgit** | Issues in git | Its creator left and built GitHub instead. |
| **git-bug** | Issues as git objects, CRDT merge. Go, GPLv3. CLI + TUI + web UI + GraphQL, bridges to GitHub/GitLab | Mature and alive: **10k stars, 2,632 commits**. The reference implementation. |
| **grite** | Append-only event log in `refs/grite/wal`, CBOR, Ed25519-signed, CRDT, sled view. Rust, MIT, agent-first | Directly parallel to this proposal. **14 stars, 126 commits** — very early. |
| **Fossil** | Ticket change artifacts inside the SCM, replayed to reconstruct state | Shipped and stable for 15+ years. Sqlite's own tracker. |
| **GitHub Issues** | Repo-local, server-side | Cross-repo epics are a documented, unfixed failure. |

Two of these are decisive.

**Fossil rejected tickets-as-files for three reasons**, and they still hold:

> *"Check-ins in fossil are immutable. So if tickets were part of the check-in, then there would
> be no way to add new tickets to a check-in as new bugs are discovered."*

> Thousands of ticket files would clutter the source tree.

> *"We want tickets to be managed from the web interface and to have a permission system that is
> distinct from check-in permissions."*

The third is the one that kills naive repo-local storage. **Ticket permissions are not repo
permissions.** A support engineer filing a bug, a designer raising a defect, an outsider
reporting a vulnerability — none of them have, or should have, write access to the repository.
Making the tracker repo-local makes repo write access a prerequisite for participating.

**GitHub proves the cross-repo problem is real, not theoretical.** Backlinks do not work across
repos, checking an item in a project does not close the linked issue, and epics spanning a
frontend and a backend repo have no home. This is the single most-requested unfixed thing about
GitHub Issues, and it is a direct consequence of repo-local architecture.

## The insight

**An append-only event log solves all three requirements at once.**

Store events, not state: `issue.created`, `field.set`, `issue.transitioned`, `comment.added`.
Each event carries actor, timestamp, and provenance. Current state is a *projection* — replay the
log to get it, and rebuild the projection any time.

- **History** is not a feature bolted on; it is the storage model. There is nothing to "preserve"
  because nothing is overwritten.
- **Federation** becomes trivial, because appends do not conflict. Two agents editing the same
  issue in different clones produce two events, and merging is concatenation plus a deterministic
  order. No three-way merge, no manual resolution.
- **Scale** is a projection concern, not a storage concern. Swap SQLite for Postgres and rebuild
  the projection; the log is untouched.
- **Backup** is copying an append-only file.

Fossil reached this in 2007 (ticket change artifacts). git-bug and grite reached it via CRDTs.
It is the settled answer.

## Options assessed

**A. Centralised database, current-state rows.** Fast, simple, boring. Fails the history
requirement without bolt-on audit tables, and forecloses federation entirely. Rejected.

**B. Issues as files in the working tree, committed.** The obvious reading of "tickets in git".
Empirically the worst option: merge conflicts, source tree clutter, and it fails all three of
Fossil's objections. Rejected on evidence.

**C. Event log in a dedicated git ref.** Working tree stays pristine, travels with the code,
offline-capable, no server needed. What git-bug and grite do. Strong — but inherits Fossil's
permissions objection and the cross-repo epic problem.

**D. Centralised event log, server-owned.** Solves permissions and cross-repo cleanly. Loses the
git-native properties that made the idea interesting.

**E. Event log with a pluggable *home*.** ← **recommended.** One event format. The log can live
in a git ref *or* on the server, and the index projects across every source it knows about.

## Recommendation

**Federated by design, centralised by default.**

```
                 ┌─────────────────────────────────────────┐
   repo A ──┐    │            Index (projection)           │
   refs/    │    │  disposable · rebuildable · queryable    │
   canon/   ├───▶│                                          │◀── UI
   events   │    │  SQLite now → Postgres at scale          │◀── MCP / agents
            │    └─────────────────────────────────────────┘◀── CLI
   repo B ──┤                        ▲
            │                        │
   org      └────────────────────────┘
   stream        (server-owned events: no natural repo)
```

- **The event log is the source of truth**, wherever it lives. One format, one schema.
- **A repo can own its issues** in `refs/canon/events` — pristine working tree, offline, travels
  with the code, and merges without conflict because appends commute. This is the right home for
  engineering work, agent work, and the increment ledger itself.
- **The org stream is server-owned** and holds everything with no natural repo: support tickets,
  design defects, security reports, and **cross-repo epics**. Repo-local issues link to it.
- **Permissions attach to the stream, not the filesystem.** Filing a ticket never requires repo
  write access, answering Fossil's objection without giving up the git-native path.
- **The index is disposable.** It is a cache with no authority. Delete it and rebuild from the
  logs. This is what makes a projection bug a five-minute fix rather than a data repair script.

The thing this buys, which nothing else on the market has: **an issue can move**. Work that starts
as a repo-local increment can be promoted to the org stream when it turns out to span three
services, without changing its identity or losing its history — because the events are the same
events either way.

## Scope for the first week

Federation is the architecture, not the week-one deliverable. Building it now means shipping
plumbing and no product by Sunday.

**Build:** the event log, the projection, the schema validator, the API, the MCP server, and the
UI — with the server as the only log home.

**Design for, do not build:** the git-ref transport. Because the event log is the storage model
from day one, adding a second home later is a *transport*, not a rewrite.

**One thing must be right this week and cannot be deferred:** the event schema. Everything
federates on it. Getting it wrong means a migration of the only thing that was supposed to be
immutable.

## Build on existing work?

Both candidates solve *distributed storage of issues in git*. Neither attempts the problem Canon
exists for — an organisation's tracker where configuration cannot diverge. The overlap is the
mechanism, not the product.

| | git-bug | grite | Canon needs |
|---|---|---|---|
| Event/CRDT storage in git | ✅ | ✅ | ✅ |
| Org-wide schema as code | ✗ — fixed model | ✗ — fixed model | **the entire wedge** |
| Permissions ≠ repo access | ✗ | ✗ | ✅ (Fossil's objection) |
| Cross-repo epics / org stream | ✗ strictly repo-local | ✗ strictly repo-local | ✅ |
| Multi-user server | ✗ local web UI | ✗ no server | ✅ |
| MCP | ✗ | ✗ (CLI + JSON) | ✅ |
| Measured flow metrics | ✗ | ✗ | ✅ |
| Licence | **GPLv3** | MIT | Apache-2.0 intended |

**Building on git-bug** would inherit a proven storage layer, a web UI, GraphQL and working
GitHub/GitLab bridges. Two costs. First, **GPLv3 is viral** — linking it makes Canon GPLv3,
which contradicts the Apache-2.0 commitment in the spec and constitution rule 14. That is a
licence decision, not a technical one, and it is reversible only before the first release.
Second, its data model is fixed around bugs, comments and labels, with no schema layer; adding
`canon.yaml` means fighting the grain of the codebase.

**Building on grite** is licence-compatible (MIT) and its event model is the one recommended
here. But it is 126 commits old with a bus factor of one, it would force Rust as the
implementation language, and it supplies only the layer that is *easiest* to write. Everything
Canon actually needs — schema, permissions, org stream, server, UI, MCP — would still be ours.

**Decision: build the storage layer fresh, and interoperate rather than depend.**

The append-only log plus projection is a few hundred lines of well-understood code. The valuable
part of the prior art is the *design*, which is free to borrow: events in a dedicated ref, CRDT
last-write-wins with commutative sets, periodic snapshots to bound replay cost, per-actor
identity with optional signing. Take the lessons, not the dependency.

To keep the door open, Canon's event format should be close enough to import a git-bug or grite
repository, and an importer is a natural early contribution back to either community.

**Revisit if:** the Apache-2.0 commitment is dropped, in which case git-bug becomes a serious
foundation and the bridges alone are worth weeks; or grite matures enough that interoperating at
the event format is more valuable than owning the layer.

## Consequences

**Good.** History is inherent. Backups are a file copy. Scale is a projection swap. Federation
stays open without being paid for now. Agents get an audit trail for free, since every event
already carries provenance.

**Costs.** Event sourcing is more code than CRUD, and the projection must be kept correct.
Reads always go through a projection, so a rebuild is required after any projection bug. Event
schema changes need versioning discipline from the first commit.

**Watch.** Re-check both projects before any public launch. `grite` validates the direction and
may become worth interoperating with. `git-bug` is the incumbent in this space and its bridges
are the fastest path to Jira/GitHub migration if the licence question is ever revisited.
