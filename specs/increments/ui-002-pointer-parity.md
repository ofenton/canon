# ui-002: Pointer parity, and a narrow screen

**Traces:** R65, R67

## The judgement the plan flagged

`docs-006` recorded that this increment would have to define "action" before it could
claim parity, because a literal reading demands a button for `j` and `k` that nobody would
press. Here is the definition, and it is encoded rather than argued:

**Every action declares its pointer path in the registry, beside its key.** Most are
selectors for controls. Two are the content itself:

| | |
|---|---|
| `j`, `k` | `"row"` — a pointer does not move a selection step by step, it clicks the row it wants |
| `Enter` | `".title"` — the thing you would click to open something is its name |

An affordance added to satisfy a test rather than a person is worse than an honest
exception, so the exception is named in the registry, explained where it is declared, and
driven by the browser test as a click.

## What was added

Three controls the keyboard already had and the pointer did not: **help** (`?`),
**reload** (`r`) and **back** (`Escape`, shown only in a detail view). And the title now
opens on **one** click — double-click was the only way to open a product, and an
affordance nobody discovers is not one.

Pointer dispatch is now a loop over the registry rather than a hand-written listener per
control. Adding an action with a button is one entry, not an entry plus a listener that
can be forgotten.

## Parity is structural, not a list

`TestEveryActionHasAPointerPath` parses the registry and fails on any action without a
`pointer`. It also checks that a selector naming a control matches something that exists,
so a renamed id fails here rather than silently doing nothing.

That is the guard that matters: the previous state of this UI was keyboard-first with
pointer support added where someone thought of it, and *thinking of it* is what failed.
Mutation: removing `pointer` from `r` fails with `"r" has no pointer path, so it can only
be performed from a keyboard`.

## 400px

The table stops being a table. Each row becomes a block and each cell carries the column
name it was given when the row was built, so every column's content is visible.

The alternative — letting the table scroll inside its own box — would satisfy "the page
does not scroll sideways" while hiding half of every row behind a gesture nobody knows is
there. R67 says the interface remains **usable**, and that is not it.

Two things the tests passed but a screenshot caught:

- **"Conformance" was clipped.** The nav scrolled horizontally within itself, which no
  assertion covered. The keyboard hints beside each item are useless on a screen that is
  almost certainly being touched, so they are hidden below 46rem and all four items fit.
- **Titles wrapped right-aligned**, which is hard to read. The title cell now puts its
  label above the value rather than beside it.

Neither was a test failure. Looking at the thing remains necessary.

## I deleted my own work

Midway through, verifying the parity test could fail, I mutated `index.html`, confirmed
the failure, and ran `git checkout internal/ui/assets/index.html` to undo it. The file had
no committed version on this branch, so that discarded **every change in this increment**,
not the mutation.

Reapplied from the same edits. The habit that avoids it — copying the file before mutating
it and restoring from the copy — is the one I had used earlier in `ui-001` and dropped
here.

## Evidence

- `e2e/narrow.mjs` — 21 checks: every action driven by pointer only, four views at 400px,
  cells carrying their column names
- `e2e/keyboard.mjs` — keyboard-only and pointer-only runs still pass
- `e2e/urls.mjs` — unaffected
- `internal/ui` — 9 structural tests
- `docs/architecture.md` — 37 invariants, every named test exists
