package ingest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fixedTime = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

func now() time.Time { return fixedTime }

// repo builds a real git repository and commits a sequence of ledger states, each at
// a given time. Real git, because deriving history from git is the whole feature and
// a fake would test the fake.
func repo(t *testing.T, states []struct{ ledger, when, message string }) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	dir := t.TempDir()

	run := func(env []string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run(nil, "init", "-q", "-b", "main")
	run(nil, "config", "user.email", "t@example.com")
	run(nil, "config", "user.name", "T")

	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if err := os.WriteFile(filepath.Join(dir, LedgerPath), []byte(s.ledger), 0o644); err != nil {
			t.Fatal(err)
		}
		run(nil, "add", LedgerPath)
		run([]string{"GIT_AUTHOR_DATE=" + s.when, "GIT_COMMITTER_DATE=" + s.when},
			"commit", "-q", "-m", s.message)
	}
	return dir
}

func ledger(status string) string {
	return `# Increment plan

## feat-001: Reindex on write

- **Type:** feature
- **Status:** ` + status + `
- **Tier:** 2 (High)
- **Traces:** R1, R2
- **Scope:** Reindex on write. No other changes.
- **Acceptance Criteria:**
  - [ ] WHEN a row is written THE SYSTEM SHALL reindex it
  - [ ] THE SYSTEM SHALL respond in under 200ms
- **Dependencies:** none
- **Risk:** Low
- **Evidence:** _(filled in at verify)_

## Sequencing

Prose about the plan, which is not an increment.
`
}

// AC: WHEN given a repository containing specs/increment-plan.md THE SYSTEM SHALL
// ingest every increment without per-repository configuration.
func TestIngestReadsTheLedger(t *testing.T) {
	dir := repo(t, []struct{ ledger, when, message string }{
		{ledger("planned"), "2026-08-01T09:00:00Z", "plan feat-001"},
	})

	r, err := Repo(dir, now)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if len(r.Increments) != 1 {
		t.Fatalf("got %d increments, want 1 — 'Sequencing' is prose, not an increment", len(r.Increments))
	}
	inc := r.Increments[0]
	if inc.ID != "feat-001" || inc.Title != "Reindex on write" {
		t.Fatalf("parsed %q / %q", inc.ID, inc.Title)
	}
	if inc.Status != "planned" || inc.Type != "feature" {
		t.Fatalf("status %q type %q", inc.Status, inc.Type)
	}
	if len(inc.Traces) != 2 || inc.Traces[0] != "R1" {
		t.Fatalf("traces = %v, want R1 and R2", inc.Traces)
	}
	if len(inc.DependsOn) != 0 {
		t.Fatalf("dependencies = %v, want none — 'none' is not an id", inc.DependsOn)
	}
	if len(inc.Criteria) != 2 || inc.Criteria[0].Met {
		t.Fatalf("criteria = %+v, want two, unmet", inc.Criteria)
	}
	if r.IngestedAt != fixedTime {
		t.Fatalf("ingested_at = %s, want the supplied clock", r.IngestedAt)
	}
}

// AC: THE SYSTEM SHALL derive each status transition and its timestamp from the
// ledger file's commit history rather than approximating it.
func TestTransitionsComeFromCommitHistory(t *testing.T) {
	dir := repo(t, []struct{ ledger, when, message string }{
		{ledger("planned"), "2026-08-01T09:00:00Z", "plan"},
		{ledger("approved"), "2026-08-01T11:30:00Z", "approve"},
		{ledger("in-progress"), "2026-08-02T09:15:00Z", "start"},
		{ledger("in-review"), "2026-08-02T17:45:00Z", "hand over"},
		{ledger("done"), "2026-08-03T10:00:00Z", "ship"},
	})

	r, err := Repo(dir, now)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	got := r.Increments[0].Transitions
	want := []struct{ from, to, at string }{
		{"", "planned", "2026-08-01T09:00:00Z"},
		{"planned", "approved", "2026-08-01T11:30:00Z"},
		{"approved", "in-progress", "2026-08-02T09:15:00Z"},
		{"in-progress", "in-review", "2026-08-02T17:45:00Z"},
		{"in-review", "done", "2026-08-03T10:00:00Z"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d transitions, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].From != w.from || got[i].To != w.to || got[i].At != w.at {
			t.Errorf("transition %d = %s->%s at %s, want %s->%s at %s",
				i, got[i].From, got[i].To, got[i].At, w.from, w.to, w.at)
		}
		if got[i].Commit == "" {
			t.Errorf("transition %d names no commit", i)
		}
	}
}

