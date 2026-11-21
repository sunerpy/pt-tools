import { afterEach, describe, expect, it } from "vitest";

import {
  RECORDER_GLOBAL,
  RECORDER_MAX_BODY,
  RECORDER_MAX_ENTRIES,
  RECORDER_MAX_PARAMS,
  RENDERED_DOM_MAX_CHARS,
} from "../../recorder/constants";
import {
  clickSeedingToggle,
  matchPatternFor,
  normalizeRequests,
  probeRecorder,
  readDom,
  readRecorder,
  recordTabsWith,
  type TabRecorderApi,
} from "./tab-recorder";

interface FakeTab {
  id: number;
  url: string;
  probes: number;
}

/** 假的 chrome API：页面第二次探测时加载完、请求静下来；记录注册、开关标签页与执行的脚本 */
function fakeApi(options: { failExecute?: string; finalUrl?: (url: string) => string } = {}) {
  const log: string[] = [];
  const tabs = new Map<number, FakeTab>();
  let nextTab = 1;
  let clock = 0;
  let registered = false;
  const api: TabRecorderApi = {
    async registerRecorder(matches) {
      log.push(`register ${matches.join(",")}`);
      registered = true;
    },
    async unregisterRecorder() {
      log.push("unregister");
      registered = false;
    },
    async isRecorderRegistered() {
      return registered;
    },
    async executeScript(tabId, world, func) {
      const tab = tabs.get(tabId);
      if (!tab) throw new Error("no tab");
      if (options.failExecute && tab.url.includes(options.failExecute)) {
        throw new Error("page crashed");
      }
      log.push(`exec ${tabId} ${world} ${func.name}`);
      if (func === probeRecorder) {
        tab.probes += 1;
        const settled = tab.probes >= 2;
        return {
          readyState: settled ? "complete" : "loading",
          href: options.finalUrl ? options.finalUrl(tab.url) : tab.url,
          installed: true,
          inflight: settled ? 0 : 1,
          idleMs: settled ? 5000 : 0,
          sinceLoadMs: settled ? 5000 : 0,
        };
      }
      if (func === clickSeedingToggle) {
        return { clicked: true, text: "当前做种", code: "getusertorrentlist('seeding')", at: 10 };
      }
      if (func === readRecorder) {
        return [
          {
            kind: "xhr",
            method: "post",
            url: `${new URL(tab.url).origin}/ajax.php`,
            status: 200,
            startedAt: 11,
          },
          { kind: "evil", method: 42, url: 7 },
        ];
      }
      if (func === readDom) return `<html>${tab.url}</html>`;
      return undefined;
    },
    async createTab(url) {
      const id = nextTab++;
      tabs.set(id, { id, url, probes: 0 });
      log.push(`open ${id} ${url}`);
      return id;
    },
    async removeTab(tabId) {
      tabs.delete(tabId);
      log.push(`close ${tabId}`);
    },
    now: () => clock,
    async sleep(ms) {
      clock += ms;
      log.push(`sleep ${ms}`);
    },
  };
  return {
    api,
    log,
    tabs,
    isRegistered: () => registered,
    setRegistered: (v: boolean) => (registered = v),
  };
}

const ORIGIN = "https://pt.example";
const JOBS = [
  { key: "search" as const, url: `${ORIGIN}/torrents.php` },
  { key: "detail" as const, url: `${ORIGIN}/details.php?id=1` },
  { key: "userinfo" as const, url: `${ORIGIN}/userdetails.php?id=9`, clickSeeding: true },
];

