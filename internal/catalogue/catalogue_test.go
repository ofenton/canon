package catalogue

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/source"
)

var fixed = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

func now() time.Time { return fixed }

const ledger = `# Increment plan

## feat-001: Something

- **Type:** feature
- **Status:** done
- **Scope:** A thing.
- **Test Strategy:**
  - Unit
- **Rollback Plan:** Revert
- **Risk:** Low
- **Evidence:** shipped

## Sequencing
`

const spec = `# Widgets

## Problem

Widgets are hard to find.
`

// product builds a repository that follows the template.
func product(t *testing.T, root, name string, commit bool) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "specs", "increment-plan.md"), []byte(ledger), 0o644)
	os.WriteFile(filepath.Join(dir, "specs", "product.md"), []byte(spec), 0o644)

	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_DATE=2026-08-01T09:00:00Z", "GIT_COMMITTER_DATE=2026-08-01T09:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@e.com")
	run("config", "user.name", "T")
	if commit {
		run("add", ".")
		run("commit", "-q", "-m", "plan feat-001")
	}
	return dir
}

// AC: WHEN given an organisation THE SYSTEM SHALL discover repositories containing a
// ledger and list them as products.
func TestDiscoverFindsProductsByArtifact(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	product(t, root, "widgets", true)
	product(t, root, "gadgets", true)
	// Not a product: no ledger.
	os.MkdirAll(filepath.Join(root, "notes", ".git"), 0o755)

	found, err := source.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("found %v, want the two repositories with a ledger", found)
	}
	for _, f := range found {
		if strings.Contains(f, "notes") {
			t.Fatal("a directory with no ledger is not a product")
		}
	}
}

// AC: THE SYSTEM SHALL show each product's purpose from its own spec.
func TestCatalogueCarriesPurposeAndCounts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	product(t, root, "widgets", true)

	sources, _ := source.Discover(root)
	c := New()
	c.Refresh(sources, now)

	entries := c.Entries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Name() != "Widgets" {
		t.Fatalf("name = %q, want the spec's title", e.Name())
	}
	if !strings.Contains(e.Repository.Purpose, "hard to find") {
		t.Fatalf("purpose = %q, want it from the spec", e.Repository.Purpose)
	}
	if len(e.Repository.Increments) != 1 {
		t.Fatalf("got %d increments", len(e.Repository.Increments))
	}
}

// AC: THE SYSTEM SHALL state when each repository was last ingested.
func TestRefreshTimeIsRecorded(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	product(t, root, "widgets", true)
	sources, _ := source.Discover(root)

	c := New()
	if !c.RefreshedAt().IsZero() {
		t.Fatal("a catalogue that has never refreshed must say so")
	}
	c.Refresh(sources, now)
	if c.RefreshedAt() != fixed {
		t.Fatalf("refreshed at %s, want the supplied clock", c.RefreshedAt())
	}
	if c.Entries()[0].RefreshedAt != fixed {
		t.Fatal("each entry carries its own refresh time")
	}
}

// A source that cannot be read is kept and reported, not dropped. A repository
// mid-adoption — the template's files committed to nothing yet — is a real state, and
// a product that silently vanishes from a catalogue is worse than one with an error.
//
// Found in the wild: a checkout beside this one had the files and no commits.
func TestAnUnreadableSourceIsReportedNotDropped(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	product(t, root, "widgets", true)
	product(t, root, "halfway", false) // files, no commits

	sources, _ := source.Discover(root)
	if len(sources) != 2 {
		t.Fatalf("both should be discovered, got %v", sources)
	}

	c := New()
	c.Refresh(sources, now)
	entries := c.Entries()
	if len(entries) != 2 {
		t.Fatalf("got %d entries; a failing source must still appear", len(entries))
	}

	var failed *Entry
	for _, e := range entries {
		if e.Err != "" {
			failed = e
		}
	}
	if failed == nil {
		t.Fatal("the uncommitted repository should carry an error")
	}
	if !strings.Contains(failed.Err, "no commits") {
		t.Fatalf("the error should say why: %q", failed.Err)
	}
	if failed.Name() == "" {
		t.Fatal("a failing product still needs a name")
	}
}

// Refreshing replaces rather than accumulates, or a removed product lingers for ever.
func TestRefreshReplaces(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	a := product(t, root, "widgets", true)
	product(t, root, "gadgets", true)

	c := New()
	sources, _ := source.Discover(root)
	c.Refresh(sources, now)
	if len(c.Entries()) != 2 {
		t.Fatalf("expected two products")
	}

	c.Refresh([]string{a}, now)
	if len(c.Entries()) != 1 {
		t.Fatalf("refresh should replace, got %d entries", len(c.Entries()))
	}
}

// Reads must not touch the filesystem, which is what makes the answer fast and what
// "as of the last refresh" means.
func TestReadsDoNotTouchTheSource(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	dir := product(t, root, "widgets", true)

	c := New()
	c.Refresh([]string{dir}, now)

	// Remove the repository entirely. The catalogue still answers.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	entries := c.Entries()
	if len(entries) != 1 || entries[0].Repository == nil {
		t.Fatal("a read went to disk; it must answer from what was ingested")
	}
	if _, ok := c.Entry("Widgets"); !ok {
		t.Fatal("lookup by name should also answer from memory")
	}
}

// AC: WHEN a source cannot be read THE SYSTEM SHALL report which one and ingest the rest.
//
// At the catalogue rather than in source, because the requirement is about what a person
// sees. A source that failed has to survive as far as the API, and the way it does that
// is by being an entry — the same shape an unreadable repository already takes.
func TestAFailedSourceAppearsRatherThanVanishing(t *testing.T) {
	root := t.TempDir()
	product(t, root, "orders", true)

	c := New()
	c.RefreshFrom(source.Resolve([]source.Source{
		{Line: "/no/such/place", Kind: source.Directory},
		{Line: root, Kind: source.Directory},
	}), now)

	entries := c.Entries()
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want the good source and the bad one", len(entries))
	}
	var failed, read int
	for _, e := range entries {
		if e.Err != "" {
			failed++
			if !strings.Contains(e.Err, "/no/such/place") {
				t.Errorf("the failure does not name its source: %s", e.Err)
			}
			continue
		}
		read++
	}
	if failed != 1 || read != 1 {
		t.Fatalf("%d failed and %d read; one bad source must not empty the catalogue", failed, read)
	}
}
