package conform

import (
	"strings"
	"testing"

	"github.com/ofenton/canon/internal/ingest"
)

func inc(id, status string, fields map[string]string) ingest.Increment {
	if fields == nil {
		fields = map[string]string{}
	}
	// The fields every well-formed increment carries, so a fixture testing one rule
	// does not trip the others.
	for k, v := range map[string]string{
		"scope": "s", "test_strategy": "t", "rollback_plan": "r", "risk": "Low",
	} {
		if _, ok := fields[k]; !ok {
			fields[k] = v
		}
	}
	return ingest.Increment{ID: id, Title: id, Status: status, Type: "feature", Fields: fields}
}

func find(r Report, rule string) *Finding {
	for i := range r.Findings {
		if r.Findings[i].Rule == rule {
			return &r.Findings[i]
		}
	}
	return nil
}

// AC: WHEN a repository fails a rule THE SYSTEM SHALL name the rule and the increment,
// and continue with the others.
func TestOneBadIncrementDoesNotStopTheReport(t *testing.T) {
	r := &ingest.Repository{Name: "acme", Increments: []ingest.Increment{
		inc("feat-001", "done", map[string]string{"evidence": "shipped"}),
		inc("feat-002", "in-progres", nil), // typo
		inc("feat-003", "done", nil),       // no evidence
	}}

	rep := Check(r, ingest.CommitStats{})
	if rep.Conforms() {
		t.Fatal("expected errors")
	}
	var subjects []string
	for _, f := range rep.Findings {
		subjects = append(subjects, f.Subject)
	}
	joined := strings.Join(subjects, ",")
	if !strings.Contains(joined, "feat-002") || !strings.Contains(joined, "feat-003") {
		t.Fatalf("both bad increments should be reported, got %v", subjects)
	}
	if f := find(rep, "status"); f == nil || !strings.Contains(f.Message, "in-progres") {
		t.Fatalf("the bad status should be quoted back: %+v", f)
	}
}

// AC: WHEN an increment traces to a requirement that does not exist THE SYSTEM SHALL
// report it. A dangling trace looks fine to any grep and traces nothing.
func TestDanglingTraceIsReported(t *testing.T) {
	one := inc("feat-001", "approved", nil)
	one.Traces = []string{"R1", "R99"}
	r := &ingest.Repository{
		Name:         "acme",
		Requirements: []ingest.Requirement{{ID: "R1", Text: "a real one"}},
		Increments:   []ingest.Increment{one},
	}

	rep := Check(r, ingest.CommitStats{})
	f := find(rep, "trace")
	if f == nil || !strings.Contains(f.Message, "R99") {
		t.Fatalf("expected R99 reported as dangling, got %+v", rep.Findings)
	}
	if strings.Contains(f.Message, "R1") {
		t.Fatal("R1 exists and should not be reported")
	}
}

// AC: THE SYSTEM SHALL report the proportion of commits carrying no increment
// reference. A note, not an error: work that needs no increment is normal.
func TestReferenceDisciplineIsANoteNotAnError(t *testing.T) {
	r := &ingest.Repository{Name: "acme", Increments: []ingest.Increment{inc("feat-001", "approved", nil)}}
	rep := Check(r, ingest.CommitStats{Total: 100, Tracked: 80, Declared: 5})

	f := find(rep, "reference")
	if f == nil {
		t.Fatal("expected a reference-discipline finding")
	}
	if f.Severity != Note {
		t.Fatalf("severity = %s; forcing an increment for every typo is what produces placeholders", f.Severity)
	}
	if !strings.Contains(f.Message, "15 of 100") {
		t.Fatalf("expected 15 unexplained of 100, got %q", f.Message)
	}
	if !rep.Conforms() {
		t.Fatal("a note must not make a repository non-conforming")
	}
}

