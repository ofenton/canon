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

### The history rewrite, and a wrong recommendation

I recommended publishing with `api.test` left in history, on the reasoning that the pack was
8.60 MiB either way because the binary compresses well. **That was wrong, and I had not measured
it.** Asked to rewrite anyway:

```
pack before:  8.67 MiB
pack after:   650.66 KiB
```

Thirteen times smaller. The number I quoted was the pack *after* removing the file from HEAD only,
and I assumed the rest rather than checking. A fresh clone is now 651 KiB.

The rewrite was done in a fresh clone rather than in place, verified before force-pushing: 159
commits before and after, identical file lists, identical content hashes for `internal/api/api.go`,
`README.md`, `CHANGELOG.md` and `specs/increment-plan.md`, and all eleven packages passing.

### Branch protection, and a second wrong call

Protection was configured with `enforce_admins: false`, and a probe commit pushed straight to main
and was accepted. For a repository with one maintainer that setting makes protection decorative —
the only person who commits is exactly the person exempted.

With `enforce_admins: true`:

```
remote: error: GH006: Protected branch update failed for refs/heads/main.
remote: - 3 of 3 required status checks are expected.
```

Now it refuses everybody, including the owner. Three checks required: `build`, `ui`, `ledger`;
force-pushes and deletions disabled.

### Not verified

**An empty probe commit is on main**, titled "probe: this should be refused". It was created to test
protection while `enforce_admins` was still false, and cannot be reverted — there is nothing in it —
nor force-pushed away, now that protection correctly forbids that. It is noise in the history and the
honest record of how protection was verified. Removing it would mean relaxing the protection that
was just proved to work.

**No release or tag.** `canon version` prints a commit hash. A first tagged release, with a built
binary attached, is the obvious next step and is not this increment.

**Nothing has been announced.** There is no release, no tag, and no version. `canon version` prints
a commit hash. A first tagged release is the obvious next thing and is not this increment.
