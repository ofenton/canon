package projection

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/event"
)

func newLog(t *testing.T) *event.Store {
	t.Helper()
	s, err := event.Open(filepath.Join(t.TempDir(), "canon.db"))
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func at(min int) time.Time {
	return time.Date(2026, 8, 22, 12, min, 0, 0, time.UTC)
}

func human(id string) event.Actor { return event.Actor{ID: id, Kind: event.ActorHuman} }

// seed writes a small but representative history: two issues, one of which is
// created, retitled, transitioned twice and reparented.
func seed(t *testing.T, log *event.Store) {
	t.Helper()
	events := []*event.Event{
		event.New("issue.created", "CANON-1", at(0), human("ollie"),
			map[string]any{"title": "Search is slow", "state": "todo"}),
		event.New("issue.created", "CANON-2", at(1), human("ollie"),
			map[string]any{"title": "Epic: search", "state": "todo"}),
		event.New("field.set", "CANON-1", at(2), human("ollie"),
			map[string]any{"field": "title", "value": "Search p95 is 4.2s"}),
		event.New("issue.reparented", "CANON-1", at(3), human("ollie"),
			map[string]any{"parent": "CANON-2"}),
		event.New("issue.transitioned", "CANON-1", at(4), human("ollie"),
			map[string]any{"from": "todo", "to": "in_progress"}),
		event.New("issue.transitioned", "CANON-1", at(5),
			event.Actor{ID: "agent:one", Kind: event.ActorAgent, Model: "claude-opus-5"},
			map[string]any{"from": "in_progress", "to": "done"}),
	}
	for _, e := range events {
		if err := log.Append(e); err != nil {
			t.Fatalf("seed append: %v", err)
		}
	}
}

// AC: WHEN `canon rebuild` runs THE SYSTEM SHALL discard all projections and
// reproduce identical state from the event log.
func TestRebuildIsDeterministic(t *testing.T) {
	log := newLog(t)
	seed(t, log)

	p := New(log)
	if err := p.Rebuild(); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}
	first := p.Snapshot()

	if err := p.Rebuild(); err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	second := p.Snapshot()

	if first != second {
		t.Errorf("rebuild is not deterministic\n first:  %s\n second: %s", first, second)
	}

	// A fresh projection over the same log must agree with a rebuilt one.
	fresh := New(log)
	if err := fresh.Rebuild(); err != nil {
		t.Fatalf("fresh rebuild: %v", err)
	}
	if fresh.Snapshot() != first {
		t.Errorf("a fresh projection disagrees with a rebuilt one\n fresh: %s\n built: %s",
			fresh.Snapshot(), first)
	}
}

func TestProjectedState(t *testing.T) {
	log := newLog(t)
	seed(t, log)
	p := New(log)
	if err := p.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	one, ok := p.Issue("CANON-1")
	if !ok {
		t.Fatal("CANON-1 missing from projection")
	}
	if one.Title != "Search p95 is 4.2s" {
		t.Errorf("title: got %q, want the later value", one.Title)
	}
	if one.State != "done" {
		t.Errorf("state: got %q want done", one.State)
	}
	if one.Parent != "CANON-2" {
		t.Errorf("parent: got %q want CANON-2", one.Parent)
	}
	if one.CreatedAt != at(0) || one.UpdatedAt != at(5) {
		t.Errorf("timestamps: created %v updated %v", one.CreatedAt, one.UpdatedAt)
	}
	// Provenance must survive projection: the last actor was an agent.
	if one.LastActor.Kind != event.ActorAgent || one.LastActor.Model != "claude-opus-5" {
		t.Errorf("last actor not preserved: %+v", one.LastActor)
	}
	if got := len(p.Children("CANON-2")); got != 1 {
		t.Errorf("CANON-2 children: got %d want 1", got)
	}
}

