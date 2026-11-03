// @vitest-environment happy-dom
/*
 * 钉子：条目详情页（媒体中心 M3）。打开时读条目与路由里的季；大图写资料、评分、IMDb；「你的 NAS 上」三行；
 * 季 tab 换季重新读、路由跟着换；集卡的状态与说明、缺的集点「搜资源」去按条目搜索；种子行与能不能删；
 * 演职员导演在前；推荐点了进那一部；没订阅时「追剧」打开订阅弹窗；读不到时能重试；电影没有季、种子块叫「这部片的种子」。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  item: vi.fn(),
  itemSeason: vi.fn(),
  setStatus: vi.fn(),
  addBlocks: vi.fn(),
  profiles: vi.fn(async () => ({ items: [], resolutions: [], sources: [], codecs: [] })),
  get: vi.fn(),
  episodes: vi.fn(async () => []),
  runs: vi.fn(async () => ({ items: [], total: 0 })),
}));
const organize = vi.hoisted(() => ({
  rescrape: vi.fn(),
  libraries: vi.fn(async () => []),
  preview: vi.fn(),
}));
const media = vi.hoisted(() => ({ settings: vi.fn(async () => ({ image_base_url: "" })) }));
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
    subscribeApi: api,
    organizeApi: organize,
    mediaApi: media,
    downloadersApi: { list: vi.fn(async () => []) },
    sitesApi: { list: vi.fn(async () => ({})) },
    downloaderDirectoriesApi: {
      ...real.downloaderDirectoriesApi,
      listAll: vi.fn(async () => ({})),
    },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return { ...real, ElMessage: Object.assign(vi.fn(), ui) };
});

let view: MountedView | null = null;
let router: Router;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);
const qa = (id: string) => [...document.querySelectorAll<HTMLElement>(`[data-testid=${id}]`)];

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

const tlou = {
  item: {
    media_type: "tv",
    tmdb_id: 100088,
    title: "长夜将尽",
    original_title: "The Long Night",
    year: 2025,
    overview: "十年前的一桩旧案在暴雨夜重新浮出水面。",
    genres: ["悬疑", "犯罪"],
    countries: ["CN"],
    runtime: 45,
    status: "Returning Series",
    poster_path: "/p.jpg",
    backdrop_path: "/b.jpg",
    rating: 8.4,
    votes: 1204,
    imdb_id: "tt3581920",
    tvdb_id: 392256,
    studios: [],
    directors: ["陈默"],
    cast: [{ name: "周屿", character: "周远", order: 0, profile_path: "/a.jpg" }],
    seasons_count: 2,
    episodes_count: 24,
  },
  seasons: [
    { n: 1, name: "第 1 季", episode_count: 12, have: 12 },
    { n: 2, name: "第 2 季", episode_count: 12, have: 6 },
    { n: 0, name: "特别篇", episode_count: 2, have: 0 },
  ],
  nas: {
    library: {
      seasons: [
        { n: 1, have: 12, total: 12 },
        { n: 2, have: 6, total: 12 },
      ],
      files: 18,
      size: 82.6 * 1024 ** 3,
      libraries: ["剧集库"],
      record_id: 77,
    },
    seeding: { count: 4, hr_pending: 1, hr_left_seconds: 2 * 86400, lowest_ratio: 0.42 },
  },
  subscriptions: [
    {
      id: 5,
      status: "active",
      season: 2,
      follow: true,
      lock: { group: "HHWEB", resolution: "2160p" },
    },
  ],
  torrents: [
    {
      site: "hdsky",
      torrent_id: "31",
      title: "The.Long.Night.S02E01-E04.2160p-HHWEB",
      season: 2,
      episodes: "E01–E04",
      version: "2160p HHWEB",
      size: 27.3 * 1024 ** 3,
      downloader_id: 1,
      downloader: "qb",
      hash: "abc",
      state: "seeding",
      percent: 100,
      ratio: 1.31,
      seeding_seconds: 18 * 86400,
      hr: true,
      hr_hours: 72,
      deletable: true,
      reason: "H&R 已满足",
    },
    {
      site: "hdsky",
      title: "The.Long.Night.S02E07.2160p-HHWEB",
      season: 2,
      episodes: "E07",
      size: 7 * 1024 ** 3,
      downloader_id: 1,
      downloader: "qb",
      hash: "def",
      state: "downloading",
      percent: 63,
      eta_seconds: 720,
      ratio: 0,
      seeding_seconds: 0,
      hr: false,
      deletable: false,
      reason: "受保护：下载中",
    },
    {
      site: "ourbits",
      title: "The.Long.Night.S01.2160p",
      season: 1,
      episodes: "整季",
      size: 86 * 1024 ** 3,
      downloader_id: 1,
      downloader: "qb",
      hash: "ghi",
      state: "seeding",
      percent: 100,
      ratio: 2,
      seeding_seconds: 41 * 86400,
      hr: false,
      deletable: true,
      reason: "可以删",
    },
  ],
  recommendations: [
    {
      id: 9,
      media_type: "tv",
      title: "雾港",
      original_title: "",
      in_library: true,
      subscribed: false,
    },
    {
      id: 10,
      media_type: "tv",
      title: "北方来信",
      original_title: "",
      in_library: false,
      subscribed: true,
    },
  ],
  errors: [],
};

const season2 = {
  tmdb_id: 100088,
  season: 2,
  name: "第 2 季",
  subscription: {
    id: 5,
    status: "active",
    follow: true,
    airtime: true,
    search_delay_hours: 2,
    lock: { group: "HHWEB", resolution: "2160p" },
  },
  episodes: [
    {
      episode: 1,
      name: "归来",
      air_date: "2026-10-05",
      state: "library",
      version: "2160p HHWEB",
      size: 6.8 * 1024 ** 3,
      still_path: "/s1.jpg",
    },
    {
      episode: 7,
      name: "失踪者",
      air_date: "2026-10-26",
      state: "downloading",
      percent: 63,
      eta_seconds: 720,
    },
    { episode: 8, name: "灯塔", air_date: "2026-10-27", state: "waiting_version" },
    { episode: 9, name: "雨夜", air_date: "2026-10-28", state: "missing" },
    { episode: 10, name: "第 10 集", air_date: "2099-11-03", state: "upcoming" },
  ],
  errors: [],
};

async function mountAt(path: string, opts: { mobile?: boolean } = {}) {
  viewport(Boolean(opts.mobile));
  const Page = (await import("./MediaItem.vue")).default;
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/media/item/:type(movie|tv)/:id(\\d+)", name: "media-item", component: Page },
      { path: "/:p(.*)*", component: { render: () => null } },
    ],
  });
  await router.push(path);
  await router.isReady();
  // 页面靠路由参数取条目：挂一个 RouterView，换条目（点推荐）时同一个页面跟着路由重新取
  const { h } = await import("vue");
  const { RouterView } = await import("vue-router");
  view = mountView({ render: () => h(RouterView) }, { router });
  await vi.waitFor(() => expect(api.item).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("条目详情", () => {
  it("剧集：资料、你的 NAS 上、季与集、种子、演职员、推荐", async () => {
    api.item.mockResolvedValue(tlou);
    api.itemSeason.mockResolvedValue(season2);
    await mountAt("/media/item/tv/100088?from=subscriptions&season=2");
    expect(api.item).toHaveBeenCalledWith("tv", 100088);
    await vi.waitFor(() => expect(api.itemSeason).toHaveBeenCalledWith(100088, 2));
    await flush();

    expect(q("mi-title")!.textContent).toBe("长夜将尽");
    const hero = q("mi-hero")!.textContent ?? "";
    expect(hero).toContain("The Long Night · 2025 · 2 季 24 集");
    expect(q("mi-pills")!.textContent).toContain("中国大陆");
    expect(q("mi-pills")!.textContent).toContain("连载中");
    expect(q("mi-rating")!.textContent).toContain("8.4");
    expect(q("mi-rating")!.textContent).toContain("TMDB 1,204 人评分");
    expect(q("mi-imdb")!.getAttribute("href")).toBe("https://www.imdb.com/title/tt3581920/");
    expect(q("mi-tvdb")).not.toBeNull();
    expect(hero).toContain("导演 陈默 · 主演 周屿");
    expect(q("mi-sub")!.textContent).toContain("追剧中 · 第 2 季");
    expect(q("mi-rescrape"), "有整理记录时能重新刮削").not.toBeNull();

    expect(q("mi-nas-library")!.textContent).toContain("库里 S01 12/12 · S02 6/12");
    expect(q("mi-nas-library")!.textContent).toContain("剧集库 · 18 个文件 · 82.6 GB");
    expect(q("mi-nas-seeding")!.textContent).toContain("1 个 H&R 还差 2 天");
    expect(q("mi-nas-subscription")!.textContent).toContain("版本锁 HHWEB 2160p");

    expect(q("mi-tab-2")!.classList.contains("is-on")).toBe(true);
    expect(q("mi-tab-2")!.textContent).toContain("6/12");
    expect(q("mi-tab-0")!.textContent).toContain("特别篇");
    expect(q("mi-legend")!.textContent).toContain("已入库 1");
    expect(q("mi-ep-1")!.textContent).toContain("E01 · 归来");
    expect(q("mi-ep-1")!.textContent).toContain("2160p HHWEB · 6.8 GB");
    expect(q("mi-ep-7")!.textContent).toContain("63% · 还要 12 分钟");
    expect(q("mi-ep-8")!.textContent).toContain("锁定 HHWEB 2160p");
    expect(q("mi-ep-act-8")!.textContent).toContain("看版本");
    expect(q("mi-ep-10")!.textContent).toContain("播出后 2 小时开始搜");
    expect(q("mi-ep-10")!.textContent).toContain("季终");

    const rows = qa("mi-trow");
    expect(rows, "第 2 季的两个").toHaveLength(2);
    expect(rows[0]!.textContent).toContain("S02E01–E04 · 2160p HHWEB · 27.3 GB");
    expect(rows[0]!.textContent).toContain("H&R 已满足");
    expect(rows[1]!.textContent).toContain("受保护：下载中");
    expect(rows[1]!.textContent).toContain("63% · 还要 12 分钟");

    const cast = q("mi-cast")!.textContent ?? "";
    expect(cast.indexOf("陈默")).toBeLessThan(cast.indexOf("周屿"));
    expect(cast).toContain("周远");
    expect(q("mi-cast-all")!.textContent).toContain("全部 2 位");
    expect(q("mi-rec-9")!.textContent).toContain("已入库");
    expect(q("mi-rec-10")!.textContent).toContain("已订阅");

    // 缺的集点「搜资源」：按条目搜索这一季
    q("mi-ep-act-9")!.click();
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe("/search"));
    expect(router.currentRoute.value.query).toEqual({
      media_type: "tv",
      tmdb_id: "100088",
      season: "2",
    });
  });

  it("换季重新读这一季、路由跟着换；种子块跟着季", async () => {
    api.item.mockResolvedValue(tlou);
    api.itemSeason.mockImplementation(async (_id: number, n: number) => ({
      ...season2,
      season: n,
      episodes:
        n === 1
          ? [{ episode: 1, name: "第一集", air_date: "2025-01-01", state: "library" }]
          : season2.episodes,
    }));
    await mountAt("/media/item/tv/100088?from=history");
    // 路由里没给季：打开订阅的那一季
    await vi.waitFor(() => expect(api.itemSeason).toHaveBeenCalledWith(100088, 2));
    q("mi-tab-1")!.click();
    await vi.waitFor(() => expect(api.itemSeason).toHaveBeenLastCalledWith(100088, 1));
    await vi.waitFor(() => expect(router.currentRoute.value.query.season).toBe("1"));
    expect(router.currentRoute.value.query.from).toBe("history");
    await flush();
    expect(q("mi-ep-1")!.textContent).toContain("第一集");
    expect(qa("mi-trow")).toHaveLength(1);
    expect(qa("mi-trow")[0]!.textContent).toContain("S01 整季");
  });

  it("没订阅：「追剧」打开订阅弹窗；更多里能手动整理到这个条目；推荐点了进那一部，面包屑还是进来的那一页", async () => {
    api.item.mockResolvedValue({
      ...tlou,
      subscriptions: [],
      nas: { ...tlou.nas, library: { seasons: [], files: 0, size: 0, libraries: [] } },
    });
    api.itemSeason.mockResolvedValue({ ...season2, subscription: undefined });
    await mountAt("/media/item/tv/100088?from=history");
    await flush();
    expect(q("mi-sub")!.textContent).toContain("追剧");
    expect(q("mi-rescrape"), "没有整理记录时不显示重新刮削").toBeNull();
    expect(q("mi-nas-subscription")!.textContent).toContain("没有追剧");
    expect(q("mi-nas-library")!.textContent).toContain("库里没有");
    q("mi-sub")!.click();
    await vi.waitFor(() => expect(q("subscribe-dialog")).not.toBeNull());

    q("mi-more")!.click();
    await vi.waitFor(() => expect(q("mi-organize")).not.toBeNull());
    q("mi-organize")!.click();
    await vi.waitFor(() => expect(q("organize-dialog")).not.toBeNull());
    expect(q("organize-source"), "填源路径；条目已经定好").not.toBeNull();

    q("mi-rec-9")!.click();
    await vi.waitFor(() => expect(router.currentRoute.value.params.id).toBe("9"));
    expect(router.currentRoute.value.query.from, "面包屑还是从整理历史进来的").toBe("history");
    await vi.waitFor(() => expect(api.item).toHaveBeenLastCalledWith("tv", 9));
  });

  it("桌面上简介点了不盖一层，悬停看全文", async () => {
    api.item.mockResolvedValue(tlou);
    api.itemSeason.mockResolvedValue(season2);
    await mountAt("/media/item/tv/100088");
    const overview = q("mi-overview")!;
    expect(overview.getAttribute("role")).toBeNull();
    expect(overview.getAttribute("title")).toBe("十年前的一桩旧案在暴雨夜重新浮出水面。");
    overview.click();
    await flush();
    expect(q("mi-overview")!.classList.contains("is-open")).toBe(false);
  });

  it("重新刮削调整理记录的刮削", async () => {
    api.item.mockResolvedValue(tlou);
    api.itemSeason.mockResolvedValue(season2);
    organize.rescrape.mockResolvedValue({ records: 3, written: 6 });
    await mountAt("/media/item/tv/100088");
    q("mi-rescrape")!.click();
    await vi.waitFor(() => expect(organize.rescrape).toHaveBeenCalledWith(77));
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalled());
  });

  it("电影：没有季与集，种子块叫「这部片的种子」，没有种子时写明", async () => {
    api.item.mockResolvedValue({
      ...tlou,
      item: {
        ...tlou.item,
        media_type: "movie",
        tmdb_id: 693134,
        title: "沙丘2",
        tvdb_id: undefined,
        runtime: 166,
      },
      seasons: [],
      subscriptions: [],
      torrents: [],
      recommendations: [],
      errors: ["推荐没取到：TMDB 超时"],
    });
    await mountAt("/media/item/movie/693134?from=explore");
    expect(api.itemSeason).not.toHaveBeenCalled();
    expect(q("mi-seasons")).toBeNull();
    expect(q("mi-torrents")!.textContent).toContain("这部片的种子");
    expect(q("mi-torrents-empty")!.textContent).toContain("这部片没有下载过的种子");
    expect(q("mi-recs"), "没有推荐时整块不显示").toBeNull();
    expect(q("mi-errors")!.textContent).toContain("推荐没取到：TMDB 超时");
    expect(q("mi-sub")!.textContent).toContain("订阅");
    expect(q("mi-hero")!.textContent).toContain("2 小时 46 分");
  });

  it("读不到时写明原因，能重试", async () => {
    api.item.mockRejectedValueOnce(new Error("TMDB 上没有这个条目"));
    await mountAt("/media/item/tv/424242");
    await vi.waitFor(() => expect(q("mi-state")!.textContent).toContain("TMDB 上没有这个条目"));
    api.item.mockResolvedValue(tlou);
    api.itemSeason.mockResolvedValue(season2);
    q("mi-reload")!.click();
    await vi.waitFor(() => expect(q("mi-title")).not.toBeNull());
  });

  it("手机：你的 NAS 上是白卡，种子行点了从底部弹出操作", async () => {
    api.item.mockResolvedValue(tlou);
    api.itemSeason.mockResolvedValue(season2);
    await mountAt("/media/item/tv/100088", { mobile: true });
    await flush();
    expect(q("mi-nas")!.classList.contains("mi-nas--card")).toBe(true);
    expect(q("mi-imdb"), "手机不放 IMDb").toBeNull();
    expect(document.querySelector("#pt-mhead-sub")!.textContent).toContain("剧集 · 2025 · 追剧中");
    qa("mi-trow")[0]!.click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("在下载器里打开"));

    // 简介三行省略：点了展开全文，再点收起
    const overview = q("mi-overview")!;
    expect(overview.getAttribute("aria-expanded")).toBe("false");
    overview.click();
    await vi.waitFor(() => expect(q("mi-overview")!.classList.contains("is-open")).toBe(true));
    expect(q("mi-overview")!.getAttribute("aria-expanded")).toBe("true");
    q("mi-overview")!.click();
    await vi.waitFor(() => expect(q("mi-overview")!.classList.contains("is-open")).toBe(false));
  });

  it("手机：开头连续入库超过 3 集收成一行，展开以后换成「收起」", async () => {
    const lib = (n: number) => ({
      episode: n,
      name: `第 ${n} 集`,
      air_date: "2026-10-01",
      state: "library",
    });
    api.item.mockResolvedValue(tlou);
    api.itemSeason.mockResolvedValue({
      ...season2,
      episodes: [lib(1), lib(2), lib(3), lib(4), season2.episodes[3]],
    });
    await mountAt("/media/item/tv/100088?season=2", { mobile: true });
    await vi.waitFor(() => expect(q("mi-collapse")).not.toBeNull());
    expect(q("mi-collapse")!.textContent).toContain("E01–E04 · 4 集都已入库");
    expect(q("mi-expand")!.textContent).toContain("展开");
    expect(q("mi-ep-1"), "收起的集不列").toBeNull();
    expect(q("mi-ep-9")).not.toBeNull();
    q("mi-expand")!.click();
    await vi.waitFor(() => expect(q("mi-ep-1")).not.toBeNull());
    expect(q("mi-expand")!.textContent).toContain("收起");
    q("mi-expand")!.click();
    await vi.waitFor(() => expect(q("mi-ep-1")).toBeNull());
  });
});
