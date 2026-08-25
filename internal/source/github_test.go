package source

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// noNetwork is where githubAPI points unless a test has deliberately pointed it at a
// stub. Without this, a test that forgets to call org() reaches api.github.com — which is
// how TestOneBadSourceDoesNotHideTheGoodOnes was found making a real request against a
// real organisation, passing, and being slow for a reason nobody would have looked into.
const noNetwork = "http://127.0.0.1:1/tests-do-not-use-the-network"

func TestMain(m *testing.M) {
	githubAPI = noNetwork
	os.Exit(m.Run())
}

// org stands up a stub of the part of GitHub Canon uses.
//
// A stub rather than the real API: this is the one place Canon depends on somebody
// else's JSON, and a test that reached github.com would depend on a network, a rate
// limit and an organisation that keeps its shape. The clone URLs it hands back are
// file:// paths to real repositories, so everything downstream — fetching, mirroring,
// ingesting — runs for real.
func org(t *testing.T, withLedger, without []string) map[string]string {
	t.Helper()
	urls := map[string]string{}
	var repos []map[string]any

	for _, name := range withLedger {
		_, url := origin(t)
		urls[name] = url
		repos = append(repos, map[string]any{"full_name": "acme/" + name, "clone_url": url})
	}
	for _, name := range without {
		repos = append(repos, map[string]any{"full_name": "acme/" + name, "clone_url": "file:///nowhere"})
	}

	has := map[string]bool{}
	for _, name := range withLedger {
		has["acme/"+name] = true
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/orgs/acme/repos"):
			// One page: the stub returns everything, and fewer than perPage stops
			// the pager, which is the behaviour being relied on.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(repos)
		case strings.HasPrefix(r.URL.Path, "/repos/") && strings.HasSuffix(r.URL.Path, "/contents/specs/increment-plan.md"):
			name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/contents/specs/increment-plan.md")
			if !has[name] {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	githubAPI = srv.URL
	t.Cleanup(func() { githubAPI = noNetwork })
	return urls
}

// AC: WHEN a source names an organisation THE SYSTEM SHALL ingest every repository in it
// that contains the ledger.
//
// AC: WHEN a repository in the organisation does not contain the ledger THE SYSTEM SHALL
// skip it without reporting it as a failure.
func TestAnOrganisationExpandsToItsProducts(t *testing.T) {
	t.Setenv("GH_TOKEN", "test-token")
	org(t, []string{"orders", "billing"}, []string{"website", "dotfiles", "notes"})

	got := Resolve([]Source{{"github:acme", Organisation}}, t.TempDir())
	if got[0].Err != nil {
		t.Fatalf("expanding: %v", got[0].Err)
	}
	if len(got[0].Paths) != 2 {
		t.Fatalf("got %d repositories, want the two with a ledger: %v", len(got[0].Paths), got[0].Paths)
	}
	// The three without are not errors. Most of an organisation has not adopted the
	// template, and reporting each one would bury the sources that really failed.
	for _, p := range got[0].Paths {
		if strings.Contains(p, "website") || strings.Contains(p, "notes") {
			t.Errorf("%s has no ledger and should have been skipped", p)
		}
	}
}

// AC: WHEN no token is available THE SYSTEM SHALL say so for that source and continue
// with the others.
//
// Continue, not refuse: public repositories are legitimately readable without a token, so
// Canon reads what it can and names what it could not have seen. Paths and Err together,
// the same pairing a stale remote uses.
func TestWithoutATokenCanonReadsWhatItCanAndSaysSo(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	org(t, []string{"orders"}, nil)

	got := Resolve([]Source{{"github:acme", Organisation}}, t.TempDir())
	if len(got[0].Paths) != 1 {
		t.Fatalf("got %d repositories; the public ones must still be read", len(got[0].Paths))
	}
	if got[0].Err == nil {
		t.Fatal("a partial view must say it is partial")
	}
	if !strings.Contains(got[0].Err.Error(), "GH_TOKEN") {
		t.Errorf("the reason should say what to set: %v", got[0].Err)
	}
}

// One unreachable organisation must not take the rest of the list with it.
func TestAnOrganisationThatCannotBeReadIsReportedAlone(t *testing.T) {
	t.Setenv("GH_TOKEN", "test-token")
	root := t.TempDir()
	repo(t, root+"/orders")
	org(t, nil, nil)

	got := Resolve([]Source{{"github:nobody", Organisation}, {root, Directory}}, t.TempDir())
	if got[0].Err == nil {
		t.Fatal("an organisation that answers 404 must report")
	}
	if len(got[1].Paths) != 1 || got[1].Err != nil {
		t.Errorf("the local source was affected: %v %v", got[1].Paths, got[1].Err)
	}
}

// A refusal has to say the likely cause, because GitHub's status codes do not. A 404 is
// as often "you cannot see this" as "this is not there", and the difference is a token.
func TestARefusalNamesItsLikelyCause(t *testing.T) {
	for _, tc := range []struct {
		name, tok string
		status    int
		limit     string
		want      string
	}{
		{"404 without a token", "", http.StatusNotFound, "", "GH_TOKEN"},
		{"404 with a token", "t", http.StatusNotFound, "", "cannot see it"},
		{"rate limited", "", http.StatusForbidden, "0", "rate limited"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GH_TOKEN", tc.tok)
			t.Setenv("GITHUB_TOKEN", "")
			res := &http.Response{StatusCode: tc.status, Status: fmt.Sprint(tc.status), Header: http.Header{}}
			if tc.limit != "" {
				res.Header.Set("X-RateLimit-Remaining", tc.limit)
			}
			if err := describe(res); !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %q, want it to mention %q", err, tc.want)
			}
		})
	}
}
