package metrics

import (
	"testing"
	"time"

	"github.com/ofenton/canon/internal/ingest"
)

func at(s string) string { return s }

func increment(id, status string, trs ...[2]string) ingest.Increment {
	inc := ingest.Increment{ID: id, Title: id, Status: status, Type: "feature"}
	var prev string
	for _, t := range trs {
		inc.Transitions = append(inc.Transitions, ingest.Transition{
			From: prev, To: t[0], At: t[1], Commit: "deadbee",
		})
		prev = t[0]
	}
	return inc
}

func window() (time.Time, time.Time) {
	to := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	return to.AddDate(0, 0, -30), to
}

// AC: THE SYSTEM SHALL report cycle and lead time from transitions derived from
// commit history.
func TestFlowFromIngestedTransitions(t *testing.T) {
	r := &ingest.Repository{Name: "acme", Increments: []ingest.Increment{
		// approved 09:00, started 10:00, done 14:00 — cycle 4h, lead 5h.
		increment("feat-001", "done",
			[2]string{"approved", at("2026-08-01T09:00:00Z")},
			[2]string{"in-progress", at("2026-08-01T10:00:00Z")},
			[2]string{"done", at("2026-08-01T14:00:00Z")}),
	}}

	from, to := window()
	f := Ingested(r, from, to, 24*time.Hour)

	if f.Completed != 1 {
		t.Fatalf("completed = %d, want 1", f.Completed)
	}
	if got := f.CycleTime.P50 * 24; got < 3.9 || got > 4.1 {
		t.Errorf("cycle p50 = %.2f hours, want 4 — active to closed", got)
	}
	if got := f.LeadTime.P50 * 24; got < 4.9 || got > 5.1 {
		t.Errorf("lead p50 = %.2f hours, want 5 — first seen to closed", got)
	}
}

// An increment removed from the ledger did not finish; it stopped existing. Counting
// it as completed would flatter every repository that ever reverted a plan.
func TestRemovedIsNotCompleted(t *testing.T) {
	r := &ingest.Repository{Name: "acme", Increments: []ingest.Increment{
		increment("feat-001", ingest.Removed,
			[2]string{"approved", at("2026-08-01T09:00:00Z")},
			[2]string{"in-progress", at("2026-08-01T10:00:00Z")},
			[2]string{ingest.Removed, at("2026-08-01T11:00:00Z")}),
	}}

	from, to := window()
	f := Ingested(r, from, to, 24*time.Hour)
	if f.Completed != 0 {
		t.Fatalf("completed = %d; a removed increment did not complete", f.Completed)
	}
}

// Work still open is ageing, not completed, and ageing is the number that moves before
// the damage lands.
func TestUnfinishedWorkAges(t *testing.T) {
	r := &ingest.Repository{Name: "acme", Increments: []ingest.Increment{
		increment("feat-001", "in-progress",
			[2]string{"approved", at("2026-08-01T09:00:00Z")},
			[2]string{"in-progress", at("2026-08-01T10:00:00Z")}),
	}}

	from, to := window()
	f := Ingested(r, from, to, 24*time.Hour)
	if f.InProgress != 1 {
		t.Fatalf("in progress = %d, want 1", f.InProgress)
	}
	if len(f.Ageing) != 1 || f.Ageing[0].ID != "feat-001" {
		t.Fatalf("ageing = %+v, want feat-001", f.Ageing)
	}
	if f.Completed != 0 {
		t.Fatalf("completed = %d, want 0", f.Completed)
	}
}

// The template's statuses are fixed, and every one of them must have a category or
// flow silently ignores work sitting in it.
func TestEveryTemplateStatusIsCategorised(t *testing.T) {
	want := []string{"planned", "approved", "in-progress", "in-review", "done", "abandoned", ingest.Removed}
	s := TemplateSchema()
	for _, name := range want {
		if !s.HasState(name) {
			t.Errorf("status %q has no category, so work sitting in it is invisible to flow", name)
		}
	}
}

// The conversion must not invent a creation time. Lead time measures from the first
// thing that happened, which for a ledger is the first transition.
func TestCreationIsTheFirstTransition(t *testing.T) {
	r := &ingest.Repository{Name: "acme", Increments: []ingest.Increment{
		increment("feat-001", "approved", [2]string{"planned", at("2026-08-01T09:00:00Z")}),
	}}
	issues := FromIngest(r)
	if len(issues) != 1 {
		t.Fatal("expected one issue")
	}
	want := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	if !issues[0].CreatedAt.Equal(want) {
		t.Fatalf("created at %s, want the first transition at %s", issues[0].CreatedAt, want)
	}
}
