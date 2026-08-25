// Package ingest reads a repository that follows the agentic SDLC template and
// derives everything Canon shows from it.
//
// Nothing here writes. A repository is the source of truth for its own work, and this
// package's whole job is to be a faithful reader of it — including being honest about
// what it could not read.
package ingest

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// LedgerPath and SpecPath are where the template puts them. Fixed, not configurable:
// a convention with a configuration option is not a convention.
const (
	LedgerPath = "specs/increment-plan.md"
	SpecPath   = "specs/product.md"
)

// commit is one commit that touched a file.
type commit struct {
	SHA string
	At  time.Time
}

// touching returns every commit that changed a path, oldest first.
func touching(repo, path string) ([]commit, error) {
	out, err := git(repo, "log", "--format=%H%x1f%aI", "--reverse", "--", path)
	if err != nil {
		return nil, err
	}

	var commits []commit
	for _, line := range strings.Split(out, "\n") {
		sha, when, ok := strings.Cut(strings.TrimSpace(line), "\x1f")
		if !ok || sha == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, when)
		if err != nil {
			return nil, fmt.Errorf("commit %s has an unreadable author date %q: %w", sha, when, err)
		}
		commits = append(commits, commit{SHA: sha, At: at.UTC()})
	}
	return commits, nil
}

// fileAt returns a file's full content at one commit.
//
// Reading whole files rather than parsing diffs is the decision this package turns
// on. A diff shows a changed line without reliably showing which increment it belongs
// to — the heading is often outside the hunk's context — so a diff parser guesses,
// and guesses wrongly in exactly the case that matters: a long increment whose status
// line is far from its heading. Reading the file at each commit and comparing parsed
// states is unambiguous. It costs one parse per commit, which is nothing.
func fileAt(repo, sha, path string) (string, bool, error) {
	out, err := git(repo, "show", sha+":"+path)
	if err != nil {
		// A commit before the file existed, or one that deleted it. Not an error:
		// the history of a file legitimately begins somewhere.
		return "", false, nil
	}
	return out, true, nil
}

// head returns the current commit, or empty for a repository with no commits.
func head(repo string) string {
	out, err := git(repo, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// remote returns the origin URL, or empty if there is none.
func remote(repo string) string {
	out, err := git(repo, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// git runs one command in a repository.
//
// Shelling out rather than using a git library: everything needed here is three
// commands, and a library would be a dependency and a second implementation of
// git's own semantics to be subtly wrong about.
func git(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}
