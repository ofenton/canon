package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// tempRepo builds a real git repository with commits carrying real author dates,
// because the whole point of this feature is reading metadata git actually produces.
func tempRepo(t *testing.T, commits []struct{ message, when string }) string {
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
	run(nil, "config", "user.email", "test@example.com")
	run(nil, "config", "user.name", "Test Person")
	run(nil, "remote", "add", "origin", "https://example.com/acme/widgets.git")

	for i, c := range commits {
		name := filepath.Join(dir, "file")
		if err := os.WriteFile(name, []byte(strings.Repeat("x", i+1)), 0o644); err != nil {
			t.Fatal(err)
		}
		run(nil, "add", "file")
		run([]string{"GIT_AUTHOR_DATE=" + c.when, "GIT_COMMITTER_DATE=" + c.when},
			"commit", "-q", "-m", c.message)
	}
	return dir
}

// canonIn runs the CLI in dir, returning combined output.
func canonIn(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	out := captureStdout(t, func() error { return run(args) })
	return out.text, out.err
}

type captured struct {
	text string
	err  error
}

func captureStdout(t *testing.T, fn func() error) captured {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w

	errCh := make(chan error, 1)
	go func() { errCh <- fn() }()
	runErr := <-errCh

	w.Close()
	os.Stdout = old
	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	r.Close()
	return captured{text: string(buf[:n]), err: runErr}
}

// setUpCanon puts a schema, a log and an actor in dir so the CLI can write.
func setUpCanon(t *testing.T, dir, actor string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "internal", "schema", "testdata", "canon.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "canon.yaml"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := canonIn(t, dir, "bootstrap", "-actor", actor, "-team", "platform"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
}

// AC: WHEN a commit is supplied after the fact THE SYSTEM SHALL link it to an
// existing issue and record the link with its original commit timestamp.
func TestLinkSweepsARangeAndKeepsCommitTimestamps(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{
		{"Reindex on write\n\nIncrement: CANON-1", "2026-03-02T10:00:00Z"},
		{"Tidy the logs", "2026-03-03T11:00:00Z"}, // no reference at all
		{"CANON-1: cache the query plan", "2026-03-04T12:00:00Z"},
	})
	setUpCanon(t, dir, "ollie")

	// The issue predates the commits, or linking would be dated before it existed.
	if _, err := canonIn(t, dir, "events"); err != nil {
		t.Fatalf("events: %v", err)
	}
	createIssueAt(t, dir, "CANON-1", "2026-03-01T09:00:00Z")

	out, err := canonIn(t, dir, "link", "-actor", "ollie", "-range", "main")
	if err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}

	if !strings.Contains(out, "linked 2 commit(s)") {
		t.Fatalf("expected two commits linked, got:\n%s", out)
	}
	if !strings.Contains(out, "1 carry no issue reference") {
		t.Fatalf("expected the unreferenced commit to be reported, got:\n%s", out)
	}
	if !strings.Contains(out, "no issue") || !strings.Contains(out, "Tidy the logs") {
		t.Fatalf("the unreferenced commit should be named, got:\n%s", out)
	}

	// The recorded events must carry the commits' own author dates.
	events, err := canonIn(t, dir, "events", "-subject", "CANON-1")
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	for _, want := range []string{"2026-03-02T10:00:00Z", "2026-03-04T12:00:00Z"} {
		if !strings.Contains(events, want) {
			t.Fatalf("expected a link dated %s in:\n%s", want, events)
		}
	}
	if !strings.Contains(events, "acme/widgets") {
		t.Fatalf("expected the repository to be recorded, got:\n%s", events)
	}
}

// AC: WHEN the same commit is linked twice THE SYSTEM SHALL record it once.
func TestLinkIsSafeToRepeat(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{
		{"Reindex on write\n\nIncrement: CANON-1", "2026-03-02T10:00:00Z"},
	})
	setUpCanon(t, dir, "ollie")
	createIssueAt(t, dir, "CANON-1", "2026-03-01T09:00:00Z")

	for range 3 {
		if _, err := canonIn(t, dir, "link", "-actor", "ollie", "-range", "main"); err != nil {
			t.Fatalf("link: %v", err)
		}
	}

	events, err := canonIn(t, dir, "events", "-subject", "CANON-1")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(events, "issue.commit_linked"); n != 1 {
		t.Fatalf("got %d link events, want 1 — repeating a sweep must not duplicate", n)
	}
}

// -dry-run has to write nothing, or nobody will trust it on a large range.
func TestLinkDryRunWritesNothing(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{
		{"Reindex on write\n\nIncrement: CANON-1", "2026-03-02T10:00:00Z"},
	})
	setUpCanon(t, dir, "ollie")
	createIssueAt(t, dir, "CANON-1", "2026-03-01T09:00:00Z")

	out, err := canonIn(t, dir, "link", "-actor", "ollie", "-range", "main", "-dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(out, "would link") {
		t.Fatalf("expected a preview, got:\n%s", out)
	}

	events, err := canonIn(t, dir, "events", "-subject", "CANON-1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(events, "issue.commit_linked") {
		t.Fatal("a dry run must not write a link event")
	}
}

func TestIssueFromReadsTrailersAndInlineIDs(t *testing.T) {
	cases := []struct{ message, want string }{
		{"Reindex on write\n\nIncrement: feat-001", "FEAT-001"},
		{"Reindex on write\n\nissue: CANON-42", "CANON-42"},
		{"CANON-12: fix the thing", "CANON-12"},
		{"Fix the thing (CANON-9)", "CANON-9"},
		{"Tidy the logs", ""},
		{"NOJIRA: quick fix", ""},
		{"Bump to v1.2-3", ""},
		// A trailer wins over a passing mention.
		{"Mentions CANON-99 in passing\n\nIncrement: feat-002", "FEAT-002"},
	}
	for _, c := range cases {
		if got := issueFrom(c.message); got != c.want {
			t.Errorf("issueFrom(%q) = %q, want %q", c.message, got, c.want)
		}
	}
}

// createIssueAt writes a backdated issue straight through the domain, which is what
// the CLI has no command for yet — feat-024 adds one.
func createIssueAt(t *testing.T, dir, id, when string) {
	t.Helper()
	at, err := time.Parse(time.RFC3339, when)
	if err != nil {
		t.Fatal(err)
	}
	sch, err := schema.Load(filepath.Join(dir, "canon.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := event.Open(filepath.Join(dir, "canon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	e := enforce.New(sch, store)
	p, err := e.Principal("ollie")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.CreateAs(p, id, "story", map[string]string{"title": "Search is slow"}, "platform", at); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
}

// gitIn runs one git command in a test repository.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE=2026-03-04T10:00:00Z", "GIT_COMMITTER_DATE=2026-03-04T10:00:00Z",
		"GIT_AUTHOR_NAME=Test Person", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test Person", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// writeAndCommit adds one file and commits it with a given author date.
func writeAndCommit(t *testing.T, dir, name, message, when string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", name)
	cmd := exec.Command("git", "-C", dir, "commit", "-q", "-m", message)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+when, "GIT_COMMITTER_DATE="+when,
		"GIT_AUTHOR_NAME=Test Person", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test Person", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit %s: %v\n%s", name, err, out)
	}
}
