package enforce

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/projection"
)

// mustCreateTyped creates an issue of a given type, so trees respect the schema's
// declared nesting.
func mustCreateTyped(t *testing.T, e *Enforcer, id, issueType string) {
	t.Helper()
	fields := map[string]string{"title": id}
	if err := e.Create(id, issueType, fields, at(0), human()); err != nil {
		t.Fatalf("create %s (%s): %v", id, issueType, err)
	}
}

// tree builds epic → feature → story → two tasks, matching the schema's levels.
func tree(t *testing.T, e *Enforcer) {
	t.Helper()
	mustCreateTyped(t, e, "EPIC", "epic")
	mustCreateTyped(t, e, "FEATURE", "feature")
	mustCreateTyped(t, e, "STORY", "story")
	mustCreateTyped(t, e, "SUB-1", "task")
	mustCreateTyped(t, e, "SUB-2", "bug")
	link := func(child, parent string, min int) {
		if err := e.Reparent(child, parent, at(min), human()); err != nil {
			t.Fatalf("reparent %s -> %s: %v", child, parent, err)
		}
	}
	link("FEATURE", "EPIC", 1)
	link("STORY", "FEATURE", 2)
	link("SUB-1", "STORY", 3)
	link("SUB-2", "STORY", 4)
}

func view(t *testing.T, log *event.Store) *projection.Projection {
	t.Helper()
	p := projection.New(log)
	if err := p.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	return p
}

// AC: THE SYSTEM SHALL store all work as a single Issue entity with an optional
// parent reference.
// AC: THE SYSTEM SHALL contain no storage-level distinction between epic, story and
// sub-task.
func TestHierarchyIsRelationsNotTypes(t *testing.T) {
	e, log := fixture(t)
	tree(t, e)
	p := view(t, log)

	for _, id := range []string{"EPIC", "FEATURE", "STORY", "SUB-1", "SUB-2"} {
		if _, ok := p.Issue(id); !ok {
			t.Fatalf("%s missing", id)
		}
	}
	story, _ := p.Issue("STORY")
	if story.Parent != "FEATURE" {
		t.Errorf("STORY parent: got %q want FEATURE", story.Parent)
	}
	if got := p.Children("STORY"); len(got) != 2 {
		t.Errorf("STORY children: got %v want 2", got)
	}

	// Every event in the log is about an issue. No epic/story/subtask event types,
	// no separate tables — depth is a parent reference and nothing else.
	events, err := log.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if !strings.HasPrefix(ev.Type, "issue.") && !strings.HasPrefix(ev.Type, "field.") {
			t.Errorf("event type %q is not an issue fact", ev.Type)
		}
		for _, forbidden := range []string{"epic", "story", "subtask", "sub_task"} {
			if strings.Contains(strings.ToLower(ev.Type), forbidden) {
				t.Errorf("event type %q encodes a hierarchy level; depth must be a relation", ev.Type)
			}
		}
	}
}

// The projection must expose no per-level concept either.
func TestProjectionHasNoHierarchyTypes(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join("..", "projection"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"Epic", "Story", "SubTask", "Subtask"}
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				for _, bad := range forbidden {
					if ts.Name.Name == bad {
						t.Errorf("%s declares type %s: hierarchy must be a relation, not a type",
							name, bad)
					}
				}
				return true
			})
		}
	}
}

// AC: WHEN an issue with children is deleted THE SYSTEM SHALL re-parent its children
// to that issue's parent.
func TestDeleteRefusesAnIllegalLift(t *testing.T) {
	e, log := fixture(t)
	tree(t, e)

	// Deleting the middle of the tree must lift its children, not orphan them.
	// Deleting the story would lift its tasks under a feature, which the hierarchy
	// forbids — so the delete is refused rather than producing an invalid tree.
	err := e.Delete("STORY", at(5), human())
	if err == nil {
		t.Fatal("deleting a story whose children cannot be lifted must be refused")
	}
	for _, want := range []string{"SUB-1", "SUB-2", "hierarchy"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must name what is in the way; %q missing from: %v", want, err)
		}
	}

	// Move the children out, and the delete becomes legal.
	for _, id := range []string{"SUB-1", "SUB-2"} {
		if err := e.Reparent(id, "", at(6), human()); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Delete("STORY", at(7), human()); err != nil {
		t.Fatalf("delete after clearing the children: %v", err)
	}
	p := view(t, log)
	if _, ok := p.Issue("STORY"); ok {
		t.Error("STORY must be gone from the projection")
	}
	for _, id := range []string{"SUB-1", "SUB-2"} {
		if _, ok := p.Issue(id); !ok {
			t.Errorf("%s was destroyed by the delete", id)
		}
	}
}

