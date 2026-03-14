#!/usr/bin/env python3
"""
download_godot_docs.py — Download and convert Godot documentation RST files.

Fetches .rst files from the godotengine/godot-docs GitHub repository for a
specific Godot version, converts them to Markdown, and saves them to the
docs/{version}/ directory alongside this script.

Directories fetched (all .rst files, recursively):
  about/          — Introduction, FAQ, release notes, whats-new
  classes/        — Full Godot class reference (~700+ files)
  contributing/   — Engine internals, development docs (what the original
                    script called "engine_details" — mapped to contributing/)

Usage:
  python3 download_godot_docs.py                  # auto-detect version from project.godot
  python3 download_godot_docs.py 4.3              # explicit version
  python3 download_godot_docs.py 4.3 --dirs about classes   # subset of dirs
  python3 download_godot_docs.py 4.3 --dry-run    # list files without downloading
  python3 download_godot_docs.py 4.3 --resume     # skip files already downloaded

Requirements: Python 3.6+ standard library only (no pip installs needed).
"""

import argparse
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path


# ─── Configuration ────────────────────────────────────────────────────────────

REPO = "godotengine/godot-docs"
BASE_RAW = "https://raw.githubusercontent.com"
BASE_API = "https://api.github.com"

# Directories to fetch from the repo. Each is fetched recursively.
# "engine_details" in the original script intent maps to "contributing/" —
# this is where Godot's internal architecture and engine development docs live.
DEFAULT_DIRS = ["about", "classes", "contributing"]

# Delay between requests (seconds). GitHub allows ~60 unauthenticated req/min.
# Keep this at 0.5s or higher to stay well within the limit.
REQUEST_DELAY = 0.5

# Max retries per file on network error.
MAX_RETRIES = 3


# ─── Version detection ────────────────────────────────────────────────────────

def detect_godot_version(script_dir: Path) -> str | None:
    """
    Try to detect the Godot project version from project.godot.

    project.godot is an INI-like file. The Godot version is stored as:
        [application]
        config/features=PackedStringArray("4.3", "Forward Plus")

    The version is always the first element of config/features.
    We walk up from the script directory looking for project.godot, since
    this script lives at addons/gd-scope/scripts/ and project.godot is at
    the project root.
    """
    search = script_dir
    for _ in range(5):  # don't walk past 5 levels up
        candidate = search / "project.godot"
        if candidate.exists():
            content = candidate.read_text(encoding="utf-8", errors="ignore")
            # Match: config/features=PackedStringArray("4.3", ...)
            m = re.search(r'config/features\s*=\s*PackedStringArray\("(\d+\.\d+)"', content)
            if m:
                return m.group(1)
        search = search.parent
    return None


# ─── GitHub API helpers ───────────────────────────────────────────────────────

