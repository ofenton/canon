package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ofenton/canon/internal/ingest"
)

// ingestCmd reads a repository and prints what Canon derived from it.
//
// The point of having this as a command is that ingestion should be inspectable
// before it is trusted: a reader of other people's repositories that cannot show its
// working is asking to be believed.
func ingestCmd(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the ingested repository as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	r, err := ingest.Repo(path, time.Now)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}

	fmt.Printf("%s\n", r.Name)
	if r.Purpose != "" {
		fmt.Printf("  %.100s\n", r.Purpose)
	}
	fmt.Printf("\n  head %s", short(r.Head))
	if r.Remote != "" {
		fmt.Printf(" · %s", r.Remote)
	}
	fmt.Printf("\n  %d increments · %d requirements\n\n", len(r.Increments), len(r.Requirements))

	byStatus := map[string]int{}
	var transitions int
	for _, inc := range r.Increments {
		byStatus[inc.Status]++
		transitions += len(inc.Transitions)
	}
	for _, status := range []string{"planned", "approved", "in-progress", "in-review", "done", "abandoned"} {
		if n := byStatus[status]; n > 0 {
			fmt.Printf("  %-14s %4d\n", status, n)
		}
	}
	fmt.Printf("\n  %d status transitions, derived from %s\n", transitions, ingest.LedgerPath)
	return nil
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
