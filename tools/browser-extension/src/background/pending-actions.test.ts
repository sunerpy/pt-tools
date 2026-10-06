import { describe, expect, it, vi } from "vitest";

import {
  type BackendPendingAction,
  createPendingActionPoller,
  type PendingActionDeps,
} from "./pending-actions";

const NOW = Date.parse("2026-10-06T12:00:00Z");

function action(
  id: number,
  minutesAgo: number,
  url = `https://hdsky.me/details.php?id=${id}`,
): BackendPendingAction {
  return {
    id,
    type: "open_tab",
    target_url: url,
    created_at: new Date(NOW - minutesAgo * 60_000).toISOString(),
  };
}

/** 模拟后端：返回所有未 ack 的动作；ack 之后不再返回 */
function fakeDeps(actions: BackendPendingAction[], overrides: Partial<PendingActionDeps> = {}) {
  const acked = new Set<number>();
  let opened: number[] = [];
  const store: Record<string, unknown> = {};
  const deps: PendingActionDeps = {
    fetchPending: vi.fn(async () => actions.filter((a) => !acked.has(a.id))),
    ack: vi.fn(async (_baseUrl: string, id: number) => {
      acked.add(id);
    }),
    openTab: vi.fn(async (url: string) => {
      opened.push(Number(new URL(url).searchParams.get("id")));
    }),
    isKnownSiteUrl: (url: string) => url.startsWith("https://hdsky.me/"),
    loadOpenedIds: async () => (store.opened as number[] | undefined) ?? [],
    saveOpenedIds: async (ids: number[]) => {
      store.opened = ids;
    },
    now: () => NOW,
    log: { info: vi.fn(), warn: vi.fn() },
    ...overrides,
  };
  return {
    deps,
    acked,
    opened: () => opened,
    resetOpened: () => {
      opened = [];
    },
    store,
  };
}

/*
 * 原来按 created_at > since 取动作，处理完一批后把游标推进到最晚的成功动作：
 * 较早的动作打开失败时会被越过，之后既没 ack，也不再返回，永久丢失。
 */
describe("pending action poller", () => {
  it("较早的动作打开失败，下一轮还会重试，不会被较晚的成功动作越过", async () => {
    const f = fakeDeps([action(1, 10), action(2, 5)]);
    let failFirst = true;
    f.deps.openTab = vi.fn(async (url: string) => {
      const id = Number(new URL(url).searchParams.get("id"));
      if (id === 1 && failFirst) {
        failFirst = false;
        throw new Error("tabs.create failed");
      }
    });
    const poller = createPendingActionPoller(f.deps);

    await poller.poll("http://pt-tools.local");
    expect([...f.acked]).toEqual([2]);

    await poller.poll("http://pt-tools.local");
    expect([...f.acked].sort()).toEqual([1, 2]);
  });

  it("标签页已打开但 ack 失败：下一轮只补 ack，不再重复打开", async () => {
    const f = fakeDeps([action(7, 1)]);
    let ackFails = true;
    f.deps.ack = vi.fn(async (_baseUrl: string, id: number) => {
      if (ackFails) {
        ackFails = false;
        throw new Error("ack HTTP 502");
      }
      f.acked.add(id);
    });
    const poller = createPendingActionPoller(f.deps);

    await poller.poll("http://pt-tools.local");
    await poller.poll("http://pt-tools.local");

    expect(f.opened()).toEqual([7]);
    expect([...f.acked]).toEqual([7]);
    expect(f.store.opened).toEqual([]);
  });

  it("上一轮没结束时不会并发再跑一轮", async () => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const f = fakeDeps([action(3, 1)]);
    f.deps.fetchPending = vi.fn(async () => {
      await gate;
      return [action(3, 1)];
    });
    const poller = createPendingActionPoller(f.deps);

    const first = poller.poll("http://pt-tools.local");
    const second = poller.poll("http://pt-tools.local");
    release();
    await Promise.all([first, second]);

    expect(f.deps.fetchPending).toHaveBeenCalledTimes(1);
    expect(f.opened()).toEqual([3]);
  });

  it("非已知站点的地址不打开，直接 ack", async () => {
    const f = fakeDeps([action(9, 1, "https://evil.example/x?id=9")]);
    const poller = createPendingActionPoller(f.deps);

    await poller.poll("http://pt-tools.local");

    expect(f.opened()).toEqual([]);
    expect([...f.acked]).toEqual([9]);
  });
});