describe("recordTabsWith", () => {
  it("开页前注册记录器，依次开页、等静、读记录和 DOM、关页，最后注销", async () => {
    const fake = fakeApi();
    const recordings = await recordTabsWith(fake.api, ORIGIN, JOBS, {
      throttleMs: 1500,
      pollMs: 300,
      quietMs: 1000,
    });

    expect(fake.log[0]).toBe("register https://pt.example/*");
    expect(fake.log.at(-1)).toBe("unregister");
    expect(fake.isRegistered()).toBe(false);
    expect(fake.tabs.size).toBe(0);
    expect(fake.log.filter((line) => line.startsWith("open"))).toHaveLength(3);
    expect(fake.log.filter((line) => line.startsWith("close"))).toHaveLength(3);
    // 标签页之间照 THROTTLE 间隔，一次只开一个
    expect(fake.log.filter((line) => line === "sleep 1500")).toHaveLength(2);
    expect(fake.log.indexOf("close 1")).toBeLessThan(
      fake.log.indexOf("open 2 https://pt.example/details.php?id=1"),
    );

    expect(recordings.map((r) => r.key)).toEqual(["search", "detail", "userinfo"]);
    expect(recordings[0].click).toBeNull();
    expect(recordings[2].click).toMatchObject({ clicked: true });
    // 点了以后至少等一个静默期再看请求有没有静下来
    expect(fake.log.indexOf("sleep 1000")).toBeGreaterThan(
      fake.log.indexOf("exec 3 MAIN clickSeedingToggle"),
    );
    // 只在个人页点「当前做种」
    expect(fake.log.filter((line) => line.endsWith("clickSeedingToggle"))).toEqual([
      "exec 3 MAIN clickSeedingToggle",
    ]);
    expect(recordings[0].html).toBe("<html>https://pt.example/torrents.php</html>");
    // 页面里读回来的记录逐项校验：坏的那条丢掉，方法统一成大写
    expect(recordings[0].requests).toEqual([
      expect.objectContaining({
        kind: "xhr",
        method: "POST",
        url: `${ORIGIN}/ajax.php`,
        status: 200,
      }),
    ]);
  });

  it("某个页面出错：记下错误，标签页照样关，记录器照样注销，后面的页面照开", async () => {
    const fake = fakeApi({ failExecute: "/details.php?id=1" });
    const recordings = await recordTabsWith(fake.api, ORIGIN, JOBS, {
      throttleMs: 0,
      pollMs: 300,
      pageTimeoutMs: 600,
    });
    expect(recordings[1].timedOut).toBe(true);
    expect(recordings[1].error).toBe("page crashed");
    expect(recordings[2].html).toContain("userdetails.php");
    expect(fake.tabs.size).toBe(0);
    expect(fake.isRegistered()).toBe(false);
  });

  // 站点迟迟不回页面时，chrome.scripting.executeScript 会一直等到文档出来（本机无头 Chrome 实测：页面 25 秒才回，调用就等 25 秒）
  it("页面一直不出来、往页面里执行脚本一直不返回时，也按上限收尾", async () => {
    const fake = fakeApi();
    fake.api.executeScript = () => new Promise<unknown>(() => undefined);
    const started = Date.now();
    const recordings = await recordTabsWith(fake.api, ORIGIN, JOBS, {
      throttleMs: 0,
      pollMs: 10,
      pageTimeoutMs: 60,
      clickTimeoutMs: 60,
      scriptTimeoutMs: 30,
    });
    expect(Date.now() - started).toBeLessThan(3000);
    expect(recordings.map((r) => r.timedOut)).toEqual([true, true, true]);
    expect(recordings.every((r) => r.html === null && r.requests.length === 0)).toBe(true);
    // 什么都没读到时记下原因，一键采集据此写提示
    expect(recordings.map((r) => r.error)).toEqual(Array(3).fill("page did not load in time"));
    expect(fake.tabs.size).toBe(0);
    expect(fake.isRegistered()).toBe(false);
  });

  it("跳到别的网站时不读那边的页面", async () => {
    const fake = fakeApi({ finalUrl: () => "https://login.other.example/" });
    const [recording] = await recordTabsWith(fake.api, ORIGIN, [JOBS[0]], { throttleMs: 0 });
    expect(recording.error).toBe("left the site");
    expect(recording.html).toBeNull();
    expect(fake.log.some((line) => line.endsWith("readDom"))).toBe(false);
    expect(fake.tabs.size).toBe(0);
  });

  it("先清掉上次留下的注册", async () => {
    const fake = fakeApi();
    fake.setRegistered(true);
    await recordTabsWith(fake.api, ORIGIN, [JOBS[0]], { throttleMs: 0 });
    expect(fake.log.slice(0, 2)).toEqual(["unregister", "register https://pt.example/*"]);
  });

  it("注册失败时不开标签页", async () => {
    const fake = fakeApi();
    fake.api.registerRecorder = async () => {
      throw new Error("no permission");
    };
    await expect(recordTabsWith(fake.api, ORIGIN, JOBS, { throttleMs: 0 })).rejects.toThrow(
      "no permission",
    );
    expect(fake.log.some((line) => line.startsWith("open"))).toBe(false);
  });

  it("匹配模式不带端口", () => {
    expect(matchPatternFor("http://127.0.0.1:18766")).toBe("http://127.0.0.1/*");
  });
});