def github_request(url: str, token: str | None = None) -> dict:
    """Make a GitHub API request, respecting rate limits."""
    headers = {"User-Agent": "gd-scope-docs/1.0"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            # Check rate limit headers
            remaining = resp.headers.get("X-RateLimit-Remaining", "60")
            if int(remaining) < 5:
                reset = int(resp.headers.get("X-RateLimit-Reset", time.time() + 60))
                wait = max(0, reset - time.time()) + 1
                print(f"  Rate limit low ({remaining} remaining), waiting {wait:.0f}s...")
                time.sleep(wait)
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        if e.code == 403:
            raise RuntimeError(
                "GitHub API rate limit hit or access denied.\n"
                "Set GITHUB_TOKEN env var for higher limits:\n"
                "  export GITHUB_TOKEN=github_pat_..."
            ) from e
        if e.code == 404:
            raise RuntimeError(f"Not found: {url}\nCheck that the version branch exists.") from e
        raise


def list_rst_files(version: str, directories: list[str], token: str | None) -> list[str]:
    """
    Use the GitHub tree API to recursively list all .rst files in the
    specified directories for the given version branch.

    Returns a list of repo-relative paths like "classes/class_node.rst".
    """
    print(f"Fetching file tree for branch '{version}'...")
    url = f"{BASE_API}/repos/{REPO}/git/trees/{version}?recursive=1"
    data = github_request(url, token)

    if data.get("truncated"):
        print("  Warning: tree was truncated by GitHub (>100k entries). Some files may be missing.")

    all_paths = [item["path"] for item in data.get("tree", []) if item["type"] == "blob"]
    rst_paths = [
        p for p in all_paths
        if p.endswith(".rst") and any(p.startswith(d + "/") for d in directories)
    ]

    # Report counts per directory
    for d in directories:
        count = sum(1 for p in rst_paths if p.startswith(d + "/"))
        print(f"  {d}/: {count} .rst files")

    print(f"  Total: {len(rst_paths)} files")
    return sorted(rst_paths)


# ─── RST to Markdown converter ────────────────────────────────────────────────

# Substitution definitions used throughout Godot's generated class reference.
# .. |void| replace:: void  etc. — we handle these without needing to parse the
# .. |name| replace:: value directive; the common ones are hardcoded here and
# any file-specific ones are collected during the first pass of rst_to_markdown.
_GODOT_SUBSTITUTIONS: dict[str, str] = {
    "void": "void",
    "const": "*(const)*",
    "virtual": "*(virtual)*",
    "static": "*(static)*",
    "vararg": "*(vararg)*",
    "constructor": "*(constructor)*",
    "operator": "*(operator)*",
}


def clean_inline(text: str, substitutions: dict | None = None) -> str:
    """
    Convert RST inline markup to Markdown equivalents.

    Handles Godot's Sphinx-generated class reference patterns:
      - |substitution| references (|void|, |const|, |virtual|, etc.)
      - :ref:`DisplayName<target>` → DisplayName
      - :role:`text` → text  (any other Sphinx role)
      - ``literal`` → `literal`
      - RST escaped characters: \\( → (, \\  → space, etc.
    """
    subs = {**_GODOT_SUBSTITUTIONS, **(substitutions or {})}
    # |substitution| references
    text = re.sub(r"\|(\w+)\|", lambda m: subs.get(m.group(1), m.group(1)), text)
    # :ref:`DisplayName<target>` → DisplayName
    text = re.sub(r":ref:`([^<`]+)<[^>]+>`", r"\1", text)
    # :ref:`Name` (no angle bracket)
    text = re.sub(r":ref:`([^`]+)`", r"\1", text)
    # Any other Sphinx role: :role:`text` → text
    text = re.sub(r":\w+:`([^`]+)`", r"\1", text)
    # ``literal`` → `literal`
    text = re.sub(r"``([^`]+)``", r"`\1`", text)
    # RST escaped characters: "\ " → " ", "\:" → ":", "\(" → "(" etc.
    text = re.sub(r"\\(.)", r"\1", text)
    # Collapse runs of spaces created by escape removal
    text = re.sub(r"  +", " ", text)
    return text.strip()


def _parse_grid_table(lines: list[str], start: int) -> tuple[list[list[str]], int]:
    """
    Parse an RST grid table (+---+---+ border style).

    Column boundaries are derived from the '+' positions in the first border
    line of the ORIGINAL (indented) line, not the stripped version. This is
    critical because the cell content may contain '|' characters that must
    not be treated as column separators.

    Returns (rows, index_of_first_line_after_table).
    """
    # Find the first border line; record '+' positions in original line.
    col_bounds: list[int] | None = None
    i = start
    while i < len(lines):
        stripped = lines[i].strip()
        if re.match(r"^\+[-=]+(\+[-=]+)*\+$", stripped):
            col_bounds = [m.start() for m in re.finditer(r"\+", lines[i])]
            break
        i += 1

    if col_bounds is None:
        return [], start

    rows: list[list[str]] = []
    current_cells: list[str] | None = None
    i = start

    while i < len(lines):
        line = lines[i]
        stripped = line.strip()

        # Border line (--- or ===): flush current row
        if re.match(r"^\+[-=]+(\+[-=]+)*\+$", stripped):
            if current_cells is not None:
                rows.append(current_cells)
                current_cells = None
            i += 1
            continue

        # Data row
        if stripped.startswith("|") and stripped.endswith("|"):
            cells = []
            padded = line.rstrip()
            for j in range(len(col_bounds) - 1):
                left = col_bounds[j] + 1
                right = col_bounds[j + 1]
                if left >= len(padded):
                    cell = ""
                elif right <= len(padded):
                    cell = padded[left:right].strip()
                else:
                    # Line is shorter than the table border width (RST doesn't
                    # require right-padding). Grab from left to end, stripping
                    # only the single outermost closing '|' (the column border),
                    # not all '|' characters — inner pipes are part of cell content
                    # (e.g. the closing | of a |substitution| reference).
                    segment = padded[left:].rstrip()
                    if segment.endswith("|"):
                        segment = segment[:-1]
                    cell = segment.strip()
                cells.append(cell)
            if current_cells is None:
                current_cells = cells
            else:
                # Multi-line cell: append to existing
                for j, cell in enumerate(cells):
                    if j < len(current_cells) and cell:
                        current_cells[j] = (current_cells[j] + " " + cell).strip()
            i += 1
            continue

        # Anything else ends the table
        if stripped and not stripped.startswith("+"):
            break
        i += 1

    if current_cells is not None:
        rows.append(current_cells)
    return rows, i


def _parse_list_table(lines: list[str], start: int) -> tuple[list[list[str]], int]:
    """
    Parse an RST .. list-table:: directive.

    list-table format uses bullet items for rows and sub-bullets for columns:
        * - col1_value
          - col2_value
        * - row2_col1
          - row2_col2

    Returns (rows, index_of_first_line_after_table).
    """
    i = start + 1  # skip .. list-table:: line
    # Skip options (:widths:, :header-rows:, :stub-columns:, etc.)
    while i < len(lines) and re.match(r"^\s+:\w[\w-]*:", lines[i]):
        i += 1
    # Skip blank lines after options
    while i < len(lines) and not lines[i].strip():
        i += 1

    rows: list[list[str]] = []
    current_row: list[str] | None = None

    while i < len(lines):
        line = lines[i]
        stripped = line.strip()

        # New row: "   * - cell"
        if re.match(r"^\s+\* - ", line) or re.match(r"^\* - ", stripped):
            if current_row is not None:
                rows.append(current_row)
            cell = re.sub(r"^\s*\* - ", "", line).strip()
            current_row = [cell]
            i += 1
            continue

        # Next column in same row: "     - cell"
        if re.match(r"^\s+- ", line) and current_row is not None and not stripped.startswith("* -"):
            cell = re.sub(r"^\s*- ", "", line).strip()
            current_row.append(cell)
            i += 1
            continue

        # Continuation of last cell (deeper indent, not a bullet)
        if line.startswith("      ") and stripped and current_row:
            current_row[-1] = (current_row[-1] + " " + stripped).strip()
            i += 1
            continue

        # End of table: non-indented content
        if not line.startswith(" ") and stripped:
            break
        # End of table: blank line followed by non-indented content
        if not stripped:
            peek = i + 1
            while peek < len(lines) and not lines[peek].strip():
                peek += 1
            if peek < len(lines) and not lines[peek].startswith(" "):
                break
        i += 1

    if current_row is not None:
        rows.append(current_row)
    return rows, i


def _table_to_markdown(rows: list[list[str]], substitutions: dict | None = None) -> str:
    """Convert parsed table rows to a GitHub-Flavored Markdown table."""
    if not rows:
        return ""

    # Clean inline markup in all cells
    cleaned = [[clean_inline(cell, substitutions) for cell in row] for row in rows]

    # Normalize column count
    max_cols = max(len(row) for row in cleaned)
    cleaned = [row + [""] * (max_cols - len(row)) for row in cleaned]

    # Drop trailing columns that are empty in every row (artefact of grid table padding)
    while max_cols > 1 and all(row[max_cols - 1] == "" for row in cleaned):
        cleaned = [row[:-1] for row in cleaned]
        max_cols -= 1

    out = [
        "| " + " | ".join(cleaned[0]) + " |",
        "| " + " | ".join(["---"] * max_cols) + " |",
    ]
    for row in cleaned[1:]:
        out.append("| " + " | ".join(row) + " |")
    return "\n".join(out)


def rst_to_markdown(rst_text: str) -> str:
    """
    Convert a Godot RST documentation file to Markdown.

    This is a purpose-built converter for Godot's specific RST patterns —
    not a general RST parser. It handles:
      - Section headings (=, -, ~, ^ underlines)
      - Code blocks (.. code-block:: gdscript/etc.)
      - Admonitions (.. note::, .. warning::, .. seealso::, .. tip::)
      - Grid tables (+---+---+ border style) — class reference method listings
      - List-tables (.. list-table::) — alternate tabular format
      - Substitution definitions (.. |void| replace:: void)
      - Inline roles (:ref:, :class:, :meth:, etc.) and |substitutions|
      - RST escaped characters (backslash sequences)
      - RST field lists and class directives (stripped)
      - ``literal`` inline code

    It intentionally ignores:
      - .. toctree:: directives (navigation, not content)
      - .. image:: and .. figure:: (binary assets not downloaded)
      - :github_url:, :orphan:, etc. (file metadata)
    """
    lines = rst_text.split("\n")

    # First pass: collect file-specific substitution definitions.
    # These are declared at the top of each file, e.g.:
    #   .. |void| replace:: void
    #   .. |const| replace:: ⚙
    substitutions: dict[str, str] = {}
    for line in lines:
        m = re.match(r"^\.\. \|(\w+)\| replace::\s*(.+)$", line.strip())
        if m:
            substitutions[m.group(1)] = m.group(2).strip()
    all_subs = {**_GODOT_SUBSTITUTIONS, **substitutions}

    out: list[str] = []
    i = 0
    in_code_block = False

    # RST heading level markers — first encountered defines the hierarchy.
    heading_chars: list[str] = []

    def heading_level(char: str) -> str:
        if char not in heading_chars:
            heading_chars.append(char)
        return "#" * (heading_chars.index(char) + 1)

    while i < len(lines):
        line = lines[i]
        stripped = line.strip()

        # ── Code block content ───────────────────────────────────────────────
        if in_code_block:
            if line and not line[0].isspace():
                out.append("```")
                out.append("")
                in_code_block = False
                # Fall through — reprocess this line
            else:
                out.append(line[4:] if line.startswith("    ") else
                           line[1:] if line.startswith("\t") else line)
                i += 1
                continue

        # ── Grid tables (+---+---+) ──────────────────────────────────────────
        if re.match(r"^\+[-=]+(\+[-=]+)*\+$", stripped):
            rows, i = _parse_grid_table(lines, i)
            md = _table_to_markdown(rows, all_subs)
            if md:
                out.append(md)
                out.append("")
            continue

        # ── Directives (must check before regular content) ───────────────────
        if re.match(r"^\.\. ", stripped):

            # Substitution definitions — already collected, skip
            if re.match(r"^\.\. \|\w+\| replace::", stripped):
                i += 1
                continue

            # Code block
            m = re.match(r"^\.\. code-block::\s*(\S*)", stripped)
            if m:
                lang = m.group(1) or "text"
                out.append(f"```{lang}")
                in_code_block = True
                i += 1
                if i < len(lines) and not lines[i].strip():
                    i += 1
                continue

            # list-table
            if re.match(r"^\.\. list-table::", stripped):
                rows, i = _parse_list_table(lines, i)
                md = _table_to_markdown(rows, all_subs)
                if md:
                    out.append(md)
                    out.append("")
                continue

            # Admonitions
            adm = re.match(
                r"^\.\. (note|warning|tip|important|seealso|deprecated|versionadded|versionchanged)::",
                stripped,
            )
            if adm:
                kind = adm.group(1)
                label = {
                    "note": "Note", "warning": "Warning", "tip": "Tip",
                    "important": "Important", "seealso": "See also",
                    "deprecated": "Deprecated", "versionadded": "Added",
                    "versionchanged": "Changed",
                }.get(kind, kind.capitalize())
                body_lines = []
                i += 1
                while i < len(lines) and (
                    lines[i].startswith("   ") or lines[i].startswith("\t") or not lines[i].strip()
                ):
                    if lines[i].strip():
                        body_lines.append(lines[i].strip())
                    i += 1
                body = clean_inline(" ".join(body_lines), all_subs)
                if body:
                    out.append(f"> **{label}:** {body}")
                    out.append("")
                continue

            # Block directives to skip entirely (with their indented bodies)
            if re.match(
                r"^\.\. (toctree|image|figure|literalinclude|include|raw|only|"
                r"tabbed|tabs|tab|rst-class|highlight|default-role|"
                r"currentmodule|moduleauthor|sectionauthor)::",
                stripped,
            ):
                i += 1
                while i < len(lines) and (
                    lines[i].startswith("   ") or lines[i].startswith("\t")
                    or (lines[i].strip() and lines[i][0] == " ")
                ):
                    i += 1
                continue

            # All other directives: skip single line
            i += 1
            continue

        # ── Label targets (.. _name:) ────────────────────────────────────────
        if re.match(r"^\.\. _[\w-]+:", stripped):
            i += 1
            continue

        # ── File-level field metadata (:github_url:, :orphan:, etc.) ─────────
        if re.match(r"^:\w[\w-]*:", line) and not out:
            i += 1
            continue

        # ── Section headings ─────────────────────────────────────────────────
        if i + 1 < len(lines):
            next_line = lines[i + 1]
            ns = next_line.strip()
            if (ns and all(c == ns[0] for c in ns)
                    and ns[0] in "=-~^+*#"
                    and len(ns) >= max(len(stripped), 1)
                    and stripped):
                title = clean_inline(stripped, all_subs)
                if title:
                    out.append(f"{heading_level(ns[0])} {title}")
                    out.append("")
                i += 2
                if i < len(lines) and not lines[i].strip():
                    i += 1
                continue

        # Skip bare underline/overline lines not consumed above
        if (stripped and all(c == stripped[0] for c in stripped)
                and stripped[0] in "=-~^+*#" and len(stripped) > 2):
            i += 1
            continue

        # ── Regular content ──────────────────────────────────────────────────
        # RST "::" at end of a paragraph introduces a literal/code block.
        # "Variables can be declared with ``var``::" → strip "::", open code fence.
        if stripped.endswith("::") and not stripped.startswith(".."):
            # Strip "::" (or " ::" leaving a colon if meaningful) and open a code block.
            prefix = stripped[:-2].rstrip()
            if prefix:
                out.append(clean_inline(prefix, all_subs))
            out.append("```")
            in_code_block = True
            i += 1
            # Skip blank line that typically follows
            if i < len(lines) and not lines[i].strip():
                i += 1
            continue

        out.append(clean_inline(line, all_subs))
        i += 1

    if in_code_block:
        out.append("```")
        out.append("")

    result = "\n".join(out)
    result = re.sub(r"\n{3,}", "\n\n", result)
    return result.strip()


# ─── Download ─────────────────────────────────────────────────────────────────

def download_file(url: str, token: str | None = None) -> str | None:
    """Download a single URL and return its text content, or None on failure."""
    headers = {"User-Agent": "gd-scope-docs/1.0"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, headers=headers)
    for attempt in range(1, MAX_RETRIES + 1):
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return resp.read().decode("utf-8", errors="replace")
        except urllib.error.HTTPError as e:
            if e.code == 404:
                return None  # File doesn't exist on this branch — skip silently
            if attempt == MAX_RETRIES:
                print(f"    HTTP {e.code} after {MAX_RETRIES} attempts: {url}")
                return None
        except urllib.error.URLError as e:
            if attempt == MAX_RETRIES:
                print(f"    Network error after {MAX_RETRIES} attempts: {e.reason}")
                return None
        time.sleep(attempt * 1.0)  # Back off on retry
    return None


# ─── Main ─────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="Download Godot documentation RST files and convert to Markdown.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument(
        "version",
        nargs="?",
        help="Godot version branch (e.g. '4.3', '4.2', 'stable'). "
             "Auto-detected from project.godot if not specified.",
    )
    parser.add_argument(
        "--dirs",
        nargs="+",
        default=DEFAULT_DIRS,
        metavar="DIR",
        help=f"Directories to fetch (default: {' '.join(DEFAULT_DIRS)})",
    )
    parser.add_argument(
        "--output",
        default=None,
        metavar="PATH",
        help="Output directory (default: docs/{version}/ next to this script)",
    )
    parser.add_argument(
        "--resume",
        action="store_true",
        help="Skip files that already exist in the output directory",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="List files that would be downloaded without fetching anything",
    )
    parser.add_argument(
        "--delay",
        type=float,
        default=REQUEST_DELAY,
        metavar="SECS",
        help=f"Delay between requests in seconds (default: {REQUEST_DELAY})",
    )
    parser.add_argument(
        "--token",
        default=os.environ.get("GITHUB_TOKEN"),
        metavar="TOKEN",
        help="GitHub personal access token (or set GITHUB_TOKEN env var). "
             "Raises rate limit from 60 to 5000 req/hour.",
    )
    args = parser.parse_args()

    script_dir = Path(__file__).parent.resolve()

    # ── Resolve version ───────────────────────────────────────────────────────
    version = args.version
    if not version:
        version = detect_godot_version(script_dir)
        if version:
            print(f"Detected Godot version from project.godot: {version}")
        else:
            print("Could not detect Godot version from project.godot.")
            print("Pass a version explicitly: python3 download_godot_docs.py 4.3")
            sys.exit(1)

    # ── Output directory ──────────────────────────────────────────────────────
    if args.output:
        output_dir = Path(args.output)
    else:
        # Default: docs/{version}/ relative to the script's parent (addons/gd-scope/)
        output_dir = script_dir.parent / "docs" / version
    output_dir.mkdir(parents=True, exist_ok=True)
    print(f"Output directory: {output_dir}")

    # ── Enumerate RST files ───────────────────────────────────────────────────
    try:
        rst_paths = list_rst_files(version, args.dirs, args.token)
    except RuntimeError as e:
        print(f"\nError: {e}")
        sys.exit(1)

    if not rst_paths:
        print("No .rst files found. Check the version branch and directory names.")
        sys.exit(1)

    if args.dry_run:
        print("\nDry run — files that would be downloaded:")
        for path in rst_paths:
            print(f"  {path}")
        print(f"\nTotal: {len(rst_paths)} files")
        return

    # ── Download and convert ──────────────────────────────────────────────────
    total = len(rst_paths)
    downloaded = 0
    skipped = 0
    failed = 0

    print(f"\nDownloading {total} files...")
    if not args.token:
        print("Tip: Set GITHUB_TOKEN for 5000 req/hour instead of 60.")
        print(f"     At {args.delay}s delay this will take ~{total * args.delay / 60:.0f} minutes.\n")

    for idx, rst_path in enumerate(rst_paths, 1):
        # Derive output filename: flatten subdirectory structure within each top-level dir
        # e.g. classes/class_node.rst       → {output_dir}/class_node.md
        #      about/introduction.rst       → {output_dir}/introduction.md
        #      contributing/dev_faq.rst     → {output_dir}/dev_faq.md
        # Files with the same basename in different directories get their dir prefixed.
        parts = Path(rst_path).parts
        if len(parts) > 2:
            # Nested: contributing/tutorials/core_types.rst → contributing_core_types.md
            stem = "_".join(parts[1:])
        else:
            stem = parts[-1]
        stem = re.sub(r"\.rst$", "", stem)
        # Sanitize: replace non-alphanumeric (except _-) with _
        stem = re.sub(r"[^\w-]", "_", stem)
        out_path = output_dir / f"{stem}.md"

        if args.resume and out_path.exists():
            skipped += 1
            continue

        raw_url = f"{BASE_RAW}/{REPO}/{version}/{rst_path}"
        print(f"  [{idx:4d}/{total}] {rst_path}", end="", flush=True)

        rst_content = download_file(raw_url, args.token)
        if rst_content is None:
            print(" — FAILED")
            failed += 1
            continue

        md_content = rst_to_markdown(rst_content)

        out_path.write_text(md_content, encoding="utf-8")
        downloaded += 1
        print(f" → {out_path.name} ({len(md_content)} chars)")

        time.sleep(args.delay)

    # ── Summary ───────────────────────────────────────────────────────────────
    print(f"\n{'─' * 50}")
    print(f"Done.")
    print(f"  Downloaded: {downloaded}")
    if skipped:
        print(f"  Skipped (already exist): {skipped}")
    if failed:
        print(f"  Failed: {failed}")
    print(f"  Output: {output_dir}")


if __name__ == "__main__":
    main()
