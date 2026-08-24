package enforce

import (
	"sort"
	"time"

	"github.com/ofenton/canon/internal/schema"
)

// Schema usage.
//
// Jira instances reach 800 custom fields with over half unused in a year because
// nobody ever sees the aggregate. Every individual request was reasonable; no one was
// ever shown the total, so no one ever said no.
//
// Canon's answer is that the schema is one reviewed file — but a reviewer approving
// the 40th field has the same problem as the Jira admin approving the 800th unless
// they can see what the existing 39 are actually doing. This report is the other half
// of that argument: the diff shows what is being added, and this shows what is already
// there and dead.
//
// It counts issues, not events. "Twelve issues use this field" is the question a
// reviewer is really asking; "this field was written 400 times" flatters a field that
// one issue churned on.

// Usage is one declared thing and what it is actually doing.
type Usage struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Count is how many live issues use it.
	Count int `json:"count"`
	// LastUsed is the most recent time any issue using it was touched. Zero when
	// nothing uses it.
	LastUsed time.Time `json:"last_used,omitzero"`
	// Detail carries what makes this row make sense — the values an enum actually
	// takes, say. Empty when there is nothing to add.
	Detail map[string]int `json:"detail,omitempty"`
}

// Used reports whether anything uses this at all.
func (u Usage) Used() bool { return u.Count > 0 }

// SchemaUsage reports every field, state, issue type, team and role in the schema
// with how many issues use it and when it was last touched.
//
// Everything declared appears, whether used or not: the unused rows are the point.
func (e *Enforcer) SchemaUsage() ([]Usage, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}

	type tally struct {
		count int
		last  time.Time
		by    map[string]int
	}
	counts := map[string]*tally{}
	note := func(kind, name string, touched time.Time, value string) {
		if name == "" {
			return
		}
		key := kind + "\x1f" + name
		t := counts[key]
		if t == nil {
			t = &tally{by: map[string]int{}}
			counts[key] = t
		}
		t.count++
		if touched.After(t.last) {
			t.last = touched
		}
		if value != "" {
			t.by[value]++
		}
	}

	for _, id := range e.view.IssueIDs() {
		issue, ok := e.view.Issue(id)
		if !ok {
			continue
		}
		touched := issue.UpdatedAt
		note("state", issue.State, touched, "")
		note("issue_type", issue.Type, touched, "")
		note("team", issue.Team, touched, "")

		// Title and evidence are schema fields that the projection promotes out of
		// the Fields map — title onto Issue, evidence onto each transition. Counting
		// only Fields reported both as unused, which on a schema where every issue
		// has a title is not a small inaccuracy: it is the report advising somebody
		// to delete the one required field.
		if issue.Title != "" {
			note("field", "title", touched, "")
		}
		for _, tr := range issue.Transitions {
			if tr.Evidence != "" {
				note("field", "evidence", tr.At, "")
			}
		}
		for name, value := range issue.Fields {
			// An enum's values matter as much as the field: a four-value enum where
			// everything is p2 is a field pretending to be a decision.
			note("field", name, touched, enumValue(e.schema, name, value))
		}
		for name := range issue.Multi {
			note("field", name, touched, "")
		}
		for name := range issue.Checklists {
			note("field", name, touched, "")
		}
	}

	// Roles are held by actors rather than issues, so they count differently and are
	// counted separately rather than being forced into the same shape.
	for _, id := range e.view.ActorIDs() {
		actor, ok := e.view.Actor(id)
		if !ok {
			continue
		}
		for _, role := range actor.Roles {
			note("role", role, time.Time{}, "")
		}
		for _, team := range actor.Teams {
			note("team_member", team, time.Time{}, "")
		}
	}

	var out []Usage
	add := func(kind, name string) {
		u := Usage{Kind: kind, Name: name}
		if t := counts[kind+"\x1f"+name]; t != nil {
			u.Count, u.LastUsed = t.count, t.last
			if len(t.by) > 0 {
				u.Detail = t.by
			}
		}
		out = append(out, u)
	}
	for _, f := range e.schema.Fields {
		add("field", f.Name)
	}
	for _, st := range e.schema.States {
		add("state", st.Name)
	}
	for _, it := range e.schema.IssueTypes {
		add("issue_type", it.Name)
	}
	for _, t := range e.schema.Teams {
		add("team", t.Name)
	}
	for _, r := range e.schema.Roles {
		add("role", r.Name)
	}

	// Unused first: the rows that need a decision should not be at the bottom of a
	// long list of healthy ones.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Used() != out[j].Used() {
			return !out[i].Used()
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Count > out[j].Count
	})
	return out, nil
}

// enumValue returns the value if the field is an enum, so the report can show the
// distribution. Other types are not tallied: the distinct values of a free-text field
// are just its contents.
func enumValue(s *schema.Schema, field, value string) string {
	f, ok := s.Field(field)
	if !ok || f.Type != schema.Enum {
		return ""
	}
	return value
}
