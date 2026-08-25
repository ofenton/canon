# ui-001: Every view has a URL

**Traces:** R64

## What this was for

Canon's whole claim is that it can answer questions no single repository can — what is
blocked across every product, how faithfully each follows the template. A tool that can
answer those and cannot send the answer to anybody has done half the job. Before this
increment, nothing in the interface touched `location` or `history`: every screen was
reachable only by arriving at the root and navigating again.

## What was built

Query parameters rather than paths. The UI is served at the root and the API under
`/api`, so a path-based router in the page would need the server to serve this file for
arbitrary paths — more moving parts than a nicer-looking URL is worth here.

One function writes the URL and one reads it, and one `navigate()` is the only way a view
changes. Every control — nav, the status filter, "blocked only", the pager, opening a
product — goes through it, so a control cannot be added that changes the screen and not
the address bar.

`pushState`, not `replaceState`, including for filters and paging: back after narrowing a
list should widen it again. `Escape` out of a product calls `history.back()` rather than
mutating state directly, so the address bar and the screen cannot disagree about where
you are.

A product detail carries none of the list's filters, because it applies none of them —
`?product=Canon&status=done` named state the view ignored.

## Acceptance criteria

- **WHEN a view is reached THE SYSTEM SHALL update the URL so that opening it reproduces
  the view** — `e2e/urls.mjs`. Tested in the strong form: the URL is copied and opened in
  a **fresh page**, and the restored view, filter and rows are compared. A URL that
  updates but does not restore is worse than no URL, because it looks shareable.
- **WHEN the browser back button is used THE SYSTEM SHALL return to the previous view** —
  three views, then back twice and forward once. A single back passes against a history
  that only ever replaces; a sequence does not.
- **WHEN a URL naming a product that does not exist is opened THE SYSTEM SHALL say so** —
  a blank state naming the product, plus a control back to the catalogue. This is the
  failure mode a linkable UI creates for itself: links outlive the things they name.

## Two tests that could not fail

Both found by trying to break them rather than by reading them.

**The filter check skipped itself.** `syncURL` runs before `render`, so the address bar is
correct a paint before the toolbar exists. The check waited for the URL and then asked
`if (await filter.count())` — which was 0, so the whole block was skipped and the suite
reported a pass. It now waits for the control. The pointer run in `e2e/keyboard.mjs` had
the same guard and the same silent pass; it is now unconditional.

**Mutation.** Changing `pushState` to `replaceState` fails the back-button check (times
out at the first `goBack`, non-zero exit). Adding a `sortBy` field to the state object
fails `TestEveryPieceOfViewStateIsInTheURL` on both halves.

## The structural guard

`TestEveryPieceOfViewStateIsInTheURL` parses the state initialiser and asserts each
shareable key appears in both `stateToParams` and `applyURL`. It is deliberately the
shape that fails when the *next* increment adds state: `ui-003` adds a search query, and
this test will refuse it until the query is in the URL. That is the reason ui-001 was
sequenced first rather than retrofitted three times.

`cursor` and `limit` are exempt and named in the test rather than inferred — the first is
a highlight that resets on every navigation, the second is fixed rather than chosen.

`TestNavigationNeverReloadsThePage` bans `location.href =`, `location.assign` and
`window.open`, and requires `pushState`/`replaceState`/`popstate` to still be present: a
full page load would discard the history everything above depends on.

## Not done

The e2e suite fails a mutation by throwing rather than by reporting a `FAIL` line, so a
broken build produces a stack trace and the checks after it never run. The exit code is
right and CI fails, which is what matters; the output is worse than it could be.
`e2e/keyboard.mjs` has always behaved this way.

## Evidence

- `e2e/urls.mjs` — 15 checks, all passing
- `e2e/keyboard.mjs` — keyboard-only and pointer-only runs, all passing
- `internal/ui` — 7 structural tests
- `docs/architecture.md` — two invariants added, `check-architecture.py`: 28 invariants,
  every named test exists
