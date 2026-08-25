# ui-003: Search across every product

**Traces:** R66

## What was built

One input, `/api/increments?q=`, matching across every product Canon has read. No
per-field controls: the fields come from somebody else's ledger, so a control per field
would be a control per convention Canon does not own — the accretion this product refuses.

Everything an increment carries is searched: id, title, status, type, the product's name,
acceptance criteria, traced requirements, and the field **values** the template does not
fix — scope, rollback plan, risk, evidence. Those are where the words people actually
remember live. A search that reads only titles finds nothing when somebody searches for
the thing they were worried about.

## Field labels are not searched, deliberately

`rollback` matches **nothing**, which looks like a bug and is not. Every increment has a
Rollback Plan, so matching the label would return all 64 — a result that answers no
question anybody asked. `revert` matches 6, because that is a word somebody wrote.

I found this by choosing `rollback` as the word for the browser test, watching it return 0,
and checking the API before assuming the search was broken.

## Three defects the tests found

**The table stole the caret.** `table()` ends by focusing the selected row, which is how
the keyboard stays usable across a re-render. Searching re-renders on every pause, so
every 250ms the caret was yanked out of the search box and into the results. Typing more
than one word was impossible. Now: whoever has the caret keeps it.

That is a genuine interaction bug, and neither the Go tests nor a glance at the screen
would have found it — it needed a browser test that types more than one keystroke.

**Restoring focus after `navigate()` does not work.** `render` is asynchronous, so
anything that refocuses immediately refocuses the input that is about to be replaced. The
caret is restored inside the toolbar builder instead, which is the only place that runs
after the new input exists.

**My own check was wrong first.** It sampled `document.activeElement` immediately after
the URL changed — during the render, when the caret is briefly gone by construction. It
now waits for the caret to come back, which distinguishes "never" from "not yet".

## Refining returns to the first page

Both halves. The UI resets `offset` when the query changes; the API reports the total it
actually found, so an offset past a narrowed result returns nothing rather than the
unfiltered page. A refined search holding an old offset reads as "no results" for a search
that has plenty.

## The guard ui-001 was sequenced to create

`TestEveryPieceOfViewStateIsInTheURL` parses the state object and demands every shareable
field appear in both `stateToParams` and `applyURL`. Adding `query` to the state without
putting it in the URL fails it. That is exactly what ui-001 was ordered first to
guarantee, and it worked without anybody having to remember R64.

## Evidence

- `internal/api` — 3 search tests, including eight query cases and the paging interaction
- `e2e/narrow.mjs` — searching narrows, the query is in the URL, the caret survives,
  refining resets the page, case is ignored, an empty result says what was searched
- All four browser suites and all nine packages pass
- `docs/architecture.md` — 39 invariants, every named test exists
