package source

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ofenton/canon/internal/ingest"
)

// githubAPI is the API root. A variable so a test can point it at a stub: this is the
// one place in Canon that depends on somebody else's JSON, and testing it against the
// real thing would make the suite depend on a network and a rate limit.
var githubAPI = "https://api.github.com"

// checkers bounds how many repositories are examined at once.
//
// An organisation with two hundred repositories needs two hundred requests to find out
// which follow the template, and doing that one at a time is a minute of startup. Eight
// is polite to the host and fast enough that nobody notices.
const checkers = 8

// perPage is GitHub's maximum. Fewer pages is fewer round trips.
const perPage = 100

// token reads a credential from the environment.
//
// Canon stores none and asks for none. GH_TOKEN and GITHUB_TOKEN are what the GitHub CLI
// and Actions already set, so on most machines this works without anybody configuring
// anything — which is the point.
func token() string {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

// repository is the part of GitHub's answer Canon uses.
type repository struct {
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
	Archived bool   `json:"archived"`
}

// expandOrg lists the repositories in an organisation that follow the template.
//
// Returns clone URLs, not paths: expansion answers "which repositories", and fetching
// them is the same code path an explicitly listed repository takes. Keeping those
// separate is what stops this becoming a second way to get a repository on disk.
//
// The returned error may accompany results. A missing token is the case that matters:
// public repositories are legitimately readable without one, so Canon reads what it can
// and says what it could not have seen, rather than refusing the source outright.
func expandOrg(org string) ([]string, error) {
	repos, err := listRepos(org)
	if err != nil {
		return nil, err
	}

	urls := make([]string, 0, len(repos))
	var mu sync.Mutex
	var wg sync.WaitGroup
	work := make(chan repository)

	for range checkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range work {
				// A repository without the ledger is not a failure. Most of an
				// organisation has not adopted the template, and reporting each one
				// would bury the sources that genuinely could not be read.
				if !hasLedger(r.FullName) {
					continue
				}
				mu.Lock()
				urls = append(urls, r.CloneURL)
				mu.Unlock()
			}
		}()
	}
	for _, r := range repos {
		work <- r
	}
	close(work)
	wg.Wait()

	// Stable order, because the catalogue is keyed by path and a shuffling list would
	// make two identical runs look different.
	sort.Strings(urls)
	if token() == "" {
		return urls, fmt.Errorf("read %s without a token, so private repositories are not listed; set GH_TOKEN", org)
	}
	return urls, nil
}

// listRepos pages through an organisation, falling back to a user account.
//
// Falling back rather than asking the caller to say which: `github:ofenton` is what
// somebody writes, and whether that name is an organisation or a person is GitHub's
// distinction, not theirs.
func listRepos(org string) ([]repository, error) {
	var all []repository
	for _, kind := range []string{"orgs", "users"} {
		all = nil
		var failed error
		for page := 1; ; page++ {
			batch, err := get[[]repository](fmt.Sprintf("%s/%s/%s/repos?per_page=%d&page=%d",
				githubAPI, kind, org, perPage, page))
			if err != nil {
				failed = err
				break
			}
			all = append(all, batch...)
			if len(batch) < perPage {
				break
			}
		}
		if failed == nil {
			return all, nil
		}
		if kind == "users" {
			return nil, failed
		}
	}
	return all, nil
}

// hasLedger reports whether a repository carries the artifact, without cloning it.
//
// One request against the contents endpoint. Cloning an entire organisation to find out
// which tenth of it follows the template would be the obvious alternative and a much
// worse one.
func hasLedger(fullName string) bool {
	req, err := http.NewRequest(http.MethodHead,
		fmt.Sprintf("%s/repos/%s/contents/%s", githubAPI, fullName, ingest.LedgerPath), nil)
	if err != nil {
		return false
	}
	res, err := send(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

// get fetches and decodes one page.
func get[T any](url string) (T, error) {
	var zero T
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return zero, err
	}
	res, err := send(req)
	if err != nil {
		return zero, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return zero, describe(res)
	}
	var out T
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return zero, fmt.Errorf("reading the answer from %s: %w", url, err)
	}
	return out, nil
}

// send adds what every request needs and performs it.
func send(req *http.Request) (*http.Response, error) {
	req.Header.Set("Accept", "application/vnd.github+json")
	if t := token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

// describe turns a refusal into something worth reading.
//
// GitHub's status codes are ambiguous on their own — a 404 is as likely to mean "you
// cannot see this" as "this does not exist — so the likely cause is named alongside it.
func describe(res *http.Response) error {
	switch res.StatusCode {
	case http.StatusNotFound:
		if token() == "" {
			return fmt.Errorf("not found, or private and no token is set (GH_TOKEN)")
		}
		return fmt.Errorf("not found, or the token cannot see it")
	case http.StatusUnauthorized, http.StatusForbidden:
		if res.Header.Get("X-RateLimit-Remaining") == "0" {
			return fmt.Errorf("rate limited by GitHub; a token raises the limit (GH_TOKEN)")
		}
		return fmt.Errorf("refused by GitHub (%s)", res.Status)
	default:
		return fmt.Errorf("GitHub answered %s", res.Status)
	}
}
