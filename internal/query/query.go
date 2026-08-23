// Package query filters issues, and expresses boards as saved queries.
//
// The language is deliberately small: term, comparison, negation, implicit AND.
// A query language is the easiest thing in a tracker to over-build, and JQL is what
// the far end of that road looks like — a dialect nobody can write without the
// documentation open. Everything here fits on one screen and is checked against
// canon.yaml, so a query that names a field the organisation does not have is
// refused rather than silently matching nothing.
package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ofenton/canon/internal/projection"
	"github.com/ofenton/canon/internal/schema"
)

// Reserved terms address an issue's built-in attributes rather than a schema field.
var reserved = map[string]func(*projection.Issue) string{
	"state":  func(i *projection.Issue) string { return i.State },
	"team":   func(i *projection.Issue) string { return i.Team },
	"parent": func(i *projection.Issue) string { return i.Parent },
	"title":  func(i *projection.Issue) string { return i.Title },
	"actor":  func(i *projection.Issue) string { return i.LastActor.ID },
	// category groups states, so "category:closed" survives a schema that renames
	// its closing states — which is the entire point of having categories.
	"category": nil, // resolved against the schema, see value()
	// ancestor is resolved against the projection in Filter, not from the issue.
	ancestorKey: nil,
	// blocked and depends_on are relations, resolved in Filter for the same reason.
	blockedKey:   nil,
	dependsOnKey: nil,
}

// term is one condition.
type term struct {
	key      string
	value    string
	negated  bool
	contains bool // title~foo does a substring match; everything else is exact
}

// Query is a conjunction of terms. There is no OR: two queries are two boards, and
// that has been enough for every real case so far.
type Query struct {
	terms []term
	raw   string
}

// Parse builds a query, checking every key against the schema.
func Parse(raw string, s *schema.Schema) (*Query, error) {
	q := &Query{raw: strings.TrimSpace(raw)}
	if q.raw == "" {
		return q, nil
	}

	for _, field := range strings.Fields(q.raw) {
		t := term{}
		if strings.HasPrefix(field, "!") {
			t.negated = true
			field = field[1:]
		}

		var key, value string
		switch {
		case strings.Contains(field, "~"):
			key, value, _ = strings.Cut(field, "~")
			t.contains = true
		case strings.Contains(field, "="):
			key, value, _ = strings.Cut(field, "=")
		case strings.Contains(field, ":"):
			key, value, _ = strings.Cut(field, ":")
		default:
			return nil, fmt.Errorf("term %q has no value; write key=value, key:value or key~substring", field)
		}

		if key == "" {
			return nil, fmt.Errorf("term %q has no key", field)
		}
		if err := checkKey(key, s); err != nil {
			return nil, err
		}
		if err := checkValue(key, value, s); err != nil {
			return nil, err
		}
		t.key, t.value = key, strings.Trim(value, `"`)
		q.terms = append(q.terms, t)
	}
	return q, nil
}

func checkKey(key string, s *schema.Schema) error {
	if _, ok := reserved[key]; ok {
		return nil
	}
	if s.HasField(key) {
		return nil
	}
	valid := append(reservedNames(), s.FieldNames()...)
	sort.Strings(valid)
	return fmt.Errorf("query key %q is not a field in this organisation's schema; valid keys are %s",
		key, strings.Join(valid, ", "))
}

// checkValue rejects a value the schema could never produce. A query for a state
// that does not exist is almost always a typo, and silently matching nothing is the
// least helpful possible response.
func checkValue(key, value string, s *schema.Schema) error {
	switch key {
	case "state":
		if value != "" && !s.HasState(value) {
			return fmt.Errorf("no state %q in the schema", value)
		}
	case "category":
		switch schema.Category(value) {
		case schema.Open, schema.Active, schema.Closed:
		default:
			return fmt.Errorf("no category %q; categories are open, active, closed", value)
		}
	case ancestorKey, dependsOnKey:
		// An issue id, not a schema value. A query for an issue that does not exist
		// returns nothing, which is the honest answer rather than an error.
		return nil
	case blockedKey:
		if value != "true" && value != "false" {
			return fmt.Errorf("blocked takes true or false, got %q", value)
		}
		return nil
	default:
		if def, ok := s.Field(key); ok && def.Type == schema.Enum && value != "" {
			for _, permitted := range def.Values {
				if value == permitted {
					return nil
				}
			}
			return fmt.Errorf("field %q does not take %q; permitted values are %s",
				key, value, strings.Join(def.Values, ", "))
		}
	}
	return nil
}

func reservedNames() []string {
	out := make([]string, 0, len(reserved))
	for name := range reserved {
		out = append(out, name)
	}
	return out
}

