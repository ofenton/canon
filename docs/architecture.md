# Architecture

Current state of the system, as built. Regenerate or update this when structure changes —
`assess-codebase` reads it, and stale architecture docs actively mislead agents.

Keep it descriptive: what exists and how it fits together. Decisions and rationale belong in
`docs/constitution.md` (if they are rules) or in the relevant increment's detail file (if they
were made while building).

## Context

_What this system does, and what it talks to._

## Components

| Component | Responsibility | Language / framework | Owner |
|---|---|---|---|

## Data

_Stores, what lives in each, and where personal data is._

## Runtime

_Where it runs, how it is deployed, how it is observed._

## Cloud dependencies

_Each managed service used, what it would take to replace it, and roughly how long. Filling this
in honestly is the cheapest insurance against a lock-in nobody chose._

| Service | Used for | Replaceable with | Cost to move |
|---|---|---|---|

## Known constraints

_Things that shape every change — scaling limits, integration contracts, licences._
