import type { SiteLoginStateRecord } from "../modules/sync/api-client";
import { KNOWN_SITES, STORAGE_KEYS } from "./constants";
import { get, set } from "./storage";
import type { KnownSiteStatus } from "./types";

/*
 * 弹窗打开时先显示上次读到的结果，再在后台重新读：以前每次打开都从头读站点 Cookie、ping pt-tools、再取登录状态，
 * 网络慢时要空等好几秒。background 每次读完都存一份；弹窗直接读 chrome.storage，不用等 service worker 醒来。
 */

type SiteFields = Omit<KnownSiteStatus, "site">;

interface StoredSites {
  at: string;
  /** 按站点编号存，不存站点定义：还原时按当时扩展里的定义拼回去 */
  sites: Record<string, SiteFields>;
}

interface StoredLogins {
  at: string;
  baseUrl: string;
  logins: SiteLoginStateRecord[];
}

export interface Snapshot<T> {
  /** 读到这份结果的时间（ISO） */
  at: string;
  data: T;
}

function sameBaseUrl(a: string, b: string): boolean {
  const norm = (u: string) => u.trim().replace(/\/+$/, "");
  return norm(a) !== "" && norm(a) === norm(b);
}

export async function saveSitesSnapshot(
  statuses: KnownSiteStatus[],
  now = new Date(),
): Promise<void> {
  const sites: Record<string, SiteFields> = {};
  for (const { site, ...fields } of statuses) {
    sites[site.id] = fields;
  }
  await set<StoredSites>(STORAGE_KEYS.sitesSnapshot, { at: now.toISOString(), sites });
}

/** 上次的站点状态，按现在的站点定义还原：新版本去掉的站点丢掉，新加的站点等这次重新读 */
export async function loadSitesSnapshot(): Promise<Snapshot<KnownSiteStatus[]> | null> {
  const stored = await get<StoredSites>(STORAGE_KEYS.sitesSnapshot);
  if (!stored?.at || !stored.sites) return null;
  const data: KnownSiteStatus[] = [];
  for (const site of KNOWN_SITES) {
    const fields = stored.sites[site.id];
    if (fields) data.push({ ...fields, site });
  }
  return data.length > 0 ? { at: stored.at, data } : null;
}

export async function saveLoginSnapshot(
  baseUrl: string,
  logins: SiteLoginStateRecord[],
  now = new Date(),
): Promise<void> {
  await set<StoredLogins>(STORAGE_KEYS.loginSnapshot, { at: now.toISOString(), baseUrl, logins });
}

/** 上次从这个 pt-tools 读到的登录状态；换了 pt-tools 地址时不用 */
export async function loadLoginSnapshot(
  baseUrl: string,
): Promise<Snapshot<SiteLoginStateRecord[]> | null> {
  const stored = await get<StoredLogins>(STORAGE_KEYS.loginSnapshot);
  if (!stored?.at || !Array.isArray(stored.logins) || !sameBaseUrl(stored.baseUrl, baseUrl))
    return null;
  return { at: stored.at, data: stored.logins };
}
