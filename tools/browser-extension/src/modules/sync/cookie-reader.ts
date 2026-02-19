import { KNOWN_SITES, type KnownSite } from "../../core/constants";
import { t } from "../../core/i18n";
import type { SiteCookieData } from "../../core/types";

type CookieHealthStatus = "valid" | "expiring" | "expired" | "missing";

interface CookieHealthResult {
  status: CookieHealthStatus;
  expireDays: number | null;
  cookieString: string;
}

function ensureCookiesApi(): void {
  if (!chrome.cookies) {
    throw new Error(t("error.cookiePermission"));
  }
}

function toCookieString(cookies: chrome.cookies.Cookie[]): string {
  return cookies.map((cookie) => `${cookie.name}=${cookie.value}`).join("; ");
}

async function getCookiesAcrossDomains(site: KnownSite): Promise<chrome.cookies.Cookie[]> {
  ensureCookiesApi();
  const results: chrome.cookies.Cookie[] = [];
  for (const domain of site.domains) {
    const cookies = await chrome.cookies.getAll({ domain });
    results.push(...cookies);
  }
  return results;
}

async function getBestDomainCookies(site: KnownSite): Promise<chrome.cookies.Cookie[]> {
  ensureCookiesApi();
  for (const domain of site.domains) {
    const cookies = await chrome.cookies.getAll({ domain });
    if (cookies.length > 0) {
      return cookies;
    }
  }
  return [];
}

export async function readSiteCookies(domain: string): Promise<string> {
  const cookies = await chrome.cookies.getAll({ domain });
  if (cookies.length === 0) {
    return "";
  }

  return toCookieString(cookies);
}

export async function checkCookieHealth(site: KnownSite): Promise<CookieHealthResult> {
  const domainCookies = await getBestDomainCookies(site);
  const allCookies = await getCookiesAcrossDomains(site);
  const cookieString = toCookieString(domainCookies);

  if (site.cookieNames.length === 0) {
    return {
      status: "valid",
      expireDays: null,
      cookieString,
    };
  }

  const required = new Map<string, chrome.cookies.Cookie>();
  for (const cookie of allCookies) {
    if (site.cookieNames.includes(cookie.name) && !required.has(cookie.name)) {
      required.set(cookie.name, cookie);
    }
  }

  if (required.size !== site.cookieNames.length) {
    return {
      status: "missing",
      expireDays: null,
      cookieString,
    };
  }

  const nowSeconds = Date.now() / 1000;
  const remainingDays = Array.from(required.values())
    .map((cookie) => cookie.expirationDate)
    .filter((value): value is number => typeof value === "number")
    .map((expiration) => Math.floor((expiration - nowSeconds) / 86400));

  const expireDays = remainingDays.length > 0 ? Math.min(...remainingDays) : null;

  if (expireDays !== null && expireDays < 0) {
    return {
      status: "expired",
      expireDays,
      cookieString,
    };
  }

  if (expireDays !== null && expireDays <= 7) {
    return {
      status: "expiring",
      expireDays,
      cookieString,
    };
  }

  return {
    status: "valid",
    expireDays,
    cookieString,
  };
}

export async function readAllPtSiteCookies(): Promise<SiteCookieData[]> {
  const results: SiteCookieData[] = [];

  for (const site of KNOWN_SITES) {
    if (site.syncField !== "cookie") {
      continue;
    }

    let cookieString = "";
    let matchedDomain = "";

    for (const domain of site.domains) {
      const current = await readSiteCookies(domain);
      if (current) {
        cookieString = current;
        matchedDomain = domain;
        break;
      }
    }

    if (!cookieString || !matchedDomain) {
      continue;
    }

    results.push({
      siteName: site.name,
      domain: matchedDomain,
      cookies: cookieString,
      capturedAt: new Date().toISOString(),
    });
  }

  return results;
}
