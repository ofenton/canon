// Package source says where Canon looks.
//
// A source is a place, not a repository to register. It is a local directory, a local
// repository, a repository to fetch, or an organisation to expand — and the last of
// those is why this is a list of places rather than a registry: inside an organisation
// nothing is registered, because a product appears by committing a ledger.
//
// The grammar is one line per source, `#` for comments, and nothing else. That is a
// constraint, not an oversight: ADR-0010 draws the line between the list (which says
// what Canon reads) and configuration (which would say how Canon behaves), and the
// operational form of that line is that a nested key must never parse. See
// TestTheListHasNoSchema.
package source

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ofenton/canon/internal/ingest"
)

// Kind is what a source line names.
type Kind string

const (
	// Directory is a local directory holding repositories, scanned one level deep.
	Directory Kind = "directory"
	// Repository is one local repository.
	Repository Kind = "repository"
	// Remote is a repository to fetch. Arrives in feat-041.
	Remote Kind = "remote"
	// Organisation is a host and organisation to expand. Arrives in feat-042.
	Organisation Kind = "organisation"
)

// Source is one line of the list, as written and as understood.
type Source struct {
	// Line is exactly what was written, so a report can quote it back.
	Line string
	Kind Kind
}

// Result is what one source resolved to.
//
// Paths and Err are not exclusive: a directory that holds one unreadable entry still
// yields the readable ones. A source that fails entirely yields no paths and an Err,
// and is still returned — reporting which source failed is the whole point of R71, and
// a source that vanishes from the output cannot be reported.
type Result struct {
	Source Source
	Paths  []string
	Err    error
}

// Parse reads a list of sources.
//
// It cannot fail on content. A line it does not understand becomes a source whose kind
// is unrecognised, which Resolve reports — the alternative is refusing to start because
// of one bad line, which would let a typo in a list of forty hide the other thirty-nine.
func Parse(r io.Reader) ([]Source, error) {
	var out []Source
	scan := bufio.NewScanner(r)
	for scan.Scan() {
		line := scan.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, Source{Line: line, Kind: classify(line)})
	}
	return out, scan.Err()
}

// ParseFile reads a list from a path.
func ParseFile(path string) ([]Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// classify decides what a line names, from its shape alone.
//
// Deliberately syntactic. Asking the filesystem or the network here would make the
// meaning of a line depend on when it was read, and a source that is a repository on
// Monday and a directory on Tuesday is not something anyone can reason about.
func classify(line string) Kind {
	switch {
	case strings.HasPrefix(line, "github:"):
		return Organisation
	case strings.Contains(line, "://"):
		return Remote
	// scp-style git remotes: user@host:path. Distinguished from a Windows drive letter
	// by requiring the @, which a path does not have.
	case strings.Contains(line, "@") && strings.Contains(line, ":"):
		return Remote
	default:
		return Directory
	}
}

// Resolve turns sources into repository paths.
//
// One source failing never stops the others, which is the same rule the catalogue
// applies to one unparseable ledger and for the same reason: the sources most worth
// reporting on are the ones that fail.
// cache is where fetched repositories are kept; see DefaultCacheDir.
func Resolve(sources []Source, cache string) []Result {
	out := make([]Result, 0, len(sources))
	for _, s := range sources {
		out = append(out, resolve(s, cache))
	}
	return out
}

func resolve(s Source, cache string) Result {
	res := Result{Source: s}
	switch s.Kind {
	case Remote:
		// Paths and Err together is the stale case: the remote was unreachable and
		// the previous fetch is still on disk. Reported, and still served.
		path, err := fetch(cache, s.Line)
		res.Err = err
		if path != "" {
			res.Paths = []string{path}
		}
		return res
	case Organisation:
		res.Err = fmt.Errorf("expanding an organisation is not built yet; list its repositories")
		return res
	}

	path, err := expand(s.Line)
	if err != nil {
		res.Err = err
		return res
	}
	info, err := os.Stat(path)
	if err != nil {
		res.Err = fmt.Errorf("%s cannot be read: %w", s.Line, err)
		return res
	}
	if !info.IsDir() {
		res.Err = fmt.Errorf("%s is not a directory", s.Line)
		return res
	}
	// A repository is a directory that is one. Checked rather than declared, so a line
	// does not have to say which kind it is and cannot say so wrongly.
	if IsRepo(path) {
		res.Source.Kind = Repository
		res.Paths = []string{path}
		return res
	}
	found, err := Discover(path)
	if err != nil {
		res.Err = err
		return res
	}
	if len(found) == 0 {
		// Not an error — a directory that holds no products today may hold one
		// tomorrow — but silence here reads as "Canon is working", so it is said.
		res.Err = fmt.Errorf("%s holds no repository with %s", s.Line, ingest.LedgerPath)
		return res
	}
	res.Paths = found
	return res
}

// expand resolves a leading ~ and makes the path absolute.
func expand(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}
	return filepath.Abs(path)
}

// Paths flattens results to the repositories to ingest.
func Paths(results []Result) []string {
	var out []string
	for _, r := range results {
		out = append(out, r.Paths...)
	}
	return out
}

// Discover finds repositories under a root directory.
//
// A repository is anything with a ledger at the path the template fixes. Searching by
// artifact rather than by a list means adopting Canon is committing a file, not
// registering anywhere — which is the adoption story ADR-0009 wanted.
//
// One level deep only: a parent directory of checkouts is the shape people have, and
// walking an entire filesystem to find ledgers would be slow and surprising.
func Discover(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", root, err)
	}

	var found []string
	// The root may itself be a repository rather than a directory of them.
	if IsRepo(root) {
		found = append(found, root)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name())
		if IsRepo(path) {
			found = append(found, path)
		}
	}
	sort.Strings(found)
	return found, nil
}

func IsRepo(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ingest.LedgerPath)); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}
