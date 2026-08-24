package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "canon.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// AC: THE SYSTEM SHALL read the entire issue schema from one canon.yaml at a
// configured path.
func TestLoadsTheWholeSchema(t *testing.T) {
	s, err := Load("testdata/canon.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s.States) != 5 {
		t.Errorf("states: got %d want 5", len(s.States))
	}
	if len(s.Transitions) != 6 {
		t.Errorf("transitions: got %d want 6", len(s.Transitions))
	}
	if len(s.Fields) != 6 {
		t.Errorf("fields: got %d want 6", len(s.Fields))
	}
	// The two richer types must survive a load with their configuration intact.
	if kpi, ok := s.Field("kpi"); !ok || kpi.Type != MultiEnum || len(kpi.Values) != 4 {
		t.Errorf("kpi should be a multi_enum with four values, got %+v", kpi)
	}
	if acc, ok := s.Field("acceptance"); !ok || acc.Type != Checklist {
		t.Errorf("acceptance should be a checklist, got %+v", acc)
	}
	if got := s.RequiredChecklists("in_review"); len(got) != 1 || got[0] != "acceptance" {
		t.Errorf("in_review should require the acceptance checklist, got %v", got)
	}
	if len(s.IssueTypes) != 5 {
		t.Errorf("issue types: got %d want 5", len(s.IssueTypes))
	}
	if !s.HierarchyDeclared() {
		t.Error("the sample schema must declare a hierarchy")
	}

	// The schema must answer the questions enforcement will ask of it.
	if !s.HasState("in_review") || s.HasState("nope") {
		t.Error("HasState is wrong")
	}
	if !s.CanTransition("todo", "in_progress") {
		t.Error("todo -> in_progress must be permitted")
	}
	if s.CanTransition("todo", "done") {
		t.Error("todo -> done must not be permitted")
	}
	if !s.RequiresEvidence("in_review") {
		t.Error("in_review must require evidence")
	}
	if s.RequiresEvidence("done") {
		t.Error("done must not require evidence")
	}
	got := s.PermittedFrom("in_review")
	if len(got) != 2 || got[0] != "done" || got[1] != "in_progress" {
		t.Errorf("PermittedFrom(in_review) = %v, want [done in_progress] sorted", got)
	}
}

// AC: WHEN canon.yaml is syntactically invalid THE SYSTEM SHALL refuse to start and
// name the offending line number.
func TestInvalidSyntaxNamesTheLine(t *testing.T) {
	path := write(t, `version: 1
states:
  - name: todo
   category: open
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected a syntax error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "line 2") {
		t.Errorf("error must name a line number, got: %v", err)
	}
	// yaml.v3 attributes an indentation error to the line the block began on, so a
	// bare number can point two lines above the real fault. The error must therefore
	// show the region, with the offending text in it, or it is not actionable.
	if !strings.Contains(msg, "category: open") {
		t.Errorf("error must show the offending region, got:\n%v", err)
	}
	if !strings.Contains(msg, "->") {
		t.Errorf("error should mark the reported line, got:\n%v", err)
	}
}

// AC: WHEN canon.yaml references an undefined state in a transition THE SYSTEM
// SHALL refuse to start and name the transition.
func TestUndefinedTransitionStateNamesTheTransition(t *testing.T) {
	path := write(t, `version: 1
states:
  - name: todo
    category: open
  - name: done
    category: closed
transitions:
  - from: todo
    to: shipped
fields:
  - name: title
    type: string
issue_types:
  - name: task
    fields: [title]
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected an undefined-state error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "todo") || !strings.Contains(msg, "shipped") {
		t.Errorf("error must name the transition, got: %v", err)
	}
	if !strings.Contains(msg, "line") {
		t.Errorf("error should locate the transition, got: %v", err)
	}
}

func TestRejectsStructurallyInvalidSchemas(t *testing.T) {
	cases := map[string]struct{ yaml, wants string }{
		"no states": {`version: 1
fields: [{name: title, type: string}]
issue_types: [{name: task, fields: [title]}]
hierarchy: {levels: [[task]]}
`, "at least one state"},

		"duplicate state": {`version: 1
states: [{name: todo, category: open}, {name: todo, category: closed}]
fields: [{name: title, type: string}]
issue_types: [{name: task, fields: [title]}]
hierarchy: {levels: [[task]]}
`, "duplicate state"},

		"bad category": {`version: 1
states: [{name: todo, category: sideways}]
fields: [{name: title, type: string}]
issue_types: [{name: task, fields: [title]}]
hierarchy: {levels: [[task]]}
`, "category"},

		"issue type references unknown field": {`version: 1
states: [{name: todo, category: open}]
fields: [{name: title, type: string}]
issue_types: [{name: task, fields: [title, nonexistent]}]
hierarchy: {levels: [[task]]}
`, "nonexistent"},

		"enum without values": {`version: 1
states: [{name: todo, category: open}]
fields: [{name: priority, type: enum}]
issue_types: [{name: task, fields: [priority]}]
hierarchy: {levels: [[task]]}
`, "values"},

		"unknown field type": {`version: 1
states: [{name: todo, category: open}]
fields: [{name: title, type: quaternion}]
issue_types: [{name: task, fields: [title]}]
hierarchy: {levels: [[task]]}
`, "quaternion"},

		"unsupported version": {`version: 99
states: [{name: todo, category: open}]
fields: [{name: title, type: string}]
issue_types: [{name: task, fields: [title]}]
hierarchy: {levels: [[task]]}
`, "version"},

		"unknown key": {`version: 1
states: [{name: todo, category: open}]
fields: [{name: title, type: string}]
issue_types: [{name: task, fields: [title]}]
hierarchy: {levels: [[task]]}
sprints: []
`, "sprints"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(write(t, tc.yaml))
			if err == nil {
				t.Fatalf("expected rejection")
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("error should mention %q, got: %v", tc.wants, err)
			}
		})
	}
}

// Every validation problem should be reported at once. Fixing a schema one error per
// run is the kind of friction that makes people avoid changing it.
func TestReportsAllProblemsAtOnce(t *testing.T) {
	path := write(t, `version: 1
states: [{name: todo, category: sideways}]
transitions: [{from: todo, to: nowhere}]
fields: [{name: priority, type: enum}]
issue_types: [{name: task, fields: [missing]}]
hierarchy: {levels: [[task]]}
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected rejection")
	}
	for _, want := range []string{"sideways", "nowhere", "values", "missing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("all problems should be reported together; missing %q in:\n%v", want, err)
		}
	}
}

func TestMissingFileIsClear(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err == nil || !strings.Contains(err.Error(), "absent.yaml") {
		t.Errorf("error must name the missing path, got: %v", err)
	}
}
