#!/usr/bin/env python3
"""Send a Telegram release announcement (MarkdownV2) and pin it.

Reads from environment:
    TG_BOT_TOKEN, TG_CHAT_ID  - Telegram credentials
    REL_TAG, REL_NAME, REL_URL, REL_BODY  - GitHub release info
    REPO  - 'owner/name' for fallback links
    GITHUB_RUN_STARTED_AT  - optional ISO date; falls back to today

Exits non-zero if sendMessage fails. pinChatMessage failure is logged
as a warning but does NOT fail the workflow (insufficient bot rights
shouldn't block the release).

CLI:
    python3 tg_release_announce.py            # send for real (env-driven)
    python3 tg_release_announce.py --smoke    # print rendered preview only
    python3 tg_release_announce.py --smoke --send  # preview + actually send
"""

import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime

TOKEN = os.environ.get("TG_BOT_TOKEN", "")
CHAT_ID = os.environ.get("TG_CHAT_ID", "")
TAG = os.environ.get("REL_TAG", "").strip()
NAME = os.environ.get("REL_NAME", "").strip() or TAG
URL = os.environ.get("REL_URL", "").strip()
BODY = os.environ.get("REL_BODY", "").strip()
REPO = os.environ.get("REPO", "").strip()

API = f"https://api.telegram.org/bot{TOKEN}"

# MarkdownV2 reserved characters (full set, used outside structured tokens).
_MDV2_RE = re.compile(r"([_*\[\]()~`>#+\-=|{}.!\\])")

# Inside link URLs only `\` and `)` need to be escaped.
_URL_RE = re.compile(r"([\\\)])")

# Inside code spans only `\` and backtick need escaping.
_CODE_RE = re.compile(r"([\\`])")

# Trailing release-please noise: ` (#123)`, ` (a1b2c3d)`, or both.
# Also handles `[#123](url) ([sha](url))` style by stripping the parenthesized
# tail after we've extracted links.
_TRAILING_REF_RE = re.compile(r"\s*\((?:#?\d+|[0-9a-f]{7,40})\)\s*$")

# Version-tag style H2 such as `[0.31.0] - 2026-05-16` — pure noise after
# release-please squashes its own changelog entry under the previous H2.
_VERSION_TAG_H2 = re.compile(r"^\s*\[\d+\.\d+\.\d+(?:[^\]]*)?\]")

# Duplicate PR link artifact from release-please squash merges:
# `([#330](issues/330)) ([#330](pull/330))` → keep only the first link.
_DUP_PR_RE = re.compile(
    r"\(\[#(\d+)\]\(([^)]+)\)\)\s*\(\[#\1\]\([^)]+\)\)"
)

# Inline tokenizer: order matters — match code first, then links, then
# bold/italic/strike. Each branch carries a named group so we can dispatch.
_INLINE_RE = re.compile(
    r"(?P<code>`[^`\n]+`)"
    r"|(?P<link>\[(?P<ltxt>[^\]\n]*)\]\((?P<lurl>[^)\n]+)\))"
    r"|(?P<bold>\*\*(?P<btxt>[^*\n]+)\*\*)"
    r"|(?P<strike>~~(?P<stxt>[^~\n]+)~~)"
    r"|(?P<italic>(?<![A-Za-z0-9])\*(?P<itxt>[^*\n]+)\*(?![A-Za-z0-9]))"
    r"|(?P<emph>(?<![A-Za-z0-9_])_(?P<etxt>[^_\n]+)_(?![A-Za-z0-9_]))"
)

MAX_CHARS = 3600  # TG sendMessage limit is 4096 UTF-16 chars; reserve room for header + footer

