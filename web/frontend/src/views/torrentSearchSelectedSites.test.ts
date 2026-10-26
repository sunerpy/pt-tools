// @vitest-environment happy-dom
/*
 * 钉子：选了站点时只问这些站点（2026-10-26 用户：「筛选了比如星陨阁时，其余站点不应参与搜索」）。
 * 请求里的 sites 就是选中的站点，后端只搜它们；不选时问全部可用站点。选中的站点后来不可用了（停用、删掉）时去掉它。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ multiSite: vi.fn(), getSites: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    searchApi: { getSites: api.getSites, multiSite: api.multiSite, clearCache: vi.fn() },
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    siteCategoriesApi: { getAll: vi.fn(() => Promise.resolve({})) },
    torrentPushApi: { push: vi.fn(), batchPush: vi.fn() },
  };
});

let view: MountedView | null = null;

const empty = { items: [], totalResults: 0, siteResults: {}, errors: [], durationMs: 5 };

/** 上一次搜索存在 sessionStorage 里的条件：页面打开时恢复选中的站点 */
function remember(selectedSites: string[]) {
  sessionStorage.setItem(
    "pt-tools-search-cache",
    JSON.stringify({ keyword: "上一次", results: [], selectedSites, timestamp: Date.now() }),
  );
}

async function mountAndSearch(keyword: string) {
  const TorrentSearch = (await import("./TorrentSearch.vue")).default;
  view = mountView(TorrentSearch);
  const sel = "input[placeholder='输入关键词，回车即搜']";
  await vi.waitFor(() => expect(document.querySelector(sel)).not.toBeNull());
  const input = document.querySelector<HTMLInputElement>(sel)!;
  // 等页面读完站点列表、恢复上一次的条件（有缓存时查询框里是上一次的关键词）
  if (sessionStorage.getItem("pt-tools-search-cache")) {
    await vi.waitFor(() => expect(input.value).toBe("上一次"));
  }
  input.value = keyword;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await Promise.resolve();
  input.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
  await vi.waitFor(() => expect(api.multiSite).toHaveBeenCalledTimes(1));
  return api.multiSite.mock.calls[0]![0];
}

afterEach(() => {
  view?.unmount();
  view = null;
  sessionStorage.clear();
  vi.clearAllMocks();
});

describe("种子搜索：选了站点时只问这些站点", () => {
  it("只选星陨阁：请求里只有它", async () => {
    api.getSites.mockResolvedValue(["hdsky", "mteam", "xingyunge"]);
    api.multiSite.mockResolvedValue(empty);
    remember(["xingyunge"]);
    const req = await mountAndSearch("天气之子");
    expect(req.sites).toEqual(["xingyunge"]);
  });

  it("不选：问全部可用站点", async () => {
    api.getSites.mockResolvedValue(["hdsky", "mteam", "xingyunge"]);
    api.multiSite.mockResolvedValue(empty);
    const req = await mountAndSearch("天气之子");
    expect(req.sites).toEqual(["hdsky", "mteam", "xingyunge"]);
  });
});
