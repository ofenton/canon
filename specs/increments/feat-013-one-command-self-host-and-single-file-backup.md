# feat-013: One-command self-host and single-file backup

## Context

The self-host promise: one binary, one command, one file. Two of those three were already true.

## The finding

**"Copy the data file" was not a valid backup, and the README said it was.**

SQLite runs in WAL mode — chosen in feat-001 so readers are not blocked during appends and so the
file can be copied while running. The second half of that reasoning was wrong. WAL keeps recent
commits in a `-wal` sidecar until a checkpoint folds them into the main file, so copying
`canon.db` alone captures only what has been checkpointed. On a young database that is almost
nothing:

```
--- PASS: TestPlainCopyOfTheMainFileLosesData
    a plain copy of canon.db recovered 0 of 500 events — the rest were in the WAL
```

Zero of five hundred. Copying all three files instead trades one hazard for another: without
pausing writes they can be captured at different instants and produce a torn backup.

`VACUUM INTO` takes a read transaction and writes a single defragmented, internally consistent
file without blocking writers. That is what makes "keep this one file" true, so `canon backup`
does that and the README now says so.

## Design notes

**Backups are never overwritten.** `VACUUM INTO` refuses an existing destination and the command
checks first with a clearer message. A backup that silently replaces an earlier one is how people
lose the good copy.

**The command reports what it captured** — event count, size, duration, and the restore command —
because a backup you cannot verify is a hope.

**`Checkpoint()` is exposed separately** for archiving a stopped instance, where folding the WAL
in and discarding the sidecars is the right move.

## Evidence

**Verified by:** implementing session, `inc/feat-013-selfhost`

### One documented command starts a working instance with no external service dependencies

From a directory containing only the binary and a schema:

```
$ ls
canon    canon.yaml

$ ./canon bootstrap -actor ollie -team platform
registered ollie as admin in team platform

$ ./canon serve -addr :8091
created 5 issues; server still running
```

No database to provision, no runtime to install, no reverse proxy. The static-binary assertion
from chore-002 still runs in CI.

### All data in a single file, copyable while running, restoring to identical state

```
$ ./canon backup -out backup.db          # taken while the server was serving
wrote backup.db (13 events, 20.0 KiB) in 1ms
restore with: canon serve -db backup.db

restored instance sees: 5 issues
original instance sees: 5 issues
```

```
--- PASS: TestBackupIsConsistentWhileWriting
    backup captured 1248 events, taken while writes were in flight
--- PASS: TestBackupNeverOverwrites
--- PASS: TestPlainCopyOfTheMainFileLosesData
```

`TestBackupIsConsistentWhileWriting` runs a writer goroutine throughout, then asserts the backup
is a single file with no `-wal`/`-shm` sidecars, that every event decodes, and that all 500
events written before the backup are present.

### Scope

`git diff --cached --stat main` — run. Backup in `internal/event`, the command in `cmd/canon`,
and a README correction.

The README change is in scope by necessity: this increment discovered that a claim already in the
README was false, and shipping the fix while leaving the false claim would be worse than either.

### Not verified

Restore is tested by opening the backup and reading it, not by a full disaster-recovery drill on
another machine.

`Checkpoint()` is implemented and unit-tested only indirectly; no command exposes it yet.

CI runs on the pull request.
