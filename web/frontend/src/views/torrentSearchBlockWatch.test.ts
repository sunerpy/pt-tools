// @vitest-environment happy-dom
/*
 * 黑名单与等免费（媒体中心 M2b）：按条目搜索卡片脚注的「不要」「等免费」、拉黑以后默认不显示与最后一行、移出黑名单；
 * 推送弹窗的等免费模式；关键字搜索每行「更多」里的等免费、不要、移出黑名单；手机卡片底部两半。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import type { MediaSearchMember, MediaSearchResult, TorrentBlock } from "@/api";
import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  multiSite: vi.fn(),
  searchMedia: vi.fn(),
  blocks: vi.fn(),
  addBlocks: vi.fn(),
  removeBlock: vi.fn(),
  watchSites: vi.fn(),
  watchCreate: vi.fn(),
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
          { id: 2, name: "tr", type: "transmission", is_default: false, enabled: true },
        ]),
      ),
    },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    siteCategoriesApi: { getAll: vi.fn(() => Promise.resolve({})) },
    torrentPushApi: { push: vi.fn(), batchPush: vi.fn() },
    mediaApi: { ...real.mediaApi, settings: vi.fn(() => Promise.resolve({ image_base_url: "" })) },
    subscribeApi: {
      ...real.subscribeApi,
      explore: vi.fn(() => Promise.resolve({ items: [], page: 1, total_pages: 1 })),
      searchMedia: api.searchMedia,
      profiles: vi.fn(() =>
        Promise.resolve({ items: [], resolutions: [], sources: [], codecs: [] }),
      ),
      blocks: api.blocks,
      addBlocks: api.addBlocks,
      removeBlock: api.removeBlock,
    },
    freeWatchApi: { ...real.freeWatchApi, sites: api.watchSites, create: api.watchCreate },
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
  size_bytes: 4 * 1024 ** 3,
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

/** 两组：OurBits + CHDBits 是同一资源（OurBits 能等免费），PTTime 一组（不能等免费） */
const searchResult = (over: Partial<MediaSearchResult> = {}): MediaSearchResult => ({
  item: { media_type: "tv", tmdb_id: 231620, season: 2, title: "长夜将尽", year: 2025, seasons: 2 },
  subscription: { id: 3, status: "active", lock: null },
  profile: { id: 1, name: "默认" },
  keywords: ["长夜将尽"],
  episodes: [],
  groups: [
    {
      group_id: "h:aaa",
      members: [member("ourbits", "1", { hr: true, hr_hours: 72 }), member("chdbits", "2")],
      meta: { resolution: "2160p", group: "CHDWEB" },
      verdict: { score: 88 },
      covers: [8],
      downloading: [],
      in_library: false,
    },
    {
      group_id: "s:pttime:3",
      members: [
        member("pttime", "3", {
          title: "The.Long.Night.S02E08.1080p.WEB-DL.H264-ADWeb",
          meta: { resolution: "1080p", group: "ADWeb" },
          verdict: { score: 40 },
        }),
      ],
      meta: { resolution: "1080p", group: "ADWeb" },
      verdict: { score: 40 },
      covers: [8],
      downloading: [],
      in_library: false,
    },
  ],
  total: 3,
  blocked_count: 0,
  unmatched_count: 2,
  errors: [],
  ...over,
});

const block = (id: number, site: string, torrentID: string, over: Partial<TorrentBlock> = {}) =>
  ({
    id,
    site_name: site,
    torrent_id: torrentID,
    media_type: "tv",
    tmdb_id: 231620,
    season: 2,
    media_title: "长夜将尽 第 2 季",
    title: "T",
    reason: "",
    created_at: "2026-11-03T08:00:00Z",
    ...over,
  }) as TorrentBlock;

async function mountEntry(result = searchResult()) {
  api.searchMedia.mockResolvedValue(result);
  api.watchSites.mockResolvedValue({ sites: ["OurBits"] });
  api.multiSite.mockResolvedValue({
    items: [],
    totalResults: 0,
    siteResults: {},
    errors: [],
    durationMs: 5,
  });
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:p(.*)*", component: { render: () => null } }],
  });
  await router.push("/search?media_type=tv&tmdb_id=231620");
  await router.isReady();
  const TorrentSearch = (await import("./TorrentSearch.vue")).default;
  view = mountView(TorrentSearch, { router });
  await vi.waitFor(() =>
    expect(document.querySelector("[data-testid='es-summary']")).not.toBeNull(),
  );
  // 能等免费的站点是另一个请求：等它回来，「等免费」才点得了
  await vi.waitFor(() => expect(api.watchSites).toHaveBeenCalled());
  await new Promise((r) => setTimeout(r, 0));
}