# Representative sample mirroring the real v0.40.3 release-body shape:
# a version-tag H2, a real Bug Fixes section (with a release-please detail
# sub-block), and dependency/chore walls of dependabot bumps that must fold.
_SMOKE_BODY = '''
## What's Changed

## [0.31.0] - 2026-07-08

### Features

* feat(rss/notify): Sprint 1 — backend skeleton + all mode + throttle ([#101](https://github.com/sunerpy/pt-tools/pull/101)) ([a1b2c3d](https://github.com/sunerpy/pt-tools/commit/a1b2c3d))
* feat(chatops): bilingual command descriptions ([#102](https://github.com/sunerpy/pt-tools/pull/102)) ([4d5e6f7](https://github.com/sunerpy/pt-tools/commit/4d5e6f7))

### Bug Fixes

* fix(chatops/qq): WebSocket half-open detection ([#103](https://github.com/sunerpy/pt-tools/pull/103)) ([8a9b0c1](https://github.com/sunerpy/pt-tools/commit/8a9b0c1))
      - detail sub-line that must not be counted

### Dependencies (Frontend)

- **pnpm**: Bump vue-tsc from 3.3.5 to 3.3.6 in /web/frontend ([#441](https://github.com/sunerpy/pt-tools/pull/441))
Bumps vue-tsc from 3.3.5 to 3.3.6.
      ---
      updated-dependencies:
      - dependency-name: vue-tsc
      ...
- **pnpm**: Bump oxfmt from 0.56.0 to 0.57.0 in /web/frontend ([#442](https://github.com/sunerpy/pt-tools/pull/442))
- **pnpm**: Bump vite from 8.1.0 to 8.1.3 in /web/frontend ([#445](https://github.com/sunerpy/pt-tools/pull/445))
- **pnpm**: Bump vitest from 4.1.9 to 4.1.10 in /web/frontend ([#443](https://github.com/sunerpy/pt-tools/pull/443))
- **pnpm**: Bump oxlint from 1.71.0 to 1.72.0 in /web/frontend ([#444](https://github.com/sunerpy/pt-tools/pull/444))
- **pnpm**: Bump eslint from 9.0.0 to 9.1.0 in /web/frontend ([#447](https://github.com/sunerpy/pt-tools/pull/447))

### Dependencies (Go)

- **go**: Bump golang.org/x/text from 0.38.0 to 0.39.0 ([#440](https://github.com/sunerpy/pt-tools/pull/440))
- **go**: Bump gorm.io/gorm from 1.31.1 to 1.31.2 ([#430](https://github.com/sunerpy/pt-tools/pull/430))

### Chores

- chore(deps): update actions/checkout to v5 ([#450](https://github.com/sunerpy/pt-tools/pull/450))
- chore: bump go toolchain metadata ([#451](https://github.com/sunerpy/pt-tools/pull/451))
- chore: refresh generated mocks ([#452](https://github.com/sunerpy/pt-tools/pull/452))

**Full Changelog**: https://github.com/sunerpy/pt-tools/compare/v0.30.1...v0.31.0
'''


def esc(s: str) -> str:
    """Full MarkdownV2 escape for plain text."""
    return _MDV2_RE.sub(r"\\\1", s)


def _esc_url(s: str) -> str:
    return _URL_RE.sub(r"\\\1", s)


def _esc_code(s: str) -> str:
    return _CODE_RE.sub(r"\\\1", s)


def _convert_inline(s: str) -> str:
    """Translate inline GFM tokens to MarkdownV2, escaping the rest."""
    out = []
    pos = 0
    for m in _INLINE_RE.finditer(s):
        # Escape the gap before this match as plain text.
        if m.start() > pos:
            out.append(esc(s[pos:m.start()]))
        if m.group("code"):
            inner = m.group("code")[1:-1]
            out.append("`" + _esc_code(inner) + "`")
        elif m.group("link"):
            txt = m.group("ltxt")
            url = m.group("lurl")
            out.append("[" + esc(txt) + "](" + _esc_url(url) + ")")
        elif m.group("bold"):
            out.append("*" + esc(m.group("btxt")) + "*")
        elif m.group("strike"):
            out.append("~" + esc(m.group("stxt")) + "~")
        elif m.group("italic"):
            out.append("_" + esc(m.group("itxt")) + "_")
        elif m.group("emph"):
            out.append("_" + esc(m.group("etxt")) + "_")
        pos = m.end()
    if pos < len(s):
        out.append(esc(s[pos:]))
    return "".join(out)


