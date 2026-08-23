package schema

import (
	"strings"
	"testing"
)

// AC: THE SYSTEM SHALL declare the permitted nesting of issue types as ordered levels
// in canon.yaml, with several types allowed at one level.
func TestNestingRules(t *testing.T) {
	s, err := Load("testdata/canon.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !s.HierarchyDeclared() {
		t.Fatal("no hierarchy declared")
	}

	legal := [][2]string{
		{"feature", "epic"},
		{"story", "feature"},
		{"task", "story"},
		{"bug", "story"}, // two types share a level
	}
	for _, pair := range legal {
		if err := s.CanNest(pair[0], pair[1]); err != nil {
			t.Errorf("a %s under a %s should be permitted: %v", pair[0], pair[1], err)
		}
	}

	illegal := [][2]string{
		{"epic", "feature"}, // inverted
		{"epic", "task"},    // badly inverted
		{"story", "epic"},   // skips a level, and skipping is off
		{"task", "epic"},
		{"bug", "task"}, // same level
		{"feature", "feature"},
	}
	for _, pair := range illegal {
		if err := s.CanNest(pair[0], pair[1]); err == nil {
			t.Errorf("a %s under a %s must be refused", pair[0], pair[1])
		}
	}
}

// AC: WHEN a caller sets a parent whose type is not the level immediately above the
// child's THE SYSTEM SHALL name the permitted parent types.
func TestRefusalNamesThePermittedParents(t *testing.T) {
	s, err := Load("testdata/canon.yaml")
	if err != nil {
		t.Fatal(err)
	}
	err = s.CanNest("task", "epic")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "story") {
		t.Errorf("the error must say what a task may sit under, got: %v", err)
	}

	if err := s.CanNest("epic", "feature"); err == nil {
		t.Fatal("expected a refusal")
	} else if !strings.Contains(err.Error(), "top of the hierarchy") {
		t.Errorf("a top-level type should say so, got: %v", err)
	}

	if got := strings.Join(s.ParentTypesFor("task"), ","); got != "story" {
		t.Errorf("ParentTypesFor(task) = %q", got)
	}
	if got := strings.Join(s.ChildTypesFor("story"), ","); got != "bug,task" {
		t.Errorf("ChildTypesFor(story) = %q", got)
	}
	if got := s.ParentTypesFor("epic"); len(got) != 0 {
		t.Errorf("an epic has no permitted parents, got %v", got)
	}
}

func TestSkippingWhenAllowed(t *testing.T) {
	s := load(t, `version: 1
states: [{name: todo, category: open}]
fields: [{name: title, type: string, required: true}]
issue_types:
  - {name: epic, fields: [title]}
  - {name: story, fields: [title]}
  - {name: task, fields: [title]}
hierarchy:
  levels: [[epic], [story], [task]]
  allow_skipping: true
`)
	if err := s.CanNest("task", "epic"); err != nil {
		t.Errorf("skipping is allowed here: %v", err)
	}
	if err := s.CanNest("epic", "task"); err == nil {
		t.Error("skipping must not permit inversion")
	}
	if got := strings.Join(s.ParentTypesFor("task"), ","); got != "epic,story" {
		t.Errorf("with skipping, a task may sit under either, got %q", got)
	}
}

// AC: WHEN canon.yaml declares a hierarchy THE SYSTEM SHALL require every issue type
// to appear in exactly one level.
func TestEveryTypeMustBePlaced(t *testing.T) {
	base := `version: 1
states: [{name: todo, category: open}]
fields: [{name: title, type: string, required: true}]
issue_types:
  - {name: epic, fields: [title]}
  - {name: story, fields: [title]}
  - {name: task, fields: [title]}
`
	cases := map[string]struct{ hierarchy, wants string }{
		"a type left out":     {"hierarchy: {levels: [[epic], [story]]}", "task"},
		"a type placed twice": {"hierarchy: {levels: [[epic], [story, epic], [task]]}", "more than one"},
		"an undefined type":   {"hierarchy: {levels: [[epic], [saga], [story], [task]]}", "saga"},
		"an empty level":      {"hierarchy: {levels: [[epic], [], [story], [task]]}", "empty"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(write(t, base+tc.hierarchy+"\n"))
			if err == nil {
				t.Fatal("expected rejection")
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("error should mention %q, got: %v", tc.wants, err)
			}
		})
	}
}

// A schema with no hierarchy permits no nesting at all, and says so.
func TestNoHierarchyMeansNoNesting(t *testing.T) {
	s := load(t, `version: 1
states: [{name: todo, category: open}]
fields: [{name: title, type: string, required: true}]
issue_types: [{name: task, fields: [title]}]
`)
	if s.HierarchyDeclared() {
		t.Fatal("no hierarchy was declared")
	}
	err := s.CanNest("task", "task")
	if err == nil {
		t.Fatal("nesting must be refused with no hierarchy")
	}
	if !strings.Contains(err.Error(), "declares no hierarchy") {
		t.Errorf("the refusal should say what is missing, got: %v", err)
	}
}

func load(t *testing.T, body string) *Schema {
	t.Helper()
	s, err := Load(write(t, body))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return s
}
