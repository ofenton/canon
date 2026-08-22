# 0002 — The repository is the source of truth

**Status:** accepted
**Date:** 2026-08-22

## Context

Work state can live in a tracker (Jira, Azure DevOps Boards) or in the repository. Both is the
tempting answer and the wrong one: when two systems both claim to hold the truth, they diverge
within days, and every question becomes "which one is right?"

Agents make this sharper. An agent can read and update a file in the working tree as part of the
same action that changes the code. Reaching a tracker means an API, credentials, network access
and a round trip — and, more importantly, state that no longer moves atomically with the diff it
describes.

## Decision

**`specs/increment-plan.md` in the git repository is the single source of truth for work state.**
The repository is hosted in Azure DevOps.

- Status changes are commits. The git log is the audit trail.
- `check-traceability.py` verifies the ledger against git, so the claim is enforced rather than
  asserted.
- If work items are also tracked in ADO Boards, they are a **projection**: the repository is
  written first and the tracker follows. Never the reverse.
- Where the two disagree, the repository wins. No exceptions — an exception is how you get two
  sources of truth back.

## Consequences

**Good.** State moves atomically with the code it describes: one commit changes the
implementation and the status together, and a branch carries both. State is reviewable in a pull
request alongside the diff. It survives every tool in the chain, because it is markdown in git.
It works offline, and needs no credentials.

**Costs.** People who live in a board do not see the ledger. Reporting across many repositories
means reading many ledgers rather than one query. Neither is free, and the mitigation is a
projection, not a second master.

**If ADO Boards integration is added later**, it must be one-way (repository → ADO) and derived
entirely from the ledger. Add the ADO work item id as a field on the increment so the link is
explicit and checkable. Do not let a board transition write back into the ledger; that would
recreate the problem this decision exists to avoid. No such integration is built today.

## Alternatives considered

**Tracker as source of truth, repo as documentation.** Rejected: state stops moving atomically
with the code, agents need credentials and network access for routine work, and the ledger decays
into stale prose nobody trusts.

**Both authoritative, reconciled periodically.** Rejected: this is the failure mode, not a
mitigation. Reconciliation is only defined when one side is authoritative.
