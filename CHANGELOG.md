# Changelog

Canon is pre-1.0. Breaking changes happen and are recorded here; nothing else is.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[semantic versioning](https://semver.org/) once there is a release to version.

## Unreleased

### Added

- **Authentication.** Canon issues bearer tokens (`canon token -actor <id>`), stored as a SHA-256
  hash and shown once. Authentication turns on per actor: an actor holding a token must present
  one, an actor holding none is trusted as before, and `canon serve` names who is still claimable.
- **Full-text search.** A bare word in a query searches an issue's title, fields, multi-values and
  checklist items. `team=platform` and friends are unchanged.
- **Schema usage report.** `canon usage` and `GET /api/schema/usage` list every declared field,
  state, issue type, team and role with how many issues use it, unused first.
- **Webhooks.** Declared in `canon.yaml`, delivered asynchronously with bounded retries. A write
  never waits on a subscriber.
- **Declared teams.** `teams:` in `canon.yaml`. A schema that declares none accepts any team, so
  existing instances are unaffected until they declare one.
- **Backdated writes.** `?at=<RFC 3339>` on issue write routes, gated by a new `backdate` grant.
- **Commit linking.** `canon link` records commits against issues with their own author timestamps,
  and `canon trace` reports how much work carries an issue reference.
- **One-command issue creation.** `canon new "<title>"` from inside a repository.

### Changed

- **BREAKING: every JSON key is now snake_case.** Routes serialising domain types returned Go field
  names — `GET /api/issues` gave `ID`, `Title`, `State`, `LastActor` — while hand-written responses
  already returned `depends_on` and `linked_by`. Everything is snake_case now. A client reading
  `issue.ID` must read `issue.id`.
- **BREAKING: reads require authentication.** Every `/api/` route is behind authentication, not just
  writes. An unregistered or unauthenticated caller previously received `200` from `GET /api/issues`
  and now receives `401`.
- **BREAKING: registry writes require the `administer` permission.** Registering actors, granting or
  revoking roles, and changing team membership previously succeeded for any registered actor — which
  meant any actor could grant itself any role. Roles needing this must add `administer` to their
  `can:` list in `canon.yaml`.
- `GET /api/events` redacts stored token hashes.

### Fixed

- **Any registered actor could grant itself any role**, including `admin`. See the `administer`
  change above. This was the most serious defect found in the project.
- **No read route authenticated**, so an unauthenticated caller could read an entire instance.
- Issue references in commit messages resolve case-insensitively, so `canon trace` and `canon link`
  can no longer disagree about whether a commit is tracked.
- Imported history carries the timestamps of the commits it came from, so flow metrics measure when
  work happened rather than when the import ran. Every cycle time previously read `0d`.
- A superseded render no longer paints over the current one in the web UI.
- Durations shorter than a day are reported rather than rounded to zero.
