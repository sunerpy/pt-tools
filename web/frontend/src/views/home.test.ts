// @vitest-environment happy-dom
/*
 * 钉子：首页（媒体中心 M8，画板 02 桌面、07 手机；首页 NAS 概览 H2 的方案 A）。打开时读一次 GET /api/home（用缓存），
 * 「刷新」「重试」不用缓存；今天播出卡的动作（播放去媒体服务器、搜资源带上这一季、看版本滚到这一集）；待处理的 chip 去对应的页面，
 * 抽屉里的「忽略」逐个忽略这一行的问题再重新读、「处理」打开删除弹窗带上这一季和这几集；继续观看、最近入库、这周还要播的链接；
 * 右栏的速度、磁盘、下载器与三条状态行，速度跟着外壳的 transfer-stats；一块取不到只在那一块写原因；
 * 没连媒体服务器时继续观看整块不显示；手机上整张今天播出卡可点、不列下载器、磁盘各列一块、「刷新」在底部。
 * NAS 概览：TrueNAS 形状的来源（主机行、四张数值卡、存储池、硬盘与告警）、本机来源（下载器速度与下载盘、右栏不再列存储）、
 * 没有来源、连不上与登录失败、正在连、读不到的那类写原因、其余来源、两边都取不到、手机的摘要；「刷新」连同 GET /api/nas 重取。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router } from "vue-router";

import type { HomeData, NasKind, NasSnapshot, NasSource, NasView } from "@/api";
import { useRuntimeStore } from "@/stores/runtime";
import { type MountedView, mountView } from "@/test-mount";

const home = vi.hoisted(() => ({ get: vi.fn() }));
const nas = vi.hoisted(() => ({ get: vi.fn() }));
const organize = vi.hoisted(() => ({ dismissIssue: vi.fn(async (_id: number) => ({})) }));
const subscribe = vi.hoisted(() => ({
  // 删除弹窗打开时读预览；这里只看它带了什么，不让它读完
  deletePreview: vi.fn(() => new Promise(() => {})),
  deleteItem: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    homeApi: home,
    nasApi: { ...real.nasApi, ...nas },
    organizeApi: { ...real.organizeApi, ...organize },
    subscribeApi: { ...real.subscribeApi, ...subscribe },
    mediaApi: { ...real.mediaApi, settings: vi.fn(async () => ({ image_base_url: "" })) },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return { ...real, ElMessage: Object.assign(vi.fn(), ui) };
});

const TB = 1024 ** 4;
const GB = 1024 ** 3;
const MB = 1024 ** 2;

const TRUENAS: NasKind = {
  name: "truenas",
  label: "TrueNAS",
  experimental: false,
  username: "none",
  secret: "required",
  plain: "never",
};

/** 现在往前几秒的时刻（数据的时间按真实的现在算，「刚刚更新」才对得上） */
const ago = (sec: number) => new Date(Date.now() - sec * 1000).toISOString();

/** 用户 TrueNAS 形状的一个来源（数字按 2026-11-05 只读查询的量级编的） */
function truenas(over: Partial<NasSource> = {}, snap: Partial<NasSnapshot> = {}): NasSource {
  return {
    id: "1",
    name: "家里的 TrueNAS",
    kind: "truenas",
    enabled: true,
    experimental: false,
    url: "https://127.0.0.1:8443",
    has_secret: true,
    insecure_ack: false,
    state: "ok",
    last_ok_at: ago(2),
    snapshot: {
      at: ago(2),
      system: {
        hostname: "truenas",
        platform: "TrueNAS SCALE",
        version: "25.04.1",
        model: "i7-12700",
        cores: 8,
        uptime_seconds: 18 * 86400 + 18 * 3600 + 120,
      },
      cpu: { percent: 3.8, iowait: 12, load1: 3.7, load5: 3.1, load15: 2.9 },
      memory: { total: 64 * GB, used: 45.6 * GB, available: 18.4 * GB },
      network: { rx_bps: 9.4 * MB, tx_bps: 3.6 * MB, interfaces: ["ens161np1"] },
      pools: [
        {
          name: "UniData",
          status: "在线",
          healthy: true,
          size: 28.9 * TB,
          used: 24.7 * TB,
          free: 4.2 * TB,
          disks: 3,
        },
        {
          name: "QbitSSD",
          status: "在线",
          healthy: true,
          size: 3.6 * TB,
          used: 2.8 * TB,
          free: 0.8 * TB,
          disks: 1,
        },
      ],
      disks: [
        {
          name: "sdb",
          model: "ST16000NM000J",
          kind: "hdd",
          size: 14.6 * TB,
          temp_c: 37,
          pool: "UniData",
        },
        {
          name: "nvme0n1",
          model: "Colorful CN600",
          kind: "nvme",
          size: 1.8 * TB,
          temp_c: 59,
          pool: "SSDData",
        },
        { name: "sda", model: "Virtual disk", kind: "ssd", size: 46 * GB, pool: "boot-pool" },
      ],
      alerts: [
        { level: "info", title: "有应用可以更新", at: ago(3 * 86400) },
        { level: "critical", title: "Rsync 任务失败", at: ago(600) },
        { level: "warning", title: "存储池 UniData 已用超过 80%", at: ago(2 * 86400) },
        { level: "critical", title: "应用目录同步失败", at: ago(7200) },
      ],
      apps: { running: 1, total: 3, upgradable: 0 },
      facets: { cpu_temp: { status: "unsupported", reason: "TrueNAS 没有给 CPU 温度" } },
      ...snap,
    },
    history: [
      { at: ago(62), cpu: 5, mem: 70, rx: 4 * MB, tx: MB },
      { at: ago(32), cpu: 9, mem: 71, rx: 12 * MB, tx: 2 * MB },
      { at: ago(2), cpu: 3.8, mem: 71.3, rx: 9.4 * MB, tx: 3.6 * MB },
    ],
    ...over,
  };
}

