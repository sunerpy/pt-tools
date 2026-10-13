import { KNOWN_SITES, STORAGE_KEYS, type KnownSite } from "../core/constants";
import { t } from "../core/i18n";
import { logger } from "../core/logger";
import { createMessage, onMessage, sendToContent, sendToPopup } from "../core/messages";
import { hasWebNavigationPermission } from "../core/permissions";
import {
  get,
  getActiveSession,
  getAutoSyncMap,
  getConnection,
  getLastSyncMap,
  getLastSyncRun,
  getSyncHashMap,
  getSyncSchedule,
  getTabStatus,
  getTabStatusMap,
  remove,
  removeTabStatus,
  saveSession,
  set,
  setActiveSessionId,
  setAutoSync,
  setConnection,
  setLastSync,
  setLastSyncRun,
  setLastVisit,
  setSyncHash,
  setSyncSchedule,
  setTabStatus,
  updateConnection,
} from "../core/storage";
import type {
  CapturedPage,
  CollectionSession,
  KnownSiteStatus,
  MessageSender,
  PtToolsConnection,
  SiteDetectedPayload,
  SiteInfo,
  StatusPayload,
  SyncRunSummary,
  SyncSchedule,
  SyncSettingsView,
  TabSiteStatus,
  UnknownSiteStatus,
} from "../core/types";
import {
  PtToolsApiClient,
  PtToolsApiError,
  checkCookieHealth,
  fetchWithTimeout,
  friendlyErrorMessage,
} from "../modules/sync";
import {
  CookieCloudError,
  EMPTY_COOKIECLOUD_STATE,
  cookieCloudView,
  getCookieCloudConfig,
  getCookieCloudState,
  mergeCookieCloudInput,
  setCookieCloudConfig,
  setCookieCloudState,
  testCookieCloud,
  uploadToCookieCloud,
} from "../modules/cookiecloud";
import type { SiteLoginStateRecord } from "../modules/sync";
import { autoCollect } from "../modules/collector/auto-collector";
import { extractDomain, matchKnownSite } from "../utils";
import {
  type BackendPendingAction,
  createPendingActionPoller,
  isPendingActionArray,
} from "./pending-actions";
import {
  AuthExpiredError,
  DEFAULT_SYNC_SCHEDULE,
  type ScheduledSyncDeps,
  createLock,
  normalizeSchedule,
  pushFingerprint,
  runSync,
} from "./scheduled-sync";
import { createTabGenerations } from "./tab-generation";

interface MessageResponse<T> {
  ok: boolean;
  data?: T;
  error?: string;
}

const AUTO_SYNC_INTERVAL_MS = 30000;
const autoSyncThrottle = new Map<string, number>();

function isMessageResponse<T>(value: unknown): value is MessageResponse<T> {
  return typeof value === "object" && value !== null && "ok" in value;
}

function normalizeCookieDomain(domain: string): string {
  return domain.replace(/^\./, "").toLowerCase();
}

function matchesDomain(domain: string, knownDomain: string): boolean {
  return domain === knownDomain || domain.endsWith(`.${knownDomain}`);
}

function toCurrentSite(status: TabSiteStatus): SiteInfo | null {
  if (status.mode === "known" && status.known) {
    return {
      name: status.known.site.name,
      url: "",
      schema: status.known.site.schema,
      authMethod: status.known.site.authMethod,
    };
  }

  if (status.mode === "unknown" && status.unknown) {
    return {
      name: extractDomain(status.unknown.url) || t("error.unknownSite"),
      url: status.unknown.url,
      schema: status.unknown.detectedSchema,
      authMethod: "cookie",
    };
  }

  return null;
}

/** Badge 按标签页设置：不带 tabId 改的是全局默认值，后台标签页加载完 PT 站，当前标签页的图标也跟着变 */
async function updateBadge(tabId: number, status: TabSiteStatus): Promise<void> {
  if (status.mode === "known") {
    await chrome.action.setBadgeText({ tabId, text: "✓" });
    await chrome.action.setBadgeBackgroundColor({ tabId, color: "#1f8a70" });
    return;
  }

  if (status.mode === "unknown") {
    await chrome.action.setBadgeText({ tabId, text: "?" });
    await chrome.action.setBadgeBackgroundColor({ tabId, color: "#e6a23c" });
    return;
  }

  await chrome.action.setBadgeText({ tabId, text: "" });
}

async function getActiveTab(): Promise<chrome.tabs.Tab | null> {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  return tab ?? null;
}