describe("normalizeRequests", () => {
  it("不是数组时为空", () => {
    expect(normalizeRequests({ evil: true })).toEqual([]);
  });

  it("条数、参数与回应正文按上限截断（存进 chrome.storage.local，一共只有 10 MiB）", () => {
    const raw = Array.from({ length: RECORDER_MAX_ENTRIES + 10 }, (_, index) => ({
      kind: "xhr",
      method: "POST",
      url: `https://pt.example/ajax.php?n=${index}`,
      status: 200,
      requestBody: "p".repeat(RECORDER_MAX_PARAMS * 3),
      responseBody: "r".repeat(RECORDER_MAX_BODY * 4),
      startedAt: index,
    }));
    const out = normalizeRequests(raw);
    expect(out).toHaveLength(RECORDER_MAX_ENTRIES);
    expect(out.every((entry) => entry.requestBody?.length === RECORDER_MAX_PARAMS)).toBe(true);
    expect(out.every((entry) => entry.responseBody?.length === RECORDER_MAX_BODY)).toBe(true);
  });

  it("没超上限的照原样留着", () => {
    const [entry] = normalizeRequests([
      {
        kind: "fetch",
        method: "get",
        url: "https://pt.example/ajax.php",
        status: 200,
        requestBody: "a=1",
        responseBody: '{"status":1}',
        startedAt: 5,
      },
    ]);
    expect(entry).toMatchObject({
      kind: "fetch",
      method: "GET",
      requestBody: "a=1",
      responseBody: '{"status":1}',
    });
  });
});

describe("渲染后页面的上限", () => {
  it("渲染后的 DOM 超过上限时截断，没超过的照原样", async () => {
    const fake = fakeApi();
    const original = fake.api.executeScript.bind(fake.api);
    fake.api.executeScript = async (tabId, world, func, args) => {
      if (func === readDom) {
        const tab = fake.tabs.get(tabId);
        return tab?.url.includes("torrents.php")
          ? `<html>${"x".repeat(RENDERED_DOM_MAX_CHARS * 2)}</html>`
          : "<html>small</html>";
      }
      return original(tabId, world, func, args);
    };
    const recordings = await recordTabsWith(fake.api, ORIGIN, JOBS, { throttleMs: 0 });
    expect(recordings[0].html).toHaveLength(RENDERED_DOM_MAX_CHARS);
    expect(recordings[1].html).toBe("<html>small</html>");
  });
});

/*
 * clickSeedingToggle 在页面里跑：这里用假的 document 验证它挑哪个元素。
 */
interface FakeElement {
  attrs: Record<string, string>;
  textContent: string;
  id: string;
  rowHead?: string;
  clicks: number;
  getAttribute(name: string): string | null;
  closest(selector: string): unknown;
  click(): void;
}

function element(attrs: Record<string, string>, text: string, rowHead?: string): FakeElement {
  return {
    attrs,
    textContent: text,
    id: attrs.id ?? "",
    rowHead,
    clicks: 0,
    getAttribute(name) {
      return this.attrs[name] ?? null;
    },
    closest() {
      const head = this.rowHead;
      return head === undefined ? null : { querySelector: () => ({ textContent: head }) };
    },
    click() {
      this.clicks += 1;
    },
  };
}

