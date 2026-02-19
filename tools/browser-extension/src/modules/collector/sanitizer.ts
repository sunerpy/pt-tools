import { SENSITIVE_PATTERNS } from "../../core/constants";

export function sanitizeHtml(html: string): string {
  let sanitized = html;

  for (const { pattern, replacement } of SENSITIVE_PATTERNS) {
    sanitized = sanitized.replace(pattern, replacement);
  }

  sanitized = sanitized.replace(
    /(passkey|authkey|apikey)("\s*:\s*"|=)([^"&\s<]+)/gi,
    "$1$2REMOVED",
  );
  sanitized = sanitized.replace(/("token"\s*:\s*")([^"]+)(")/gi, "$1REMOVED$3");

  return sanitized;
}
