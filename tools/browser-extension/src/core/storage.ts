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

/*
 * 同一个 key 的读改写排队依次执行。background 里的多个异步事件（几个标签页同时导航、几个站点同时同步）
 * 原来各自「读整张表 → 改 → 写回」，都读到旧表，后写的把先写的覆盖掉。
 * 只在同一个 JS 上下文里串行：这些 key 只由 background 写，popup 经消息让 background 改。
 */
const updateQueues = new Map<string, Promise<unknown>>();

/**
 * 在队列里读出 key 的当前值交给 fn：返回新值则写入，返回 undefined 表示不改。
 */
export function updateKey<T>(
  key: string,
  fn: (current: T | null) => T | undefined,
): Promise<T | null> {
  const previous = updateQueues.get(key) ?? Promise.resolve();
  const run = previous.then(async () => {
    const current = await get<T>(key);
    const next = fn(current);
    if (next === undefined) {
      return current;
    }
    await set(key, next);
    return next;
  });
  // 一次更新失败不能卡住后面的更新
  const settled = run.catch(() => undefined);
  updateQueues.set(key, settled);
  void settled.then(() => {
    if (updateQueues.get(key) === settled) {
      updateQueues.delete(key);
    }
  });
  return run;
}

export async function getConnection(): Promise<PtToolsConnection> {
  return (await get<PtToolsConnection>(STORAGE_KEYS.connection)) ?? DEFAULT_CONNECTION;
}

export async function setConnection(connection: PtToolsConnection): Promise<void> {
  await updateKey<PtToolsConnection>(STORAGE_KEYS.connection, () => connection);
}

/**
 * 只改连接状态字段，并且只在地址还是 expectedBaseUrl 时改，返回是否改了。
 * 同步、轮询这类网络请求结束后用它：请求期间用户可能在设置页换了地址，
 * 原来请求结束后把请求前读到的整个 connection 写回，把新地址改回旧的。
 */
export async function updateConnection(
  expectedBaseUrl: string,
  patch: Partial<Pick<PtToolsConnection, "connected" | "lastSync">>,
): Promise<boolean> {
  let applied = false;
  await updateKey<PtToolsConnection>(STORAGE_KEYS.connection, (current) => {
    const connection = current ?? DEFAULT_CONNECTION;
    if (connection.baseUrl?.trim() !== expectedBaseUrl.trim()) {
      return undefined;
    }
    applied = true;
    return { ...connection, ...patch };
  });
  return applied;
}

export async function getSessions(): Promise<CollectionSession[]> {
  return (await get<CollectionSession[]>(STORAGE_KEYS.sessions)) ?? [];
}

/** 采集 session 的上限：数量与总字节数。session 里是整页 HTML，chrome.storage.local 的配额有限 */
export const MAX_SESSIONS = 10;
export const MAX_SESSIONS_BYTES = 4 * 1024 * 1024;

/**
 * 按创建时间从新到旧保留，超过数量或总字节数的旧 session 丢掉；keepId（刚保存的那个）一定保留。
 * 原来每个未知域都追加一个永久 session，没有任何上限，配额用满之后所有保存都失败。
 */
function pruneSessions(sessions: CollectionSession[], keepId: string): CollectionSession[] {
  const newestFirst = [...sessions].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  const keep = new Set<string>();
  let bytes = 0;
  const kept = newestFirst.find((item) => item.id === keepId);
  if (kept) {
    keep.add(kept.id);
    bytes += JSON.stringify(kept).length;
  }
  for (const item of newestFirst) {
    if (keep.has(item.id)) {
      continue;
    }
    const size = JSON.stringify(item).length;
    if (keep.size >= MAX_SESSIONS || bytes + size > MAX_SESSIONS_BYTES) {
      continue;
    }
    keep.add(item.id);
    bytes += size;
  }
  return sessions.filter((item) => keep.has(item.id));
}

export async function saveSession(session: CollectionSession): Promise<void> {
  await updateKey<CollectionSession[]>(STORAGE_KEYS.sessions, (current) => {
    const sessions = [...(current ?? [])];
    const index = sessions.findIndex((item) => item.id === session.id);
    if (index >= 0) {
      sessions[index] = session;
    } else {
      sessions.push(session);
    }
    return pruneSessions(sessions, session.id);
  });
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
  await updateKey<Record<string, TabSiteStatus>>(STORAGE_KEYS.tabStatusMap, (map) => ({
    ...map,
    [String(tabId)]: status,
  }));
}

export async function getTabStatus(tabId: number): Promise<TabSiteStatus | null> {
  const map = await getTabStatusMap();
  return map[String(tabId)] ?? null;
}

export async function removeTabStatus(tabId: number): Promise<void> {
  await updateKey<Record<string, TabSiteStatus>>(STORAGE_KEYS.tabStatusMap, (map) => {
    const next = { ...map };
    delete next[String(tabId)];
    return next;
  });
}

export async function getAutoSyncMap(): Promise<Record<string, boolean>> {
  return (await get<Record<string, boolean>>(STORAGE_KEYS.autoSyncMap)) ?? {};
}

export async function setAutoSync(siteId: string, enabled: boolean): Promise<void> {
  await updateKey<Record<string, boolean>>(STORAGE_KEYS.autoSyncMap, (map) => ({
    ...map,
    [siteId]: enabled,
  }));
}

export async function getLastSyncMap(): Promise<Record<string, string>> {
  return (await get<Record<string, string>>(STORAGE_KEYS.lastSyncMap)) ?? {};
}

export async function setLastSync(siteId: string, timestamp: string): Promise<void> {
  await updateKey<Record<string, string>>(STORAGE_KEYS.lastSyncMap, (map) => ({
    ...map,
    [siteId]: timestamp,
  }));
}

export async function getLastVisitMap(): Promise<Record<string, string>> {
  return (await get<Record<string, string>>(STORAGE_KEYS.lastVisitMap)) ?? {};
}

export async function setLastVisit(siteId: string, ts: number | string): Promise<void> {
  const iso = typeof ts === "number" ? new Date(ts).toISOString() : ts;
  await updateKey<Record<string, string>>(STORAGE_KEYS.lastVisitMap, (map) => ({
    ...map,
    [siteId]: iso,
  }));
}
