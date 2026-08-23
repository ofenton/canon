# feat-002: Projection engine with snapshots

## Context

Current state is a projection over the event log ([ADR-0003](../../docs/decisions/0003-storage-history-and-federation.md)).
The projection is a cache with no authority: discard it and rebuild, any time.

## Design notes

**An unknown event type fails the rebuild; it is never skipped.** Silently ignoring an event
means the projection quietly disagrees with the log, which is the one failure a rebuildable cache
cannot detect on its own. The same applies to an event referring to an issue that was never
created — that is an inconsistent log, and refusing it is the point.

This caught a bug in this increment's own throughput fixture, which transitioned `CANON-0` before
creating it. The projection was right and the test was wrong.

**`Snapshot()` returns a digest, not a struct.** Determinism is the property that makes a
rebuildable cache trustworthy: if two replays of one log disagree, every downstream answer is
suspect. A digest makes that a one-line assertion instead of a deep comparison.

**`EventsRead()` exists for the tests.** Asserting a checkpoint bounds replay requires observing
how many events were actually read; without it the test would pass whether or not the checkpoint
did anything.

**Provenance is projected, not merely stored.** `LastActor` carries the actor kind and model, so
"which agent last touched this" costs no replay to answer, and cycle time comes from the
projected `Transitions` list rather than a re-scan of the log.

**Checkpoints deep-copy.** A shallow copy would share the fields map and transition slice with
the live projection, so restoring a checkpoint would mutate the thing it restored from.

## Evidence

**Verified by:** implementing session, `inc/feat-002-projection-engine`

### WHEN `canon rebuild` runs THE SYSTEM SHALL discard all projections and reproduce identical state from the event log

```
$ canon rebuild -db /tmp/demo2.db
replayed 2 events in 0s
digest 89e481f69a72c4c8
$ canon rebuild -db /tmp/demo2.db
replayed 2 events in 0s
digest 89e481f69a72c4c8
```

```
--- PASS: TestRebuildIsDeterministic (0.01s)
--- PASS: TestProjectedState (0.00s)
--- PASS: TestIncrementalMatchesRebuild (0.00s)
```

`TestRebuildIsDeterministic` asserts three things: rebuilding twice agrees, a fresh projection
agrees with a rebuilt one, and incremental catchup agrees with a full rebuild.

### WHEN a snapshot exists THE SYSTEM SHALL replay only events after it

```
--- PASS: TestSnapshotBoundsReplay (0.00s)
```

Restores a checkpoint, appends one event, and asserts catchup read exactly one event — so the
checkpoint demonstrably bounded the replay rather than appearing to.

### THE SYSTEM SHALL rebuild projections for 10,000 events in under 5 seconds

```
projection_test.go:222: rebuilt 10000 events in 11ms (871393 events/sec)
--- PASS: TestRebuildThroughput (0.07s)
```

450x under budget.

### Scope

`git diff --cached --stat main` — run:

```
 cmd/canon/main.go                        |  48 ++-    rebuild subcommand
 internal/projection/projection.go        | 285 +++    the engine
 internal/projection/projection_test.go   | 236 +++    tests
 specs/increment-plan.md                  |   2 +-     status
```

No files outside Scope. No binaries staged.

### Not verified

Nothing outstanding. CI runs on the pull request.