// Deleting a root lifts its children to no parent rather than deleting them.
func TestDeleteRootLeavesChildrenParentless(t *testing.T) {
	e, log := fixture(t)
	tree(t, e)
	if err := e.Delete("EPIC", at(4), human()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	p := view(t, log)
	feature, ok := p.Issue("FEATURE")
	if !ok {
		t.Fatal("FEATURE was deleted along with its parent; children must survive")
	}
	if feature.Parent != "" {
		t.Errorf("FEATURE parent: got %q, want empty", feature.Parent)
	}
}

// Deletion is a tombstone, not an erasure: the history stays readable.
func TestDeleteIsRecordedNotErased(t *testing.T) {
	e, log := fixture(t)
	tree(t, e)
	before, err := log.Count()
	if err != nil {
		t.Fatal(err)
	}
	// A feature lifts cleanly to no parent, since an epic is its only legal parent
	// and the epic is what is being deleted.
	if err := e.Delete("EPIC", at(5), human()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	after, err := log.Count()
	if err != nil {
		t.Fatal(err)
	}
	if after <= before {
		t.Error("delete must append events, not remove them")
	}
	// The children's moves must be explicit facts, not an implicit projection rule,
	// so the audit trail says why each child changed parent.
	events, _ := log.Subject("FEATURE")
	var reparents int
	for _, ev := range events {
		if ev.Type == "issue.reparented" {
			reparents++
		}
	}
	if reparents < 2 {
		t.Errorf("FEATURE has %d reparent events; the lift must be recorded explicitly", reparents)
	}
}

// Cycles in the hierarchy are impossible by construction: a child's type sits
// strictly below its parent's, so following parents upward strictly decreases the
// level and must terminate. This replaces the generic cycle check feat-005 added,
// which was guarding a case the type rules make unreachable.
func TestHierarchyCyclesAreImpossibleByConstruction(t *testing.T) {
	e, _ := fixture(t)
	tree(t, e)

	// Every inversion that would have formed a cycle is refused on type grounds.
	inversions := [][2]string{
		{"EPIC", "FEATURE"},
		{"EPIC", "SUB-1"},
		{"FEATURE", "STORY"},
		{"STORY", "SUB-1"},
		{"SUB-1", "SUB-2"}, // same level
	}
	for _, pair := range inversions {
		err := e.Reparent(pair[0], pair[1], at(9), human())
		if err == nil {
			t.Errorf("%s under %s must be refused", pair[0], pair[1])
			continue
		}
		if !strings.Contains(err.Error(), "cannot sit under") &&
			!strings.Contains(err.Error(), "top of the hierarchy") {
			t.Errorf("the refusal should be on type grounds, got: %v", err)
		}
	}

	// Self-parenting is refused for the same reason: a type cannot sit under itself.
	if err := e.Reparent("STORY", "STORY", at(9), human()); err == nil {
		t.Error("an issue must not be its own parent")
	}

	// And a legal move still works.
	if err := e.Reparent("SUB-1", "STORY", at(10), human()); err != nil {
		t.Errorf("a task under a story is legal: %v", err)
	}
}

func TestDeleteUnknownIssue(t *testing.T) {
	e, _ := fixture(t)
	if err := e.Delete("NOPE", at(1), human()); err == nil ||
		!strings.Contains(err.Error(), "NOPE") {
		t.Errorf("deleting an unknown issue must be rejected naming it, got: %v", err)
	}
}

func TestDeletedIssueCannotBeWrittenTo(t *testing.T) {
	e, _ := fixture(t)
	mustCreate(t, e, "CANON-1")
	if err := e.Delete("CANON-1", at(1), human()); err != nil {
		t.Fatal(err)
	}
	if err := e.SetField("CANON-1", "title", "x", at(2), human()); err == nil {
		t.Error("a deleted issue must not accept further writes")
	}
	if err := e.Transition("CANON-1", "in_progress", "", at(3), human()); err == nil {
		t.Error("a deleted issue must not accept transitions")
	}
}

// AC: WHEN a schema change would leave existing issues nested in a way the hierarchy
// does not permit THE SYSTEM SHALL refuse to apply it and name the offending issues.
func TestMigrationRefusesANarrowedHierarchy(t *testing.T) {
	e, log := fixture(t)
	tree(t, e) // epic → feature → story → task, bug

	// A hierarchy without "feature": the story under it becomes illegal, and so
	// does the feature under the epic.
	narrowed := loadSchema(t, `version: 1
states:
  - {name: todo, category: open}
  - {name: in_progress, category: active}
  - {name: in_review, category: active, requires_evidence: true}
  - {name: done, category: closed}
  - {name: abandoned, category: closed}
transitions:
  - {from: todo, to: in_progress}
  - {from: in_progress, to: in_review}
  - {from: in_review, to: done}
fields:
  - {name: title, type: string, required: true}
  - {name: priority, type: enum, values: [p1, p2, p3, p4]}
  - {name: component, type: string}
  - {name: evidence, type: text}
issue_types:
  - {name: epic, fields: [title]}
  - {name: feature, fields: [title, priority]}
  - {name: story, fields: [title, priority]}
  - {name: task, fields: [title, priority]}
  - {name: bug, fields: [title, priority, component]}
hierarchy:
  levels: [[epic], [story], [task, bug], [feature]]
`)
	err := CheckMigration(log, narrowed)
	if err == nil {
		t.Fatal("a hierarchy change that invalidates existing nestings must be refused")
	}
	msg := err.Error()
	for _, want := range []string{"feature under epic", "story under feature", "FEATURE", "STORY"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal must name %q; got:\n%v", want, err)
		}
	}
	// The legal nestings must not be reported.
	if strings.Contains(msg, "task under story") || strings.Contains(msg, "bug under story") {
		t.Errorf("a still-legal nesting was reported as a problem:\n%v", err)
	}
}

// AC: WHEN a schema removes the hierarchy entirely THE SYSTEM SHALL refuse to apply
// it while any issue has a parent.
func TestMigrationRefusesRemovingTheHierarchy(t *testing.T) {
	e, log := fixture(t)
	tree(t, e)

	flat := loadSchema(t, `version: 1
states: [{name: todo, category: open}, {name: done, category: closed}]
transitions: [{from: todo, to: done}]
fields: [{name: title, type: string, required: true}]
issue_types:
  - {name: epic, fields: [title]}
  - {name: feature, fields: [title]}
  - {name: story, fields: [title]}
  - {name: task, fields: [title]}
  - {name: bug, fields: [title]}
`)
	if err := CheckMigration(log, flat); err == nil {
		t.Fatal("removing the hierarchy while issues are nested must be refused")
	}

	// Clear every parent, and the same change becomes applicable.
	for _, id := range []string{"FEATURE", "STORY", "SUB-1", "SUB-2"} {
		if err := e.Reparent(id, "", at(20), human()); err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckMigration(log, flat); err != nil {
		t.Errorf("with no parents left, the change must be applicable: %v", err)
	}
}

// AC: WHEN a schema change is compatible with every existing nesting THE SYSTEM SHALL
// apply it.
func TestMigrationAllowsAWiderHierarchy(t *testing.T) {
	e, log := fixture(t)
	tree(t, e)

	// Same levels, but skipping permitted: strictly more permissive.
	wider := loadSchema(t, mustRead(t, filepath.Join("..", "schema", "testdata", "canon.yaml"))+
		"\n")
	if err := CheckMigration(log, wider); err != nil {
		t.Errorf("an unchanged schema must be applicable: %v", err)
	}

	extended := loadSchema(t, strings.Replace(
		mustRead(t, filepath.Join("..", "schema", "testdata", "canon.yaml")),
		"  allow_skipping: false", "  allow_skipping: true", 1))
	if err := CheckMigration(log, extended); err != nil {
		t.Errorf("permitting skipping is strictly wider and must be applicable: %v", err)
	}
}

// Both kinds of problem must be reported together, so one run tells an operator
// everything they have to fix.
func TestMigrationReportsStatesAndNestingsTogether(t *testing.T) {
	e, log := fixture(t)
	tree(t, e)
	if err := e.Transition("SUB-1", "in_progress", "", at(21), human()); err != nil {
		t.Fatal(err)
	}

	breaking := loadSchema(t, `version: 1
states: [{name: todo, category: open}, {name: done, category: closed}]
transitions: [{from: todo, to: done}]
fields: [{name: title, type: string, required: true}]
issue_types:
  - {name: epic, fields: [title]}
  - {name: feature, fields: [title]}
  - {name: story, fields: [title]}
  - {name: task, fields: [title]}
  - {name: bug, fields: [title]}
hierarchy:
  levels: [[epic], [feature], [task, bug], [story]]
`)
	err := CheckMigration(log, breaking)
	if err == nil {
		t.Fatal("expected refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "in_progress") {
		t.Errorf("the removed state must be reported; got:\n%v", err)
	}
	if !strings.Contains(msg, "hierarchy does not permit") {
		t.Errorf("the illegal nesting must be reported; got:\n%v", err)
	}
}
