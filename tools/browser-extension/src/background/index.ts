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
  getTabStatus,
  getTabStatusMap,
  removeTabStatus,
  saveSession,
  set,
  setActiveSessionId,
  setAutoSync,
  setConnection,
  setLastSync,
  setLastVisit,
  setTabStatus,
} from "../core/storage";
import type {
  CapturedPage,
  CollectionSession,
  KnownSiteStatus,
  PtToolsConnection,
  SiteDetectedPayload,
  SiteInfo,
  StatusPayload,
  TabSiteStatus,
  UnknownSiteStatus,
} from "../core/types";
import { PtToolsApiClient, checkCookieHealth, friendlyErrorMessage } from "../modules/sync";
import type { SiteLoginStateRecord } from "../modules/sync";
import { autoCollect } from "../modules/collector/auto-collector";
import { extractDomain, matchKnownSite } from "../utils";

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

async function updateBadge(status: TabSiteStatus): Promise<void> {
  if (status.mode === "known") {
    await chrome.action.setBadgeText({ text: "✓" });
    await chrome.action.setBadgeBackgroundColor({ color: "#1f8a70" });
    return;
  }

  if (status.mode === "unknown") {
    await chrome.action.setBadgeText({ text: "?" });
    await chrome.action.setBadgeBackgroundColor({ color: "#e6a23c" });
    return;
  }

  await chrome.action.setBadgeText({ text: "" });
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
  await updateBadge(status);
  await publishStatus();
  return status;
}

