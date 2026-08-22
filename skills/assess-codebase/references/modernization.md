# Modernization lens

## Contents
- The question this lens answers
- Establish current-state facts
- The six options
- Behavioral deltas
- Sequencing
- Rating severity

## The question this lens answers

**"What would it cost to move this, and what breaks if we do?"** Not "is this old". Age is not
a defect; a stable system that nobody needs to change may be correctly left alone. The finding
is always a cost, a risk, or a blocked capability.

## Establish current-state facts

Before recommending anything, write down what is actually true:

- Languages, runtimes and framework versions, and which are out of support
- Where it runs, and how it is deployed
- What it integrates with, in both directions
- Data stores, their schemas, and their size
- What tests exist — this determines whether any migration is safe at all
- Who still knows it, from `git log`
- The compliance and data-residency constraints it operates under

Two facts dominate everything else: **test coverage of the behaviour you must preserve**, and
**whether the behaviour is documented anywhere other than the code**. Without either, every
option below gets substantially more expensive, and saying so is more valuable than a
recommendation.

## The six options

For each component, consider all six before recommending. The bias toward rewriting is strong
and usually wrong.

| Option | Means | Fits when |
|---|---|---|
| **Retain** | Leave it; revisit later | Stable, low change rate, no blocked capability |
| **Rehost** | Move as-is (lift and shift) | Infrastructure is the problem, code is not |
| **Replatform** | Minor changes for a new platform | Managed services remove real toil |
| **Refactor** | Restructure incrementally, same behaviour | Code is the problem and tests exist |
| **Rewrite** | Rebuild from the requirements | Behaviour is genuinely wanted-different, or the stack is a dead end |
| **Replace** | Buy or adopt something else | It is not differentiating |

State the cost of each option you rejected. A recommendation without the rejected alternatives
is not reviewable.

## Behavioral deltas

The defining risk of modernization is not that the new thing fails — it is that the new thing
works differently in ways nobody noticed. Enumerate them explicitly:

- Rounding, precision, date and timezone handling
- Sort order, collation, and case sensitivity
- Null and empty-string semantics
- Error messages and status codes that callers depend on
- Timeout and retry behaviour
- Anything an integration parses out of a response

Each intentional difference goes in its increment's **Behavioral Deltas** field. Each
unintentional one is a defect. The distinction only exists if you wrote the list down first.

## Sequencing

Prefer strangling over cutovers: run new alongside old, move traffic incrementally, keep the
rollback live until confidence is earned. Recommend the seams that make this possible — an
anti-corruption layer, a routing point, a dual-write window — as their own early increments.

A big-bang cutover is sometimes right, but only when you can say precisely why incremental
is impossible.

## Rating severity

| Severity | Test |
|---|---|
| Critical | Out of support with no security patches, or a hard end-of-life date |
| High | Blocks a needed capability, or the knowledge is down to one person |
| Medium | Rising cost or friction, no deadline |
| Low | Would be nice, no forcing function |

For each finding give a rough order of magnitude — days, weeks, or quarters — and say what
would sharpen the estimate. An unqualified estimate will be quoted back as a commitment.
