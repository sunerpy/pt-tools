/*
 * 定时同步（M22）：按设置的间隔把 Cookie 同步到 pt-tools，并上传到 CookieCloud（打开了的话）。
 *
 * pt-tools：只推 Cookie 有变化的站点（按上次推送的指纹判断，指纹带着 pt-tools 的地址，见 pushFingerprint）。
 * 每推一次 pt-tools 都会启用站点、刷新站点实例、做一次登录探测并重载调度器，没变的不推。
 * 「全部站点」推浏览器里登录着的 Cookie 站点：pt-tools 里已启用的照旧推；没启用的带 keep_enabled 推，
 * pt-tools 只把 Cookie 存下、站点保持停用（不替用户启用）。以前只推已启用的，而 pt-tools 启用 Cookie 站点要先有 Cookie，
 * 新站点两头都等着对方。旧版 pt-tools 不认 keep_enabled（会把站点启用），所以先看 /api/ping 的 features，不认就照旧只推已启用的。
 * 单独打开了「自动同步」的站点照旧算在里面。要同步（已启用或打开了自动同步）、但浏览器里没有可用 Cookie 的站点不推，
 * 按站点记在结果的 missing 里（结果里列出来，也会发通知），不悄悄跳过；没启用的站点没有 Cookie 不算缺。
 * 推 Cookie 的各条路共用一把锁（withPushLock）：读 Cookie、比指纹、推送、记指纹在锁里一起做。
 * 格式认不出的 Cookie（见 modules/sync/cookie-reader 的 unverified）带 verify_cookie 推：pt-tools 先拿它登录一次，登录上了才保存。
 * 只替要同步的站点这么做（没启用的站点不替用户去访问），pt-tools 不认 verify_cookie 时记成失败、写明要更新；
 * 被拒绝的那组记下指纹与原因，Cookie 没变就不再推，原因照样写进结果。
 * CookieCloud：Cookie 与目标都没变时不传（见 modules/cookiecloud/upload）。
 *
 * 依赖都从外面传进来（chrome API、存储、网络），这里只有挑站点与判断的逻辑，好测。
 */
import { t } from "../core/i18n";
import type { KnownSite } from "../core/constants";
import type { RejectedPush, SyncRunSummary, SyncSchedule } from "../core/types";
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

/** SiteCookie 是一个站点可以同步的 Cookie；verify 为 true 时格式认不出，要 pt-tools 验证过才保存 */
export interface SiteCookie {
  cookie: string;
  verify: boolean;
}

