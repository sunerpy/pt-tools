/*
 * CookieCloud 的明文：{cookie_data: {域名: [Cookie…]}, local_storage_data: {…}, update_time}。
 * 扩展只放 pt-tools 支持的 PT 站点的 Cookie（KNOWN_SITES 里的域名，每个域名下浏览器里的全部 Cookie），
 * 不碰别的网站。上传前和服务端已有的合并：只替换自己的域名，别的域名与 local_storage_data 原样保留，
 * 和 CookieCloud 扩展共用一个 UUID 时不会把它同步的数据冲掉。
 */
import { KNOWN_SITES } from "../../core/constants";

/** 一条 Cookie：chrome.cookies 给的字段，和 CookieCloud 扩展上传的一样。 */
export interface CookieCloudCookie {
  name: string;
  value: string;
  domain: string;
  path: string;
  hostOnly: boolean;
  httpOnly: boolean;
  secure: boolean;
  session: boolean;
  sameSite: string;
  storeId?: string;
  expirationDate?: number;
}

export interface CookieCloudData {
  cookie_data: Record<string, CookieCloudCookie[]>;
  local_storage_data?: Record<string, unknown>;
  update_time?: string;
  [key: string]: unknown;
}

/** siteDomains 是要上传的域名（去重、小写）。 */
export function siteDomains(): string[] {
  const set = new Set<string>();
  for (const site of KNOWN_SITES) {
    for (const domain of site.domains) {
      set.add(domain.toLowerCase());
    }
  }
  return [...set].sort();
}

function toCookie(c: chrome.cookies.Cookie): CookieCloudCookie {
  const out: CookieCloudCookie = {
    name: c.name,
    value: c.value,
    domain: c.domain,
    path: c.path,
    hostOnly: c.hostOnly,
    httpOnly: c.httpOnly,
    secure: c.secure,
    session: c.session,
    sameSite: c.sameSite,
    storeId: c.storeId,
  };
  if (typeof c.expirationDate === "number") {
    out.expirationDate = c.expirationDate;
  }
  return out;
}

/**
 * collectSiteCookies 读出各个 PT 站点域名下浏览器里的 Cookie（chrome.cookies.getAll 按域名取，含子域名）。
 * 没有 Cookie 的域名不出现。
 */
export async function collectSiteCookies(): Promise<Record<string, CookieCloudCookie[]>> {
  const out: Record<string, CookieCloudCookie[]> = {};
  for (const domain of siteDomains()) {
    const cookies = await chrome.cookies.getAll({ domain });
    if (cookies.length > 0) {
      out[domain] = cookies.map(toCookie);
    }
  }
  return out;
}

/**
 * mergeCookieData 把 ours 合进服务端已有的数据：ownedDomains 里的域名整个换成 ours 的（ours 里没有的就删掉，
 * 浏览器里已经没有的 Cookie 不留在服务端），别的域名与其余字段原样保留。
 */
export function mergeCookieData(
  existing: CookieCloudData | null,
  ours: Record<string, CookieCloudCookie[]>,
  ownedDomains: string[],
  now: Date,
): CookieCloudData {
  const base: CookieCloudData = existing ? { ...existing } : { cookie_data: {} };
  const cookieData: Record<string, CookieCloudCookie[]> = { ...(base.cookie_data ?? {}) };
  for (const domain of ownedDomains) {
    delete cookieData[domain];
  }
  for (const [domain, list] of Object.entries(ours)) {
    cookieData[domain] = list;
  }
  return {
    ...base,
    cookie_data: cookieData,
    local_storage_data: base.local_storage_data ?? {},
    update_time: now.toISOString(),
  };
}

/** parseCookieData 检查解密出来的内容是不是 CookieCloud 的数据（cookie_data 是对象）。 */
export function parseCookieData(plain: string): CookieCloudData | null {
  let value: unknown;
  try {
    value = JSON.parse(plain);
  } catch {
    return null;
  }
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return null;
  }
  const data = value as Record<string, unknown>;
  const cookieData = data.cookie_data;
  if (
    cookieData !== undefined &&
    (typeof cookieData !== "object" || cookieData === null || Array.isArray(cookieData))
  ) {
    return null;
  }
  return { ...data, cookie_data: (cookieData as Record<string, CookieCloudCookie[]>) ?? {} };
}

/**
 * cookieDigest 是 ours 的指纹（SHA-256）：名字、值、域名、路径、hostOnly 与过期时间。过期时间也算：pt-tools 导入时丢掉已经过期的
 * Cookie，站点只把过期时间往后挪（值不变）时也要重新上传，不然到了旧的过期时间 pt-tools 就取不到了。定时上传时指纹没变就不传。
 */
export async function cookieDigest(ours: Record<string, CookieCloudCookie[]>): Promise<string> {
  const rows: string[] = [];
  for (const domain of Object.keys(ours).sort()) {
    for (const c of ours[domain]) {
      rows.push(
        JSON.stringify([
          domain,
          c.domain,
          c.path,
          c.name,
          c.value,
          c.hostOnly,
          c.expirationDate ?? null,
        ]),
      );
    }
  }
  rows.sort();
  const bytes = new TextEncoder().encode(rows.join("\n"));
  const hash = new Uint8Array(await crypto.subtle.digest("SHA-256", bytes));
  return Array.from(hash, (b) => b.toString(16).padStart(2, "0")).join("");
}