/** 本机来源：网卡、存储池、硬盘、告警与应用都读不到 */
function local(over: Partial<NasSource> = {}): NasSource {
  const no = (reason: string) => ({ status: "unsupported" as const, reason });
  return {
    id: "local",
    name: "本机",
    kind: "local",
    enabled: true,
    experimental: false,
    has_secret: false,
    insecure_ack: false,
    state: "ok",
    last_ok_at: ago(5),
    snapshot: {
      at: ago(5),
      system: {
        hostname: "a1b2c3d4e5f6",
        platform: "Linux",
        version: "6.12.15-production+truenas",
        model: "i7-12700",
        cores: 8,
        uptime_seconds: 3 * 3600 + 5 * 60,
      },
      cpu: { percent: 12.5, iowait: 1.2, load1: 1, load5: 0.8, load15: 0.6, temp_c: 48 },
      memory: { total: 64 * GB, used: 32 * GB, available: 32 * GB },
      network: { rx_bps: 0, tx_bps: 0, interfaces: null },
      pools: null,
      disks: null,
      alerts: null,
      apps: { running: 0, total: 0, upgradable: 0 },
      facets: {
        network: no("容器里的网卡计数只有容器自己的流量"),
        pools: no("本机读不到存储池"),
        disks: no("本机读不到硬盘"),
        alerts: no("本机没有告警"),
        apps: no("本机读不到应用"),
      },
    },
    history: [{ at: ago(5), cpu: 12.5, mem: 50, rx: 0, tx: 0 }],
    ...over,
  };
}

/** GET /api/nas 的回应：home 是首页显示的那个，其余的跟在后面 */
function nasView(
  shown: NasSource | null,
  rest: NasSource[] = [],
  over: Partial<NasView> = {},
): NasView {
  return {
    home_source: "",
    home: shown?.id ?? "",
    local_enabled: true,
    local_supported: true,
    kinds: [TRUENAS],
    sources: [...(shown ? [shown] : []), ...rest],
    ...over,
  };
}

/** 首页接口的 nas 块与 GET /api/nas 一起换成这个来源 */
function withNas(shown: NasSource | null, rest: NasSource[] = [], over: Partial<NasView> = {}) {
  const v = nasView(shown, rest, over);
  nas.get.mockResolvedValue(v);
  return {
    nas: {
      data: {
        source: shown,
        others: rest
          .filter((r) => r.enabled && r.state !== "unsupported")
          .map((r) => ({ id: r.id, name: r.name, kind: r.kind, state: r.state })),
      },
    },
  } satisfies Partial<HomeData>;
}

function data(over: Partial<HomeData> = {}): HomeData {
  return {
    at: "2026-11-04T20:00:00Z",
    calendar: {
      data: {
        today: [
          {
            subscription_id: 1,
            media_type: "tv",
            tmdb_id: 100088,
            title: "长夜将尽",
            season: 2,
            episode: 9,
            air_date: "2026-11-04",
            state: "library",
            version: "2160p HHWEB",
            open_url: "http://jf.test/web/#/details?id=ep9",
          },
          {
            subscription_id: 2,
            media_type: "tv",
            tmdb_id: 200,
            title: "雾港",
            season: 1,
            episode: 21,
            air_date: "2026-11-04",
            state: "missing",
          },
          {
            subscription_id: 3,
            media_type: "tv",
            tmdb_id: 300,
            title: "北岸",
            season: 3,
            episode: 5,
            air_date: "2026-11-04",
            state: "waiting_version",
            lock: "HHWEB 2160p",
          },
        ],
        week: [
          {
            subscription_id: 1,
            media_type: "tv",
            tmdb_id: 100088,
            title: "长夜将尽",
            season: 2,
            episode: 10,
            air_date: "2026-11-05",
            state: "upcoming",
          },
        ],
        errors: [],
        subscriptions: 3,
      },
    },
    todo: {
      data: {
        groups: [
          {
            kind: "organize_failed",
            count: 2,
            label: "整理失败 2",
            target: "/media/history?status=failed",
            tone: "dang",
          },
          {
            kind: "server_removed",
            count: 1,
            label: "媒体服务器里删了 1 部",
            target: "drawer",
            tone: "warn",
          },
        ],
        items: [
          {
            kind: "organize_failed",
            title: "雾港 S01E20",
            detail: "目标已存在",
            target: "/media/history?status=failed",
          },
          {
            kind: "server_removed",
            title: "北岸 第 3 季",
            detail: "Jellyfin 里没了",
            target: "drawer",
            issue_ids: [7, 8],
            media_type: "tv",
            tmdb_id: 300,
            season: 3,
            episodes: [4, 5],
          },
        ],
      },
    },
    resume: {
      data: [
        {
          server_id: 1,
          name: "Jellyfin",
          kind: "jellyfin",
          user: "admin",
          items: [
            {
              server_id: 1,
              item_id: "abc",
              media_type: "tv",
              title: "长夜将尽",
              season: 1,
              episode: 13,
              percent: 40,
              remaining_seconds: 1080,
              open_url: "http://jf.test/web/#/details?id=abc",
            },
          ],
        },
      ],
    },
    recent: {
      data: {
        today_count: 1,
        items: [
          {
            media_type: "movie",
            tmdb_id: 603,
            title: "黑客帝国",
            quality: "4K",
            source: "pt-tools",
            at: "2026-11-04T08:00:00Z",
          },
        ],
      },
    },
    downloaders: {
      data: {
        downloaders: [
          {
            id: 1,
            name: "qb",
            type: "qbittorrent",
            online: true,
            down: 3 * MB,
            up: MB,
            downloading: 4,
            seeding: 1284,
          },
          {
            id: 2,
            name: "tr",
            type: "transmission",
            online: false,
            down: 0,
            up: 0,
            downloading: 0,
            seeding: 0,
            error: "连不上",
          },
        ],
        down: 3 * MB,
        up: MB,
        downloading: 4,
        seeding: 1284,
        speed_history: [],
      },
    },
    storage: {
      data: [
        {
          kind: "download",
          label: "下载盘 · qb",
          path: "/downloads",
          free: 1.2 * TB,
          total: 8 * TB,
          low: false,
        },
        {
          kind: "download",
          label: "下载盘 · tr",
          path: "/tr",
          free: 3 * TB,
          total: 8 * TB,
          low: false,
        },
        {
          kind: "library",
          label: "媒体库 · 电影",
          path: "/media",
          free: 10 * GB,
          total: 8 * TB,
          low: true,
        },
      ],
    },
    status: {
      data: {
        health: { at: "2026-11-04T09:30:00Z", ok: 12, warn: 0, fail: 1, skip: 0 },
        backup: { enabled: true, late: false, days: 0, last_ok_at: "2026-11-04T04:30:00Z" },
        servers: [
          {
            id: 1,
            name: "家里",
            kind: "jellyfin",
            version: "12.2",
            online: true,
            movies: 1204,
            series: 86,
            episodes: 3000,
            open_url: "http://jf.test/web/",
          },
        ],
      },
    },
    nas: { data: { source: truenas(), others: [] } },
    ...over,
  };
}

