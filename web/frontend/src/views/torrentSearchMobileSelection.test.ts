// @vitest-environment happy-dom
/*
 * 钉子：移动端切换分类分段或「仅免费」之后，被筛掉的勾选项不能留在已选列表里。
 *
 * 桌面 el-table 换数据时自己会清空勾选；手机上的行卡是页面自己维护 selectedTorrents，
 * 原来筛选一变，看不见的那几条还留在里面 —— 「已选 2 个」配一张只剩 1 张卡的列表，
 * 点「批量推送」会把用户已经看不见的种子一起推给下载器。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ batchPush: vi.fn() }));
const ui = vi.hoisted(() => ({ confirm: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    searchApi: {
      getSites: vi.fn(() => Promise.resolve(["mteam"])),
      multiSite: vi.fn(),
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
    torrentPushApi: { push: vi.fn(), batchPush: api.batchPush },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessageBox: { confirm: ui.confirm },
    ElMessage: Object.assign(vi.fn(), {
      success: vi.fn(),
      error: vi.fn(),
      warning: vi.fn(),
      info: vi.fn(),
    }),
  };
});

function item(id: string, title: string, category: string, isFree: boolean) {
  return {
    id,
    title,
    category,
    isFree,
    sizeBytes: 1024 ** 3,
    seeders: 5,
    leechers: 1,
    snatched: 10,
    sourceSite: "mteam",
    downloadUrl: `https://example.com/${id}.torrent`,
  };
}

let view: MountedView | null = null;

/** 手机视口：useIsMobile 读的是 matchMedia("(max-width: 768px)") */
function mobileViewport() {
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches: query.includes("max-width: 768px"),
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

/** 搜索页挂载时会从 sessionStorage 恢复上一次的结果，借它铺数据，不用真的去搜 */
async function mountWithResults() {
  mobileViewport();
  sessionStorage.setItem(
    "pt-tools-search-cache",
    JSON.stringify({
      keyword: "测试",
      results: [
        item("1", "电影甲", "电影/HD", true),
        item("2", "电影乙", "电影/HD", false),
        item("3", "剧集丙", "影剧/综艺/HD", true),
      ],
      siteResultCounts: { mteam: 3 },
      errors: [],
      searchTime: 120,
      totalResults: 3,
      selectedSites: [],
      categoryFilters: {},
      timestamp: Date.now(),
    }),
  );
  const TorrentSearch = (await import("./TorrentSearch.vue")).default;
  view = mountView(TorrentSearch);
  await vi.waitFor(() => expect(checkbox("电影甲")).not.toBeNull());
}

function checkbox(title: string): HTMLInputElement | null {
  return document.querySelector<HTMLInputElement>(`[aria-label='选择 ${title}'] input`);
}

async function pick(...titles: string[]) {
  for (const t of titles) checkbox(t)!.click();
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain(`已选 ${titles.length} 个种子`),
  );
}

/** 打开批量推送对话框并确认，返回推给后端的标题 */
async function batchPushTitles(): Promise<string[]> {
  ui.confirm.mockResolvedValue("confirm");
  api.batchPush.mockResolvedValue({
    successCount: 1,
    skippedCount: 0,
    failedCount: 0,
    results: [],
  });
  buttonByText("批量推送").click();
  await vi.waitFor(() => expect(document.querySelector(".el-dialog")).not.toBeNull());
  buttonByText("批量推送", document.querySelector(".el-dialog")!).click();
  await vi.waitFor(() => expect(api.batchPush).toHaveBeenCalledTimes(1));
  return api.batchPush.mock.calls[0]![0].torrents.map(
    (t: { torrentTitle: string }) => t.torrentTitle,
  );
}

afterEach(() => {
  view?.unmount();
  view = null;
  sessionStorage.clear();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("移动端：筛选之后按当前结果修剪已选", () => {
  it("打开「仅免费」：不免费的那条从已选里剪掉，批量推送只推看得见的", async () => {
    await mountWithResults();
    await pick("电影甲", "电影乙");

    buttonByText("仅免费").click();
    await vi.waitFor(() => expect(checkbox("电影乙")).toBeNull());
    expect(document.body.textContent).toContain("已选 1 个种子");

    expect(await batchPushTitles()).toEqual(["电影甲"]);
  });

  it("切换分类分段：归不进这一档的勾选项剪掉", async () => {
    await mountWithResults();
    await pick("电影甲", "剧集丙");

    const movieSeg = [...document.querySelectorAll<HTMLElement>(".el-segmented__item")].find(
      (el) => (el.textContent ?? "").trim() === "电影",
    )!;
    movieSeg.click();
    await vi.waitFor(() => expect(checkbox("剧集丙")).toBeNull());
    expect(document.body.textContent).toContain("已选 1 个种子");

    expect(await batchPushTitles()).toEqual(["电影甲"]);
  });

  it("筛选放宽回来之后不会自己把剪掉的再勾回去，留下的照旧保留", async () => {
    await mountWithResults();
    await pick("电影甲", "电影乙");
    buttonByText("仅免费").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("已选 1 个种子"));

    buttonByText("仅免费").click();
    await vi.waitFor(() => expect(checkbox("电影乙")).not.toBeNull());
    expect(document.body.textContent).toContain("已选 1 个种子");
    expect(checkbox("电影甲")!.checked).toBe(true);
    expect(checkbox("电影乙")!.checked).toBe(false);
  });
});
