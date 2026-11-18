// @vitest-environment happy-dom
/*
 * 钉子：探索页。打开时取电影的本周趋势；切到剧集、热门时重新取；搜索要填名字；已订阅的不再显示「订阅」；
 * 点「订阅」打开订阅弹窗，订阅后刷新列表；剧集点「追剧」，按追剧的默认设置建；读不到时写明原因。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Router, createMemoryHistory, createRouter } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  explore: vi.fn(),
  profiles: vi.fn(),
  create: vi.fn(),
  bangumiCalendar: vi.fn(),
  bangumiTMDB: vi.fn(),
}));
const dl = vi.hoisted(() => ({ list: vi.fn() }));
const sites = vi.hoisted(() => ({ list: vi.fn() }));
// 「媒体识别」里换了图片地址：海报要从这里加载
const media = vi.hoisted(() => ({
  settings: vi.fn(async () => ({ image_base_url: "https://static.example.com/t/p/" })),
}));
const organize = vi.hoisted(() => ({
  libraries: vi.fn(async () => [
    { id: 4, name: "剧集库", kind: "tv" },
    { id: 5, name: "电影库", kind: "movie" },
  ]),
}));
const ui = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    subscribeApi: api,
    downloadersApi: dl,
    sitesApi: sites,
    mediaApi: media,
    organizeApi: organize,
    // 追剧对话框的保存目录会读下载器的预设目录
    downloaderDirectoriesApi: {
      ...real.downloaderDirectoriesApi,
      listAll: vi.fn(async () => ({})),
    },
  };
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
let router: Router;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

/** 打开下拉框，点文字是 text 的选项 */
async function pick(id: string, text: string) {
  const sel = q(id)!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const listId = sel.querySelector("input")?.getAttribute("aria-controls");
  await vi.waitFor(() => {
    const opt = [
      ...(document
        .getElementById(listId ?? "")
        ?.querySelectorAll<HTMLElement>(".el-select-dropdown__item") ?? []),
    ].find((o) => (o.textContent ?? "").trim() === text);
    expect(opt, text).toBeTruthy();
    opt!.click();
  });
}