function withDocument(elements: FakeElement[]): void {
  (globalThis as unknown as { document: unknown }).document = {
    querySelectorAll: () => elements,
  };
}

afterEach(() => {
  delete (globalThis as unknown as { document?: unknown }).document;
});

describe("clickSeedingToggle", () => {
  it("上游 NexusPHP：点「当前做种」那一行的展开链接，认领按钮不点", () => {
    const uploaded = element(
      { href: "javascript: getusertorrentlistajax('9', 'uploaded', 'ka'); klappe_news('a')" },
      " 显示/隐藏",
      "发布种子",
    );
    const seeding = element(
      { href: "javascript: getusertorrentlistajax('9', 'seeding', 'ka1'); klappe_news('a1')" },
      " 显示/隐藏",
      "当前做种",
    );
    const claim = element({ id: "claim-all-seeding", onclick: "claimAll()" }, "认领全部做种");
    withDocument([uploaded, claim, seeding]);
    const result = clickSeedingToggle();
    expect(result).toMatchObject({ clicked: true, text: "显示/隐藏" });
    expect(seeding.clicks).toBe(1);
    expect(uploaded.clicks + claim.clicks).toBe(0);
  });

  it("HDWinG 那样文字是「当前做种」、onclick 调 getusertorrentlist 的", () => {
    const seeding = element(
      { href: "javascript:void(0)", onclick: "getusertorrentlist('seeding','ka1')" },
      "当前做种",
    );
    withDocument([seeding]);
    expect(clickSeedingToggle().clicked).toBe(true);
    expect(seeding.clicks).toBe(1);
  });

  it("繁体与英文的文字、href 是 javascript: 的也点", () => {
    for (const text of ["當前做種", "Seeding"]) {
      const el = element({ href: "javascript:void(0)" }, text);
      withDocument([el]);
      expect(clickSeedingToggle().clicked).toBe(true);
      expect(el.clicks).toBe(1);
    }
  });

  it("签到入口写着 Seeding 也不点；普通链接与别的 onclick 不点", () => {
    const sign = element({ href: "javascript:void(0)", onclick: "doSign()" }, "Seeding");
    const plain = element({ href: "seeding.php" }, "当前做种");
    const other = element({ href: "#", onclick: "showSeeding()" }, "当前做种");
    withDocument([sign, plain, other]);
    expect(clickSeedingToggle()).toEqual({ clicked: false });
    expect(sign.clicks + plain.clicks + other.clicks).toBe(0);
  });

  it("只点一个", () => {
    const a = element(
      { href: "javascript: getusertorrentlistajax('9', 'seeding', 'ka1')" },
      "显示",
    );
    const b = element({ href: "javascript:void(0)" }, "当前做种");
    withDocument([b, a]);
    clickSeedingToggle();
    expect(a.clicks + b.clicks).toBe(1);
    expect(a.clicks).toBe(1);
  });
});

describe("probeRecorder / readRecorder", () => {
  it("读页面里挂的记录器", () => {
    const g = globalThis as unknown as Record<string, unknown>;
    g[RECORDER_GLOBAL] = {
      read: () => [{ url: "x" }],
      status: () => ({ inflight: 2, lastActivity: Date.now() }),
    };
    g.window = globalThis;
    g.document = { readyState: "complete" };
    g.location = { href: "https://pt.example/a" };
    try {
      expect(readRecorder(RECORDER_GLOBAL)).toEqual([{ url: "x" }]);
      expect(probeRecorder(RECORDER_GLOBAL)).toMatchObject({
        readyState: "complete",
        href: "https://pt.example/a",
        installed: true,
        inflight: 2,
      });
    } finally {
      delete g[RECORDER_GLOBAL];
      delete g.location;
      delete g.window;
    }
  });
});
