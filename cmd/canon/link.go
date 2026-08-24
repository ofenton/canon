package main

import (
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// trailerRe finds an issue reference in a commit message trailer, which is the
// convention the SDLC template already uses: "Increment: feat-001".
//
// It also matches a bare id anywhere in the subject, because plenty of teams write
// "CANON-12: fix the thing" and refusing to understand that would mean this command
// only works for people who already had the discipline it exists to help with.
var (
	trailerRe = regexp.MustCompile(`(?mi)^\s*(?:increment|issue|canon|refs?)\s*:\s*([a-z]{2,10}-\d+)\s*$`)
	inlineRe  = regexp.MustCompile(`\b([A-Z][A-Z0-9]{1,9}-\d+)\b`)
)

// linkCmd links commits to issues, either one at a time or by sweeping a range.
func linkCmd(args []string) error {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	actorID := fs.String("actor", "", "who is recording the link")
	issue := fs.String("issue", "", "issue to link to (default: read from each commit message)")
	commitRange := fs.String("range", "", "commit range to sweep, e.g. main..HEAD")
	sha := fs.String("commit", "", "a single commit to link (default HEAD when no range is given)")
	repo := fs.String("repo", ".", "path to the git repository")
	dbPath := fs.String("db", "canon.db", "path to the event log")
	schemaPath := fs.String("schema", "canon.yaml", "path to canon.yaml")
	dryRun := fs.Bool("dry-run", false, "print what would be linked and write nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *actorID == "" {
		return fmt.Errorf("-actor is required")
	}

	spec := *commitRange
	if spec == "" {
		if *sha != "" {
			spec = *sha + "^!"
		} else {
			spec = "HEAD^!"
		}
	}
	commits, err := readCommits(*repo, spec)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		fmt.Printf("no commits in %s\n", spec)
		return nil
	}

	origin := remoteURL(*repo)
	branch := currentBranch(*repo)

	// Ask which ids exist before deciding what each reference names. A dry run reads
	// this too, so a preview says the same thing the real run will do.
	known, err := knownIssues(*dbPath, *schemaPath)
	if err != nil {
		return err
	}

	// Resolve every commit's target before writing anything, so a range with one
	// unreadable reference reports it rather than half-applying.
	type target struct {
		commit gitCommit
		issue  string
	}
	var targets []target
	var unmatched []gitCommit
	for _, c := range commits {
		id := *issue
		if id == "" {
			id = issueFrom(c.Subject + "\n" + c.Body)
		}
		if id == "" {
			unmatched = append(unmatched, c)
			continue
		}
		targets = append(targets, target{commit: c, issue: resolveRef(id, known)})
	}

	for _, c := range unmatched {
		fmt.Printf("  no issue  %s  %s\n", c.SHA[:7], c.Subject)
	}
	if len(targets) == 0 {
		fmt.Printf("\n%d commit(s) carry no issue reference. Link them with -issue <id>.\n", len(unmatched))
		return nil
	}

	if *dryRun {
		for _, t := range targets {
			fmt.Printf("  would link %s  %s  → %s\n", t.commit.SHA[:7], t.commit.Subject, t.issue)
		}
		return nil
	}

	sch, err := schema.Load(*schemaPath)
	if err != nil {
		return err
	}
	store, err := event.Open(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	e := enforce.New(sch, store)
	p, err := e.Principal(*actorID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	var linked, already int
	for _, t := range targets {
		wrote, err := e.LinkCommit(p, t.issue, enforce.Commit{
			SHA:        t.commit.SHA,
			Message:    t.commit.Subject,
			Repository: origin,
			Branch:     branch,
			Author:     t.commit.Author,
			At:         t.commit.At,
		}, now)
		if err != nil {
			return fmt.Errorf("linking %s to %s: %w", t.commit.SHA[:7], t.issue, err)
		}
		mark := "  "
		if !wrote {
			// Say so rather than counting it again. A sweep re-run over the same
			// range should report nothing new, not repeat its first run's total.
			mark, already = "= ", already+1
		}
		fmt.Printf("  %s%s  %s  → %s\n", mark, t.commit.SHA[:7], t.commit.Subject, t.issue)
		if wrote {
			linked++
		}
	}
	fmt.Printf("\nlinked %d commit(s)", linked)
	if already > 0 {
		fmt.Printf("; %d already linked", already)
	}
	if len(unmatched) > 0 {
		fmt.Printf("; %d carry no issue reference", len(unmatched))
	}
	fmt.Println()
	return nil
}

// gitCommit is one commit as git reports it.
type gitCommit struct {
	SHA     string
	Subject string
	Body    string
	Author  string
	At      time.Time
}

// readCommits reads a range using a record separator git will never emit inside a
// field, so a commit message containing anything at all still parses.
func readCommits(repo, spec string, extra ...string) ([]gitCommit, error) {
	const (
		fieldSep  = "\x1f"
		recordSep = "\x1e"
	)
	format := strings.Join([]string{"%H", "%s", "%b", "%an", "%aI"}, fieldSep) + recordSep

	args := append([]string{"log", "--format=" + format}, extra...)
	out, err := git(repo, append(args, spec)...)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", spec, err)
	}

	var commits []gitCommit
	for _, record := range strings.Split(out, recordSep) {
		record = strings.TrimLeft(record, "\n")
		if strings.TrimSpace(record) == "" {
			continue
		}
		parts := strings.Split(record, fieldSep)
		if len(parts) < 5 {
			continue
		}
		at, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[4]))
		if err != nil {
			return nil, fmt.Errorf("commit %s has an unreadable author date %q: %w", parts[0], parts[4], err)
		}
		commits = append(commits, gitCommit{
			SHA:     strings.TrimSpace(parts[0]),
			Subject: parts[1],
			Body:    parts[2],
			Author:  parts[3],
			At:      at.UTC(),
		})
	}
	return commits, nil
}

// issueFrom finds the issue a commit message refers to, preferring an explicit
// trailer over an id mentioned in passing.
func issueFrom(message string) string {
	if m := trailerRe.FindStringSubmatch(message); m != nil {
		return m[1]
	}
	if m := inlineRe.FindStringSubmatch(message); m != nil {
		return m[1]
	}
	return ""
}

// resolveRef maps a reference as written onto the id Canon holds, ignoring case.
//
// Canon's own ledger uses lower-case ids (feat-026) and plenty of teams use upper
// (CANON-12). Guessing either way meant `canon trace` called a commit tracked while
// `canon link` refused the same commit as unknown — the two commands disagreeing
// about the same string. Neither guesses now; both ask.
func resolveRef(ref string, known map[string]string) string {
	if known == nil {
		return ref
	}
	if actual, ok := known[strings.ToUpper(ref)]; ok {
		return actual
	}
	return ref
}

func remoteURL(repo string) string {
	out, err := git(repo, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func currentBranch(repo string) string {
	out, err := git(repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// git runs one git command in a repository. Everything about this feature depends on
// reading real commit metadata, so shelling out to git is the honest implementation:
// reimplementing object parsing would be a much larger surface to get subtly wrong.
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
