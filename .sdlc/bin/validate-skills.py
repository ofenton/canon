#!/usr/bin/env python3
"""Validate skills/ against the Agent Skills specification.

Checks the rules that actually break discovery — a name that does not match its
directory, a description too vague to route on, a body long enough to crowd out
the conversation, a reference link that points nowhere.

Exits 0 when every skill is valid, 1 otherwise.
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys

NAME_RE = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")
LINK_RE = re.compile(r"\[[^\]]*\]\((?!https?://|#)([^)]+)\)")

MAX_NAME = 64
MAX_DESCRIPTION = 1024
MAX_BODY_LINES = 500
# Below this, a description almost never carries both what the skill does and when
# to use it, which is what the agent routes on.
MIN_DESCRIPTION = 40

FIRST_PERSON = re.compile(r"\b(I can|I will|I help|you can use this|use me to)\b", re.I)


def split_frontmatter(text: str) -> tuple[str, str] | None:
    if not text.startswith("---"):
        return None
    parts = text.split("---", 2)
    if len(parts) < 3:
        return None
    return parts[1], parts[2]


def parse_frontmatter(raw: str) -> dict[str, str]:
    """Minimal YAML read for the flat scalar fields the spec defines."""
    fields: dict[str, str] = {}
    key = None
    for line in raw.splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if line[0] not in " \t" and ":" in line:
            key, _, value = line.partition(":")
            key = key.strip()
            fields[key] = value.strip().strip('"').strip("'")
        elif key and line.strip():
            fields[key] = (fields[key] + " " + line.strip()).strip()
    return fields


def check_skill(skill_dir: pathlib.Path) -> list[str]:
    errors: list[str] = []
    md = skill_dir / "SKILL.md"
    rel = skill_dir.name

    if not md.exists():
        return [f"{rel}: no SKILL.md"]

    text = md.read_text()
    split = split_frontmatter(text)
    if split is None:
        return [f"{rel}: SKILL.md has no YAML frontmatter delimited by ---"]

    raw_fm, body = split
    fields = parse_frontmatter(raw_fm)

    name = fields.get("name", "")
    if not name:
        errors.append(f"{rel}: frontmatter is missing required field 'name'")
    else:
        if name != skill_dir.name:
            errors.append(f"{rel}: name '{name}' does not match directory name '{skill_dir.name}'")
        if len(name) > MAX_NAME:
            errors.append(f"{rel}: name is {len(name)} chars, max {MAX_NAME}")
        if not NAME_RE.match(name):
            errors.append(
                f"{rel}: name '{name}' must be lowercase alphanumerics separated by single hyphens"
            )
        if "claude" in name.lower() or "anthropic" in name.lower():
            errors.append(f"{rel}: name may not contain reserved words 'claude' or 'anthropic'")

    description = fields.get("description", "")
    if not description:
        errors.append(f"{rel}: frontmatter is missing required field 'description'")
    else:
        if len(description) > MAX_DESCRIPTION:
            errors.append(f"{rel}: description is {len(description)} chars, max {MAX_DESCRIPTION}")
        if len(description) < MIN_DESCRIPTION:
            errors.append(
                f"{rel}: description is only {len(description)} chars — state what it does "
                "and when to use it, including trigger phrases"
            )
        if not re.search(r"\bUse (when|after|before|for)\b", description):
            errors.append(
                f"{rel}: description does not say when to use the skill "
                "(no 'Use when ...' clause) — agents route on this"
            )
        if FIRST_PERSON.search(description):
            errors.append(f"{rel}: description must be third person, not 'I' or 'you'")
        if "<" in description and ">" in description:
            errors.append(f"{rel}: description may not contain XML tags")

    body_lines = body.strip().splitlines()
    if len(body_lines) > MAX_BODY_LINES:
        errors.append(
            f"{rel}: body is {len(body_lines)} lines, max {MAX_BODY_LINES} — "
            "move detail into references/"
        )

    for target in LINK_RE.findall(body):
        target = target.split("#", 1)[0].strip()
        if not target:
            continue
        if "\\" in target:
            errors.append(f"{rel}: link '{target}' uses backslashes — use forward slashes")
        if not (skill_dir / target).resolve().exists():
            errors.append(f"{rel}: link target does not exist — {target}")

    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", nargs="?", default="skills", help="skills directory")
    args = parser.parse_args()

    root = pathlib.Path(args.root)
    if not root.is_dir():
        print(f"error: {root} not found — run from the repository root", file=sys.stderr)
        return 1

    skill_dirs = sorted(d for d in root.iterdir() if d.is_dir() and not d.name.startswith("."))
    if not skill_dirs:
        print(f"error: no skills found in {root}", file=sys.stderr)
        return 1

    errors: list[str] = []
    for skill_dir in skill_dirs:
        errors.extend(check_skill(skill_dir))

    if errors:
        print(f"{len(errors)} problem(s) across {len(skill_dirs)} skills\n", file=sys.stderr)
        for err in errors:
            print(f"  ✗ {err}", file=sys.stderr)
        return 1

    print(f"{root}: ok — {len(skill_dirs)} skills valid")
    return 0


if __name__ == "__main__":
    sys.exit(main())
