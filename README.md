# Canon

Point it at your repositories and see what every team is building.

Canon reads repositories that follow the [agentic SDLC template](https://github.com/ofenton/canon)
and derives everything it shows: what products exist, what is in flight, how long work actually
takes, and which repositories have drifted from the convention.

**It authors nothing.** A repository owns its own work; Canon reads it.

Apache-2.0. Self-hosted. One static binary, **no dependencies at all** — `go.mod` is three lines —
no database, and nothing to configure.

## The problem

Work already happens in repositories. Agents plan it, build it and record it there, in a spec, a
ledger and a commit history that is precise about who changed what and when.

Nothing shows it together. Ask *"what is every team building, and how long is it taking"* and the
answer is somebody opening twelve repositories — or a second tracker typed into by hand, which
disagrees with the first by Wednesday.

That second tracker is the common answer and the wrong one. It creates two sources of truth for the
same facts, with no reconciliation beyond somebody remembering.

We know because we built one. Canon began as a Jira replacement, and **96% of the data it held was
reconstructible from the repositories it tracked**. A thing you can rebuild from git is a cache of
git. See [ADR-0009](docs/decisions/0009-canon-as-aggregator.md).

## Quick start

Requires Go 1.26+ to build. Nothing else.

```bash
git clone https://github.com/ofenton/canon.git && cd canon
make build

./bin/canon catalogue ~/code      # what products are there?
./bin/canon serve -source ~/code
```

A product is any repository containing `specs/increment-plan.md`. There is nothing to register and
nothing to configure: **adopting Canon is committing that file.**

Where Canon looks is a list of **sources** — places, not repositories to register:

```
# canon.sources
~/code                            a local directory, scanned one level deep
git@github.com:ofenton/orders     one repository, fetched
github:ofenton                    every repository in an organisation that has the ledger
```

One line per source, `#` for comments, and nothing else — the list says what Canon reads, never how
it behaves. Pass it with `-sources <file>`, name places directly with `-source` (repeatable), or
leave both off and Canon reads `canon.sources` if it is there and the working directory if it is
not. Fetching and organisation expansion are not built yet; those lines report what to do instead.
## What it tells you

```bash
$ canon flow ~/code/widgets

Widgets — last 30 days
  completed 45 · started 46 · in progress 2

  cycle time     p50 14m  · p85 1.2h  · p95 9.1h   (n=44)
  lead time      p50 3.1h · p85 15.1h · p95 17.2h  (n=45)
```

```bash
$ canon conform ~/code/widgets

  warning  —  cycle time understates the work: 6 of 26 increments record
              in-progress within 2m0s of in-review, so it measures two
              commits rather than the work. Set in-progress before
              starting, not alongside the result
  note     —  9 of 143 commits (6%) carry no increment reference
```

That warning is the point of a central view: it is a property of *how a team runs the loop*, not of
any one commit, so no repository-local check can see it.

## Where the numbers come from

The template requires that every status change is a commit. So `git log -p specs/increment-plan.md`
**is** the transition log — exact, with no heuristic:

```
feat-025
  09:18:37  (new)     -> approved
  10:26:42  approved  -> in-review
  10:26:48  in-review -> done
```

The one honest limit: two status changes in one commit share a timestamp. That is exact about what
git recorded, which is the most any reader can claim.

## Opinions Canon holds

- **It derives, never authors.** Anything typed into Canon that became the truth would recreate the
  two-sources problem it exists to remove.
- **Enforcement lives at the edge.** An aggregator cannot refuse a commit that already happened.
  `validate-plan.py` refuses in each repository's hook and CI, where refusing works. Canon runs the
  same rules everywhere and reports who is failing them.
- **The schema is the template, and is not configurable.** A schema with no configuration cannot
  drift.
- **No estimation.** No story points, velocity or burndown, and a test that parses the source to
  keep it that way. Flow is measured from transitions that were committed anyway.
- **Every action works by keyboard and by pointer**, asserted by two browser runs: one that sends no
  clicks, one that sends no keys.

## Agents

```bash
canon mcp -source ~/code
```

The same reads, over MCP. Tools are *derived* from the HTTP route table, and a test asserts parity —
so an agent can never be offered a surface that lags the one humans get.

## What is not built

- **Remote discovery.** `Discover` reads a local directory; pointing Canon at a GitHub organisation
  is not implemented.
- **Intake.** Raising a request through Canon, as a pull request against a repository's ledger, is
  designed ([ADR-0009](docs/decisions/0009-canon-as-aggregator.md)) and not built.
- **Work with no repository.** Support and operations tickets have no home here. This is the largest
  capability the reframe gave up.
- **Incremental refresh.** Each refresh re-reads every repository.

## Development

```bash
make check     # vet, tests, the workflow linter, the architecture check
make build
```

See [`docs/architecture.md`](docs/architecture.md) for the map, and
[`CONTRIBUTING.md`](CONTRIBUTING.md) before opening a pull request.
