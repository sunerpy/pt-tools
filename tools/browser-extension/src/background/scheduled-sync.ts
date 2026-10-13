/*
 * 定时同步（M22）：按设置的间隔把 Cookie 同步到 pt-tools，并上传到 CookieCloud（打开了的话）。
 *
 * pt-tools：只推 Cookie 有变化的站点（按上次推送的指纹判断，指纹带着 pt-tools 的地址，见 pushFingerprint）。
 * 每推一次 pt-tools 都会启用站点、刷新站点实例、做一次登录探测并重载调度器，没变的不推。「全部站点」只推 pt-tools 里
 * 已经启用的站点，不替用户启用新站点；单独打开了「自动同步」的站点照旧算在里面。
 * 推 Cookie 的各条路共用一把锁（withPushLock）：读 Cookie、比指纹、推送、记指纹在锁里一起做。
 * CookieCloud：Cookie 与目标都没变时不传（见 modules/cookiecloud/upload）。
 *
 * 依赖都从外面传进来（chrome API、存储、网络），这里只有挑站点与判断的逻辑，好测。
 */
import { t } from "../core/i18n";
import type { KnownSite } from "../core/constants";
import type { SyncRunSummary, SyncSchedule } from "../core/types";
import type {
  CookieCloudConfig,
  CookieCloudState,
  UploadResult,
} from "../modules/cookiecloud/upload";

export const MIN_SYNC_INTERVAL = 10;
export const MAX_SYNC_INTERVAL = 1440;
export const DEFAULT_SYNC_SCHEDULE: SyncSchedule = {
  enabled: false,
  intervalMinutes: 60,
  allSites: false,
};

/** normalizeSchedule 把弹窗传来的设置整理成合法的值（间隔按分钟取整并夹在范围里）。 */
export function normalizeSchedule(raw: unknown): SyncSchedule {
  const value = (typeof raw === "object" && raw !== null ? raw : {}) as Partial<SyncSchedule>;
  const interval = Number(value.intervalMinutes);
  return {
    enabled: value.enabled === true,
    intervalMinutes: Number.isFinite(interval)
      ? Math.min(MAX_SYNC_INTERVAL, Math.max(MIN_SYNC_INTERVAL, Math.round(interval)))
      : DEFAULT_SYNC_SCHEDULE.intervalMinutes,
    allSites: value.allSites === true,
  };
}

export async function sha256Hex(text: string): Promise<string> {
  const hash = new Uint8Array(
    await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text)),
  );
  return Array.from(hash, (b) => b.toString(16).padStart(2, "0")).join("");
}

/**
 * pushFingerprint 是「推给哪个 pt-tools 的哪份 Cookie」的指纹：带上地址，换了 pt-tools 实例以后 Cookie 没变也会重新推。
 */
export function pushFingerprint(baseUrl: string, cookie: string): Promise<string> {
  return sha256Hex(`${baseUrl.trim().replace(/\/+$/, "")}\n${cookie}`);
}

/**
 * createLock 是一把异步锁：fn 一个接一个跑。推 Cookie 的各条路（定时、手动、批量、Cookie 变化时的自动同步）共用一把，
 * 先读到的旧 Cookie 不会晚于新的推出去、把后端改回旧的。
 */
export function createLock(): <T>(fn: () => Promise<T>) => Promise<T> {
  let tail: Promise<unknown> = Promise.resolve();
  return <T>(fn: () => Promise<T>): Promise<T> => {
    const run = tail.then(fn, fn);
    tail = run.catch(() => undefined);
    return run;
  };
}

/** AuthExpiredError 表示 pt-tools 的登录过期了：这一轮不再推别的站点。 */
export class AuthExpiredError extends Error {
  constructor() {
    super("auth required");
    this.name = "AuthExpiredError";
  }
}

export interface ScheduledSyncDeps {
  sites: KnownSite[];
  getSchedule(): Promise<SyncSchedule>;
  getAutoSyncMap(): Promise<Record<string, boolean>>;
  /** getHash 是这个站点上次推送成功的指纹（见 pushFingerprint） */
  getHash(siteId: string): Promise<string | undefined>;
  /** withPushLock 在推 Cookie 的锁里跑 fn（和手动、自动同步共用一把） */
  withPushLock<T>(fn: () => Promise<T>): Promise<T>;
  /** ptTools 返回连着的 pt-tools 地址；没配地址或没连上时返回 null */
  ptTools(): Promise<string | null>;
  /** enabledSites 返回 pt-tools 里已启用的站点名（小写）；登录过期时抛 AuthExpiredError */
  enabledSites(baseUrl: string): Promise<string[]>;
  /** readCookie 返回可以同步的 Cookie；不齐全、没有时返回空串 */
  readCookie(site: KnownSite): Promise<string>;
  /** pushCookie 把 Cookie 推给 pt-tools；登录过期时抛 AuthExpiredError */
  pushCookie(baseUrl: string, site: KnownSite, cookie: string): Promise<void>;
  /** recordPushed 记下推送成功：指纹与时间 */
  recordPushed(siteId: string, hash: string, at: string): Promise<void>;
  /** authExpired 在 pt-tools 登录过期时调用（把连接标成断开） */
  authExpired(baseUrl: string): Promise<void>;
  getCookieCloudConfig(): Promise<CookieCloudConfig>;
  getCookieCloudState(): Promise<CookieCloudState | null>;
  saveCookieCloudState(state: CookieCloudState): Promise<void>;
  upload(
    cfg: CookieCloudConfig,
    state: CookieCloudState | null,
    opts: { force: boolean },
  ): Promise<{ result: UploadResult; state: CookieCloudState }>;
  errorText(error: unknown): string;
  now(): Date;
}