async function buildKnownSiteStatus(site: KnownSite): Promise<KnownSiteStatus> {
  const [health, autoSyncMap, lastSyncMap] = await Promise.all([
    checkCookieHealth(site),
    getAutoSyncMap(),
    getLastSyncMap(),
  ]);

  return {
    site,
    cookieStatus: health.status,
    cookieExpireDays: health.expireDays,
    hasCookie: health.hasCookie,
    lastSync: lastSyncMap[site.id] ?? null,
    autoSync: autoSyncMap[site.id] ?? false,
  };
}

async function getStatusPayload(): Promise<StatusPayload> {
  const [activeSession, connection, activeTab] = await Promise.all([
    getActiveSession(),
    getConnection(),
    getActiveTab(),
  ]);
  const tabSiteStatus = activeTab?.id ? await getTabStatus(activeTab.id) : null;

  return {
    activeSession,
    pageCount: activeSession?.pages.length ?? 0,
    currentSite: tabSiteStatus ? toCurrentSite(tabSiteStatus) : null,
    connection,
  };
}

async function publishStatus(): Promise<void> {
  const payload = await getStatusPayload();
  await sendToPopup(createMessage("STATUS_UPDATE", payload));
}

async function ensureContentScript(tabId: number): Promise<boolean> {
  try {
    const response = await chrome.tabs.sendMessage(tabId, {
      type: "PING",
      payload: {},
      timestamp: Date.now(),
    });
    return response?.ok === true;
  } catch {
    try {
      await chrome.scripting.executeScript({
        target: { tabId },
        files: ["content.js"],
      });
      return true;
    } catch {
      return false;
    }
  }
}

async function detectViaContent(tabId: number): Promise<SiteDetectedPayload | null> {
  const injected = await ensureContentScript(tabId);
  if (!injected) {
    return null;
  }

  try {
    const response = await sendToContent(tabId, createMessage("DETECT_SITE", {}));
    if (!isMessageResponse<SiteDetectedPayload>(response) || !response.ok || !response.data) {
      return null;
    }
    return response.data;
  } catch {
    return null;
  }
}

async function setTabMode(tabId: number, status: TabSiteStatus): Promise<TabSiteStatus> {
  await setTabStatus(tabId, status);
  await updateBadge(tabId, status);
  await publishStatus();
  return status;
}

const tabGenerations = createTabGenerations();

async function detectTabSiteStatus(tabId: number, url: string): Promise<TabSiteStatus> {
  const known = matchKnownSite(url);
  if (known) {
    return { mode: "known", known: await buildKnownSiteStatus(known) };
  }

  const detected = await detectViaContent(tabId);
  if (detected?.mode === "unknown" && detected.detectedSchema) {
    const unknown: UnknownSiteStatus = {
      detectedSchema: detected.detectedSchema,
      pageType: detected.pageType,
      url: detected.url,
    };
    return { mode: "unknown", unknown };
  }

  return { mode: "none" };
}

async function resolveTabSiteStatus(tabId: number, url: string): Promise<TabSiteStatus> {
  const isCurrent = tabGenerations.begin(tabId);
  const status = await detectTabSiteStatus(tabId, url);
  if (!isCurrent()) {
    // 同一标签页已经开始了更新的解析（比如已经离开这个页面）：这次的结果作废，不写状态、不动 Badge
    return status;
  }
  return setTabMode(tabId, status);
}

async function resolveCurrentTabStatus(): Promise<TabSiteStatus> {
  const tab = await getActiveTab();
  if (!tab?.id || !tab.url) {
    return { mode: "none" };
  }

  const existing = await getTabStatus(tab.id);
  if (existing) {
    return existing;
  }

  return resolveTabSiteStatus(tab.id, tab.url);
}

function buildUnknownSiteInfo(page: CapturedPage): SiteInfo {
  const url = new URL(page.url);
  return {
    name: url.hostname,
    url: `${url.protocol}//${url.host}`,
    schema: page.detectedSchema,
    authMethod: "cookie",
  };
}

async function getOrCreateUnknownSession(page: CapturedPage): Promise<CollectionSession> {
  const existing = await getActiveSession();
  if (existing && existing.status !== "exported") {
    const hostA = extractDomain(existing.site.url);
    const hostB = extractDomain(page.url);
    if (hostA && hostA === hostB) {
      return existing;
    }
  }

  const session: CollectionSession = {
    id: crypto.randomUUID(),
    site: buildUnknownSiteInfo(page),
    pages: [],
    createdAt: new Date().toISOString(),
    status: "collecting",
  };

  await saveSession(session);
  await setActiveSessionId(session.id);
  return session;
}

