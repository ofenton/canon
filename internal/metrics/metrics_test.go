package metrics

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/projection"
	"github.com/ofenton/canon/internal/schema"
)

func testSchema(t *testing.T) *schema.Schema {
	t.Helper()
	s, err := schema.Load(filepath.Join("..", "schema", "testdata", "canon.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func day(n int) time.Time { return time.Date(2026, 8, n, 12, 0, 0, 0, time.UTC) }

// issue builds a projected issue with a transition history.
func issue(id string, created time.Time, steps ...[2]any) *projection.Issue {
	i := &projection.Issue{ID: id, CreatedAt: created, Fields: map[string]string{}}
	for _, step := range steps {
		to := step[0].(string)
		at := step[1].(time.Time)
		from := i.State
		i.Transitions = append(i.Transitions, projection.Transition{From: from, To: to, At: at})
		i.State = to
	}
	return i
}

// AC: WHEN an operator requests flow metrics THE SYSTEM SHALL report cycle time and
// throughput derived from recorded state transitions.
func TestCycleTimeAndThroughputFromTransitions(t *testing.T) {
	s := testSchema(t)
	issues := []*projection.Issue{
		// active on the 2nd, closed on the 4th: 2 days.
		issue("CANON-1", day(1), [2]any{"in_progress", day(2)}, [2]any{"done", day(4)}),
		// active on the 2nd, closed on the 12th: 10 days.
		issue("CANON-2", day(1), [2]any{"in_progress", day(2)}, [2]any{"done", day(12)}),
		// active on the 5th, closed on the 9th: 4 days.
		issue("CANON-3", day(3), [2]any{"in_progress", day(5)}, [2]any{"done", day(9)}),
		// still going.
		issue("CANON-4", day(6), [2]any{"in_progress", day(7)}),
	}

	flow := Compute(issues, s, day(1), day(15), 24*time.Hour)

	if flow.Completed != 3 {
		t.Errorf("completed: got %d want 3", flow.Completed)
	}
	if flow.InProgress != 1 {
		t.Errorf("in progress: got %d want 1", flow.InProgress)
	}
	if flow.CycleTime.Count != 3 {
		t.Fatalf("cycle time count: got %d want 3", flow.CycleTime.Count)
	}
	if flow.CycleTime.P50 != 4 {
		t.Errorf("p50 cycle time: got %v want 4 (2, 4, 10)", flow.CycleTime.P50)
	}
	if flow.CycleTime.Max != 10 {
		t.Errorf("max cycle time: got %v want 10", flow.CycleTime.Max)
	}
	// Lead time is created → closed, which is longer than cycle time.
	if flow.LeadTime.Max != 11 {
		t.Errorf("max lead time: got %v want 11", flow.LeadTime.Max)
	}
	if flow.CycleTime.Sample[0].ID != "CANON-2" {
		t.Errorf("slowest should be listed first, got %v", flow.CycleTime.Sample)
	}

	var total int
	for _, b := range flow.Throughput {
		total += b.Completed
	}
	if total != 3 {
		t.Errorf("throughput total: got %d want 3", total)
	}
}

// Ageing must surface unfinished work, because it moves before cycle time does.
func TestAgeingSurfacesUnfinishedWork(t *testing.T) {
	s := testSchema(t)
	issues := []*projection.Issue{
		issue("CANON-1", day(1), [2]any{"in_progress", day(2)}),
		issue("CANON-2", day(1), [2]any{"in_progress", day(9)}),
		issue("CANON-3", day(1)), // never started
	}
	flow := Compute(issues, s, day(1), day(10), 24*time.Hour)
	if len(flow.Ageing) != 2 {
		t.Fatalf("ageing: got %d want 2", len(flow.Ageing))
	}
	if flow.Ageing[0].ID != "CANON-1" || flow.Ageing[0].Days != 8 {
		t.Errorf("oldest first: got %+v", flow.Ageing[0])
	}
}

// Reopened work must measure from the first time it became active, because that is
// the calendar time the requester actually waited.
func TestReopenedWorkMeasuresFromFirstActive(t *testing.T) {
	s := testSchema(t)
	i := issue("CANON-1", day(1),
		[2]any{"in_progress", day(2)},
		[2]any{"in_review", day(3)},
		[2]any{"in_progress", day(4)},
		[2]any{"in_review", day(5)},
		[2]any{"done", day(6)})
	flow := Compute([]*projection.Issue{i}, s, day(1), day(10), 24*time.Hour)
	if flow.CycleTime.P50 != 4 {
		t.Errorf("cycle time should run from first active (day 2) to close (day 6), got %v", flow.CycleTime.P50)
	}
}

func TestWindowExcludesWorkOutsideIt(t *testing.T) {
	s := testSchema(t)
	issues := []*projection.Issue{
		issue("OLD", day(1), [2]any{"in_progress", day(1)}, [2]any{"done", day(2)}),
		issue("NEW", day(8), [2]any{"in_progress", day(8)}, [2]any{"done", day(9)}),
	}
	flow := Compute(issues, s, day(5), day(12), 24*time.Hour)
	if flow.Completed != 1 {
		t.Errorf("only work closed in the window counts, got %d", flow.Completed)
	}
	if flow.CycleTime.Sample[0].ID != "NEW" {
		t.Errorf("wrong issue measured: %v", flow.CycleTime.Sample)
	}
}

func TestEmptyIsNotAnError(t *testing.T) {
	flow := Compute(nil, testSchema(t), day(1), day(10), 24*time.Hour)
	if flow.Completed != 0 || flow.CycleTime.Count != 0 {
		t.Errorf("empty input should produce empty metrics, got %+v", flow)
	}
}

// AC: WHEN canon.yaml defines a field named as an estimate THE SYSTEM SHALL refuse
// to start.
func TestEstimateFieldsAreRefused(t *testing.T) {
	base := `version: 1
states: [{name: todo, category: open}, {name: done, category: closed}]
transitions: [{from: todo, to: done}]
issue_types: [{name: task, fields: [title]}]
fields:
  - {name: title, type: string, required: true}
`
	for _, name := range []string{"storyPoints", "story_points", "Points", "estimate", "VELOCITY", "t-shirt-size", "effort"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "canon.yaml")
			body := base + "  - {name: " + name + ", type: number}\n"
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := schema.Load(path)
			if err != nil {
				t.Fatalf("schema itself should load: %v", err)
			}
			err = CheckNoEstimateFields(s)
			if err == nil {
				t.Fatalf("field %q must be refused", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error must name the field, got: %v", err)
			}
			if !strings.Contains(err.Error(), "cycle time") {
				t.Errorf("error should point at the alternative, got: %v", err)
			}
		})
	}

	// A legitimate field must not be caught by an over-eager match.
	if err := CheckNoEstimateFields(testSchema(t)); err != nil {
		t.Errorf("the standard schema must load: %v", err)
	}
}

// AC: THE SYSTEM SHALL provide no story point, velocity, estimate or burndown field
// in the schema, API or UI.
//
// Asserted against the source, so it holds when someone later adds a helper.
func TestNoEstimationAnywhereInTheSource(t *testing.T) {
	banned := []string{"StoryPoints", "Velocity", "Burndown", "Estimate", "Estimated"}
	fset := token.NewFileSet()
	roots := []string{".", "../schema", "../projection", "../enforce", "../api", "../query", "../../cmd/canon"}

	for _, root := range roots {
		pkgs, err := parser.ParseDir(fset, root, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", root, err)
		}
		for _, pkg := range pkgs {
			for name, file := range pkg.Files {
				if strings.HasSuffix(name, "_test.go") || strings.Contains(name, "metrics.go") {
					continue
				}
				ast.Inspect(file, func(n ast.Node) bool {
					var ident string
					switch node := n.(type) {
					case *ast.FuncDecl:
						ident = node.Name.Name
					case *ast.TypeSpec:
						ident = node.Name.Name
					case *ast.Field:
						if len(node.Names) > 0 {
							ident = node.Names[0].Name
						}
					default:
						return true
					}
					for _, bad := range banned {
						if ident == bad {
							t.Errorf("%s declares %q: Canon measures flow, it does not estimate", name, ident)
						}
					}
					return true
				})
			}
		}
	}
}

// Work that takes hours must not report as zero. A two-day project reported p50, p85
// and p95 all as 0d, which reads as a broken metric rather than as fast work.
func TestSubDayDurationsSurviveRounding(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want float64
	}{
		{24 * time.Hour, 1},
		{12 * time.Hour, 0.5},
		{time.Hour, 0.0417},
		{15 * time.Minute, 0.0104},
		{time.Minute, 0.0007},
	}
	for _, c := range cases {
		if got := days(c.d); got != c.want {
			t.Errorf("days(%s) = %v, want %v", c.d, got, c.want)
		}
		if c.d > 0 && days(c.d) == 0 {
			t.Errorf("days(%s) rounded to zero", c.d)
		}
	}
}
