# feat-031: Authenticate the actor

## Context

Canon authorised without authenticating. `X-Canon-Actor` was taken at face value, which was honest
for a single-tenant instance on a trusted network and dishonest for anything else — and it is what
kept the README saying *do not expose this to a network you do not control*.

This was deferred deliberately in the original spec, on the reasoning that enforcement takes a
`Principal` and how a Principal is constructed is nobody else's business. That reasoning held:
`Verify` is the only new seam, and no authorisation code changed.

## Why Canon issues its own tokens

The alternative considered was delegating to a provider — Cognito was the specific suggestion. It
was rejected because it contradicts two commitments already made. Canon promises *one static binary,
one file of data, no external services*, and self-hosting is the cheaper-than-Jira argument; a
tracker that cannot start without an AWS account is not self-hosted. And an open-source tool that
requires one vendor's identity service is not the vendor-neutral thing this was meant to be.

So Canon verifies identity and never owns it long-term. `enforce.Verify` is one function: replacing
it with one that trusts a signed OIDC claim from Cognito, Entra, Okta or Keycloak changes that
function and nothing above it. Built-in tokens are the default that works with nothing installed,
not the only thing that can ever work.

## Design notes

**SHA-256, not bcrypt or argon2.** A slow KDF exists to defend low-entropy secrets people chose
themselves; these are 256 random bits with no dictionary to attack, so a KDF would buy nothing and
make authentication the slowest part of every read. It matters here more than usual because the log
is append-only: anything written is permanent, in a file people back up and copy around.

**Shown once.** A tracker that can tell you an existing token is a tracker whose database hands out
credentials.

**Revocation withdraws every token an actor holds.** Somebody revoking is responding to a suspected
leak and does not know which token leaked. Rotation is revoke then issue — the same two steps and
one fewer concept than per-token revocation.

**Constant-time comparison** against every candidate hash. Returning on the first mismatch would
leak, through timing, how much of a guess was right.

**Tokens carry a `canon_` prefix**, so a leaked one is recognisable to secret scanners.

**`canon token` is local, like `canon bootstrap`.** Needing a token to get a token is a locked door
with the key inside. It requires filesystem access to the log, which is a stronger check than any
role.

## The two defects this found

### Any actor could grant itself any role

Verified live against a running instance before writing a line of the fix:

```
mallory is a reporter. Now, as mallory:
  grant self admin -> 204
  mallory now holds: ['admin', 'reporter']
```

The API resolved a principal for every registry write and then never checked its authority. This
has been open since feat-015, and it makes authentication pointless on its own — proving who you
are does not help if anyone can become an administrator. Every other permission in the schema was
decoration.

`administer` is now a verb in the closed list, required for registering actors, granting and
revoking roles, team membership, and issuing tokens for somebody else. It is deliberately
**unscoped by team**: administration is org-wide, and a team-scoped role reaching it would let a
team mint its own administrators.

```
{"error":"mallory holds role(s) reporter, which do not permit \"administer\" on mallory;
          roles that would permit it: admin"}
```

### No read route authenticated at all

Found by the first boundary test: `GET /api/issues` returned 200 to an unauthenticated caller.
Every *write* called `principal()`; no read did. An unauthenticated caller could read the entire
tracker.

The fix is structural rather than another twenty call sites. Authentication is now middleware
wrapping every `/api/` route, and `principal()` reads what it resolved from the request context. A
per-handler check is a check somebody forgets — demonstrated by the fact that somebody did, in
every read handler, for six increments.

## Migration, and its honest cost

Authentication turns on **per actor**, not per instance. An actor holding a token must present one;
an actor holding none is trusted as before.

A global switch was the first implementation, and a test caught what it does: issuing the first
token locks out every administrator who has not yet issued themselves one. Per-actor means an
operator can onboard progressively.

The cost is real: until every actor has a token, a caller can still claim to be one of the
stragglers. That is not a regression — it is exactly the old behaviour, for exactly the actors
nobody has onboarded — but it must not be mistaken for done, so both `canon token` and `canon serve`
name who is still claimable:

```
  auth   PARTIAL — still claimable without a token: mallory
```

## Evidence

**Verified by:** implementing session, `inc/feat-031-authentication`

### The boundary, live

```
$ canon token -actor ollie
  canon_X1k6udlYjidoADYCYSdgOVnQPk_qEjAw553RdcoYrjE
This is the only time it is shown — Canon stores a hash, not the token.

Still claimable without proof: mallory
Issue tokens for those too, or they remain impersonable.

claiming ollie:  {"error":"ollie holds a token, so a claimed identity is not accepted;
                  send Authorization: Bearer <token>"}
with the token:  {"issues":[],"limit":200,"offset":0,"total":0}
claiming mallory (no token yet): 200
```

### The web UI

```
without a token: ci holds a token, so a claimed identity is not accepted; send Authorization: Bearer
with a token   : 3 issues Title State Team CANON-1 Search is slow in_progress
address bar    : ?actor=ci
```

The token is accepted from `?token=`, kept in memory, and stripped from the address bar so it does
not sit in browser history. All 27 keyboard checks pass with authentication off.

### Tests

Nine in `internal/enforce`: identification, the log holding no token, revocation withdrawing all
tokens, rotation, unknown and empty tokens, the off-until-first-token rule, unknown actors, that
only an administrator may administer, and that administration is not team-scoped.

Seven in `internal/api`: claims refused once a token exists (reads *and* writes), the token
overriding a contradicting header, revocation over HTTP, the self-escalation attempt, every registry
write gated, self-rotation allowed, and no route disclosing a token or hash.

Two existing expectations changed, both encoding the old behaviour: an unregistered actor could read
(now 401), and the events route returned stored hashes (now `[redacted]`).

Full suite green across eleven packages.

### Scope

`git diff --cached --stat main` — run. `internal/enforce/auth.go`, the `administer` verb and its
grants, token events in the projection, authentication middleware and token routes in the API,
`canon token`, the startup banner, the UI's token handling, MCP descriptions, sixteen tests, and
the README.

### Not verified

**The token can arrive in a query string.** The web UI cannot set a header on a page load, so
`?token=` is accepted; the UI strips it immediately, but it has already been sent, and query strings
reach server logs and proxies. The alternative is a cookie and a session layer, which is a larger
thing than this wants to be. **This is the weakest part of the increment** and the first thing to
revisit for a hostile network.

**No expiry.** A token lives until revoked. Expiry needs a clock in verification and a rotation
story for agents that run unattended, and half of that is worse than none.

**No per-token identity.** Revocation is all-or-nothing, so an actor cannot retire a laptop's token
while keeping their CI one. Recording a label per token and revoking by it is a small increment
nobody has asked for yet.

**No rate limiting.** Nothing slows repeated invalid tokens. Guessing 256 bits is not the threat;
an unauthenticated caller making the server do work is, and that is unaddressed.

**Bootstrap is unchanged and still creates an actor with no token**, so a fresh instance is open
until somebody runs `canon token`. Making bootstrap issue one would be better, and would change what
that command prints in a way I did not want to fold into a security increment.
