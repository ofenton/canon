package enforce

import (
	"fmt"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/schema"
)

// Dependencies.
//
// One relation, one direction: A depends on B. Jira offers blocks, is-blocked-by,
// relates-to, duplicates and clones, and teams spend meetings deciding which to use.
// A single directed edge expresses all the ordering anyone actually acts on.
//
// Cycles are recorded, not refused. This is the opposite of the hierarchy's rule and
// the difference is deliberate: a parent chain is a tree, so a cycle there is
// meaningless, but two pieces of work genuinely can be waiting on each other. A tool
// that refuses to record that pushes the truth somewhere it cannot be seen. So the
// write succeeds and the cycle is reported — loudly, everywhere the issue appears —
// because a cycle means nothing in it can start.

// DependencyResult reports what happened, including a cycle if one was created.
type DependencyResult struct {
	Cycle []string
}

// Warning describes the cycle in the terms a reader needs to act on it.
func (r DependencyResult) Warning() string {
	if len(r.Cycle) == 0 {
		return ""
	}
	return fmt.Sprintf("dependency cycle: %s → %s. Nothing in this cycle can start until one of these dependencies is removed.",
		strings.Join(r.Cycle, " → "), r.Cycle[0])
}

// AddDependency records that one issue depends on another.
func (e *Enforcer) AddDependency(p Principal, id, on string, at time.Time) (DependencyResult, error) {
	if err := e.refresh(); err != nil {
		return DependencyResult{}, err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return DependencyResult{}, fmt.Errorf("unknown issue %s", id)
	}
	if _, ok := e.view.Issue(on); !ok {
		return DependencyResult{}, fmt.Errorf("unknown issue %s", on)
	}
	if id == on {
		// The one case that is genuinely meaningless rather than merely bad.
		return DependencyResult{}, fmt.Errorf("%s cannot depend on itself", id)
	}
	for _, existing := range issue.DependsOn {
		if existing == on {
			return DependencyResult{}, fmt.Errorf("%s already depends on %s", id, on)
		}
	}

	if err := e.authorise(p, schema.DependOp, id, issue.Team); err != nil {
		return DependencyResult{}, err
	}
	if err := e.append("issue.dependency_added", id, at, p.Actor,
		map[string]any{"on": on}); err != nil {
		return DependencyResult{}, err
	}

	// Report the cycle after the write, not instead of it.
	if err := e.refresh(); err != nil {
		return DependencyResult{}, err
	}
	for _, cycle := range e.view.DependencyCycles() {
		for _, member := range cycle {
			if member == id {
				return DependencyResult{Cycle: cycle}, nil
			}
		}
	}
	return DependencyResult{}, nil
}

// RemoveDependency drops a dependency.
func (e *Enforcer) RemoveDependency(p Principal, id, on string, at time.Time) error {
	if err := e.refresh(); err != nil {
		return err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return fmt.Errorf("unknown issue %s", id)
	}
	var found bool
	for _, existing := range issue.DependsOn {
		if existing == on {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s does not depend on %s", id, on)
	}
	if err := e.authorise(p, schema.DependOp, id, issue.Team); err != nil {
		return err
	}
	return e.append("issue.dependency_removed", id, at, p.Actor, map[string]any{"on": on})
}

// Dependencies describes both directions plus whether the issue is blocked.
type Dependencies struct {
	DependsOn  []string   `json:"depends_on"`
	Dependents []string   `json:"dependents"`
	Blocked    bool       `json:"blocked"`
	BlockedBy  []string   `json:"blocked_by"`
	Cycles     [][]string `json:"cycles,omitempty"`
}

// DependenciesOf returns what an issue waits on, what waits on it, and why it is
// blocked if it is.
func (e *Enforcer) DependenciesOf(id string) (Dependencies, error) {
	if err := e.refresh(); err != nil {
		return Dependencies{}, err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return Dependencies{}, fmt.Errorf("unknown issue %s", id)
	}

	blocked, blockers := e.view.Blocked(id, e.isClosed)
	// Empty slices, not nil: JSON renders nil as null, and a client that has to
	// handle both null and [] for the same "nothing here" is being made to do work
	// the server should have done.
	out := Dependencies{
		DependsOn:  nonNil(issue.DependsOn),
		Dependents: nonNil(e.view.Dependents(id)),
		Blocked:    blocked,
		BlockedBy:  nonNil(blockers),
	}
	for _, cycle := range e.view.DependencyCycles() {
		for _, member := range cycle {
			if member == id {
				out.Cycles = append(out.Cycles, cycle)
				break
			}
		}
	}
	return out, nil
}

// Cycles returns every dependency cycle in the project.
func (e *Enforcer) Cycles() ([][]string, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}
	return e.view.DependencyCycles(), nil
}

// IsBlocked reports whether an issue is waiting on unfinished work.
func (e *Enforcer) IsBlocked(id string) (bool, []string) {
	if err := e.refresh(); err != nil {
		return false, nil
	}
	return e.view.Blocked(id, e.isClosed)
}

// isClosed asks the schema, so "finished" means whatever the organisation says it
// means rather than a hardcoded state name.
func (e *Enforcer) isClosed(state string) bool {
	for _, st := range e.schema.States {
		if st.Name == state {
			return st.Category == schema.Closed
		}
	}
	return false
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
