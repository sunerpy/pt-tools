import {
  RECORDER_FILE,
  RECORDER_GLOBAL,
  RECORDER_MAX_BODY,
  RECORDER_MAX_ENTRIES,
  RECORDER_MAX_PARAMS,
  RECORDER_SCRIPT_ID,
  RENDERED_DOM_MAX_CHARS,
} from "../../recorder/constants";

/*
 * 页面脚本另取的内容（HDWinG 的优惠状态、做种列表）只在浏览器里跑页面脚本才看得到：
 * 一键采集在后台（不激活）打开列表页、一个详情页和个人页，开页前用 registerContentScripts 在页面脚本之前装上请求记录器，
 * 等页面加载完、请求静下来，读走记录与渲染后的 DOM，关掉标签页；全部开完注销记录器（出错也收尾）。
 * 个人页上只点「当前做种」那一个展开按钮，别的一概不点，签到更不点。
 */

export interface RecordedRequest {
  kind: "xhr" | "fetch";
  method: string;
  url: string;
  status?: number;
  contentType?: string | null;
  requestBody?: string;
  responseBody?: string;
  error?: string;
  startedAt: number;
  endedAt?: number;
}

export interface ClickResult {
  clicked: boolean;
  text?: string;
  code?: string;
  /** 页面里点击的时间（和记录里的 startedAt 同一个时钟） */
  at?: number;
}

export type TabKey = "search" | "detail" | "userinfo";

export interface TabJob {
  key: TabKey;
  url: string;
  /** 个人页：点一下「当前做种」，记下它触发的请求 */
  clickSeeding?: boolean;
}

export interface TabRecording {
  key: TabKey;
  url: string;
  finalUrl: string;
  /** 页面脚本跑完以后的 DOM（未脱敏，调用方负责脱敏） */
  html: string | null;
  requests: RecordedRequest[];
  click: ClickResult | null;
  error?: string;
  /** 总时长到了上限还没静下来 */
  timedOut?: boolean;
}

export type RecordTabs = (origin: string, jobs: TabJob[]) => Promise<TabRecording[]>;

/** 后台标签页用到的 chrome API；测试换成假的 */
export interface TabRecorderApi {
  registerRecorder(matches: string[]): Promise<void>;
  unregisterRecorder(): Promise<void>;
  isRecorderRegistered(): Promise<boolean>;
  executeScript(
    tabId: number,
    world: "MAIN" | "ISOLATED",
    func: (...args: never[]) => unknown,
    args?: unknown[],
  ): Promise<unknown>;
  createTab(url: string): Promise<number>;
  removeTab(tabId: number): Promise<void>;
  now(): number;
  sleep(ms: number): Promise<void>;
}

export interface TabRecorderOptions {
  /** 两个标签页之间的间隔（同 THROTTLE_MS） */
  throttleMs: number;
  /** 一个页面从打开到静下来的总时长上限 */
  pageTimeoutMs?: number;
  /** 点了「当前做种」以后等请求静下来的上限 */
  clickTimeoutMs?: number;
  /** 没有新请求、也没有请求在途多久算静下来 */
  quietMs?: number;
  pollMs?: number;
  /** 点「当前做种」、读记录、读 DOM 时，往页面里执行一次脚本最多等多久 */
  scriptTimeoutMs?: number;
}

const DEFAULT_PAGE_TIMEOUT_MS = 20000;
const DEFAULT_CLICK_TIMEOUT_MS = 10000;
const DEFAULT_QUIET_MS = 1500;
const DEFAULT_POLL_MS = 300;
const DEFAULT_SCRIPT_TIMEOUT_MS = 5000;

/**
 * 最多等 ms 毫秒，到时回 undefined。页面还没开始返回（站点慢、卡住）时 executeScript 会一直等到文档出来，
 * 不加上限，一个页面就能让整次采集挂住、后台标签页一直开着。
 */
function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T | undefined> {
  return new Promise<T | undefined>((resolve, reject) => {
    const timer = setTimeout(() => resolve(undefined), Math.max(0, ms));
    promise.then(
      (value) => {
        clearTimeout(timer);
        resolve(value);
      },
      (error: unknown) => {
        clearTimeout(timer);
        reject(error);
      },
    );
  });
}

