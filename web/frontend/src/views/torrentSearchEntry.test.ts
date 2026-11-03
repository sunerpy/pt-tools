// @vitest-environment happy-dom
/*
 * 按条目搜索（媒体中心 M2a，画板 06、09、10）：打字给 TMDB 候选，选了按条目搜；分组卡片的折叠、展开、改选站点推送；
 * 筛选计数（同组并集、组间交集、勾一个其他组的数跟着变）；按发布时间排；部分站点没回应；去掉条目回到关键字表格。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import type { MediaSearchMember, MediaSearchResult } from "@/api";
import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  multiSite: vi.fn(),
  push: vi.fn(),
  explore: vi.fn(),
  searchMedia: vi.fn(),
  profiles: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    searchApi: {
      getSites: vi.fn(() => Promise.resolve(["hdsky", "ourbits"])),
      multiSite: api.multiSite,
      clearCache: vi.fn(),
    },
    downloadersApi: {
      list: vi.fn(() =>
        Promise.resolve([
          { id: 1, name: "qb", type: "qbittorrent", is_default: true, enabled: true },
        ]),
      ),
    },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    siteCategoriesApi: { getAll: vi.fn(() => Promise.resolve({})) },
    torrentPushApi: { push: api.push, batchPush: vi.fn() },
    mediaApi: { ...real.mediaApi, settings: vi.fn(() => Promise.resolve({ image_base_url: "" })) },
    subscribeApi: {
      ...real.subscribeApi,
      explore: api.explore,
      searchMedia: api.searchMedia,
      profiles: api.profiles,
    },
    // 等免费与黑名单（M2b）另有 torrentSearchBlockWatch.test.ts；这里只给空的，不发真请求
    freeWatchApi: { ...real.freeWatchApi, sites: vi.fn(() => Promise.resolve({ sites: [] })) },
  };
});

let view: MountedView | null = null;

function viewport(kind: "wide" | "mobile") {
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches:
          kind === "mobile"
            ? query.includes("max-width: 768px")
            : query.includes("min-width: 1280px"),
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      }) as MediaQueryList,
  );
}

const member = (
  site: string,
  id: string,
  over: Partial<MediaSearchMember> = {},
): MediaSearchMember => ({
  site,
  torrent_id: id,
  title: `Chang.Ye.Jiang.Jin.S02E08.2160p.WEB-DL.H265-${site}`,
  subtitle: "长夜将尽 第二季 第08集",
  size_bytes: 4.71 * 1024 ** 3,
  seeders: 10,
  leechers: 0,
  discount: "NONE",
  free: false,
  hr: false,
  download_url: `/api/site/${site}/torrent/${id}/download`,
  meta: { resolution: "2160p", source: "WEB-DL", group: "CHDWEB", season: 2, episode: 8 },
  verdict: { score: 88 },
  ...over,
});

const searchResult = (): MediaSearchResult => ({
  item: {
    media_type: "tv",
    tmdb_id: 231620,
    season: 2,
    title: "长夜将尽",
    year: 2025,
    seasons: 2,
    total_episodes: 8,
  },
  subscription: { id: 3, status: "active", lock: { group: "HHWEB", resolution: "2160p" } },
  profile: { id: 1, name: "默认" },
  keywords: ["Chang Ye Jiang Jin S02", "长夜将尽"],
  episodes: [
    ...Array.from({ length: 6 }, (_, i) => ({ episode: i + 1, state: "library" as const })),
    { episode: 7, state: "downloading", site: "HDSky" },
    { episode: 8, state: "waiting_version" },
  ],
  groups: [
    {
      group_id: "h:aaa",
      members: [
        member("OurBits", "1", {
          seeders: 41,
          free: true,
          discount: "FREE",
          free_left_seconds: 18 * 3600,
          uploaded_at: 100,
        }),
        member("CHDBits", "2", {
          seeders: 23,
          discount: "PERCENT_50",
          hr: true,
          hr_hours: 120,
          uploaded_at: 90,
        }),
      ],
      meta: { resolution: "2160p", source: "WEB-DL", group: "CHDWEB" },
      verdict: { score: 88 },
      span: { start: 8, end: 8 },
      covers: [8],
      downloading: [],
      in_library: false,
    },
    {
      group_id: "s:pttime:3",
      members: [
        member("PTTime", "3", {
          title: "The.Long.Night.S02E08.1080p.WEB-DL.H264-ADWeb",
          seeders: 212,
          uploaded_at: 300,
          meta: { resolution: "1080p", source: "WEB-DL", group: "ADWeb", chinese_subs: true },
          verdict: { score: 0, reject: "分辨率 1080p 不在档案里" },
        }),
      ],
      meta: { resolution: "1080p", source: "WEB-DL", group: "ADWeb", chinese_subs: true },
      verdict: { score: 0, reject: "分辨率 1080p 不在档案里" },
      span: { start: 8, end: 8 },
      covers: [8],
      downloading: [],
      in_library: false,
    },
    {
      group_id: "h:bbb",
      members: [
        member("HDSky", "4", {
          title: "Chang.Ye.Jiang.Jin.S02E01-E06.2160p.WEB-DL.H265-HHWEB",
          uploaded_at: 500,
          verdict: { score: 92 },
        }),
      ],
      meta: { resolution: "2160p", group: "HHWEB" },
      verdict: { score: 92 },
      span: { start: 1, end: 6 },
      covers: [],
      downloading: [],
      in_library: true,
    },
  ],
  total: 4,
  blocked_count: 0,
  unmatched_count: 6,
  errors: [{ site: "TTG", error: "请求超时" }],
});

const explorePage = (kind: "tv" | "movie") => ({
  items:
    kind === "tv"
      ? [
          {
            id: 231620,
            media_type: "tv",
            title: "长夜将尽",
            original_title: "Chang Ye Jiang Jin",
            year: 2025,
            countries: ["CN"],
            in_library: false,
            subscribed: true,
          },
        ]
      : [
          {
            id: 77,
            media_type: "movie",
            title: "长夜行",
            original_title: "",
            year: 2023,
            genre_ids: [18],
            in_library: false,
            subscribed: false,
          },
        ],
  page: 1,
  total_pages: 1,
});

async function mountPage(path?: string) {
  api.explore.mockImplementation((q: { kind: "tv" | "movie" }) =>
    Promise.resolve(explorePage(q.kind)),
  );
  api.searchMedia.mockResolvedValue(searchResult());
  api.profiles.mockResolvedValue({
    items: [
      { id: 1, name: "默认" },
      { id: 2, name: "只要 1080p" },
    ],
    resolutions: [],
    sources: [],
    codecs: [],
  });
  api.multiSite.mockResolvedValue({
    items: [],
    totalResults: 0,
    siteResults: {},
    errors: [],
    durationMs: 5,
  });
  const TorrentSearch = (await import("./TorrentSearch.vue")).default;
  let router;
  if (path) {
    router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/:p(.*)*", component: { render: () => null } }],
    });
    await router.push(path);
    await router.isReady();
  }
  view = mountView(TorrentSearch, { router });
  const sel = path ? ".es-q input" : "input[placeholder='输入关键词，回车即搜']";
  await vi.waitFor(() => expect(document.querySelector(sel)).not.toBeNull());
  return document.querySelector<HTMLInputElement>(sel)!;
}

async function type(input: HTMLInputElement, text: string) {
  input.dispatchEvent(new FocusEvent("focus"));
  input.value = text;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await Promise.resolve();
}

/** 打字、等候选、点「长夜将尽」：按条目搜。el-input 只在字变了时发 input，所以打的字要和框里现有的不同 */
async function pickEntry(input: HTMLInputElement, text = "长夜") {
  await type(input, text);
  await vi.waitFor(
    () => expect(document.querySelector("[data-testid='es-cand-tv-231620']")).not.toBeNull(),
    { timeout: 3000 },
  );
  document.querySelector<HTMLButtonElement>("[data-testid='es-cand-tv-231620']")!.click();
  await vi.waitFor(() =>
    expect(document.querySelector("[data-testid='es-summary']")).not.toBeNull(),
  );
}

