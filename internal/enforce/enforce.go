// Package enforce is the only path by which events reach the log.
//
// Every write is checked against the organisation's schema first. There is no
// bypass, and there is no per-project override to bypass it with. This is the
// product: Jira instances reach 90 workflows and 800 fields because each addition
// is locally reasonable and nobody sees the aggregate. Here the only way to add a
// field is to change canon.yaml, which is a diff a human approves.
//
// A rejected write appends nothing. Validation that still writes would be worse
// than none, because the log would then contain states the schema forbids and every
// projection built from it would be wrong.
package enforce

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/projection"
	"github.com/ofenton/canon/internal/schema"
)

// Enforcer validates writes against a schema and appends them to a log.
type Enforcer struct {
	schema *schema.Schema
	log    *event.Store
	view   *projection.Projection
}

// New returns an enforcer over one schema and one log.
func New(s *schema.Schema, log *event.Store) *Enforcer {
	return &Enforcer{schema: s, log: log, view: projection.New(log)}
}

// UseSchema swaps in a new schema.
//
// Additive changes take effect immediately: the schema is consulted per write rather
// than compiled into anything, so there is nothing to migrate and nothing to restart.
// Callers must run CheckMigration first — this method does not re-validate, because
// refusing here would leave the process running an older schema than the file on disk.
func (e *Enforcer) UseSchema(s *schema.Schema) { e.schema = s }

// Schema returns the schema currently enforced.
func (e *Enforcer) Schema() *schema.Schema { return e.schema }

// Create records a new issue.
func (e *Enforcer) Create(id, issueType string, fields map[string]string, at time.Time, actor event.Actor) error {
	if err := e.refresh(); err != nil {
		return err
	}
	if _, exists := e.view.Issue(id); exists {
		return fmt.Errorf("issue %s already exists", id)
	}

	it, ok := e.issueType(issueType)
	if !ok {
		return fmt.Errorf("unknown issue type %q; defined types are %s",
			issueType, strings.Join(e.issueTypeNames(), ", "))
	}

	allowed := map[string]bool{}
	for _, name := range it.Fields {
		allowed[name] = true
	}
	for name, value := range fields {
		if !allowed[name] {
			return fmt.Errorf("field %q is not on issue type %q; that type declares %s",
				name, issueType, strings.Join(it.Fields, ", "))
		}
		if err := e.checkValue(name, value); err != nil {
			return err
		}
	}
	for _, name := range it.Fields {
		def, _ := e.schema.Field(name)
		if def.Required && fields[name] == "" {
			return fmt.Errorf("field %q is required on issue type %q", name, issueType)
		}
	}

	initial := e.initialState()
	if initial == "" {
		return fmt.Errorf("schema defines no state in the open category to create issues in")
	}

	payload := map[string]any{"type": issueType, "state": initial}
	for name, value := range fields {
		payload[name] = value
	}
	if title, ok := fields["title"]; ok {
		payload["title"] = title
	}
	return e.append("issue.created", id, at, actor, payload)
}

// SetField records a field value.
func (e *Enforcer) SetField(id, field, value string, at time.Time, actor event.Actor) error {
	if err := e.refresh(); err != nil {
		return err
	}
	if _, ok := e.view.Issue(id); !ok {
		return fmt.Errorf("unknown issue %s", id)
	}
	if !e.schema.HasField(field) {
		return fmt.Errorf("field %q is not defined in the schema; defined fields are %s",
			field, strings.Join(e.schema.FieldNames(), ", "))
	}
	if err := e.checkValue(field, value); err != nil {
		return err
	}
	return e.append("field.set", id, at, actor,
		map[string]any{"field": field, "value": value})
}

// Transition moves an issue to a new state.
func (e *Enforcer) Transition(id, to, evidence string, at time.Time, actor event.Actor) error {
	if err := e.refresh(); err != nil {
		return err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return fmt.Errorf("unknown issue %s", id)
	}
	if !e.schema.HasState(to) {
		return fmt.Errorf("state %q is not defined in the schema; defined states are %s",
			to, strings.Join(e.stateNames(), ", "))
	}
	if !e.schema.CanTransition(issue.State, to) {
		permitted := e.schema.PermittedFrom(issue.State)
		if len(permitted) == 0 {
			return fmt.Errorf("%s is in state %q, which the schema permits no transitions from",
				id, issue.State)
		}
		return fmt.Errorf("%s cannot move from %q to %q; permitted transitions from %q are %s",
			id, issue.State, to, issue.State, strings.Join(permitted, ", "))
	}
	if e.schema.RequiresEvidence(to) && strings.TrimSpace(evidence) == "" {
		return fmt.Errorf("state %q requires evidence; supply it with the transition", to)
	}

	payload := map[string]any{"from": issue.State, "to": to}
	if evidence != "" {
		payload["evidence"] = evidence
	}
	return e.append("issue.transitioned", id, at, actor, payload)
}

// Reparent sets or clears an issue's parent.
//
// Nesting is governed by issue type, not by individual issues: the schema declares an
// ordering — epic, feature, story, then task or bug — and a child's type must sit at
// the level below its parent's.
//
// That ordering is also why there is no cycle check here. A child's level is strictly
// greater than its parent's, so following parents upward strictly decreases the level
// and must terminate. The generic check this replaced was guarding a case the type
// rules make unreachable.
func (e *Enforcer) Reparent(id, parent string, at time.Time, actor event.Actor) error {
	if err := e.refresh(); err != nil {
		return err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return fmt.Errorf("unknown issue %s", id)
	}
	if parent != "" {
		parentIssue, ok := e.view.Issue(parent)
		if !ok {
			return fmt.Errorf("unknown parent %s", parent)
		}
		if err := e.schema.CanNest(issue.Type, parentIssue.Type); err != nil {
			return fmt.Errorf("cannot make %s a child of %s: %w", id, parent, err)
		}
	}
	return e.append("issue.reparented", id, at, actor, map[string]any{"parent": parent})
}

