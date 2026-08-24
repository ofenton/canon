# Security

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub's advisory form](https://github.com/ofenton/canon/security/advisories/new), not as a public
issue.

Include what you found, how to reproduce it, and what an attacker could do with it. You will get an
acknowledgement within a few days. Canon is a small project maintained in spare time; expect
honesty about timelines rather than a service-level commitment.

## What Canon protects, and what it does not

Canon is **pre-1.0 and has not been audited**. It is designed to be self-hosted on a network you
control. Read this before exposing an instance more widely than that.

### Known limitations

These are deliberate, documented, and would not be treated as vulnerabilities — though arguments for
changing them are welcome as issues:

- **Tokens may arrive in a query string.** The web UI cannot set a header on a page load, so
  `?token=` is accepted. The UI strips it from the address bar immediately, but it has already been
  sent, and query strings reach server logs and proxies.
- **Tokens do not expire.** A token is valid until revoked.
- **Revocation is all or nothing.** Revoking withdraws every token an actor holds; there is no way
  to retire one device's token while keeping another's.
- **No rate limiting.** Nothing slows repeated invalid tokens or expensive queries.
- **Authentication is per actor during migration.** An actor holding no token can still be claimed
  by anybody. `canon serve` names them at startup; an instance is only fully protected once every
  actor holds a token.
- **`canon bootstrap` creates an actor with no token**, so a fresh instance is open until somebody
  runs `canon token`.
- **The event log is not encrypted at rest** and contains everything anybody ever wrote.
- **Webhook deliveries are unsigned**, so a subscriber cannot verify a delivery came from Canon.

### What is defended

- Tokens are 256 random bits, stored as a SHA-256 hash, shown once, and compared in constant time.
- `GET /api/events` redacts stored hashes.
- Registry operations — registering actors, granting roles, team membership, issuing tokens for
  somebody else — require the `administer` permission, which is deliberately not team-scoped.
- The event log rejects `UPDATE` and `DELETE` at the database level, so history cannot be rewritten
  by anything holding a connection.
