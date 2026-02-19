import { KNOWN_SITES, type KnownSite } from "../core/constants";
import { t } from "../core/i18n";
import { logger } from "../core/logger";
import { createMessage, onMessage, sendToContent, sendToPopup } from "../core/messages";
import {
  getActiveSession,
  getAutoSyncMap,
  getConnection,
  getLastSyncMap,
  getTabStatus,
  getTabStatusMap,
  removeTabStatus,
  saveSession,
  setActiveSessionId,
  setAutoSync,
  setConnection,
  setLastSync,
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
import { PtToolsApiClient, checkCookieHealth } from "../modules/sync";
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
    const [activeTab, map] = await Promise.all([getActiveTab(), getTabStatusMap()]);
    if (activeTab?.id && map[String(activeTab.id)]?.mode === "known") {
      await resolveCurrentTabStatus();
    }
  } catch (error: unknown) {
    logger.warn("Auto-sync failed", { siteId: matched.id, error });
  }
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

onMessage("SYNC_COOKIES", async ({ baseUrl, username, password }) => {
  const api = new PtToolsApiClient(baseUrl);
  const loginOk = username && password ? await api.login(username, password) : true;

  if (!loginOk) {
    throw new Error(t("error.ptToolsLoginFailed"));
  }

  const connected = await api.ping();
  const connection: PtToolsConnection = {
    baseUrl,
    sessionId: "",
    connected,
    lastSync: connected ? new Date().toISOString() : null,
  };

  await setConnection(connection);
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

void (async () => {
  const tab = await getActiveTab();
  if (tab?.id && tab.url) {
    await resolveTabSiteStatus(tab.id, tab.url);
  } else {
    await publishStatus();
  }
})().catch((error: unknown) => logger.warn("Background bootstrap failed", error));