def _strip_trailing_refs(content: str) -> str:
    """Drop the noisy ` (#123) (a1b2c3d)` tail — possibly twice."""
    prev = None
    while prev != content:
        prev = content
        content = _TRAILING_REF_RE.sub("", content)
    return content


def _localize_section(title: str) -> str:
    """Translate common release-please / git-cliff English section headings to Chinese.

    Falls back to original title when not in the mapping.
    """
    t = title.strip()
    if _is_dependency_section(t):
        return "依赖更新"
    if _is_chore_section(t):
        return "杂项"
    mapping = {
        "What's Changed": "更新概览",
        "Whats Changed": "更新概览",
        "Changelog": "更新日志",
        "Features": "新功能",
        "Feature": "新功能",
        "New Features": "新功能",
        "Bug Fixes": "Bug 修复",
        "Bug Fix": "Bug 修复",
        "Fixes": "修复",
        "Performance Improvements": "性能优化",
        "Performance": "性能优化",
        "Refactors": "代码重构",
        "Refactor": "代码重构",
        "Documentation": "文档更新",
        "Docs": "文档更新",
        "Tests": "测试",
        "Test": "测试",
        "Builds": "构建",
        "Build": "构建",
        "Build System": "构建系统",
        "CI": "CI/CD",
        "Continuous Integration": "CI/CD",
        "Style": "样式调整",
        "Styles": "样式调整",
        "Chores": "杂项",
        "Chore": "杂项",
        "Reverts": "回滚",
        "Revert": "回滚",
        "Breaking Changes": "破坏性变更",
        "Breaking": "破坏性变更",
        "Security": "安全修复",
        "Deprecations": "废弃",
        "Dependencies": "依赖更新",
        "Dependency Updates": "依赖更新",
    }
    return mapping.get(t, title)


def _section_emoji(title: str) -> str:
    """Map a release-please section heading to a contextual emoji.

    Empty string returned when no match — caller falls back to plain bold heading.
    """
    t = title.strip().lower()
    if t in ("what's changed", "whats changed", "changelog", "更新内容", "变更", "更新概览", "更新日志"):
        return "📋"
    if _is_dependency_section(t):
        return "🧩"
    mapping = (
        ("feature", "✨"),
        ("功能", "✨"),
        ("bug fix", "🐛"),
        ("修复", "🐛"),
        ("fix", "🐛"),
        ("performance", "⚡️"),
        ("性能", "⚡️"),
        ("perf", "⚡️"),
        ("refactor", "♻️"),
        ("重构", "♻️"),
        ("doc", "📚"),
        ("文档", "📚"),
        ("test", "✅"),
        ("测试", "✅"),
        ("ci", "🔧"),
        ("build", "📦"),
        ("构建", "📦"),
        ("style", "🎨"),
        ("样式", "🎨"),
        ("chore", "🧹"),
        ("杂项", "🧹"),
        ("revert", "⏪"),
        ("回滚", "⏪"),
        ("break", "💥"),
        ("破坏", "💥"),
        ("security", "🔐"),
        ("安全", "🔐"),
        ("deprecation", "⚠️"),
        ("废弃", "⚠️"),
        ("dependency", "🧩"),
        ("依赖", "🧩"),
    )
    for needle, emoji in mapping:
        if needle in t:
            return emoji
    return "📌"


def _conventional_emoji(content: str) -> str:
    """Detect conventional commits prefix in a bullet body and return emoji.

    Looks at the start of the content for `feat:` / `fix:` / `feat(scope):` etc.
    Returns "" if no conventional prefix detected (caller keeps generic bullet).
    """
    m = re.match(r"^(?P<type>[a-zA-Z]+)(?:\([^)]+\))?!?:\s", content)
    if not m:
        return ""
    t = m.group("type").lower()
    mapping = {
        "feat": "✨",
        "fix": "🐛",
        "perf": "⚡️",
        "refactor": "♻️",
        "docs": "📚",
        "test": "✅",
        "ci": "🔧",
        "build": "📦",
        "style": "🎨",
        "chore": "🧹",
        "revert": "⏪",
        "deps": "🧩",
    }
    return mapping.get(t, "")


