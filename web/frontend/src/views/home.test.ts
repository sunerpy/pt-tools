// @vitest-environment happy-dom
/*
 * 钉子：首页（媒体中心 M8，画板 02 桌面、07 手机）。打开时读一次 GET /api/home（用缓存），「刷新」「重试」不用缓存；
 * 今天播出卡的动作（播放去媒体服务器、搜资源带上这一季、看版本滚到这一集）；待处理的 chip 去对应的页面，
 * 抽屉里的「忽略」逐个忽略这一行的问题再重新读、「处理」打开删除弹窗带上这一季和这几集；继续观看、最近入库、这周还要播的链接；
 * NAS 状态的速度、磁盘、下载器与三条状态行，速度跟着外壳的 transfer-stats；一块取不到只在那一块写原因；
 * 没连媒体服务器时继续观看整块不显示；手机上整张今天播出卡可点、不列下载器、磁盘各列一块、「刷新」在底部。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router } from "vue-router";

import type { HomeData } from "@/api";
import { useRuntimeStore } from "@/stores/runtime";
import { type MountedView, mountView } from "@/test-mount";

const home = vi.hoisted(() => ({ get: vi.fn() }));
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
    ...over,
  };
}

let view: MountedView | null = null;
let router: Router;

beforeEach(() => {
  setActivePinia(createPinia());
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

function viewport(mobile: boolean) {
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches: mobile && query.includes("max-width: 768px"),
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

  it("NAS 状态：速度、各块盘、各台下载器与三条状态行", async () => {
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

  it("NAS 状态的速度跟着外壳这一拍取到的", async () => {
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

  it("「刷新」不用缓存", async () => {
    home.get.mockResolvedValue(data());
    await mount();
    const btn = document.querySelector<HTMLButtonElement>(
      '#pt-head-acts [data-testid="hm-refresh"]',
    )!;
    btn.click();
    await flush();
    expect(home.get).toHaveBeenLastCalledWith(true);
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
