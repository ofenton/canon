# feat-007: Agent identity, provenance and proposals

## Context

The two-gate pattern from this project's own development process, implemented inside the product:
an agent proposes, a human decides.

Provenance (R12) and evidence-required transitions (R14) landed earlier in feat-001 and feat-004.
The new work is R15 — turning a refusal into a stored proposal with an approval path.

## Design notes

**A proposal is a record, not a failed request.** The useful artifact is the attempt: what the
agent wanted to do, to what, with what evidence, and under which role. The two alternatives both
fail — refusing outright means an agent that found real work either stops or retries blindly and
the finding is lost; letting it proceed makes the gate decorative.

**The approver must themselves be permitted to perform the operation.** Otherwise approval
becomes a way to launder an operation through someone who could not do it either, which is worse
than having no gate. `TestApproverMustBePermitted` covers it.

**Only a human may decide.** An agent approving its own proposal, or another agent's, makes the
gate ceremonial. Checked on actor kind, not role, because a role could later be granted to an
agent by accident.

**A stale proposal is refused.** The proposal records the state the subject was in when it was
made. If the issue has moved since, approval is refused with an explanation rather than applying
a transition that is no longer legal — otherwise a stale proposal becomes a way around the schema.
`TestStaleProposalIsRefused` covers it.

**The applied event records both halves.** It is written on the approver's authority — the actor
is the human — and carries `proposed_by` and the proposal id in its payload. Attributing it to
the agent would misrepresent who decided; attributing it only to the human would lose who found
the work.

**The API returns 202, not 403**, with the proposal id, so an agent can reference what it created.

## Evidence

**Verified by:** implementing session, `inc/feat-007-proposals`

### An agent's refused attempt becomes a stored proposal with an id

```
$ curl -X POST /api/issues/CANON-1/transition -H 'X-Canon-Actor: agent:one' -d '{"to":"done"}'
202 {
  "status": "proposal_required",
  "proposal_id": "PROP-1",
  "operation": "transition:in_review->done",
  "subject": "CANON-1",
  "role": "agent",
  "message": "agent:one may not transition:in_review->done on CANON-1 directly;
              recorded as proposal PROP-1 for human approval (role \"agent\")"
}
```

```
--- PASS: TestAgentAttemptBecomesAStoredProposal
```

Asserts exactly one event is appended, the issue does **not** move, and the proposal records the
proposer and their model.

### A human approving applies the transition, recording both actors

```
$ curl -X POST /api/proposals/PROP-1/approve -H 'X-Canon-Actor: ollie'      → 204

CANON-1 is now done, last touched by ollie
PROP-1  transition:in_review->done  on CANON-1  [approved]
   proposed by agent:one (claude-opus-5)
   decided by ollie
applied by ollie (human), proposed by agent:one, ref PROP-1
```

```
--- PASS: TestApprovalAppliesTheTransitionRecordingBothActors
--- PASS: TestRejectionDoesNotApply
```

### The gate cannot be circumvented

```
$ curl -X POST /api/proposals/PROP-1/approve -H 'X-Canon-Actor: agent:one'
422  proposal PROP-1 must be decided by a human; agent:one is agent
```

```
--- PASS: TestAgentCannotApprove         --- PASS: TestApproverMustBePermitted
--- PASS: TestCannotApproveTwice         --- PASS: TestStaleProposalIsRefused
--- PASS: TestUnknownProposal
```

### Provenance and evidence (R12, R14) — regression

Landed in feat-001 and feat-004; still holding:

```
--- PASS: TestAppendRecordsProvenance     (model id on every agent event)
--- PASS: TestAgentRefusalIsAProposal
--- PASS: TestEveryRouteIsExercised       (all 21 routes)
```

### The parity test earned its keep

Adding four proposal routes failed the build until the contract test exercised them:

```
--- FAIL: TestEveryRouteIsExercised
    routes never exercised: GET /api/proposals, GET /api/proposals/{id},
    POST /api/proposals/{id}/approve, POST /api/proposals/{id}/reject
```

A new endpoint cannot reach main without being proven to work.

### Scope

`git diff --cached --stat main` — run. Proposals in `enforce`, the projected `Proposal` entity and
its three event types in `projection`, four routes and the 202 body in `api`.

### Not verified

**The README's route table is now stale** — it documents 17 routes and there are 21. docs-001 is
in review on a separate branch, so updating it here would conflict. Needs a follow-up commit once
both are merged; recorded rather than left to be discovered.

CI runs on the pull request.