const card = (key: string) => document.querySelector<HTMLElement>(`[data-testid='es-card-${key}']`);
const cardKeys = () =>
  [...document.querySelectorAll("[data-testid^='es-card-']")].map((el) =>
    el.getAttribute("data-testid"),
  );
const lastLine = () =>
  document.querySelector("[data-testid='es-unmatched-line']")?.textContent?.replace(/\s+/g, " ");
const dialog = (title: string) =>
  [...document.querySelectorAll<HTMLElement>(".el-dialog")].find(
    (el) => el.querySelector(".el-dialog__title")?.textContent?.trim() === title,
  );

afterEach(() => {
  view?.unmount();
  view = null;
  sessionStorage.clear();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("按条目搜索：不要", () => {
  it("拉黑这一组的全部成员（只对这一季），拉黑的默认不显示，最后一行写条数，点了灰显、能移出", async () => {
    viewport("wide");
    await mountEntry();
    card("h:aaa")!.querySelector<HTMLButtonElement>("[data-testid='es-act-block']")!.click();
    await vi.waitFor(() => expect(dialog("不要这个种子")).toBeDefined());
    const dlg = dialog("不要这个种子")!;
    expect(dlg.textContent).toContain("ourbits");
    expect(dlg.textContent).toContain("chdbits");
    expect(dlg.querySelector("[data-testid='block-scope']")?.textContent).toContain(
      "只对「长夜将尽 第 2 季」生效",
    );
    // 有订阅：能把制作组（首选解析出的 CHDWEB）加进标题排除词
    expect(dlg.textContent).toContain("以后也不要这个制作组（CHDWEB）");
    // el-input 把 data-testid 放在里面的 input 上
    const reason = dlg.querySelector<HTMLInputElement>("input[data-testid='block-reason']")!;
    reason.value = "音画不同步";
    reason.dispatchEvent(new Event("input", { bubbles: true }));
    dlg.querySelector<HTMLElement>("[data-testid='block-exclude-group']")!.click();
    api.addBlocks.mockResolvedValue({
      blocks: [block(11, "ourbits", "1"), block(12, "chdbits", "2")],
      excluded_group: "CHDWEB",
    });
    api.blocks.mockResolvedValue({
      items: [
        block(11, "ourbits", "1", { reason: "音画不同步" }),
        block(12, "chdbits", "2", { reason: "音画不同步" }),
        // 别的条目的不算
        block(13, "pttime", "3", { tmdb_id: 999, media_title: "别的剧" }),
      ],
    });
    dlg.querySelector<HTMLButtonElement>("[data-testid='block-confirm']")!.click();
    await vi.waitFor(() => expect(api.addBlocks).toHaveBeenCalledTimes(1));
    expect(api.addBlocks.mock.calls[0]![0]).toEqual({
      media_type: "tv",
      tmdb_id: 231620,
      season: 2,
      media_title: "长夜将尽 第 2 季",
      reason: "音画不同步",
      items: [
        {
          site: "ourbits",
          torrent_id: "1",
          title: "Chang.Ye.Jiang.Jin.S02E08.2160p.WEB-DL.H265-ourbits",
        },
        {
          site: "chdbits",
          torrent_id: "2",
          title: "Chang.Ye.Jiang.Jin.S02E08.2160p.WEB-DL.H265-chdbits",
        },
      ],
      exclude_group: true,
    });
    await vi.waitFor(() => expect(cardKeys()).toEqual(["es-card-s:pttime:3"]));
    expect(lastLine()).toContain("已拉黑 2 条 · 对不上这个条目的 2 条没显示");
    expect(document.querySelector("[data-testid='es-summary']")?.textContent).toContain("1 条结果");

    // 点「已拉黑 2 条」：拉黑的那张卡回来，排在最后、灰显，成员行与脚注都能移出
    document.querySelector<HTMLButtonElement>("[data-testid='es-show-blocked']")!.click();
    await vi.waitFor(() => expect(cardKeys()).toEqual(["es-card-s:pttime:3", "es-card-h:aaa"]));
    expect(card("h:aaa")!.classList).toContain("is-blocked");
    expect(card("h:aaa")!.textContent).toContain("已拉黑");
    expect(card("h:aaa")!.querySelector("[data-testid='es-act-block']")).toBeNull();
    expect(lastLine()).toContain("灰显在上面");
    api.removeBlock.mockResolvedValue({ ok: true });
    api.blocks.mockResolvedValue({ items: [] });
    card("h:aaa")!.querySelector<HTMLButtonElement>("[data-testid='es-act-unblock']")!.click();
    await vi.waitFor(() => expect(api.removeBlock).toHaveBeenCalledTimes(2));
    expect(api.removeBlock.mock.calls.map((c) => c[0])).toEqual([11, 12]);
    await vi.waitFor(() =>
      expect(card("h:aaa")!.querySelector("[data-testid='es-act-block']")).not.toBeNull(),
    );
    expect(lastLine()).not.toContain("已拉黑");
  });

  it("服务端标了拉黑的：最后一行写标题与原因；移出对这个条目的以后，对所有条目的那条还在就仍算拉黑", async () => {
    viewport("wide");
    const res = searchResult({ blocked_count: 1 });
    res.groups[1]!.members[0]!.block = { id: 21, reason: "假种" };
    res.groups[1]!.blocked = true;
    await mountEntry(res);
    expect(cardKeys()).toEqual(["es-card-h:aaa"]);
    expect(lastLine()).toContain(
      "已拉黑 1 条：The.Long.Night.S02E08.1080p.WEB-DL.H264-ADWeb（不要这个种子：假种）· 对不上这个条目的 2 条没显示",
    );
    document.querySelector<HTMLButtonElement>("[data-testid='es-show-blocked']")!.click();
    await vi.waitFor(() => expect(card("s:pttime:3")).not.toBeNull());
    api.removeBlock.mockResolvedValue({ ok: true });
    api.blocks.mockResolvedValue({
      items: [block(22, "PTTime", "3", { media_type: "", tmdb_id: 0, season: 0, reason: "全局" })],
    });
    card("s:pttime:3")!
      .querySelector<HTMLButtonElement>("[data-testid='es-unblock-pttime-3']")!
      .click();
    await vi.waitFor(() => expect(api.removeBlock).toHaveBeenCalledWith(21));
    await vi.waitFor(() => expect(lastLine()).toContain("灰显在上面"));
    expect(card("s:pttime:3")!.textContent).toContain("已拉黑");
  });
});

describe("等免费", () => {
  it("卡片脚注的「等免费」打开推送弹窗的等免费模式：下载器单选、等多久，建好带上条目与 H&R", async () => {
    viewport("wide");
    await mountEntry();
    // PTTime 读不了种子详情：点不了
    expect(
      card("s:pttime:3")!.querySelector<HTMLButtonElement>("[data-testid='es-act-watch']")!
        .disabled,
    ).toBe(true);
    card("h:aaa")!.querySelector<HTMLButtonElement>("[data-testid='es-act-watch']")!.click();
    await vi.waitFor(() => expect(dialog("等免费再下载")).toBeDefined());
    const dlg = dialog("等免费再下载")!;
    expect(dlg.textContent).toContain("每 5 分钟看一次这个种子的优惠");
    expect(dlg.textContent).not.toContain("添加后立即开始");
    expect(dlg.querySelector("[data-testid='watch-days'] input")).not.toBeNull();
    api.watchCreate.mockResolvedValue({ id: 1 });
    buttonByText("开始等", dlg).click();
    await vi.waitFor(() => expect(api.watchCreate).toHaveBeenCalledTimes(1));
    expect(api.watchCreate.mock.calls[0]![0]).toMatchObject({
      site: "ourbits",
      torrent_id: "1",
      title: "Chang.Ye.Jiang.Jin.S02E08.2160p.WEB-DL.H265-ourbits",
      size_bytes: 4 * 1024 ** 3,
      has_hr: true,
      hr_seed_time_h: 72,
      downloader_id: 1,
      media_type: "tv",
      tmdb_id: 231620,
      season: 2,
      days: 7,
    });
  });

  it("手机卡片底部两半：等免费、不要", async () => {
    viewport("mobile");
    await mountEntry();
    const m = document.querySelector<HTMLElement>("[data-testid='es-mcard-h:aaa']")!;
    const acts = [...m.querySelectorAll(".es-mact")].map((b) => b.textContent?.trim());
    expect(acts).toEqual(["等免费", "不要"]);
    m.querySelector<HTMLButtonElement>("[data-testid='es-act-watch']")!.click();
    await vi.waitFor(() => expect(dialog("等免费再下载")).toBeDefined());
    expect(dialog("不要这个种子"), "点的是等免费，不是不要").toBeUndefined();
    m.querySelector<HTMLButtonElement>("[data-testid='es-act-block']")!.click();
    await vi.waitFor(() => expect(dialog("不要这个种子")).toBeDefined());
  });
});

describe("关键字搜索的「更多」", () => {
  const hit = (id: string, site: string) => ({
    id,
    title: `Show.S01E0${id}.1080p`,
    sourceSite: site,
    sizeBytes: 1024,
    seeders: 1,
    leechers: 0,
    snatched: 0,
    isFree: false,
    discountLevel: "NONE",
    downloadUrl: `/api/site/${site}/torrent/${id}/download`,
  });

  async function mountKeyword(items: ReturnType<typeof hit>[]) {
    api.watchSites.mockResolvedValue({ sites: ["ourbits"] });
    api.multiSite.mockResolvedValue({
      items,
      totalResults: items.length,
      siteResults: {},
      errors: [],
      durationMs: 5,
    });
    const TorrentSearch = (await import("./TorrentSearch.vue")).default;
    view = mountView(TorrentSearch);
    const sel = "input[placeholder='输入关键词，回车即搜']";
    await vi.waitFor(() => expect(document.querySelector(sel)).not.toBeNull());
    const input = document.querySelector<HTMLInputElement>(sel)!;
    input.value = "Show";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
    await vi.waitFor(() =>
      expect(document.querySelectorAll(".el-table__body tbody tr")).toHaveLength(items.length),
    );
  }

  /** 点开一行的「更多」，点里面的一项（下拉被 teleport 到 body） */
  async function rowMenu(key: string, item: string) {
    document.querySelector<HTMLButtonElement>(`[data-testid='ts-more-${key}']`)!.click();
    await vi.waitFor(() =>
      expect(document.querySelector(`[data-testid='ts-${item}-${key}']`)).not.toBeNull(),
    );
    document.querySelector<HTMLElement>(`[data-testid='ts-${item}-${key}']`)!.click();
  }

  it("不要：对所有条目拉黑，行上标「已拉黑」，再点移出黑名单", async () => {
    viewport("wide");
    await mountKeyword([hit("1", "hdsky")]);
    await rowMenu("hdsky:1", "block");
    await vi.waitFor(() => expect(dialog("不要这个种子")).toBeDefined());
    const dlg = dialog("不要这个种子")!;
    expect(dlg.querySelector("[data-testid='block-scope']")?.textContent).toContain(
      "对所有条目生效",
    );
    expect(
      dlg.querySelector("[data-testid='block-exclude-group']"),
      "关键字搜索没有订阅",
    ).toBeNull();
    api.addBlocks.mockResolvedValue({
      blocks: [block(31, "hdsky", "1", { media_type: "", tmdb_id: 0, season: 0 })],
    });
    dlg.querySelector<HTMLButtonElement>("[data-testid='block-confirm']")!.click();
    await vi.waitFor(() => expect(api.addBlocks).toHaveBeenCalledTimes(1));
    expect(api.addBlocks.mock.calls[0]![0]).toEqual({
      items: [{ site: "hdsky", torrent_id: "1", title: "Show.S01E01.1080p" }],
    });
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid='ts-blocked-hdsky:1']")).not.toBeNull(),
    );
    api.removeBlock.mockResolvedValue({ ok: true });
    await rowMenu("hdsky:1", "unblock");
    await vi.waitFor(() => expect(api.removeBlock).toHaveBeenCalledWith(31));
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid='ts-blocked-hdsky:1']")).toBeNull(),
    );
  });

  it("等免费：站点读不了详情时点不了；能等时建一条，不带条目，下载器单选", async () => {
    viewport("wide");
    await mountKeyword([hit("1", "hdsky"), hit("2", "ourbits")]);
    document.querySelector<HTMLButtonElement>("[data-testid='ts-more-hdsky:1']")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid='ts-watch-hdsky:1']")).not.toBeNull(),
    );
    const off = document.querySelector<HTMLElement>("[data-testid='ts-watch-hdsky:1']")!;
    expect(
      off.classList.contains("is-disabled") || off.getAttribute("aria-disabled") === "true",
    ).toBe(true);
    expect(off.textContent).toContain("这个站点读不了种子详情");
    await rowMenu("ourbits:2", "watch");
    await vi.waitFor(() => expect(dialog("等免费再下载")).toBeDefined());
    // 等多久改成 3 天
    const days = dialog("等免费再下载")!.querySelector<HTMLInputElement>(
      "[data-testid='watch-days'] input",
    )!;
    days.value = "3";
    days.dispatchEvent(new Event("input", { bubbles: true }));
    days.dispatchEvent(new Event("change", { bubbles: true }));
    api.watchCreate.mockResolvedValue({ id: 2 });
    buttonByText("开始等", dialog("等免费再下载")!).click();
    await vi.waitFor(() => expect(api.watchCreate).toHaveBeenCalledTimes(1));
    const body = api.watchCreate.mock.calls[0]![0];
    expect(body).toMatchObject({ site: "ourbits", torrent_id: "2", downloader_id: 1, days: 3 });
    expect(body).not.toHaveProperty("tmdb_id");
  });
});