/** 站点的匹配模式：不带端口（Chrome 的匹配模式省略端口即匹配所有端口） */
export function matchPatternFor(origin: string): string {
  const url = new URL(origin);
  return `${url.protocol}//${url.hostname}/*`;
}

interface ProbeResult {
  readyState: string;
  href: string;
  installed: boolean;
  inflight: number;
  idleMs: number;
  sinceLoadMs: number;
}

/** 在页面里（MAIN world）看加载到哪了、记录器有没有请求在途。必须自包含：会被序列化进页面执行 */
export function probeRecorder(key: string): ProbeResult {
  const recorder = (window as unknown as Record<string, unknown>)[key] as
    | { status?: () => { inflight: number; lastActivity: number } }
    | undefined;
  const status = recorder && typeof recorder.status === "function" ? recorder.status() : null;
  const nav = performance.getEntriesByType("navigation")[0] as
    | PerformanceNavigationTiming
    | undefined;
  const loadEnd = nav && nav.loadEventEnd > 0 ? nav.loadEventEnd : 0;
  return {
    readyState: document.readyState,
    href: location.href,
    installed: Boolean(status),
    inflight: status ? status.inflight : 0,
    idleMs: status ? Date.now() - status.lastActivity : 0,
    sinceLoadMs: loadEnd > 0 ? performance.now() - loadEnd : 0,
  };
}

/** 读走记录器的记录（MAIN world）。自包含 */
export function readRecorder(key: string): unknown {
  const recorder = (window as unknown as Record<string, unknown>)[key] as
    | { read?: () => unknown }
    | undefined;
  return recorder && typeof recorder.read === "function" ? recorder.read() : [];
}

/** 读渲染后的 DOM（ISOLATED world，DOM 是同一个）。自包含 */
export function readDom(): string | null {
  return document.documentElement ? document.documentElement.outerHTML : null;
}

/**
 * 个人页上点一下「当前做种」的展开按钮（MAIN world，和页面自己的点击一样）。自包含。
 * 只认这几种元素，按优先级取一个：
 *   3. href（javascript:）或 onclick 里调 getusertorrentlist… 且参数是 seeding（上游 NexusPHP、HDWinG）；
 *   2. 文字是「当前做种 / 當前做種 / Seeding」，并且 href 是 javascript: 或 onclick 里有 getusertorrentlist；
 *   1. 在「当前做种」那一行里、调 getusertorrentlist 的元素。
 * 属性、文字里有签到、认领、删除、退出这类字样的一律不点。只点一个，找不到就不点。
 */
export function clickSeedingToggle(): ClickResult {
  const deny =
    /签到|簽到|attendance|sign|qiandao|check-?in|打卡|claim|认领|認領|delete|删除|刪除|remove|logout|退出|vote|thank|bookmark|收藏|buy|兑换|兌換|invite|邀请|report|举报/i;
  const norm = (value: string | null | undefined): string =>
    (value ?? "").replace(/[\s :：[\]【】()（）]+/g, "").toLowerCase();
  const isLabel = (value: string): boolean =>
    value === "当前做种" || value === "當前做種" || value === "seeding";

  let best: { el: HTMLElement; score: number; code: string } | null = null;
  for (const el of Array.from(document.querySelectorAll<HTMLElement>("a[href], [onclick]"))) {
    const href = el.getAttribute("href") ?? "";
    const onclick = el.getAttribute("onclick") ?? "";
    const jsHref = /^\s*javascript:/i.test(href);
    const code = `${jsHref ? href : ""} ${onclick}`.trim();
    const text = norm(el.textContent);
    if (text.length > 40) continue;
    if (deny.test(`${href} ${onclick} ${text} ${el.id} ${el.getAttribute("class") ?? ""}`)) {
      continue;
    }
    const callsList = /getusertorrentlist/i.test(code);
    const row = el.closest("tr");
    const rowHead = row ? norm(row.querySelector("td, th")?.textContent) : "";
    let score = 0;
    if (callsList && /seeding/i.test(code)) {
      score = 3;
    } else if (isLabel(text) && (jsHref || /getusertorrentlist/i.test(onclick))) {
      score = 2;
    } else if (callsList && isLabel(rowHead)) {
      score = 1;
    }
    if (score > (best?.score ?? 0)) {
      best = { el, score, code };
    }
  }
  if (!best) {
    return { clicked: false };
  }
  const at = Date.now();
  best.el.click();
  return {
    clicked: true,
    text: (best.el.textContent ?? "").trim().slice(0, 40),
    code: best.code.slice(0, 200),
    at,
  };
}

