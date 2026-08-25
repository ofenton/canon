package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/ingest"
)

const plan = `# Increment plan

## feat-001: Do the thing

- **Type:** feature
- **Status:** done
- **Scope:** The thing.
- **Test Strategy:**
  - Unit
- **Rollback Plan:** Revert
- **Risk:** Low
- **Evidence:** shipped

## Sequencing
`

// origin builds a repository with something in it and returns a URL for it.
//
// file:// rather than a network host: git treats it as a real remote — clone, fetch,
// refs and all — so the code under test takes the same path it would over ssh, and the
// test needs nothing but git.
func origin(t *testing.T) (dir, url string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	dir = filepath.Join(t.TempDir(), "orders")
	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "specs", "increment-plan.md"), []byte(plan), 0o644)
	os.WriteFile(filepath.Join(dir, "specs", "product.md"), []byte("# Orders\n\n## Problem\n\nOrders are late.\n"), 0o644)

	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@e.com"},
		{"config", "user.name", "T"},
		{"add", "."},
		{"commit", "-q", "-m", "plan"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return dir, "file://" + dir
}

// AC: WHEN a source is a git URL THE SYSTEM SHALL clone it and ingest it like a local
// repository.
func TestARemoteIsClonedAndReadsLikeAnyOther(t *testing.T) {
	_, url := origin(t)
	cache := t.TempDir()

	got := Resolve([]Source{{url, Remote}}, cache)
	if got[0].Err != nil {
		t.Fatalf("%s: %v", url, got[0].Err)
	}
	if len(got[0].Paths) != 1 {
		t.Fatalf("got %d paths", len(got[0].Paths))
	}

	// A mirror, not a checkout: ingest reads through `git show HEAD:path`, so files on
	// disk would be spent on nothing. Asserted, because "it worked" would also be true
	// of a full clone quietly costing every repository its whole working tree.
	path := got[0].Paths[0]
	if _, err := os.Stat(filepath.Join(path, "specs")); err == nil {
		t.Error("the cache holds a working tree; a mirror is enough and a checkout is not free")
	}
	out, err := exec.Command("git", "-C", path, "show", "HEAD:specs/increment-plan.md").Output()
	if err != nil || !strings.Contains(string(out), "feat-001") {
		t.Fatalf("the mirror cannot be read: %v", err)
	}
}

// AC: WHEN the cache is deleted THE SYSTEM SHALL rebuild it and produce the same
// catalogue.
//
// The whole of R72 in one test. If this ever fails, something is being kept in the cache
// that was never read from a source, and the cache has become a store.
func TestDeletingTheCacheLosesNothing(t *testing.T) {
	_, url := origin(t)
	cache := t.TempDir()

	first := Resolve([]Source{{url, Remote}}, cache)
	before := read(t, first[0].Paths[0])

	if err := os.RemoveAll(cache); err != nil {
		t.Fatal(err)
	}
	second := Resolve([]Source{{url, Remote}}, cache)
	if second[0].Err != nil {
		t.Fatalf("rebuilding after deletion failed: %v", second[0].Err)
	}
	after := read(t, second[0].Paths[0])

	if before != after {
		t.Errorf("the catalogue changed when the cache was rebuilt:\n  before %s\n  after  %s", before, after)
	}
	// And it landed in the same place, because the path is derived from the URL rather
	// than from anything remembered.
	if first[0].Paths[0] != second[0].Paths[0] {
		t.Errorf("cache path is not deterministic:\n  %s\n  %s", first[0].Paths[0], second[0].Paths[0])
	}
}

// AC: WHEN a remote is unreachable THE SYSTEM SHALL keep serving what it read last and
// say when that was.
func TestAnUnreachableRemoteKeepsServingWhatItRead(t *testing.T) {
	dir, url := origin(t)
	cache := t.TempDir()

	if got := Resolve([]Source{{url, Remote}}, cache); got[0].Err != nil {
		t.Fatalf("first fetch: %v", got[0].Err)
	}
	// The host goes away. Everything already fetched is still on disk.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	got := Resolve([]Source{{url, Remote}}, cache)
	if got[0].Err == nil {
		t.Fatal("an unreachable remote must report, or a stale view presents itself as current")
	}
	if len(got[0].Paths) != 1 {
		t.Fatalf("got %d paths; what was read before must still be served", len(got[0].Paths))
	}
	if !strings.Contains(got[0].Err.Error(), url) {
		t.Errorf("the failure does not name its source: %v", got[0].Err)
	}
	if read(t, got[0].Paths[0]) == "" {
		t.Error("the cached repository is no longer readable")
	}
}

// A clone that fails leaves nothing behind, or the wreckage is treated as cached next
// time and never repaired.
func TestAFailedCloneLeavesNoWreckage(t *testing.T) {
	cache := t.TempDir()
	url := "file://" + filepath.Join(t.TempDir(), "nothing-here")

	got := Resolve([]Source{{url, Remote}}, cache)
	if got[0].Err == nil {
		t.Fatal("cloning nothing should fail")
	}
	if len(got[0].Paths) != 0 {
		t.Errorf("a failed clone reported a path: %v", got[0].Paths)
	}
	if _, err := os.Stat(cachePath(cache, url)); err == nil {
		t.Error("a partial clone was left in the cache; the next run would trust it")
	}
}

// Two repositories with the same name on different hosts must not collide.
func TestCachePathsDoNotCollide(t *testing.T) {
	cache := "/cache"
	a := cachePath(cache, "https://one.example/team/orders.git")
	b := cachePath(cache, "https://two.example/team/orders.git")
	if a == b {
		t.Fatalf("both landed in %s", a)
	}
	if !strings.Contains(filepath.Base(a), "orders") {
		t.Errorf("%s is not findable by a person", a)
	}
}

// read returns a stable summary of what a repository ingests to.
//
// Through ingest rather than through `git show`, because "the same catalogue" is a claim
// about what Canon derives, not about the bytes in the cache. Fingerprint exists to let
// two ingests of the same commit be compared, which is exactly the question here.
func read(t *testing.T, path string) string {
	t.Helper()
	repo, err := ingest.Repo(path, func() time.Time { return fixedTime })
	if err != nil {
		t.Fatalf("ingesting %s: %v", path, err)
	}
	return repo.Fingerprint()
}

var fixedTime = time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