let view: MountedView | null = null;
let router: Router;

beforeEach(() => {
  setActivePinia(createPinia());
  nas.get.mockResolvedValue(nasView(truenas()));
});

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

const flush = async () => {
  for (let i = 0; i < 6; i++) await new Promise((r) => setTimeout(r, 0));
};
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const text = (id: string) => (q(id)?.textContent ?? "").replace(/\s+/g, " ").trim();

/** 手机断点与「减少动态效果」：测试里减少动态效果，数字直接到位、柱不推进 */
function viewport(mobile: boolean) {
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches:
          (mobile && query.includes("max-width: 768px")) ||
          query.includes("prefers-reduced-motion: reduce"),
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      }) as unknown as MediaQueryList,
  );
}

async function mount(opts: { mobile?: boolean } = {}) {
  viewport(Boolean(opts.mobile));
  const Page = (await import("./Home.vue")).default;
  const Stub = { template: "<div />" };
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/home", component: Page },
      { path: "/:any(.*)*", component: Stub },
    ],
  });
  await router.push("/home");
  view = mountView(Page, { router });
  await flush();
}

describe("首页", () => {
  it("打开时读一次（用缓存），页头写今天几集、几集入库、几件事要处理", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    expect(home.get).toHaveBeenCalledTimes(1);
    expect(home.get).toHaveBeenCalledWith(false);
    expect(document.querySelector("#pt-head-sub")?.textContent).toContain(
      "今天 3 集播出 · 其中 1 集已入库 · 3 件事要处理",
    );
  });

  it("今天播出卡：已入库「播放」去媒体服务器，缺的「搜资源」带上这一季，等版本「看版本」滚到这一集", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    expect(text("hm-today")).toContain("3 集");
    const play = q("hm-tc-act-1-2-9") as HTMLAnchorElement;
    expect(play.tagName).toBe("A");
    expect(play.getAttribute("href")).toBe("http://jf.test/web/#/details?id=ep9");
    expect(play.getAttribute("target")).toBe("_blank");
    expect(text("hm-tc-1-2-9")).toContain("S02E09 · 2160p HHWEB");
    expect(q("hm-tc-act-2-1-21")?.getAttribute("href")).toBe(
      "/search?media_type=tv&tmdb_id=200&season=1",
    );
    expect(q("hm-tc-act-3-3-5")?.getAttribute("href")).toBe(
      "/media/item/tv/300?from=home&season=3&episode=5",
    );
    expect(text("hm-tc-3-3-5")).toContain("锁定 HHWEB 2160p");
  });

  it("待处理：chip 去对应的页面并带上筛选，抽屉里的那一类在首页处理", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    expect(text("hm-todo-count")).toBe("3");
    q("hm-chip-organize_failed")!.click();
    await flush();
    expect(router.currentRoute.value.fullPath).toBe("/media/history?status=failed");

    await router.push("/home");
    await flush();
    q("hm-chip-server_removed")!.click();
    await flush();
    const drawer = document.querySelector(".hm-todo-drawer")!;
    expect(drawer.textContent).toContain("媒体服务器里删了 1 部");
    expect(drawer.querySelector('[data-testid="hm-row-server_removed-0"]')).not.toBeNull();
    expect(drawer.querySelector('[data-testid^="hm-row-organize_failed"]')).toBeNull();
  });

  it("抽屉里「忽略」逐个忽略这一行的问题，再不用缓存重新读", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    q("hm-todo-all")!.click();
    await flush();
    expect(q("hm-row-organize_failed-0")).not.toBeNull();
    q("hm-row-dismiss-1")!.click();
    await flush();
    expect(organize.dismissIssue.mock.calls.map((c) => c[0])).toEqual([7, 8]);
    expect(ui.success).toHaveBeenCalledWith("已忽略");
    expect(home.get).toHaveBeenLastCalledWith(true);
  });

  it("抽屉里「处理」打开删除弹窗，带上这一季和这几集", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    q("hm-todo-all")!.click();
    await flush();
    q("hm-row-handle-1")!.click();
    await flush();
    expect(subscribe.deletePreview).toHaveBeenCalledWith({
      media_type: "tv",
      tmdb_id: 300,
      season: 3,
      episodes: [4, 5],
    });
  });

  it("抽屉里别的行「去处理」去对应的页面并关上抽屉", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    q("hm-todo-all")!.click();
    await flush();
    q("hm-row-go-0")!.click();
    await flush();
    expect(router.currentRoute.value.fullPath).toBe("/media/history?status=failed");
  });

  it("忽略失败时写原因、不重新读", async () => {
    home.get.mockResolvedValue(data());
    organize.dismissIssue.mockRejectedValueOnce(new Error("问题不存在"));
    await mount();
    q("hm-todo-all")!.click();
    await flush();
    q("hm-row-dismiss-1")!.click();
    await flush();
    expect(ui.error).toHaveBeenCalledWith("问题不存在");
    expect(home.get).toHaveBeenCalledTimes(1);
  });

  it("继续观看、最近入库、这周还要播的链接", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    expect(text("hm-resume")).toContain("Jellyfin · admin");
    expect(q("hm-rc-1-abc")?.getAttribute("href")).toBe("http://jf.test/web/#/details?id=abc");
    expect(text("hm-rc-1-abc")).toContain("S01E13 · 还剩 18 分钟");
    const open = q("hm-resume")!.querySelector<HTMLAnchorElement>(".hm-sec__link")!;
    expect(open.textContent).toContain("在 Jellyfin 打开");
    expect(open.getAttribute("href")).toBe("http://jf.test/web/");

    expect(text("hm-recent")).toContain("今天 1");
    expect(q("hm-pc-movie-603-0")?.getAttribute("href")).toBe("/media/item/movie/603?from=home");
    expect(text("hm-pc-movie-603-0")).toContain("4K");

    expect(q("hm-wk-1-2-10-2026-11-05")?.getAttribute("href")).toBe(
      "/media/item/tv/100088?from=home&season=2&episode=10",
    );
    expect(text("hm-wk-1-2-10-2026-11-05")).toContain("长夜将尽 S02E10");
  });

  it("右栏：速度、各块盘、各台下载器与三条状态行", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    expect(text("hm-speed-down")).toBe("3.0 MB/s");
    expect(text("hm-speed-up")).toBe("1.0 MB/s");
    expect(text("hm-speed")).toContain("下载 · 下载中 4");
    expect(text("hm-speed")).toContain("上传 · 做种 1,284");
    expect(text("hm-disk-0")).toContain("剩余 1.2 TB / 8.0 TB");
    expect(q("hm-disk-2")?.querySelector(".hm-disk__free")?.classList.contains("is-dang")).toBe(
      true,
    );
    expect(text("hm-dl-1")).toContain("↓ 3.0 MB/s");
    expect(text("hm-dl-2")).toContain("离线：连不上");
    expect(text("hm-line-health")).toContain("1 项失败");
    expect(text("hm-line-backup")).toContain("成功");
    expect(text("hm-line-server")).toContain("Jellyfin 12.2");
    expect(text("hm-line-server")).toContain("继续观看 1");
  });

  it("右栏的速度跟着外壳这一拍取到的", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    const rt = useRuntimeStore();
    rt.downloadSpeed = 5 * MB;
    rt.uploadSpeed = 2 * MB;
    rt.downloaderStats = [
      {
        downloader_id: 2,
        downloader_name: "tr",
        downloader_type: "transmission",
        reachable: true,
        download_speed: 2 * MB,
        upload_speed: MB,
        uploaded: 0,
        downloaded: 0,
        session_uploaded: 0,
        session_downloaded: 0,
        free_space: 0,
      },
    ];
    rt.stale = false;
    await flush();
    expect(text("hm-speed-down")).toBe("5.0 MB/s");
    expect(text("hm-dl-2")).toContain("↓ 2.0 MB/s");
    rt.stale = true;
    await flush();
    expect(text("hm-speed-down"), "外壳没取到时用首页接口给的").toBe("3.0 MB/s");
  });

  it("一块取不到只在那一块写原因", async () => {
    home.get.mockResolvedValue(
      data({
        calendar: { data: null, error: "订阅服务没有启动" },
        downloaders: { data: null, error: "15 秒内没取到，稍后刷新" },
        status: { data: null, error: "出错了：boom" },
      }),
    );
    await mount();
    expect(text("hm-today-error")).toBe("取不到：订阅服务没有启动");
    expect(text("hm-week")).toContain("取不到：订阅服务没有启动");
    expect(text("hm-speed")).toContain("取不到：15 秒内没取到，稍后刷新");
    expect(text("hm-downloaders")).toContain("取不到：15 秒内没取到，稍后刷新");
    expect(text("hm-lines")).toContain("取不到：出错了：boom");
    expect(q("hm-pc-movie-603-0")).not.toBeNull();
    expect(document.querySelector("#pt-head-sub")?.textContent).toContain("3 件事要处理");
  });

  it("整个接口失败时写原因，「重试」不用缓存", async () => {
    home.get.mockRejectedValueOnce(new Error("网络断了")).mockResolvedValue(data());
    await mount();
    expect(text("hm-error")).toContain("网络断了");
    q("hm-error")!.querySelector("button")!.click();
    await flush();
    expect(home.get).toHaveBeenLastCalledWith(true);
    expect(q("hm-error")).toBeNull();
    expect(q("hm-tc-1-2-9")).not.toBeNull();
  });

  it("「刷新」不用缓存，NAS 概览连同 GET /api/nas 重取", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    expect(nas.get, "挂载后马上取一次").toHaveBeenCalledTimes(1);
    const btn = document.querySelector<HTMLButtonElement>(
      '#pt-head-acts [data-testid="hm-refresh"]',
    )!;
    btn.click();
    await flush();
    expect(home.get).toHaveBeenLastCalledWith(true);
    expect(nas.get).toHaveBeenCalledTimes(2);
  });

  it("没连媒体服务器时继续观看整块不显示；没有待处理时写一句", async () => {
    home.get.mockResolvedValue(
      data({
        resume: { data: [] },
        todo: { data: { groups: [], items: [] } },
        status: { data: { health: null, backup: null, servers: [] } },
      }),
    );
    await mount();
    expect(q("hm-resume")).toBeNull();
    expect(text("hm-todo-empty")).toBe("没有要处理的事");
    expect(q("hm-todo-all")).toBeNull();
    expect(text("hm-line-server")).toContain("媒体服务器");
    expect(text("hm-line-health")).toContain("体检");
    expect(document.querySelector("#pt-head-sub")?.textContent).toContain("没有要处理的事");
  });

  it("今天只有电影时计数只写几部", async () => {
    const d = data();
    const movie = (id: number) => ({
      subscription_id: id,
      media_type: "movie" as const,
      tmdb_id: id,
      title: `电影 ${id}`,
      air_date: "2026-11-04",
      state: "missing" as const,
    });
    d.calendar.data!.today = [movie(41), movie(42)];
    home.get.mockResolvedValue(d);
    await mount();
    expect(q("hm-today")!.querySelector(".hm-sec__count")?.textContent?.trim()).toBe("2 部");
  });

  it("还没有追剧时今天播出写一句，标题右边去探索", async () => {
    const d = data();
    d.calendar.data = { today: [], week: [], errors: [], subscriptions: 0 };
    home.get.mockResolvedValue(d);
    await mount();
    expect(text("hm-today-empty")).toBe("还没有追剧");
    expect(q("hm-today")!.querySelector(".hm-sec__link")?.getAttribute("href")).toBe(
      "/media/explore",
    );
    expect(text("hm-week-empty")).toBe("这周没有要播的集");
  });

  it("手机：整张今天播出卡可点，不列下载器，磁盘各列一块，「刷新」在底部", async () => {
    home.get.mockResolvedValue(data());
    await mount({ mobile: true });
    const lib = q("hm-tc-1-2-9")!;
    expect(lib.tagName).toBe("A");
    expect(lib.getAttribute("href")).toBe("http://jf.test/web/#/details?id=ep9");
    expect(q("hm-tc-2-1-21")?.getAttribute("href")).toBe(
      "/search?media_type=tv&tmdb_id=200&season=1",
    );
    expect(q("hm-downloaders")).toBeNull();
    const sub = document.querySelector("#pt-mhead-sub")?.textContent ?? "";
    expect(sub, "手机页头不写入库的那段").toContain("今天 3 集播出 · 3 件事要处理");
    expect(sub).not.toContain("已入库");
    expect(text("hm-disk-0")).toContain("下载盘 · qb");
    expect(text("hm-disk-0")).not.toContain("/ 8.0 TB");
    expect(text("hm-disk-1")).toContain("媒体库 · 电影");
    expect(q("hm-disk-2")).toBeNull();
    expect(text("hm-speed")).not.toContain("下载中 4");
    expect(document.querySelector('#pt-head-acts [data-testid="hm-refresh"]')).toBeNull();
    const btn = document.querySelector<HTMLButtonElement>(
      '.hm-mrefresh [data-testid="hm-refresh"]',
    )!;
    btn.click();
    await flush();
    expect(home.get).toHaveBeenLastCalledWith(true);
    q("hm-todo-all")!.click();
    await flush();
    expect(document.querySelector('[data-testid="hm-row-server_removed-1"]')).not.toBeNull();
  });
});

