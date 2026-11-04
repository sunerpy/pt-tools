// @vitest-environment happy-dom
/*
 * 钉子：条目详情页（媒体中心 M3）。打开时读条目与路由里的季；大图写资料、评分、IMDb；「你的 NAS 上」四行（第 4 行是媒体服务器）；
 * 季 tab 换季重新读、路由跟着换；集卡的状态与说明、缺的集点「搜资源」去按条目搜索；种子行与能不能删；
 * 演职员导演在前；推荐点了进那一部；没订阅时「追剧」打开订阅弹窗；读不到时能重试；电影没有季、种子块叫「这部片的种子」。
 * 媒体服务器（M6）：第 4 行、「在 Jellyfin 播放」（手机在白卡底部）、集卡的播放圆；服务器里删了的能忽略。
 * 删除（M7）：大图的「删除」（手机在「更多」里）、种子行的「删除这个种子」、问题的「处理」打开删除弹窗，删完重新读；
 * 库里的文件不见了写在库里那一行与集卡上；种子块的说明写开着的保护规则。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  item: vi.fn(),
  itemSeason: vi.fn(),
  setStatus: vi.fn(),
  addBlocks: vi.fn(),
  deletePreview: vi.fn(),
  deleteItem: vi.fn(),
  profiles: vi.fn(async () => ({ items: [], resolutions: [], sources: [], codecs: [] })),
  get: vi.fn(),
  episodes: vi.fn(async () => []),
  runs: vi.fn(async () => ({ items: [], total: 0 })),
}));
const organize = vi.hoisted(() => ({
  rescrape: vi.fn(),
  libraries: vi.fn(async () => []),
  preview: vi.fn(),
  dismissIssue: vi.fn(async () => ({})),
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
      missing: [],
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
  protect_rules: ["下载中", "最短保种 24 小时", "H&R", "保留与保护标签"],
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
  it("首页的「看版本」带着 ?episode=N 进来：这一季读完滚到那一集，折起来的先展开，只滚一次", async () => {
    const scrolled: string[] = [];
    const orig = Element.prototype.scrollIntoView;
    Element.prototype.scrollIntoView = function (this: Element) {
      scrolled.push((this as HTMLElement).dataset.testid ?? "");
    };
    try {
      api.item.mockResolvedValue(tlou);
      api.itemSeason.mockResolvedValue(season2);
      await mountAt("/media/item/tv/100088?from=home&season=2&episode=8");
      await vi.waitFor(() => expect(scrolled).toEqual(["mi-ep-8"]));

      // 手机上开头 6 集都已入库会折起来：要滚到的那一集在折起来的那段里时先展开
      view?.unmount();
      scrolled.length = 0;
      const lib = (n: number) => ({
        episode: n,
        name: `第 ${n} 集`,
        air_date: "2026-10-01",
        state: "library",
      });
      api.itemSeason.mockResolvedValue({
        ...season2,
        episodes: [1, 2, 3, 4, 5, 6].map(lib).concat([season2.episodes[3]]),
      });
      await mountAt("/media/item/tv/100088?from=home&season=2&episode=2", { mobile: true });
      await vi.waitFor(() => expect(scrolled).toEqual(["mi-ep-2"]));
      expect(q("mi-ep-2")).not.toBeNull();

      // 没有这一集时不滚
      view?.unmount();
      scrolled.length = 0;
      api.itemSeason.mockResolvedValue(season2);
      await mountAt("/media/item/tv/100088?from=home&season=2&episode=99");
      await vi.waitFor(() => expect(q("mi-ep-1")).not.toBeNull());
      await flush();
      expect(scrolled).toEqual([]);
    } finally {
      Element.prototype.scrollIntoView = orig;
    }
  });

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
    expect(q("mi-ep-versions-8")!.textContent).toContain("看版本");
    expect(q("mi-ep-10")!.textContent).toContain("播出后 2 小时开始搜");
    expect(q("mi-ep-10")!.textContent).toContain("季终");

    const rows = qa("mi-trow");
    expect(rows, "第 2 季的两个").toHaveLength(2);
    expect(rows[0]!.textContent).toContain("S02E01–E04 · 2160p HHWEB · 27.3 GB");
    expect(rows[0]!.textContent).toContain("H&R 已满足");
    expect(rows[1]!.textContent).toContain("受保护：下载中");
    expect(rows[1]!.textContent).toContain("63% · 还要 12 分钟");
    expect(q("mi-torrents-sub")!.textContent).toBe(
      "删除时按这几条保护：下载中、最短保种 24 小时、H&R、保留与保护标签",
    );

    const cast = q("mi-cast")!.textContent ?? "";
    expect(cast.indexOf("陈默")).toBeLessThan(cast.indexOf("周屿"));
    expect(cast).toContain("周远");
    expect(q("mi-cast-all")!.textContent).toContain("全部 2 位");
    expect(q("mi-rec-9")!.textContent).toContain("已入库");
    expect(q("mi-rec-10")!.textContent).toContain("已订阅");

    // 缺的集点「搜资源」：按条目搜索这一季
    q("mi-ep-search-9")!.click();
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

  it("媒体服务器：第 4 行、在 Jellyfin 播放、集卡的播放圆；服务器里删了的能忽略", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const jf = {
      server_id: 1,
      name: "Jellyfin",
      kind: "jellyfin",
      synced_at: "2026-11-04T06:00:00Z",
      have: true,
      episodes: 17,
      seasons: [
        { n: 1, have: 12, total: 12 },
        { n: 2, have: 5, total: 12 },
      ],
      open_url: "https://jf.example.com/web/#/details?id=show",
      removed: [{ id: 31, season: 2, episode: 3 }],
    };
    api.item.mockResolvedValue({ ...tlou, servers: [jf] });
    api.itemSeason.mockResolvedValue({
      ...season2,
      episodes: [
        { ...season2.episodes[0], open_url: "https://jf.example.com/web/#/details?id=e1" },
        {
          episode: 3,
          name: "雨夜",
          air_date: "2026-10-12",
          state: "library",
          version: "2160p HHWEB",
          server_issue: { id: 31, server: "Jellyfin" },
        },
        {
          episode: 4,
          name: "证人",
          air_date: "2026-10-13",
          state: "library",
          source: "server",
          server: "Jellyfin",
        },
        ...season2.episodes.slice(1),
      ],
    });
    await mountAt("/media/item/tv/100088?season=2");
    await vi.waitFor(() => expect(q("mi-ep-3")).not.toBeNull());

    const row = q("mi-nas-server")!;
    expect(row.textContent).toContain("Jellyfin 已入库 17 集");
    expect(row.textContent).toContain("S02E03 在服务器里被删了");
    expect(row.querySelector(".mi-nas__desc")!.classList.contains("is-warn")).toBe(true);
    expect(q("mi-play")!.textContent).toContain("在 Jellyfin 播放");
    q("mi-play")!.click();
    expect(open).toHaveBeenCalledWith(
      "https://jf.example.com/web/#/details?id=show",
      "_blank",
      "noopener",
    );

    // 集卡：入库了、服务器里有的给播放圆（点卡片也播放）；删了的写「Jellyfin 里没有了」、动作是忽略
    expect(q("mi-ep-play-1")!.getAttribute("href")).toBe(
      "https://jf.example.com/web/#/details?id=e1",
    );
    expect(q("mi-ep-play-3"), "服务器里删了的没有播放").toBeNull();
    open.mockClear();
    q("mi-ep-1")!.querySelector<HTMLElement>(".mi-ep__title")!.click();
    expect(open).toHaveBeenCalledWith(
      "https://jf.example.com/web/#/details?id=e1",
      "_blank",
      "noopener",
    );
    expect(q("mi-ep-3")!.textContent).toContain("Jellyfin 里没有了");
    expect(q("mi-ep-3")!.querySelector(".mi-ep__note")!.classList.contains("is-warn")).toBe(true);
    expect(q("mi-ep-4")!.textContent, "算作已入库的写来源").toContain("Jellyfin 里有");
    expect(q("mi-ep-ignore-3")!.textContent).toContain("忽略");
    expect(q("mi-ep-handle-3")!.textContent).toContain("处理");
    q("mi-ep-ignore-3")!.click();
    await vi.waitFor(() => expect(organize.dismissIssue).toHaveBeenCalledWith(31));
    await vi.waitFor(() => expect(api.item).toHaveBeenCalledTimes(2));
    expect(ui.success).toHaveBeenCalledWith("已忽略");

    // 第 4 行的「忽略」：忽略这台服务器上列出的全部
    organize.dismissIssue.mockClear();
    q("mi-issue-ignore-server")!.click();
    await vi.waitFor(() => expect(organize.dismissIssue).toHaveBeenCalledWith(31));
  });

  it("媒体服务器：没有同步过的服务器时不列第 4 行、没有播放；手机的播放在白卡底部", async () => {
    api.item.mockResolvedValue(tlou);
    api.itemSeason.mockResolvedValue(season2);
    await mountAt("/media/item/tv/100088?season=2");
    await vi.waitFor(() => expect(q("mi-ep-1")).not.toBeNull());
    expect(q("mi-nas-server")).toBeNull();
    expect(q("mi-play")).toBeNull();
    expect(q("mi-ep-play-1")).toBeNull();
    view?.unmount();

    const jf = {
      server_id: 1,
      name: "Jellyfin",
      kind: "jellyfin",
      synced_at: "2026-11-04T06:00:00Z",
      have: true,
      episodes: 18,
      open_url: "https://jf.example.com/web/#/details?id=show",
      removed: [],
    };
    api.item.mockResolvedValue({ ...tlou, servers: [jf] });
    await mountAt("/media/item/tv/100088?season=2", { mobile: true });
    await vi.waitFor(() => expect(q("mi-play")).not.toBeNull());
    expect(q("mi-nas")!.contains(q("mi-play")), "手机的播放在「你的 NAS 上」白卡里").toBe(true);
    expect(q("mi-acts")!.contains(q("mi-play"))).toBe(false);
    expect(q("mi-nas-server")!.textContent).toContain("同步");
    expect(q("mi-issue-ignore-server"), "没有删了的不给忽略").toBeNull();
    expect(q("mi-issue-handle-server")).toBeNull();
  });

  it("删除：大图的「删除」打开这一季的删除弹窗，删完重新读；种子行「删除这个种子」只勾那一个", async () => {
    const preview = {
      files: [],
      torrents: [{ ...tlou.torrents[0], covers_more: false, shares: [] }],
      subscriptions: [{ id: 5, status: "active", season: 2, follow: true }],
      issues: [],
      errors: [],
    };
    api.item.mockResolvedValue(tlou);
    api.itemSeason.mockResolvedValue(season2);
    api.deletePreview.mockResolvedValue(preview);
    await mountAt("/media/item/tv/100088?season=2");
    await vi.waitFor(() => expect(q("mi-ep-1")).not.toBeNull());
    expect(q("mi-delete")!.getAttribute("aria-label")).toBe("删除第 2 季");
    q("mi-delete")!.click();
    await vi.waitFor(() => expect(q("delete-torrents")).not.toBeNull());
    expect(api.deletePreview).toHaveBeenCalledWith({
      media_type: "tv",
      tmdb_id: 100088,
      season: 2,
    });
    expect(document.querySelector(".md-dialog .el-dialog__title")!.textContent).toBe(
      "删除《长夜将尽》第 2 季",
    );
    expect(q("delete-summary")!.textContent).toBe("将删除 1 个种子（连数据），暂停追剧");
    api.deleteItem.mockResolvedValue({
      steps: [
        { kind: "subscription", target: "长夜将尽 第 2 季", id: 5, ok: true, message: "暂停了" },
      ],
      failed: 0,
    });
    q("delete-run")!.click();
    await vi.waitFor(() => expect(api.deleteItem).toHaveBeenCalled());
    await vi.waitFor(() => expect(api.item).toHaveBeenCalledTimes(2));
    q("delete-close")!.click();
    await flush();

    // 种子行的更多菜单：删除这个种子，只勾那一个、订阅保留
    api.deletePreview.mockClear();
    qa("mi-trow-more")[0]!.click();
    await vi.waitFor(() => expect(q("mi-trow-delete")).not.toBeNull());
    q("mi-trow-delete")!.click();
    await vi.waitFor(() => expect(api.deletePreview).toHaveBeenCalled());
    await vi.waitFor(() =>
      expect(q("delete-summary")!.textContent).toBe("将删除 1 个种子（连数据）"),
    );
  });

  it("问题：服务器里删了的「处理」只删那一集；库里的文件不见了写在库里那一行与集卡上，能处理、忽略", async () => {
    const jf = {
      server_id: 1,
      name: "Jellyfin",
      kind: "jellyfin",
      synced_at: "2026-11-04T06:00:00Z",
      have: true,
      episodes: 17,
      removed: [{ id: 31, season: 2, episode: 3 }],
    };
    api.item.mockResolvedValue({
      ...tlou,
      servers: [jf],
      nas: {
        ...tlou.nas,
        library: { ...tlou.nas.library, missing: [{ id: 41, season: 2, episode: 5 }] },
      },
    });
    api.itemSeason.mockResolvedValue({
      ...season2,
      episodes: [
        {
          episode: 5,
          name: "回声",
          air_date: "2026-10-09",
          state: "library",
          library_issue: { id: 41 },
        },
        ...season2.episodes,
      ],
    });
    api.deletePreview.mockResolvedValue({
      files: [],
      torrents: [],
      subscriptions: [],
      issues: [],
      errors: [],
    });
    await mountAt("/media/item/tv/100088?season=2");
    await vi.waitFor(() => expect(q("mi-ep-5")).not.toBeNull());

    const lib = q("mi-nas-library")!;
    expect(lib.textContent).toContain("S02E05 的文件不见了");
    expect(lib.querySelector(".mi-nas__desc")!.classList.contains("is-warn")).toBe(true);
    expect(q("mi-ep-5")!.textContent).toContain("库里的文件不见了");
    expect(q("mi-ep-5")!.classList.contains("has-two-acts")).toBe(true);

    q("mi-issue-handle-server")!.click();
    await vi.waitFor(() =>
      expect(api.deletePreview).toHaveBeenCalledWith({
        media_type: "tv",
        tmdb_id: 100088,
        season: 2,
        episodes: [3],
      }),
    );
    await vi.waitFor(() =>
      expect(document.querySelector(".md-dialog .el-dialog__title")!.textContent).toBe(
        "删除《长夜将尽》第 2 季 E03",
      ),
    );

    q("mi-ep-handle-5")!.click();
    await vi.waitFor(() =>
      expect(api.deletePreview).toHaveBeenLastCalledWith({
        media_type: "tv",
        tmdb_id: 100088,
        season: 2,
        episodes: [5],
      }),
    );

    q("mi-issue-ignore-library")!.click();
    await vi.waitFor(() => expect(organize.dismissIssue).toHaveBeenCalledWith(41));
  });

  it("手机：「更多」里有删除，种子的底部菜单里有「删除这个种子」", async () => {
    api.item.mockResolvedValue(tlou);
    api.itemSeason.mockResolvedValue(season2);
    api.deletePreview.mockResolvedValue({
      files: [],
      torrents: [],
      subscriptions: [],
      issues: [],
      errors: [],
    });
    await mountAt("/media/item/tv/100088?season=2", { mobile: true });
    await vi.waitFor(() => expect(q("mi-ep-1")).not.toBeNull());
    expect(q("mi-delete"), "手机的删除在「更多」里").toBeNull();
    q("mi-more")!.click();
    await vi.waitFor(() => expect(q("mi-more-delete")).not.toBeNull());
    expect(q("mi-more-delete")!.textContent!.trim()).toBe("删除第 2 季");
    q("mi-more-delete")!.click();
    await vi.waitFor(() => expect(api.deletePreview).toHaveBeenCalledTimes(1));
    expect(document.querySelector(".md-dialog")!.classList.contains("is-fullscreen")).toBe(true);

    qa("mi-trow")[0]!.click();
    await vi.waitFor(() => expect(q("mi-sheet-delete")).not.toBeNull());
    q("mi-sheet-delete")!.click();
    await vi.waitFor(() => expect(api.deletePreview).toHaveBeenCalledTimes(2));
  });

  it("没有可删的（库里没有、没有种子、没有订阅）时不给删除", async () => {
    api.item.mockResolvedValue({
      ...tlou,
      subscriptions: [],
      torrents: [],
      nas: { ...tlou.nas, library: { seasons: [], files: 0, size: 0, libraries: [], missing: [] } },
    });
    api.itemSeason.mockResolvedValue({ ...season2, subscription: undefined });
    await mountAt("/media/item/tv/100088?season=2");
    await vi.waitFor(() => expect(q("mi-ep-1")).not.toBeNull());
    expect(q("mi-delete")).toBeNull();
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
