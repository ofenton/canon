# feat-005: Issue entity with parent/child hierarchy

## Context

Constitution rule 9: all work is an `Issue` with an optional parent. Epics, stories and sub-tasks
are relations, not types. This increment adds the two structural operations that a tree needs and
a flat list does not — deletion and cycle prevention.

## Design notes

**Deletion lifts children to the grandparent.** The three options were cascade, orphan and lift.
Cascading destroys work nobody asked to destroy. Orphaning leaves children pointing at something
that no longer exists. Lifting preserves both the children and the shape of the tree around them,
and it is the only one of the three that never loses information.

**Each child's move is its own `issue.reparented` event**, not an implicit projection rule. The
history then says *why* a child changed parent — the events carry `because: parent STORY was
deleted`. In an append-only log the audit trail is the entire point of the storage model, so
inferring a change rather than recording it would give the projection knowledge the log lacks.

**Deletion is a tombstone.** `issue.deleted` removes the issue from the projected present; every
event stays in the log. History is not rewritten, which is also why a deleted issue's events
remain readable with `canon events -subject`.

**Cycle detection walks up from the proposed parent and reports the path.** `making EPIC a child
of SUB-1 would create a cycle: SUB-1 -> STORY -> EPIC` is actionable; "invalid parent" is not.
The walk also carries a seen-set, so an already-corrupt tree is refused rather than walked
forever.

**Both structural invariants are asserted against the source, not just behaviour.** One test
parses the log and fails if any event type contains `epic`, `story` or `subtask`; another parses
the projection package and fails if a type by those names is ever declared. Behavioural tests
prove today's code is right; these keep it right when someone later adds a convenience type.

## Evidence

**Verified by:** implementing session, `inc/feat-005-issue-hierarchy`

### THE SYSTEM SHALL store all work as a single Issue entity with an optional parent reference / no storage-level distinction between epic, story and sub-task

```
tree:
  EPIC
    STORY
      SUB-1
      SUB-2
```

```
--- PASS: TestHierarchyIsRelationsNotTypes (0.01s)
--- PASS: TestProjectionHasNoHierarchyTypes (0.00s)
```

Four levels of hierarchy, one entity, one event vocabulary. No event type and no projected type
encodes a level.

### WHEN an issue with children is deleted THE SYSTEM SHALL re-parent its children to that issue's parent

```
after deleting STORY (children lifted to EPIC):
  EPIC
    SUB-1
    SUB-2

SUB-1's recorded history:
  issue.created        <nil>
  issue.reparented     STORY
  issue.reparented     EPIC  (parent STORY was deleted)
```

```
--- PASS: TestDeleteReparentsChildren (0.00s)
--- PASS: TestDeleteRootLeavesChildrenParentless (0.00s)
--- PASS: TestDeleteIsRecordedNotErased (0.00s)
--- PASS: TestDeletedIssueCannotBeWrittenTo (0.00s)
--- PASS: TestDeleteUnknownIssue (0.00s)
```

Deleting a root leaves its children parentless rather than deleting them.

### WHEN a parent reference would create a cycle THE SYSTEM SHALL reject the write

```
making EPIC a child of SUB-1 would create a cycle: SUB-1 -> STORY -> EPIC
making STORY a child of STORY would create a cycle: STORY
```

```
--- PASS: TestRejectsCycles/self (0.00s)
--- PASS: TestRejectsCycles/direct (0.00s)
--- PASS: TestRejectsCycles/transitive (0.00s)
```

Each case also asserts the log was untouched, and that a legitimate move within the tree still
succeeds.

**One defect found and fixed during verification.** The first implementation reported
`SUB-1 -> STORY -> EPIC -> EPIC`, duplicating the closing node, because the walk appended the
child to a path that already ended at it. An error path that repeats a node reads as a bug in the
checker and undermines the message it exists to deliver. Fixed, and the test now asserts no node
appears twice in the reported path.

### Scope

`git diff --cached --stat main` — run:

```
 internal/enforce/enforce.go        |  75 ++++   Delete, cycle detection
 internal/enforce/hierarchy_test.go | 230 ++++   tests
 internal/projection/projection.go  |  10 ++    issue.deleted tombstone
 specs/increment-plan.md            |   2 +-    status
```

### Not verified

A deleted issue reports "unknown issue" rather than "deleted" on a subsequent write. Arguably
better — it does not confirm the id ever existed — but it is a behaviour someone may want changed,
so it is recorded rather than left to be discovered.

CI runs on the pull request.
