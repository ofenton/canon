# feat-011: Keyboard-first web UI

## Context

Flagged at Gate 1 as the increment most likely to overrun. Kept deliberately small: four views,
one action registry, no build step.

## Design notes

**The UI is mounted outside `Routes()`.** `Routes()` is the contract agents get — the MCP tool
list is derived from it, and a UI path in there would become a meaningless tool. Keeping them
apart also lets the "every route is under `/api`" test stay strict rather than carving out an
exception, and `APIHandler()` exists for callers that want the API alone.

**Every action lives in one registry.** The help dialog is generated from it, a test asserts no
action exists outside it, and the only click handlers permitted are on nav buttons that duplicate
a `g` shortcut. A pointer-only affordance is a bug here, not a gap.

**No build step, no external assets.** One HTML file with inline CSS and a module script,
embedded with `go:embed`. A self-hosted tool that fetches from a CDN is not self-hosted, and a
test greps for `http://`, `https://`, `<script src` and the usual CDN hosts.

**Refusals surface verbatim in the status bar.** The API's errors already say what to do; a UI
that replaces them with "Something went wrong" would discard the most useful thing the system
produces. `CANON-1 cannot move from "in_progress" to "done"; permitted transitions are …` is
shown as written.

**Proposals are a view, not a modal.** An agent's proposal is a queue a human works through, so
it gets a top-level destination and two keys — `a` approve, `x` reject.

## Bug found by the browser test

**`document.querySelectorAll("tbody tr")` also matched the help dialog's rows.** With 14 actions
in the registry, `j`/`k` navigation would have walked through the keyboard reference before
reaching any issue. The structural tests passed — a binding existed and was correct — and only
driving a real browser exposed it: the row count came back 15 when one issue existed.

Fixed by scoping selection to `#main`, with a test that fails on an unscoped selector so it
cannot come back. It is a good argument for the browser test earning its place rather than
relying on structural checks alone.

## Evidence

**Verified by:** implementing session, `inc/feat-011-ui`

### Every action reachable by keyboard without pointer input

Driven by Playwright with no mouse events at all:

```
PASS  ? opens keyboard help
PASS  help lists every action  — 14 rows
PASS  c opens create with the title focused  — focus=title
PASS  create asks for a title and nothing else  — 1 inputs
PASS  issue created by keyboard alone
PASS  j moves the selection  — selected=CANON-2
PASS  k moves it back  — selected=CANON-1
PASS  t transitions the selected issue
PASS  an illegal transition shows the schema's reason
      — CANON-1 cannot move from "in_progress" to "done"; permitted transitions …
PASS  / filters with a query
PASS  g m navigates  — Completed (30d) 0  In progress 1  Cycle time p5…
PASS  g p navigates  — No proposals awaiting a decision.
PASS  g i navigates  — Title State Team  CANON-2 Second issue todo
PASS  no uncaught exceptions

all keyboard checks passed
```

```
--- PASS: TestEveryActionHasAKeyboardBinding
--- PASS: TestSelectionIsScopedToMain
--- PASS: TestHelpIsGeneratedFromTheRegistry
```

The console check ignores the browser's "Failed to load resource" line for the 422 this run
deliberately provokes — that is an expected outcome the app handles, not a defect. Uncaught
exceptions still fail the run.

### The create shortcut opens a title-only field, focused

```
PASS  c opens create with the title focused  — focus=title
PASS  create asks for a title and nothing else  — 1 inputs
--- PASS: TestCreateDialogIsTitleOnlyAndFocused
```

The focus assertion matters: a shortcut that opens a dialog you then have to click into has moved
the work rather than removed it.

### Served from the binary with no separate asset deployment

```
--- PASS: TestServedFromTheBinary   (serves correctly from an empty working directory)
--- PASS: TestNoExternalAssets
```

### Scope

`git diff --cached --stat main` — run. The UI in `internal/ui`, its mount point in `api`, the
browser test in `e2e/`, and a CI job to run it. No README changes: documentation is `docs-003`
at the end of the week, per the process settled in docs-002.

### Not verified

The CI job has not run yet — it installs Playwright and a headless browser on `ubuntu-latest`,
which works locally on macOS but is unproven on that runner. It will be exercised by this pull
request.

No accessibility audit beyond keyboard reachability: `aria-selected`, `tabindex` and focus
management are in place, but screen readers have not been tested.

CI runs on the pull request.
