// Package conform reports how faithfully a repository follows the template.
//
// It reports; it never refuses. An aggregator cannot decline a commit that has
// already happened, and pretending otherwise would be theatre. Refusing is the
// repository's own job — `validate-plan.py` does it in the pre-commit hook and in CI,
// where refusing works. This runs the same rules everywhere and says who is failing
// them.
//
// Nothing here is fatal. A repository with fifty findings still ingests, because a
// report that stops at the first problem is a report nobody can act on.
package conform

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/ingest"
)

// Severity separates what is broken from what is merely worth knowing.
//
// The distinction is load-bearing: a report where everything is an error is a report
// people learn to ignore, and the untracked-commit ratio in particular is a number to
// watch rather than a failure — work that genuinely needs no increment is normal.
type Severity string

const (
	// Error means the repository breaks a rule the template enforces locally.
	Error Severity = "error"
	// Warning means the data is misleading rather than malformed.
	Warning Severity = "warning"
	// Note means worth knowing, and not a failure.
	Note Severity = "note"
)

// Finding is one thing worth saying about a repository.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	// Subject is the increment it concerns, or empty for the repository itself.
	Subject string `json:"subject,omitempty"`
	Message string `json:"message"`
}

// Report is everything conformance found about one repository.
type Report struct {
	Repository string             `json:"repository"`
	Findings   []Finding          `json:"findings"`
	Commits    ingest.CommitStats `json:"commits"`
}

// Counts summarises findings by severity.
func (r Report) Counts() map[Severity]int {
	out := map[Severity]int{}
	for _, f := range r.Findings {
		out[f.Severity]++
	}
	return out
}

// Conforms reports whether anything is actually broken. Warnings and notes do not
// make a repository non-conforming.
func (r Report) Conforms() bool { return r.Counts()[Error] == 0 }

// The template's rules, as `.sdlc/bin/validate-plan.py` defines them. Duplicated here
// deliberately rather than shelled out to: Canon reads repositories it does not
// control, and cannot assume a copy of the template's Python is present or current.
// ADR-0006 proposes distributing these from one place; until then this is the second
// copy and saying so is better than pretending otherwise.
var (
	statuses = map[string]bool{
		"planned": true, "approved": true, "in-progress": true,
		"in-review": true, "done": true, "abandoned": true,
	}
	types = map[string]bool{
		"feature": true, "fix": true, "security": true, "perf": true,
		"refactor": true, "chore": true, "docs": true,
	}
	required = []string{"type", "scope", "test_strategy", "rollback_plan", "risk"}
	terminal = map[string]bool{"done": true, "abandoned": true}
)

