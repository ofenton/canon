package enforce

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/projection"
)

// Commit links.
//
// The `NOJIRA` problem is not that developers are careless. It is that policy demands
// a ticket for work that does not warrant one, creating a ticket is expensive, and
// linking after the fact is impossible — so a placeholder is the only move left. The
// answer here is the third of those: a commit can be linked to an issue at any time,
// carrying the timestamp it actually had, so the record can be made true later
// instead of having to be true up front.
//
// The link is an event rather than a field, because who linked what and when is
// exactly the thing an audit asks about, and a field would keep only the answer.

// LinkOp is the operation of linking a commit to an issue.
//
// It is deliberately cheap to grant. Linking records that work happened; it changes
// no state, gates nothing, and refusing it would recreate the problem this feature
// exists to remove.
const LinkOp = "link"

// shaRe matches an abbreviated or full git object name. Anything shorter than seven
// characters is ambiguous in a repository of any size.
var shaRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// Commit is one commit being linked. Only SHA is required: a link with nothing but a
// hash is still a true and useful record, and demanding more would be the same
// mistake as demanding twelve fields to create an issue.
type Commit struct {
	SHA        string
	Message    string
	Repository string
	Branch     string
	Author     string
	// At is the commit's author time. Zero means "use the current instant", which is
	// right for a commit being linked as it is made.
	At time.Time
}

// LinkCommit records a commit against an issue, reporting whether it wrote anything.
//
// Linking the same commit to the same issue twice is a no-op rather than an error.
// The natural caller is a sweep over a range of commits, which will re-see commits
// it has already linked every time it runs; making that an error would mean every
// caller has to track what it has already done, and the second-best outcome — a log
// with duplicate links in it — is worse than doing nothing.
func (e *Enforcer) LinkCommit(p Principal, id string, c Commit, now time.Time) (linked bool, err error) {
	if err := e.refresh(); err != nil {
		return false, err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return false, fmt.Errorf("unknown issue %s", id)
	}

	c.SHA = strings.ToLower(strings.TrimSpace(c.SHA))
	if !shaRe.MatchString(c.SHA) {
		return false, fmt.Errorf("%q is not a commit id; expected 7 to 40 hex characters", c.SHA)
	}
	for _, existing := range issue.Commits {
		if sameCommit(existing.SHA, c.SHA) {
			return false, nil
		}
	}

	if err := e.authorise(p, LinkOp, id, issue.Team); err != nil {
		return false, err
	}

	at := c.At
	if at.IsZero() {
		at = now
	} else if at.Before(now) {
		// A commit's author time is nearly always in the past, so linking needs the
		// backdate grant; without it, `canon link` over an old range would be a way
		// around the permission. It does *not* get the before-creation check: a
		// commit routinely predates the issue that tracks it, which is exactly the
		// case this feature exists to record.
		if err := e.AuthoriseBackdate(p, id, at, now); err != nil {
			return false, err
		}
	}

	err = e.append("issue.commit_linked", id, at, p.Actor, map[string]any{
		"sha":        c.SHA,
		"message":    firstLine(c.Message),
		"repository": c.Repository,
		"branch":     c.Branch,
		"author":     c.Author,
	})
	return err == nil, err
}

// CommitsOf returns the commits linked to an issue, oldest first.
func (e *Enforcer) CommitsOf(id string) ([]projection.Commit, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return nil, fmt.Errorf("unknown issue %s", id)
	}
	out := make([]projection.Commit, len(issue.Commits))
	copy(out, issue.Commits)
	return out, nil
}

// sameCommit reports whether two object names refer to the same commit, allowing for
// one being abbreviated. Git abbreviates freely, so a sweep that used short hashes
// last week must not re-link the same work with full ones today.
func sameCommit(a, b string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	return strings.HasPrefix(b, a)
}

// firstLine keeps the subject of a commit message. The body is often long, sometimes
// enormous, and the log is not the place to store a second copy of the repository.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
