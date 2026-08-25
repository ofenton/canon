package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The binary must at minimum report a version, so that the build acceptance
// criteria have something to execute.
func TestVersionDefault(t *testing.T) {
	if version == "" {
		t.Fatal("version must not be empty")
	}
}

// Canon holds no state and reads no configuration, and this is where that is checked.
//
// Both were true in prose before they were true in the repository: the README said "no
// database, and nothing to configure" while deploy/canon.yaml sat tracked and .gitignore
// reserved *.db for a writer that no longer existed. A claim in a README is a claim; this
// is the version that fails.
//
// Reads git rather than walking the filesystem, so a local scratch file cannot fail it
// and a committed one cannot escape it.
func TestTheRepositoryHoldsNoStateOrConfiguration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	// From the root, not the package directory: go test runs in cmd/canon, where
	// ls-files reports six files and every rule below passes by seeing nothing.
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skip("not a git checkout")
	}
	out, err := exec.Command("git", "-C", strings.TrimSpace(string(root)), "ls-files").Output()
	if err != nil {
		t.Skip("not a git checkout")
	}

	files := strings.Fields(string(out))
	// A count nobody has to maintain, guarding the failure this test already had: a
	// listing that is empty or scoped to a subdirectory checks nothing and says so.
	if len(files) < 50 {
		t.Fatalf("only %d tracked files; git ls-files did not run from the repository root", len(files))
	}
	for _, f := range files {
		switch {
		case strings.HasSuffix(f, ".db"), strings.HasSuffix(f, ".sqlite"),
			strings.HasSuffix(f, ".db-wal"), strings.HasSuffix(f, ".db-shm"):
			t.Errorf("%s is a database; Canon derives everything from git and stores nothing", f)
		case (strings.HasSuffix(f, ".yaml") || strings.HasSuffix(f, ".yml")) &&
			!strings.HasPrefix(f, ".github/"):
			// CI workflows are the one legitimate YAML here. Anything else is
			// configuration, and the template fixes the schema so there is none.
			t.Errorf("%s looks like configuration; the schema is fixed by the template, not chosen", f)
		}
	}
}
