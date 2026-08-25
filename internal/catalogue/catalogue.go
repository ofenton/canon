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
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ofenton/canon/internal/conform"
	"github.com/ofenton/canon/internal/ingest"
	"github.com/ofenton/canon/internal/source"
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

// RefreshFrom reads every resolved source and replaces what the catalogue holds.
//
// A source that resolved to nothing is kept as an entry carrying its error, for the
// same reason an unreadable repository is: a product that silently disappears from a
// catalogue is worse than one that appears with an error, and a source that vanishes
// cannot be reported at all.
func (c *Catalogue) RefreshFrom(results []source.Result, now func() time.Time) {
	fresh := make(map[string]*Entry, len(results))
	at := now().UTC()

	for _, r := range results {
		if r.Err != nil && len(r.Paths) == 0 {
			fresh[r.Source.Line] = &Entry{
				Source: r.Source.Line, Err: r.Err.Error(), RefreshedAt: at,
			}
			continue
		}
		for _, path := range r.Paths {
			fresh[path] = read(path, now, at)
		}
	}

	c.mu.Lock()
	c.entries, c.last = fresh, at
	c.mu.Unlock()
}

// Refresh reads every source path and replaces what the catalogue holds.
//
// One failing source does not stop the others, for the same reason one malformed
// increment does not stop an ingest: the repositories most worth reporting on are
// the ones that fail.
func (c *Catalogue) Refresh(sources []string, now func() time.Time) {
	fresh := make(map[string]*Entry, len(sources))
	at := now().UTC()

	for _, src := range sources {
		fresh[src] = read(src, now, at)
	}

	c.mu.Lock()
	c.entries, c.last = fresh, at
	c.mu.Unlock()
}

// read ingests one repository and checks it against the template.
func read(path string, now func() time.Time, at time.Time) *Entry {
	entry := &Entry{Source: path, RefreshedAt: at}
	repo, err := ingest.Repo(path, now)
	if err != nil {
		entry.Err = err.Error()
		return entry
	}
	entry.Repository = repo
	if commits, err := ingest.Commits(path); err == nil {
		entry.Report = conform.Check(repo, commits)
	} else {
		entry.Report = conform.Check(repo, ingest.CommitStats{})
	}
	return entry
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
