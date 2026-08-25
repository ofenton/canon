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

	"github.com/ofenton/canon/internal/ingest"
)

func increment(id, status string, steps ...[2]string) ingest.Increment {
	inc := ingest.Increment{ID: id, Title: id, Status: status, Type: "feature"}
	var prev string
	for _, s := range steps {
		inc.Transitions = append(inc.Transitions, ingest.Transition{
			From: prev, To: s[0], At: s[1], Commit: "deadbee",
		})
		prev = s[0]
	}
	return inc
}

func window() (time.Time, time.Time) {
	to := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	return to.AddDate(0, 0, -30), to
}

// Cycle time is active to closed; lead time is first seen to closed. A ledger records
// no separate creation, so the first transition is the creation.
func TestCycleAndLeadTime(t *testing.T) {
	from, to := window()
	f := Compute([]ingest.Increment{
		increment("feat-001", "done",
			[2]string{"approved", "2026-08-01T09:00:00Z"},
			[2]string{"in-progress", "2026-08-01T10:00:00Z"},
			[2]string{"done", "2026-08-01T14:00:00Z"}),
	}, from, to, 24*time.Hour)

	if f.Completed != 1 {
		t.Fatalf("completed = %d, want 1", f.Completed)
	}
	if got := f.CycleTime.P50 * 24; got < 3.9 || got > 4.1 {
		t.Errorf("cycle p50 = %.2fh, want 4 — active to closed", got)
	}
	if got := f.LeadTime.P50 * 24; got < 4.9 || got > 5.1 {
		t.Errorf("lead p50 = %.2fh, want 5 — first seen to closed", got)
	}
}

// An increment removed from the ledger did not finish; it stopped existing. Counting
// it as completed would flatter every repository that reverted a plan.
func TestRemovedIsNotCompleted(t *testing.T) {
	from, to := window()
	f := Compute([]ingest.Increment{
		increment("feat-001", ingest.Removed,
			[2]string{"in-progress", "2026-08-01T10:00:00Z"},
			[2]string{ingest.Removed, "2026-08-01T11:00:00Z"}),
	}, from, to, 24*time.Hour)

	if f.Completed != 0 {
		t.Fatalf("completed = %d; a removed increment did not complete", f.Completed)
	}
}

// Ageing is the number that moves before the damage lands.
func TestUnfinishedWorkAges(t *testing.T) {
	from, to := window()
	f := Compute([]ingest.Increment{
		increment("feat-001", "in-progress", [2]string{"in-progress", "2026-08-01T10:00:00Z"}),
	}, from, to, 24*time.Hour)

	if f.InProgress != 1 || len(f.Ageing) != 1 {
		t.Fatalf("in progress %d, ageing %d, want 1 and 1", f.InProgress, len(f.Ageing))
	}
}

// Across is the question only an aggregator can answer.
func TestAcrossCombinesProducts(t *testing.T) {
	from, to := window()
	one := &ingest.Repository{Increments: []ingest.Increment{
		increment("a-001", "done", [2]string{"in-progress", "2026-08-01T09:00:00Z"}, [2]string{"done", "2026-08-01T10:00:00Z"})}}
	two := &ingest.Repository{Increments: []ingest.Increment{
		increment("b-001", "done", [2]string{"in-progress", "2026-08-02T09:00:00Z"}, [2]string{"done", "2026-08-02T10:00:00Z"})}}

	if f := Across([]*ingest.Repository{one, two}, from, to, 24*time.Hour); f.Completed != 2 {
		t.Fatalf("completed = %d across two products, want 2", f.Completed)
	}
}

// Every status the template fixes must have a category, or work sitting in it is
// invisible to flow.
func TestEveryTemplateStatusIsCategorised(t *testing.T) {
	for _, name := range []string{"planned", "approved", "in-progress", "in-review", "done", "abandoned", ingest.Removed} {
		if Category(name) == "" {
			t.Errorf("status %q has no category, so work sitting in it is invisible", name)
		}
	}
	if Category("invented") != "" {
		t.Error("an unknown status must not acquire a category")
	}
}

// AC (standing, from the constitution): no estimation, refused by name.
func TestEstimateFieldsAreRefused(t *testing.T) {
	for _, name := range []string{"storyPoints", "story_points", "Estimate", "velocity", "t-shirt-size"} {
		err := CheckNoEstimateFields(map[string]string{name: "8"})
		if err == nil {
			t.Errorf("field %q is an estimate and should be refused", name)
		}
	}
	if err := CheckNoEstimateFields(map[string]string{"scope": "a thing", "risk": "Low"}); err != nil {
		t.Fatalf("ordinary fields must be accepted: %v", err)
	}
}

// Asserted structurally rather than by convention: a rule enforced only by good
// intentions lasts until the first person who wants a story point field.
func TestNoEstimationAnywhereInTheSource(t *testing.T) {
	banned := []string{"storyPoints", "StoryPoints", "velocity", "burndown"}
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.Contains(path, "metrics") {
			return nil // this file names them in order to refuse them
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			for _, b := range banned {
				if ident.Name == b {
					t.Errorf("%s declares %q; Canon measures flow and has no estimation", path, b)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
