# feat-042: Discover an organisation

**Traces:** R52, R70

## What changed

`github:ofenton` now asks GitHub which of its repositories carry the ledger and tracks
those. This is what R52 has asked for since the reframe, and it is the point at which the
list stops naming repositories and starts naming **places**:

```
$ canon catalogue -source github:ofenton
  github:ofenton               1 found
  Canon                    5 open ·  59 done   6 error(s)     just now
```

Nothing was added to a list to make Canon see that repository. It appears because it
committed `specs/increment-plan.md`, which is the adoption story ADR-0009 chose and the
first time it has actually been true.

## Expansion answers "which", not "how"

`expandOrg` returns clone URLs, not paths. Each one then goes through the same `fetch`
that an explicitly listed repository takes. Keeping those separate is what stops
organisation expansion becoming a second way to get a repository onto disk, with its own
caching and its own bugs.

## Skipping is not failing

Most of an organisation has not adopted the template. `hasLedger` checks each repository
with one request against the contents endpoint, and a repository without the artifact is
**skipped silently** — reporting each one would bury the sources that genuinely could not
be read, which is the thing R71 exists to make visible.

One request per repository, eight at a time. Sequentially, a two-hundred-repository
organisation would be a minute of startup; the alternative of cloning everything to find
out which tenth follows the template is worse in every direction. `ofenton` expands in
2.1 seconds.

## No token is a partial answer, not a refusal

Public repositories are legitimately readable without a token, so Canon reads what it can
and names what it could not have seen:

```
github:ofenton               1 found, and: read ofenton without a token, so private
                             repositories are not listed; set GH_TOKEN
```

`Paths` and `Err` together — the same pairing `feat-041` introduced for a stale remote,
and the reason `Entry.Stale` exists. Refusing the source outright would be the literal
reading of the acceptance criterion and the wrong behaviour.

`GH_TOKEN` and `GITHUB_TOKEN` are what the GitHub CLI and Actions already set, so on most
machines this works without anybody configuring anything. Canon stores no credential and
asks for none.

## A refusal says its likely cause

GitHub's status codes are ambiguous on their own: a 404 is as often "you cannot see this"
as "this is not there", and the difference is usually a token. `describe` names the likely
cause, and `TestARefusalNamesItsLikelyCause` covers the three that matter — 404 with and
without a token, and a rate limit, which is a 403 distinguishable only by a header.

## A test was quietly using the network

`TestOneBadSourceDoesNotHideTheGoodOnes` listed `github:ofenton` as an example of an
unbuilt source kind. Once the kind was built, that test **made a real request to
api.github.com against a real organisation** — and passed. It was 1.7 seconds of the
suite's runtime for a reason nobody would have investigated.

Fixed at the root rather than in that one test: `TestMain` points `githubAPI` at a dead
port, and only a test that deliberately stands up a stub can reach anything. The package's
tests now run in 1.0 seconds instead of 2.7.

The stub returns `file://` clone URLs to real repositories, so fetching, mirroring and
ingesting all run for real — the only thing faked is GitHub's JSON.

## Verified by hand

- `github:ofenton` with no token: found Canon, skipped the rest, said what it could not see
- with `GH_TOKEN`: found Canon, no warning
- both against the real API, which the suite deliberately cannot do

## Not done

- **GitHub only.** The `github:` prefix exists so `gitlab:` can be added beside it; nothing
  else assumes a host.
- **Forks are included** if they carry a ledger. A fork of a template repository would
  appear as a product. Not seen in practice, and guessing at intent seemed worse than
  showing what is there.
- **Archived repositories are included.** `repository.Archived` is read and unused — an
  archived product is still a product, and its ledger is still a record.
- **No pagination beyond what GitHub returns.** The pager stops on a short page, which is
  correct, but has only been exercised against a single page.

## Evidence

- `internal/source` — 17 tests; organisation expansion, skipping, the token case, a
  refusal's cause, and the network guard
- Live against `github:ofenton`, with and without a token
- `docs/architecture.md` — 35 invariants, every named test exists