const cardKeys = () =>
  [...document.querySelectorAll("[data-testid^='es-card-']")].map((el) =>
    el.getAttribute("data-testid"),
  );
const facetCount = (key: string, value: string) =>
  document
    .querySelector(`[data-testid='es-facet-${key}-${value}'] .es-facets__n`)
    ?.textContent?.trim();

afterEach(() => {
  view?.unmount();
  view = null;
  sessionStorage.clear();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("按条目搜索", () => {
  it("打字时给候选；选了候选按条目搜，输入框里是条目标记", async () => {
    viewport("wide");
    const input = await mountPage();
    await pickEntry(input);
    expect(api.explore).toHaveBeenCalledWith({ kind: "tv", list: "search", q: "长夜" });
    expect(api.explore).toHaveBeenCalledWith({ kind: "movie", list: "search", q: "长夜" });
    expect(api.searchMedia).toHaveBeenCalledWith({
      media_type: "tv",
      tmdb_id: 231620,
      season: 0,
      profile_id: 0,
      include_unmatched: false,
    });
    expect(api.multiSite).not.toHaveBeenCalled();
    expect(document.querySelector("[data-testid='es-token']")?.textContent).toContain(
      "长夜将尽 · 2025 · 剧集",
    );
    expect(document.querySelector("[data-testid='es-summary']")?.textContent).toBe(
      "4 条结果 · 合并成 3 组 · 符合档案 2",
    );
    expect(document.querySelector("[data-testid='es-ep-note']")?.textContent).toBe(
      "缺 E08（等版本）· E07 下载中",
    );
    expect(document.querySelector("[data-testid='es-site-errors']")?.textContent).toBe(
      "1 个站点没回应",
    );
    document
      .querySelector("[data-testid='es-site-errors']")!
      .dispatchEvent(new MouseEvent("mouseenter"));
    await vi.waitFor(() => expect(document.body.textContent).toContain("TTG：请求超时"));
    expect(document.querySelector("[data-testid='es-unmatched-line']")?.textContent).toContain(
      "6 条没显示",
    );
    // 订阅锁着 HHWEB，这一组是 CHDWEB
    expect(document.querySelector("[data-testid='es-card-h:aaa']")?.textContent).toContain(
      "不是锁定的版本（HHWEB）",
    );
  });

  it("直接回车仍按关键字搜", async () => {
    viewport("wide");
    const input = await mountPage();
    await type(input, "长夜");
    input.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
    await vi.waitFor(() => expect(api.multiSite).toHaveBeenCalledTimes(1));
    expect(api.searchMedia).not.toHaveBeenCalled();
  });

  it("分组卡片：默认只显示首选，展开以后改选站点推送，推送带上条目", async () => {
    viewport("wide");
    api.push.mockResolvedValue({
      success: true,
      results: [{ downloaderId: 1, downloaderName: "qb", success: true }],
    });
    const input = await mountPage();
    await pickEntry(input);
    const card = () => document.querySelector("[data-testid='es-card-h:aaa']")!;
    expect(card().querySelectorAll("[data-testid='es-member']")).toHaveLength(1);
    expect(card().querySelector("[data-testid='es-more']")?.textContent).toContain(
      "另有 1 个站点：CHDBits",
    );
    card().querySelector<HTMLButtonElement>("[data-testid='es-more']")!.click();
    await vi.waitFor(() =>
      expect(card().querySelectorAll("[data-testid='es-member']")).toHaveLength(2),
    );
    expect(card().textContent).toContain("H&R 120 时");
    card().querySelector<HTMLButtonElement>("[data-testid='es-push-CHDBits-2']")!.click();
    await vi.waitFor(() => expect(document.querySelector(".el-dialog")).not.toBeNull());
    buttonByText("推送", document.querySelector(".el-dialog")!).click();
    await vi.waitFor(() => expect(api.push).toHaveBeenCalledTimes(1));
    expect(api.push.mock.calls[0]![0]).toMatchObject({
      downloadUrl: "/api/site/CHDBits/torrent/2/download",
      sourceSite: "CHDBits",
      mediaType: "tv",
      tmdbId: 231620,
      season: 2,
    });
  });

  it("整组库里都有的卡默认折叠，点了展开", async () => {
    viewport("wide");
    const input = await mountPage();
    await pickEntry(input);
    const card = () => document.querySelector<HTMLElement>("[data-testid='es-card-h:bbb']")!;
    expect(card().textContent).toContain("这一组的集都已入库，默认折叠");
    expect(card().querySelectorAll("[data-testid='es-member']")).toHaveLength(0);
    card().click();
    await vi.waitFor(() =>
      expect(card().querySelectorAll("[data-testid='es-member']")).toHaveLength(1),
    );
  });

  it("筛选：勾一个选项，别的组的数跟着变、同组的不变，卡片跟着少", async () => {
    viewport("wide");
    const input = await mountPage();
    await pickEntry(input);
    expect(facetCount("resolution", "2160p")).toBe("3");
    expect(facetCount("resolution", "1080p")).toBe("1");
    expect(facetCount("flags", "subs")).toBe("1");
    document.querySelector<HTMLButtonElement>("[data-testid='es-facet-resolution-1080p']")!.click();
    await vi.waitFor(() => expect(cardKeys()).toEqual(["es-card-s:pttime:3"]));
    expect(facetCount("resolution", "2160p"), "同组不受自己的筛选影响").toBe("3");
    expect(facetCount("group", "CHDWEB"), "别的组跟着变").toBe("0");
    expect(facetCount("group", "ADWeb")).toBe("1");
    expect(document.querySelector("[data-testid='es-summary']")?.textContent).toBe(
      "1 条结果 · 合并成 1 组 · 符合档案 0",
    );
    // 同组再勾一个是并集
    document.querySelector<HTMLButtonElement>("[data-testid='es-facet-resolution-2160p']")!.click();
    await vi.waitFor(() => expect(cardKeys()).toHaveLength(3));
  });

  it("排序：档案分按服务端的顺序，做种、发布时间按组里最好的成员", async () => {
    viewport("wide");
    const input = await mountPage();
    await pickEntry(input);
    expect(cardKeys()).toEqual(["es-card-h:aaa", "es-card-s:pttime:3", "es-card-h:bbb"]);
    document.querySelector<HTMLButtonElement>("[data-testid='es-sort-seeders']")!.click();
    await vi.waitFor(() =>
      expect(cardKeys()).toEqual(["es-card-s:pttime:3", "es-card-h:aaa", "es-card-h:bbb"]),
    );
    document.querySelector<HTMLButtonElement>("[data-testid='es-sort-uploaded']")!.click();
    await vi.waitFor(() =>
      expect(cardKeys()).toEqual(["es-card-h:bbb", "es-card-s:pttime:3", "es-card-h:aaa"]),
    );
  });

  it("换档案、换季都重新搜", async () => {
    viewport("wide");
    const input = await mountPage();
    await pickEntry(input);
    document.querySelector<HTMLElement>("[data-testid='es-profile']")!.click();
    await new Promise((r) => setTimeout(r, 0));
    const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
      (el) => el.textContent?.trim() === "档案：只要 1080p",
    )!;
    opt.click();
    await vi.waitFor(() => expect(api.searchMedia).toHaveBeenCalledTimes(2));
    expect(api.searchMedia.mock.calls[1]![0]).toMatchObject({ profile_id: 2, season: 0 });
    document.querySelector<HTMLButtonElement>("[data-testid='es-season-1']")!.click();
    await vi.waitFor(() => expect(api.searchMedia).toHaveBeenCalledTimes(3));
    // 换季不清档案，只清点的那一集
    expect(api.searchMedia.mock.calls[2]![0]).toMatchObject({ profile_id: 2, season: 1 });
  });

  it("去掉条目标记回到关键字搜索", async () => {
    viewport("wide");
    const input = await mountPage();
    await pickEntry(input);
    document.querySelector<HTMLButtonElement>("[data-testid='es-token-x']")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector("input[placeholder='输入关键词，回车即搜']")).not.toBeNull(),
    );
    expect(document.querySelector("[data-testid='es']")).toBeNull();
    // 关键字的工具栏（带「筛选」浮层）回来了
    expect(document.querySelector("[data-testid='search-facets-btn']")).not.toBeNull();
  });

  it.each([
    ["wide", "es-detail"],
    ["mobile", "es-mdetail"],
  ] as const)("%s：「条目详情」打开这一季的详情，面包屑写从种子搜索进来", async (kind, id) => {
    viewport(kind);
    await mountPage("/search?media_type=tv&tmdb_id=231620&season=2");
    await vi.waitFor(() => expect(document.querySelector(`[data-testid='${id}']`)).not.toBeNull());
    expect(document.querySelector(`[data-testid='${id}']`)!.getAttribute("href")).toBe(
      "/media/item/tv/231620?from=search&season=2",
    );
  });

  it("从探索页带着条目进来：清掉缓存里上一次的关键词，不拿它筛条目的结果", async () => {
    viewport("wide");
    sessionStorage.setItem(
      "pt-tools-search-cache",
      JSON.stringify({ keyword: "Dune", results: [], timestamp: Date.now() }),
    );
    const input = await mountPage("/search?media_type=tv&tmdb_id=231620");
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid='es-summary']")).not.toBeNull(),
    );
    // 页面挂载时先等站点、下载器、分类三个请求，再从缓存恢复：等它们都回来，否则断言跑在恢复之前
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid='es-token']")?.textContent).toContain("长夜将尽"),
    );
    await new Promise((r) => setTimeout(r, 200));
    expect(input.value).toBe("");
    expect(cardKeys()).toHaveLength(3);
    // 去掉条目回到关键字模式时，缓存的关键词写回输入框
    document.querySelector<HTMLButtonElement>("[data-testid='es-token-x']")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector<HTMLInputElement>(".es-q input")!.value).toBe("Dune"),
    );
    expect(api.searchMedia).toHaveBeenCalledWith(
      expect.objectContaining({ media_type: "tv", tmdb_id: 231620, season: 0 }),
    );
  });

  it("页面开着时地址换成一个条目（例如浏览器后退）：输入框里没搜的字清掉", async () => {
    viewport("wide");
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/:p(.*)*", component: { render: () => null } }],
    });
    await router.push("/search");
    await router.isReady();
    api.searchMedia.mockResolvedValue(searchResult());
    api.profiles.mockResolvedValue({ items: [], resolutions: [], sources: [], codecs: [] });
    const TorrentSearch = (await import("./TorrentSearch.vue")).default;
    view = mountView(TorrentSearch, { router });
    const sel = "input[placeholder='输入关键词，回车即搜']";
    await vi.waitFor(() => expect(document.querySelector(sel)).not.toBeNull());
    const input = document.querySelector<HTMLInputElement>(sel)!;
    input.value = "Dune";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    await router.push("/search?media_type=tv&tmdb_id=231620");
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid='es-summary']")).not.toBeNull(),
    );
    expect(document.querySelector<HTMLInputElement>(".es-q input")!.value).toBe("");
    expect(cardKeys()).toHaveLength(3);
  });

  it("去掉条目：输入框写回表格那次的关键词，这不算打字，不给候选", async () => {
    viewport("wide");
    const input = await mountPage();
    await type(input, "长夜");
    input.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
    await vi.waitFor(() => expect(api.multiSite).toHaveBeenCalledTimes(1));
    await pickEntry(input, "长夜将");
    expect(document.querySelector<HTMLInputElement>(".es-q input")!.value).toBe("");
    const asked = api.explore.mock.calls.length;
    document.querySelector<HTMLButtonElement>("[data-testid='es-token-x']")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector<HTMLInputElement>(".es-q input")!.value).toBe("长夜"),
    );
    await new Promise((r) => setTimeout(r, 700));
    expect(api.explore.mock.calls.length, "程序写回的关键词不该触发候选").toBe(asked);
    expect(document.querySelector("[data-testid='es-candidates']")).toBeNull();
  });

  it("没对上的点了显示出来（带 include_unmatched 再搜一次）", async () => {
    viewport("wide");
    const input = await mountPage();
    await pickEntry(input);
    api.searchMedia.mockResolvedValue({
      ...searchResult(),
      unmatched: [
        member("HDSky", "9", { title: "Some.Other.Show.S02E08", reason: "名字或季对不上这个条目" }),
      ],
    });
    document.querySelector<HTMLButtonElement>("[data-testid='es-show-unmatched']")!.click();
    await vi.waitFor(() => expect(api.searchMedia).toHaveBeenCalledTimes(2));
    expect(api.searchMedia.mock.calls[1]![0]).toMatchObject({ include_unmatched: true });
    await vi.waitFor(() => expect(document.body.textContent).toContain("名字或季对不上这个条目"));
  });
});

