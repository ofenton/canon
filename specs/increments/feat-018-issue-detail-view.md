# feat-018: Issue detail view showing hierarchy and dependencies

## Context

Four increments of relationships — hierarchy (feat-005, feat-017), dependencies (feat-016), typed
nesting (feat-020, feat-021) — existed only in the API and MCP. Pressing Enter on an issue wrote
its title to the status bar. This is where all of it becomes visible.

## Design notes

**A view, not a modal.** Relationships are the point of this screen, and you follow one by
pressing Enter on it. A dialog would make "go to what is blocking this" mean closing and
reopening, which is the wrong shape for something you walk through.

**Blocking and cycles are stated first, above the fields.** They are the reason the work is not
moving, and someone opening an issue to find out why should not have to read past its priority to
get there.

**Related issues are focusable list items.** That is what makes the graph walkable by keyboard
rather than merely displayed: `j`/`k` move through parents, children, dependencies and dependents
alike, and Enter opens whichever is selected.

**The breadcrumb reads root-first.** `E › F › S › T` is how people say it aloud, so the
nearest-first order the API returns is reversed for display.

**`/api/schema` now returns `parent_types` and `child_types` per issue type.** The parent prompt
says "must be story" before you type, rather than refusing after. A client that has to guess and
retry is one the rules were not really shared with.

## Three instances of the same bug, and a test that now catches all of them

`renderIssues` writing a list summary to the status bar wiped the refusal an action had just
reported — found and fixed in feat-012. Building this screen reintroduced it twice more:
`renderDetail` wrote key hints there, and `renderProposals` wrote `a approve · x reject`.
`renderBoards` had it all along.

The structural test caught `renderDetail` immediately, then failed to catch `renderProposals`
because it extracted "everything between renderIssues and renderBoards" — a boundary that broke
the moment a function was inserted between them. It now matches each function by brace depth and
checks all five. All hints and summaries live in the view; the status bar reports the last action
and nothing else.

That is three times for one mistake, which says the original fix was too narrow: it fixed an
instance rather than the class.

## Evidence

**Verified by:** implementing session, `inc/feat-018-detail-view`

### Fields, hierarchy and dependencies, without leaving the keyboard

A story nested four deep, blocked, with children and a dependent:

```
E › F › S
Reindex on write
Blocked — waiting on T

DETAIL      Type story · State todo · Team platform · Last touched ollie · priority p1
PARENT      F
CHILDREN    B, T
WAITS ON    T
WAITED ON BY  nothing

Esc back · p parent · d dependency · t transition · Enter follow
```

Driven by Playwright with no mouse events:

```
PASS  CANON-1 is the selected row
PASS  Enter opens the detail view
PASS  the detail view shows fields         — State
PASS  the detail view shows hierarchy      — Children
PASS  the detail view shows dependencies   — Waits on
PASS  the detail view shows reverse        — Waited on by
PASS  Escape returns to the list
```

### A blocked issue says so and names the blocker

```
PASS  a blocked issue says so and names the blocker  — Blocked — waiting on CANON-2
```

### A dependency cycle is shown

Created by keyboard from within the view, then displayed on the issue:

```
PASS  a dependency cycle is shown on the issue
      — Dependency cycle — CANON-1 → CANON-2 → CANON-1. Nothing in this cycle …
```

### Navigating to a related issue without a pointer

```
PASS  Enter on a related issue navigates to it  — CANON-2
```

All 20 keyboard checks pass, and the structural suite still holds:

```
--- PASS: TestEveryActionHasAKeyboardBinding
--- PASS: TestRenderFunctionsDoNotClobberTheStatusBar
--- PASS: TestSelectionIsScopedToMain
```

### Scope

`git diff --cached --stat main` — run. The view and its actions in `internal/ui`, the nesting
rules added to `/api/schema`, the browser test extended, and the status-bar test generalised.

The `/api/schema` addition is outside the stated scope. Without it the parent prompt cannot say
what is permitted, and a prompt that can only tell you after you are wrong is not much of a prompt.

### Not verified

No editing of fields from the detail view: transitions, parent and dependencies are reachable, but
changing a priority still needs the API. Nobody has asked, and adding an editor is where a detail
view becomes a form.

Two of my own demo scripts had bugs during this increment — a stale filter selecting the wrong
issue, and an epic sent a field its type does not declare. Both were caught by the product
behaving correctly, which is a reasonable sign, but they are the sort of thing that would have
been reported as product bugs if I had not checked.

CI runs on the pull request.
