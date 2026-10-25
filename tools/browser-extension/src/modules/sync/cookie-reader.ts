import { KNOWN_SITES, type KnownSite } from "../../core/constants";
import { t } from "../../core/i18n";
import type { SiteCookieData } from "../../core/types";

type CookieHealthStatus = "valid" | "expiring" | "expired" | "missing";

interface CookieHealthResult {
  status: CookieHealthStatus;
  expireDays: number | null;
  cookieString: string;
  hasCookie: boolean;
  /** 缺失时还差哪几个必需 Cookie（按最接近凑齐的那个域名算）；其他状态为空 */
  missingCookies: string[];
}

function ensureCookiesApi(): void {
  if (!chrome.cookies) {
    throw new Error(t("error.cookiePermission"));
  }
}

function toCookieString(cookies: chrome.cookies.Cookie[]): string {
  return cookies.map((cookie) => `${cookie.name}=${cookie.value}`).join("; ");
}

/**
 * 浏览器访问这个域名时会带上的 Cookie。按网址取，和浏览器实际发给站点的一致：包括设在上一级域名上的。
 * 以前按 domain 过滤，只认这个域名和它的子域名，把 Cookie 设在上一级域名上的站点登录着也显示「缺失」。
 */
async function siteCookies(domain: string): Promise<chrome.cookies.Cookie[]> {
  return chrome.cookies.getAll({ url: `https://${domain}/` });
}

async function getCookiesAcrossDomains(site: KnownSite): Promise<chrome.cookies.Cookie[]> {
  ensureCookiesApi();
  const results: chrome.cookies.Cookie[] = [];
  for (const domain of site.domains) {
    const cookies = await siteCookies(domain);
    results.push(...cookies);
  }
  return results;
}

/**
 * 新版 NexusPHP（xiaomlove，include/functions.php 的 logincookie）登录只设一个 c_secure_pass：值是 base64 的
 * {"user_id":…,"expires":…}.签名（PHP 写 Cookie 时把 = + / 编成 %XX），没有 c_secure_uid、c_secure_tracker_ssl（星陨阁）。
 * 旧版的 c_secure_pass 是 md5，单独在时仍算缺失。
 */
function isNexusAuthToken(value: string): boolean {
  let raw = value;
  try {
    raw = decodeURIComponent(value);
  } catch {
    // 不是合法的 %XX 编码：按原样解
  }
  try {
    return atob(raw).startsWith('{"user_id":');
  } catch {
    return false;
  }
}

/** 这一组 Cookie 要凑齐的名字：c_secure_pass 是新版 NexusPHP 的签名令牌时只要它一个，否则按站点的必需名单 */
function namesFor(site: KnownSite, cookies: chrome.cookies.Cookie[]): string[] {
  if (site.cookieNames.includes("c_secure_pass")) {
    const pass = cookies.find((cookie) => cookie.name === "c_secure_pass");
    if (pass && isNexusAuthToken(pass.value)) {
      return ["c_secure_pass"];
    }
  }
  return site.cookieNames;
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
): Promise<{ cookies: chrome.cookies.Cookie[]; complete: boolean; missing: string[] }> {
  ensureCookiesApi();
  let firstNonEmpty: chrome.cookies.Cookie[] = [];
  // 没有哪个域名凑齐时，按凑得最多的那个域名说还差哪几个
  let missing = [...site.cookieNames];
  let best: { cookies: chrome.cookies.Cookie[]; minExpiry: number } | null = null;
  for (const domain of site.domains) {
    const cookies = await siteCookies(domain);
    if (cookies.length === 0) {
      continue;
    }
    if (site.cookieNames.length === 0) {
      return { cookies, complete: true, missing: [] };
    }
    if (firstNonEmpty.length === 0) {
      firstNonEmpty = cookies;
    }
    const names = namesFor(site, cookies);
    const required = requiredCookies(cookies, names);
    if (required.size !== names.length) {
      const lacking = names.filter((name) => !required.has(name));
      if (lacking.length < missing.length) {
        missing = lacking;
      }
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
    ? { cookies: best.cookies, complete: true, missing: [] }
    : { cookies: firstNonEmpty, complete: false, missing };
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
      missingCookies: [],
    };
  }

  if (!picked.complete) {
    // 没有哪个域凑齐必需 Cookie：不给出可同步的 Cookie，免得拿残缺的一组覆盖后端
    return {
      status: "missing",
      expireDays: null,
      cookieString: "",
      hasCookie,
      missingCookies: picked.missing,
    };
  }

  const cookieString = toCookieString(picked.cookies);
  const required = requiredCookies(picked.cookies, namesFor(site, picked.cookies));
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
      missingCookies: [],
    };
  }

  if (expireDays !== null && expireDays <= 7) {
    return {
      status: "expiring",
      expireDays,
      cookieString,
      hasCookie,
      missingCookies: [],
    };
  }

  return {
    status: "valid",
    expireDays,
    cookieString,
    hasCookie,
    missingCookies: [],
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
