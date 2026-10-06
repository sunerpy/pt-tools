import type { CapturedPage } from "../../core/types";
import { detectPageType, detectSiteSchema } from "./detector";
import { sanitizeHtml, sanitizeUrl } from "./sanitizer";

export function captureCurrentPage(): CapturedPage {
  const rawHtml = document.documentElement.outerHTML;
  const detectedSchema = detectSiteSchema(document);
  const pageType = detectPageType(window.location.href, document);
  const html = sanitizeHtml(rawHtml);

  return {
    pageType,
    url: sanitizeUrl(window.location.href),
    html,
    capturedAt: new Date().toISOString(),
    detectedSchema,
  };
}
