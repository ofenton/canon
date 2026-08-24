package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// usageCmd reports what each declared thing is actually doing.
//
// `canon schema` says how big the schema is; this says how much of it is alive. The
// two together are the argument: a reviewer approving the 40th field can see what the
// existing 39 are doing, which is the thing no Jira admin has ever been shown.
func usageCmd(args []string) error {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	dbPath := fs.String("db", "canon.db", "path to the event log")
	schemaPath := fs.String("schema", "canon.yaml", "path to canon.yaml")
	unusedOnly := fs.Bool("unused", false, "list only what nothing uses")
	if err := fs.Parse(args); err != nil {
		return err
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

	rows, err := enforce.New(sch, store).SchemaUsage()
	if err != nil {
		return err
	}

	byKind := map[string][]enforce.Usage{}
	var unused int
	for _, r := range rows {
		if !r.Used() {
			unused++
		}
		if *unusedOnly && r.Used() {
			continue
		}
		byKind[r.Kind] = append(byKind[r.Kind], r)
	}

	// A fixed order, most-decided-first, so two runs read the same way.
	for _, kind := range []string{"field", "state", "issue_type", "team", "role"} {
		rows := byKind[kind]
		if len(rows) == 0 {
			continue
		}
		fmt.Printf("\n%s\n", strings.ReplaceAll(kind, "_", " "))
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Count > rows[j].Count })
		for _, r := range rows {
			if !r.Used() {
				fmt.Printf("  %-22s %6s   %s\n", r.Name, "unused", "nothing uses this")
				continue
			}
			fmt.Printf("  %-22s %6d   last used %s%s\n",
				r.Name, r.Count, ago(r.LastUsed), spread(r.Detail))
		}
	}

	fmt.Printf("\n%d declared, %d unused", len(rows)+countUsed(byKind, *unusedOnly), unused)
	if unused > 0 {
		fmt.Printf(" — every unused row is a line somebody could delete from canon.yaml")
	}
	fmt.Println()
	return nil
}

// countUsed keeps the total honest when -unused has filtered the rows out.
func countUsed(byKind map[string][]enforce.Usage, filtered bool) int {
	if !filtered {
		return 0
	}
	var n int
	for _, rows := range byKind {
		n += len(rows)
	}
	return -n
}

// spread summarises an enum's actual distribution.
//
// A four-value enum where everything is p2 is a field pretending to be a decision,
// and that is invisible from a count alone.
func spread(detail map[string]int) string {
	if len(detail) == 0 {
		return ""
	}
	values := make([]string, 0, len(detail))
	for v := range detail {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return detail[values[i]] > detail[values[j]] })
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("%s %d", v, detail[v]))
	}
	return "  (" + strings.Join(parts, ", ") + ")"
}

// ago renders a timestamp the way somebody reads it when deciding whether to delete
// something: "4 months ago" answers the question, a date makes them do arithmetic.
func ago(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "under an hour ago"
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%d months ago", int(d.Hours()/24/30))
	}
}