// Delete removes an issue and lifts its children to the deleted issue's parent.
//
// Cascading would destroy work nobody asked to destroy; orphaning would leave
// children pointing at something that no longer exists. Lifting preserves both the
// children and the shape of the tree around them.
//
// Each child's move is recorded as its own issue.reparented event rather than being
// inferred by the projection, so the history says why a child changed parent. In an
// append-only log the audit trail is the whole point of the storage model.
func (e *Enforcer) Delete(id string, at time.Time, actor event.Actor) error {
	if err := e.refresh(); err != nil {
		return err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return fmt.Errorf("unknown issue %s", id)
	}

	children := e.view.Children(id)

	// Lifting a child to its grandparent can produce a nesting the schema forbids —
	// a task under a feature, say. Refusing is better than the alternatives: silently
	// creating an invalid tree hides the problem, and cascading the delete destroys
	// work nobody asked to destroy.
	if issue.Parent != "" {
		grandparent, exists := e.view.Issue(issue.Parent)
		if exists {
			var blocked []string
			for _, child := range children {
				childIssue, _ := e.view.Issue(child)
				if err := e.schema.CanNest(childIssue.Type, grandparent.Type); err != nil {
					blocked = append(blocked, fmt.Sprintf("%s (%s)", child, childIssue.Type))
				}
			}
			if len(blocked) > 0 {
				return fmt.Errorf("cannot delete %s: its children would be lifted under %s (%s), which the hierarchy does not permit for %s. Move or delete them first",
					id, issue.Parent, grandparent.Type, strings.Join(blocked, ", "))
			}
		}
	}

	for _, child := range children {
		if err := e.append("issue.reparented", child, at, actor,
			map[string]any{"parent": issue.Parent, "because": "parent " + id + " was deleted"}); err != nil {
			return fmt.Errorf("lifting child %s: %w", child, err)
		}
	}
	return e.append("issue.deleted", id, at, actor, nil)
}

// CheckMigration reports whether a new schema can be applied to an existing log.
//
// The check is against projected state rather than the raw log: an issue that passed
// through a since-removed state is fine, an issue that currently sits in one is not.
func CheckMigration(log *event.Store, next *schema.Schema) error {
	view := projection.New(log)
	if err := view.Rebuild(); err != nil {
		return fmt.Errorf("checking migration: %w", err)
	}

	orphanedBy := map[string][]string{}
	for _, id := range view.IssueIDs() {
		issue, _ := view.Issue(id)
		if issue.State != "" && !next.HasState(issue.State) {
			orphanedBy[issue.State] = append(orphanedBy[issue.State], id)
		}
	}
	if len(orphanedBy) == 0 {
		return nil
	}

	states := make([]string, 0, len(orphanedBy))
	for state := range orphanedBy {
		states = append(states, state)
	}
	sort.Strings(states)

	var b strings.Builder
	b.WriteString("this schema change would orphan existing issues:\n")
	for _, state := range states {
		ids := orphanedBy[state]
		sort.Strings(ids)
		fmt.Fprintf(&b, "  removing state %q would strand %d issue(s): %s\n",
			state, len(ids), strings.Join(ids, ", "))
	}
	b.WriteString("\nmove them to a state the new schema defines, or keep the state")
	return fmt.Errorf("%s", b.String())
}

// append writes a validated event. Nothing else in this package touches the log.
func (e *Enforcer) append(typ, subject string, at time.Time, actor event.Actor, payload map[string]any) error {
	return e.log.Append(event.New(typ, subject, at, actor, payload))
}

// refresh brings the internal view up to date before a decision is made against it.
func (e *Enforcer) refresh() error {
	if err := e.view.Catchup(); err != nil {
		return fmt.Errorf("reading current state: %w", err)
	}
	return nil
}

func (e *Enforcer) checkValue(field, value string) error {
	def, ok := e.schema.Field(field)
	if !ok {
		return fmt.Errorf("field %q is not defined in the schema; defined fields are %s",
			field, strings.Join(e.schema.FieldNames(), ", "))
	}
	if def.Type == schema.Enum {
		for _, permitted := range def.Values {
			if value == permitted {
				return nil
			}
		}
		return fmt.Errorf("field %q does not permit %q; permitted values are %s",
			field, value, strings.Join(def.Values, ", "))
	}
	return nil
}

// initialState returns the first state in the open category. Issues are created
// there; the schema decides what "the beginning" means, not this code.
func (e *Enforcer) initialState() string {
	for _, st := range e.schema.States {
		if st.Category == schema.Open {
			return st.Name
		}
	}
	return ""
}

func (e *Enforcer) issueType(name string) (schema.IssueType, bool) {
	for _, it := range e.schema.IssueTypes {
		if it.Name == name {
			return it, true
		}
	}
	return schema.IssueType{}, false
}

func (e *Enforcer) issueTypeNames() []string {
	out := make([]string, 0, len(e.schema.IssueTypes))
	for _, it := range e.schema.IssueTypes {
		out = append(out, it.Name)
	}
	sort.Strings(out)
	return out
}

func (e *Enforcer) stateNames() []string {
	out := make([]string, 0, len(e.schema.States))
	for _, st := range e.schema.States {
		out = append(out, st.Name)
	}
	sort.Strings(out)
	return out
}
