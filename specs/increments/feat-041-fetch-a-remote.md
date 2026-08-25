# feat-041: Track a repository over the network

**Traces:** R70, R71, R72

## What changed

A `Remote` source is now fetched instead of reporting that fetching is not built. Canon
mirrors it into a cache and ingests from there, so a repository on a host reads exactly
like one on disk.

```
$ canon catalogue -source https://github.com/ofenton/canon.git
1 source(s)
  https://github.com/ofenton/canon.git 1 found

  Canon                    6 open ·  58 done   6 error(s)     just now
```

## A mirror, not a checkout

`ingest.currentFile` reads through `git show HEAD:path` and never opens a working tree, so
a full clone would spend disk on files nothing ever reads. `git clone --mirror` gives
Canon everything it uses — refs, history, `remote.origin.url` — and no checkout. Canon's
own repository is 10MB mirrored.

This is asserted rather than assumed: `TestARemoteIsClonedAndReadsLikeAnyOther` fails if a
`specs/` directory appears in the cache. "It worked" would be equally true of a full clone
quietly costing every tracked repository its entire working tree.

## The cache is a cache

R72 says Canon holds nothing in its cache that it did not read from a source, and the only
version of that claim worth having is the one that deletes it:
`TestDeletingTheCacheLosesNothing` resolves a remote, ingests it, removes the cache
directory, resolves again, and compares `Repository.Fingerprint()`. It also asserts the
rebuilt path is identical, because the location is derived from the URL rather than
remembered.

The first draft of that test compared `git show` output — the bytes in the cache. That is
the wrong question. "The same catalogue" is a claim about what Canon *derives*, and
`Fingerprint` exists precisely to let two ingests of the same commit be compared, so the
test now goes through `ingest.Repo`.

Verified by hand as well as in the suite: fetched Canon's own repository from GitHub,
deleted the cache directory, fetched again, same result.

## Stale is not broken

`Result` already allowed `Paths` and `Err` together, and this is what that was for. When a
remote cannot be reached but a previous fetch is on disk, Canon reports the failure **and
serves what it read**.

That distinction had to reach the catalogue, so `Entry` gains `Stale` alongside `Err`:

- **`Err`** — there is nothing to show.
- **`Stale`** — what is shown was true at `RefreshedAt` and may not be now.

Collapsing them would mean a dashboard that goes blank when a host blips, which is worse
than one that keeps showing yesterday and says so. `TestAStaleSourceIsServedWithItsReason`
asserts the repository is still served, `Err` is empty, and the reason survives to where
the API can show it.

## Two failure modes handled before they happened

**A partial clone.** A clone that dies halfway leaves a directory that the next run would
find, treat as cached, and try to fetch from forever. `fetch` removes it on failure, and
`TestAFailedCloneLeavesNoWreckage` checks the cache is empty afterwards.

**A credential prompt.** `GIT_TERMINAL_PROMPT=0`, because a prompt in a background refresh
blocks until the timeout and looks, from outside, exactly like an unreachable host. Every
git call is bounded at two minutes for the same reason: a refresh that never returns leaves
the catalogue serving nothing.

## Not done

- **Credentials are git's.** SSH agent, credential helper, whatever git already has. Canon
  stores none and asks for none. A private repository works if `git clone` works.
- **Refresh is sequential.** Forty remotes on the timer are forty fetches one after
  another. Fine at the scale this is being used; the fix is obvious when it isn't.
- **Nothing prunes the cache.** A source removed from the list leaves its mirror on disk
  forever. Harmless, since deleting the cache is safe, but it is not tidied.

## Evidence

- `internal/source` — 13 tests, including a real clone over `file://`, deletion and
  rebuild, an unreachable origin, and a failed clone
- `internal/catalogue.TestAStaleSourceIsServedWithItsReason`
- Live: fetched `https://github.com/ofenton/canon.git`, 10MB mirror, deleted the cache,
  rebuilt identically
- `docs/architecture.md` — 33 invariants, every named test exists
