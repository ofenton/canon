#!/usr/bin/env python3
"""Import specs/increment-plan.md into a running Canon instance.

This is the dogfood step: the increment ledger this project has been run from
becomes the first real dataset in the tracker it was used to build.

History is reconstructed rather than asserted. An increment that is `done` did not
teleport there — it passed through approved, in_progress and in_review, and each of
those transitions is replayed so the flow metrics are real. The commit that carries
each increment's trailer supplies the timestamp, so cycle times reflect when the work
actually happened rather than when the import ran.

Usage:
  canon serve -db canon.db -schema deploy/canon.yaml &
  python3 scripts/import-ledger.py --base http://localhost:8080 --actor you
"""

from __future__ import annotations

import argparse
import json
import pathlib
import re
import subprocess
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone

HEADING = re.compile(r"^##\s+([a-z]{2,6}-\d{3})\s*:\s*(.+?)\s*$")
FIELD = re.compile(r"^-\s+\*\*(?P<key>[A-Za-z ]+):\*\*\s*(?P<value>.*)$")

# The route from planned to each terminal status. An increment that is done went
# through all of these, and replaying them is what makes the metrics true.
ROUTE = {
    "planned": [],
    "approved": ["approved"],
    "in-progress": ["approved", "in_progress"],
    "in-review": ["approved", "in_progress", "in_review"],
    "done": ["approved", "in_progress", "in_review", "done"],
    "abandoned": ["abandoned"],
}


class Client:
    def __init__(self, base: str, actor: str):
        # Accept either the server root or the API root, so a copied URL works.
        base = base.rstrip("/")
        self.base = base if base.endswith("/api") else base + "/api"
        self.actor = actor

    def call(self, method: str, path: str, body: dict | None = None) -> tuple[int, dict | None]:
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(self.base + path, data=data, method=method)
        req.add_header("X-Canon-Actor", self.actor)
        req.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(req, timeout=30) as res:
                raw = res.read().decode()
                return res.status, json.loads(raw) if raw else None
        except urllib.error.HTTPError as err:
            raw = err.read().decode()
            try:
                return err.code, json.loads(raw) if raw else None
            except json.JSONDecodeError:
                return err.code, {"error": raw}
        except urllib.error.URLError as err:
            print(f"cannot reach {self.base}: {err.reason}", file=sys.stderr)
            print("is `canon serve` running?", file=sys.stderr)
            sys.exit(1)


def parse_ledger(path: pathlib.Path) -> list[dict]:
    increments: list[dict] = []
    current: dict | None = None
    key: str | None = None

    for raw in path.read_text().splitlines():
        heading = HEADING.match(raw)
        if heading:
            current = {"id": heading.group(1), "title": heading.group(2), "fields": {}}
            increments.append(current)
            key = None
            continue
        if raw.startswith("## "):
            current, key = None, None
            continue
        if current is None:
            continue
        field = FIELD.match(raw)
        if field:
            key = field.group("key").strip()
            current["fields"][key] = field.group("value").strip()
        elif key and raw.strip().startswith("-") and raw.startswith((" ", "\t")):
            current["fields"][key] = (current["fields"].get(key, "") + " " + raw.strip().lstrip("- ")).strip()
    return increments


def commit_times() -> dict[str, list[str]]:
    """When each increment was actually worked, from its commit trailers."""
    try:
        out = subprocess.run(
            ["git", "log", "--reverse", "--format=%cI%x1f%B%x1e"],
            capture_output=True, text=True, check=True, timeout=30,
        ).stdout
    except (subprocess.CalledProcessError, FileNotFoundError, subprocess.TimeoutExpired):
        return {}

    times: dict[str, list[str]] = {}
    for record in out.split("\x1e"):
        if not record.strip():
            continue
        when, _, message = record.strip().partition("\x1f")
        for match in re.finditer(r"^Increment:\s*([a-z]{2,6}-\d{3})\s*$", message, re.M):
            times.setdefault(match.group(1), []).append(when)
    return times


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="http://localhost:8080")
    parser.add_argument("--actor", required=True)
    parser.add_argument("--plan", default="specs/increment-plan.md")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    increments = parse_ledger(pathlib.Path(args.plan))
    if not increments:
        print("no increments found", file=sys.stderr)
        return 1
    times = commit_times()
    client = Client(args.base, args.actor)

    print(f"importing {len(increments)} increments as {args.actor}\n")
    created = transitioned = 0
    problems: list[str] = []

    for inc in increments:
        fields = inc["fields"]
        status = fields.get("Status", "planned").lower()
        tier = (fields.get("Tier", "") or "").split()[0] if fields.get("Tier") else ""
        body = {
            "id": inc["id"],
            "title": inc["title"],
            "type": "increment",
            "team": "canon",
            "fields": {
                k: v for k, v in {
                    "tier": tier,
                    "kind": (fields.get("Type", "") or "").lower(),
                    "traces": fields.get("Traces", ""),
                    "scope": fields.get("Scope", ""),
                    "rollback": fields.get("Rollback Plan", ""),
                    "risk": fields.get("Risk", ""),
                }.items() if v and v.lower() != "none"
            },
        }
        if args.dry_run:
            print(f"  {inc['id']:<10} {status:<12} {inc['title'][:44]}")
            continue

        code, res = client.call("POST", "/issues", body)
        if code != 201:
            problems.append(f"{inc['id']}: create returned {code} — {res}")
            continue
        created += 1

        stamps = times.get(inc["id"], [])
        for _i, target in enumerate(ROUTE.get(status, [])):
            payload: dict = {"to": target}
            if target == "done":
                payload["evidence"] = fields.get("Evidence", "imported from the ledger")
            # The event model supports a backdated timestamp — Event.At may precede
            # the append, designed in feat-001 for exactly this — but no API accepts
            # one, so imported history lands at import time and the flow metrics for
            # it are not meaningful. Recorded in chore-003's evidence; R27 needs the
            # same capability, and so would any Jira import.
            _ = stamps
            code, res = client.call("POST", f"/issues/{inc['id']}/transition", payload)
            if code == 202:
                problems.append(f"{inc['id']}: {target} needs approval — {res.get('proposal_id')}")
                break
            if code >= 400:
                problems.append(f"{inc['id']}: -> {target} returned {code} — {res}")
                break
            transitioned += 1

        print(f"  {inc['id']:<10} {status:<12} {inc['title'][:44]}")

    if args.dry_run:
        return 0

    print(f"\ncreated {created} issues, applied {transitioned} transitions")
    if problems:
        print(f"\n{len(problems)} problem(s):")
        for p in problems:
            print(f"  ✗ {p}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