def _convert_line(ln: str) -> str:
    if ln.startswith("#### "):
        title = ln[5:].strip()
        emoji = _section_emoji(title)
        zh = _localize_section(title)
        return f"{emoji} *{esc(zh)}*" if emoji else f"*{esc(zh)}*"
    if ln.startswith("### "):
        title = ln[4:].strip()
        emoji = _section_emoji(title)
        zh = _localize_section(title)
        return f"{emoji} *{esc(zh)}*" if emoji else f"*{esc(zh)}*"
    if ln.startswith("## "):
        title = ln[3:].strip()
        if _VERSION_TAG_H2.match(title):
            return ""
        emoji = _section_emoji(title)
        zh = _localize_section(title)
        return f"\n{emoji} *{esc(zh)}*" if emoji else f"\n*{esc(zh)}*"
    if ln.startswith("# "):
        title = ln[2:].strip()
        if _VERSION_TAG_H2.match(title):
            return ""
        emoji = _section_emoji(title)
        zh = _localize_section(title)
        return f"\n{emoji} *{esc(zh)}*" if emoji else f"\n*{esc(zh)}*"

    m = re.match(r"^(\s*)([*\-+])\s+(.*)$", ln)
    if m:
        indent, _, content = m.groups()
        content = _strip_trailing_refs(content)
        indent_len = len(indent.expandtabs())
        if indent_len >= 6:
            bullet = "▸"
        elif indent_len >= 2:
            bullet = "◦"
        else:
            bullet = "•"
        if indent_len < 2:
            type_emoji = _conventional_emoji(content)
            if type_emoji:
                bullet = type_emoji
        return f"{indent}{bullet} {_convert_inline(content)}"

    if not ln.strip():
        return ""
    return _convert_inline(ln)


def _strip_release_please_noise(lines):
    out = []
    skip_until_next_h2 = False
    for ln in lines:
        s = ln.rstrip()
        stripped = s.lstrip()
        if stripped.startswith("**Full Changelog**:"):
            continue
        if stripped.lower().startswith("full changelog:"):
            continue
        if stripped.startswith("## ") or stripped.startswith("# "):
            title = stripped.lstrip("#").strip().lower()
            if title in ("installation", "docker images", "from binary"):
                skip_until_next_h2 = True
                continue
            skip_until_next_h2 = False
        if stripped.startswith("### "):
            title = stripped[4:].strip().lower()
            if title in ("using docker (recommended)", "from binary", "docker images", "browser extension"):
                skip_until_next_h2 = True
                continue
        if skip_until_next_h2:
            continue
        out.append(s)
    return out


def _heading_level(ln: str) -> int:
    """Return the markdown heading level (# count) for a line, else 0.

    Only treats `#`+space as a heading (`#foo` is not a heading).
    """
    s = ln.lstrip()
    i = 0
    while i < len(s) and s[i] == "#":
        i += 1
    if i == 0 or i > 6:
        return 0
    if i < len(s) and s[i] == " ":
        return i
    return 0


def _heading_title(ln: str) -> str:
    """Strip leading `#`s and surrounding whitespace from a heading line."""
    return ln.lstrip().lstrip("#").strip()


def _is_dependency_section(title: str) -> bool:
    """True when a heading title is dependency-family (handles subsections)."""
    t = title.strip().lower()
    return any(n in t for n in ("dependencies", "dependency updates", "dependency", "deps", "依赖"))


def _is_chore_section(title: str) -> bool:
    """True when a heading title is chore-family."""
    t = title.strip().lower()
    return any(n in t for n in ("chores", "chore", "杂项"))


def _is_noise_section(title: str) -> bool:
    """True when a section heading belongs to the dependency/chore family.

    Robust to both English (pre-localization body) and the localized Chinese,
    and to release-please subsection suffixes like `Dependencies (Frontend)`.
    Never matches Features / Bug Fixes / Performance / Refactor / Docs /
    Security / Breaking / etc.
    """
    return _is_dependency_section(title) or _is_chore_section(title)