function str(value: unknown, max: number): string | undefined {
  return typeof value === "string" ? value.slice(0, max) : undefined;
}

/** 页面里读回来的记录可能被页面改过：逐项校验类型、按 recorder/constants.ts 的上限截断（页面里的记录器也按它截） */
export function normalizeRequests(raw: unknown): RecordedRequest[] {
  if (!Array.isArray(raw)) return [];
  const out: RecordedRequest[] = [];
  for (const item of raw.slice(0, RECORDER_MAX_ENTRIES)) {
    if (typeof item !== "object" || item === null) continue;
    const entry = item as Record<string, unknown>;
    const url = str(entry.url, 2000);
    if (!url) continue;
    out.push({
      kind: entry.kind === "fetch" ? "fetch" : "xhr",
      method: (str(entry.method, 10) ?? "GET").toUpperCase(),
      url,
      status: typeof entry.status === "number" ? entry.status : undefined,
      contentType: str(entry.contentType, 200) ?? null,
      requestBody: str(entry.requestBody, RECORDER_MAX_PARAMS),
      responseBody: str(entry.responseBody, RECORDER_MAX_BODY),
      error: str(entry.error, 300),
      startedAt: typeof entry.startedAt === "number" ? entry.startedAt : 0,
      endedAt: typeof entry.endedAt === "number" ? entry.endedAt : undefined,
    });
  }
  return out;
}

async function waitForQuiet(
  tabId: number,
  api: TabRecorderApi,
  timeoutMs: number,
  quietMs: number,
  pollMs: number,
): Promise<{ ok: boolean; href?: string }> {
  const deadline = api.now() + timeoutMs;
  let href: string | undefined;
  while (api.now() < deadline) {
    try {
      const probe = (await withTimeout(
        api.executeScript(tabId, "MAIN", probeRecorder, [RECORDER_GLOBAL]),
        deadline - api.now(),
      )) as ProbeResult | undefined;
      if (probe) {
        href = probe.href;
        const loaded = probe.readyState === "complete" && probe.sinceLoadMs >= quietMs;
        if (loaded && probe.inflight === 0 && (!probe.installed || probe.idleMs >= quietMs)) {
          return { ok: true, href };
        }
      }
    } catch {
      // 页面还没开始加载或正在跳转：再等
    }
    await api.sleep(pollMs);
  }
  return { ok: false, href };
}

function sameOrigin(url: string, origin: string): boolean {
  try {
    return new URL(url).origin === origin;
  } catch {
    return false;
  }
}

async function recordOne(
  job: TabJob,
  origin: string,
  api: TabRecorderApi,
  options: Required<TabRecorderOptions>,
): Promise<TabRecording> {
  const recording: TabRecording = {
    key: job.key,
    url: job.url,
    finalUrl: job.url,
    html: null,
    requests: [],
    click: null,
  };
  let tabId: number | null = null;
  try {
    tabId = await api.createTab(job.url);
    const loaded = await waitForQuiet(
      tabId,
      api,
      options.pageTimeoutMs,
      options.quietMs,
      options.pollMs,
    );
    if (!loaded.ok) recording.timedOut = true;
    if (loaded.href) recording.finalUrl = loaded.href;
    if (!sameOrigin(recording.finalUrl, origin)) {
      // 跳到了别的网站（比如人机验证、登录中心）：不读那边的页面
      recording.error = "left the site";
      return recording;
    }
    if (job.clickSeeding) {
      const click = (await withTimeout(
        api.executeScript(tabId, "MAIN", clickSeedingToggle),
        options.scriptTimeoutMs,
      )) as ClickResult | undefined;
      recording.click = click && typeof click.clicked === "boolean" ? click : { clicked: false };
      if (recording.click.clicked) {
        // 页面可能过一会儿才发请求：先等一个静默期，再等请求静下来
        await api.sleep(options.quietMs);
        const settled = await waitForQuiet(
          tabId,
          api,
          options.clickTimeoutMs,
          options.quietMs,
          options.pollMs,
        );
        if (!settled.ok) recording.timedOut = true;
      }
    }
    recording.requests = normalizeRequests(
      await withTimeout(
        api.executeScript(tabId, "MAIN", readRecorder, [RECORDER_GLOBAL]),
        options.scriptTimeoutMs,
      ),
    );
    const html = await withTimeout(
      api.executeScript(tabId, "ISOLATED", readDom),
      options.scriptTimeoutMs,
    );
    recording.html = typeof html === "string" ? html.slice(0, RENDERED_DOM_MAX_CHARS) : null;
    if (recording.timedOut && recording.html === null && recording.requests.length === 0) {
      // 到了上限页面还没出来：什么都没读到，记下原因，调用方据此提示
      recording.error = "page did not load in time";
    }
  } catch (error: unknown) {
    recording.error = error instanceof Error ? error.message : String(error);
  } finally {
    if (tabId !== null) {
      await api.removeTab(tabId).catch(() => undefined);
    }
  }
  return recording;
}