// Raw returns the query as written.
func (q *Query) Raw() string { return q.raw }

// ancestorKey is resolved against the projection rather than the issue, since an
// issue does not know its own lineage.
const ancestorKey = "ancestor"

// blockedKey and dependsOnKey ask about the dependency graph, which an issue does
// not carry on its own.
const (
	blockedKey   = "blocked"
	dependsOnKey = "depends_on"
)

// Match reports whether an issue satisfies every term.
func (q *Query) Match(issue *projection.Issue, s *schema.Schema) bool {
	for _, t := range q.terms {
		switch t.key {
		case ancestorKey, blockedKey, dependsOnKey:
			continue // relations, resolved against the projection in Filter
		}
		got := value(t.key, issue, s)
		var hit bool
		switch {
		case t.contains:
			hit = strings.Contains(strings.ToLower(got), strings.ToLower(t.value))
		default:
			hit = got == t.value
		}
		if hit == t.negated {
			return false
		}
	}
	return true
}

// Closed reports whether a state is in the schema's closed category.
func Closed(s *schema.Schema) func(string) bool {
	return func(state string) bool {
		for _, st := range s.States {
			if st.Name == state {
				return st.Category == schema.Closed
			}
		}
		return false
	}
}

// Filter returns the issues matching the query, in id order.
func (q *Query) Filter(view *projection.Projection, s *schema.Schema) []*projection.Issue {
	// ancestor= asks about the tree rather than the issue, so it is resolved once
	// against the projection instead of per-issue during matching.
	beneath := map[string]bool{}
	var scoped bool
	for _, t := range q.terms {
		if t.key != ancestorKey {
			continue
		}
		scoped = true
		for _, id := range view.Descendants(t.value, 0) {
			beneath[id] = true
		}
		if t.negated {
			// Negation is applied per-issue below; record the set either way.
			continue
		}
	}

	out := make([]*projection.Issue, 0)
	for _, id := range view.IssueIDs() {
		issue, _ := view.Issue(id)
		if !q.Match(issue, s) {
			continue
		}
		if scoped {
			var ok = true
			for _, t := range q.terms {
				if t.key != ancestorKey {
					continue
				}
				if beneath[id] == t.negated {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
		}

		if !q.matchesRelations(view, s, issue) {
			continue
		}
		out = append(out, issue)
	}
	return out
}

func value(key string, issue *projection.Issue, s *schema.Schema) string {
	if key == "category" {
		for _, st := range s.States {
			if st.Name == issue.State {
				return string(st.Category)
			}
		}
		return ""
	}
	if get, ok := reserved[key]; ok && get != nil {
		return get(issue)
	}
	return issue.Fields[key]
}

// GroupKeys lists the keys a board may group by: the reserved attributes plus every
// schema field.
func GroupKeys(s *schema.Schema) []string {
	out := append(reservedNames(), s.FieldNames()...)
	sort.Strings(out)
	return out
}

// Group buckets issues by a key, returning the buckets in a stable order.
//
// A board is exactly this: a query and a grouping key. It holds no membership of its
// own, so an issue that stops matching leaves without anything being updated — which
// is the difference between a board and a second copy of the truth.
func Group(issues []*projection.Issue, key string, s *schema.Schema) ([]string, map[string][]*projection.Issue) {
	buckets := map[string][]*projection.Issue{}
	for _, issue := range issues {
		v := value(key, issue, s)
		if v == "" {
			v = "(none)"
		}
		buckets[v] = append(buckets[v], issue)
	}

	// Grouping by state follows the schema's declared order, because a board whose
	// columns shuffle alphabetically is unreadable.
	var order []string
	if key == "state" {
		for _, st := range s.States {
			if _, ok := buckets[st.Name]; ok {
				order = append(order, st.Name)
			}
		}
		if _, ok := buckets["(none)"]; ok {
			order = append(order, "(none)")
		}
		return order, buckets
	}
	for name := range buckets {
		order = append(order, name)
	}
	sort.Strings(order)
	return order, buckets
}

// matchesRelations checks the terms that ask about the dependency graph.
func (q *Query) matchesRelations(view *projection.Projection, s *schema.Schema, issue *projection.Issue) bool {
	for _, t := range q.terms {
		switch t.key {
		case blockedKey:
			blocked, _ := view.Blocked(issue.ID, Closed(s))
			want := t.value == "true"
			if (blocked == want) == t.negated {
				return false
			}
		case dependsOnKey:
			var found bool
			for _, on := range issue.DependsOn {
				if on == t.value {
					found = true
					break
				}
			}
			if found == t.negated {
				return false
			}
		}
	}
	return true
}