def _is_top_level_bullet(ln: str) -> bool:
    """True for a top-level bullet (`* `/`- `/`+ `) with indent < 2 columns.

    Mirrors how `_convert_line` classifies top-level bullets so counts match
    the rendered `•` bullets.
    """
    m = re.match(r"^(\s*)([*\-+])\s+", ln)
    if not m:
        return False
    return len(m.group(1).expandtabs()) < 2


def _fold_noise_sections(lines, url: str):
    """Collapse dependency/chore sections into a one-line count summary.

    Operates on raw GFM lines BEFORE per-line conversion, so the important
    Features/Bug Fixes sections always survive later truncation. Section
    boundaries are heading-based: a section runs from its heading until the
    next heading of the same-or-higher level (`<=` the number of `#`).

    For each folded section the heading line is kept as-is (so emoji +
    localization still render), and its body is replaced by a single bullet:
        * 本次含 {N} 项，详见 [Release 页面]({url})
    where N is the count of TOP-LEVEL bullets. A folded section with 0
    top-level bullets is dropped entirely (heading + nothing).
    """
    out = []
    i = 0
    n = len(lines)
    while i < n:
        ln = lines[i]
        level = _heading_level(ln)
        if level and _is_noise_section(_heading_title(ln)):
            # Collect the section body: lines until the next heading whose
            # level is same-or-higher (<= this heading's level).
            j = i + 1
            count = 0
            while j < n:
                nxt_level = _heading_level(lines[j])
                if nxt_level and nxt_level <= level:
                    break
                if _is_top_level_bullet(lines[j]):
                    count += 1
                j += 1
            if count > 0:
                out.append(ln)
                out.append(f"* 本次含 {count} 项，详见 [Release 页面]({url})")
            # count == 0: drop heading + body entirely.
            i = j
            continue
        out.append(ln)
        i += 1
    return out


def _collapse_blanks(lines):
    out = []
    blank = False
    for ln in lines:
        if ln.strip() == "":
            if blank:
                continue
            blank = True
            out.append("")
        else:
            blank = False
            out.append(ln)
    # trim leading/trailing blanks
    while out and out[0] == "":
        out.pop(0)
    while out and out[-1] == "":
        out.pop()
    return out


def _maybe_truncate(text: str, url: str) -> str:
    if len(text) <= MAX_CHARS:
        return text

    lines = text.split("\n")

    pruned = [ln for ln in lines if not ln.lstrip().startswith("▸")]
    candidate = "\n".join(pruned)
    if len(candidate) <= MAX_CHARS:
        return candidate

    pruned = [ln for ln in pruned if not ln.lstrip().startswith("◦")]
    candidate = "\n".join(pruned)
    if len(candidate) <= MAX_CHARS:
        return candidate

    truncated = candidate[:MAX_CHARS]
    cut = truncated.rfind("\n•")
    if cut < 0:
        cut = truncated.rfind("\n")
    if cut > 0:
        truncated = truncated[:cut]
    omitted = candidate.count("\n•") - truncated.count("\n•")
    if omitted < 1:
        omitted = 1
    suffix = f"\n\n_…还有 {omitted} 项已省略，[查看完整说明]({_esc_url(url)})_"
    return truncated.rstrip() + suffix


def _preprocess_body(body: str) -> str:
    return _DUP_PR_RE.sub(r"([#\1](\2))", body)


def _gfm_to_markdownv2(body: str, url_for_truncation: str) -> str:
    """Convert GFM body to MarkdownV2 preserving headings, bullets, inline."""
    if not body:
        return ""
    body = _preprocess_body(body)
    lines = body.splitlines()
    lines = _strip_release_please_noise(lines)
    lines = _fold_noise_sections(lines, url_for_truncation)
    lines = _collapse_blanks(lines)
    converted = [_convert_line(ln) for ln in lines]
    text = "\n".join(converted)
    # Squash any 3+ newlines that the heading rule may have introduced.
    text = re.sub(r"\n{3,}", "\n\n", text).strip()
    if url_for_truncation:
        text = _maybe_truncate(text, url_for_truncation)
    return text