async function upsertCapturedPage(page: CapturedPage): Promise<void> {
  const activeSession = await getOrCreateUnknownSession(page);

  const nextPages = [...activeSession.pages];
  const existingIndex = nextPages.findIndex((item) => item.pageType === page.pageType);
  if (existingIndex >= 0) {
    nextPages[existingIndex] = page;
  } else {
    nextPages.push(page);
  }

  const status: CollectionSession["status"] = nextPages.length >= 3 ? "complete" : "collecting";
  const nextSession: CollectionSession = {
    ...activeSession,
    pages: nextPages,
    status,
  };

  await saveSession(nextSession);
  await publishStatus();
}

/** pushLock 是推 Cookie 的锁：定时同步、手动同步、批量同步与 Cookie 变化时的自动同步共用一把（见 scheduled-sync 的 createLock）。 */
const pushLock = createLock();

/**
 * syncKnownSite 把站点的 Cookie 推给 pt-tools，返回推了没有。skipUnchanged 时（自动同步）和上次推给同一个 pt-tools 的一样就不推：
 * 每推一次 pt-tools 都会启用站点、刷新站点实例、做一次登录探测并重载调度器。手动同步总是推。
 * 读 Cookie、比指纹、推送、记指纹都在 pushLock 里。
 */
function syncKnownSite(site: KnownSite, opts: { skipUnchanged?: boolean } = {}): Promise<boolean> {
  return pushLock(async () => {
    if (site.syncField !== "cookie") {
      throw new Error(t("error.authSyncUnsupported", site.name, site.syncField));
    }

    const connection = await getConnection();
    const baseUrl = connection.baseUrl?.trim();
    if (!baseUrl) {
      throw new Error(t("error.configurePtToolsUrl"));
    }

    const health = await checkCookieHealth(site);
    if (!health.cookieString || health.status === "missing") {
      throw new Error(t("error.cookieMissingCannotSync", site.name));
    }

    const hash = await pushFingerprint(baseUrl, health.cookieString);
    if (opts.skipUnchanged && (await getSyncHashMap())[site.id] === hash) {
      return false;
    }

    const api = new PtToolsApiClient(baseUrl);
    await api.syncSiteCredential(site, health.cookieString);

    const syncedAt = new Date().toISOString();
    await setLastSync(site.id, syncedAt);
    await setSyncHash(site.id, hash);
    // 请求期间用户可能在设置页换了地址：只在地址没变时更新连接状态，不把请求前读到的旧 connection 整个写回
    await updateConnection(baseUrl, { connected: true, lastSync: syncedAt });
    return true;
  });
}

async function maybeAutoSyncByDomain(domain: string): Promise<void> {
  const normalized = normalizeCookieDomain(domain);
  const matched = KNOWN_SITES.find((site) =>
    site.domains.some((known) => matchesDomain(normalized, known)),
  );
  if (!matched || matched.syncField !== "cookie") {
    return;
  }

  const autoSyncMap = await getAutoSyncMap();
  if (!autoSyncMap[matched.id]) {
    return;
  }

  const now = Date.now();
  const lastRun = autoSyncThrottle.get(matched.id) ?? 0;
  if (now - lastRun < AUTO_SYNC_INTERVAL_MS) {
    return;
  }

  autoSyncThrottle.set(matched.id, now);

  try {
    if (!(await syncKnownSite(matched, { skipUnchanged: true }))) {
      return;
    }
    logger.info("Auto-sync completed", { siteId: matched.id });
    notifyAutoSync("success", matched.name, t("notification.autoSyncSuccessBody", matched.name));
    const [activeTab, map] = await Promise.all([getActiveTab(), getTabStatusMap()]);
    if (activeTab?.id && map[String(activeTab.id)]?.mode === "known") {
      await resolveCurrentTabStatus();
    }
  } catch (error: unknown) {
    logger.warn("Auto-sync failed", { siteId: matched.id, error });
    const reason = error instanceof Error ? friendlyErrorMessage(error.message) : String(error);
    notifyAutoSync(
      "failure",
      matched.name,
      t("notification.autoSyncFailureBody", matched.name, reason),
    );
  }
}

function notifyAutoSync(kind: "success" | "failure", siteName: string, body: string): void {
  if (typeof chrome === "undefined" || !chrome.notifications?.create) {
    return;
  }
  const titleKey =
    kind === "success" ? "notification.autoSyncSuccess" : "notification.autoSyncFailure";
  chrome.notifications.create({
    type: "basic",
    iconUrl: chrome.runtime.getURL("icons/icon128.png"),
    title: t(titleKey, siteName),
    message: body,
    priority: kind === "failure" ? 2 : 0,
  });
}

