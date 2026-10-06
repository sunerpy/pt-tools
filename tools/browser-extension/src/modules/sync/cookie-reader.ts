import { KNOWN_SITES, type KnownSite } from "../../core/constants";
import { t } from "../../core/i18n";
import type { SiteCookieData } from "../../core/types";

type CookieHealthStatus = "valid" | "expiring" | "expired" | "missing";

interface CookieHealthResult {
  status: CookieHealthStatus;
  expireDays: number | null;
  cookieString: string;
  hasCookie: boolean;
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

/** 一组 Cookie 里每个必需名字的第一条 */
function requiredCookies(
  cookies: chrome.cookies.Cookie[],
  names: string[],
): Map<string, chrome.cookies.Cookie> {
  const required = new Map<string, chrome.cookies.Cookie>();
  for (const cookie of cookies) {
    if (names.includes(cookie.name) && !required.has(cookie.name)) {
      required.set(cookie.name, cookie);
    }
  }
  return required;
}

/**
 * 选出要同步的那个域的 Cookie：必须在**同一个域**里凑齐全部必需 Cookie，几个域都齐全时取必需 Cookie
 * 最晚过期的那个。原来健康检查把各个域的必需 Cookie 合在一起判定，发送的却是「第一个有任意 Cookie 的域」，
 * 旧域只剩统计 Cookie 时状态显示有效，同步却把统计 Cookie 写进后端、替换掉原来有效的凭证。
 * 没有必需名单的站点（passkey / api_key）取第一个有 Cookie 的域。
 */
async function pickDomainCookies(
  site: KnownSite,
): Promise<{ cookies: chrome.cookies.Cookie[]; complete: boolean }> {
  ensureCookiesApi();
  let firstNonEmpty: chrome.cookies.Cookie[] = [];
  let best: { cookies: chrome.cookies.Cookie[]; minExpiry: number } | null = null;
  for (const domain of site.domains) {
    const cookies = await chrome.cookies.getAll({ domain });
    if (cookies.length === 0) {
      continue;
    }
    if (site.cookieNames.length === 0) {
      return { cookies, complete: true };
    }
    if (firstNonEmpty.length === 0) {
      firstNonEmpty = cookies;
    }
    const required = requiredCookies(cookies, site.cookieNames);
    if (required.size !== site.cookieNames.length) {
      continue;
    }
    // 会话 Cookie 没有过期时间，按永不过期算
    const minExpiry = Math.min(
      ...Array.from(required.values(), (cookie) => cookie.expirationDate ?? Infinity),
    );
    if (!best || minExpiry > best.minExpiry) {
      best = { cookies, minExpiry };
    }
  }
  return best
    ? { cookies: best.cookies, complete: true }
    : { cookies: firstNonEmpty, complete: false };
}

export async function readSiteCookies(domain: string): Promise<string> {
  const cookies = await chrome.cookies.getAll({ domain });
  if (cookies.length === 0) {
    return "";
  }

  return toCookieString(cookies);
}

export async function checkCookieHealth(site: KnownSite): Promise<CookieHealthResult> {
  const picked = await pickDomainCookies(site);
  const hasCookie = picked.cookies.length > 0;

  // 对于 passkey / api_key 站点（cookieNames 为空），保号仍依赖浏览器登录态，
  // 因此只要检测到跨域的任意 cookie 即视为已获取，否则标记 missing。
  if (site.cookieNames.length === 0) {
    const allCookies = await getCookiesAcrossDomains(site);
    const crossDomainHasCookie = allCookies.length > 0;
    return {
      status: crossDomainHasCookie ? "valid" : "missing",
      expireDays: null,
      cookieString: toCookieString(picked.cookies),
      hasCookie: crossDomainHasCookie,
    };
  }

  if (!picked.complete) {
    // 没有哪个域凑齐必需 Cookie：不给出可同步的 Cookie，免得拿残缺的一组覆盖后端
    return {
      status: "missing",
      expireDays: null,
      cookieString: "",
      hasCookie,
    };
  }

  const cookieString = toCookieString(picked.cookies);
  const required = requiredCookies(picked.cookies, site.cookieNames);
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
      hasCookie,
    };
  }

  if (expireDays !== null && expireDays <= 7) {
    return {
      status: "expiring",
      expireDays,
      cookieString,
      hasCookie,
    };
  }

  return {
    status: "valid",
    expireDays,
    cookieString,
    hasCookie,
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
