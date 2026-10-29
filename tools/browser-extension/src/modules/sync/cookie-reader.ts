import { KNOWN_SITES, type KnownSite } from "../../core/constants";
import { t } from "../../core/i18n";
import type { SiteCookieData } from "../../core/types";

/**
 * Cookie 状态：valid / expiring / expired 是认出了登录 Cookie 的格式（站点自己声明的、NexusPHP 新旧两种），可以直接同步；
 * unverified 是有像登录用的 Cookie、格式认不出（BYR 现在是 auth_token / refresh_token），整组推给 pt-tools，
 * 由它先拿这组 Cookie 实际登录一次、登录上了才保存；missing 是没有登录 Cookie（只有统计、Cloudflare 这类）。
 */
export type CookieHealthStatus = "valid" | "expiring" | "expired" | "unverified" | "missing";

/** 认出的登录格式：declared 是站点自己声明的一组，nexus-token 是新版 NexusPHP 的签名令牌，nexus 是旧版的 uid + pass */
export type CookieFormat = "declared" | "nexus-token" | "nexus";

interface CookieHealthResult {
  status: CookieHealthStatus;
  expireDays: number | null;
  /** 可以推给 pt-tools 的 Cookie；missing 时为空 */
  cookieString: string;
  hasCookie: boolean;
  /** 认出的登录格式；unverified、missing 时为空 */
  format: CookieFormat | null;
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
 * https 和 http 两边都取：有的站点（OpenCD）没登录时把 https 跳到 http，登录 Cookie 是在 http 下设的，
 * 浏览器按协议区分 Cookie 时只按 https 取会漏掉。同名、同域、同路径的取 https 那一条。
 */
async function siteCookies(domain: string): Promise<chrome.cookies.Cookie[]> {
  const [secure, plain] = await Promise.all([
    chrome.cookies.getAll({ url: `https://${domain}/` }),
    chrome.cookies.getAll({ url: `http://${domain}/` }),
  ]);
  const seen = new Set<string>();
  return [...secure, ...plain].filter((cookie) => {
    const key = `${cookie.name}\u0000${cookie.domain}\u0000${cookie.path}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

/** 第一个有 Cookie 的域名下的全部 Cookie（passkey / api_key 站点用，它们不同步 Cookie） */
async function firstDomainCookies(site: KnownSite): Promise<chrome.cookies.Cookie[]> {
  for (const domain of site.domains) {
    const cookies = await siteCookies(domain);
    if (cookies.length > 0) {
      return cookies;
    }
  }
  return [];
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

/**
 * 没登录也会有的 Cookie：统计（Google、百度、友盟、Clarity、Matomo）、Cloudflare、语言与主题偏好，
 * 以及 NexusPHP 的 c_secure_ssl / c_secure_tracker_ssl / c_secure_login（只是偏好，单独在时说明不了登录着）。
 * 只剩这些的站点算没登录；别的 Cookie 都可能是登录用的。
 */
const NON_LOGIN_COOKIE =
  /^(?:_ga|_gid|_gat|__utm[a-z]|_pk_(?:id|ses|ref)|Hm_l(?:vt|pvt)_|HMACCOUNT|CNZZDATA|UM_distinctid|_clck|_clsk|cf_clearance$|__cf_bm$|__cfduid$|_cfuvid$|cf_chl_|__cflb$|cf_use_ob$|c_lang_folder$|c_secure_(?:ssl|tracker_ssl|login)$|lang$|language$|locale$|theme$|timezone$)/i;

function loginCandidates(cookies: chrome.cookies.Cookie[]): chrome.cookies.Cookie[] {
  return cookies.filter((cookie) => !NON_LOGIN_COOKIE.test(cookie.name));
}

/** 一组 Cookie 里每个名字的第一条 */
function firstByName(
  cookies: chrome.cookies.Cookie[],
  names: string[],
): Map<string, chrome.cookies.Cookie> {
  const found = new Map<string, chrome.cookies.Cookie>();
  for (const cookie of cookies) {
    if (names.includes(cookie.name) && !found.has(cookie.name)) {
      found.set(cookie.name, cookie);
    }
  }
  return found;
}

/**
 * 认登录 Cookie 的格式，返回格式和登录用的那几条（剩余天数按它们算）；都不是时返回 null。
 * 先认站点自己声明的一组，再认 NexusPHP：新版只有一个签名令牌 c_secure_pass；旧版要 c_secure_uid 和 c_secure_pass，
 * c_secure_tracker_ssl 这些是偏好，不强求（以前六十个站点都要三件套，少一个就不能同步）。
 */
function recognize(
  site: KnownSite,
  cookies: chrome.cookies.Cookie[],
): { format: CookieFormat; login: chrome.cookies.Cookie[] } | null {
  if (site.cookieNames.length > 0) {
    const declared = firstByName(cookies, site.cookieNames);
    if (declared.size === site.cookieNames.length) {
      return { format: "declared", login: [...declared.values()] };
    }
  }
  const nexus = firstByName(cookies, ["c_secure_uid", "c_secure_pass"]);
  const pass = nexus.get("c_secure_pass");
  if (pass && isNexusAuthToken(pass.value)) {
    return { format: "nexus-token", login: [pass] };
  }
  const uid = nexus.get("c_secure_uid");
  if (uid && pass) {
    return { format: "nexus", login: [uid, pass] };
  }
  return null;
}

/** 会话 Cookie 没有过期时间，按永不过期算 */
function minExpiry(cookies: chrome.cookies.Cookie[]): number {
  return Math.min(...cookies.map((cookie) => cookie.expirationDate ?? Infinity));
}

interface PickedCookies {
  /** 要同步的那个域名下浏览器会带的全部 Cookie */
  cookies: chrome.cookies.Cookie[];
  format: CookieFormat | null;
  /** 认出格式时登录用的那几条 */
  login: chrome.cookies.Cookie[];
  /** 有没有像登录用的 Cookie（认不出格式时据此判 unverified 还是 missing） */
  candidate: boolean;
  /** 哪个域名下有任意 Cookie */
  any: boolean;
}

/**
 * 选出要同步的那个域的 Cookie。认得格式的域优先，必须在**同一个域**里认出来，几个域都认得时取登录 Cookie 最晚过期的那个：
 * 原来健康检查把各个域的 Cookie 合在一起判定，发送的却是「第一个有任意 Cookie 的域」，旧域只剩统计 Cookie 时状态显示有效，
 * 同步却把统计 Cookie 写进后端、替换掉原来有效的凭证。都认不出时取像登录用的 Cookie 最多的域（一样多取靠前的），
 * 这一组要 pt-tools 验证过才保存。
 */
async function pickDomainCookies(site: KnownSite): Promise<PickedCookies> {
  ensureCookiesApi();
  let best: {
    cookies: chrome.cookies.Cookie[];
    format: CookieFormat;
    login: chrome.cookies.Cookie[];
    expiry: number;
  } | null = null;
  let fallback: { cookies: chrome.cookies.Cookie[]; count: number } | null = null;
  let any = false;
  for (const domain of site.domains) {
    const cookies = await siteCookies(domain);
    if (cookies.length === 0) {
      continue;
    }
    any = true;
    const found = recognize(site, cookies);
    if (found) {
      const expiry = minExpiry(found.login);
      if (!best || expiry > best.expiry) {
        best = { cookies, format: found.format, login: found.login, expiry };
      }
      continue;
    }
    const count = loginCandidates(cookies).length;
    if (count > 0 && (!fallback || count > fallback.count)) {
      fallback = { cookies, count };
    }
  }
  if (best) {
    return { cookies: best.cookies, format: best.format, login: best.login, candidate: true, any };
  }
  if (fallback) {
    return { cookies: fallback.cookies, format: null, login: [], candidate: true, any };
  }
  return { cookies: [], format: null, login: [], candidate: false, any };
}

export async function readSiteCookies(domain: string): Promise<string> {
  const cookies = await chrome.cookies.getAll({ domain });
  if (cookies.length === 0) {
    return "";
  }

  return toCookieString(cookies);
}

export async function checkCookieHealth(site: KnownSite): Promise<CookieHealthResult> {
  // 对于 passkey / api_key 站点（cookieNames 为空），保号仍依赖浏览器登录态，
  // 因此只要检测到跨域的任意 cookie 即视为已获取，否则标记 missing。
  if (site.cookieNames.length === 0) {
    const allCookies = await getCookiesAcrossDomains(site);
    const crossDomainHasCookie = allCookies.length > 0;
    return {
      status: crossDomainHasCookie ? "valid" : "missing",
      expireDays: null,
      cookieString: toCookieString(await firstDomainCookies(site)),
      hasCookie: crossDomainHasCookie,
      format: null,
    };
  }

  const picked = await pickDomainCookies(site);
  if (!picked.format) {
    // 认不出格式：有像登录用的 Cookie 就整组交给 pt-tools 验证；一个都没有就是没登录，不给出可同步的 Cookie
    return {
      status: picked.candidate ? "unverified" : "missing",
      expireDays: null,
      cookieString: picked.candidate ? toCookieString(picked.cookies) : "",
      hasCookie: picked.any,
      format: null,
    };
  }

  const cookieString = toCookieString(picked.cookies);
  const nowSeconds = Date.now() / 1000;
  const remainingDays = picked.login
    .map((cookie) => cookie.expirationDate)
    .filter((value): value is number => typeof value === "number")
    .map((expiration) => Math.floor((expiration - nowSeconds) / 86400));
  const expireDays = remainingDays.length > 0 ? Math.min(...remainingDays) : null;
  const status: CookieHealthStatus =
    expireDays !== null && expireDays < 0
      ? "expired"
      : expireDays !== null && expireDays <= 7
        ? "expiring"
        : "valid";
  return { status, expireDays, cookieString, hasCookie: true, format: picked.format };
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
