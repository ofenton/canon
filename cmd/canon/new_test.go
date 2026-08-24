package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AC: WHEN a developer runs one command with only a title THE SYSTEM SHALL create an
// issue and print its id.
func TestNewNeedsOnlyATitle(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{
		{"Reindex on write", "2026-03-02T10:00:00Z"},
	})
	setUpCanon(t, dir, "ollie")

	out, err := canonIn(t, dir, "new", "Search is slow", "-actor", "ollie")
	if err != nil {
		t.Fatalf("new: %v\n%s", err, out)
	}
	if !strings.Contains(out, "CANON-1") {
		t.Fatalf("expected the new id in the output, got:\n%s", out)
	}
	if !strings.Contains(out, "Search is slow") {
		t.Fatalf("expected the title in the output, got:\n%s", out)
	}
	// The trailer is the point: it must be printed ready to paste.
	if !strings.Contains(out, "Increment: CANON-1") {
		t.Fatalf("expected a commit trailer to paste, got:\n%s", out)
	}
}

// AC: THE SYSTEM SHALL record the branch, repository and commit the command was run
// in.
func TestNewRecordsWhereItWasRun(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{
		{"Reindex on write", "2026-03-02T10:00:00Z"},
	})
	setUpCanon(t, dir, "ollie")

	out, err := canonIn(t, dir, "new", "Search is slow", "-actor", "ollie")
	if err != nil {
		t.Fatalf("new: %v\n%s", err, out)
	}
	if !strings.Contains(out, "on main") {
		t.Fatalf("expected the branch in the output, got:\n%s", out)
	}
	if !strings.Contains(out, "acme/widgets") {
		t.Fatalf("expected the remote in the output, got:\n%s", out)
	}

	events, err := canonIn(t, dir, "events", "-subject", "CANON-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(events, "issue.commit_linked") {
		t.Fatalf("expected the current commit to be linked, got:\n%s", events)
	}
	for _, want := range []string{"main", "acme/widgets", "Reindex on write"} {
		if !strings.Contains(events, want) {
			t.Fatalf("expected %q recorded in the link, got:\n%s", want, events)
		}
	}
	// The commit predates the issue by five months, which is the case that made the
	// before-creation rule wrong for links.
	if !strings.Contains(events, "2026-03-02T10:00:00Z") {
		t.Fatalf("expected the commit's own author time, got:\n%s", events)
	}
}

// AC: WHEN the command is run outside a git repository THE SYSTEM SHALL still create
// the issue.
func TestNewWorksOutsideAGitRepository(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile(filepath.Join("..", "..", "internal", "schema", "testdata", "canon.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "canon.yaml"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := canonIn(t, dir, "bootstrap", "-actor", "ollie", "-team", "platform"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	out, err := canonIn(t, dir, "new", "Search is slow", "-actor", "ollie")
	if err != nil {
		t.Fatalf("new outside a repository must still work: %v\n%s", err, out)
	}
	if !strings.Contains(out, "CANON-1") {
		t.Fatalf("expected the issue to be created, got:\n%s", out)
	}
	if strings.Contains(out, "linked") {
		t.Fatalf("there is no commit to link outside a repository, got:\n%s", out)
	}
}

// A missing title is the one thing that cannot be defaulted, and the error has to
// show the shape of the command rather than just naming a flag.
func TestNewWithoutATitleSaysHowToCallIt(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{{"init", "2026-03-02T10:00:00Z"}})
	setUpCanon(t, dir, "ollie")

	_, err := canonIn(t, dir, "new", "-actor", "ollie")
	if err == nil {
		t.Fatal("expected a missing title to be refused")
	}
	if !strings.Contains(err.Error(), "canon new") {
		t.Fatalf("the error should show the command, got: %v", err)
	}
}

// Ids must not collide with the ones the API hands out, which is why both ask the
// same place.
func TestNewIssuesGetSequentialIDs(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{{"init", "2026-03-02T10:00:00Z"}})
	setUpCanon(t, dir, "ollie")

	for _, want := range []string{"CANON-1", "CANON-2", "CANON-3"} {
		out, err := canonIn(t, dir, "new", "a thing", "-actor", "ollie")
		if err != nil {
			t.Fatalf("new: %v\n%s", err, out)
		}
		if !strings.Contains(out, want) {
			t.Fatalf("expected %s, got:\n%s", want, out)
		}
	}
}

// Flags before the title must work too, or the command is only usable one way.
func TestNewAcceptsFlagsBeforeTheTitle(t *testing.T) {
	dir := tempRepo(t, []struct{ message, when string }{{"init", "2026-03-02T10:00:00Z"}})
	setUpCanon(t, dir, "ollie")

	out, err := canonIn(t, dir, "new", "-actor", "ollie", "Search is slow")
	if err != nil {
		t.Fatalf("new: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Search is slow") {
		t.Fatalf("expected the trailing words to be read as the title, got:\n%s", out)
	}
}
