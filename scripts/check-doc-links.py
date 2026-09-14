#!/usr/bin/env python3
"""Offline validation for repository-local Markdown links and GitHub anchors."""

from __future__ import annotations

import html
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parent.parent
FENCE_RE = re.compile(r"^\s*(`{3,}|~{3,})")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.+?)\s*#*\s*$")
INLINE_LINK_RE = re.compile(r"!?\[[^\]]*\]\(([^)]+)\)")
REFERENCE_LINK_RE = re.compile(r"^\s*\[[^\]]+\]:\s*(\S+)")
HTML_LINK_RE = re.compile(r"(?:href|src)\s*=\s*([\"'])(.*?)\1", re.IGNORECASE)
EXPLICIT_ANCHOR_RE = re.compile(r"<(?:a|span)\b[^>]*(?:id|name)=[\"']([^\"']+)", re.IGNORECASE)
HTML_TAG_RE = re.compile(r"<[^>]+>")
MARKDOWN_LINK_TEXT_RE = re.compile(r"!?\[([^\]]*)\]\([^)]*\)")


def markdown_files() -> list[Path]:
    result = subprocess.run(
        ["git", "ls-files", "-co", "--exclude-standard", "--", "*.md"],
        cwd=ROOT,
        check=True,
        text=True,
        capture_output=True,
    )
    return sorted({ROOT / line for line in result.stdout.splitlines() if line and line != "CHANGELOG.md"})


def visible_lines(text: str):
    fence: str | None = None
    for number, line in enumerate(text.splitlines(), 1):
        match = FENCE_RE.match(line)
        if match:
            marker = match.group(1)[0]
            if fence is None:
                fence = marker
            elif fence == marker:
                fence = None
            continue
        if fence is None:
            yield number, line


def github_slug(raw: str) -> str:
    value = MARKDOWN_LINK_TEXT_RE.sub(r"\1", raw)
    value = HTML_TAG_RE.sub("", value)
    value = html.unescape(value).strip().lower()
    value = re.sub(r"[`*~]", "", value)
    value = "".join(ch for ch in value if ch.isalnum() or ch in " _-")
    return re.sub(r"\s", "-", value)


def anchors_for(path: Path) -> set[str]:
    anchors: set[str] = set()
    counts: defaultdict[str, int] = defaultdict(int)
    for _, line in visible_lines(path.read_text(encoding="utf-8")):
        for explicit in EXPLICIT_ANCHOR_RE.findall(line):
            anchors.add(explicit)
        match = HEADING_RE.match(line)
        if not match:
            continue
        base = github_slug(match.group(2))
        duplicate = counts[base]
        counts[base] += 1
        anchors.add(base if duplicate == 0 else f"{base}-{duplicate}")
    return anchors


def normalize_destination(raw: str) -> str:
    value = html.unescape(raw.strip())
    if value.startswith("<") and ">" in value:
        return value[1 : value.index(">")]
    # Strip an optional Markdown title: (path "title"). Repository paths do
    # not contain spaces; percent-encoded spaces remain intact.
    return re.split(r"\s+[\"']", value, maxsplit=1)[0]


def local_destination(raw: str) -> tuple[str, str] | None:
    value = normalize_destination(raw)
    if not value or value == "#":
        return None
    parsed = urlsplit(value)
    if parsed.scheme or parsed.netloc or value.startswith(("//", "/")):
        return None
    return unquote(parsed.path), unquote(parsed.fragment)


def destinations(line: str):
    for match in INLINE_LINK_RE.finditer(line):
        yield match.group(1)
    reference = REFERENCE_LINK_RE.match(line)
    if reference:
        yield reference.group(1)
    for match in HTML_LINK_RE.finditer(line):
        yield match.group(2)


def main() -> int:
    files = markdown_files()
    anchor_cache: dict[Path, set[str]] = {}
    errors: list[str] = []
    checked = 0

    for source in files:
        try:
            text = source.read_text(encoding="utf-8")
        except UnicodeDecodeError as exc:
            errors.append(f"{source.relative_to(ROOT)}: invalid UTF-8: {exc}")
            continue

        for line_number, line in visible_lines(text):
            for raw in destinations(line):
                local = local_destination(raw)
                if local is None:
                    continue
                rel_path, fragment = local
                target = source if not rel_path else (source.parent / rel_path).resolve()
                checked += 1
                try:
                    target.relative_to(ROOT)
                except ValueError:
                    errors.append(
                        f"{source.relative_to(ROOT)}:{line_number}: link escapes repository: {raw}"
                    )
                    continue
                if not target.exists():
                    errors.append(
                        f"{source.relative_to(ROOT)}:{line_number}: missing target: {raw}"
                    )
                    continue
                if fragment and target.is_file() and target.suffix.lower() == ".md":
                    anchors = anchor_cache.setdefault(target, anchors_for(target))
                    if fragment not in anchors:
                        errors.append(
                            f"{source.relative_to(ROOT)}:{line_number}: missing anchor "
                            f"#{fragment} in {target.relative_to(ROOT)}"
                        )

    if errors:
        print(f"docs-check failed ({len(errors)} error(s), {checked} local reference(s)):")
        for error in errors:
            print(f"  - {error}")
        return 1
    print(f"docs-check passed ({len(files)} Markdown file(s), {checked} local reference(s))")
    return 0


if __name__ == "__main__":
    sys.exit(main())
