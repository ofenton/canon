#!/usr/bin/env python3
"""Import specs/increment-plan.md into a running Canon instance.

This is the dogfood step: the increment ledger this project has been run from
becomes the first real dataset in the tracker it was used to build.

History is reconstructed rather than asserted. An increment that is `done` did not
teleport there — it passed through approved, in_progress and in_review, and each of
those transitions is replayed so the flow metrics are real. The commit that carries
each increment's trailer supplies the timestamp, so cycle times reflect when the work
actually happened rather than when the import ran.

Backdating needs the `backdate` grant, so the importing actor must hold a role that
has it. Without it every transition is refused and the import reports the problem
rather than quietly landing the whole history at import time — which is what it did
before feat-023, and why the first dogfood run measured every cycle time as zero.

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
import urllib.parse
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

    def call(self, method: str, path: str, body: dict | None = None,
             at: str | None = None) -> tuple[int, dict | None]:
        data = json.dumps(body).encode() if body is not None else None
        url = self.base + path
        if at:
            url += ("&" if "?" in url else "?") + urllib.parse.urlencode({"at": at})
        req = urllib.request.Request(url, data=data, method=method)
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


def stamp_for(stamps: list[str], step: int, steps: int) -> str | None:
    """Pick the commit whose time best represents one step of an increment's route.

    An increment usually has more commits than transitions, and occasionally fewer.
    Spreading the route across whatever commits exist puts approved near the start and
    done near the last commit, which is the shape cycle time is trying to measure. It
    is an approximation, and a truer one than stamping everything with `now`.
    """
    if not stamps:
        return None
    if steps <= 1:
        return stamps[-1]
    index = round(step * (len(stamps) - 1) / (steps - 1))
    return stamps[min(index, len(stamps) - 1)]


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

        stamps = times.get(inc["id"], [])

        # The issue is created as of its first commit. An issue's own events may not
        # predate it (enforce.CheckNotBeforeCreation), so creating it at import time
        # would make every replayed transition illegal.
        code, res = client.call("POST", "/issues", body, at=stamps[0] if stamps else None)
        if code != 201:
            problems.append(f"{inc['id']}: create returned {code} — {res}")
            continue
        created += 1
        route = ROUTE.get(status, [])
        for _i, target in enumerate(route):
            payload: dict = {"to": target}
            if target == "done":
                payload["evidence"] = fields.get("Evidence", "imported from the ledger")
            # Each transition is dated from the increment's commits, so cycle time
            # measures when the work happened rather than when the import ran. This
            # needs the backdate grant (feat-023); without it the write is refused
            # and the problem is reported rather than silently landing at now.
            code, res = client.call(
                "POST", f"/issues/{inc['id']}/transition", payload,
                at=stamp_for(stamps, _i, len(route)),
            )
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