/** pickSites 是这一轮要推给 pt-tools 的站点：用 Cookie 登录的站点里，打开了「自动同步」的，以及 allSites 时 pt-tools 里已启用的。 */
export function pickSites(
  sites: KnownSite[],
  schedule: SyncSchedule,
  autoSync: Record<string, boolean>,
  enabled: string[] | null,
): KnownSite[] {
  const enabledSet = new Set((enabled ?? []).map((name) => name.toLowerCase()));
  return sites.filter(
    (site) =>
      site.syncField === "cookie" &&
      (autoSync[site.id] === true || (schedule.allSites && enabledSet.has(site.id.toLowerCase()))),
  );
}

type PushOutcome = "none" | "unchanged" | "pushed";

async function syncPtTools(
  deps: ScheduledSyncDeps,
  schedule: SyncSchedule,
): Promise<SyncRunSummary["ptTools"]> {
  const out: SyncRunSummary["ptTools"] = { synced: [], unchanged: 0, failed: [] };
  const baseUrl = await deps.ptTools();
  if (!baseUrl) {
    out.skipped = t("sync.skipNotConnected");
    return out;
  }
  const autoSync = await deps.getAutoSyncMap();
  let enabled: string[] | null = null;
  if (schedule.allSites) {
    try {
      enabled = await deps.enabledSites(baseUrl);
    } catch (error: unknown) {
      if (error instanceof AuthExpiredError) {
        await deps.authExpired(baseUrl);
        out.skipped = t("sync.skipAuthExpired");
        return out;
      }
      // 读不到站点列表：这一轮只同步单独打开了「自动同步」的站点，并写明原因（结果里看得到，也会发通知）
      out.error = t("sync.enabledSitesFailed", deps.errorText(error));
      enabled = null;
    }
  }
  const sites = pickSites(deps.sites, schedule, autoSync, enabled);
  if (sites.length === 0) {
    if (!out.error) {
      out.skipped = t("sync.skipNoSites");
    }
    return out;
  }
  for (const site of sites) {
    let outcome: PushOutcome;
    try {
      outcome = await deps.withPushLock(async (): Promise<PushOutcome> => {
        const cookie = await deps.readCookie(site);
        if (!cookie) {
          return "none";
        }
        const hash = await pushFingerprint(baseUrl, cookie);
        if ((await deps.getHash(site.id)) === hash) {
          return "unchanged";
        }
        await deps.pushCookie(baseUrl, site, cookie);
        await deps.recordPushed(site.id, hash, deps.now().toISOString());
        return "pushed";
      });
    } catch (error: unknown) {
      if (error instanceof AuthExpiredError) {
        await deps.authExpired(baseUrl);
        out.failed.push({ siteId: site.id, error: t("sync.skipAuthExpired") });
        break;
      }
      out.failed.push({ siteId: site.id, error: deps.errorText(error) });
      continue;
    }
    if (outcome === "unchanged") {
      out.unchanged += 1;
    } else if (outcome === "pushed") {
      out.synced.push(site.id);
    }
  }
  return out;
}

async function syncCookieCloud(deps: ScheduledSyncDeps): Promise<SyncRunSummary["cookieCloud"]> {
  const cfg = await deps.getCookieCloudConfig();
  if (!cfg.enabled) {
    return { uploaded: false, domains: 0, cookies: 0, skipped: t("sync.skipCookieCloudOff") };
  }
  const state = await deps.getCookieCloudState();
  try {
    const { result, state: next } = await deps.upload(cfg, state, { force: false });
    await deps.saveCookieCloudState(next);
    return { uploaded: result.uploaded, domains: result.domains, cookies: result.cookies };
  } catch (error: unknown) {
    const text = deps.errorText(error);
    const at = deps.now().toISOString();
    await deps.saveCookieCloudState({
      ...(state ?? {
        lastUploadAt: null,
        lastDigest: null,
        lastTarget: null,
        domains: 0,
        cookies: 0,
      }),
      lastError: text,
      lastErrorAt: at,
    });
    return { uploaded: false, domains: 0, cookies: 0, error: text };
  }
}

/** runSync 跑一轮：先 pt-tools，再 CookieCloud；两边互不影响。 */
export async function runSync(
  deps: ScheduledSyncDeps,
  trigger: SyncRunSummary["trigger"],
): Promise<SyncRunSummary> {
  const schedule = await deps.getSchedule();
  const ptTools = await syncPtTools(deps, schedule);
  const cookieCloud = await syncCookieCloud(deps);
  return { at: deps.now().toISOString(), trigger, ptTools, cookieCloud };
}