// A commit that does not change a status must not invent a transition. The previous
// mechanism produced a transition per commit carrying the increment's trailer, which
// is how it came to report nine minutes for four hours of work.
func TestUnrelatedCommitsProduceNoTransitions(t *testing.T) {
	body := ledger("in-progress")
	dir := repo(t, []struct{ ledger, when, message string }{
		{ledger("planned"), "2026-08-01T09:00:00Z", "plan"},
		{ledger("in-progress"), "2026-08-01T10:00:00Z", "start"},
		{body + "\nA further note about the plan.\n", "2026-08-01T11:00:00Z", "edit the prose"},
		{body + "\nAnother note.\n", "2026-08-01T12:00:00Z", "edit again"},
	})

	r, _ := Repo(dir, now)
	if n := len(r.Increments[0].Transitions); n != 2 {
		t.Fatalf("got %d transitions across 4 commits, want 2 — only status changes count", n)
	}
}

// AC: WHEN a repository is ingested twice THE SYSTEM SHALL produce the same result.
func TestIngestIsDeterministic(t *testing.T) {
	dir := repo(t, []struct{ ledger, when, message string }{
		{ledger("planned"), "2026-08-01T09:00:00Z", "plan"},
		{ledger("done"), "2026-08-02T09:00:00Z", "ship"},
	})

	first, err := Repo(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Repo(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint() != second.Fingerprint() {
		t.Fatalf("two ingests of %s disagree:\n  %s\n  %s", first.Head, first.Fingerprint(), second.Fingerprint())
	}
	if first.Head != second.Head {
		t.Fatal("head differs between ingests of an unchanged repository")
	}
}

// Two status changes in one commit share a timestamp. Stated as a limit rather than
// smoothed over: it is exact about what git recorded.
func TestTwoChangesInOneCommitShareAnInstant(t *testing.T) {
	two := strings.Replace(ledger("in-review"), "## Sequencing", `## feat-002: Second thing

- **Type:** fix
- **Status:** done
- **Scope:** Something else.
- **Evidence:** done

## Sequencing`, 1)

	dir := repo(t, []struct{ ledger, when, message string }{
		{ledger("planned"), "2026-08-01T09:00:00Z", "plan"},
		{two, "2026-08-01T09:00:30Z", "move both at once"},
	})

	r, _ := Repo(dir, now)
	if len(r.Increments) != 2 {
		t.Fatalf("got %d increments, want 2", len(r.Increments))
	}
	var at1, at2 string
	for _, inc := range r.Increments {
		last := inc.Transitions[len(inc.Transitions)-1]
		if inc.ID == "feat-001" {
			at1 = last.At
		} else {
			at2 = last.At
		}
	}
	if at1 != at2 {
		t.Fatalf("changes in one commit reported at %s and %s; they share an instant", at1, at2)
	}
}

// A malformed increment must not take the readable ones with it. This meets other
// people's markdown, and refusing a whole repository over one bad entry would make
// Canon useless for exactly the repositories most worth reporting on.
func TestAMalformedEntryDoesNotLoseTheOthers(t *testing.T) {
	broken := strings.Replace(ledger("done"), "## Sequencing", `## feat-002: Half written

- **Type:**
- no status at all
- **Scope:** Something.

## Sequencing`, 1)

	dir := repo(t, []struct{ ledger, when, message string }{
		{broken, "2026-08-01T09:00:00Z", "one good, one broken"},
	})

	r, err := Repo(dir, now)
	if err != nil {
		t.Fatalf("a malformed entry should not fail the ingest: %v", err)
	}
	if len(r.Increments) != 2 {
		t.Fatalf("got %d increments, want both parsed", len(r.Increments))
	}
	for _, inc := range r.Increments {
		if inc.ID == "feat-002" && inc.Status != "" {
			t.Fatalf("feat-002 has no status and should not have acquired one: %q", inc.Status)
		}
	}
}

// A repository with no ledger is not one this reads. Saying so is more useful than
// returning an empty result that looks like a product with no work.
func TestARepositoryWithNoLedgerIsRefused(t *testing.T) {
	dir := t.TempDir()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}} {
		exec.Command("git", append([]string{"-C", dir}, args...)...).Run()
	}
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644)
	exec.Command("git", "-C", dir, "add", "README.md").Run()
	exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run()

	_, err := Repo(dir, now)
	if err == nil || !strings.Contains(err.Error(), LedgerPath) {
		t.Fatalf("expected a refusal naming %s, got: %v", LedgerPath, err)
	}
}