/** 下拉框里现在有哪些选项 */
function optionTexts(id: string): string[] {
  const sel = q(id)!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const listId = sel.querySelector("input")?.getAttribute("aria-controls");
  return [
    ...(document
      .getElementById(listId ?? "")
      ?.querySelectorAll<HTMLElement>(".el-select-dropdown__item") ?? []),
  ].map((o) => (o.textContent ?? "").trim());
}

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
  router = createRouter({
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
    // 海报与名字都链到条目详情，面包屑写从探索进来
    const links = [...q("ex-item-693134")!.querySelectorAll("a")].map((a) =>
      a.getAttribute("href"),
    );
    expect(links.length, "海报与名字").toBe(2);
    expect(new Set(links)).toEqual(new Set(["/media/item/movie/693134?from=explore"]));
    await vi.waitFor(() =>
      expect(q("ex-item-693134")!.querySelector("img")!.getAttribute("src"), "换过的图片地址").toBe(
        "https://static.example.com/t/p/w342/p.jpg",
      ),
    );

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
    await pick("ex-list", "搜索");
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
    await pick("ex-list", "搜索");
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
      include_keywords: "",
      exclude_keywords: "",
      media_type: "movie",
      tmdb_id: 693134,
      season: 0,
      // 电影也能选媒体库：新建时带上，0 是按类型自动选
      library_id: 0,
    });
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("已订阅：沙丘2"));
    await vi.waitFor(() => expect(api.explore).toHaveBeenCalledTimes(2));
  });

  it("剧集：点「追剧」，按追剧的默认设置建（按播出时间、开着版本锁）", async () => {
    await mountPage();
    const show = {
      id: 231620,
      media_type: "tv",
      title: "现在就出发",
      original_title: "现在就出发",
      year: 2023,
      in_library: false,
      subscribed: false,
    };
    api.explore.mockResolvedValue(page([show]));
    (q("ex-kind")!.querySelectorAll("input")[1] as HTMLInputElement).click();
    await vi.waitFor(() => expect(q("ex-sub-231620")).not.toBeNull());
    expect(q("ex-sub-231620")!.textContent).toContain("追剧");
    api.create.mockResolvedValue({ id: 11, title: "现在就出发" });
    q("ex-sub-231620")!.click();
    await vi.waitFor(() => expect(q("subscribe-dialog")).not.toBeNull());
    await vi.waitFor(() => expect(organize.libraries).toHaveBeenCalled());
    expect(q("sd-schedule"), "剧集有追剧的设置").not.toBeNull();
    expect(q("sd-season")).not.toBeNull();
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
      include_keywords: "",
      exclude_keywords: "",
      schedule: { kind: "airtime", air_delay_hours: 0, air_retry_hours: 0, air_window_days: 0 },
      lock_version: true,
      follow_next_season: false,
      library_id: 0,
      media_type: "tv",
      tmdb_id: 231620,
      season: 0,
    });
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("开始追剧：现在就出发"));
  });

  it("读不到时写明原因", async () => {
    await mountPage(true);
    expect(q("ex-state")!.textContent).toContain("TMDB 暂时不能访问");
  });

  it("更多列表：电影有正在上映，换成剧集时没有的列表换回本周趋势", async () => {
    await mountPage();
    expect(optionTexts("ex-list")).toEqual([
      "本周趋势",
      "热门",
      "正在上映",
      "即将上映",
      "高分",
      "按国家与类型找",
      "搜索",
    ]);
    await pick("ex-list", "正在上映");
    await vi.waitFor(() =>
      expect(api.explore).toHaveBeenLastCalledWith({
        kind: "movie",
        list: "now_playing",
        page: 1,
        q: "",
      }),
    );
    (q("ex-kind")!.querySelectorAll("input")[1] as HTMLInputElement).click();
    await vi.waitFor(() =>
      expect(api.explore).toHaveBeenLastCalledWith({
        kind: "tv",
        list: "trending",
        page: 1,
        q: "",
      }),
    );
    expect(optionTexts("ex-list")).toContain("今天播出");
    await pick("ex-list", "今天播出");
    await vi.waitFor(() =>
      expect(api.explore).toHaveBeenLastCalledWith({
        kind: "tv",
        list: "airing_today",
        page: 1,
        q: "",
      }),
    );
  });

  it("按国家与类型找：选了才带上；综艺只有剧集有，换成电影时换回不限", async () => {
    await mountPage();
    expect(q("ex-country"), "没选按条件找时不显示").toBeNull();
    (q("ex-kind")!.querySelectorAll("input")[1] as HTMLInputElement).click();
    await flush();
    await pick("ex-list", "按国家与类型找");
    await vi.waitFor(() =>
      expect(api.explore).toHaveBeenLastCalledWith({
        kind: "tv",
        list: "discover",
        page: 1,
        q: "",
        country: "",
        genres: "",
      }),
    );
    expect(q("ex-country")!.textContent).toContain("不限国家");
    await pick("ex-country", "日本");
    await pick("ex-genres", "综艺");
    await vi.waitFor(() =>
      expect(api.explore).toHaveBeenLastCalledWith({
        kind: "tv",
        list: "discover",
        page: 1,
        q: "",
        country: "JP",
        genres: "10764|10767",
      }),
    );
    (q("ex-kind")!.querySelectorAll("input")[0] as HTMLInputElement).click();
    await vi.waitFor(() =>
      expect(api.explore).toHaveBeenLastCalledWith({
        kind: "movie",
        list: "discover",
        page: 1,
        q: "",
        country: "JP",
        genres: "",
      }),
    );
    expect(optionTexts("ex-genres")).toEqual(["不限类型", "动画", "纪录片"]);
  });

  it("番剧日历：默认今天，换一天看别的；点一部到 TMDB 上找到以后打开详情", async () => {
    const today = ((new Date().getDay() + 6) % 7) + 1;
    const other = (today % 7) + 1;
    api.bangumiCalendar.mockResolvedValue({
      days: [
        {
          weekday: today,
          name: "",
          items: [
            {
              id: 500001,
              name: "葬送のフリーレン 第2期",
              name_cn: "葬送的芙莉莲 第二季",
              air_date: "2026-01-16",
              image: "https://lain.bgm.tv/c.jpg",
              score: 8.6,
            },
          ],
        },
        { weekday: other, name: "", items: [{ id: 7, name: "Nobody" }] },
      ],
    });
    api.bangumiTMDB.mockResolvedValue({ media_type: "tv", tmdb_id: 209867, title: "葬送的芙莉莲" });
    await mountPage();
    (q("ex-kind")!.querySelectorAll("input")[2] as HTMLInputElement).click();
    await vi.waitFor(() => expect(q("ex-bgm-500001")).not.toBeNull());
    expect(q("ex-list"), "番剧日历没有 TMDB 的列表").toBeNull();
    const card = q("ex-bgm-500001")!.textContent ?? "";
    expect(card).toContain("葬送的芙莉莲 第二季");
    expect(card).toContain("2026-01-16 开播");
    expect(card).toContain("8.6");
    expect(q("ex-bgm-500001")!.querySelector("img")!.getAttribute("src")).toBe(
      "https://lain.bgm.tv/c.jpg",
    );
    expect(q("ex-days")!.textContent).toContain("今天");
    expect(document.body.textContent).toContain("番剧日历的数据来自 Bangumi");

    (q("ex-days")!.querySelectorAll("input")[other - 1] as HTMLInputElement).click();
    await vi.waitFor(() => expect(q("ex-bgm-7")).not.toBeNull());
    expect(q("ex-bgm-500001")).toBeNull();
    (q("ex-days")!.querySelectorAll("input")[today - 1] as HTMLInputElement).click();
    await vi.waitFor(() => expect(q("ex-bgm-500001")).not.toBeNull());

    q("ex-bgm-open-500001")!.click();
    await vi.waitFor(() => expect(api.bangumiTMDB).toHaveBeenCalledWith(500001));
    await vi.waitFor(() =>
      expect(router.currentRoute.value.fullPath).toBe("/media/item/tv/209867?from=explore"),
    );
  });

  it("番剧日历：TMDB 上没找到时写明，可以按名字搜", async () => {
    const today = ((new Date().getDay() + 6) % 7) + 1;
    api.bangumiCalendar.mockResolvedValue({
      days: [{ weekday: today, name: "", items: [{ id: 7, name: "Nobody", name_cn: "无名" }] }],
    });
    api.bangumiTMDB.mockRejectedValue(
      new Error("TMDB 上没找到对得上的剧集（搜过「无名」「Nobody」）"),
    );
    await mountPage();
    (q("ex-kind")!.querySelectorAll("input")[2] as HTMLInputElement).click();
    await vi.waitFor(() => expect(q("ex-bgm-7")).not.toBeNull());
    q("ex-bgm-open-7")!.click();
    await vi.waitFor(() => expect(q("ex-missing")).not.toBeNull());
    expect(q("ex-missing")!.textContent).toContain(
      "「无名」：TMDB 上没找到对得上的剧集（搜过「无名」「Nobody」）。",
    );
    q("ex-missing-search")!.click();
    await vi.waitFor(() =>
      expect(api.explore).toHaveBeenLastCalledWith({
        kind: "tv",
        list: "search",
        page: 1,
        q: "无名",
      }),
    );
    expect(q("ex-missing")).toBeNull();
  });

  it("番剧日历：读不到时写明，可以重试；这一天没有时写明", async () => {
    const today = ((new Date().getDay() + 6) % 7) + 1;
    api.bangumiCalendar.mockRejectedValueOnce(
      new Error("读不到 Bangumi 的番剧日历：连不上 Bangumi"),
    );
    await mountPage();
    (q("ex-kind")!.querySelectorAll("input")[2] as HTMLInputElement).click();
    await vi.waitFor(() => expect(q("ex-bgm-state")!.textContent).toContain("连不上 Bangumi"));
    api.bangumiCalendar.mockResolvedValue({ days: [{ weekday: today, name: "", items: [] }] });
    q("ex-bgm-reload")!.click();
    await vi.waitFor(() =>
      expect(q("ex-bgm-state")!.textContent).toContain("这一天没有在播的番剧"),
    );
  });
});
