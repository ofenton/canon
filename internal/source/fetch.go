package source

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// fetchTimeout bounds one network operation. An unreachable host must fail rather than
// hang: a refresh that never returns leaves the catalogue serving nothing, which is a
// worse outcome than serving something stale.
const fetchTimeout = 2 * time.Minute

// DefaultCacheDir is where fetched repositories live when nobody says otherwise.
//
// Under the user's cache directory on purpose. The operating system already has a
// convention for "data that can be deleted without loss", and this is exactly that:
// nothing is ever read from here that could not be read from origin.
func DefaultCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "canon")
	}
	return filepath.Join(base, "canon")
}

// cachePath is where one URL is kept.
//
// A readable name so a person can find it, and a hash so two repositories called
// `orders` on different hosts cannot collide. Deterministic, which is what makes
// deleting the cache and rebuilding it produce the same catalogue.
func cachePath(cache, url string) string {
	sum := sha256.Sum256([]byte(url))
	name := url
	if i := strings.LastIndexAny(name, "/:"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".git")
	if name == "" {
		name = "repository"
	}
	return filepath.Join(cache, name+"-"+hex.EncodeToString(sum[:4]))
}

// fetch brings the cache up to date with one remote and returns where it landed.
//
// A mirror rather than a checkout: ingest reads through `git show HEAD:path` and never
// touches a working tree, so a checkout would be disk spent on files nothing opens.
//
// The returned error is not fatal when a path comes back with it. That pairing is the
// stale case — the remote could not be reached but the last fetch is still on disk —
// and it is the difference between a dashboard that goes blank when a host blips and
// one that keeps showing yesterday and says so.
func fetch(cache, url string) (string, error) {
	path := cachePath(cache, url)

	if _, err := os.Stat(path); err == nil {
		if _, err := run(path, "fetch", "--prune", "--quiet"); err != nil {
			return path, fmt.Errorf("%s could not be fetched, showing what was read before: %w", url, err)
		}
		return path, nil
	}

	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", fmt.Errorf("preparing the cache: %w", err)
	}
	// A partial clone left behind by a failure would be treated as cached next time
	// and never repaired, so it is cleaned up here rather than diagnosed later.
	if _, err := run("", "clone", "--mirror", "--quiet", url, path); err != nil {
		os.RemoveAll(path)
		return "", fmt.Errorf("%s could not be cloned: %w", url, err)
	}
	return path, nil
}

// run executes git, bounded in time, returning stderr on failure because git says the
// useful thing there.
func run(dir string, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	// Never prompt. A credential prompt in a background refresh hangs until the
	// timeout and looks, from the outside, exactly like an unreachable host.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	done := make(chan error, 1)
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			return out.String(), fmt.Errorf("%s", strings.TrimSpace(lastLine(out.String())))
		}
		return out.String(), nil
	case <-time.After(fetchTimeout):
		_ = cmd.Process.Kill()
		<-done
		return "", fmt.Errorf("gave up after %s", fetchTimeout)
	}
}

// lastLine picks the line git actually complained about, since it prints progress first.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return s
}
