import { KNOWN_SITES } from "../core/constants";
import type { KnownSite } from "../core/constants";

export function normalizeUrl(url: string): string {
  try {
    return new URL(url).toString();
  } catch {
    return url;
  }
}

export function extractDomain(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return "";
  }
}

export function matchKnownSite(url: string): KnownSite | null {
  const domain = extractDomain(url);
  if (!domain) {
    return null;
  }

  return (
    KNOWN_SITES.find((site) =>
      site.domains.some(
        (knownDomain) => domain === knownDomain || domain.endsWith(`.${knownDomain}`),
      ),
    ) ?? null
  );
}

export function isPtLikeUrl(url: string): boolean {
  return matchKnownSite(url) !== null;
}