onMessage("SITE_DETECTED", async (payload: SiteDetectedPayload, sender) => {
  if (!sender.tabId) {
    return { ok: true };
  }

  if (payload.mode === "known" && payload.knownSiteId) {
    const site = KNOWN_SITES.find((item) => item.id === payload.knownSiteId);
    if (site) {
      const knownStatus = await buildKnownSiteStatus(site);
      await setTabMode(sender.tabId, { mode: "known", known: knownStatus });
      return { ok: true };
    }
  }

  if (payload.mode === "unknown" && payload.detectedSchema) {
    await setTabMode(sender.tabId, {
      mode: "unknown",
      unknown: {
        detectedSchema: payload.detectedSchema,
        pageType: payload.pageType,
        url: payload.url,
      },
    });
    return { ok: true };
  }

  await setTabMode(sender.tabId, { mode: "none" });
  return { ok: true };
});

onMessage("CAPTURE_PAGE", async () => {
  const tab = await getActiveTab();
  if (!tab?.id) {
    throw new Error(t("error.noActiveTab"));
  }

  const injected = await ensureContentScript(tab.id);
  if (!injected) {
    throw new Error(t("error.scriptInjectionFailed"));
  }

  await sendToContent(tab.id, createMessage("CAPTURE_PAGE", {}));
  return { dispatched: true };
});

onMessage("PAGE_CAPTURED", async (payload: CapturedPage) => {
  await upsertCapturedPage(payload);
  return { stored: true };
});

onMessage("AUTO_COLLECT", async ({ siteOrigin, schema }) => {
  const { pages, warnings } = await autoCollect(siteOrigin, schema);

  for (const page of pages) {
    await upsertCapturedPage(page);
  }

  await publishStatus();
  return { collected: pages.length, pages: pages.map((p) => p.pageType), warnings };
});

onMessage("GET_STATUS", async () => getStatusPayload());

onMessage("GET_TAB_STATUS", async () => resolveCurrentTabStatus());

onMessage("GET_ALL_SITES_STATUS", async () => {
  const statuses: KnownSiteStatus[] = [];
  for (const site of KNOWN_SITES) {
    statuses.push(await buildKnownSiteStatus(site));
  }
  return statuses;
});

onMessage("GET_SITE_LOGIN_STATE", async (): Promise<SiteLoginStateRecord[]> => {
  const connection = await getConnection();
  const baseUrl = connection.baseUrl?.trim();
  if (!baseUrl) {
    throw new Error(t("error.configurePtToolsUrl"));
  }
  const api = new PtToolsApiClient(baseUrl);
  return api.getSiteLoginStatus();
});

onMessage("SYNC_SITE_COOKIES", async ({ siteId }) => {
  const site = KNOWN_SITES.find((item) => item.id === siteId);
  if (!site) {
    throw new Error(t("error.unknownSite"));
  }

  await syncKnownSite(site);
  const refreshed = await resolveCurrentTabStatus();
  return {
    ok: true,
    status: refreshed,
  };
});

onMessage("TOGGLE_AUTO_SYNC", async ({ siteId, enabled }) => {
  await setAutoSync(siteId, enabled);
  const refreshed = await resolveCurrentTabStatus();
  return {
    ok: true,
    enabled,
    status: refreshed,
  };
});

onMessage("BATCH_SYNC_SITES", async ({ siteIds }) => {
  const synced: string[] = [];
  const failed: Array<{ siteId: string; error: string }> = [];

  for (const siteId of siteIds) {
    const site = KNOWN_SITES.find((item) => item.id === siteId);
    if (!site) {
      failed.push({ siteId, error: t("error.unknownSite") });
      continue;
    }

    try {
      await syncKnownSite(site);
      synced.push(siteId);
    } catch (error: unknown) {
      failed.push({
        siteId,
        error: error instanceof Error ? error.message : t("feedback.syncFailed"),
      });
    }
  }

  await publishStatus();
  return { synced, failed };
});

onMessage("BATCH_OPEN_TABS", async ({ siteIds, timeoutMs }) => {
  const ok: string[] = [];
  const failed: Array<{ siteId: string; reason: string }> = [];
  const skipped: string[] = [];
  const delayMs = Math.max(0, timeoutMs ?? 150);

  for (const siteId of siteIds) {
    const site = KNOWN_SITES.find((item) => item.id === siteId);
    if (!site) {
      failed.push({ siteId, reason: t("error.unknownSite") });
      continue;
    }
    const domain = site.domains[0];
    if (!domain) {
      skipped.push(siteId);
      continue;
    }
    try {
      await chrome.tabs.create({ url: `https://${domain}`, active: false });
      ok.push(siteId);
      if (delayMs > 0) {
        await new Promise((resolve) => setTimeout(resolve, delayMs));
      }
    } catch (error: unknown) {
      failed.push({ siteId, reason: error instanceof Error ? error.message : "open tab failed" });
    }
  }

  return { ok, failed, skipped };
});

