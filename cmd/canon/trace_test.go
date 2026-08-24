package main

import (
	"strings"
	"testing"
)

// mixedRepo has one of each category, so every branch of the report is exercised by
// the same fixture.
func mixedRepo(t *testing.T) string {
	t.Helper()
	return tempRepo(t, []struct{ message, when string }{
		{"Reindex on write\n\nIncrement: CANON-1", "2026-03-01T10:00:00Z"},                 // tracked
		{"Cache the query plan\n\nIncrement: CANON-1", "2026-03-02T10:00:00Z"},             // tracked
		{"Fix a typo in the README\n\nUntracked: one-word change", "2026-03-03T10:00:00Z"}, // declared
		{"NOJIRA: bump the linter", "2026-03-04T10:00:00Z"},                                // placeholder
		{"Tidy the logs", "2026-03-05T10:00:00Z"},                                          // unexplained
	})
}

// AC: WHEN an operator requests a report over a range THE SYSTEM SHALL give the
// proportion of commits carrying no issue reference.
func TestTraceReportsTheProportion(t *testing.T) {
	dir := mixedRepo(t)
	setUpCanon(t, dir, "ollie")
	createIssueAt(t, dir, "CANON-1", "2026-02-01T09:00:00Z")

	out, err := canonIn(t, dir, "trace", "-range", "main")
	if err != nil {
		t.Fatalf("trace: %v\n%s", err, out)
	}
	if !strings.Contains(out, "5 commits") {
		t.Fatalf("expected the total, got:\n%s", out)
	}
	// Two of five are tracked, so three of five carry no working reference.
	if !strings.Contains(out, "carrying no working issue reference: 60.0%") {
		t.Fatalf("expected the headline proportion, got:\n%s", out)
	}
}

// AC: THE SYSTEM SHALL count deliberately untracked commits separately from
// unexplained ones.
func TestTraceSeparatesDeclaredFromUnexplained(t *testing.T) {
	dir := mixedRepo(t)
	setUpCanon(t, dir, "ollie")
	createIssueAt(t, dir, "CANON-1", "2026-02-01T09:00:00Z")

	out, err := canonIn(t, dir, "trace", "-range", "main")
	if err != nil {
		t.Fatalf("trace: %v\n%s", err, out)
	}
	for _, want := range []string{"tracked", "declared untracked", "placeholder", "unexplained"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected a %q category, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "(20.0% declared, 40.0% not)") {
		t.Fatalf("expected the declared share split out, got:\n%s", out)
	}
}

// AC: THE SYSTEM SHALL name the unexplained commits so they can be linked
// afterwards.
func TestTraceNamesTheUnexplainedCommits(t *testing.T) {
	dir := mixedRepo(t)
	setUpCanon(t, dir, "ollie")
	createIssueAt(t, dir, "CANON-1", "2026-02-01T09:00:00Z")

	out, err := canonIn(t, dir, "trace", "-range", "main")
	if err != nil {
		t.Fatalf("trace: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Tidy the logs") {
		t.Fatalf("the unexplained commit must be named, got:\n%s", out)
	}
	if !strings.Contains(out, "canon link") {
		t.Fatalf("expected the report to say how to fix it, got:\n%s", out)
	}
	// A placeholder is reported as itself, not as a reference.
	if !strings.Contains(out, "placeholder references") || !strings.Contains(out, "bump the linter") {
		t.Fatalf("expected the placeholder named, got:\n%s", out)
	}
}

// A reference that resolves to nothing looks tracked to any grep and is not. This is
// the case a simple "does the message match a ticket pattern" check gets wrong.
func TestTraceCatchesReferencesToIssuesThatDoNotExist(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{
		{"Reindex on write\n\nIncrement: CANON-1", "2026-03-01T10:00:00Z"},
		{"Cache the query plan\n\nIncrement: CANON-999", "2026-03-02T10:00:00Z"},
	})
	setUpCanon(t, dir, "ollie")
	createIssueAt(t, dir, "CANON-1", "2026-02-01T09:00:00Z")

	out, err := canonIn(t, dir, "trace", "-range", "main")
	if err != nil {
		t.Fatalf("trace: %v\n%s", err, out)
	}
	if !strings.Contains(out, "unknown issue") {
		t.Fatalf("expected a dangling reference to be its own category, got:\n%s", out)
	}
	if !strings.Contains(out, "CANON-999") {
		t.Fatalf("expected the dangling id named, got:\n%s", out)
	}
}

// The gate is on unexplained work only. Declaring work untracked is the behaviour
// this is trying to encourage, so it must not trip the threshold.
func TestTraceGateCountsOnlyUnexplainedWork(t *testing.T) {
	dir := mixedRepo(t)
	setUpCanon(t, dir, "ollie")
	createIssueAt(t, dir, "CANON-1", "2026-02-01T09:00:00Z")

	// One of five is unexplained: 20%.
	if _, err := canonIn(t, dir, "trace", "-range", "main", "-max-untracked-pct", "25"); err != nil {
		t.Fatalf("20%% unexplained should pass a 25%% threshold: %v", err)
	}
	_, err := canonIn(t, dir, "trace", "-range", "main", "-max-untracked-pct", "10")
	if err == nil {
		t.Fatal("20% unexplained should fail a 10% threshold")
	}
	if !strings.Contains(err.Error(), "20.0%") {
		t.Fatalf("the failure should give the measured number, got: %v", err)
	}
}