def _release_date() -> str:
    raw = os.environ.get("GITHUB_RUN_STARTED_AT", "").strip()
    if raw:
        try:
            return datetime.fromisoformat(raw.replace("Z", "+00:00")).strftime("%Y-%m-%d")
        except Exception:
            pass
    return datetime.now().strftime("%Y-%m-%d")


def _previous_tag(tag: str) -> str:
    """Best-effort: find the prior tag for `git log` range."""
    try:
        r = subprocess.run(
            ["git", "describe", "--tags", "--abbrev=0", f"{tag}^"],
            capture_output=True, text=True, timeout=5,
        )
        if r.returncode == 0:
            return r.stdout.strip()
    except Exception:
        pass
    return ""


def _contributors_line(tag: str) -> str:
    """Run git log to gather unique contributor names. Silent on failure."""
    if not tag:
        return ""
    prev = _previous_tag(tag)
    rng = f"{prev}..{tag}" if prev else tag
    try:
        r = subprocess.run(
            ["git", "log", "--format=%aN", rng],
            capture_output=True, text=True, timeout=8,
        )
        if r.returncode != 0:
            return ""
        names = sorted({n.strip() for n in r.stdout.splitlines() if n.strip()})
        # Filter out common bot accounts
        names = [n for n in names if not re.search(r"\[bot\]|github-actions|release-please", n, re.I)]
        if not names:
            return ""
        joined = "、".join(esc(n) for n in names[:20])
        more = "" if len(names) <= 20 else esc(f" 等 {len(names)} 位")
        return f"👥 {esc('感谢贡献者：')}{joined}{more}"
    except Exception:
        return ""


def build_message() -> str:
    title = esc(NAME)
    tag = esc(TAG)
    date = esc(_release_date())
    rendered_body = _gfm_to_markdownv2(BODY, URL)

    parts = [
        f"🎉🎉🎉  *{title}*  🎉🎉🎉",
        "",
        f"🏷 *{esc('版本')}* `{_esc_code(TAG)}`",
        f"📅 *{esc('发布时间')}* {date}",
        f"🤖 *{esc('发布方式')}* {esc('GitHub Actions 自动发布')}",
    ]
    if rendered_body:
        parts.append("")
        parts.append(rendered_body)

    parts.append("")
    parts.append("━━━━━━━━━━━━━━━━━━")
    parts.append(f"🔗 *{esc('快速访问')}*")
    if URL:
        parts.append(f"  ▸ [{esc('GitHub Release 页面')}]({_esc_url(URL)})")
    if REPO:
        changelog_url = f"https://github.com/{REPO}/blob/main/CHANGELOG.md"
        parts.append(f"  ▸ [{esc('完整 CHANGELOG')}]({_esc_url(changelog_url)})")
        docker_hub_url = "https://hub.docker.com/r/sunerpy/pt-tools/tags"
        parts.append(f"  ▸ [{esc('Docker Hub 镜像')}]({_esc_url(docker_hub_url)})")

    contrib = _contributors_line(TAG)
    if contrib:
        parts.append("")
        parts.append(contrib)

    parts.append("")
    parts.append(f"🐳 *{esc('Docker 一键拉取')}*")
    parts.append("```bash")
    parts.append(f"docker pull sunerpy/pt-tools:{TAG}")
    parts.append("```")
    parts.append("")
    parts.append(f"💬 {esc('遇到问题？欢迎在 GitHub 提 issue 或在群里反馈 🙏')}")
    return "\n".join(parts)


def http_post(path: str, data: dict) -> dict:
    body = urllib.parse.urlencode(data).encode()
    req = urllib.request.Request(f"{API}/{path}", data=body)
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return json.load(r)
    except urllib.error.HTTPError as e:
        try:
            return json.loads(e.read().decode())
        except Exception:
            return {"ok": False, "error_code": e.code, "description": str(e)}


