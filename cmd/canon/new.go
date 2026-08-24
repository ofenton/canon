package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// newCmd creates an issue from wherever the developer is standing.
//
// This is the other half of the NOJIRA answer. Linking after the fact makes the
// record correctable; this makes it cheap to be correct in the first place. A ticket
// that costs one command and one argument is a ticket people will actually create,
// and the placeholder exists precisely because the alternative costs a browser, a
// project picker and twelve fields.
func newCmd(args []string) error {
	// The title comes first, so the common case reads like a sentence:
	//   canon new "Search is slow"
	// Flags may still come first for anyone who prefers it, in which case whatever
	// is left over is the title.
	var title string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		title, args = args[0], args[1:]
	}

	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	actorID := fs.String("actor", "", "who is creating the issue")
	issueType := fs.String("type", "", "issue type (default: the most granular in the hierarchy)")
	team := fs.String("team", "", "team that owns it")
	parent := fs.String("parent", "", "issue this one sits under")
	repo := fs.String("repo", ".", "path to the git repository")
	dbPath := fs.String("db", "canon.db", "path to the event log")
	schemaPath := fs.String("schema", "canon.yaml", "path to canon.yaml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if title == "" {
		title = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
	if title == "" {
		return fmt.Errorf("a title is required: canon new \"Search is slow\"")
	}
	if *actorID == "" {
		return fmt.Errorf("-actor is required")
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

	kind := *issueType
	if kind == "" {
		if kind, err = e.DefaultIssueType(); err != nil {
			return err
		}
	}
	id, err := e.NextIssueID()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if err := e.CreateAs(p, id, kind, map[string]string{"title": title}, *team, now); err != nil {
		return err
	}
	if *parent != "" {
		if err := e.ReparentAs(p, id, *parent, now); err != nil {
			return fmt.Errorf("created %s, but setting its parent failed: %w", id, err)
		}
	}

	// Where the work is happening, recorded as a link rather than as schema fields.
	// An org's canon.yaml is unlikely to define "branch" or "repository", and adding
	// them would be exactly the per-project accretion this product refuses.
	origin := whereWeAre(*repo)
	if origin.sha != "" {
		_, err := e.LinkCommit(p, id, enforce.Commit{
			SHA:        origin.sha,
			Message:    origin.subject,
			Repository: origin.remote,
			Branch:     origin.branch,
			Author:     origin.author,
			At:         origin.at,
		}, now)
		if err != nil {
			// The issue exists and that is the thing that had to succeed. Losing the
			// link is worth a warning, not a failure that leaves the developer
			// unsure whether to run the command again.
			fmt.Printf("%s created, but linking %s failed: %v\n", id, origin.sha[:7], err)
			origin.sha = ""
		}
	}

	fmt.Printf("%s  %s\n", id, title)
	if origin.branch != "" {
		fmt.Printf("  on %s", origin.branch)
		if origin.remote != "" {
			fmt.Printf(" in %s", origin.remote)
		}
		fmt.Println()
	}
	if origin.sha != "" {
		fmt.Printf("  linked %s  %s\n", origin.sha[:7], origin.subject)
	}
	// The trailer is the point of the whole command: the next commit carries it, and
	// nothing has to be typed twice or remembered.
	fmt.Printf("\nput this in your commit message:\n\n  Increment: %s\n", id)
	return nil
}

// origin is where the command was run: nothing here is required, and every field is
// empty when the command is run outside a git repository.
type origin struct {
	sha, subject, author, branch, remote string
	at                                   time.Time
}

// whereWeAre reads the current commit and branch, tolerating every way that can fail.
//
// Outside a repository, in a repository with no commits, or with git missing
// entirely, this returns zero values and the issue is still created. Refusing to
// file a ticket because the shell happened to be in the wrong directory would be a
// silly reason to send someone back to a browser.
func whereWeAre(repo string) origin {
	var o origin
	o.branch = currentBranch(repo)
	o.remote = remoteURL(repo)

	commits, err := readCommits(repo, "HEAD^!")
	if err != nil || len(commits) == 0 {
		return o
	}
	c := commits[0]
	o.sha, o.subject, o.author, o.at = c.SHA, c.Subject, c.Author, c.At
	return o
}