// Applying only new events must reach the same state as a full rebuild.
func TestIncrementalMatchesRebuild(t *testing.T) {
	log := newLog(t)
	seed(t, log)

	incremental := New(log)
	if err := incremental.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	if err := log.Append(event.New("issue.transitioned", "CANON-2", at(9), human("ollie"),
		map[string]any{"from": "todo", "to": "in_progress"})); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := incremental.Catchup(); err != nil {
		t.Fatalf("catchup: %v", err)
	}

	full := New(log)
	if err := full.Rebuild(); err != nil {
		t.Fatalf("full rebuild: %v", err)
	}
	if incremental.Snapshot() != full.Snapshot() {
		t.Errorf("incremental diverged from rebuild\n inc:  %s\n full: %s",
			incremental.Snapshot(), full.Snapshot())
	}
}

// AC: WHEN a snapshot exists THE SYSTEM SHALL replay only events after it.
func TestSnapshotBoundsReplay(t *testing.T) {
	log := newLog(t)
	seed(t, log)

	p := New(log)
	if err := p.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	state := p.Snapshot()

	restored := New(log)
	if err := restored.Restore(p.Checkpoint()); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Snapshot() != state {
		t.Error("restoring a checkpoint did not reproduce the state")
	}

	// After restoring, catchup must read only events beyond the checkpoint.
	if err := log.Append(event.New("field.set", "CANON-2", at(10), human("ollie"),
		map[string]any{"field": "title", "value": "Epic: search performance"})); err != nil {
		t.Fatalf("append: %v", err)
	}
	before := restored.EventsRead()
	if err := restored.Catchup(); err != nil {
		t.Fatalf("catchup: %v", err)
	}
	if read := restored.EventsRead() - before; read != 1 {
		t.Errorf("catchup read %d events, want 1 — the checkpoint is not bounding replay", read)
	}
}

// AC: THE SYSTEM SHALL rebuild projections for 10,000 events in under 5 seconds.
func TestRebuildThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("throughput test skipped in short mode")
	}
	log := newLog(t)
	const (
		issues = 500
		n      = 10_000
	)
	// Every issue is created before anything else refers to it: a log where a
	// transition precedes its creation is inconsistent, and the projection is
	// meant to refuse it rather than replay it.
	if err := log.AppendBatch(func(yield func(*event.Event) bool) {
		for i := range issues {
			if !yield(event.New("issue.created", fmt.Sprintf("CANON-%d", i), at(0), human("ollie"),
				map[string]any{"title": fmt.Sprintf("issue %d", i), "state": "todo"})) {
				return
			}
		}
		for i := range n - issues {
			subject := fmt.Sprintf("CANON-%d", i%issues)
			e := event.New("field.set", subject, at(1), human("ollie"),
				map[string]any{"field": "note", "value": fmt.Sprintf("update %d", i)})
			if i%3 == 0 {
				e = event.New("issue.transitioned", subject, at(1), human("ollie"),
					map[string]any{"from": "todo", "to": "in_progress"})
			}
			if !yield(e) {
				return
			}
		}
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	p := New(log)
	start := time.Now()
	if err := p.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("rebuilt %d events in %s (%.0f events/sec)", n, elapsed.Round(time.Millisecond),
		float64(n)/elapsed.Seconds())
	if elapsed > 5*time.Second {
		t.Errorf("rebuild of %d events took %s, budget is 5s", n, elapsed)
	}
}

func TestUnknownEventTypeIsRefused(t *testing.T) {
	log := newLog(t)
	if err := log.Append(event.New("issue.exploded", "CANON-1", at(0), human("ollie"), nil)); err != nil {
		t.Fatalf("append: %v", err)
	}
	p := New(log)
	err := p.Rebuild()
	if err == nil {
		t.Fatal("an unknown event type must fail the rebuild, not be skipped")
	}
	if !contains(err.Error(), "issue.exploded") {
		t.Errorf("error must name the offending type, got %q", err)
	}
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
