#!/usr/bin/env python3
"""Build the guides' PDFs: knowledge/guide/<name>.pdf from <name>.md, for the
routine builder guide and the competitions guide.

Renders the Markdown to HTML (front matter dropped, links to other bundle pages
pointed at GitHub) and prints it to A4 with headless Chrome or Chromium.

    pip install markdown
    CHROME=/path/to/chrome python3 scripts/guide-pdfs.py

Run it again whenever a guide changes, and commit the PDFs with it.
"""

import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

import markdown

ROOT = Path(__file__).resolve().parent.parent
GUIDE = ROOT / "knowledge/guide"
GUIDES = ["routine-guide", "competition-guide"]
BLOB = "https://github.com/jackpardy/tariff-calculator/blob/master/knowledge/guide/"

STYLE = """
@page { size: A4; margin: 18mm 16mm; }
body { font: 10.5pt/1.5 system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; color: #1a1a1a; }
header { border-bottom: 3px solid #485fc7; margin-bottom: 1.2rem; padding-bottom: 0.6rem; }
header h1 { font-size: 22pt; margin: 0; color: #485fc7; }
header p { margin: 0.2rem 0 0; color: #555; }
h1 { font-size: 15pt; margin: 1.6rem 0 0.5rem; color: #2b3a8c; break-after: avoid; }
h2 { font-size: 12pt; margin: 1.2rem 0 0.4rem; break-after: avoid; }
li { margin: 0.15rem 0; }
li, p { break-inside: avoid; }
a { color: #2b3a8c; text-decoration: none; }
code { font-size: 0.95em; }
"""


def chrome():
    for candidate in (os.environ.get("CHROME"), "chromium", "chromium-browser", "google-chrome", "chrome"):
        if candidate and (shutil.which(candidate) or Path(candidate).exists()):
            return candidate
    sys.exit("No Chrome or Chromium found; set CHROME to its path.")


def four_space_lists(body):
    """Python-Markdown nests lists by 4 spaces; the guide indents by the width
    of the item marker ("1. " is 3), as GitHub allows. Widen each indent level,
    and start a list that follows a paragraph on a new block."""
    lines = []
    for line in body.split("\n"):
        indent = len(line) - len(line.lstrip(" "))
        if indent and line.strip():
            line = " " * (indent + (indent + 2) // 3) + line.lstrip(" ")
        if re.match(r"(- |\d+\. )", line) and lines and lines[-1].strip() and not re.match(r"(- |\d+\. )", lines[-1]):
            lines.append("")
        lines.append(line)
    return "\n".join(lines)


def build(name):
    source, output = GUIDE / f"{name}.md", GUIDE / f"{name}.pdf"
    text = source.read_text()
    front, body = re.match(r"^---\n(.*?)\n---\n(.*)$", text, re.S).groups()
    title = re.search(r"^title: (.*)$", front, re.M).group(1)
    # Other bundle pages ("features.md", "../domain/tariff.md") open on GitHub.
    body = re.sub(r"\]\((?!https?:|#)([^)#]+\.(?:md|pdf))(#[^)]*)?\)", lambda m: f"]({BLOB}{m.group(1)}{m.group(2) or ''})", body)
    html = markdown.markdown(four_space_lists(body), extensions=["toc", "sane_lists"])
    page = f"""<!doctype html><html lang="en"><head><meta charset="utf-8"><title>{title}</title>
<style>{STYLE}</style></head><body>
<header><h1>{title}</h1><p>Trampoline Tariff Calculator · https://tariff.pardy.ie</p></header>
{html}</body></html>"""
    with tempfile.TemporaryDirectory() as tmp:
        src = Path(tmp) / "guide.html"
        src.write_text(page)
        subprocess.run([chrome(), "--headless", "--no-sandbox", "--disable-gpu", "--no-pdf-header-footer",
                        f"--print-to-pdf={output}", src.as_uri()], check=True, capture_output=True)
    print(f"wrote {output.relative_to(ROOT)}")


def main():
    for name in GUIDES:
        build(name)


if __name__ == "__main__":
    main()
