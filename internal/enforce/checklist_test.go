package enforce

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ofenton/canon/internal/schema"
)

func story(t *testing.T, e *Enforcer, id string) Principal {
	t.Helper()
	admin := adminPrincipal(t, e)
	if err := e.CreateAs(admin, id, "story", map[string]string{"title": id}, "platform", at(1)); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	return admin
}

// AC: THE SYSTEM SHALL provide a checklist field whose items are individually
// checkable and countable.
func TestChecklistItemsAreCheckableAndCountable(t *testing.T) {
	e, _ := fixture(t)
	p := story(t, e, "S1")

	criteria := []string{
		"WHEN a query contains a quote THE SYSTEM SHALL return matching rows",
		"THE SYSTEM SHALL return identical results for the fixture queries",
		"THE SYSTEM SHALL respond in under 200ms at p95",
	}
	for _, text := range criteria {
		if err := e.AddChecklistItem(p, "S1", "acceptance", text, at(2)); err != nil {
			t.Fatalf("add %q: %v", text, err)
		}
	}

	view, err := e.Projection()
	if err != nil {
		t.Fatal(err)
	}
	issue, _ := view.Issue("S1")
	if done, total := issue.ChecklistProgress("acceptance"); done != 0 || total != 3 {
		t.Fatalf("progress: got %d/%d want 0/3", done, total)
	}

	if err := e.SetChecklistItem(p, "S1", "acceptance", criteria[0], true, at(3)); err != nil {
		t.Fatal(err)
	}
	if err := e.SetChecklistItem(p, "S1", "acceptance", criteria[2], true, at(4)); err != nil {
		t.Fatal(err)
	}
	view, _ = e.Projection()
	issue, _ = view.Issue("S1")
	if done, total := issue.ChecklistProgress("acceptance"); done != 2 || total != 3 {
		t.Errorf("progress: got %d/%d want 2/3", done, total)
	}

	// Who met it, and when, is recorded — that is the point of items being events.
	for _, item := range issue.Checklists["acceptance"] {
		if item.Checked && item.CheckedBy != "ollie" {
			t.Errorf("item %q checked by %q", item.Text, item.CheckedBy)
		}
	}

	// Unchecking is a fact too, not an erasure.
	if err := e.SetChecklistItem(p, "S1", "acceptance", criteria[0], false, at(5)); err != nil {
		t.Fatal(err)
	}
	view, _ = e.Projection()
	issue, _ = view.Issue("S1")
	if done, _ := issue.ChecklistProgress("acceptance"); done != 1 {
		t.Errorf("after unchecking: got %d met want 1", done)
	}
}

// AC: WHEN a state is marked as requiring a complete checklist THE SYSTEM SHALL
// refuse entry to it while any item is unchecked.
func TestIncompleteChecklistBlocksTheTransition(t *testing.T) {
	e, _ := fixture(t)
	p := story(t, e, "S1")
	for _, text := range []string{"first criterion", "second criterion"} {
		if err := e.AddChecklistItem(p, "S1", "acceptance", text, at(2)); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.TransitionAs(p, "S1", "in_progress", "", at(3)); err != nil {
		t.Fatal(err)
	}

	err := e.TransitionAs(p, "S1", "in_review", "tests pass", at(4))
	if err == nil {
		t.Fatal("in_review requires the acceptance checklist to be complete")
	}
	for _, want := range []string{"0 of 2", "first criterion", "second criterion"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must say what is outstanding; %q missing from: %v", want, err)
		}
	}

	if err := e.SetChecklistItem(p, "S1", "acceptance", "first criterion", true, at(5)); err != nil {
		t.Fatal(err)
	}
	err = e.TransitionAs(p, "S1", "in_review", "tests pass", at(6))
	if err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("one of two met should still refuse and say so, got: %v", err)
	}

	if err := e.SetChecklistItem(p, "S1", "acceptance", "second criterion", true, at(7)); err != nil {
		t.Fatal(err)
	}
	if err := e.TransitionAs(p, "S1", "in_review", "tests pass", at(8)); err != nil {
		t.Errorf("with every criterion met the transition must be permitted: %v", err)
	}
}

