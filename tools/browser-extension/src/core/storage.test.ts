import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { STORAGE_KEYS } from "./constants";
import { t } from "./i18n";
import {
  OPTIONAL_PAGE_TYPES,
  getConnection,
  getLastSyncMap,
  getSessions,
  isQuotaError,
  saveSession,
  setConnection,
  setLastSync,
  updateConnection,
} from "./storage";
import type { CapturedPage, CollectionSession, PageType } from "./types";

/** chrome.storage.local 的替身：get / set 之间让出事件循环，读改写的竞争才看得出来 */
let area: Record<string, unknown> = {};
/**
 * 模拟配额：写入以后所有键名加值的 JSON 长度超过它就像 Chrome 一样拒绝（整次写入不生效）；null 为不限。
 * 真的 Chrome 按 UTF-8 字节、把 < 记成 6 字节，这里只看长度，测的是超出以后的处理。
 */
let quota: number | null = null;
/** 模拟别的写入错误（不是配额） */
let failWith: Error | null = null;

function usedChars(items: Record<string, unknown>): number {
  return Object.entries(items).reduce(
    (total, [key, value]) => total + key.length + JSON.stringify(value).length,
    0,
  );
}

beforeEach(() => {
  area = {};
  quota = null;
  failWith = null;
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
          if (failWith) throw failWith;
          if (quota !== null && usedChars({ ...area, ...items }) > quota) {
            throw new Error("Resource::kQuotaBytes quota exceeded");
          }
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

function page(pageType: PageType, html: string): CapturedPage {
  return {
    pageType,
    url: `https://pt.example/${pageType}`,
    html,
    capturedAt: "2026-11-22T00:00:00Z",
    detectedSchema: "NexusPHP",
  };
}

/** 一键采集的会话：必采页面小，后台标签页记下的渲染后页面与请求记录大 */
function collected(id: string, opts: { required?: number; optional?: number } = {}) {
  const required = "r".repeat(opts.required ?? 1024);
  const optional = "o".repeat(opts.optional ?? 64 * 1024);
  return {
    id,
    site: { name: id, url: `https://${id}.example`, schema: "NexusPHP", authMethod: "cookie" },
    pages: [
      page("index", required),
      page("search", required),
      page("rendered_search", optional),
      page("requests_search", optional),
      page("detail", required),
      page("rendered_userinfo", optional),
      page("script", required),
    ],
    createdAt: "2026-11-22T00:00:00Z",
    status: "complete",
    facts: { cookieNames: ["c_secure_uid"], warnings: ["⚠ 没有 H&R 页"] },
    identity: { username: "someone", userId: "42" },
  } as CollectionSession;
}

function storedSessions(): CollectionSession[] {
  return (area[STORAGE_KEYS.sessions] as CollectionSession[] | undefined) ?? [];
}

/*
 * 一键采集一次存约 20 个页面，扩展没有 unlimitedStorage，chrome.storage.local 一共 10 MiB：
 * 原来超出配额时 saveSession 直接失败，整次采集都丢了。
 */
describe("saveSession: 超出配额时", () => {
  it("正常大小的会话照原样存下，不去掉页面、没有提示", async () => {
    quota = 10 * 1024 * 1024;
    const session = collected("ok");

    const result = await saveSession(session);

    expect(result.dropped).toEqual([]);
    expect(result.warning).toBeNull();
    expect(result.session).toEqual(session);
    const [stored] = storedSessions();
    expect(stored.pages.map((item) => item.pageType)).toEqual(
      session.pages.map((item) => item.pageType),
    );
    expect(stored.facts?.warnings).toEqual(["⚠ 没有 H&R 页"]);
  });

  it("去掉渲染后页面与请求记录再存一次，提示里写明去掉了哪些", async () => {
    // 必采页面约 5 KB，三个可选页面各 64 KB：整份存不下，去掉可选页面以后存得下
    quota = 32 * 1024;
    const session = collected("big");

    const result = await saveSession(session);

    expect(result.dropped).toEqual(["rendered_search", "requests_search", "rendered_userinfo"]);
    expect(result.warning).toBe(
      t("collector.warnQuotaDropped", "rendered_search, requests_search, rendered_userinfo"),
    );
    const [stored] = storedSessions();
    expect(stored.pages.map((item) => item.pageType)).toEqual([
      "index",
      "search",
      "detail",
      "script",
    ]);
    expect(stored.pages.some((item) => OPTIONAL_PAGE_TYPES.has(item.pageType))).toBe(false);
    // 原来的提示留着，新提示加在后面；站点信息与身份照存
    expect(stored.facts?.warnings).toEqual(["⚠ 没有 H&R 页", result.warning]);
    expect(stored.facts?.cookieNames).toEqual(["c_secure_uid"]);
    expect(stored.identity).toEqual({ username: "someone", userId: "42" });
    expect(result.session).toEqual(stored);
  });

  it("去掉可选页面以后还是存不下才报错，存储里原来的会话不动", async () => {
    await saveSession(collected("old"));
    const before = structuredClone(area);
    // 必采页面本身就放不下
    quota = usedChars(area) + 16 * 1024;

    await expect(saveSession(collected("huge", { required: 64 * 1024 }))).rejects.toThrow(
      t("error.sessionTooLarge"),
    );
    expect(area).toEqual(before);
  });

  it("没有可以去掉的页面时直接报存不下", async () => {
    quota = 1024;
    const session = collected("plain", { required: 4096 });
    session.pages = session.pages.filter((item) => !OPTIONAL_PAGE_TYPES.has(item.pageType));

    await expect(saveSession(session)).rejects.toThrow(t("error.sessionTooLarge"));
    expect(storedSessions()).toEqual([]);
  });

  it("不是配额的错误照原样抛出，不去掉页面重试", async () => {
    failWith = new Error("IO error: disk is busy");

    await expect(saveSession(collected("io"))).rejects.toThrow("IO error: disk is busy");
    expect(chrome.storage.local.set).toHaveBeenCalledTimes(1);
  });

  it("认得出 Chrome 新旧两种配额错误", () => {
    expect(isQuotaError(new Error("Resource::kQuotaBytes quota exceeded"))).toBe(true);
    expect(isQuotaError(new Error("QUOTA_BYTES quota exceeded"))).toBe(true);
    expect(isQuotaError(new Error("IO error"))).toBe(false);
  });
});
