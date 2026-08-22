# Security lens

## Contents
- Where to look first
- Trust boundaries
- Checklist by category
- Tooling
- Rating severity

## Where to look first

Follow untrusted input. Every finding of consequence starts with data the attacker controls
reaching somewhere it should not. Map the entry points — HTTP handlers, message consumers,
file uploads, CLI arguments, webhooks — then trace each one inward until it hits a sink:
a query, a shell, a deserializer, a file path, a template, an outbound request.

## Trust boundaries

Draw the boundaries explicitly and check what is validated at each crossing:

- Internet → application
- Application → database, cache, queue
- Service → service (is internal traffic actually authenticated?)
- Application → third party (what is sent, what is trusted back)
- User → user (multi-tenant isolation)

The common failure is not missing validation but validation at the wrong boundary — checked at
the edge, then re-entered by an internal path that skips it.

## Checklist by category

**Injection** — SQL/NoSQL built by concatenation; shell invocation with user data; template
injection; LDAP and XML; path traversal in file operations; SSRF in outbound URLs.

**AuthN/AuthZ** — Missing checks on individual handlers rather than middleware; object-level
authorization (can user A fetch user B's record by id?); privilege escalation via mass
assignment; token expiry, rotation and revocation; session fixation.

**Secrets** — Credentials in the repo, in CI config, in log output, in error responses, in
client bundles. Check git history, not just the working tree.

**Data** — Encryption in transit and at rest; PII in logs and analytics; retention; what
appears in exception traces returned to users.

**Dependencies** — Known CVEs; unpinned versions; packages that are unmaintained or have
changed ownership; lockfile drift from the manifest.

**Configuration** — Debug mode in production; permissive CORS; missing security headers;
default credentials; over-broad cloud IAM; storage buckets and DB instances open to the world.

**Client-side** (web) — XSS sinks (`innerHTML`, `dangerouslySetInnerHTML`, `eval`); CSRF on
state-changing requests; secrets in the bundle; postMessage origin checks.

## Tooling

Run what the ecosystem provides, then confirm by reading. Typical: `npm audit` / `pip-audit` /
`cargo audit` for dependencies; `semgrep` or `bandit` / `gosec` / `brakeman` for SAST;
`gitleaks` or `trufflehog` for secrets in history; `checkov` or `tfsec` for IaC.

Every scanner finding must be confirmed by reading the code before it becomes a finding.
Report the confirmed ones with the scanner rule id, and say how many you dismissed and why.

## Rating severity

| Severity | Test |
|---|---|
| Critical | Remotely exploitable, unauthenticated, leads to data loss or RCE |
| High | Exploitable by an authenticated user, crosses a tenant or privilege boundary |
| Medium | Requires unusual conditions, or the impact is bounded |
| Low | Defence in depth; no direct exploit path |

Do not inflate. An assessment where everything is Critical tells the reader nothing about
what to do on Monday.