// Check runs every rule over an ingested repository.
func Check(r *ingest.Repository, commits ingest.CommitStats) Report {
	rep := Report{Repository: r.Name, Commits: commits}
	add := func(rule string, sev Severity, subject, format string, args ...any) {
		rep.Findings = append(rep.Findings, Finding{
			Rule: rule, Severity: sev, Subject: subject,
			Message: fmt.Sprintf(format, args...),
		})
	}

	known := map[string]bool{}
	for _, req := range r.Requirements {
		known[req.ID] = true
	}
	ids := map[string]bool{}
	for _, inc := range r.Increments {
		ids[inc.ID] = true
	}

	var inProgress []string
	for _, inc := range r.Increments {
		switch {
		case inc.Status == "":
			add("status", Error, inc.ID, "has no status")
		case !statuses[inc.Status]:
			add("status", Error, inc.ID, "status %q is not one of %s",
				inc.Status, keys(statuses))
		}
		if inc.Status == "in-progress" {
			inProgress = append(inProgress, inc.ID)
		}

		if inc.Type != "" && !types[inc.Type] {
			add("type", Error, inc.ID, "type %q is not one of %s", inc.Type, keys(types))
		}
		for _, field := range required {
			if inc.Fields[field] == "" && !(field == "type" && inc.Type != "") {
				add("required", Error, inc.ID, "has no %s", strings.ReplaceAll(field, "_", " "))
			}
		}

		if terminal[inc.Status] {
			if inc.Fields["evidence"] == "" {
				add("evidence", Error, inc.ID, "is %s with no evidence", inc.Status)
			}
			for _, c := range inc.Criteria {
				if !c.Met {
					add("criteria", Error, inc.ID, "is %s with an unticked criterion: %.60s",
						inc.Status, c.Text)
				}
			}
		}

		for _, dep := range inc.DependsOn {
			if !ids[dep] {
				add("dependency", Error, inc.ID, "depends on %s, which is not in this ledger", dep)
			}
		}
		// A trace to a requirement that does not exist looks fine to any grep and
		// traces nothing.
		for _, trace := range inc.Traces {
			if strings.HasPrefix(trace, "R") && len(known) > 0 && !known[trace] {
				add("trace", Error, inc.ID, "traces to %s, which is not in the spec", trace)
			}
		}
	}

	// The WIP limit is the main brake on an agent half-finishing several things.
	if len(inProgress) > 1 {
		sort.Strings(inProgress)
		add("wip", Error, "", "%d increments are in-progress at once: %s",
			len(inProgress), strings.Join(inProgress, ", "))
	}

	rep.Findings = append(rep.Findings, cycleTimeReliability(r)...)
	rep.Findings = append(rep.Findings, referenceDiscipline(commits)...)
	return rep
}

// unreliableGap is how close in-progress and in-review must be before cycle time
// stops describing the work. Two minutes is generous: it is roughly the time to make
// two commits, and nothing real finishes that fast.
const unreliableGap = 2 * time.Minute

// cycleTimeReliability reports when in-progress is recorded so close to in-review
// that cycle time measures the commits rather than the work.
//
// This is the rule no repository-local check can produce, because it is a property of
// how a team runs the loop rather than of any one commit. Found by measuring this
// repository: the median gap was 3.4 minutes, because `in-progress` was being
// committed alongside the finished code instead of before the work started —
// `implement-increment` says to set it at step 2, before the tests are written.
func cycleTimeReliability(r *ingest.Repository) []Finding {
	var measured, suspicious int
	for _, inc := range r.Increments {
		start, review := firstAt(inc, "in-progress"), firstAt(inc, "in-review")
		if start.IsZero() || review.IsZero() || !review.After(start) {
			continue
		}
		measured++
		if review.Sub(start) < unreliableGap {
			suspicious++
		}
	}
	if measured == 0 || suspicious == 0 {
		return nil
	}
	return []Finding{{
		Rule: "cycle-time", Severity: Warning,
		Message: fmt.Sprintf(
			"cycle time understates the work: %d of %d increments record in-progress "+
				"within %s of in-review, so it measures two commits rather than the work. "+
				"Set in-progress before starting, not alongside the result",
			suspicious, measured, unreliableGap),
	}}
}

// referenceDiscipline reports how much work carries no increment reference.
//
// A note, not an error. Work that genuinely needs no increment is the Direct track and
// is normal; what is not normal is nobody knowing how much of it there is. Forcing an
// increment for every typo is what produces placeholder references in the first place.
func referenceDiscipline(c ingest.CommitStats) []Finding {
	if c.Total == 0 {
		return nil
	}
	unexplained := c.Total - c.Tracked - c.Declared
	if unexplained == 0 {
		return nil
	}
	pct := float64(unexplained) * 100 / float64(c.Total)
	return []Finding{{
		Rule: "reference", Severity: Note,
		Message: fmt.Sprintf("%d of %d commits (%.0f%%) carry no increment reference and were not "+
			"declared untracked", unexplained, c.Total, pct),
	}}
}

func firstAt(inc ingest.Increment, status string) time.Time {
	for _, t := range inc.Transitions {
		if t.To == status {
			if at, err := time.Parse(time.RFC3339, t.At); err == nil {
				return at
			}
		}
	}
	return time.Time{}
}

func keys(m map[string]bool) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
