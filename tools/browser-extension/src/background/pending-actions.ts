// ---- Pending action polling (T17) ----
// Backend queues "open SiteX tab" actions; extension polls every 30s,
// executes via chrome.tabs.create on KNOWN_SITES only, then acks.
// R-EC10 enforces 24h TTL server-side.
//
// 每轮取回全部未 ack、未过期的动作，不再带 since 游标：原来按 created_at > since 取，
// 处理完一批后把游标推进到最晚的成功动作，较早的动作打开失败时会被越过，之后既没 ack 也不再返回。
// 打开了标签页、ack 却失败的动作记在 openedIds 里，下一轮只补 ack，不重复打开。

export const POLL_MAX_ACTION_AGE_MS = 24 * 60 * 60 * 1000;

export interface BackendPendingAction {
  id: number;
  type: string;
  target_url: string;
  site_name?: string;
  reason?: string;
  created_at?: string;
  expires_at?: string;
}

export function isPendingActionArray(value: unknown): value is BackendPendingAction[] {
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

export interface PendingActionDeps {
  /** 取回全部未 ack、未过期的动作（按创建时间升序） */
  fetchPending(baseUrl: string): Promise<BackendPendingAction[]>;
  ack(baseUrl: string, actionId: number): Promise<void>;
  openTab(url: string): Promise<void>;
  isKnownSiteUrl(url: string): boolean;
  /** 已打开标签页、还没 ack 成功的动作 ID（持久化，扩展 service worker 重启后仍在） */
  loadOpenedIds(): Promise<number[]>;
  saveOpenedIds(ids: number[]): Promise<void>;
  now(): number;
  log: {
    info(message: string, data?: unknown): void;
    warn(message: string, data?: unknown): void;
  };
}

function actionTimestampMs(action: BackendPendingAction, now: number): number {
  if (action.created_at) {
    const t = Date.parse(action.created_at);
    if (!Number.isNaN(t)) return t;
  }
  return now;
}

export interface PendingActionPoller {
  /** 跑一轮；上一轮还没结束时返回同一个 Promise，不并发再跑 */
  poll(baseUrl: string): Promise<void>;
}

export function createPendingActionPoller(deps: PendingActionDeps): PendingActionPoller {
  let inFlight: Promise<void> | null = null;

  async function ackQuietly(baseUrl: string, id: number, what: string): Promise<boolean> {
    try {
      await deps.ack(baseUrl, id);
      return true;
    } catch (error: unknown) {
      deps.log.warn(`ack of ${what} action failed`, { id, error });
      return false;
    }
  }

  async function runOnce(baseUrl: string): Promise<void> {
    const actions = await deps.fetchPending(baseUrl);
    const pendingIds = new Set(actions.map((a) => a.id));
    // 已经不在待处理列表里的（ack 成功或过期）不用再记
    const opened = new Set((await deps.loadOpenedIds()).filter((id) => pendingIds.has(id)));
    const now = deps.now();

    for (const action of actions) {
      if (now - actionTimestampMs(action, now) > POLL_MAX_ACTION_AGE_MS) {
        deps.log.info("pollPendingActions skipping expired action", { id: action.id });
        continue;
      }
      if (action.type !== "open_tab") {
        deps.log.info("pollPendingActions skipping unsupported action type", {
          id: action.id,
          type: action.type,
        });
        await ackQuietly(baseUrl, action.id, "unsupported");
        continue;
      }
      if (!deps.isKnownSiteUrl(action.target_url)) {
        deps.log.warn("pollPendingActions skipping non-known-site URL", { id: action.id });
        await ackQuietly(baseUrl, action.id, "skipped");
        continue;
      }
      if (!opened.has(action.id)) {
        try {
          await deps.openTab(action.target_url);
        } catch (error: unknown) {
          // 不 ack：下一轮还会取回来重试
          deps.log.warn("pollPendingActions open_tab failed", { id: action.id, error });
          continue;
        }
        opened.add(action.id);
        await deps.saveOpenedIds([...opened]);
        deps.log.info("pollPendingActions executed open_tab", { id: action.id });
      }
      if (await ackQuietly(baseUrl, action.id, "open_tab")) {
        opened.delete(action.id);
      }
    }
    await deps.saveOpenedIds([...opened]);
  }

  return {
    poll(baseUrl: string): Promise<void> {
      if (inFlight) {
        return inFlight;
      }
      inFlight = runOnce(baseUrl).finally(() => {
        inFlight = null;
      });
      return inFlight;
    },
  };
}
