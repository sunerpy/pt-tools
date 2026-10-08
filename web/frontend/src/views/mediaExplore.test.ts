// @vitest-environment happy-dom
/*
 * 钉子：探索页。打开时取电影的本周趋势；切到剧集、热门时重新取；搜索要填名字；已订阅的不再显示「订阅」；
 * 点「订阅」打开订阅弹窗，订阅后刷新列表；读不到时写明原因。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  explore: vi.fn(),
  profiles: vi.fn(),
  create: vi.fn(),
}));
const dl = vi.hoisted(() => ({ list: vi.fn() }));
const sites = vi.hoisted(() => ({ list: vi.fn() }));
const ui = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, subscribeApi: api, downloadersApi: dl, sitesApi: sites };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      info: ui.info,
      warning: vi.fn(),
    }),
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

const page = (items: unknown[]) => ({ items, page: 1, total_pages: 3 });
const dune = {
  id: 693134,
  media_type: "movie",
  title: "沙丘2",
  original_title: "Dune: Part Two",
  year: 2024,
  vote_average: 8.3,
  poster_path: "/p.jpg",
  in_library: false,
  subscribed: false,
};
const opp = {
  id: 872585,
  media_type: "movie",
  title: "奥本海默",
  original_title: "Oppenheimer",
  year: 2023,
  in_library: true,
  subscribed: true,
  subscription_id: 4,
};

async function mountPage(fail = false) {
  if (fail) api.explore.mockRejectedValue(new Error("TMDB 暂时不能访问"));
  else api.explore.mockResolvedValue(page([dune, opp]));
  api.profiles.mockResolvedValue({
    items: [{ id: 2, name: "4K" }],
    resolutions: [],
    sources: [],
    codecs: [],
  });
  dl.list.mockResolvedValue([{ id: 1, name: "qb", enabled: true }]);
  sites.list.mockResolvedValue({ hdsky: { enabled: true }, off: { enabled: false } });
  const Page = (await import("./MediaExplore.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:p(.*)*", component: { render: () => null } }],
  });
  view = mountView(Page, { router });
  await vi.waitFor(() => expect(api.explore).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("探索", () => {
  it("列表：趋势、已入库与已订阅；翻页和切换类型重新取", async () => {
    await mountPage();
    expect(api.explore).toHaveBeenCalledWith({ kind: "movie", list: "trending", page: 1, q: "" });
    const grid = q("ex-grid")!.textContent ?? "";
    expect(grid).toContain("沙丘2");
    expect(grid).toContain("2024");
    expect(grid).toContain("8.3");
    expect(q("ex-item-872585")!.textContent).toContain("已入库");
    expect(q("ex-item-872585")!.textContent).toContain("已订阅");
    expect(q("ex-sub-872585"), "已订阅的不再显示「订阅」").toBeNull();
    expect(q("ex-subscribed-872585")).not.toBeNull();
    expect(document.body.textContent).toContain("未经 TMDB 认可或认证");

    (q("ex-kind")!.querySelectorAll("input")[1] as HTMLInputElement).click();
    await vi.waitFor(() =>
      expect(api.explore).toHaveBeenLastCalledWith({
        kind: "tv",
        list: "trending",
        page: 1,
        q: "",
      }),
    );
  });

  it("搜索：没填名字时不取，填了回车搜", async () => {
    await mountPage();
    (q("ex-list")!.querySelectorAll("input")[2] as HTMLInputElement).click();
    await flush();
    expect(api.explore).toHaveBeenCalledTimes(1);
    expect(q("ex-state")!.textContent).toContain("输入名字搜索");
    const el = q("ex-keyword")!;
    const input = (el instanceof HTMLInputElement ? el : el.querySelector("input"))!;
    input.value = "沙丘";
    input.dispatchEvent(new Event("input"));
    input.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter" }));
    await vi.waitFor(() =>
      expect(api.explore).toHaveBeenLastCalledWith({
        kind: "movie",
        list: "search",
        page: 1,
        q: "沙丘",
      }),
    );
  });

  it("切到搜索时还没回来的趋势请求不再写进列表", async () => {
    await mountPage();
    let resolve!: (v: unknown) => void;
    api.explore.mockReturnValueOnce(new Promise((r) => (resolve = r)));
    (q("ex-kind")!.querySelectorAll("input")[1] as HTMLInputElement).click();
    await flush();
    (q("ex-list")!.querySelectorAll("input")[2] as HTMLInputElement).click();
    await flush();
    resolve(page([{ ...dune, id: 1, media_type: "tv", title: "迟到的剧集" }]));
    await flush();
    await flush();
    expect(q("ex-grid"), "空的搜索不显示迟到的结果").toBeNull();
    expect(q("ex-state")!.textContent).toContain("输入名字搜索");
  });

  it("订阅：打开弹窗，只发接口认的字段，订阅后刷新列表", async () => {
    await mountPage();
    api.create.mockResolvedValue({ id: 9, title: "沙丘2" });
    q("ex-sub-693134")!.click();
    await vi.waitFor(() => expect(q("subscribe-dialog")).not.toBeNull());
    await vi.waitFor(() => expect(api.profiles).toHaveBeenCalled());
    expect(q("sd-name")!.textContent).toContain("沙丘2 (2024)");
    expect(q("sd-season"), "电影没有季").toBeNull();
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      profile_id: 0,
      sites: [],
      downloader_id: 0,
      category: "",
      tags: "",
      save_path: "",
      upgrade: false,
      media_type: "movie",
      tmdb_id: 693134,
      season: 0,
    });
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("已订阅：沙丘2"));
    await vi.waitFor(() => expect(api.explore).toHaveBeenCalledTimes(2));
  });

  it("读不到时写明原因", async () => {
    await mountPage(true);
    expect(q("ex-state")!.textContent).toContain("TMDB 暂时不能访问");
  });
});
