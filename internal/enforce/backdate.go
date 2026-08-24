package enforce

import (
	"fmt"
	"time"
)

// BackdateOp is the operation of writing an event dated earlier than now.
//
// It is a permission of its own rather than a property of the write it accompanies,
// because the risk is different in kind. Being allowed to transition an issue says
// nothing about whether you should be allowed to record that it happened last
// Tuesday: the first changes what is true, the second changes what the record says
// was true. Every existing schema omits the verb, so backdating is refused by
// default until somebody grants it deliberately.
const BackdateOp = "backdate"

// AuthoriseBackdate decides whether p may write an event against subject dated at,
// given the current instant now.
//
// A write dated now or later than now is not a backdate and needs no grant — the
// common case must stay free, or every ordinary write would pay for this feature.
// Callers pass the timestamp they intend to use and proceed only if this returns
// nil; nothing is written here.
func (e *Enforcer) AuthoriseBackdate(p Principal, subject string, at, now time.Time) error {
	if !at.Before(now) {
		// Clock skew between a client and the server is ordinary; a timestamp
		// meaningfully ahead of the server is not, and would produce an event that
		// appears not to have happened yet.
		if at.Sub(now) > futureTolerance {
			return fmt.Errorf("cannot write %s dated %s: that is in the future (now is %s)",
				subject, at.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
		}
		return nil
	}

	if err := e.refresh(); err != nil {
		return err
	}

	// A team-scoped role must not reach outside its team to backdate, so the check
	// carries the issue's team exactly as an ordinary write would.
	var ownerTeam string
	if issue, ok := e.view.Issue(subject); ok {
		ownerTeam = issue.Team
	}

	return e.authorise(p, BackdateOp, subject, ownerTeam)
}

// CheckNotBeforeCreation refuses an event dated before the issue it describes existed.
//
// This is separate from AuthoriseBackdate because it is not true of every backdated
// write. An issue's own history cannot begin before the issue does — transitioning it
// last March when it was created in August describes something that did not happen.
// A *commit*, though, routinely predates the issue that tracks it: that is the whole
// NOJIRA case, where work is done first and recorded afterwards. Folding both rules
// into one check made linking real history impossible, which was found building
// feat-024 and is why the two are now asked separately.
func (e *Enforcer) CheckNotBeforeCreation(subject string, at time.Time) error {
	if err := e.refresh(); err != nil {
		return err
	}
	issue, ok := e.view.Issue(subject)
	if !ok {
		return nil
	}
	if at.Before(issue.CreatedAt) {
		return fmt.Errorf("cannot write %s dated %s: the issue was created %s, and an event before that would describe an issue that did not exist",
			subject, at.UTC().Format(time.RFC3339), issue.CreatedAt.UTC().Format(time.RFC3339))
	}
	return nil
}

// futureTolerance is the clock skew accepted before a timestamp is treated as
// future-dated. Small enough that nobody backdates forwards by accident, large
// enough that an unsynchronised laptop still works.
const futureTolerance = 2 * time.Minute
