# fix-002: Imported history carries real timestamps

## Context

Starting the instance up showed every flow metric as `0d`. I assumed a rounding
problem — `days()` rounded to two decimal places, so anything under about fifteen
minutes became zero — and that was real, but it was not the cause.

The cause was the importer. Every one of an increment's transitions carried the
moment the import ran:

```
2026-08-24T08:55:13  planned      -> approved
2026-08-24T08:55:13  approved     -> in_progress
2026-08-24T08:55:13  in_progress  -> in_review
2026-08-24T08:55:13  in_review    -> done
```

Cycle time was not rounding to zero. It **was** zero.

The script already gathered the right timestamps and threw them away, with an honest
comment saying why:

```python
# The event model supports a backdated timestamp — Event.At may precede
# the append, designed in feat-001 for exactly this — but no API accepts
# one, so imported history lands at import time ...
_ = stamps
```

feat-023 built that API. This is the increment that goes back and collects.

## Design notes

**The issue is created as of its first commit.** An issue's own events may not predate
it, so creating it at import time would make every replayed transition illegal. The
create is backdated first and the history then has room to happen.

**The route is spread across whatever commits exist.** An increment usually has more
commits than transitions and occasionally fewer, so `approved` lands near the first
commit and `done` near the last. It is an approximation and the docstring says so — a
truer one than stamping everything with `now`.

**A missing grant is reported, not worked around.** Backdating needs the `backdate`
role grant; without it every transition is refused and the import says so, rather than
quietly landing the whole history at import time, which is exactly the failure this
increment is fixing.

**Precision was a real second bug.** Two decimal places on a day is fifteen-minute
granularity, which would have reported a genuine eleven-minute cycle time as zero even
with correct timestamps. Four places resolve to about nine seconds. The unit stays
days because `p50_days` is in the API's field names and a client should not have to
know which release changed what it meant.

**Formatting belongs in the UI.** Days are what the API reports; hours and minutes are
what a reader acts on. A team shipping in an afternoon should not be told its cycle
time is `0d`.

## Evidence

**Verified by:** implementing session, `inc/fix-002-sub-day-durations`

### Before and after, same ledger, same repository

```
before:  cycle_time p50=0     p85=0      p95=0      max=0     (n=31)
after:   cycle_time p50=0.0074 p85=0.0483 p95=0.3755 max=0.473 (n=31)
```

Transitions now land where the work did:

```
  created 2026-08-24T08:26:42
  2026-08-24T08:26:42  planned      -> approved
  2026-08-24T08:26:42  approved     -> in_progress
  2026-08-24T08:26:42  in_progress  -> in_review
  2026-08-24T08:26:48  in_review    -> done
```

And the flow view reads in units somebody can act on:

```
Cycle time   p50 11m · p85 1.2h · p95 9h (n=31)
Lead time    p50 16m · p85 1.2h · p95 9.1h (n=31)
```

35 issues, 128 transitions, imported clean.

### Tests

`TestSubDayDurationsSurviveRounding` covers a day, twelve hours, an hour, fifteen
minutes and one minute, asserting none of them round to zero. Full suite green across
all ten packages.

### Scope

`git diff --cached --stat main` — run. `scripts/import-ledger.py` (backdated create
and transitions, `stamp_for`, `Client.call` taking `at`), `days()` precision, the UI
duration formatter, and one test.

### Not verified

**`stamp_for` is an approximation and cannot be otherwise.** The ledger records that an
increment reached `in_review`; it does not record when. Distributing the route across
an increment's commits is a defensible guess, and a squashed increment with two commits
maps four transitions onto two timestamps. Real cycle times going forward come from
transitions recorded as they happen; this only affects imported history.

**Nothing asserts the importer's behaviour in CI.** It is a script with no test, and
this increment made it materially more complicated. The evidence above is a manual
before-and-after against real data. A test would need a fixture repository and a
running server, which is why it does not have one — but that is a reason, not a
justification.
