package enforce

import (
	"fmt"
	"time"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/projection"
)

// Proposals are the product's version of the two-gate pattern: an agent proposes, a
// human decides.
//
// The alternative designs both fail. Refusing outright means an agent that found real
// work either stops or retries blindly, and the finding is lost. Letting the agent
// proceed means the gate is decorative. Recording the attempt keeps the work and the
// decision, and the useful artifact turns out to be the attempt itself: what the
// agent wanted to do, to what, with what evidence.

// Re-exported so callers need not import projection to read a proposal.
type (
	Proposal       = projection.Proposal
	ProposalStatus = projection.ProposalStatus
)

const (
	ProposalOpen     = projection.ProposalOpen
	ProposalApproved = projection.ProposalApproved
	ProposalRejected = projection.ProposalRejected
)

// recordProposal stores an attempt an actor was not permitted to perform outright.
func (e *Enforcer) recordProposal(p Principal, required *ProposalRequired, from, to, evidence string, at time.Time) error {
	id, err := e.nextProposalID()
	if err != nil {
		return err
	}
	required.ProposalID = id
	return e.append("proposal.created", id, at, p.Actor, map[string]any{
		"issue":     required.Subject,
		"operation": required.Operation,
		"role":      required.Role,
		"evidence":  evidence,
		"from":      from,
		"to":        to,
	})
}

func (e *Enforcer) nextProposalID() (string, error) {
	if err := e.refresh(); err != nil {
		return "", err
	}
	return fmt.Sprintf("PROP-%d", len(e.view.Proposals(""))+1), nil
}

// Proposals returns the open proposals awaiting a decision.
func (e *Enforcer) Proposals() ([]*Proposal, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}
	return e.view.Proposals(ProposalOpen), nil
}

// AllProposals returns every proposal, decided or not.
func (e *Enforcer) AllProposals() ([]*Proposal, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}
	return e.view.Proposals(""), nil
}

// Projection exposes the current view, for callers that need to read state.
func (e *Enforcer) Projection() (*projection.Projection, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}
	return e.view, nil
}

// ApproveProposal applies a proposed operation on the approver's authority.
func (e *Enforcer) ApproveProposal(by Principal, id string, at time.Time) error {
	proposal, err := e.decidable(by, id)
	if err != nil {
		return err
	}

	// The approver must be able to perform the operation themselves. Otherwise
	// approval becomes a way to launder an operation through someone who could not
	// do it either, which is worse than no gate at all.
	issue, ok := e.view.Issue(proposal.Subject)
	if !ok {
		return fmt.Errorf("proposal %s refers to issue %s, which no longer exists", id, proposal.Subject)
	}
	if err := e.authorise(by, proposal.Operation, proposal.Subject, issue.Team); err != nil {
		return fmt.Errorf("cannot approve %s: %w", id, err)
	}

	// The world may have moved since the proposal was made. Applying a transition
	// from a state that is no longer current would let a stale proposal bypass the
	// schema, so the precondition is rechecked rather than trusted.
	if proposal.From != "" && issue.State != proposal.From {
		return fmt.Errorf("proposal %s was made when %s was in %q, but it is now in %q; the transition no longer applies",
			id, proposal.Subject, proposal.From, issue.State)
	}

	if err := e.append("proposal.approved", id, at, by.Actor, nil); err != nil {
		return err
	}
	// Applied on the approver's authority, carrying the proposer's identity so the
	// event records both halves of the decision.
	return e.append("issue.transitioned", proposal.Subject, at, by.Actor, map[string]any{
		"from":        proposal.From,
		"to":          proposal.To,
		"evidence":    proposal.Evidence,
		"proposal":    id,
		"proposed_by": proposal.ProposedBy,
	})
}

// RejectProposal records that a human declined a proposed operation.
func (e *Enforcer) RejectProposal(by Principal, id, reason string, at time.Time) error {
	if _, err := e.decidable(by, id); err != nil {
		return err
	}
	return e.append("proposal.rejected", id, at, by.Actor, map[string]any{"reason": reason})
}

// decidable returns a proposal if this principal may decide it.
func (e *Enforcer) decidable(by Principal, id string) (*Proposal, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}
	proposal, ok := e.view.Proposal(id)
	if !ok {
		return nil, fmt.Errorf("unknown proposal %q", id)
	}
	if proposal.Status != ProposalOpen {
		return nil, fmt.Errorf("proposal %s was already %s by %s", id, proposal.Status, proposal.DecidedBy)
	}
	// The whole point is that a person decides. An agent approving its own proposal,
	// or another agent's, would make the gate decorative.
	if by.Actor.Kind != event.ActorHuman {
		return nil, fmt.Errorf("proposal %s must be decided by a human; %s is %s",
			id, by.Actor.ID, by.Actor.Kind)
	}
	return proposal, nil
}

// proposalFor turns an authorisation outcome into a stored proposal where the actor's
// role allows one. Returns the error to give the caller.
func (e *Enforcer) proposalFor(p Principal, err error, from, to, evidence string, at time.Time) error {
	var required *ProposalRequired
	if !AsProposalRequired(err, &required) {
		return err
	}
	if recordErr := e.recordProposal(p, required, from, to, evidence, at); recordErr != nil {
		return fmt.Errorf("recording proposal: %w", recordErr)
	}
	return required
}
