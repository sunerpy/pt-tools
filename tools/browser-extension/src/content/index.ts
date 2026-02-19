import { createMessage, onMessage, sendToBackground } from "../core/messages";
import { lookupPtDomain } from "../core/pt-sites";
import type { PageType, SiteDetectedPayload } from "../core/types";
import { captureCurrentPage, detectPageType } from "../modules/collector";
import { matchKnownSite } from "../utils";

function getHostname(url: string): string {
  try {
    return new URL(url).hostname.toLowerCase();
  } catch {
    return "";
  }
}

function getPathname(url: string): string {
  try {
    return new URL(url).pathname.toLowerCase();
  } catch {
    return "";
  }
}

function isExtensionContextValid(): boolean {
  try {
    return chrome?.runtime?.id !== undefined;
  } catch {
    return false;
  }
}

function inferPageType(url: string, doc: Document): PageType {
  const detected = detectPageType(url, doc);
  if (detected !== "unknown") {
    return detected;
  }

  const pathname = getPathname(url);
  if (/login\.php|signup\.php/i.test(pathname)) return "index";
  if (/userdetails\.php/i.test(pathname)) return "userinfo";
  if (/details\.php/i.test(pathname)) return "detail";
  if (/torrents\.php|browse\.php/i.test(pathname)) return "search";
  if (/mybonus\.php/i.test(pathname)) return "bonus";

  if (doc.querySelector("table.torrents, .torrent-list, .torrent-table")) return "search";
  if (doc.querySelector("#kdescr, .torrent-description, .details")) return "detail";
  return "unknown";
}

function detectSite(url: string, doc: Document): SiteDetectedPayload {
  const pageType = inferPageType(url, doc);

  const known = matchKnownSite(url);
  if (known) {
    return {
      mode: "known",
      knownSiteId: known.id,
      detectedSchema: known.schema,
      pageType,
      url,
    };
  }

  const hostname = getHostname(url);
  const schema = lookupPtDomain(hostname);
  if (schema) {
    return {
      mode: "unknown",
      detectedSchema: schema,
      pageType,
      url,
    };
  }

  return { mode: "none", pageType: "unknown", url };
}

function isPtRelatedPage(): boolean {
  const hostname = getHostname(window.location.href);
  if (!hostname) return false;
  if (matchKnownSite(window.location.href)) return true;
  if (lookupPtDomain(hostname)) return true;
  return false;
}

onMessage("PING" as Parameters<typeof onMessage>[0], async () => ({ pong: true }));

onMessage("DETECT_SITE", async () => detectSite(window.location.href, document));

onMessage("CAPTURE_PAGE", async () => {
  const captured = captureCurrentPage();
  if (isExtensionContextValid()) {
    await sendToBackground(createMessage("PAGE_CAPTURED", captured));
  }
  return captured;
});

if (isPtRelatedPage() && isExtensionContextValid()) {
  const payload = detectSite(window.location.href, document);
  if (payload.mode !== "none") {
    void sendToBackground(createMessage("SITE_DETECTED", payload)).catch(() => {});
  }
}
