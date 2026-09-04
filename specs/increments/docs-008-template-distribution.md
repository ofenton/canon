# docs-008: Settle how the template is distributed

**Traces:** R70

## Why it needed settling

ADR-0006 has been `proposed` since 24 August, and its chosen option stopped existing two days
later. It decided *"B for the mechanism, C for the rules"* — where C was "the validators read
states and types from the org's `canon.yaml`". ADR-0009 deleted `canon.yaml`.

So the ADR's own consequence — *"`validate-plan.py` stops holding `STATUSES` and `TYPES` as
literals"* — is now backwards. Those literals **are** the convention, in the file that enforces it,
with no indirection to a service that might be unreachable.

An ADR left proposed while the ground moves under it is worse than one that was never written,
because it reads as a decision.

## What was built

`adopt.sh` in the template. It does not copy the template — it classifies it, executing the table
ADR-0006 already contained but never acted on:

| Class | On every run |
|---|---|
| **managed** — `.sdlc/`, `skills/` | Replaced. An improvement that does not reach a project is one that project does not have. |
| **seeded** — `AGENTS.md`, `specs/`, `docs/` | Written only when absent. Then it is the project's, and drift is reported, never fixed. |
| **merged** — `.gitignore` | Template rules appended in a marked block; everything else untouched. |
| **skipped** — `README.md`, the example | The project's front door, and content that would appear in a real ledger as work nobody did. |

Reporting rather than enforcing is the same posture Canon takes to conformance, for the same
reason: the tool cannot know whether a project's `AGENTS.md` differs because it is stale or because
that project needed something. It can make the difference visible, which is what a copy could never
do.

## Three bugs, all found by testing rather than reading

**Every guard refused every repository under a symlink.** `init.sh`, `hooks/install.sh` and the new
installer all compared `git rev-parse --show-toplevel` — a *physical* path — against a logical one.
On macOS `/var` is a symlink to `/private/var`, so anything under it never matched and was refused
as "inside another repository", naming the same directory twice. Three files, one bug, present
before this increment.

The dry run against Puzzlo passed, because `/Users` is not symlinked. Only pointing the script at a
real path under a symlink found it — which is what the test suite does.

**The worked example arrived in the ledger.** The example increment's *record* was never copied,
and my test asserted exactly that and passed. The seeded **ledger** still listed `sec-001:
Parameterize the search query` — so Puzzlo, an iOS puzzle game, adopted the workflow with a planned
SQL injection fix in it. The record and the ledger entry are two different things, and the check now
covers both. Only stripped when the run seeded the ledger and the entry is still `planned`: a tool
must never delete a `sec-001` that turned out to be real.

**My own duplicate check could not fail.** `grep -F` with an embedded newline treats it as two
separate patterns and matches when either is present, so a check for `__pycache__/` appearing twice
matched it appearing once. Counted now.

## Tested on a repository that already existed

**Puzzlo** — SwiftUI iOS app plus an AWS serverless backend, 109 files, 60 commits, shipped. It has
a README somebody wrote and a `.gitignore` tuned to Xcode: exactly the two files a copy destroys.

- `README.md` untouched. `.gitignore` gained five rules and kept every one of its own.
- All five validators pass against Swift, Python and Terraform with no modification.
- Running the installer twice changes nothing.
- Canon now reads three sources from one directory: Canon, Puzzlo, and a repository with no commits
  — which it reports rather than dropping.

## What the adoption then proved about the loop

Puzzlo's `chore-001` is **`in-review`, not `done`.** Four criteria are met and evidenced. The fifth
is *"THE SYSTEM SHALL provide a `docs/constitution.md` agreed by a human"* — and nobody has read it
for Puzzlo. I tried to mark the increment done anyway, and `validate-plan.py` blocked the commit:
*status is done but criterion is unticked*, *status is done but Evidence is empty*.

Marking it done would have been one keystroke and a lie. That the template caught it, in a
repository that had adopted it forty seconds earlier, is the most useful thing this increment
produced.

## Not done

**Who owns compatibility** is deferred, with a reason rather than by omission. Two projects is not
enough to have the problem, and `.sdlc/VERSION` makes it answerable when it is: a project can be
asked what it is on, and an upgrade is an explicit act with a diff. Inventing a compatibility policy
for forty projects while running two would be process for a problem nobody has.

**Puzzlo appears in Canon as `<Product / feature name>`**, because its `specs/product.md` is still
the template's placeholder. Honest, and ugly. Canon reports it as a warning, which is right, but
the catalogue would read better if an unwritten spec said so in words.

## Evidence

- `docs/decisions/0006-distributing-the-template.md` — accepted, with C withdrawn and all four open
  questions answered or explicitly deferred
- `template/.sdlc/bin/adopt.sh`, and `tests/adopt.sh` — 21 checks, all passing
- Puzzlo: adopted, all validators green, `chore-001` held at the human gate