/**
 * 依次在后台标签页里打开这些页面并记录（一次只开一个），返回每个页面的记录。
 * 记录器只在这期间注册着：开第一个页面前注册，最后一个页面关掉以后注销，中途出错也注销。
 */
export async function recordTabsWith(
  api: TabRecorderApi,
  origin: string,
  jobs: TabJob[],
  options: TabRecorderOptions,
): Promise<TabRecording[]> {
  const resolved: Required<TabRecorderOptions> = {
    throttleMs: options.throttleMs,
    pageTimeoutMs: options.pageTimeoutMs ?? DEFAULT_PAGE_TIMEOUT_MS,
    clickTimeoutMs: options.clickTimeoutMs ?? DEFAULT_CLICK_TIMEOUT_MS,
    quietMs: options.quietMs ?? DEFAULT_QUIET_MS,
    pollMs: options.pollMs ?? DEFAULT_POLL_MS,
    scriptTimeoutMs: options.scriptTimeoutMs ?? DEFAULT_SCRIPT_TIMEOUT_MS,
  };
  if (jobs.length === 0) return [];
  // 上次采集中途 service worker 被停掉时留下的注册：先清掉
  if (await api.isRecorderRegistered().catch(() => false)) {
    await api.unregisterRecorder().catch(() => undefined);
  }
  await api.registerRecorder([matchPatternFor(origin)]);
  const recordings: TabRecording[] = [];
  try {
    for (const [index, job] of jobs.entries()) {
      if (index > 0) await api.sleep(resolved.throttleMs);
      recordings.push(await recordOne(job, origin, api, resolved));
    }
  } finally {
    await api.unregisterRecorder().catch(() => undefined);
  }
  return recordings;
}

/** 用真的 chrome API */
export function chromeTabRecorderApi(): TabRecorderApi {
  return {
    async registerRecorder(matches) {
      await chrome.scripting.registerContentScripts([
        {
          id: RECORDER_SCRIPT_ID,
          matches,
          js: [RECORDER_FILE],
          runAt: "document_start",
          world: "MAIN",
          allFrames: false,
          persistAcrossSessions: false,
        },
      ]);
    },
    async unregisterRecorder() {
      await chrome.scripting.unregisterContentScripts({ ids: [RECORDER_SCRIPT_ID] });
    },
    async isRecorderRegistered() {
      const scripts = await chrome.scripting.getRegisteredContentScripts({
        ids: [RECORDER_SCRIPT_ID],
      });
      return scripts.length > 0;
    },
    async executeScript(tabId, world, func, args = []) {
      const injection = {
        target: { tabId },
        world,
        func,
        args,
      } as unknown as chrome.scripting.ScriptInjection<unknown[], unknown>;
      const [first] = await chrome.scripting.executeScript(injection);
      return first?.result;
    },
    async createTab(url) {
      const tab = await chrome.tabs.create({ url, active: false });
      if (tab.id === undefined) throw new Error("tab has no id");
      return tab.id;
    },
    async removeTab(tabId) {
      await chrome.tabs.remove(tabId);
    },
    now: () => Date.now(),
    sleep: (ms) =>
      new Promise((resolve) => {
        setTimeout(resolve, ms);
      }),
  };
}

/** 后台启动时清掉上次留下的记录器注册（采集中途 service worker 被停掉时会留下） */
export async function cleanupStaleRecorder(
  api: TabRecorderApi = chromeTabRecorderApi(),
): Promise<void> {
  if (await api.isRecorderRegistered().catch(() => false)) {
    await api.unregisterRecorder().catch(() => undefined);
  }
}