def main() -> int:
    text = build_message()
    print(f"--- composed message ({len(text)} chars, {len(text.encode('utf-8'))} bytes) ---")
    print(text)
    print("--- end composed ---")

    resp = http_post("sendMessage", {
        "chat_id": CHAT_ID,
        "text": text,
        "parse_mode": "MarkdownV2",
        "disable_web_page_preview": "true",
    })
    print(f"sendMessage: {json.dumps(resp, ensure_ascii=False)[:600]}")
    if not resp.get("ok"):
        print("ERROR: sendMessage failed", file=sys.stderr)
        return 1

    msg_id = resp["result"]["message_id"]

    pin = http_post("pinChatMessage", {
        "chat_id": CHAT_ID,
        "message_id": str(msg_id),
        "disable_notification": "false",
    })
    print(f"pinChatMessage: {json.dumps(pin, ensure_ascii=False)}")
    if not pin.get("ok"):
        # WARN — don't fail the workflow on insufficient pin rights
        print("WARN: pinChatMessage failed (bot may lack admin/pin rights). Release still announced.", file=sys.stderr)

    return 0


def _smoke_test():
    """Manual smoke test — preview the rendered output without a real release."""
    sample_body = _SMOKE_BODY
    os.environ.setdefault('REL_TAG', 'v0.31.0-smoke')
    os.environ.setdefault('REL_NAME', 'pt-tools v0.31.0 (smoke test)')
    os.environ.setdefault('REL_URL', 'https://github.com/sunerpy/pt-tools/releases/tag/v0.31.0')
    os.environ.setdefault('REL_BODY', sample_body)
    os.environ.setdefault('REPO', 'sunerpy/pt-tools')

    global TAG, NAME, URL, BODY, REPO
    TAG = os.environ['REL_TAG']
    NAME = os.environ['REL_NAME']
    URL = os.environ['REL_URL']
    BODY = os.environ['REL_BODY']
    REPO = os.environ['REPO']

    text = build_message()
    print(text)
    print(f"\n--- length: {len(text)} chars, {len(text.encode('utf-8'))} bytes ---")


def _selftest() -> int:
    """Assert the noise-fold behavior against a representative sample body.

    Verifies dependency/chore walls collapse to count summaries while real
    Features/Bug Fixes sections survive untouched.
    """
    url = "https://github.com/sunerpy/pt-tools/releases/tag/v0.31.0"
    rendered = _gfm_to_markdownv2(_SMOKE_BODY, url)
    failures = []

    def check(cond, msg):
        if not cond:
            failures.append(msg)

    check("本次含 6 项" in rendered, "Dependencies (Frontend) should fold to 6 项")
    check("本次含 2 项" in rendered, "Dependencies (Go) should fold to 2 项")
    check("本次含 3 项" in rendered, "Chores should fold to 3 项")

    for token in ("Bump", "vue-tsc", "oxlint", "vitest", "golang.org/x/text", "updated-dependencies"):
        check(token not in rendered, f"noise token {token!r} should be gone")

    check("WebSocket" in rendered, "Bug Fixes bullet content should survive")
    check("bilingual" in rendered, "Features bullet content should survive")

    check("🧩 *依赖更新*" in rendered, "dependency heading should localize with emoji")
    check("🧹 *杂项*" in rendered, "chore heading should localize with emoji")
    check("🐛 *Bug 修复*" in rendered, "Bug Fixes heading should localize with emoji")

    summary_count = rendered.count("本次含")
    check(summary_count == 3, f"expected 3 fold summaries, got {summary_count}")

    if failures:
        print("SELFTEST FAILED:", file=sys.stderr)
        for f in failures:
            print(f"  - {f}", file=sys.stderr)
        print("--- rendered ---", file=sys.stderr)
        print(rendered, file=sys.stderr)
        return 1
    print("SELFTEST PASSED — all assertions hold.")
    print("--- rendered ---")
    print(rendered)
    return 0


if __name__ == "__main__":
    if "--selftest" in sys.argv:
        sys.exit(_selftest())
    if "--smoke" in sys.argv:
        _smoke_test()
        if "--send" in sys.argv:
            sys.exit(main())
        sys.exit(0)
    sys.exit(main())
