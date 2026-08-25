package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ofenton/canon/internal/catalogue"
	"github.com/ofenton/canon/internal/conform"
)

// catalogueCmd lists every product found under a root.
//
// Discovery is by artifact: anything with a ledger at the path the template fixes.
// Adopting Canon is committing a file, not registering anywhere.
func catalogueCmd(args []string) error {
	fs := flag.NewFlagSet("catalogue", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the catalogue as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root := "."
	if fs.NArg() > 0 {
		root = fs.Arg(0)
	}

	sources, err := catalogue.Discover(root)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		fmt.Printf("no products under %s\n\n  A product is a repository with %s.\n",
			root, "specs/increment-plan.md")
		return nil
	}

	c := catalogue.New()
	c.Refresh(sources, time.Now)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(c.Entries())
	}

	fmt.Printf("%d product(s) under %s\n\n", len(sources), root)
	for _, e := range c.Entries() {
		if e.Err != "" {
			// Not dropped. A repository mid-adoption — the template's files
			// committed to nothing yet — is a real state somebody will hit, and a
			// product that silently vanishes from a catalogue is worse than one
			// that appears with a reason.
			fmt.Printf("  %-22s not yet readable\n  %-22s %s\n", e.Name(), "", e.Err)
			continue
		}
		r := e.Repository
		counts := e.Report.Counts()
		health := "conforms"
		if n := counts[conform.Error]; n > 0 {
			health = fmt.Sprintf("%d error(s)", n)
		} else if n := counts[conform.Warning]; n > 0 {
			health = fmt.Sprintf("%d warning(s)", n)
		}

		var open, done int
		for _, inc := range r.Increments {
			switch inc.Status {
			case "done", "abandoned":
				done++
			default:
				open++
			}
		}
		fmt.Printf("  %-22s %3d open · %3d done   %-14s %s\n",
			e.Name(), open, done, health, ago(e.RefreshedAt))
		if r.Purpose != "" {
			fmt.Printf("  %-22s %.72s\n", "", r.Purpose)
		}
	}
	fmt.Printf("\n  Read from disk at %s. Reads answer from this, not from git.\n",
		c.RefreshedAt().Format(time.RFC3339))
	return nil
}

// ago renders a timestamp the way somebody reads it when deciding whether to trust a
// number: "4 hours ago" answers the question, a date makes them do arithmetic.
func ago(t time.Time) string {
	if t.IsZero() {
		return "never read"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