// The rule no repository-local check can produce: a property of how a team runs the
// loop rather than of any one commit.
func TestUnreliableCycleTimeIsWarned(t *testing.T) {
	quick := inc("feat-001", "done", map[string]string{"evidence": "e"})
	quick.Transitions = []ingest.Transition{
		{To: "in-progress", At: "2026-08-01T10:00:00Z"},
		{From: "in-progress", To: "in-review", At: "2026-08-01T10:00:30Z"},
	}
	real := inc("feat-002", "done", map[string]string{"evidence": "e"})
	real.Transitions = []ingest.Transition{
		{To: "in-progress", At: "2026-08-01T09:00:00Z"},
		{From: "in-progress", To: "in-review", At: "2026-08-01T14:00:00Z"},
	}
	r := &ingest.Repository{Name: "acme", Increments: []ingest.Increment{quick, real}}

	rep := Check(r, ingest.CommitStats{})
	f := find(rep, "cycle-time")
	if f == nil {
		t.Fatal("expected a cycle-time warning")
	}
	if f.Severity != Warning {
		t.Fatalf("severity = %s, want warning — the data is misleading, not malformed", f.Severity)
	}
	if !strings.Contains(f.Message, "1 of 2") {
		t.Fatalf("expected 1 of 2 reported, got %q", f.Message)
	}
}

// A repository where every increment records a real gap must not be warned, or the
// warning means nothing.
func TestHonestCycleTimeIsNotWarned(t *testing.T) {
	one := inc("feat-001", "done", map[string]string{"evidence": "e"})
	one.Transitions = []ingest.Transition{
		{To: "in-progress", At: "2026-08-01T09:00:00Z"},
		{From: "in-progress", To: "in-review", At: "2026-08-01T14:00:00Z"},
	}
	rep := Check(&ingest.Repository{Name: "acme", Increments: []ingest.Increment{one}}, ingest.CommitStats{})
	if f := find(rep, "cycle-time"); f != nil {
		t.Fatalf("a repository with real gaps was warned: %q", f.Message)
	}
}

// The WIP limit is the main brake on an agent half-finishing several things.
func TestMoreThanOneInProgressIsReported(t *testing.T) {
	r := &ingest.Repository{Name: "acme", Increments: []ingest.Increment{
		inc("feat-001", "in-progress", nil),
		inc("feat-002", "in-progress", nil),
	}}
	rep := Check(r, ingest.CommitStats{})
	f := find(rep, "wip")
	if f == nil || !strings.Contains(f.Message, "feat-001, feat-002") {
		t.Fatalf("expected both named, got %+v", rep.Findings)
	}
}

// A conforming repository reports nothing, or the report is noise.
func TestAConformingRepositoryReportsNothing(t *testing.T) {
	one := inc("feat-001", "done", map[string]string{"evidence": "shipped"})
	one.Criteria = []ingest.Criterion{{Text: "it works", Met: true}}
	rep := Check(&ingest.Repository{Name: "acme", Increments: []ingest.Increment{one}},
		ingest.CommitStats{Total: 10, Tracked: 10})
	if len(rep.Findings) != 0 {
		t.Fatalf("a clean repository produced %d findings: %+v", len(rep.Findings), rep.Findings)
	}
	if !rep.Conforms() {
		t.Fatal("a clean repository should conform")
	}
}

// Multi-line fields are written as indented sub-lists under an empty inline value.
// Reading only the inline part reported every increment in this repository as having
// no test strategy — 54 false errors, which is the kind of storm that makes a
// conformance report worthless.
func TestMultiLineFieldsCountAsPresent(t *testing.T) {
	one := inc("feat-001", "approved", map[string]string{
		"test_strategy": "Unit: the parser. Integration: a real repository.",
	})
	rep := Check(&ingest.Repository{Name: "acme", Increments: []ingest.Increment{one}}, ingest.CommitStats{})
	for _, f := range rep.Findings {
		if strings.Contains(f.Message, "test strategy") {
			t.Fatalf("a multi-line test strategy was reported missing: %q", f.Message)
		}
	}
}
