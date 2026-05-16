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

MAX_BYTES = 3500  # leave headroom under TG's 4096 limit for header + footer


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


def _convert_line(ln: str) -> str:
    # Preserve fenced/indented code roughly: lines starting with 4 spaces or
    # tab → wrap as code (escape only backtick/backslash).
    if ln.startswith("    ") or ln.startswith("\t"):
        return "`" + _esc_code(ln.lstrip()) + "`"

    if ln.startswith("#### "):
        return "*" + esc(ln[5:].strip()) + "*"
    if ln.startswith("### "):
        return "*" + esc(ln[4:].strip()) + "*"
    if ln.startswith("## "):
        return "\n*" + esc(ln[3:].strip()) + "*"
    if ln.startswith("# "):
        return "\n*" + esc(ln[2:].strip()) + "*"

    m = re.match(r"^(\s*)([*\-+])\s+(.*)$", ln)
    if m:
        indent, _, content = m.groups()
        content = _strip_trailing_refs(content)
        bullet = "◦" if len(indent) >= 2 else "•"
        return f"{indent}{bullet} {_convert_inline(content)}"

    if not ln.strip():
        return ""
    return _convert_inline(ln)


def _strip_release_please_noise(lines):
    out = []
    for ln in lines:
        s = ln.rstrip()
        # Drop "Full Changelog:" lines — URL is provided separately.
        if s.lstrip().startswith("**Full Changelog**:"):
            continue
        if s.lstrip().lower().startswith("full changelog:"):
            continue
        out.append(s)
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
    if len(text.encode("utf-8")) <= MAX_BYTES:
        return text
    # Truncate at last `•` bullet boundary inside the byte budget.
    encoded = text.encode("utf-8")[:MAX_BYTES]
    truncated = encoded.decode("utf-8", errors="ignore")
    # Find last bullet line start
    cut = truncated.rfind("\n•")
    if cut < 0:
        cut = truncated.rfind("\n")
    if cut > 0:
        truncated = truncated[:cut]
    omitted = text.count("\n•") - truncated.count("\n•")
    if omitted < 1:
        omitted = 1
    suffix = f"\n\n_…还有 {omitted} 项已省略，[查看完整说明]({_esc_url(url)})_"
    return truncated.rstrip() + suffix


def _gfm_to_markdownv2(body: str, url_for_truncation: str) -> str:
    """Convert GFM body to MarkdownV2 preserving headings, bullets, inline."""
    if not body:
        return ""
    lines = body.splitlines()
    lines = _strip_release_please_noise(lines)
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
        f"🎉 *{title}*",
        f"🏷 `{_esc_code(TAG)}` · 📅 {date} · 🤖 {esc('自动发布')}",
    ]
    if rendered_body:
        parts.append("")
        parts.append(rendered_body)

    parts.append("")
    if URL:
        parts.append(f"🔗 [{esc('Release 页面')}]({_esc_url(URL)})")
    if REPO:
        changelog_url = f"https://github.com/{REPO}/blob/main/CHANGELOG.md"
        parts.append(f"📜 [{esc('CHANGELOG')}]({_esc_url(changelog_url)})")

    contrib = _contributors_line(TAG)
    if contrib:
        parts.append(contrib)

    parts.append("")
    parts.append("```bash")
    parts.append("# Docker")
    parts.append(f"docker pull sunerpy/pt-tools:{TAG}")
    parts.append("```")
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
    sample_body = '''
## What's Changed

### Features

* feat(rss/notify): Sprint 1 — backend skeleton + all mode + throttle ([#101](https://github.com/sunerpy/pt-tools/pull/101)) ([a1b2c3d](https://github.com/sunerpy/pt-tools/commit/a1b2c3d))
* feat(chatops): bilingual command descriptions ([#102](https://github.com/sunerpy/pt-tools/pull/102)) ([4d5e6f7](https://github.com/sunerpy/pt-tools/commit/4d5e6f7))

### Bug Fixes

* fix(chatops/qq): WebSocket half-open detection ([#103](https://github.com/sunerpy/pt-tools/pull/103)) ([8a9b0c1](https://github.com/sunerpy/pt-tools/commit/8a9b0c1))

### Documentation

* docs(chatops): retake screenshots at 1920x1080

**Full Changelog**: https://github.com/sunerpy/pt-tools/compare/v0.30.1...v0.31.0
'''
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


if __name__ == "__main__":
    if "--smoke" in sys.argv:
        _smoke_test()
        if "--send" in sys.argv:
            sys.exit(main())
        sys.exit(0)
    sys.exit(main())
