package main

import (
	"flag"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// Untracked work.
//
// The `NOJIRA` placeholder is a symptom of a policy that demands a ticket for every
// commit and offers no way to say "this one does not need one". People comply in the
// only way left to them: they type something ticket-shaped that means nothing. The
// answer is not stricter enforcement, which produces junk tickets on top of the
// placeholders. It is to make the untracked proportion visible, and to give
// deliberately untracked work somewhere honest to live.
//
// So this report separates four things a single "has a ticket?" check would flatten:
// work that is tracked, work someone deliberately declared untracked and said why,
// work carrying a placeholder that means nothing, and work nobody explained at all.
// An organisation can then decide what it will tolerate, instead of pretending the
// number is zero.

var (
	// declaredRe is the sanctioned alternative to a placeholder: say it is untracked
	// and say why. A reason is required — "Untracked:" with nothing after it is just
	// NOJIRA with extra characters.
	declaredRe = regexp.MustCompile(`(?mi)^\s*untracked\s*:\s*(\S.*?)\s*$`)

	// placeholderRe matches the ticket-shaped things people type when the process
	// leaves them no other option. Reported by name, because "2% placeholders" is
	// the number that tells an organisation its policy is not working.
	placeholderRe = regexp.MustCompile(`(?i)\b(no[-_ ]?jira|no[-_ ]?ticket|no[-_ ]?issue|n/?a)\b`)
)

// classification is what a commit turned out to be.
type classification int

const (
	tracked classification = iota
	declared
	placeholder
	danglingRef
	unexplained
)

func (c classification) String() string {
	switch c {
	case tracked:
		return "tracked"
	case declared:
		return "declared untracked"
	case placeholder:
		return "placeholder"
	case danglingRef:
		return "unknown issue"
	default:
		return "unexplained"
	}
}

type classified struct {
	commit gitCommit
	what   classification
	detail string
}

// traceCmd reports how much of a commit range carries an issue reference.
func traceCmd(args []string) error {
	fs := flag.NewFlagSet("trace", flag.ContinueOnError)
	commitRange := fs.String("range", "", "commit range to report on, e.g. main~50..main")
	repo := fs.String("repo", ".", "path to the git repository")
	maxUntracked := fs.Float64("max-untracked-pct", -1, "exit non-zero above this percentage of unexplained commits")
	includeMerges := fs.Bool("merges", false, "count merge commits too; they carry no work of their own")
	dbPath := fs.String("db", "canon.db", "path to the event log")
	schemaPath := fs.String("schema", "canon.yaml", "path to canon.yaml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	spec := *commitRange
	if spec == "" {
		spec = "HEAD"
	}

	// A merge commit is not work; the commits it joins are, and they are already in
	// the range. Counting merges as unexplained put 25 of Canon's own 33 "unexplained"
	// commits into that bucket — a number nobody would act on, and one that would
	// teach a team to ignore the report.
	var filter []string
	if !*includeMerges {
		filter = append(filter, "--no-merges")
	}
	commits, err := readCommits(*repo, spec, filter...)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		fmt.Printf("no commits in %s\n", spec)
		return nil
	}

	// Which references resolve is Canon's knowledge, not git's. A commit naming an
	// issue that does not exist looks tracked to any grep and is not.
	known, err := knownIssues(*dbPath, *schemaPath)
	if err != nil {
		return err
	}

	results := make([]classified, 0, len(commits))
	for _, c := range commits {
		results = append(results, classify(c, known))
	}
	return report(results, spec, *maxUntracked)
}

// classify decides what one commit is, preferring the most specific reading. An
// explicit declaration beats a placeholder that happens to appear in the same
// message, and a real reference beats both.
func classify(c gitCommit, known map[string]string) classified {
	message := c.Subject + "\n" + c.Body

	if ref := issueFrom(message); ref != "" {
		if known == nil {
			// With no log to ask, a reference is taken at face value.
			return classified{commit: c, what: tracked, detail: ref}
		}
		if id, ok := known[strings.ToUpper(ref)]; ok {
			return classified{commit: c, what: tracked, detail: id}
		}
		// Reported as written, so the reader can find it in the commit message.
		return classified{commit: c, what: danglingRef, detail: ref}
	}
	if m := declaredRe.FindStringSubmatch(message); m != nil {
		return classified{commit: c, what: declared, detail: m[1]}
	}
	if m := placeholderRe.FindStringSubmatch(message); m != nil {
		return classified{commit: c, what: placeholder, detail: m[1]}
	}
	return classified{commit: c, what: unexplained}
}

// knownIssues reads the ids Canon has, or nil if there is no log to read.
//
// A missing log is not an error: the proportions are still worth having, and
// demanding a database to count commits would make this useless in CI on a checkout.
func knownIssues(dbPath, schemaPath string) (map[string]string, error) {
	sch, err := schema.Load(schemaPath)
	if err != nil {
		return nil, nil
	}
	store, err := event.Open(dbPath)
	if err != nil {
		return nil, nil
	}
	defer store.Close()

	view, err := enforce.New(sch, store).Projection()
	if err != nil {
		return nil, err
	}
	// Keyed by upper case so a reference in any casing resolves, valued with the id
	// as Canon actually holds it so the caller can write against it.
	known := map[string]string{}
	for _, id := range view.IssueIDs() {
		known[strings.ToUpper(id)] = id
	}
	return known, nil
}

func report(results []classified, spec string, maxUntracked float64) error {
	counts := map[classification]int{}
	for _, r := range results {
		counts[r.what]++
	}
	total := len(results)
	pct := func(n int) float64 { return float64(n) * 100 / float64(total) }

	fmt.Printf("%d commits in %s\n\n", total, spec)
	for _, c := range []classification{tracked, declared, placeholder, danglingRef, unexplained} {
		if counts[c] == 0 {
			continue
		}
		fmt.Printf("  %-19s %4d  %5.1f%%\n", c, counts[c], pct(counts[c]))
	}

	// The headline number. Deliberately untracked work is untracked — it is counted
	// here rather than excused — but it is separated from the part nobody explained,
	// because those two ask for different responses.
	untracked := counts[declared] + counts[placeholder] + counts[danglingRef] + counts[unexplained]
	unexplainedPct := pct(counts[unexplained])
	fmt.Printf("\n  carrying no working issue reference: %.1f%%", pct(untracked))
	if counts[declared] > 0 {
		fmt.Printf("  (%.1f%% declared, %.1f%% not)", pct(counts[declared]), pct(untracked-counts[declared]))
	}
	fmt.Println()

	show := func(c classification, heading, advice string) {
		var rows []classified
		for _, r := range results {
			if r.what == c {
				rows = append(rows, r)
			}
		}
		if len(rows) == 0 {
			return
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].commit.At.Before(rows[j].commit.At) })
		fmt.Printf("\n%s\n", heading)
		for _, r := range rows {
			line := fmt.Sprintf("  %s  %s", r.commit.SHA[:7], r.commit.Subject)
			if r.detail != "" {
				line += "  [" + r.detail + "]"
			}
			fmt.Println(line)
		}
		if advice != "" {
			fmt.Printf("  → %s\n", advice)
		}
	}

	// Named, not just counted: a commit nobody can find is a commit nobody can link.
	show(unexplained, "unexplained", "canon link -issue <id> -commit <sha>, or say why: Untracked: <reason>")
	show(danglingRef, "referencing an issue that does not exist", "the reference looks fine to any grep, and resolves to nothing")
	show(placeholder, "placeholder references", "these mean nothing; Untracked: <reason> at least records the decision")

	if maxUntracked >= 0 && unexplainedPct > maxUntracked {
		return fmt.Errorf("%.1f%% of commits are unexplained, above the %.1f%% this repository allows",
			unexplainedPct, maxUntracked)
	}
	return nil
}
