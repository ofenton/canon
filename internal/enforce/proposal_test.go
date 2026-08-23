package enforce

import (
	"strings"
	"testing"

	"github.com/ofenton/canon/internal/event"
)

func agentPrincipal(t *testing.T, e *Enforcer) Principal {
	t.Helper()
	register(t, e, "agent:one", "agent", "platform")
	p, err := e.Principal("agent:one")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func adminPrincipal(t *testing.T, e *Enforcer) Principal {
	t.Helper()
	register(t, e, "ollie", "admin", "platform")
	p, err := e.Principal("ollie")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// setup creates one issue in in_review, worked by the agent.
func setup(t *testing.T, e *Enforcer) (Principal, Principal) {
	t.Helper()
	admin := adminPrincipal(t, e)
	agent := agentPrincipal(t, e)
	if err := e.CreateAs(admin, "CANON-1", "task", map[string]string{"title": "x"}, "platform", at(1)); err != nil {
		t.Fatal(err)
	}
	if err := e.TransitionAs(agent, "CANON-1", "in_progress", "", at(2)); err != nil {
		t.Fatal(err)
	}
	if err := e.TransitionAs(agent, "CANON-1", "in_review", "312 passed", at(3)); err != nil {
		t.Fatal(err)
	}
	return admin, agent
}

// AC: WHEN an agent attempts a transition it lacks permission for THE SYSTEM SHALL
// record a proposal awaiting human approval and return the proposal id.
func TestAgentAttemptBecomesAStoredProposal(t *testing.T) {
	e, log := fixture(t)
	_, agent := setup(t, e)

	before, _ := log.Count()
	err := e.TransitionAs(agent, "CANON-1", "done", "", at(4))
	if err == nil {
		t.Fatal("the agent must not complete work outright")
	}
	var required *ProposalRequired
	if !AsProposalRequired(err, &required) {
		t.Fatalf("expected a proposal, got: %v", err)
	}
	if required.ProposalID == "" {
		t.Fatal("the proposal must carry an id the caller can refer to")
	}
	after, _ := log.Count()
	if after != before+1 {
		t.Errorf("recording a proposal should append exactly one event, got %d", after-before)
	}

	// The issue itself must not have moved.
	view, err := e.Projection()
	if err != nil {
		t.Fatal(err)
	}
	issue, _ := view.Issue("CANON-1")
	if issue.State != "in_review" {
		t.Errorf("state: got %q, a proposal must not apply the change", issue.State)
	}

	open, err := e.Proposals()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 {
		t.Fatalf("open proposals: got %d want 1", len(open))
	}
	p := open[0]
	if p.Operation != "transition:in_review->done" || p.Subject != "CANON-1" {
		t.Errorf("proposal: %+v", p)
	}
	if p.ProposedBy != "agent:one" || p.Model != "claude-opus-5" {
		t.Errorf("proposal must record who proposed it and their model: %+v", p)
	}
	if p.Status != ProposalOpen {
		t.Errorf("status: got %q want open", p.Status)
	}
}

// AC: WHEN a human approves a proposal THE SYSTEM SHALL apply the original transition
// with both actors recorded.
func TestApprovalAppliesTheTransitionRecordingBothActors(t *testing.T) {
	e, log := fixture(t)
	admin, agent := setup(t, e)

	var required *ProposalRequired
	if !AsProposalRequired(e.TransitionAs(agent, "CANON-1", "done", "", at(4)), &required) {
		t.Fatal("expected a proposal")
	}

	if err := e.ApproveProposal(admin, required.ProposalID, at(5)); err != nil {
		t.Fatalf("approve: %v", err)
	}

	view, err := e.Projection()
	if err != nil {
		t.Fatal(err)
	}
	issue, _ := view.Issue("CANON-1")
	if issue.State != "done" {
		t.Fatalf("state after approval: got %q want done", issue.State)
	}

	// Both actors must be recoverable: the agent proposed, the human approved.
	events, err := log.Subject("CANON-1")
	if err != nil {
		t.Fatal(err)
	}
	var applied *event.Event
	for _, ev := range events {
		if ev.Type == "issue.transitioned" && ev.Payload["to"] == "done" {
			applied = ev
		}
	}
	if applied == nil {
		t.Fatal("no transition to done was applied")
	}
	if applied.Actor.ID != "ollie" {
		t.Errorf("the applying actor must be the approver, got %q", applied.Actor.ID)
	}
	if applied.Payload["proposed_by"] != "agent:one" {
		t.Errorf("the applied event must record who proposed it, got %v", applied.Payload["proposed_by"])
	}
	if applied.Payload["proposal"] != required.ProposalID {
		t.Errorf("the applied event must reference the proposal, got %v", applied.Payload["proposal"])
	}

	if open, _ := e.Proposals(); len(open) != 0 {
		t.Errorf("an approved proposal must leave the open list, got %d", len(open))
	}
}

func TestRejectionDoesNotApply(t *testing.T) {
	e, _ := fixture(t)
	admin, agent := setup(t, e)
	var required *ProposalRequired
	AsProposalRequired(e.TransitionAs(agent, "CANON-1", "done", "", at(4)), &required)

	if err := e.RejectProposal(admin, required.ProposalID, "not ready", at(5)); err != nil {
		t.Fatalf("reject: %v", err)
	}
	view, _ := e.Projection()
	issue, _ := view.Issue("CANON-1")
	if issue.State != "in_review" {
		t.Errorf("state after rejection: got %q, must not move", issue.State)
	}
	if open, _ := e.Proposals(); len(open) != 0 {
		t.Errorf("a rejected proposal must leave the open list")
	}
	all, err := e.AllProposals()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Status != ProposalRejected {
		t.Errorf("the rejection must be retained with its status, got %+v", all)
	}
	if all[0].Reason != "not ready" {
		t.Errorf("the reason must be recorded, got %q", all[0].Reason)
	}
}

// An agent must not approve its own proposal — that would make the gate decorative.
func TestAgentCannotApprove(t *testing.T) {
	e, _ := fixture(t)
	_, agent := setup(t, e)
	var required *ProposalRequired
	AsProposalRequired(e.TransitionAs(agent, "CANON-1", "done", "", at(4)), &required)

	err := e.ApproveProposal(agent, required.ProposalID, at(5))
	if err == nil {
		t.Fatal("an agent must not approve a proposal")
	}
	if !strings.Contains(err.Error(), "human") {
		t.Errorf("the error should say why, got: %v", err)
	}
}

func TestCannotApproveTwice(t *testing.T) {
	e, _ := fixture(t)
	admin, agent := setup(t, e)
	var required *ProposalRequired
	AsProposalRequired(e.TransitionAs(agent, "CANON-1", "done", "", at(4)), &required)

	if err := e.ApproveProposal(admin, required.ProposalID, at(5)); err != nil {
		t.Fatal(err)
	}
	if err := e.ApproveProposal(admin, required.ProposalID, at(6)); err == nil {
		t.Error("approving a resolved proposal must be refused")
	}
}

// An approver must themselves be permitted to perform the operation, or approval
// becomes a way to launder an operation through someone who cannot do it either.
func TestApproverMustBePermitted(t *testing.T) {
	e, _ := fixture(t)
	_, agent := setup(t, e)
	register(t, e, "jo", "reporter", "platform")
	reporter, err := e.Principal("jo")
	if err != nil {
		t.Fatal(err)
	}

	var required *ProposalRequired
	AsProposalRequired(e.TransitionAs(agent, "CANON-1", "done", "", at(4)), &required)

	if err := e.ApproveProposal(reporter, required.ProposalID, at(5)); err == nil {
		t.Fatal("a reporter must not approve a transition they could not perform")
	}
}

// If the world moved on, an approval must not apply a transition that is no longer
// legal — otherwise a stale proposal is a way to bypass the schema.
func TestStaleProposalIsRefused(t *testing.T) {
	e, _ := fixture(t)
	admin, agent := setup(t, e)
	var required *ProposalRequired
	AsProposalRequired(e.TransitionAs(agent, "CANON-1", "done", "", at(4)), &required)

	// A human moves it back to in_progress; in_progress -> done is not permitted.
	if err := e.TransitionAs(admin, "CANON-1", "in_progress", "", at(5)); err != nil {
		t.Fatal(err)
	}
	err := e.ApproveProposal(admin, required.ProposalID, at(6))
	if err == nil {
		t.Fatal("approving a proposal whose precondition has changed must be refused")
	}
	if !strings.Contains(err.Error(), "in_progress") {
		t.Errorf("the error should explain what changed, got: %v", err)
	}
}

func TestUnknownProposal(t *testing.T) {
	e, _ := fixture(t)
	admin := adminPrincipal(t, e)
	if err := e.ApproveProposal(admin, "nope", at(1)); err == nil ||
		!strings.Contains(err.Error(), "nope") {
		t.Errorf("unknown proposal must be refused naming it, got: %v", err)
	}
}
