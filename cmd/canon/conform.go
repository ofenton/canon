package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ofenton/canon/internal/conform"
	"github.com/ofenton/canon/internal/ingest"
)

// conformCmd reports how faithfully a repository follows the template.
//
// Exits non-zero only on an error-severity finding, so this is usable in CI without
// making a note about untracked commits fail somebody's build.
func conformCmd(args []string) error {
	fs := flag.NewFlagSet("conform", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the report as JSON")
	strict := fs.Bool("strict", false, "exit non-zero on warnings as well as errors")
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
	commits, err := ingest.Commits(path)
	if err != nil {
		return err
	}
	report := conform.Check(r, commits)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printReport(r, report)
	}

	counts := report.Counts()
	if counts[conform.Error] > 0 || (*strict && counts[conform.Warning] > 0) {
		return fmt.Errorf("%s does not conform", r.Name)
	}
	return nil
}

func printReport(r *ingest.Repository, report conform.Report) {
	fmt.Printf("%s\n", r.Name)
	fmt.Printf("  %d increments · %d requirements · head %s\n\n",
		len(r.Increments), len(r.Requirements), short(r.Head))

	counts := report.Counts()
	if len(report.Findings) == 0 {
		fmt.Printf("  conforms — nothing to report\n")
		return
	}
	for _, sev := range []conform.Severity{conform.Error, conform.Warning, conform.Note} {
		if n := counts[sev]; n > 0 {
			fmt.Printf("  %-8s %d\n", sev, n)
		}
	}
	fmt.Println()

	for _, sev := range []conform.Severity{conform.Error, conform.Warning, conform.Note} {
		for _, f := range report.Findings {
			if f.Severity != sev {
				continue
			}
			subject := f.Subject
			if subject == "" {
				subject = "—"
			}
			fmt.Printf("  %-8s %-12s %s\n", f.Severity, subject, wrap(f.Message, 62, 24))
		}
	}
}

// wrap keeps a long message readable in a column without a dependency.
func wrap(s string, width, indent int) string {
	var out, line string
	pad := "\n" + fmt.Sprintf("%*s", indent, "")
	for _, word := range splitWords(s) {
		if len(line)+len(word)+1 > width && line != "" {
			out += line + pad
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	return out + line
}

func splitWords(s string) []string {
	var out []string
	var w string
	for _, r := range s {
		if r == ' ' || r == '\n' {
			if w != "" {
				out = append(out, w)
				w = ""
			}
			continue
		}
		w += string(r)
	}
	if w != "" {
		out = append(out, w)
	}
	return out
}