/** RejectedAgainError：这组 Cookie 上次已经被 pt-tools 拒绝、没变，这次不推；消息是上次的原因 */
class RejectedAgainError extends Error {
  constructor(reason: string) {
    super(reason);
    this.name = "RejectedAgainError";
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
  /** ptToolsSites 返回 pt-tools 里的站点名（小写）与启用状态；登录过期时抛 AuthExpiredError */
  ptToolsSites(baseUrl: string): Promise<Array<{ name: string; enabled: boolean }>>;
  /** keepEnabledSupported：这个 pt-tools 认不认推送时的 keep_enabled（看 /api/ping 的 features）；读不到时为 false */
  keepEnabledSupported(baseUrl: string): Promise<boolean>;
  /** verifySupported：这个 pt-tools 认不认推送时的 verify_cookie（看 /api/ping 的 features）；读不到时为 false */
  verifySupported(baseUrl: string): Promise<boolean>;
  /** readCookie 返回可以同步的 Cookie；没有登录 Cookie 时返回 null */
  readCookie(site: KnownSite): Promise<SiteCookie | null>;
  /**
   * pushCookie 把 Cookie 推给 pt-tools；keepEnabled 时停用的站点只存 Cookie、不启用；verify 时 pt-tools 先验证、登录上了才保存。
   * 登录过期时抛 AuthExpiredError
   */
  pushCookie(
    baseUrl: string,
    site: KnownSite,
    cookie: string,
    opts: { keepEnabled: boolean; verify: boolean },
  ): Promise<void>;
  /** recordPushed 记下推送成功：指纹与时间（同时清掉这个站点被拒绝的记录） */
  recordPushed(siteId: string, hash: string, at: string): Promise<void>;
  /** getRejected 是这个站点上次被 pt-tools 验证拒绝的那组 Cookie */
  getRejected(siteId: string): Promise<RejectedPush | undefined>;
  /** recordRejected 记下 pt-tools 验证后拒绝保存的那组 Cookie */
  recordRejected(siteId: string, rejected: RejectedPush): Promise<void>;
  /** isRejection 判断推送失败是不是 pt-tools 验证后拒绝保存（没登录上），而不是网络、正在探测这类过一会儿会好的 */
  isRejection(error: unknown): boolean;
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

/**
 * PickedSite 是这一轮要推的一个站点。required：要同步的（打开了「自动同步」，或 allSites 时 pt-tools 里已启用的），
 * 浏览器里没有可用 Cookie 时记进 missing；不是 required 的是 allSites 时 pt-tools 里没启用的，有 Cookie 才推、
 * 推的时候带 keep_enabled（只存 Cookie，不启用），没有 Cookie 不算缺 —— 不然六十多个内置站点会全列成缺失。
 */
export interface PickedSite {
  site: KnownSite;
  required: boolean;
}

/**
 * pickSites 是这一轮要推给 pt-tools 的站点（只看用 Cookie 登录的站点）。
 * disabled：allSites 时也带上的、pt-tools 站点列表里停用的站点（pt-tools 认 keep_enabled 时才传）。
 * 只认列表里明确停用的：扩展比 pt-tools 新、多认得的站点，pt-tools 那边没有，推过去只会失败。
 */
export function pickSites(
  sites: KnownSite[],
  schedule: SyncSchedule,
  autoSync: Record<string, boolean>,
  enabled: string[] | null,
  disabled: string[] | null = null,
): PickedSite[] {
  const enabledSet = new Set((enabled ?? []).map((name) => name.toLowerCase()));
  const disabledSet = new Set((disabled ?? []).map((name) => name.toLowerCase()));
  const picked: PickedSite[] = [];
  for (const site of sites) {
    if (site.syncField !== "cookie") continue;
    const id = site.id.toLowerCase();
    if (autoSync[site.id] === true || (schedule.allSites && enabledSet.has(id))) {
      picked.push({ site, required: true });
    } else if (schedule.allSites && disabledSet.has(id)) {
      picked.push({ site, required: false });
    }
  }
  return picked;
}

type PushOutcome = "none" | "unchanged" | "pushed";

async function syncPtTools(
  deps: ScheduledSyncDeps,
  schedule: SyncSchedule,
): Promise<SyncRunSummary["ptTools"]> {
  const missing: string[] = [];
  const out: SyncRunSummary["ptTools"] = { synced: [], unchanged: 0, failed: [], missing };
  const baseUrl = await deps.ptTools();
  if (!baseUrl) {
    out.skipped = t("sync.skipNotConnected");
    return out;
  }
  const autoSync = await deps.getAutoSyncMap();
  let enabled: string[] | null = null;
  let disabled: string[] | null = null;
  if (schedule.allSites) {
    try {
      const ptSites = await deps.ptToolsSites(baseUrl);
      enabled = ptSites.filter((s) => s.enabled).map((s) => s.name);
      disabled = ptSites.filter((s) => !s.enabled).map((s) => s.name);
    } catch (error: unknown) {
      if (error instanceof AuthExpiredError) {
        await deps.authExpired(baseUrl);
        out.skipped = t("sync.skipAuthExpired");
        return out;
      }
      // 读不到站点列表：这一轮只同步单独打开了「自动同步」的站点，并写明原因（结果里看得到，也会发通知）
      out.error = t("sync.enabledSitesFailed", deps.errorText(error));
      enabled = null;
      disabled = null;
    }
  }
  // 推没启用的站点要 pt-tools 认 keep_enabled：旧版本会把站点启用，那就照旧只推已启用的
  if (disabled !== null && disabled.length > 0 && !(await deps.keepEnabledSupported(baseUrl))) {
    disabled = null;
  }
  const sites = pickSites(deps.sites, schedule, autoSync, enabled, disabled);
  if (sites.length === 0) {
    if (!out.error) {
      out.skipped = t("sync.skipNoSites");
    }
    return out;
  }
  // 这一轮第一次要验证时才问 pt-tools 认不认 verify_cookie
  let verifyOk: Promise<boolean> | null = null;
  for (const { site, required } of sites) {
    let outcome: PushOutcome;
    try {
      outcome = await deps.withPushLock(async (): Promise<PushOutcome> => {
        const read = await deps.readCookie(site);
        // 格式认不出的 Cookie 要 pt-tools 去站点验证：没启用、没打开自动同步的站点不替用户这么做
        if (!read?.cookie || (read.verify && !required)) {
          return "none";
        }
        const hash = await pushFingerprint(baseUrl, read.cookie);
        if ((await deps.getHash(site.id)) === hash) {
          return "unchanged";
        }
        if (read.verify) {
          const rejected = await deps.getRejected(site.id);
          if (rejected?.hash === hash) {
            throw new RejectedAgainError(rejected.reason);
          }
          verifyOk ??= deps.verifySupported(baseUrl);
          if (!(await verifyOk)) {
            throw new Error(t("error.cookieVerifyUnsupported", site.name));
          }
        }
        try {
          await deps.pushCookie(baseUrl, site, read.cookie, {
            keepEnabled: !required,
            verify: read.verify,
          });
        } catch (error: unknown) {
          if (read.verify && deps.isRejection(error)) {
            await deps.recordRejected(site.id, { hash, reason: deps.errorText(error) });
          }
          throw error;
        }
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
    } else if (required) {
      missing.push(site.name);
    }
  }
  // 挑出来的只有没启用、浏览器里又都没有 Cookie 的站点：和「没有要同步的站点」一样写明，不当成一次成功的同步
  if (
    out.synced.length === 0 &&
    out.unchanged === 0 &&
    out.failed.length === 0 &&
    missing.length === 0 &&
    !out.error
  ) {
    out.skipped = t("sync.skipNoSites");
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

/**
 * problemsOf 是一轮同步里要用户处理的问题（通知与弹窗的提示用），没有问题时为空串：pt-tools 这一轮没有同步
 * （没连上、登录过期、没有要同步的站点）、读不到站点列表、站点没同步上、没有可用的 Cookie、CookieCloud 出错。
 */
export function problemsOf(run: SyncRunSummary | null): string {
  if (!run) {
    return "";
  }
  const parts: string[] = [];
  if (run.ptTools.skipped) {
    parts.push(t("sync.problemSkipped", run.ptTools.skipped));
  }
  if (run.ptTools.error) {
    parts.push(run.ptTools.error);
  }
  if (run.ptTools.failed.length > 0) {
    parts.push(
      t(
        "sync.problemPtTools",
        run.ptTools.failed.length,
        run.ptTools.failed.map((f) => f.error).join("; "),
      ),
    );
  }
  const missing = run.ptTools.missing ?? [];
  if (missing.length > 0) {
    parts.push(t("sync.problemMissing", missing.length, missing.join(", ")));
  }
  if (run.cookieCloud.error) {
    parts.push(t("sync.problemCookieCloud", run.cookieCloud.error));
  }
  return parts.join("\n");
}

/** runHadProblems 判断「立即同步一次」以后该不该说「已经同步了」：problemsOf 有内容时为 true。 */
export function runHadProblems(run: SyncRunSummary | null): boolean {
  return problemsOf(run) !== "";
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