onMessage("CHECK_EXTENSION_UPDATE", async () => {
  const currentVersion = chrome.runtime.getManifest().version;
  const releaseUrl = "https://github.com/sunerpy/pt-tools/releases";
  if (!chrome.runtime.requestUpdateCheck) {
    return { currentVersion, status: "unavailable", releaseUrl };
  }
  const status = await new Promise<"checking" | "no_update" | "update_available" | "throttled">(
    (resolve) => {
      chrome.runtime.requestUpdateCheck((result) => resolve(result));
    },
  );
  return { currentVersion, status, releaseUrl };
});

onMessage("SYNC_COOKIES", async ({ baseUrl, username, password }) => {
  const api = new PtToolsApiClient(baseUrl);
  const isExplicitTest = Boolean(username && password);
  const loginOk = isExplicitTest ? await api.login(username!, password!) : true;

  if (!loginOk) {
    throw new Error(t("error.ptToolsLoginFailed"));
  }

  let connected = false;
  try {
    await api.ping();
    connected = true;
  } catch (error) {
    if (isExplicitTest) {
      throw error;
    }
    logger.warn("ping failed during settings save", { error });
  }

  const connection: PtToolsConnection = {
    baseUrl,
    sessionId: "",
    connected,
    lastSync: connected ? new Date().toISOString() : null,
  };

  await setConnection(connection);
  if (connected) {
    await ensurePollAlarm();
  } else {
    await clearPollAlarm();
  }
  await publishStatus();
  return connection;
});

// ---- 定时同步与 CookieCloud（M22） ----
// 逻辑在 ./scheduled-sync 与 modules/cookiecloud；这里接 chrome API、存储与网络。

const SYNC_ALARM_NAME = "pt-tools-cookie-sync";

async function getSchedule(): Promise<SyncSchedule> {
  return normalizeSchedule((await getSyncSchedule()) ?? DEFAULT_SYNC_SCHEDULE);
}

/** applySyncSchedule 按设置建或删定时同步的 alarm；间隔没变时不重建（不打乱已经排好的下一次）。 */
async function applySyncSchedule(schedule: SyncSchedule): Promise<void> {
  const existing = await chrome.alarms.get(SYNC_ALARM_NAME);
  if (!schedule.enabled) {
    if (existing) {
      await chrome.alarms.clear(SYNC_ALARM_NAME);
    }
    return;
  }
  if (existing?.periodInMinutes === schedule.intervalMinutes) {
    return;
  }
  await chrome.alarms.create(SYNC_ALARM_NAME, {
    delayInMinutes: schedule.intervalMinutes,
    periodInMinutes: schedule.intervalMinutes,
  });
}

function asAuthExpired(error: unknown): unknown {
  return error instanceof PtToolsApiError && error.code === "auth_required"
    ? new AuthExpiredError()
    : error;
}

function errorText(error: unknown): string {
  return error instanceof Error ? friendlyErrorMessage(error.message) : String(error);
}

const scheduledSyncDeps: ScheduledSyncDeps = {
  sites: KNOWN_SITES,
  getSchedule,
  getAutoSyncMap,
  getHash: async (siteId) => (await getSyncHashMap())[siteId],
  withPushLock: (fn) => pushLock(fn),
  ptTools: async () => {
    const connection = await getConnection();
    const baseUrl = connection.baseUrl?.trim();
    return connection.connected && baseUrl ? baseUrl : null;
  },
  enabledSites: async (baseUrl) => {
    try {
      const sites = await new PtToolsApiClient(baseUrl).getSites();
      return sites.filter((site) => site.enabled).map((site) => site.name.toLowerCase());
    } catch (error: unknown) {
      throw asAuthExpired(error);
    }
  },
  readCookie: async (site) => {
    const health = await checkCookieHealth(site);
    return health.status === "valid" || health.status === "expiring" ? health.cookieString : "";
  },
  pushCookie: async (baseUrl, site, cookie) => {
    try {
      await new PtToolsApiClient(baseUrl).syncSiteCredential(site, cookie);
    } catch (error: unknown) {
      throw asAuthExpired(error);
    }
  },
  recordPushed: async (siteId, hash, at) => {
    await setSyncHash(siteId, hash);
    await setLastSync(siteId, at);
  },
  authExpired: async (baseUrl) => {
    await updateConnection(baseUrl, { connected: false });
    await clearPollAlarm();
  },
  getCookieCloudConfig,
  getCookieCloudState,
  saveCookieCloudState: setCookieCloudState,
  upload: uploadToCookieCloud,
  errorText,
  now: () => new Date(),
};

