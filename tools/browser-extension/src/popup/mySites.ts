import type { KnownSiteStatus } from "../core/types";
import type { SiteLoginStateRecord } from "../modules/sync";

/**
 * 弹窗「我的站点」的一行：浏览器里的 Cookie 状态，加上 pt-tools 里的登录状态（保号剩几天）。
 * 默认只列「我的」站点 —— 浏览器里有这个站的 Cookie，或者 pt-tools 里启用了它；
 * 以前把六十多个内置站点全列出来，要在一长串里找自己的那几个。
 */
export interface MySiteRow {
  status: KnownSiteStatus;
  login?: SiteLoginStateRecord;
  /** 浏览器里有可同步的 Cookie（Cookie 站点、Cookie 有效或快过期） */
  syncable: boolean;
  /** pt-tools 里启用了这个站点 */
  configured: boolean;
  /**
   * pt-tools 里有这个站点、没启用，扩展推过它的 Cookie（定时同步只存 Cookie、不替用户启用）：
   * 行上写「Cookie 已存，待启用」，下一步是到 pt-tools 里启用它。
   */
  savedNotEnabled: boolean;
  /** 要注意：Cookie 过期 / 快过期，或保号快到期 */
  attention: boolean;
}

export type LoginTone = "ok" | "warn" | "bad" | "muted";

const WARN_TIERS = new Set(["14d", "7d"]);
const BAD_TIERS = new Set(["3d", "banned-imminent"]);

export function loginTone(login?: SiteLoginStateRecord): LoginTone {
  if (!login || login.tier === "unknown" || !login.tier) return "muted";
  if (BAD_TIERS.has(login.tier)) return "bad";
  if (WARN_TIERS.has(login.tier)) return "warn";
  return "ok";
}

function needsAttention(s: KnownSiteStatus, login?: SiteLoginStateRecord): boolean {
  if (s.hasCookie && (s.cookieStatus === "expired" || s.cookieStatus === "expiring")) return true;
  const tone = loginTone(login);
  return tone === "warn" || tone === "bad";
}

/**
 * 拼出列表：showAll 为 false 时只留有 Cookie 或 pt-tools 里启用了的站点。
 * 排序：要注意的在前，然后是 pt-tools 里启用了的，再按名字。
 */
export function buildMySites(
  sites: KnownSiteStatus[],
  logins: SiteLoginStateRecord[],
  showAll: boolean,
): MySiteRow[] {
  const byName = new Map(logins.map((l) => [l.site_name.toLowerCase(), l]));
  const rows: MySiteRow[] = [];
  for (const status of sites) {
    const login = byName.get(status.site.id.toLowerCase());
    const configured = login?.enabled === true;
    if (!showAll && !status.hasCookie && !configured) continue;
    rows.push({
      status,
      login,
      configured,
      savedNotEnabled: Boolean(login) && !configured && Boolean(status.lastSync),
      syncable:
        status.site.syncField === "cookie" &&
        (status.cookieStatus === "valid" || status.cookieStatus === "expiring"),
      attention: needsAttention(status, login),
    });
  }
  const rank = (r: MySiteRow) => (r.attention ? 0 : 2) + (r.configured ? 0 : 1);
  return rows.sort(
    (a, b) => rank(a) - rank(b) || a.status.site.name.localeCompare(b.status.site.name),
  );
}

/** 「我的」站点有几个（不受 showAll 影响），用于「显示全部 / 只看我的」的文案 */
export function countMine(sites: KnownSiteStatus[], logins: SiteLoginStateRecord[]): number {
  return buildMySites(sites, logins, false).length;
}

/**
 * 一行的登录状态写什么。loginsKnown 为假（会话过期或连不上 pt-tools，也没有上次存下的登录状态）时是 unknown：
 * 不知道 pt-tools 里启没启用，不能写「pt-tools 未启用」——以前就这么写，已经启用的站点也显示未启用。
 */
export type LoginLabel =
  | { kind: "unknown" }
  | { kind: "savedNotEnabled" }
  | { kind: "notInPtTools" }
  | { kind: "tier"; login: SiteLoginStateRecord };

export function loginLabel(row: MySiteRow, loginsKnown: boolean): LoginLabel {
  if (!loginsKnown) return { kind: "unknown" };
  // 定时同步只存了 Cookie、没替用户启用：写明下一步是到 pt-tools 里启用
  if (row.savedNotEnabled) return { kind: "savedNotEnabled" };
  if (!row.login || !row.configured) return { kind: "notInPtTools" };
  return { kind: "tier", login: row.login };
}

/** Cookie 缺失、浏览器里又有这个站的 Cookie 时还差哪几个必需 Cookie（多半是没登录，或者站点换了 Cookie）；其他情况为空 */
export function missingCookieNames(s: KnownSiteStatus): string[] {
  return s.cookieStatus === "missing" && s.hasCookie ? (s.missingCookies ?? []) : [];
}
