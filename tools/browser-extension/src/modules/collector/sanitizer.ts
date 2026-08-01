import { SENSITIVE_PATTERNS } from "../../core/constants";

export function sanitizeHtml(html: string): string {
  let sanitized = html;

  for (const { pattern, replacement } of SENSITIVE_PATTERNS) {
    sanitized = sanitized.replace(pattern, replacement);
  }

  // Two value terminators, one per form. `key=value`: any quote, `&`, whitespace, `<`
  // or `>` ends it, so the tag's closing `'>` / `">` / `>` survives (swallowing it left
  // the attribute open and the parser absorbed the following rows). `"key":"value"`:
  // only the closing double quote ends it, so `'` and `&` inside JSON are redacted too.
  sanitized = sanitized.replace(
    /(passkey|authkey|apikey)(?:(=)[^"'&\s<>]+|("\s*:\s*")[^"<>]*("))/gi,
    "$1$2$3REMOVED$4",
  );
  sanitized = sanitized.replace(/("token"\s*:\s*")([^"]+)(")/gi, "$1REMOVED$3");

  return sanitized;
}
