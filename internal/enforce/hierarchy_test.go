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

// tree builds epic -> story -> two sub-tasks, all as plain issues with parents.
func tree(t *testing.T, e *Enforcer) {
	t.Helper()
	mustCreate(t, e, "EPIC")
	mustCreate(t, e, "STORY")
	mustCreate(t, e, "SUB-1")
	mustCreate(t, e, "SUB-2")
	link := func(child, parent string, min int) {
		if err := e.Reparent(child, parent, at(min), human()); err != nil {
			t.Fatalf("reparent %s -> %s: %v", child, parent, err)
		}
	}
	link("STORY", "EPIC", 1)
	link("SUB-1", "STORY", 2)
	link("SUB-2", "STORY", 3)
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

	for _, id := range []string{"EPIC", "STORY", "SUB-1", "SUB-2"} {
		if _, ok := p.Issue(id); !ok {
			t.Fatalf("%s missing", id)
		}
	}
	story, _ := p.Issue("STORY")
	if story.Parent != "EPIC" {
		t.Errorf("STORY parent: got %q want EPIC", story.Parent)
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
func TestDeleteReparentsChildren(t *testing.T) {
	e, log := fixture(t)
	tree(t, e)

	// Deleting the middle of the tree must lift its children, not orphan them.
	if err := e.Delete("STORY", at(4), human()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	p := view(t, log)

	if _, ok := p.Issue("STORY"); ok {
		t.Error("STORY must be gone from the projection")
	}
	for _, id := range []string{"SUB-1", "SUB-2"} {
		child, ok := p.Issue(id)
		if !ok {
			t.Fatalf("%s was orphaned by the delete", id)
		}
		if child.Parent != "EPIC" {
			t.Errorf("%s parent: got %q, want EPIC (lifted to the grandparent)", id, child.Parent)
		}
	}
	if got := p.Children("EPIC"); len(got) != 2 {
		t.Errorf("EPIC children after delete: got %v want SUB-1 and SUB-2", got)
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
	story, ok := p.Issue("STORY")
	if !ok {
		t.Fatal("STORY was deleted along with its parent; children must survive")
	}
	if story.Parent != "" {
		t.Errorf("STORY parent: got %q, want empty", story.Parent)
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
	if err := e.Delete("STORY", at(4), human()); err != nil {
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
	events, _ := log.Subject("SUB-1")
	var reparents int
	for _, ev := range events {
		if ev.Type == "issue.reparented" {
			reparents++
		}
	}
	if reparents < 2 {
		t.Errorf("SUB-1 has %d reparent events; the lift must be recorded explicitly", reparents)
	}
}

// AC: WHEN a parent reference would create a cycle THE SYSTEM SHALL reject the write.
func TestRejectsCycles(t *testing.T) {
	e, log := fixture(t)
	tree(t, e)

	cases := map[string][2]string{
		"self":       {"STORY", "STORY"},
		"direct":     {"EPIC", "STORY"},
		"transitive": {"EPIC", "SUB-1"},
	}
	for name, pair := range cases {
		t.Run(name, func(t *testing.T) {
			before, _ := log.Count()
			err := e.Reparent(pair[0], pair[1], at(9), human())
			if err == nil {
				t.Fatalf("reparenting %s under %s must be rejected", pair[0], pair[1])
			}
			if !strings.Contains(strings.ToLower(err.Error()), "cycle") {
				t.Errorf("error should say it is a cycle, got: %v", err)
			}
			// The error must show the path, or it is impossible to act on.
			if !strings.Contains(err.Error(), pair[0]) {
				t.Errorf("error should name the issue, got: %v", err)
			}
			// A path that repeats a node reads as a bug in the checker.
			if strings.Contains(err.Error(), pair[0]+" -> "+pair[0]) {
				t.Errorf("cycle path repeats a node, got: %v", err)
			}
			if segments := strings.Split(err.Error(), " -> "); len(segments) > 1 {
				seen := map[string]bool{}
				for _, seg := range segments[1:] {
					seg = strings.TrimSpace(seg)
					if seen[seg] {
						t.Errorf("cycle path repeats %q: %v", seg, err)
					}
					seen[seg] = true
				}
			}
			after, _ := log.Count()
			if after != before {
				t.Error("a rejected reparent must append nothing")
			}
		})
	}

	// A legitimate move within the tree must still work.
	if err := e.Reparent("SUB-1", "EPIC", at(10), human()); err != nil {
		t.Errorf("moving a leaf up the tree is not a cycle: %v", err)
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