describe("首页的 NAS 概览", () => {
  const kpi = (key: string) => text(`hn-kpi-${key}`);
  const val = (key: string) =>
    (q(`hn-kpi-${key}`)?.querySelector('[data-testid="hn-kpi-val"]')?.textContent ?? "")
      .replace(/\s+/g, "")
      .trim();

  it("TrueNAS 来源：标题行、主机行与两个要处理的标签", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    const sec = q("hn")!;
    expect(sec.querySelector(".hm-sec__title")?.textContent).toBe("NAS 概览");
    expect(sec.querySelector(".hm-sec__count")?.textContent?.trim()).toBe("TrueNAS");
    expect(sec.querySelector(".hm-sec__link")?.getAttribute("href")).toBe("/system/nas");
    expect(text("hn-live")).toBe("刚刚更新");
    expect(q("hn-live")!.classList.contains("is-ok")).toBe(true);
    expect(q("hn-others"), "只有一个来源时不写其余的").toBeNull();

    const host = text("hn-host");
    expect(host).toContain("truenas");
    expect(text("hn-platform")).toBe("TrueNAS SCALE 25.04.1");
    expect(host).toContain("i7-12700 · 8 核");
    expect(host).toContain("64.0 GB");
    expect(host).toContain("已运行 18 天 18 小时");
    expect(text("hn-facts")).toContain("2 条严重告警");
    expect(text("hn-facts")).toContain("UniData 已用 85%");
    expect(q("hn-note")).toBeNull();
  });

  it("TrueNAS 来源：四张数值卡，读不到的温度写一句、悬停看原因", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    expect(val("cpu")).toBe("3.8%");
    expect(kpi("cpu")).toContain("8 核");
    expect(kpi("cpu")).toContain("负载 3.7 · IO 等待 12% · 读不到温度");
    expect(q("hn-kpi-cpu")!.querySelector(".hn-kpi__sub")?.getAttribute("title")).toBe(
      "TrueNAS 没有给 CPU 温度",
    );
    expect(val("mem")).toBe("71%");
    expect(kpi("mem")).toContain("共 64.0 GB");
    expect(kpi("mem")).toContain("已用 45.6 GB · 可用 18.4 GB");
    expect(val("net")).toBe("↓9.4MB/s");
    // 大字只放数字，「↓」与单位和 CPU 的「%」一样是小字（375 宽的两列卡放不下整串大字）
    const small = [
      ...q("hn-kpi-net")!.querySelectorAll('[data-testid="hn-kpi-val"] .hn-kpi__unit'),
    ];
    expect(small.map((e) => e.textContent?.trim())).toEqual(["↓", "MB/s"]);
    expect(kpi("net")).toContain("ens161np1");
    expect(kpi("net")).toContain("上行 3.6 MB/s · 所有网卡合计");
    expect(val("storage")).toBe("85%");
    expect(kpi("storage")).toContain("2 个存储池");
    expect(kpi("storage")).toContain("已用 27.5 TB · 剩余 5.0 TB");
    expect(q("hn-kpi-storage")!.querySelectorAll(".hn-seg__s")).toHaveLength(2);
    expect(kpi("storage")).toContain("UniData 85%");
    // 每张卡一个色相；前三张有柱，存储卡是分段条
    expect(q("hn-kpi-cpu")!.style.getPropertyValue("--hue")).toBe("var(--pt-kpi-1)");
    expect(q("hn-kpi-net")!.style.getPropertyValue("--hue")).toBe("var(--pt-kpi-2)");
    expect(q("hn-kpi-mem")!.style.getPropertyValue("--hue")).toBe("var(--pt-kpi-3)");
    expect(q("hn-kpi-storage")!.style.getPropertyValue("--hue")).toBe("var(--pt-kpi-4)");
    expect(q("hn-kpi-cpu")!.querySelectorAll(".pt-bars i")).toHaveLength(3);
    expect(q("hn-kpi-storage")!.querySelector(".pt-bars")).toBeNull();
  });

  it("TrueNAS 来源：存储池、硬盘与告警和应用；右栏照常列下载盘与媒体库", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    expect(text("hn-pools")).toContain("都健康");
    expect(text("hn-pool-0")).toContain("UniData");
    expect(text("hn-pool-0")).toContain("在线");
    expect(text("hn-pool-0")).toContain("已用 24.7 TB · 共 28.9 TB · 3 块硬盘 · 快满了");
    expect(q("hn-pool-0")!.querySelector(".hn-pool__pct")?.classList.contains("is-warn")).toBe(
      true,
    );
    expect(text("hn-pool-1")).toContain("78%");
    expect(text("hn-pool-1")).not.toContain("快满了");

    expect(text("hn-disks")).toContain("最高 59 °C · nvme0n1");
    expect(text("hn-disk-tile-0")).toContain("37°C");
    expect(text("hn-disk-tile-0")).toContain("HDD · 14.6 TB");
    expect(q("hn-disk-tile-0")?.getAttribute("title")).toBe(
      "sdb · ST16000NM000J · 14.6 TB · UniData",
    );
    expect(text("hn-disk-tile-2")).toContain("没有温度");

    expect(text("hn-alerts")).toContain("TrueNAS 里没关掉的");
    expect(text("hn-acount-critical")).toBe("2严重");
    expect(q("hn-acount-critical")!.classList.contains("is-dang")).toBe(true);
    expect(text("hn-acount-info")).toBe("1信息");
    const rows = [...q("hn-alerts")!.querySelectorAll(".hn-arow__t")].map((e) => e.textContent);
    expect(rows, "最新的三条，新的在前").toEqual([
      "Rsync 任务失败",
      "应用目录同步失败",
      "存储池 UniData 已用超过 80%",
    ]);
    expect(text("hn-apps")).toBe("应用 运行中 1 个 · 共 3 个 · 没有可更新的");

    expect(text("hm-storage")).toContain("下载盘 · qb");
    expect(q("hn-local-storage")).toBeNull();
  });

  it("Glances 来源：存储池不写健康，硬盘只有温度，告警写从哪来，应用不写可更新几个", async () => {
    const GLANCES: NasKind = {
      name: "glances",
      label: "Glances",
      experimental: false,
      username: "optional",
      secret: "with_username",
      plain: "without_credentials",
    };
    const src = truenas(
      {
        id: "2",
        name: "书房的 NAS",
        kind: "glances",
        url: "http://127.0.0.1:61208",
        has_secret: false,
      },
      {
        system: {
          hostname: "nas",
          platform: "Debian GNU/Linux 12 (bookworm)",
          version: "6.1.0-28-amd64",
          model: "Intel(R) N100",
          cores: 4,
          uptime_seconds: 86400,
        },
        pools: [
          {
            name: "/volume1",
            status: "",
            size: 16.3 * TB,
            used: 8.5 * TB,
            free: 7.8 * TB,
            disks: 0,
          },
        ],
        disks: [{ name: "NVMe 1", model: "", kind: "nvme", size: 0, temp_c: 41 }],
        alerts: [],
        apps: { running: 4, total: 5, upgradable: 0, updates_unknown: true },
        facets: {},
      },
    );
    home.get.mockResolvedValue(data(withNas(src, [], { kinds: [TRUENAS, GLANCES] })));
    await mount();
    expect(text("hn-pools"), "读不到健康时不写「都健康」").not.toContain("都健康");
    expect(text("hn-pool-0")).toContain("/volume1");
    expect(text("hn-pool-0")).toContain("已用 8.5 TB · 共 16.3 TB");
    expect(text("hn-disk-tile-0")).toContain("41°C");
    expect(text("hn-disk-tile-0")).toContain("NVMe");
    expect(text("hn-alerts"), "Glances 的告警是 pt-tools 按软 RAID 阵列算的").toContain(
      "按软 RAID 阵列的状态",
    );
    expect(text("hn-alerts")).not.toContain("里没关掉的");
    expect(text("hn-alerts")).toContain("没有告警");
    expect(text("hn-apps")).toBe("应用 运行中 4 个 · 共 5 个");
  });

  it("本机来源：网络卡是下载器速度，存储是下载盘与媒体库，右栏不再列一遍", async () => {
    const d = data(withNas(local(), [], { kinds: [] }));
    // 网络卡的柱是下载器的下载速度：三拍 1、2、3 MB/s（本机来源自己的网卡计数是 0）
    d.downloaders.data!.speed_history = [1, 2, 3].map((n, i) => ({
      at: ago(65 - i * 30),
      down: n * MB,
      up: 0,
      answered: 1,
      total: 1,
    }));
    home.get.mockResolvedValue(d);
    await mount();
    const netBars = [...q("hn-kpi-net")!.querySelectorAll<HTMLElement>(".pt-bars__track i")];
    expect(netBars.length, "柱跟着下载器速度的采样，不是本机网卡的一拍").toBe(3);
    const heights = netBars.map((b) => parseFloat(b.style.height));
    expect(heights[0]).toBeLessThan(heights[1]!);
    expect(heights[1]).toBeLessThan(heights[2]!);
    expect(q("hn")!.querySelector(".hm-sec__count")?.textContent?.trim()).toBe("本机");
    expect(text("hn-host")).toContain("pt-tools 所在主机");
    expect(q("hn-host")!.querySelector(".hn-host__title")?.getAttribute("title")).toBe(
      "主机名 a1b2c3d4e5f6",
    );
    expect(text("hn-platform")).toBe("Linux 6.12.15-production+truenas");
    expect(text("hn-host")).toContain("已运行 3 小时 5 分");
    expect(q("hn-facts")).toBeNull();

    expect(kpi("cpu")).toContain("负载 1.0 · IO 等待 1.2% · 48 °C");
    expect(kpi("net")).toContain("下载器速度");
    expect(kpi("net")).toContain("本机读不到网卡");
    expect(val("net")).toBe("↓3.0MB/s");
    expect(kpi("net")).toContain("上传 1.0 MB/s · 所有下载器合计");
    // 三块盘各 8 TB，剩余 1.2 TB、3 TB 与 10 GB：已用 82%
    expect(val("storage")).toBe("82%");
    expect(kpi("storage")).toContain("所在的盘");

    expect(text("hn-disk-0")).toContain("下载盘 · qb");
    expect(text("hn-disk-0")).toContain("已用 6.8 TB · 共 8.0 TB");
    expect(q("hn-disk-2")!.querySelector(".hn-pool__pct")?.classList.contains("is-dang")).toBe(
      true,
    );
    expect(q("hn-local-more")!.querySelector(".hn-blk__note")?.textContent).toBe(
      "本机来源读不到这些。",
    );
    expect(q("hn-local-connect"), "没有能接入的类型时不放链接").toBeNull();
    expect(q("hn-pools")).toBeNull();
    expect(q("hm-storage"), "右栏不再列下载盘与媒体库").toBeNull();
    expect(q("hm-disk-0")).toBeNull();
    expect(q("hm-downloaders")).not.toBeNull();
  });

  it("本机来源、有能接入的类型时，虚线框写能接入什么并给链接", async () => {
    home.get.mockResolvedValue(data(withNas(local())));
    await mount();
    expect(text("hn-local-more")).toContain("接入 TrueNAS 以后，这里会显示每块硬盘的温度");
    expect(q("hn-local-connect")?.getAttribute("href")).toBe("/system/nas");
  });

  it("没有来源时是空状态卡：本机来源关着时按钮去 NAS 监控", async () => {
    home.get.mockResolvedValue(
      data(
        withNas(null, [local({ enabled: false, state: "disabled" })], {
          kinds: [],
          local_enabled: false,
        }),
      ),
    );
    await mount();
    expect(text("hn-empty")).toContain("还没有 NAS 监控来源");
    expect(text("hn-empty")).toContain("本机来源已关闭");
    expect(text("hn-connect")).toBe("去 NAS 监控");
    expect(q("hn-live")).toBeNull();
    expect(q("hm-storage"), "没有来源时右栏照常列存储").not.toBeNull();
    q("hn-connect")!.click();
    await flush();
    expect(router.currentRoute.value.fullPath).toBe("/system/nas");
  });

  it("没有来源、有能接入的类型时写能接入什么，按钮是「接入 NAS」", async () => {
    home.get.mockResolvedValue(data(withNas(null, [], { local_supported: false })));
    await mount();
    expect(text("hn-empty")).toContain(
      "接入 TrueNAS 以后，这里显示 CPU、内存、网络、存储池、硬盘温度与告警。",
    );
    expect(text("hn-empty")).toContain("本机来源只支持 Linux。");
    expect(text("hn-connect")).toBe("接入 NAS");
  });

  it("连不上：红色说明写原因与下次重试，标题行写数据多旧，数值卡变灰", async () => {
    const down = truenas(
      {
        state: "error",
        message: "dial tcp 127.0.0.1:8443: connect: connection refused",
        last_ok_at: ago(185),
        next_try_at: new Date(Date.now() + 90_000).toISOString(),
      },
      { at: ago(185) },
    );
    home.get.mockResolvedValue(data(withNas(down)));
    await mount();
    expect(text("hn-note")).toContain(
      "连不上 家里的 TrueNAS：dial tcp 127.0.0.1:8443: connect: connection refused。2 分钟后再试",
    );
    expect(q("hn-note")!.querySelector("a")?.getAttribute("href")).toBe("/system/nas");
    expect(text("hn-live")).toBe("连不上 · 这是 3 分钟前的数据");
    expect(q("hn-live")!.classList.contains("is-dang")).toBe(true);
    expect(q("hn")!.classList.contains("is-stale")).toBe(true);
    expect(q("hn-kpi-cpu")!.style.getPropertyValue("--hue")).toBe("var(--pt-t4)");
    expect(val("cpu"), "旧数据照常列出来").toBe("3.8%");
  });

  it("连着但数据晚了三拍以上：更新时间改用提醒色，90 秒以内照常", async () => {
    home.get.mockResolvedValue(data(withNas(truenas({ last_ok_at: ago(130) }, { at: ago(130) }))));
    await mount();
    expect(text("hn-live")).toBe("2 分钟前更新");
    expect(q("hn-live")!.classList.contains("is-warn")).toBe(true);
    expect(q("hn")!.classList.contains("is-stale"), "连着的来源不按旧数据画").toBe(false);
    view?.unmount();

    home.get.mockResolvedValue(data(withNas(truenas({ last_ok_at: ago(80) }, { at: ago(80) }))));
    await mount();
    expect(text("hn-live")).toBe("1 分钟前更新");
    expect(q("hn-live")!.classList.contains("is-ok")).toBe(true);
  });

  it("登录失败、还没读到过数据：说明写去哪里处理，卡里写还没有数据", async () => {
    const bad = truenas({
      state: "auth_failed",
      message: "API Key 不对",
      snapshot: null,
      history: [],
    });
    home.get.mockResolvedValue(data(withNas(bad)));
    await mount();
    expect(q("hn-note")!.querySelector(".hn-note__text")?.textContent).toBe(
      "家里的 TrueNAS 登录失败：API Key 不对。改了设置或点「重新连接」以后才会再试",
    );
    expect(text("hn-wait")).toBe("还没有读到过 家里的 TrueNAS 的数据");
    expect(text("hn-live")).toBe("登录失败");
    expect(q("hn-kpi-cpu")).toBeNull();
  });

  it("正在连：标题行写正在连，卡里写读到以后显示", async () => {
    home.get.mockResolvedValue(
      data(withNas(local({ state: "connecting", snapshot: null, history: [] }))),
    );
    await mount();
    expect(text("hn-live")).toBe("正在连");
    expect(text("hn-wait")).toBe("正在连 本机，读到数据以后显示在这里");
    expect(q("hn-note")).toBeNull();
  });

  it("来源读不到的那类写原因，不当成 0", async () => {
    const partial = truenas(
      {},
      {
        pools: null,
        disks: null,
        alerts: null,
        facets: {
          pools: { status: "unsupported", reason: "这个来源读不到存储池" },
          disks: { status: "denied" },
          alerts: { status: "error", reason: "超时" },
          apps: { status: "unsupported" },
        },
      },
    );
    home.get.mockResolvedValue(data(withNas(partial)));
    await mount();
    expect(val("storage")).toBe("—");
    expect(kpi("storage")).toContain("这个来源读不到存储池");
    expect(text("hn-pools")).toContain("这个来源读不到存储池");
    expect(text("hn-disks")).toContain("账号没有读这一类的权限");
    expect(text("hn-alerts")).toContain("超时");
    expect(q("hn-acount-critical")).toBeNull();
    expect(text("hn-apps")).toBe("应用：这个来源读不到");
    expect(q("hn-facts"), "读不到告警与存储池时没有要处理的标签").toBeNull();
  });

  it("首页没在显示的来源：有连不上的写几个有问题，悬停看各自的状态", async () => {
    const other = truenas({ id: "2", name: "公司的 TrueNAS", state: "error" });
    home.get.mockResolvedValue(data(withNas(truenas(), [local(), other])));
    await mount();
    expect(text("hn-others")).toBe("1 个来源有问题");
    expect(q("hn-others")!.classList.contains("is-dang")).toBe(true);
    expect(q("hn-others")?.getAttribute("title")).toBe(
      "本机：已连上\n公司的 TrueNAS（TrueNAS）：连不上",
    );
    expect(q("hn-others")?.getAttribute("href")).toBe("/system/nas");
  });

  it("首页没在显示的来源都好时写还有几个", async () => {
    home.get.mockResolvedValue(data(withNas(truenas(), [local()])));
    await mount();
    expect(text("hn-others")).toBe("另有 1 个来源");
    expect(q("hn-others")!.classList.contains("is-dang")).toBe(false);
  });

  it("首页接口与 GET /api/nas 都取不到时写原因", async () => {
    nas.get.mockRejectedValue(new Error("NAS 监控没有启动\n"));
    home.get.mockResolvedValue(data({ nas: { data: null, error: "NAS 监控没有启动" } }));
    await mount();
    expect(text("hn-error")).toBe("取不到 NAS 监控：NAS 监控没有启动");
    expect(q("hm-storage"), "右栏照常列存储").not.toBeNull();
  });

  it("GET /api/nas 取到的比首页接口带的新：换成它", async () => {
    home.get.mockResolvedValue(data());
    nas.get.mockResolvedValue(nasView(local()));
    await mount();
    expect(text("hn-host")).toContain("pt-tools 所在主机");
    expect(q("hm-storage")).toBeNull();
  });

  it("手机：数值卡 2×2，硬盘与告警各缩成一行摘要，不写其余来源", async () => {
    home.get.mockResolvedValue(data(withNas(truenas(), [local()])));
    await mount({ mobile: true });
    expect(q("hn")!.classList.contains("is-mobile")).toBe(true);
    expect(q("hn-disks")).toBeNull();
    expect(q("hn-alerts")).toBeNull();
    expect(q("hn-pool-0")).not.toBeNull();
    expect(text("hn-msum-alerts")).toBe("告警 2 严重 · 1 警告 · 1 信息");
    expect(q("hn-msum-alerts")!.querySelector("b")?.classList.contains("is-dang")).toBe(true);
    expect(text("hn-msum-disks")).toBe("硬盘 3 块 · 最高 59 °C（nvme0n1）");
    expect(kpi("storage")).toContain("剩余 5.0 TB");
    expect(kpi("storage")).not.toContain("UniData 85%");
    expect(q("hn-others")).toBeNull();
    expect(q("hn-kpi-cpu")!.querySelectorAll(".pt-bars i")).toHaveLength(3);
  });

  /** 一张数值卡副行的整句、卡窄时换成的短句，与悬停提示（换哪一句由 .hn-kpi 的容器查询按卡宽判，这里钉两句都在） */
  function subs(k: string) {
    const el = q(`hn-kpi-${k}`)!.querySelector(".hn-kpi__sub")!;
    return {
      full: el.querySelector(".hn-kpi__full")?.textContent ?? null,
      short: el.querySelector(".hn-kpi__short")?.textContent ?? null,
      hasShort: el.classList.contains("has-short"),
      title: el.getAttribute("title"),
    };
  }

  it("数值卡窄的时候换短副行：只写最要紧的一两项，整句在悬停提示里", async () => {
    home.get.mockResolvedValue(data(withNas(truenas(), [local()])));
    await mount();
    expect(subs("cpu")).toEqual({
      full: "负载 3.7 · IO 等待 12% · 读不到温度",
      short: "负载 3.7",
      hasShort: true,
      title: "TrueNAS 没有给 CPU 温度",
    });
    expect(subs("mem")).toMatchObject({
      full: "已用 45.6 GB · 可用 18.4 GB",
      short: "已用 45.6 GB",
    });
    expect(subs("mem").title).toBe("已用 45.6 GB · 可用 18.4 GB");
    expect(subs("net")).toMatchObject({
      full: "上行 3.6 MB/s · 所有网卡合计",
      short: "上行 3.6 MB/s",
    });
    expect(subs("storage").short).toMatch(/^剩余 /);
    const aside = q("hn-kpi-net")!.querySelector(".hn-kpi__aside")!;
    expect(aside.textContent, "网卡名放得下，卡窄时照写").toBe("ens161np1");
    expect(aside.classList.contains("is-wide")).toBe(false);
  });

  it("本机来源：「本机读不到网卡」卡窄时不写；副行的短句是上传、负载与温度、已用", async () => {
    home.get.mockResolvedValue(data(withNas(local(), [], { kinds: [] })));
    await mount({ mobile: true });
    const aside = q("hn-kpi-net")!.querySelector(".hn-kpi__aside")!;
    expect(aside.textContent).toBe("本机读不到网卡");
    expect(aside.classList.contains("is-wide")).toBe(true);
    expect(subs("net")).toMatchObject({
      full: "上传 1.0 MB/s · 所有下载器合计",
      short: "上传 1.0 MB/s",
    });
    expect(subs("cpu").short).toBe("负载 1.0 · 48 °C");
    expect(subs("mem").short).toBe("已用 32.0 GB");
  });

  it("读不到的那类只有原因，没有短句", async () => {
    home.get.mockResolvedValue(
      data(
        withNas(
          truenas({}, { facets: { network: { status: "denied", reason: "没有读网卡的权限" } } }),
        ),
      ),
    );
    await mount();
    expect(subs("net")).toMatchObject({ full: "没有读网卡的权限", short: null, hasShort: false });
  });
});
