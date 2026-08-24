# Contributing to Canon

Thank you for looking. Canon is small on purpose, and the fastest way to get a change merged is to
understand two things about how it is built.

## Canon is developed in increments

Work is tracked in [`specs/increment-plan.md`](specs/increment-plan.md) — the ledger — and each
increment gets a branch, a pull request, and a written record in `specs/increments/` explaining what
was decided and what was *not* verified. The repository is the source of truth; there is no separate
board.

You do not need to follow this for a small fix. Match the ceremony to the work:

| Your change | What to do |
|---|---|
| A typo, a broken link, an obvious one-line fix | Open a pull request. Put `Untracked: <reason>` in the commit message. |
| A change with a testable outcome | Add an increment to the ledger, then build it |
| A new capability, or anything that changes the schema's shape | Open an issue first — the design conversation is the valuable part |

Commits that belong to an increment carry `Increment: <id>` in the trailer. `canon trace` reports
how much work carries no reference, and the honest answer for this repository is about 8%. That is
the point of the number: work that genuinely needs no ticket is normal, and pretending otherwise is
what produces `NOJIRA`.

## What gets a change refused

Canon holds opinions, and some pull requests are declined on principle rather than on quality:

- **Story points, velocity, burndown, or any estimate.** Flow is measured from recorded transitions.
  `internal/metrics` refuses these field names by name, and a test asserts it.
- **Per-project or per-team configuration.** One `canon.yaml` for the organisation is the entire
  argument. There is no override, and adding one would not be a feature.
- **A runtime API for changing the schema.** Fields, states and types change by pull request against
  `canon.yaml`. A test asserts no such route exists.
- **A second way to do something that already works.** The query language, the permission verbs and
  the event types are all deliberately short, closed lists.

If you think one of these is wrong, that is worth an issue. It is not worth a surprise pull request.

## Before you open a pull request

```bash
make check          # vet, all tests, the workflow linter
make build
```

The web UI has an acceptance criterion that every action is reachable by keyboard, asserted by a
Playwright suite that never uses the mouse:

```bash
cd e2e && npm install && npx playwright install chromium-headless-shell
./bin/canon serve -addr :8099 &
node e2e/keyboard.mjs http://localhost:8099 <your-actor-id>
```

## Writing the change

Match the surrounding code. Two conventions matter more than the rest:

- **Comments explain why, not what.** Most comments here record a decision, a trade-off, or a defect
  that motivated the shape of the code. If the reasoning is obvious from the code, no comment.
- **Errors say what to do.** `field "storyPoints" is not defined in the schema; defined fields are
  component, evidence, priority, title` — not `invalid field`. Agents act on the first and retry
  blindly on the second.

## Reporting bugs

Include what you ran, what happened, and what you expected. If it involves a schema, include the
`canon.yaml`. `canon schema` and `canon usage` output are usually the fastest way to explain the
state of an instance.

For anything security-related, read [SECURITY.md](SECURITY.md) first — please do not open a public
issue.
