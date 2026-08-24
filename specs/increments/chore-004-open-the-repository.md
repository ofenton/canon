# chore-004: Open the repository

## Context

Open-sourcing was the plan from the first spec — "better could also mean cheaper" — and it had not
happened. The repository was private, which also meant branch protection was unavailable all week,
so every merge this project made went in unprotected.

## What an arriving stranger needs

**A changelog, because a breaking change already shipped.** `fix-003` renamed every JSON key to
snake_case and nothing recorded it. Two more breaking changes landed in `feat-031`: reads now require
authentication, and registry writes now require the `administer` permission. All three are in
`CHANGELOG.md` under Unreleased, written for somebody who has to fix their client, not for somebody
admiring the project.

**Contribution guidance that says what will be refused.** `CONTRIBUTING.md` states the four things
declined on principle — estimation fields, per-project configuration, a runtime schema API, and a
second way to do something that already works — because a contributor discovering those in review
has wasted their evening. It also gives permission to skip the increment workflow for small changes,
using the `Untracked:` trailer, since the alternative is a contributor inventing a placeholder.

**A private route for vulnerabilities.** `SECURITY.md` points at GitHub's advisory form and lists
the known limitations that are *not* vulnerabilities — tokens in query strings, no expiry,
all-or-nothing revocation, no rate limiting, the partial-migration window, an unencrypted log,
unsigned webhooks. Publishing that list is uncomfortable and it is the honest thing: a researcher
should not spend a weekend finding something already written down.

## The defect found while preparing to publish

Scanning history before making it public — because publishing is irreversible in practice, anything
public may already have been copied — turned up a **14MB compiled test binary, `api.test`, committed
in `feat-021` and carried for eleven increments**.

`check-tracked.py` exists precisely to catch this and did not, because it only fails when a *tracked*
file matches an ignore rule, and there was no rule for `*.test`. The check is only ever as good as
`.gitignore`. That is now stated in `.gitignore` itself, next to the new rule, so the next person
who wonders why the check passed has the answer where they are looking.

The same scan found no credentials, no keys, and no personal addresses beyond the GitHub noreply
address.

## Evidence

**Verified by:** implementing session, `inc/chore-004-open-source`

### The scan

```
secrets in history:      none
64-char hex in tracked:  none outside vendored node_modules
emails in history:       ofenton@users.noreply.github.com only
largest tracked file:    api.test, 14432 KB      ← removed
```

### Files added

```
CHANGELOG.md      3 breaking changes recorded, with what a client must do
CONTRIBUTING.md   the increment workflow, and what is refused on principle
SECURITY.md       private reporting, and the limitations that are not bugs
.gitignore        *.test, with the reason it was missing
```

### Not verified

**`api.test` is still in two historical commits.** Removing it from history needs `git filter-repo`
and a force-push, which the environment refused as a destructive operation — correctly, and I did
not work around it. The pack is 8.60 MiB either way, because the binary compresses well, so the cost
of leaving it is a slightly larger clone rather than anything structural. **This is a decision to
take before publishing, not after**: rewriting history is cheap while one person holds the only copy
and unpleasant once other people have clones.

**Branch protection is not yet configured**, because the repository is still private at the time of
writing this record. It is the last step and it needs the visibility change first.

**Nothing has been announced.** There is no release, no tag, and no version. `canon version` prints
a commit hash. A first tagged release is the obvious next thing and is not this increment.