/** problemsOf 是一轮同步里出的问题（给通知用），没有问题时为空串。 */
function problemsOf(run: SyncRunSummary | null): string {
  if (!run) {
    return "";
  }
  const parts: string[] = [];
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
  if (run.cookieCloud.error) {
    parts.push(t("sync.problemCookieCloud", run.cookieCloud.error));
  }
  return parts.join("\n");
}

let syncInFlight: Promise<SyncRunSummary> | null = null;

/** runScheduledSync 跑一轮定时同步（同一时间只跑一轮），记下结果；出了和上一轮不一样的问题时发一条通知。 */
function runScheduledSync(trigger: SyncRunSummary["trigger"]): Promise<SyncRunSummary> {
  if (syncInFlight) {
    return syncInFlight;
  }
  syncInFlight = (async () => {
    try {
      const previous = await getLastSyncRun();
      const summary = await runSync(scheduledSyncDeps, trigger);
      await setLastSyncRun(summary);
      const problems = problemsOf(summary);
      if (trigger === "schedule" && problems && problems !== problemsOf(previous)) {
        notifySyncProblem(problems);
      }
      return summary;
    } finally {
      syncInFlight = null;
    }
  })();
  return syncInFlight;
}

function notifySyncProblem(body: string): void {
  if (typeof chrome === "undefined" || !chrome.notifications?.create) {
    return;
  }
  chrome.notifications.create({
    type: "basic",
    iconUrl: chrome.runtime.getURL("icons/icon128.png"),
    title: t("sync.problemTitle"),
    message: body,
    priority: 1,
  });
}

/** requireExtensionPage 拒绝网页里的内容脚本发来的设置类消息：这些只在扩展的弹窗里改。 */
function requireExtensionPage(sender: MessageSender): void {
  if (sender.tabId !== undefined) {
    throw new Error(t("error.extensionPageOnly"));
  }
}

async function syncSettingsView(): Promise<SyncSettingsView> {
  const [schedule, cfg, state, lastRun] = await Promise.all([
    getSchedule(),
    getCookieCloudConfig(),
    getCookieCloudState(),
    getLastSyncRun(),
  ]);
  return { schedule, cookieCloud: cookieCloudView(cfg, state), lastRun };
}

onMessage("GET_SYNC_SETTINGS", async (_payload, sender) => {
  requireExtensionPage(sender);
  return syncSettingsView();
});

onMessage("SAVE_SYNC_SCHEDULE", async (payload, sender) => {
  requireExtensionPage(sender);
  const schedule = normalizeSchedule(payload);
  await setSyncSchedule(schedule);
  await applySyncSchedule(schedule);
  return syncSettingsView();
});

onMessage("RUN_SYNC_NOW", async (_payload, sender) => {
  requireExtensionPage(sender);
  await runScheduledSync("manual");
  return syncSettingsView();
});

onMessage("SAVE_COOKIECLOUD", async (payload, sender) => {
  requireExtensionPage(sender);
  const current = await getCookieCloudConfig();
  const next = mergeCookieCloudInput(payload, current);
  await setCookieCloudConfig(next);
  const changed =
    next.serverUrl !== current.serverUrl ||
    next.uuid !== current.uuid ||
    next.password !== current.password ||
    next.cryptoType !== current.cryptoType;
  if (changed) {
    // 换了服务地址、UUID、密码或加密方式：下一次定时同步重新上传
    const state = (await getCookieCloudState()) ?? EMPTY_COOKIECLOUD_STATE;
    await setCookieCloudState({ ...state, lastDigest: null, lastTarget: null });
  }
  return syncSettingsView();
});

onMessage("TEST_COOKIECLOUD", async (_payload, sender) => {
  requireExtensionPage(sender);
  return testCookieCloud(await getCookieCloudConfig());
});

onMessage("UPLOAD_COOKIECLOUD", async (payload, sender) => {
  requireExtensionPage(sender);
  const cfg = await getCookieCloudConfig();
  const state = await getCookieCloudState();
  try {
    const { result, state: next } = await uploadToCookieCloud(cfg, state, {
      force: true,
      overwrite: payload?.overwrite === true,
    });
    await setCookieCloudState(next);
    return { result, settings: await syncSettingsView() };
  } catch (error: unknown) {
    await setCookieCloudState({
      ...(state ?? EMPTY_COOKIECLOUD_STATE),
      lastError: errorText(error),
      lastErrorAt: new Date().toISOString(),
    });
    if (
      error instanceof CookieCloudError &&
      error.code === "decrypt" &&
      payload?.overwrite !== true
    ) {
      // 让弹窗问用户要不要覆盖（改过密码时）；不是错误，不抛
      return { needsOverwrite: true, message: error.message, settings: await syncSettingsView() };
    }
    throw error;
  }
});

