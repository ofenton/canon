package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repo makes a directory that is a product: the artifact, and a git repository.
func repo(t *testing.T, dir string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "specs", "increment-plan.md"), []byte("# Increment plan\n"), 0o644)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

const list = `
# Where Canon looks. One line per source.
~/code                            # a directory of checkouts

git@github.com:ofenton/orders     # one repository, fetched
https://github.com/ofenton/api.git
github:ofenton                    # an organisation
`

// AC: THE SYSTEM SHALL parse the list without a schema.
func TestASourceListIsLinesAndComments(t *testing.T) {
	got, err := Parse(strings.NewReader(list))
	if err != nil {
		t.Fatal(err)
	}
	want := []Source{
		{"~/code", Directory},
		{"git@github.com:ofenton/orders", Remote},
		{"https://github.com/ofenton/api.git", Remote},
		{"github:ofenton", Organisation},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sources, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("source %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// AC: THE SYSTEM SHALL parse the list without a schema, so it cannot become
// configuration.
//
// The line ADR-0010 draws: this file says *what* Canon reads, never *how* it behaves,
// and the operational form of that is that a nested key must never parse. A `key:`
// followed by an indented list is the shape that would arrive first, so it is the shape
// asserted against — it must come out as two opaque lines, not a structure.
func TestTheListHasNoSchema(t *testing.T) {
	got, err := Parse(strings.NewReader("sources:\n  - ~/code\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sources, want 2 opaque lines: %v", len(got), got)
	}
	if got[0].Line != "sources:" || got[1].Line != "- ~/code" {
		t.Errorf("lines were interpreted rather than taken whole: %v", got)
	}

	// And nothing here can grow into a decoder without this failing.
	src, err := os.ReadFile("source.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"encoding/json", "yaml", "toml", "encoding/xml"} {
		if strings.Contains(string(src), banned) {
			t.Errorf("source.go references %q; the list is an argument, not configuration", banned)
		}
	}
}

// AC: WHEN given a list of sources THE SYSTEM SHALL ingest every repository each one
// names.
func TestADirectoryAndARepositoryBothResolve(t *testing.T) {
	root := t.TempDir()
	repo(t, filepath.Join(root, "orders"))
	repo(t, filepath.Join(root, "api"))
	alone := repo(t, filepath.Join(t.TempDir(), "standalone"))

	got := Resolve([]Source{{root, Directory}, {alone, Directory}}, t.TempDir())
	if len(got) != 2 {
		t.Fatalf("got %d results", len(got))
	}
	if len(got[0].Paths) != 2 || got[0].Err != nil {
		t.Errorf("a directory should yield both repositories: %d, %v", len(got[0].Paths), got[0].Err)
	}
	// A line does not say which kind it is, and cannot say so wrongly: the kind is
	// settled by looking, and reported back so a listing can explain itself.
	if got[1].Source.Kind != Repository {
		t.Errorf("a directory that is a repository resolved as %q", got[1].Source.Kind)
	}
	if len(got[1].Paths) != 1 {
		t.Errorf("a repository should yield itself: %v", got[1].Paths)
	}
}

// AC: WHEN a source cannot be read THE SYSTEM SHALL report which one and ingest the rest.
func TestOneBadSourceDoesNotHideTheGoodOnes(t *testing.T) {
	root := t.TempDir()
	repo(t, filepath.Join(root, "orders"))
	empty := t.TempDir()

	got := Resolve([]Source{
		{"/no/such/place", Directory},
		{root, Directory},
		{empty, Directory},
		{"file:///no/such/repository", Remote},
	}, t.TempDir())
	if len(got) != 4 {
		t.Fatalf("got %d results, want one per source — a source that vanishes cannot be reported", len(got))
	}
	if got[0].Err == nil || !strings.Contains(got[0].Err.Error(), "/no/such/place") {
		t.Errorf("a missing source must name itself: %v", got[0].Err)
	}
	if len(got[1].Paths) != 1 || got[1].Err != nil {
		t.Errorf("the good source was affected by the bad one: %v %v", got[1].Paths, got[1].Err)
	}
	// A directory with no products is not a failure of Canon's, but silence would read
	// as "working".
	if got[2].Err == nil {
		t.Error("a directory holding no product should say so")
	}
	// An unreachable remote reports without taking the others down with it.
	if got[3].Err == nil {
		t.Errorf("%q should have reported a problem", got[3].Source.Line)
	}
}

func TestPathsFlattensWhatWasFound(t *testing.T) {
	root := t.TempDir()
	repo(t, filepath.Join(root, "a"))
	repo(t, filepath.Join(root, "b"))
	if n := len(Paths(Resolve([]Source{{root, Directory}, {"/nope", Directory}}, t.TempDir()))); n != 2 {
		t.Fatalf("got %d paths, want 2", n)
	}
}

// A tilde is what anybody writes in a list of checkouts, and expanding it is the
// difference between a list that works and one that silently finds nothing.
//
// Proved by planting a repository under a home directory rather than by reading the
// error text: the first version of this test asserted that the message mentioned the
// expanded path, which it deliberately does not — a report quotes back what was
// written. That test passed for the wrong reason and could not have caught the bug.
func TestATildeIsExpanded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := os.UserHomeDir(); err != nil {
		t.Skip("home directory cannot be overridden here")
	}
	repo(t, filepath.Join(home, "orders"))

	got := Resolve([]Source{{"~", Directory}}, t.TempDir())
	if got[0].Err != nil {
		t.Fatalf("~ did not resolve: %v", got[0].Err)
	}
	if len(got[0].Paths) != 1 || !strings.Contains(got[0].Paths[0], "orders") {
		t.Fatalf("~ resolved to %v, want the repository under it", got[0].Paths)
	}
}