// An empty checklist counts as complete. Refusing on "no criteria yet" would make
// the gate impossible to pass rather than merely strict.
func TestEmptyChecklistDoesNotBlock(t *testing.T) {
	e, _ := fixture(t)
	p := story(t, e, "S1")
	if err := e.TransitionAs(p, "S1", "in_progress", "", at(3)); err != nil {
		t.Fatal(err)
	}
	if err := e.TransitionAs(p, "S1", "in_review", "ok", at(4)); err != nil {
		t.Errorf("an empty checklist must not block: %v", err)
	}
}

// AC: THE SYSTEM SHALL provide a field type holding several values from a declared set.
// AC: WHEN a multi-value field is given a value outside its declared set THE SYSTEM
// SHALL reject the write naming the permitted values.
func TestMultiValueFields(t *testing.T) {
	e, _ := fixture(t)
	p := story(t, e, "S1")

	if err := e.SetMulti(p, "S1", "kpi", []string{"conversion", "churn"}, at(2)); err != nil {
		t.Fatalf("setting two permitted values: %v", err)
	}
	view, _ := e.Projection()
	issue, _ := view.Issue("S1")
	if got := strings.Join(issue.Multi["kpi"], ","); got != "churn,conversion" {
		t.Errorf("kpi: got %q want both values, sorted", got)
	}

	err := e.SetMulti(p, "S1", "kpi", []string{"conversion", "vibes"}, at(3))
	if err == nil {
		t.Fatal("a value outside the declared set must be refused")
	}
	if !strings.Contains(err.Error(), "vibes") || !strings.Contains(err.Error(), "p95_latency") {
		t.Errorf("the refusal must name the offending value and the permitted ones, got: %v", err)
	}

	// The refused write must not have changed anything.
	view, _ = e.Projection()
	issue, _ = view.Issue("S1")
	if got := strings.Join(issue.Multi["kpi"], ","); got != "churn,conversion" {
		t.Errorf("a refused write changed the value: %q", got)
	}

	if err := e.SetMulti(p, "S1", "priority", []string{"p1"}, at(4)); err == nil {
		t.Error("a single-value enum must not accept a multi-value write")
	}
}

func TestChecklistOperationsAreRefusedOnNonChecklistFields(t *testing.T) {
	e, _ := fixture(t)
	p := story(t, e, "S1")
	if err := e.AddChecklistItem(p, "S1", "priority", "x", at(2)); err == nil {
		t.Error("priority is not a checklist")
	}
	if err := e.AddChecklistItem(p, "S1", "nonexistent", "x", at(2)); err == nil {
		t.Error("an undefined field must be refused")
	}
	if err := e.AddChecklistItem(p, "S1", "acceptance", "  ", at(2)); err == nil {
		t.Error("an empty item must be refused")
	}
	if err := e.SetChecklistItem(p, "S1", "acceptance", "never added", true, at(2)); err == nil {
		t.Error("checking an item that does not exist must be refused")
	}
}

// A state cannot require a checklist that is not one.
func TestSchemaRefusesABadChecklistRequirement(t *testing.T) {
	_, err := schemaFrom(t, `version: 1
states:
  - {name: todo, category: open}
  - {name: done, category: closed, requires_checklist: [priority]}
transitions: [{from: todo, to: done}]
fields:
  - {name: title, type: string, required: true}
  - {name: priority, type: enum, values: [p1]}
issue_types: [{name: task, fields: [title]}]
hierarchy: {levels: [[task]]}
`)
	if err == nil || !strings.Contains(err.Error(), "priority") {
		t.Errorf("requiring a non-checklist field must be refused, got: %v", err)
	}
}

func schemaFrom(t *testing.T, body string) (*schema.Schema, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "canon.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return schema.Load(path)
}