if (chrome.alarms?.onAlarm) {
  chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name !== SYNC_ALARM_NAME) {
      return;
    }
    void runScheduledSync("schedule").catch((error: unknown) => {
      logger.warn("scheduled sync failed", error);
    });
  });
}
// ---- end 定时同步与 CookieCloud ----

chrome.tabs.onActivated.addListener(({ tabId }) => {
  void (async () => {
    try {
      const tab = await chrome.tabs.get(tabId);
      if (tab.url) {
        await resolveTabSiteStatus(tabId, tab.url);
      }
    } catch {
      // tabs permission may not be granted yet
    }
  })();
});

chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  if (!tab.url) {
    return;
  }
  if (changeInfo.status !== "complete" && !changeInfo.url) {
    return;
  }
  void resolveTabSiteStatus(tabId, tab.url).catch(() => {});
});

chrome.tabs.onRemoved.addListener((tabId) => {
  tabGenerations.forget(tabId);
  void removeTabStatus(tabId);
});

if (chrome.cookies?.onChanged) {
  chrome.cookies.onChanged.addListener((changeInfo) => {
    void maybeAutoSyncByDomain(changeInfo.cookie.domain);
  });
}

chrome.permissions.onAdded.addListener((permissions) => {
  if (permissions.permissions?.includes("cookies") && chrome.cookies?.onChanged) {
    chrome.cookies.onChanged.addListener((changeInfo) => {
      void maybeAutoSyncByDomain(changeInfo.cookie.domain);
    });
  }
});

chrome.runtime.onInstalled.addListener(() => {
  void chrome.action.setBadgeText({ text: "" });
});

chrome.runtime.onStartup.addListener(() => {
  void publishStatus();
});

// ---- webNavigation visit tracking (T14) ----
// Tracks real user visits to KNOWN_SITES to feed backend last-visit signal.
// Privacy boundary: hostSuffix filter limits events to known PT site domains only.

async function handleVisitDetected(
  details:
    | chrome.webNavigation.WebNavigationFramedCallbackDetails
    | chrome.webNavigation.WebNavigationTransitionCallbackDetails,
): Promise<void> {
  // Skip subframes (iframes / workers)
  if (details.frameId !== 0) {
    return;
  }
  const site = matchKnownSite(details.url);
  if (!site) {
    return;
  }
  // Permission gate — silently skip if user has not granted webNavigation
  if (!(await hasWebNavigationPermission())) {
    return;
  }

  const visitedAt = Date.now();
  await setLastVisit(site.id, visitedAt);

  const connection = await getConnection();
  const baseUrl = connection.baseUrl?.trim();
  if (!baseUrl) {
    return;
  }

  const api = new PtToolsApiClient(baseUrl);
  void api.reportVisit(site.id, visitedAt);
}

const visitNavigationFilter: chrome.events.UrlFilter[] = KNOWN_SITES.flatMap((site) =>
  site.domains.map((hostSuffix) => ({ hostSuffix })),
);

if (chrome.webNavigation?.onCompleted) {
  // Initial navigation + reloads
  chrome.webNavigation.onCompleted.addListener(
    (details) => {
      void handleVisitDetected(details).catch((error) => {
        logger.warn("handleVisitDetected (onCompleted) failed", error);
      });
    },
    { url: visitNavigationFilter },
  );
}

if (chrome.webNavigation?.onHistoryStateUpdated) {
  // SPA route changes (e.g. mTorrent client-side routing)
  chrome.webNavigation.onHistoryStateUpdated.addListener(
    (details) => {
      void handleVisitDetected(details).catch((error) => {
        logger.warn("handleVisitDetected (onHistoryStateUpdated) failed", error);
      });
    },
    { url: visitNavigationFilter },
  );
}
// ---- end webNavigation visit tracking ----

// ---- Pending action polling (T17) ----
// 动作的取回、执行与 ack 见 ./pending-actions；这里只接 chrome API、存储与网络。
// R28 forbids setInterval — we use chrome.alarms.

const POLL_ALARM_NAME = "pt-tools-poll";
const POLL_PERIOD_MINUTES = 0.5;
/** 旧版本按 since 游标取动作，已不再使用；启动时清掉 */
const LEGACY_POLL_SINCE_KEY = "pt_tools_pending_actions_since";
/** 已打开标签页、还没 ack 成功的动作 ID */
const POLL_OPENED_IDS_KEY = "pt_tools_pending_actions_opened";

