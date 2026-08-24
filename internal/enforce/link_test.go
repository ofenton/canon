package enforce

import (
	"strings"
	"testing"

	"github.com/ofenton/canon/internal/schema"
)

func linkFixture(t *testing.T) (*Enforcer, Principal) {
	t.Helper()
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")
	if err := e.CreateAs(admin, "CANON-1", "story", map[string]string{"title": "Search is slow"}, "platform", at(0)); err != nil {
		t.Fatalf("create: %v", err)
	}
	return e, admin
}

// AC: WHEN a commit is supplied after the fact THE SYSTEM SHALL link it and record
// its original timestamp.
func TestLinkedCommitKeepsItsOwnTimestamp(t *testing.T) {
	e, admin := linkFixture(t)
	authored := at(5)

	_, err := e.LinkCommit(admin, "CANON-1", Commit{
		SHA:        "a1b2c3d4e5f6789",
		Message:    "Reindex on write\n\nA long body that should not be stored.",
		Repository: "ofenton/canon",
		Branch:     "inc/feat-001",
		At:         authored,
	}, at(40))
	if err != nil {
		t.Fatalf("link: %v", err)
	}

	commits, err := e.CommitsOf("CANON-1")
	if err != nil {
		t.Fatalf("read commits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1", len(commits))
	}
	if !commits[0].At.Equal(authored) {
		t.Fatalf("recorded at %s, want the commit's own %s", commits[0].At, authored)
	}
	if commits[0].Message != "Reindex on write" {
		t.Fatalf("message = %q, want only the subject", commits[0].Message)
	}
	if commits[0].LinkedBy != "ollie" {
		t.Fatalf("linked_by = %q, want the actor who recorded it", commits[0].LinkedBy)
	}
}

// AC: WHEN the same commit is linked twice THE SYSTEM SHALL record it once.
func TestLinkingTheSameCommitTwiceRecordsItOnce(t *testing.T) {
	e, admin := linkFixture(t)
	c := Commit{SHA: "a1b2c3d4e5f6789", Message: "Reindex on write", At: at(5)}

	for i := range 3 {
		wrote, err := e.LinkCommit(admin, "CANON-1", c, at(40))
		if err != nil {
			t.Fatalf("link %d: %v", i, err)
		}
		// Only the first call writes, and the caller is told so — a sweep that
		// re-runs must be able to report nothing new rather than repeating a total.
		if want := i == 0; wrote != want {
			t.Fatalf("link %d reported wrote=%v, want %v", i, wrote, want)
		}
	}

	commits, _ := e.CommitsOf("CANON-1")
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1 — a repeated link must be a no-op", len(commits))
	}
}

// A sweep that used short hashes last week must not re-link the same work with full
// ones today.
func TestAbbreviatedAndFullShasAreTheSameCommit(t *testing.T) {
	e, admin := linkFixture(t)
	full := "a1b2c3d4e5f6789012345678901234567890abcd"

	if _, err := e.LinkCommit(admin, "CANON-1", Commit{SHA: full[:7], At: at(5)}, at(40)); err != nil {
		t.Fatalf("link short: %v", err)
	}
	if _, err := e.LinkCommit(admin, "CANON-1", Commit{SHA: full, At: at(5)}, at(40)); err != nil {
		t.Fatalf("link full: %v", err)
	}

	commits, _ := e.CommitsOf("CANON-1")
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1 — %s and %s are the same commit", len(commits), full[:7], full)
	}
}

// AC: THE SYSTEM SHALL list the commits linked to an issue.
func TestCommitsAreListedInAuthorOrder(t *testing.T) {
	e, admin := linkFixture(t)
	// Linked newest-first, which is the order `git log` hands them over in.
	for _, c := range []Commit{
		{SHA: "cccccccc", Message: "third", At: at(9)},
		{SHA: "bbbbbbbb", Message: "second", At: at(7)},
		{SHA: "aaaaaaaa", Message: "first", At: at(5)},
	} {
		if _, err := e.LinkCommit(admin, "CANON-1", c, at(40)); err != nil {
			t.Fatalf("link %s: %v", c.SHA, err)
		}
	}

	commits, _ := e.CommitsOf("CANON-1")
	if len(commits) != 3 {
		t.Fatalf("got %d commits, want 3", len(commits))
	}
	for i := 1; i < len(commits); i++ {
		if commits[i].At.Before(commits[i-1].At) {
			t.Fatalf("commits should read oldest first, got %s then %s",
				commits[i-1].Message, commits[i].Message)
		}
	}
}

// Only a hash is required. Demanding more would be the same mistake as demanding
// twelve fields to create an issue.
func TestASheerHashIsEnough(t *testing.T) {
	e, admin := linkFixture(t)
	if _, err := e.LinkCommit(admin, "CANON-1", Commit{SHA: "abcdef1"}, at(40)); err != nil {
		t.Fatalf("a link with only a sha should be accepted: %v", err)
	}
}

func TestRejectsSomethingThatIsNotACommitID(t *testing.T) {
	e, admin := linkFixture(t)
	for _, bad := range []string{"", "NOJIRA", "abc", "zzzzzzzz", strings.Repeat("a", 41)} {
		if _, err := e.LinkCommit(admin, "CANON-1", Commit{SHA: bad}, at(40)); err == nil {
			t.Fatalf("%q should not be accepted as a commit id", bad)
		}
	}
}

func TestRejectsLinkToUnknownIssue(t *testing.T) {
	e, admin := linkFixture(t)
	_, err := e.LinkCommit(admin, "CANON-404", Commit{SHA: "abcdef1"}, at(40))
	if err == nil || !strings.Contains(err.Error(), "unknown issue") {
		t.Fatalf("expected an unknown-issue error, got: %v", err)
	}
}

// Backdating a link past the grant would make `canon link` a way around it.
func TestLinkingAnOldCommitNeedsTheBackdateGrant(t *testing.T) {
	e, _ := linkFixture(t)
	// A member may link, but holds no backdate grant.
	member := actor("sam", "member", "platform")

	if _, err := e.LinkCommit(member, "CANON-1", Commit{SHA: "abcdef1"}, at(40)); err != nil {
		t.Fatalf("a member should be able to link a commit made now: %v", err)
	}
	_, err := e.LinkCommit(member, "CANON-1", Commit{SHA: "bbbbbbb", At: at(5)}, at(40))
	if err == nil || !strings.Contains(err.Error(), "backdate") {
		t.Fatalf("linking an old commit should need the backdate grant, got: %v", err)
	}
}

func TestLinkIsAKnownVerb(t *testing.T) {
	for _, v := range schema.Verbs {
		if v == LinkOp {
			return
		}
	}
	t.Fatalf("%q is not in schema.Verbs (%v), so no role could ever be granted it",
		LinkOp, schema.Verbs)
}
