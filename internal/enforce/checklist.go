package enforce

import (
	"fmt"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/projection"
	"github.com/ofenton/canon/internal/schema"
)

// Checklists and multi-valued fields.
//
// An acceptance criterion is not a paragraph, it is a thing that is either met or
// not. Storing criteria as text means nobody can count them, filter on them or gate
// a transition behind them — the information is there and unusable. Items are
// individually checkable events, so "three of five met" is a count and the log says
// who met which.

// SetMulti records the values of a multi-valued field.
func (e *Enforcer) SetMulti(p Principal, id, field string, values []string, at time.Time) error {
	if err := e.refresh(); err != nil {
		return err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return fmt.Errorf("unknown issue %s", id)
	}
	def, ok := e.schema.Field(field)
	if !ok {
		return fmt.Errorf("field %q is not defined in the schema; defined fields are %s",
			field, strings.Join(e.schema.FieldNames(), ", "))
	}
	if def.Type != schema.MultiEnum {
		return fmt.Errorf("field %q is a %s, not a multi-value field", field, def.Type)
	}
	for _, v := range values {
		if !contains(def.Values, v) {
			return fmt.Errorf("field %q does not permit %q; permitted values are %s",
				field, v, strings.Join(def.Values, ", "))
		}
	}
	if err := e.authorise(p, schema.FieldOp(field), id, issue.Team); err != nil {
		return err
	}

	payload := make([]any, len(values))
	for i, v := range values {
		payload[i] = v
	}
	return e.append("field.multi_set", id, at, p.Actor,
		map[string]any{"field": field, "values": payload})
}

// AddChecklistItem adds one criterion to a checklist field.
func (e *Enforcer) AddChecklistItem(p Principal, id, field, text string, at time.Time) error {
	issue, def, err := e.checklistField(p, id, field)
	if err != nil {
		return err
	}
	_ = def
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("a checklist item needs text")
	}
	for _, item := range issue.Checklists[field] {
		if item.Text == text {
			return fmt.Errorf("%s already has that item in %q", id, field)
		}
	}
	return e.append("checklist.item_added", id, at, p.Actor,
		map[string]any{"field": field, "text": text})
}

// SetChecklistItem marks one criterion met or unmet.
func (e *Enforcer) SetChecklistItem(p Principal, id, field, text string, checked bool, at time.Time) error {
	issue, _, err := e.checklistField(p, id, field)
	if err != nil {
		return err
	}
	var found bool
	for _, item := range issue.Checklists[field] {
		if item.Text == text {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s has no item %q in %q", id, text, field)
	}
	kind := "checklist.item_unchecked"
	if checked {
		kind = "checklist.item_checked"
	}
	return e.append(kind, id, at, p.Actor, map[string]any{"field": field, "text": text})
}

// RemoveChecklistItem drops a criterion.
func (e *Enforcer) RemoveChecklistItem(p Principal, id, field, text string, at time.Time) error {
	if _, _, err := e.checklistField(p, id, field); err != nil {
		return err
	}
	return e.append("checklist.item_removed", id, at, p.Actor,
		map[string]any{"field": field, "text": text})
}

func (e *Enforcer) checklistField(p Principal, id, field string) (*projection.Issue, schema.Field, error) {
	if err := e.refresh(); err != nil {
		return nil, schema.Field{}, err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return nil, schema.Field{}, fmt.Errorf("unknown issue %s", id)
	}
	def, ok := e.schema.Field(field)
	if !ok {
		return nil, schema.Field{}, fmt.Errorf("field %q is not defined in the schema; defined fields are %s",
			field, strings.Join(e.schema.FieldNames(), ", "))
	}
	if def.Type != schema.Checklist {
		return nil, schema.Field{}, fmt.Errorf("field %q is a %s, not a checklist", field, def.Type)
	}
	if err := e.authorise(p, schema.FieldOp(field), id, issue.Team); err != nil {
		return nil, schema.Field{}, err
	}
	return issue, def, nil
}