function urlMatchesKnownSite(rawURL: string): KnownSite | null {
  let host: string;
  try {
    host = new URL(rawURL).hostname.toLowerCase();
  } catch {
    return null;
  }
  for (const site of KNOWN_SITES) {
    for (const domain of site.domains) {
      const normalized = domain.toLowerCase();
      if (host === normalized || host.endsWith(`.${normalized}`)) {
        return site;
      }
    }
  }
  return null;
}

class PendingAuthError extends Error {
  constructor() {
    super("pending poll unauthorized");
    this.name = "PendingAuthError";
  }
}

async function fetchPending(baseUrl: string): Promise<BackendPendingAction[]> {
  const url = `${baseUrl.replace(/\/+$/, "")}/api/extension/actions/pending`;
  // 带超时：请求卡住时不让一轮轮询一直挂着
  const response = await fetchWithTimeout(url, {
    method: "GET",
    headers: { Accept: "application/json" },
    credentials: "include",
    redirect: "manual",
  });
  if (response.status === 401 || response.status === 0 || response.type === "opaqueredirect") {
    throw new PendingAuthError();
  }
  if (!response.ok) {
    throw new Error(`pending fetch HTTP ${response.status}`);
  }
  const text = await response.text();
  if (!text) return [];
  const data: unknown = JSON.parse(text);
  if (!isPendingActionArray(data)) {
    throw new Error("pending response did not match schema");
  }
  return data;
}

async function ackAction(baseUrl: string, actionId: number): Promise<void> {
  const url = `${baseUrl.replace(/\/+$/, "")}/api/extension/actions/${encodeURIComponent(
    String(actionId),
  )}/ack`;
  const response = await fetchWithTimeout(url, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    credentials: "include",
    redirect: "manual",
  });
  if (!response.ok) {
    throw new Error(`ack HTTP ${response.status}`);
  }
}

const pendingPoller = createPendingActionPoller({
  fetchPending,
  ack: ackAction,
  openTab: async (url) => {
    await chrome.tabs.create({ url, active: false });
  },
  isKnownSiteUrl: (url) => urlMatchesKnownSite(url) !== null,
  loadOpenedIds: async () => (await get<number[]>(POLL_OPENED_IDS_KEY)) ?? [],
  saveOpenedIds: (ids) => set(POLL_OPENED_IDS_KEY, ids),
  now: () => Date.now(),
  log: logger,
});

export async function pollPendingActions(): Promise<void> {
  const connection = await getConnection();
  const baseUrl = connection.baseUrl?.trim();
  if (!baseUrl || !connection.connected) {
    return;
  }
  try {
    await pendingPoller.poll(baseUrl);
  } catch (error: unknown) {
    if (error instanceof PendingAuthError) {
      await clearPollAlarm();
      await updateConnection(baseUrl, { connected: false });
      logger.warn("pollPendingActions unauthorized; pausing poll until re-login");
      return;
    }
    logger.warn("pollPendingActions fetch failed", error);
  }
}

async function ensurePollAlarm(): Promise<void> {
  const existing = await chrome.alarms.get(POLL_ALARM_NAME);
  if (existing) {
    return;
  }
  await chrome.alarms.create(POLL_ALARM_NAME, { periodInMinutes: POLL_PERIOD_MINUTES });
}

async function clearPollAlarm(): Promise<void> {
  await chrome.alarms.clear(POLL_ALARM_NAME);
}

if (chrome.alarms?.onAlarm) {
  chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name !== POLL_ALARM_NAME) {
      return;
    }
    void pollPendingActions().catch((error) => {
      logger.warn("pollPendingActions handler failed", error);
    });
  });
}
// ---- end pending action polling ----

void (async () => {
  logger.info("Background bootstrap", {
    pollAlarmName: POLL_ALARM_NAME,
    pollPeriodMinutes: POLL_PERIOD_MINUTES,
  });
  const bootConnection = await getConnection();
  if (bootConnection.connected) {
    await ensurePollAlarm();
  } else {
    await clearPollAlarm();
  }
  await applySyncSchedule(await getSchedule());
  void STORAGE_KEYS.batchTabQueue;
  await remove(LEGACY_POLL_SINCE_KEY);
  const tab = await getActiveTab();
  if (tab?.id && tab.url) {
    await resolveTabSiteStatus(tab.id, tab.url);
  } else {
    await publishStatus();
  }
})().catch((error: unknown) => logger.warn("Background bootstrap failed", error));
