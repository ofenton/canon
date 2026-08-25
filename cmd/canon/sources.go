package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ofenton/canon/internal/source"
)

// defaultList is read when no source is named. Canon looks for it in the working
// directory, which is where an operator's list belongs — it is theirs, not Canon's.
const defaultList = "canon.sources"

// cacheDir is where fetched repositories are kept. A package variable because every
// command that can name a source can also fetch one, and threading it through each
// would be four signatures carrying the same thing.
var cacheDir = source.DefaultCacheDir()

// lines collects a repeatable flag.
type lines []string

func (l *lines) String() string     { return strings.Join(*l, ",") }
func (l *lines) Set(v string) error { *l = append(*l, v); return nil }

// sourceFlags registers the two ways to say where to look, and returns a function that
// reads them once the flag set has been parsed.
//
// Two ways rather than one because they answer different questions: -source is for
// trying something, and a list is for running something. The list is the one that
// belongs under review, so it is the one that has a default.
func sourceFlags(fs *flag.FlagSet) func() ([]source.Source, error) {
	var direct lines
	fs.Var(&direct, "source", "a place to look; repeatable")
	list := fs.String("sources", "", "a file listing places to look (default "+defaultList+" if present)")
	fs.StringVar(&cacheDir, "cache", source.DefaultCacheDir(), "where fetched repositories are kept")

	return func() ([]source.Source, error) {
		if len(direct) > 0 {
			var out []source.Source
			for _, line := range direct {
				parsed, err := source.Parse(strings.NewReader(line))
				if err != nil {
					return nil, err
				}
				out = append(out, parsed...)
			}
			if *list == "" {
				return out, nil
			}
			from, err := source.ParseFile(*list)
			return append(out, from...), err
		}

		if *list != "" {
			return source.ParseFile(*list)
		}
		if _, err := os.Stat(defaultList); err == nil {
			return source.ParseFile(defaultList)
		}
		// The working directory. Canon reading the repository it is run from is the
		// smallest useful thing it can do, and it is how this project reads itself.
		return []source.Source{{Line: ".", Kind: source.Directory}}, nil
	}
}

// report prints what each source resolved to, so a failure is visible at the point of
// starting rather than only as a missing product in the UI.
func report(results []source.Result) {
	for _, r := range results {
		switch {
		case r.Err != nil && len(r.Paths) == 0:
			fmt.Printf("  %-28s %s\n", r.Source.Line, r.Err)
		case r.Err != nil:
			fmt.Printf("  %-28s %d found, and: %s\n", r.Source.Line, len(r.Paths), r.Err)
		default:
			fmt.Printf("  %-28s %d found\n", r.Source.Line, len(r.Paths))
		}
	}
}
