import { STORAGE_KEYS } from "./constants";
import type { CollectionSession, PtToolsConnection, TabSiteStatus } from "./types";

const DEFAULT_CONNECTION: PtToolsConnection = {
  baseUrl: "http://localhost:8080",
  sessionId: "",
  connected: false,
  lastSync: null,
};

export async function get<T>(key: string): Promise<T | null> {
  const result = await chrome.storage.local.get(key);
  return (result[key] as T | undefined) ?? null;
}

export async function set<T>(key: string, value: T): Promise<void> {
  await chrome.storage.local.set({ [key]: value });
}

export async function remove(key: string): Promise<void> {
  await chrome.storage.local.remove(key);
}

export async function getConnection(): Promise<PtToolsConnection> {
  return (await get<PtToolsConnection>(STORAGE_KEYS.connection)) ?? DEFAULT_CONNECTION;
}

export async function setConnection(connection: PtToolsConnection): Promise<void> {
  await set(STORAGE_KEYS.connection, connection);
}

export async function getSessions(): Promise<CollectionSession[]> {
  return (await get<CollectionSession[]>(STORAGE_KEYS.sessions)) ?? [];
}

export async function saveSession(session: CollectionSession): Promise<void> {
  const sessions = await getSessions();
  const index = sessions.findIndex((item) => item.id === session.id);
  if (index >= 0) {
    sessions[index] = session;
  } else {
    sessions.push(session);
  }
  await set(STORAGE_KEYS.sessions, sessions);
}

export async function getActiveSessionId(): Promise<string | null> {
  return get<string>(STORAGE_KEYS.activeSessionId);
}

export async function setActiveSessionId(sessionId: string): Promise<void> {
  await set(STORAGE_KEYS.activeSessionId, sessionId);
}

export async function getActiveSession(): Promise<CollectionSession | null> {
  const sessionId = await getActiveSessionId();
  if (!sessionId) {
    return null;
  }

  const sessions = await getSessions();
  return sessions.find((item) => item.id === sessionId) ?? null;
}

export async function getTabStatusMap(): Promise<Record<string, TabSiteStatus>> {
  return (await get<Record<string, TabSiteStatus>>(STORAGE_KEYS.tabStatusMap)) ?? {};
}

export async function setTabStatus(tabId: number, status: TabSiteStatus): Promise<void> {
  const map = await getTabStatusMap();
  map[String(tabId)] = status;
  await set(STORAGE_KEYS.tabStatusMap, map);
}

export async function getTabStatus(tabId: number): Promise<TabSiteStatus | null> {
  const map = await getTabStatusMap();
  return map[String(tabId)] ?? null;
}

export async function removeTabStatus(tabId: number): Promise<void> {
  const map = await getTabStatusMap();
  delete map[String(tabId)];
  await set(STORAGE_KEYS.tabStatusMap, map);
}

export async function getAutoSyncMap(): Promise<Record<string, boolean>> {
  return (await get<Record<string, boolean>>(STORAGE_KEYS.autoSyncMap)) ?? {};
}

export async function setAutoSync(siteId: string, enabled: boolean): Promise<void> {
  const map = await getAutoSyncMap();
  map[siteId] = enabled;
  await set(STORAGE_KEYS.autoSyncMap, map);
}

export async function getLastSyncMap(): Promise<Record<string, string>> {
  return (await get<Record<string, string>>(STORAGE_KEYS.lastSyncMap)) ?? {};
}

export async function setLastSync(siteId: string, timestamp: string): Promise<void> {
  const map = await getLastSyncMap();
  map[siteId] = timestamp;
  await set(STORAGE_KEYS.lastSyncMap, map);
}