// A spec is optional: a repository with a ledger and no spec is mid-adoption, and
// reporting on it is more useful than refusing it.
func TestASpecIsOptional(t *testing.T) {
	dir := repo(t, []struct{ ledger, when, message string }{
		{ledger("planned"), "2026-08-01T09:00:00Z", "plan"},
	})

	r, err := Repo(dir, now)
	if err != nil {
		t.Fatalf("a repository with no spec should still ingest: %v", err)
	}
	if r.Name == "" {
		t.Fatal("a product with no spec still needs a name")
	}
}

// An increment can leave the ledger — a revert, or a plan withdrawn — and come back.
// Recording the removal is what keeps the history continuous: without it, an
// increment that vanished and returned reads as having been created twice.
//
// Found by ingesting this repository, where a reverted planning commit removed five
// increments and a later commit put them back.
func TestAnIncrementCanLeaveTheLedgerAndReturn(t *testing.T) {
	empty := "# Increment plan\n\n## Sequencing\n\nNothing planned.\n"
	dir := repo(t, []struct{ ledger, when, message string }{
		{ledger("planned"), "2026-08-01T09:00:00Z", "plan"},
		{ledger("in-review"), "2026-08-01T10:00:00Z", "hand over"},
		{empty, "2026-08-01T11:00:00Z", "revert the plan"},
		{empty + "\nstill nothing\n", "2026-08-01T11:30:00Z", "an unrelated edit"},
		{ledger("in-review"), "2026-08-02T09:00:00Z", "reinstate"},
		{ledger("done"), "2026-08-02T10:00:00Z", "ship"},
	})

	r, err := Repo(dir, now)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	got := r.Increments[0].Transitions
	want := []struct{ from, to string }{
		{"", "planned"},
		{"planned", "in-review"},
		{"in-review", Removed},
		{Removed, "in-review"},
		{"in-review", "done"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d transitions, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].From != w.from || got[i].To != w.to {
			t.Errorf("transition %d = %q->%q, want %q->%q", i, got[i].From, got[i].To, w.from, w.to)
		}
	}

	// The history must be continuous: every transition starts where the last ended.
	for i := 1; i < len(got); i++ {
		if got[i].From != got[i-1].To {
			t.Fatalf("history breaks at %d: %q follows %q", i, got[i].From, got[i-1].To)
		}
	}
}

// A field's value may be on the following indented lines rather than inline — Test
// Strategy and Acceptance Criteria are always written that way. Reading only the
// inline part reported every increment in this repository as having no test strategy.
func TestMultiLineFieldValuesAreCaptured(t *testing.T) {
	body := `# Plan

## feat-001: Something

- **Type:** feature
- **Status:** approved
- **Scope:** One line, inline.
- **Test Strategy:**
  - Unit: the parser
  - Integration: a real repository
- **Risk:** Low

## Sequencing
`
	dir := repo(t, []struct{ ledger, when, message string }{
		{body, "2026-08-01T09:00:00Z", "plan"},
	})
	r, err := Repo(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	got := r.Increments[0].Fields["test_strategy"]
	if got == "" {
		t.Fatal("a multi-line field was read as empty")
	}
	if !strings.Contains(got, "Unit: the parser") || !strings.Contains(got, "Integration") {
		t.Fatalf("test strategy = %q, want both lines", got)
	}
	if r.Increments[0].Fields["scope"] != "One line, inline." {
		t.Fatalf("an inline field was disturbed: %q", r.Increments[0].Fields["scope"])
	}
}
