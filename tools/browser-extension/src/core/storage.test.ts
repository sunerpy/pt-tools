import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { STORAGE_KEYS } from "./constants";
import {
  getConnection,
  getLastSyncMap,
  getSessions,
  saveSession,
  setConnection,
  setLastSync,
  updateConnection,
} from "./storage";
import type { CollectionSession } from "./types";

/** chrome.storage.local 的替身：get / set 之间让出事件循环，读改写的竞争才看得出来 */
let area: Record<string, unknown> = {};

beforeEach(() => {
  area = {};
  const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
  vi.stubGlobal("chrome", {
    storage: {
      local: {
        get: vi.fn(async (key: string) => {
          await tick();
          return key in area ? { [key]: structuredClone(area[key]) } : {};
        }),
        set: vi.fn(async (items: Record<string, unknown>) => {
          await tick();
          Object.assign(area, structuredClone(items));
        }),
        remove: vi.fn(async (key: string) => {
          await tick();
          delete area[key];
        }),
      },
    },
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function session(id: string, createdAt: string, html = "<html></html>"): CollectionSession {
  return {
    id,
    site: { name: id, url: `https://${id}.example`, schema: "NexusPHP", authMethod: "cookie" },
    pages: [
      {
        pageType: "index",
        url: `https://${id}.example/`,
        html,
        capturedAt: createdAt,
        detectedSchema: "NexusPHP",
      },
    ],
    createdAt,
    status: "collecting",
  } as CollectionSession;
}

/*
 * 读整张表 → 改 → 写回：两个站点几乎同时同步时都读到旧表，后写的把先写的覆盖掉。
 */
describe("storage: 并发更新同一张表", () => {
  it("两个站点同时记录同步时间，两条都留下", async () => {
    await Promise.all([
      setLastSync("a", "2026-10-06T00:00:00Z"),
      setLastSync("b", "2026-10-06T00:00:01Z"),
    ]);

    expect(await getLastSyncMap()).toEqual({
      a: "2026-10-06T00:00:00Z",
      b: "2026-10-06T00:00:01Z",
    });
  });
});

/*
 * 自动同步在网络请求前读出整个 connection，请求结束后把它写回：期间用户在设置页换了地址，
 * 旧同步完成时把新地址改回旧的。
 */
describe("updateConnection", () => {
  it("地址已经换了：不提交旧同步的结果，也不改回旧地址", async () => {
    await setConnection({
      baseUrl: "http://old:8080",
      sessionId: "",
      connected: false,
      lastSync: null,
    });
    await setConnection({
      baseUrl: "http://new:8080",
      sessionId: "",
      connected: false,
      lastSync: null,
    });

    const applied = await updateConnection("http://old:8080", { connected: true, lastSync: "t1" });

    expect(applied).toBe(false);
    expect(await getConnection()).toMatchObject({
      baseUrl: "http://new:8080",
      connected: false,
      lastSync: null,
    });
  });

  it("地址没变：只更新连接状态字段", async () => {
    await setConnection({
      baseUrl: "http://pt:8080",
      sessionId: "s",
      connected: false,
      lastSync: null,
    });

    expect(await updateConnection("http://pt:8080", { connected: true, lastSync: "t2" })).toBe(
      true,
    );
    expect(await getConnection()).toEqual({
      baseUrl: "http://pt:8080",
      sessionId: "s",
      connected: true,
      lastSync: "t2",
    });
  });
});

/*
 * 每个未知域都追加一个永久 session，内容是完整页面 HTML，没有任何上限：
 * 适配几个大站之后 chrome.storage.local 配额用满，之后所有保存都失败。
 */
describe("saveSession: 限量", () => {
  it("最多保留最近的 10 个 session", async () => {
    for (let i = 0; i < 12; i++) {
      await saveSession(session(`s${i}`, `2026-10-06T00:00:${String(i).padStart(2, "0")}Z`));
    }

    const ids = (await getSessions()).map((s) => s.id);
    expect(ids).toHaveLength(10);
    expect(ids).not.toContain("s0");
    expect(ids).not.toContain("s1");
    expect(ids).toContain("s11");
  });

  it("总大小超限时丢掉最旧的，刚保存的那个一定保留", async () => {
    const big = "x".repeat(3 * 1024 * 1024);
    await saveSession(session("old", "2026-10-06T00:00:00Z", big));
    await saveSession(session("new", "2026-10-06T00:00:01Z", big));

    const ids = (await getSessions()).map((s) => s.id);
    expect(ids).toEqual(["new"]);
    expect(area[STORAGE_KEYS.sessions]).toBeDefined();
  });
});
