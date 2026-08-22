---
name: track-increment-state
description: Reads and updates the increment ledger at specs/increment-plan.md — the single source of truth for what is planned, in progress, verified and shipped. Use before starting any work to find the next increment, whenever an increment changes status, and whenever asked "where are we", "what's next", or "what's the status". This skill is the only writer of increment status.
license: Apache-2.0
allowed-tools: Bash(python3:*) Bash(git:*) Read Edit Write Grep Glob
---

# Tracking increment state

The ledger is `specs/increment-plan.md`. It is git-tracked markdown so state survives session
restarts, is reviewable in a PR, and is readable by humans and agents alike.

Each increment also has a detail file at `specs/increments/<id>-<slug>.md` holding the full
record and its evidence. The ledger holds the summary; the detail file holds the depth.

## Before you do anything

```bash
cat specs/increment-plan.md
python3 .sdlc/bin/validate-plan.py
```

If the validator reports errors, **fix the ledger before doing any other work**. A malformed
ledger means every downstream agent is working from bad state.

## Status lifecycle

```
planned ──approve──▶ approved ──start──▶ in-progress ──submit──▶ in-review ──accept──▶ done
   │                    │                     │                      │
   └────────────────────┴─────────────────────┴──────────────────────┴──▶ abandoned
```

| Status | Means | Set by |
|---|---|---|
| `planned` | Drafted, not yet agreed | `plan-increments` |
| `approved` | Human said build it (**Gate 1**) | Human |
| `in-progress` | Branch open, code being written | `implement-increment` |
| `in-review` | Acceptance criteria met, awaiting sign-off | `verify-increment` |
| `done` | Signed off and merged (**Gate 2**) | Human |
| `abandoned` | Dropped — keep the row, record why | Human |

Transitions not on this diagram are illegal. Only a human moves an increment to `approved` or
`done`; those are the two gates. If you believe an increment is ready, move it to `in-review`
and say so — do not mark it `done` yourself.

**At most one increment may be `in-progress`.** The validator enforces this. It is a WIP limit,
and it is the main thing stopping an agent from half-finishing four things.

## Increment format

Every increment in the ledger uses exactly this template. The canonical copy is
[.sdlc/templates/increment.md](../../.sdlc/templates/increment.md); read it before writing one.

```markdown
## sec-001: Remediate SQL injection in search endpoint

- **Type:** security
- **Status:** approved
- **Tier:** 2 (High)
- **Traces:** SEC-007
- **Scope:** Parameterize the SQL query in `SearchService.search()`. No other changes.
- **Acceptance Criteria:**
  - [ ] WHEN a search query contains SQL metacharacters THE SYSTEM SHALL treat them as literal text
  - [ ] WHEN a search query contains a single quote THE SYSTEM SHALL return matching rows without error
  - [ ] THE SYSTEM SHALL return identical results for the queries in `tests/fixtures/queries.json`
- **Test Strategy:**
  - Add injection test: malicious input is safely escaped
  - Add boundary test: legitimate special characters still work
  - Run full regression suite
- **Dependencies:** none
- **Rollback Plan:** Revert `SearchService.search()` to previous implementation
- **Risk:** Low — isolated change to one method
- **Evidence:** _(filled in at verify)_
```

Acceptance criteria are written in EARS (`WHEN … THE SYSTEM SHALL …`) so that each one names a
trigger and an observable response, and translates directly into a test.

The two fields that do the most work are **Scope** and **Rollback Plan**. Scope ending in
"No other changes." is what keeps an agent from drifting. A rollback plan you cannot write in
one line means the increment is too big — split it.

## Making a change

1. Read the ledger and the increment's detail file.
2. Confirm the transition is legal (see lifecycle above).
3. Edit `specs/increment-plan.md` — change `Status:`, tick acceptance criteria, fill `Evidence:`.
4. Mirror anything substantive into `specs/increments/<id>-<slug>.md`.
5. Run `python3 .sdlc/bin/validate-plan.py` and `python3 .sdlc/bin/check-traceability.py`.
   Fix errors, re-run until both are clean.
6. Commit:
   ```bash
   git commit -m "state: sec-001 → in-review" -m "Increment: sec-001"
   ```

Never batch several status changes into one commit — the git log is the audit trail of how the
work actually progressed.

## Answering "what's next"

Report, in this order:
1. Any increment `in-progress` (there is at most one) — that is the answer.
2. Otherwise the lowest-Tier `approved` increment whose dependencies are all `done`.
3. If nothing is `approved`, say so and name the `planned` increments awaiting Gate 1.

Do not start work while answering a status question. Report, then wait.