// Counting commits must not require a database, or this is useless in CI on a fresh
// checkout.
func TestTraceWorksWithoutALog(t *testing.T) {
	dir := mixedRepo(t)

	out, err := canonIn(t, dir, "trace", "-range", "main")
	if err != nil {
		t.Fatalf("trace without a log: %v\n%s", err, out)
	}
	if !strings.Contains(out, "5 commits") {
		t.Fatalf("expected the report anyway, got:\n%s", out)
	}
	// Without a log nothing can be checked for existence, so a reference is taken at
	// face value rather than being reported as dangling.
	if strings.Contains(out, "unknown issue") {
		t.Fatalf("no log means no existence check, got:\n%s", out)
	}
}

func TestClassifyPrefersTheMostSpecificReading(t *testing.T) {
	known := map[string]string{"CANON-1": "CANON-1"}
	cases := []struct {
		message string
		want    classification
	}{
		{"Reindex\n\nIncrement: CANON-1", tracked},
		{"Reindex\n\nIncrement: CANON-9", danglingRef},
		{"Typo\n\nUntracked: one-word change", declared},
		{"NOJIRA: bump the linter", placeholder},
		{"NO-TICKET bump the linter", placeholder},
		{"Tidy the logs", unexplained},
		// A declaration beats a placeholder in the same message.
		{"NOJIRA bump\n\nUntracked: dependency bump", declared},
		// A real reference beats both.
		{"NOJIRA bump\n\nIncrement: CANON-1", tracked},
		// "Untracked:" with no reason is NOJIRA with extra characters.
		{"Tidy\n\nUntracked:", unexplained},
	}
	for _, c := range cases {
		got := classify(gitCommit{Subject: strings.SplitN(c.message, "\n", 2)[0], Body: c.message}, known)
		if got.what != c.want {
			t.Errorf("classify(%q) = %v, want %v", c.message, got.what, c.want)
		}
	}
}

// A merge commit is not work — the commits it joins are, and they are already in the
// range. Counting merges as unexplained produced a number nobody would act on.
func TestTraceIgnoresMergeCommitsByDefault(t *testing.T) {
	dir := mergeRepo(t)

	out, err := canonIn(t, dir, "trace", "-range", "HEAD")
	if err != nil {
		t.Fatalf("trace: %v\n%s", err, out)
	}
	if strings.Contains(out, "Merge branch") {
		t.Fatalf("a merge commit is not unexplained work, got:\n%s", out)
	}

	withMerges, err := canonIn(t, dir, "trace", "-range", "HEAD", "-merges")
	if err != nil {
		t.Fatalf("trace -merges: %v\n%s", err, withMerges)
	}
	if !strings.Contains(withMerges, "Merge branch") {
		t.Fatalf("-merges should include them, got:\n%s", withMerges)
	}
}

// mergeRepo has a real merge commit joining two branches.
func mergeRepo(t *testing.T) string {
	t.Helper()
	dir := tempRepo(t, []struct{ message, when string }{
		{"Reindex on write\n\nIncrement: CANON-1", "2026-03-01T10:00:00Z"},
	})
	gitIn(t, dir, "checkout", "-q", "-b", "side")
	writeAndCommit(t, dir, "side.txt", "Side work\n\nIncrement: CANON-1", "2026-03-02T10:00:00Z")
	gitIn(t, dir, "checkout", "-q", "main")
	writeAndCommit(t, dir, "main.txt", "Main work\n\nIncrement: CANON-1", "2026-03-03T10:00:00Z")
	gitIn(t, dir, "merge", "-q", "--no-ff", "-m", "Merge branch 'side'", "side")
	return dir
}

// Canon's own ledger uses lower-case ids; plenty of teams use upper. Guessing either
// way made `canon trace` call a commit tracked while `canon link` refused the same
// commit as unknown — found by importing Canon's own ledger and linking its commits.
func TestReferencesResolveWhateverTheirCase(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{
		{"Reindex on write\n\nIncrement: canon-1", "2026-03-01T10:00:00Z"},
		{"Cache the query plan\n\nIncrement: CANON-1", "2026-03-02T10:00:00Z"},
	})
	setUpCanon(t, dir, "ollie")
	createIssueAt(t, dir, "CANON-1", "2026-02-01T09:00:00Z")

	// Both castings are tracked...
	out, err := canonIn(t, dir, "trace", "-range", "main")
	if err != nil {
		t.Fatalf("trace: %v\n%s", err, out)
	}
	if !strings.Contains(out, "tracked") || !strings.Contains(out, "100.0%") {
		t.Fatalf("both castings should be tracked, got:\n%s", out)
	}

	// ...and both actually link, which is the half that used to fail.
	linked, err := canonIn(t, dir, "link", "-actor", "ollie", "-range", "main")
	if err != nil {
		t.Fatalf("link: %v\n%s", err, linked)
	}
	if !strings.Contains(linked, "linked 2 commit(s)") {
		t.Fatalf("both commits should link, got:\n%s", linked)
	}
}

// A reference naming nothing is reported as written, so a reader can find it in the
// commit message rather than a normalised form of it.
func TestUnknownReferenceIsReportedAsWritten(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{
		{"Cache the query plan\n\nIncrement: feat-999", "2026-03-02T10:00:00Z"},
	})
	setUpCanon(t, dir, "ollie")
	createIssueAt(t, dir, "CANON-1", "2026-02-01T09:00:00Z")

	out, err := canonIn(t, dir, "trace", "-range", "main")
	if err != nil {
		t.Fatalf("trace: %v\n%s", err, out)
	}
	if !strings.Contains(out, "feat-999") {
		t.Fatalf("expected the reference as written, got:\n%s", out)
	}
}
