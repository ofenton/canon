# feat-029: Webhook on every transition

## Context

Canon has had no way to tell anything else that work moved. The last of the Should requirements, and
the most cuttable — which is why it was sequenced last.

## Design notes

**The write never waits.** This is the whole design constraint. A tracker whose transitions get
slower because an integration is having a bad day is a tracker people stop using, and an outbound
call on the write path is the most common way that happens. `Send` returns before the subscriber has
been contacted; delivery happens on a goroutine and cannot fail the transition.

**A failure is logged, not returned.** There is no answer a transition could give to "the subscriber
is down" except to carry on. The event is already in the log, and the log is the record. A webhook
is a courtesy.

**Retries are bounded, at five.** A subscriber down for nine retries is down and the tenth is not
information. An unbounded retry against something decommissioned is a queue that grows for ever, and
the usual outcome is somebody finding it months later having filled a disk. The final failure logs
everything needed to replay it by hand.

**Concurrency is bounded, at eight.** An instance transitioning a thousand issues during a migration
must not open a thousand sockets to somebody else's server.

**Webhooks live in `canon.yaml`, with no runtime API.** Where notifications go is an organisational
decision. A tracker that lets any team point a firehose of state changes at any URL is how an
integration nobody remembers configuring ends up leaking work items to a defunct vendor. Adding one
is a pull request, like everything else.

**A webhook watching an undefined state is refused at load**, the same failure mode as a role
granting a misspelled field: it would fire never, and silently.

**A nil `Sender` is a working `Sender` that does nothing.** A schema with no webhooks produces nil,
and nil sends nothing. Otherwise every call site needs a check and one of them eventually will not
have it.

## Evidence

**Verified by:** implementing session, `inc/feat-029-webhooks`

### A real subscriber, a real transition

Two webhooks configured: a live listener, and a port with nothing on it.

```
  transition -> in_progress:
  0.012 total

  SUBSCRIBER GOT  CANON-1 todo -> in_progress  by ollie (human)
```

The transition returned in **12ms** while one subscriber received the delivery and a second, dead
endpoint was being retried in the background. That is the property this increment exists to have.

### Tests

Six in `internal/webhook`: delivery contents and provenance; retries counted exactly (one attempt
plus two retries, then stop); `Send` returning against a subscriber that never answers; an
unreachable host surviving; `states:` narrowing delivery; and the nil sender.

Five in `internal/schema`: valid load, unknown state refused by name, URL must be absolute http or
https, retries bounded, and configuration-only.

Two in `internal/api`, end to end over HTTP, because the wiring is where this would silently not
happen: a real transition through the real API reaching a real subscriber, and a slow subscriber not
slowing the write.

Full suite green across eleven packages.

### Scope

`git diff --cached --stat main` — run. `internal/webhook`, `schema.Webhook` and its validation, the
`webhooks` key, the notify call after a successful transition, wiring in `canon serve` with a
bounded shutdown wait, thirteen tests, and the README.

### Not verified

**No signing.** A subscriber cannot verify a delivery came from Canon. For an internal endpoint on a
private network that is survivable; for anything else it is not, and an HMAC over the body with a
shared secret is the obvious next step. Left out because a secret in `canon.yaml` needs the
env-var indirection that does not exist yet, and half a security feature is worse than none.

**Deliveries are lost on restart.** They are in memory, and `Close` waits five seconds. A subscriber
that is down while Canon restarts misses those transitions permanently. The honest fix is a delivery
queue in the log itself, which is a much larger increment; the honest mitigation today is that the
log holds the truth and a subscriber can replay from `/api/events`.

**Only transitions fire.** Not creation, field changes, dependencies or checklist items. R24 asked
for transitions and that is what this does, but a subscriber wanting "tell me when anything happens"
cannot have it.

**No per-webhook filtering beyond state.** No filtering by team, by issue type, or by actor kind —
so a subscriber wanting only agent activity receives everything and filters at their end.
