#!/usr/bin/env python3
"""Send a Telegram release announcement (MarkdownV2) and pin it.

Reads from environment:
    TG_BOT_TOKEN, TG_CHAT_ID  - Telegram credentials
    REL_TAG, REL_NAME, REL_URL, REL_BODY  - GitHub release info
    REPO  - 'owner/name' for fallback links

Exits non-zero if sendMessage fails. pinChatMessage failure is logged
as a warning but does NOT fail the workflow (insufficient bot rights
shouldn't block the release).
"""

import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

TOKEN = os.environ["TG_BOT_TOKEN"]
CHAT_ID = os.environ["TG_CHAT_ID"]
TAG = os.environ.get("REL_TAG", "").strip()
NAME = os.environ.get("REL_NAME", "").strip() or TAG
URL = os.environ.get("REL_URL", "").strip()
BODY = os.environ.get("REL_BODY", "").strip()
REPO = os.environ.get("REPO", "").strip()

API = f"https://api.telegram.org/bot{TOKEN}"

# MarkdownV2 reserved characters (escape outside code spans)
_MDV2_RE = re.compile(r"([_*\[\]()~`>#+\-=|{}.!\\])")


def esc(s: str) -> str:
    return _MDV2_RE.sub(r"\\\1", s)


def escape_body(body: str, max_lines: int = 30) -> str:
    """Crudely render GitHub release body as MarkdownV2.

    GitHub bodies are GFM. We do a minimal best-effort conversion:
    - Drop empty leading/trailing lines.
    - Truncate to max_lines, append a 'View on GitHub' link.
    - Escape every line as plain text (we lose GFM formatting, but
      we never emit invalid MarkdownV2 which would 400 the API).
    """
    lines = body.splitlines()
    truncated = False
    if len(lines) > max_lines:
        lines = lines[:max_lines]
        truncated = True
    out = []
    for ln in lines:
        out.append(esc(ln))
    if truncated and URL:
        out.append("")
        out.append(f"\\[ {esc('查看完整发布说明')} \\]\\(" + esc(URL) + "\\)")
    return "\n".join(out).strip()


def build_message() -> str:
    title = esc(NAME)
    tag = esc(TAG)
    rendered_body = escape_body(BODY)
    parts = [
        f"🎉 *{title}*",
        f"_{esc('版本：')}_`{tag}`",
    ]
    if rendered_body:
        parts.append("")
        parts.append(rendered_body)
    parts.append("")
    if URL:
        parts.append(f"🔗 [{esc('Release 页面')}]({esc(URL)})")
    if REPO:
        parts.append(f"📜 [{esc('CHANGELOG')}]({esc(f'https://github.com/{REPO}/blob/main/CHANGELOG.md')})")
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
    print(f"--- composed message ({len(text)} chars) ---")
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


if __name__ == "__main__":
    sys.exit(main())
