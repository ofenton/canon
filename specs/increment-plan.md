# Increment plan

The ledger. This file is the single source of truth for what is planned, being built, and shipped.
Edit it only through the conventions in `skills/track-increment-state/SKILL.md`, and run
`python3 .sdlc/bin/validate-plan.py` after every change.

At most one increment may be `in-progress` at a time.

---

## chore-001: Adopt the increment workflow

_Your first increment. Commit the scaffold with `Increment: chore-001` in the trailer, then
mark this done — that is the whole loop, run once, on the workflow itself._

- **Type:** chore
- **Status:** approved
- **Traces:** none
- **Tier:** 2 (High)
- **Scope:** Add `AGENTS.md`, `skills/`, `.sdlc/` and `specs/` to the repository, and write the
  project constitution. No product code changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL provide an `AGENTS.md` naming the three planes, the two gates and the three tracks
  - [x] WHEN `validate-skills.py` runs THE SYSTEM SHALL report every skill valid
  - [x] WHEN `validate-plan.py` runs THE SYSTEM SHALL report the ledger well formed
  - [ ] WHEN `check-traceability.py` runs THE SYSTEM SHALL report the ledger traces cleanly
  - [ ] THE SYSTEM SHALL provide a `docs/constitution.md` agreed by a human
- **Test Strategy:**
  - Run both validators from a clean checkout
  - Start a fresh agent session and confirm it finds the ledger unprompted
- **Dependencies:** none
- **Rollback Plan:** Delete `.sdlc/`, `skills/`, `specs/` and `AGENTS.md`
- **Risk:** Low — additive, touches no product code
- **Evidence:** _(filled in at verify)_