async function resolveTabSiteStatus(tabId: number, url: string): Promise<TabSiteStatus> {
  const known = matchKnownSite(url);
  if (known) {
    const knownStatus = await buildKnownSiteStatus(known);
    return setTabMode(tabId, {
      mode: "known",
      known: knownStatus,
    });
  }

  const detected = await detectViaContent(tabId);
  if (detected?.mode === "unknown" && detected.detectedSchema) {
    const unknown: UnknownSiteStatus = {
      detectedSchema: detected.detectedSchema,
      pageType: detected.pageType,
      url: detected.url,
    };
    return setTabMode(tabId, { mode: "unknown", unknown });
  }

  return setTabMode(tabId, { mode: "none" });
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

async function syncKnownSite(site: KnownSite): Promise<void> {
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

  const api = new PtToolsApiClient(baseUrl);
  await api.syncSiteCredential(site, health.cookieString);

  const syncedAt = new Date().toISOString();
  await setLastSync(site.id, syncedAt);
  await setConnection({
    ...connection,
    connected: true,
    lastSync: syncedAt,
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
    await syncKnownSite(matched);
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
  const pages = await autoCollect(siteOrigin, schema);

  for (const page of pages) {
    await upsertCapturedPage(page);
  }

  await publishStatus();
  return { collected: pages.length, pages: pages.map((p) => p.pageType) };
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
// Backend queues "open SiteX tab" actions; extension polls every 30s,
// executes via chrome.tabs.create on KNOWN_SITES only, then acks.
// Replaces v1 batchOpenTabsForSync (push-on-demand model). R-EC10 enforces
// 24h TTL server-side. R28 forbids setInterval — we use chrome.alarms.

const POLL_ALARM_NAME = "pt-tools-poll";
const POLL_PERIOD_MINUTES = 0.5;
const POLL_LAST_TIMESTAMP_KEY = "pt_tools_pending_actions_since";
const POLL_MAX_ACTION_AGE_MS = 24 * 60 * 60 * 1000;

interface BackendPendingAction {
  id: number;
  type: string;
  target_url: string;
  site_name?: string;
  reason?: string;
  created_at?: string;
  expires_at?: string;
}

function isPendingActionArray(value: unknown): value is BackendPendingAction[] {
  if (!Array.isArray(value)) return false;
  return value.every(
    (item) =>
      typeof item === "object" &&
      item !== null &&
      typeof (item as { id?: unknown }).id === "number" &&
      typeof (item as { type?: unknown }).type === "string" &&
      typeof (item as { target_url?: unknown }).target_url === "string",
  );
}

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

function actionTimestampMs(action: BackendPendingAction): number {
  if (action.created_at) {
    const t = Date.parse(action.created_at);
    if (!Number.isNaN(t)) return t;
  }
  return Date.now();
}

class PendingAuthError extends Error {
  constructor() {
    super("pending poll unauthorized");
    this.name = "PendingAuthError";
  }
}

async function fetchPending(baseUrl: string, since: number): Promise<BackendPendingAction[]> {
  const url = new URL(`${baseUrl.replace(/\/+$/, "")}/api/extension/actions/pending`);
  if (since > 0) {
    url.searchParams.set("since", String(since));
  }
  const response = await fetch(url.toString(), {
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
  const response = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    credentials: "include",
    redirect: "manual",
  });
  if (!response.ok) {
    throw new Error(`ack HTTP ${response.status}`);
  }
}

async function getPollSince(): Promise<number> {
  return (await get<number>(POLL_LAST_TIMESTAMP_KEY)) ?? 0;
}

async function setPollSince(unixSeconds: number): Promise<void> {
  await set(POLL_LAST_TIMESTAMP_KEY, unixSeconds);
}

export async function pollPendingActions(): Promise<void> {
  const connection = await getConnection();
  const baseUrl = connection.baseUrl?.trim();
  if (!baseUrl || !connection.connected) {
    return;
  }
  let actions: BackendPendingAction[];
  try {
    const since = await getPollSince();
    actions = await fetchPending(baseUrl, since);
  } catch (error: unknown) {
    if (error instanceof PendingAuthError) {
      await clearPollAlarm();
      await setConnection({ ...connection, connected: false });
      logger.warn("pollPendingActions unauthorized; pausing poll until re-login");
      return;
    }
    logger.warn("pollPendingActions fetch failed", error);
    return;
  }
  if (actions.length === 0) {
    return;
  }
  const now = Date.now();
  let maxCreatedSec = 0;
  for (const action of actions) {
    const createdMs = actionTimestampMs(action);
    if (now - createdMs > POLL_MAX_ACTION_AGE_MS) {
      logger.info("pollPendingActions skipping expired action", { id: action.id });
      continue;
    }
    if (action.type !== "open_tab") {
      logger.info("pollPendingActions skipping unsupported action type", {
        id: action.id,
        type: action.type,
      });
      try {
        await ackAction(baseUrl, action.id);
      } catch (error: unknown) {
        logger.warn("ack of unsupported action failed", { id: action.id, error });
      }
      const createdSec = Math.floor(createdMs / 1000);
      if (createdSec > maxCreatedSec) maxCreatedSec = createdSec;
      continue;
    }
    const matched = urlMatchesKnownSite(action.target_url);
    if (!matched) {
      logger.warn("pollPendingActions skipping non-known-site URL", {
        id: action.id,
        url: action.target_url,
      });
      try {
        await ackAction(baseUrl, action.id);
      } catch (error: unknown) {
        logger.warn("ack of skipped action failed", { id: action.id, error });
      }
      const createdSec = Math.floor(createdMs / 1000);
      if (createdSec > maxCreatedSec) maxCreatedSec = createdSec;
      continue;
    }
    try {
      await chrome.tabs.create({ url: action.target_url, active: false });
      await ackAction(baseUrl, action.id);
      logger.info("pollPendingActions executed open_tab", {
        id: action.id,
        siteId: matched.id,
      });
      const createdSec = Math.floor(createdMs / 1000);
      if (createdSec > maxCreatedSec) maxCreatedSec = createdSec;
    } catch (error: unknown) {
      logger.warn("pollPendingActions open_tab failed", { id: action.id, error });
    }
  }
  if (maxCreatedSec > 0) {
    await setPollSince(maxCreatedSec);
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
  void STORAGE_KEYS.batchTabQueue;
  const tab = await getActiveTab();
  if (tab?.id && tab.url) {
    await resolveTabSiteStatus(tab.id, tab.url);
  } else {
    await publishStatus();
  }
})().catch((error: unknown) => logger.warn("Background bootstrap failed", error));
