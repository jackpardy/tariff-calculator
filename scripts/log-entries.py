#!/usr/bin/env python3
"""Move the log's new entries into knowledge/log.md.

A change doesn't edit knowledge/log.md, which every pull request used to
edit at the same place, so they all conflicted. It adds a file of its own,
knowledge/log/YYYY-MM-DD-what.md, holding its line (or lines), as log.md has
them: "* **Update**: ...". After a merge to master, the "Update log" workflow
runs this to put each under its date in log.md, newest first, and delete
the files.

    python3 scripts/log-entries.py          # move the entries into log.md
    python3 scripts/log-entries.py --check  # only check them (CI, every PR)
"""

import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
LOG = ROOT / "knowledge" / "log.md"
ENTRIES = ROOT / "knowledge" / "log"
NAME = re.compile(r"^(\d{4}-\d{2}-\d{2})-[a-z0-9][a-z0-9-]*\.md$")
LINE = re.compile(r"^\* \*\*[A-Z][a-z]+\*\*: \S")


def entries():
    """The entry files with their date, lines and when they were added
    (newest first), and the problems found in them."""
    found, problems = [], []
    for path in sorted(ENTRIES.glob("*.md")):
        if path.name == "README.md":
            continue
        m = NAME.match(path.name)
        if not m:
            problems.append(f"{path.name}: name it YYYY-MM-DD-what.md (lower case, hyphens)")
            continue
        lines = [l.rstrip() for l in path.read_text(encoding="utf-8").splitlines() if l.strip()]
        if not lines:
            problems.append(f"{path.name}: it's empty")
            continue
        bad = [l for l in lines if not LINE.match(l)]
        if bad:
            problems.append(f"{path.name}: each line is one entry, as in log.md: \"* **Update**: ...\" (not {bad[0][:40]!r})")
            continue
        found.append((m.group(1), added(path), path, lines))
    # Newest date first; within a date, the newest added first.
    found.sort(key=lambda e: (e[0], e[1]), reverse=True)
    return found, problems


def added(path):
    """When the file was first committed (seconds), or now if it isn't yet."""
    out = subprocess.run(
        ["git", "log", "--diff-filter=A", "--format=%ct", "--", str(path.relative_to(ROOT))],
        cwd=ROOT, capture_output=True, text=True,
    ).stdout.split()
    return int(out[-1]) if out else 2**62


def move(found):
    """Puts the entries in log.md under their dates and deletes their files."""
    text = LOG.read_text(encoding="utf-8")
    head, _, body = text.partition("\n## ")
    sections = []  # [date, [lines]] in the page's order
    for chunk in ("## " + body).split("\n## ") if body else []:
        chunk = chunk[3:] if chunk.startswith("## ") else chunk
        date, _, rest = chunk.partition("\n")
        sections.append([date.strip(), [l for l in rest.splitlines() if l.strip()]])
    by_date = {s[0]: s for s in sections}
    # Oldest first, so each goes on top and the newest ends up first.
    for date, _, path, lines in reversed(found):
        if date not in by_date:
            by_date[date] = [date, []]
            sections.append(by_date[date])
        by_date[date][1][:0] = lines
        path.unlink()
    sections.sort(key=lambda s: s[0], reverse=True)
    out = head.rstrip("\n") + "\n\n" + "\n\n".join("## " + d + "\n" + "\n".join(ls) for d, ls in sections) + "\n"
    LOG.write_text(out, encoding="utf-8")


def main():
    found, problems = entries()
    for p in problems:
        print(f"log entry: {p}", file=sys.stderr)
    if problems:
        sys.exit(1)
    if "--check" in sys.argv:
        print(f"{len(found)} log entries waiting, all fine")
        return
    if found:
        move(found)
    print(f"moved {len(found)} log entries into knowledge/log.md")


if __name__ == "__main__":
    main()
