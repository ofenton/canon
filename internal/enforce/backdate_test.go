package enforce

import (
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// AC: WHEN a caller supplies at on a write THE SYSTEM SHALL record that instant as
// the event time.
func TestBackdatedWriteRecordsTheSuppliedTime(t *testing.T) {
	e, log := fixture(t)
	admin := actor("ollie", "admin", "platform")
	now := at(30)
	then := at(10)

	if err := e.AuthoriseBackdate(admin, "CANON-1", then, now); err != nil {
		t.Fatalf("authorise backdate: %v", err)
	}
	if err := e.CreateAs(admin, "CANON-1", "story", map[string]string{"title": "Backdated"}, "platform", then); err != nil {
		t.Fatalf("create: %v", err)
	}

	events, err := log.All()
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !events[0].At.Equal(then) {
		t.Fatalf("event time = %s, want the supplied %s", events[0].At, then)
	}
}

// AC: WHEN a caller supplies at in the future THE SYSTEM SHALL refuse the write and
// say so.
func TestFutureDatedWriteIsRefused(t *testing.T) {
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")
	now := at(30)

	err := e.AuthoriseBackdate(admin, "CANON-1", at(45), now)
	if err == nil {
		t.Fatal("expected a future-dated write to be refused")
	}
	if !strings.Contains(err.Error(), "future") {
		t.Fatalf("error should say the time is in the future, got: %v", err)
	}
}

// A write dated before the issue it targets was created would produce a history in
// which an issue was edited before it existed.
func TestWriteDatedBeforeTheIssueExistedIsRefused(t *testing.T) {
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")

	if err := e.CreateAs(admin, "CANON-1", "story", map[string]string{"title": "Real"}, "platform", at(20)); err != nil {
		t.Fatalf("create: %v", err)
	}
	err := e.CheckNotBeforeCreation("CANON-1", at(10))
	if err == nil {
		t.Fatal("expected a write dated before creation to be refused")
	}
	if !strings.Contains(err.Error(), "created") {
		t.Fatalf("error should mention when the issue was created, got: %v", err)
	}
}

// AC: WHEN a caller lacks the backdate permission THE SYSTEM SHALL refuse the write.
func TestBackdateRequiresPermission(t *testing.T) {
	e, _ := fixture(t)
	member := actor("sam", "member", "platform")

	err := e.AuthoriseBackdate(member, "CANON-1", at(10), at(30))
	if err == nil {
		t.Fatal("expected a member without the backdate grant to be refused")
	}
	if !strings.Contains(err.Error(), "backdate") {
		t.Fatalf("error should name the operation, got: %v", err)
	}
}

// Present-dated writes must not need the permission, or every ordinary write would.
func TestPresentDatedWriteNeedsNoPermission(t *testing.T) {
	e, _ := fixture(t)
	member := actor("sam", "member", "platform")
	now := at(30)

	if err := e.AuthoriseBackdate(member, "CANON-1", now, now); err != nil {
		t.Fatalf("a write at the current instant should need no grant: %v", err)
	}
}

// AC: THE SYSTEM SHALL order the log by arrival, not by the supplied time.
//
// This is the property that makes backdating safe: Seq is assigned on append, so a
// backdated event replays after the events that were already there rather than
// rewriting what they produced.
func TestLogIsOrderedByArrivalNotBySuppliedTime(t *testing.T) {
	e, log := fixture(t)
	admin := actor("ollie", "admin", "platform")

	if err := e.CreateAs(admin, "CANON-1", "story", map[string]string{"title": "First"}, "platform", at(20)); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Arrives second, dated earlier.
	if err := e.SetField("CANON-1", "title", "Renamed earlier", at(21), event.Actor{ID: "ollie", Kind: event.ActorHuman}); err != nil {
		t.Fatalf("set field: %v", err)
	}

	events, err := log.All()
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var seqs []int64
	var times []time.Time
	for _, ev := range events {
		seqs = append(seqs, ev.Seq)
		times = append(times, ev.At)
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("sequence must increase on arrival: %v", seqs)
		}
	}

	view, err := e.Projection()
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	issue, _ := view.Issue("CANON-1")
	if issue.Title != "Renamed earlier" {
		t.Fatalf("the later-arriving event must win regardless of its date, got %q", issue.Title)
	}
}

// A grant of "backdate" has to validate against the schema's closed verb list, or
// the permission would be refused by name and this feature would be unreachable.
func TestBackdateIsAKnownVerb(t *testing.T) {
	for _, v := range schema.Verbs {
		if v == BackdateOp {
			return
		}
	}
	t.Fatalf("%q is not in schema.Verbs (%v), so no role could ever be granted it",
		BackdateOp, schema.Verbs)
}
