// Package catalogue holds every product Canon knows about.
//
// A product is a repository that follows the template, so the catalogue is the set of
// conforming repositories. It is entirely derived: nothing here is authored, and
// discarding it and refreshing produces the same thing.
//
// Reads never touch the network or the filesystem. Refreshing is a separate act with
// a recorded time, so a stale view can say it is stale rather than presenting itself
// as current — which matters more for an aggregator than for a system that owns its
// data, because here "current" always means "as of the last time we looked".
package catalogue

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ofenton/canon/internal/conform"
	"github.com/ofenton/canon/internal/ingest"
)

// Entry is one product, as of the last refresh.
type Entry struct {
	Repository *ingest.Repository `json:"repository,omitempty"`
	Report     conform.Report     `json:"conformance"`
	// Source is where it was read from.
	Source string `json:"source"`
	// Err is why this source could not be read, if it could not be. A source that
	// fails is kept and reported rather than dropped: a product that silently
	// disappears from a catalogue is worse than one that appears with an error.
	Err string `json:"error,omitempty"`
	// RefreshedAt is when this entry was last read.
	RefreshedAt time.Time `json:"refreshed_at"`
}

// Name is what to call this product, even when reading it failed.
func (e Entry) Name() string {
	if e.Repository != nil && e.Repository.Name != "" {
		return e.Repository.Name
	}
	return filepath.Base(e.Source)
}

// Catalogue is the set of products, safe for concurrent reads while refreshing.
type Catalogue struct {
	mu      sync.RWMutex
	entries map[string]*Entry
	last    time.Time
}

// New builds an empty catalogue.
func New() *Catalogue { return &Catalogue{entries: map[string]*Entry{}} }

// Refresh reads every source and replaces what the catalogue holds.
//
// One failing source does not stop the others, for the same reason one malformed
// increment does not stop an ingest: the repositories most worth reporting on are
// the ones that fail.
func (c *Catalogue) Refresh(sources []string, now func() time.Time) {
	fresh := make(map[string]*Entry, len(sources))
	at := now().UTC()

	for _, src := range sources {
		entry := &Entry{Source: src, RefreshedAt: at}
		repo, err := ingest.Repo(src, now)
		if err != nil {
			entry.Err = err.Error()
			fresh[src] = entry
			continue
		}
		entry.Repository = repo
		if commits, err := ingest.Commits(src); err == nil {
			entry.Report = conform.Check(repo, commits)
		} else {
			entry.Report = conform.Check(repo, ingest.CommitStats{})
		}
		fresh[src] = entry
	}

	c.mu.Lock()
	c.entries, c.last = fresh, at
	c.mu.Unlock()
}

// Entries returns every product, sorted by name, as of the last refresh.
func (c *Catalogue) Entries() []*Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]*Entry, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Entry returns one product by name.
func (c *Catalogue) Entry(name string) (*Entry, bool) {
	for _, e := range c.Entries() {
		if e.Name() == name {
			return e, true
		}
	}
	return nil, false
}

// RefreshedAt is when the catalogue was last read. Zero means never.
func (c *Catalogue) RefreshedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.last
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
	if isRepo(root) {
		found = append(found, root)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name())
		if isRepo(path) {
			found = append(found, path)
		}
	}
	sort.Strings(found)
	return found, nil
}

func isRepo(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ingest.LedgerPath)); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}
