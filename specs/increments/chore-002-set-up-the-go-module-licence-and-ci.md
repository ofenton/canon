# chore-002: Set up the Go module, licence and CI

## Context

First increment of the build. Establishes the toolchain that every later increment depends on,
per [ADR-0004](../../docs/decisions/0004-go-and-apache-2.md).

## Design notes

**A minimal `cmd/canon/main.go` was added**, which brushes against the "no application code"
scope. It is a `version` stub and nothing else. `make build` cannot produce a binary without a
main package, and the first acceptance criterion requires a binary to exist, so the alternative
was an increment that could not be verified. Recorded here rather than expanded silently.

**`CGO_ENABLED=0` is exported in the Makefile, not passed per-invocation.** A single accidental
cgo build would produce a dynamically linked binary and quietly break the self-host story, which
is the one property this increment exists to establish.

**CI asserts staticness rather than trusting the flag.** The `ldd` check on `ubuntu-latest` fails
the build if the binary ever becomes dynamically linked.

**A second CI job validates the ledger.** The pre-commit hook can be bypassed with `--no-verify`;
CI cannot. Cheap to add now, and it is what makes the ledger trustworthy in a pull request.

## Evidence

**Verified by:** implementing session, on `inc/chore-002-go-module-licence-ci`
**Go:** 1.26.3

### WHEN `make build` runs THE SYSTEM SHALL produce a single static binary with no external runtime dependencies

```
$ make build
go build -trimpath -ldflags '-s -w -X main.version=49301ab-dirty' -o bin/canon ./cmd/canon
$ ./bin/canon version
49301ab-dirty
```

Statically linked, verified on the deployment target by inspecting the ELF program headers of a
`GOOS=linux GOARCH=amd64 CGO_ENABLED=0` build:

```
ELF64=True  program headers=6
PT_INTERP (needs a dynamic linker): no
PT_DYNAMIC (dynamic linking):       no
=> STATIC
```

**Caveat, recorded rather than glossed:** on darwin the same build links `libSystem.B.dylib` and
`libresolv.9.dylib`. That is unavoidable for Go on macOS and is not true of the Linux target,
which is what self-hosting runs on. The CI assertion runs on Linux for this reason.

### WHEN `make test` runs THE SYSTEM SHALL execute the test suite and exit non-zero on failure

Checked in both directions rather than assumed:

```
with a deliberately failing test:  make test exit=2
with tests passing:                make test exit=0
```

### THE SYSTEM SHALL carry an Apache-2.0 LICENSE file at the repository root

```
$ grep -m1 'Apache License' LICENSE
                                 Apache License, Version 2.0, January 2004
$ wc -l LICENSE
     202 LICENSE
```

Retrieved verbatim from `apache.org/licenses/LICENSE-2.0.txt`.

### Scope — two violations, found after merge

This section originally listed the expected files from memory instead of running
`git diff --stat`. That is the whole reason the check exists, and skipping it let two things
through:

1. **`internal/event/event.go` and `internal/event/sample_test.go` shipped under chore-002.**
   They are feat-001's work. A `git add -A` swept them in from the working tree while both
   increments were in flight. The code is correct and feat-001 was already approved, so it has
   not been reverted — reverting to re-add identical code would be churn. Recorded here, and
   feat-001's evidence notes that its schema portion landed early.
2. **A 2.4 MB `canon` binary was committed at the repository root.** `/bin/` was ignored but a
   stray root-level build was not. Removed, and `/canon` added to `.gitignore`. The blob remains
   in history; stripping it is cheap now and expensive after the repo is public.

Root cause in both cases: `git add -A` with two increments' work in one tree, and a Scope check
that was written rather than run.

### CI, on Linux — https://github.com/ofenton/canon/pull/1

Run [32600923478](https://github.com/ofenton/canon/actions/runs/32600923478): both jobs green in 38s.

```
success  build
success  ledger
```

The static-binary assertion, run on the actual deployment target rather than inferred:

```
bin/canon: ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, stripped
```

### Defects found during verification

CI failed on its first run and this increment was returned to `in-progress`. Two defects, both
fixed on this branch:

1. **The workflow YAML did not parse.** `echo 'run: make fmt'` put a colon-space inside a plain
   YAML scalar. This fails remotely as a "workflow file issue" — 0s, no jobs, no logs, and
   `gh run view --log-failed` returns "log not found", which is a slow thing to diagnose.
   `.sdlc/bin/lint-workflows.py` now parses every workflow, wired into `make check`.
2. **`bin/` in `.gitignore` matched any directory named `bin` at any depth**, silently swallowing
   `.sdlc/bin` so a new script there could not be added. Anchored to `/bin/`. Already-tracked
   files were unaffected, which is what made it quiet.

### Not verified

Nothing outstanding.