describe("关键字搜索的解析标签与「筛选」浮层", () => {
  const hit = (id: string, title: string, meta: Record<string, unknown>) => ({
    id,
    title,
    subtitle: "",
    sourceSite: "hdsky",
    sizeBytes: 4 * 1024 ** 3,
    seeders: 10,
    leechers: 0,
    snatched: 0,
    isFree: false,
    discountLevel: "NONE",
    downloadUrl: `/api/site/hdsky/torrent/${id}/download`,
    meta,
  });

  it("标题下写解析标签；勾一个分辨率，表格只剩那几行，同组的数不变、别的组跟着变", async () => {
    viewport("wide");
    const input = await mountPage();
    api.multiSite.mockResolvedValue({
      items: [
        hit("1", "Show.S01E01.2160p.WEB-DL.H265-CHDWEB", {
          resolution: "2160p",
          source: "WEB-DL",
          video_codec: "H.265",
          group: "CHDWEB",
        }),
        hit("2", "Show.S01E01.2160p.WEB-DL.H265-HHWEB", {
          resolution: "2160p",
          source: "WEB-DL",
          video_codec: "H.265",
          group: "HHWEB",
        }),
        hit("3", "Show.S01E01.1080p.WEB-DL.H264-ADWeb", {
          resolution: "1080p",
          source: "WEB-DL",
          video_codec: "H.264",
          group: "ADWeb",
          chinese_subs: true,
        }),
      ],
      totalResults: 3,
      siteResults: { hdsky: 3 },
      errors: [],
      durationMs: 5,
    });
    await type(input, "Show");
    input.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
    const rows = () => [...document.querySelectorAll(".el-table__body tbody tr")];
    await vi.waitFor(() => expect(rows()).toHaveLength(3));
    expect([...rows()[2]!.querySelectorAll(".pt-tag")].map((t) => t.textContent?.trim())).toEqual(
      expect.arrayContaining(["1080p", "WEB-DL", "H.264", "ADWeb", "中字"]),
    );
    document.querySelector<HTMLButtonElement>("[data-testid='search-facets-btn']")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid='es-facet-resolution-1080p']")).not.toBeNull(),
    );
    expect(facetCount("resolution", "2160p")).toBe("2");
    document.querySelector<HTMLButtonElement>("[data-testid='es-facet-resolution-1080p']")!.click();
    await vi.waitFor(() => expect(rows()).toHaveLength(1));
    expect(rows()[0]!.textContent).toContain("ADWeb");
    expect(facetCount("resolution", "2160p"), "同组不受自己的筛选影响").toBe("2");
    expect(facetCount("group", "CHDWEB"), "别的组跟着变").toBe("0");
    expect(document.querySelector("[data-testid='search-facets-btn']")?.textContent).toContain(
      "筛选 · 1",
    );
    // 重新搜一次，筛选清掉
    input.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
    await vi.waitFor(() => expect(rows()).toHaveLength(3));
    expect(document.querySelector("[data-testid='search-facets-btn']")?.textContent?.trim()).toBe(
      "筛选",
    );
  });
});

describe("按条目搜索 · 手机", () => {
  it("筛选面板里的选择点「看 N 条结果」才生效", async () => {
    viewport("mobile");
    const input = await mountPage();
    await pickEntry(input);
    expect(document.querySelectorAll("[data-testid^='es-mcard-']")).toHaveLength(3);
    document.querySelector<HTMLButtonElement>("[data-testid='es-mfilter']")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid='es-mfacet-resolution']")).not.toBeNull(),
    );
    const pill = [
      ...document.querySelectorAll<HTMLButtonElement>(
        "[data-testid='es-mfacet-resolution'] button",
      ),
    ].find((b) => b.textContent?.includes("1080p"))!;
    pill.click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid='es-mpanel-apply']")?.textContent).toContain(
        "看 1 条结果",
      ),
    );
    expect(document.querySelectorAll("[data-testid^='es-mcard-']"), "还没生效").toHaveLength(3);
    document.querySelector<HTMLButtonElement>("[data-testid='es-mpanel-apply']")!.click();
    await vi.waitFor(() =>
      expect(document.querySelectorAll("[data-testid^='es-mcard-']")).toHaveLength(1),
    );
    expect(document.querySelector("[data-testid='es-mfilter']")?.textContent).toContain("筛选 · 1");
  });
});
