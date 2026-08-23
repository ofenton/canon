package enforce

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

func fixture(t *testing.T) (*Enforcer, *event.Store) {
	t.Helper()
	s, err := schema.Load(filepath.Join("..", "schema", "testdata", "canon.yaml"))
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	log, err := event.Open(filepath.Join(t.TempDir(), "canon.db"))
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return New(s, log), log
}

func human() event.Actor { return event.Actor{ID: "ollie", Kind: event.ActorHuman} }

// fixtureWithoutRoles builds an enforcer over a schema that defines no roles, to
// prove authorisation is opt-in.
func fixtureWithoutRoles(t *testing.T) (*Enforcer, *event.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "canon.yaml")
	if err := os.WriteFile(path, []byte(`version: 1
states: [{name: todo, category: open}, {name: done, category: closed}]
transitions: [{from: todo, to: done}]
fields: [{name: title, type: string, required: true}]
issue_types: [{name: task, fields: [title]}]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := schema.Load(path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	log, err := event.Open(filepath.Join(t.TempDir(), "canon.db"))
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return New(s, log), log
}

func at(min int) time.Time { return time.Date(2026, 8, 23, 9, min, 0, 0, time.UTC) }

func mustCreate(t *testing.T, e *Enforcer, id string) {
	t.Helper()
	if err := e.Create(id, "task", map[string]string{"title": "a task"}, at(0), human()); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
}

// AC: WHEN a caller sets a field not defined in canon.yaml THE SYSTEM SHALL reject
// the write and name the valid fields.
func TestRejectsUndefinedField(t *testing.T) {
	e, _ := fixture(t)
	mustCreate(t, e, "CANON-1")

	err := e.SetField("CANON-1", "storyPoints", "8", at(1), human())
	if err == nil {
		t.Fatal("setting an undefined field must be rejected")
	}
	msg := err.Error()
	if !strings.Contains(msg, "storyPoints") {
		t.Errorf("error must name the offending field, got: %v", err)
	}
	for _, valid := range []string{"title", "priority", "component", "evidence"} {
		if !strings.Contains(msg, valid) {
			t.Errorf("error must list the valid fields; %q missing from: %v", valid, err)
		}
	}
}

func TestRejectsValueOutsideEnum(t *testing.T) {
	e, _ := fixture(t)
	mustCreate(t, e, "CANON-1")

	if err := e.SetField("CANON-1", "priority", "p1", at(1), human()); err != nil {
		t.Fatalf("p1 is a valid priority: %v", err)
	}
	err := e.SetField("CANON-1", "priority", "urgent-ish", at(2), human())
	if err == nil {
		t.Fatal("a value outside the enum must be rejected")
	}
	if !strings.Contains(err.Error(), "p1") {
		t.Errorf("error must list the permitted values, got: %v", err)
	}
}

// AC: WHEN a caller transitions to a state not permitted from the current state THE
// SYSTEM SHALL reject the write and name the permitted transitions.
func TestRejectsIllegalTransition(t *testing.T) {
	e, _ := fixture(t)
	mustCreate(t, e, "CANON-1")

	// todo -> done is not in the schema; todo -> in_progress and -> abandoned are.
	err := e.Transition("CANON-1", "done", "", at(1), human())
	if err == nil {
		t.Fatal("an unpermitted transition must be rejected")
	}
	msg := err.Error()
	if !strings.Contains(msg, "in_progress") || !strings.Contains(msg, "abandoned") {
		t.Errorf("error must name the permitted transitions, got: %v", err)
	}

	if err := e.Transition("CANON-1", "in_progress", "", at(2), human()); err != nil {
		t.Fatalf("todo -> in_progress is permitted: %v", err)
	}
}

func TestRejectsUndefinedState(t *testing.T) {
	e, _ := fixture(t)
	mustCreate(t, e, "CANON-1")
	err := e.Transition("CANON-1", "shipped", "", at(1), human())
	if err == nil || !strings.Contains(err.Error(), "shipped") {
		t.Errorf("transition to an undefined state must be rejected naming it, got: %v", err)
	}
}

func TestRejectsUnknownIssueType(t *testing.T) {
	e, _ := fixture(t)
	err := e.Create("CANON-9", "saga", map[string]string{"title": "x"}, at(0), human())
	if err == nil || !strings.Contains(err.Error(), "saga") {
		t.Errorf("unknown issue type must be rejected naming it, got: %v", err)
	}
}

func TestRejectsFieldNotOnIssueType(t *testing.T) {
	e, _ := fixture(t)
	// "epic" declares only title; component belongs to bug.
	err := e.Create("CANON-9", "epic", map[string]string{"title": "x", "component": "search"}, at(0), human())
	if err == nil || !strings.Contains(err.Error(), "component") {
		t.Errorf("a field not on the issue type must be rejected, got: %v", err)
	}
}

func TestRequiredFieldsAreEnforced(t *testing.T) {
	e, _ := fixture(t)
	err := e.Create("CANON-9", "task", map[string]string{}, at(0), human())
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("a missing required field must be rejected naming it, got: %v", err)
	}
}

// Rejected writes must not reach the log. A validation error that still appends is
// worse than no validation, because the log then contains states the schema forbids.
func TestRejectedWritesAppendNothing(t *testing.T) {
	e, log := fixture(t)
	mustCreate(t, e, "CANON-1")
	before, err := log.Count()
	if err != nil {
		t.Fatal(err)
	}

	_ = e.SetField("CANON-1", "storyPoints", "8", at(1), human())
	_ = e.Transition("CANON-1", "done", "", at(2), human())
	_ = e.Create("CANON-2", "saga", map[string]string{"title": "x"}, at(3), human())

	after, err := log.Count()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("rejected writes appended %d events to the log", after-before)
	}
}

// AC: WHEN a schema change would leave existing issues in an undefined state THE
// SYSTEM SHALL refuse to apply it and list the affected issue ids.
func TestRefusesOrphaningSchemaChange(t *testing.T) {
	e, log := fixture(t)
	mustCreate(t, e, "CANON-1")
	mustCreate(t, e, "CANON-2")
	if err := e.Transition("CANON-1", "in_progress", "", at(1), human()); err != nil {
		t.Fatal(err)
	}

	// A schema without in_progress orphans CANON-1 but not CANON-2.
	narrowed := loadSchema(t, `version: 1
states:
  - {name: todo, category: open}
  - {name: done, category: closed}
transitions:
  - {from: todo, to: done}
fields:
  - {name: title, type: string, required: true}
issue_types:
  - {name: task, fields: [title]}
`)
	err := CheckMigration(log, narrowed)
	if err == nil {
		t.Fatal("a schema change that orphans an issue must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "CANON-1") {
		t.Errorf("error must list the affected issue ids, got: %v", err)
	}
	if strings.Contains(msg, "CANON-2") {
		t.Errorf("error must not list unaffected issues, got: %v", err)
	}
	if !strings.Contains(msg, "in_progress") {
		t.Errorf("error should name the removed state, got: %v", err)
	}
}

// AC: WHEN a schema change is purely additive THE SYSTEM SHALL apply it without
// restart or data migration.
func TestAdditiveSchemaChangeApplies(t *testing.T) {
	e, log := fixture(t)
	mustCreate(t, e, "CANON-1")

	widened := loadSchema(t, mustRead(t, filepath.Join("..", "schema", "testdata", "canon.yaml"))+`
`)
	if err := CheckMigration(log, widened); err != nil {
		t.Fatalf("an identical schema must be applicable: %v", err)
	}

	// Adding a state and a field is additive: nothing existing becomes invalid.
	extended := loadSchema(t, strings.Replace(
		mustRead(t, filepath.Join("..", "schema", "testdata", "canon.yaml")),
		"  - name: component\n    type: string\n",
		"  - name: component\n    type: string\n  - name: severity\n    type: enum\n    values: [low, high]\n",
		1))
	if err := CheckMigration(log, extended); err != nil {
		t.Fatalf("adding a field must be applicable: %v", err)
	}

	// Applying it must not require a restart: the enforcer swaps schema in place.
	e.UseSchema(extended)
	if err := e.SetField("CANON-1", "severity", "high", at(5), human()); err != nil {
		t.Fatalf("the new field must be usable immediately: %v", err)
	}
}

// AC: THE SYSTEM SHALL expose no API or UI operation that adds a field, state or
// issue type at runtime.
//
// Asserted against the source rather than by inspection, so it keeps holding.
func TestNoRuntimeSchemaMutation(t *testing.T) {
	forbidden := []string{"AddField", "AddState", "AddIssueType", "AddTransition",
		"CreateField", "DefineField", "RegisterState"}

	fset := token.NewFileSet()
	roots := []string{".", filepath.Join("..", "schema"), filepath.Join("..", "..", "cmd", "canon")}
	for _, root := range roots {
		pkgs, err := parser.ParseDir(fset, root, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", root, err)
		}
		for _, pkg := range pkgs {
			for name, file := range pkg.Files {
				if strings.HasSuffix(name, "_test.go") {
					continue
				}
				ast.Inspect(file, func(n ast.Node) bool {
					fn, ok := n.(*ast.FuncDecl)
					if !ok {
						return true
					}
					for _, bad := range forbidden {
						if fn.Name.Name == bad {
							t.Errorf("%s defines %s: the schema must not be mutable at runtime",
								name, bad)
						}
					}
					return true
				})
			}
		}
	}
}

func loadSchema(t *testing.T, body string) *schema.Schema {
	t.Helper()
	path := filepath.Join(t.TempDir(), "canon.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := schema.Load(path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	return s
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
