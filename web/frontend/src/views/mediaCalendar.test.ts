// @vitest-environment happy-dom
/*
 * 钉子：订阅日历（媒体中心 M3）。默认月视图，按整周取；翻页、换周视图重新取，路由记下视图与日期；
 * 一格最多三项，多的点了换成那一周；点一项打开条目详情（带上季）；没读到 TMDB 的订阅写明；手机按天列出有内容的天。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ calendar: vi.fn() }));
const media = vi.hoisted(() => ({
  settings: vi.fn(async () => ({ image_base_url: "", tmdb_key_set: true })),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, subscribeApi: api, mediaApi: media };
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

const ep = (n: number, day: string, state = "upcoming") => ({
  subscription_id: 5,
  media_type: "tv",
  tmdb_id: 100088,
  title: "最后生还者",
  season: 2,
  episode: n,
  name: `第 ${n} 集`,
  air_date: day,
  state,
  air_at: new Date(`${day}T21:30:00`).toISOString(),
});

function calendar(from: string, to: string, entries: unknown[] = [], errors: string[] = []) {
  return { from, to, today: "2026-11-03", entries, errors };
}

async function mountAt(path: string, mobile = false) {
  viewport(mobile);
  const Page = (await import("./MediaCalendar.vue")).default;
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/media/calendar", name: "media-calendar", component: Page },
      { path: "/media/item/:type/:id", name: "media-item", component: { render: () => null } },
      { path: "/:p(.*)*", component: { render: () => null } },
    ],
  });
  await router.push(path);
  await router.isReady();
  const { h } = await import("vue");
  const { RouterView } = await import("vue-router");
  view = mountView({ render: () => h(RouterView) }, { router });
  await vi.waitFor(() => expect(api.calendar).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("订阅日历", () => {
  it("月视图：按整周取，一格最多三项，多的点了换成那一周", async () => {
    api.calendar.mockImplementation(async (from: string, to: string) =>
      calendar(
        from,
        to,
        [
          ep(1, "2026-11-03", "library"),
          ep(2, "2026-11-03", "missing"),
          ep(3, "2026-11-03"),
          ep(4, "2026-11-03"),
          {
            subscription_id: 6,
            media_type: "movie",
            tmdb_id: 1,
            title: "沙丘2",
            air_date: "2026-11-05",
            state: "upcoming",
          },
        ],
        ["坏的剧 (2020) 第 1 季：TMDB 上没有"],
      ),
    );
    await mountAt("/media/calendar?date=2026-11-15");
    expect(api.calendar).toHaveBeenCalledWith("2026-10-26", "2026-12-06");
    expect(q("mc-label")!.textContent).toBe("2026 年 11 月");
    const cell = q("mc-cell-2026-11-03")!;
    expect(cell.classList.contains("is-today")).toBe(true);
    expect(cell.querySelectorAll("[data-testid=mc-entry]")).toHaveLength(3);
    expect(cell.textContent).toContain("S02E01");
    expect(cell.textContent, "月视图的一项不写时刻，写在悬停的提示里").not.toContain("21:30");
    expect(cell.querySelector("[data-testid=mc-entry]")!.getAttribute("title")).toBe(
      "最后生还者 第 2 季 E01 · 第 1 集 · 21:30 播出",
    );
    expect(q("mc-cell-2026-11-05")!.textContent).toContain("上映");
    expect(
      q("mc-cell-2026-11-05")!.querySelector("[data-testid=mc-entry]")!.getAttribute("title"),
    ).toBe("沙丘2 电影上映");
    expect(q("mc-mbar"), "桌面的翻页在卡头").toBeNull();
    expect(q("mc-bar")!.querySelector("[data-testid=mc-prev]")).not.toBeNull();
    expect(q("mc-cell-2026-10-26")!.classList.contains("is-out")).toBe(true);
    expect(q("mc-errors")!.textContent).toContain("坏的剧 (2020) 第 1 季");
    expect(document.querySelector("#pt-head-sub")!.textContent).toContain(
      "2026 年 11 月 · 4 集 · 1 部电影 · 已入库 1",
    );

    q("mc-more-2026-11-03")!.click();
    await vi.waitFor(() =>
      expect(api.calendar).toHaveBeenLastCalledWith("2026-11-02", "2026-11-08"),
    );
    await vi.waitFor(() =>
      expect(router.currentRoute.value.query).toEqual({ view: "week", date: "2026-11-03" }),
    );
    await flush();
    expect(
      q("mc-cell-2026-11-03")!.querySelectorAll("[data-testid=mc-entry]"),
      "周视图全列出",
    ).toHaveLength(4);
    expect(q("mc-cell-2026-11-03")!.textContent).toContain("第 2 季 E02 · 第 2 集");
  });

  it("翻页、回到今天；点一项打开条目详情", async () => {
    api.calendar.mockImplementation(async (from: string, to: string) =>
      calendar(from, to, [ep(7, "2026-11-10")]),
    );
    await mountAt("/media/calendar?view=week&date=2026-11-03");
    expect(api.calendar).toHaveBeenCalledWith("2026-11-02", "2026-11-08");
    q("mc-next")!.click();
    await vi.waitFor(() =>
      expect(api.calendar).toHaveBeenLastCalledWith("2026-11-09", "2026-11-15"),
    );
    q("mc-prev")!.click();
    q("mc-prev")!.click();
    await vi.waitFor(() =>
      expect(api.calendar).toHaveBeenLastCalledWith("2026-10-26", "2026-11-01"),
    );
    q("mc-next")!.click();
    q("mc-next")!.click();
    await vi.waitFor(() =>
      expect(q("mc-cell-2026-11-10")?.querySelector("[data-testid=mc-entry]")).not.toBeNull(),
    );
    q("mc-cell-2026-11-10")!.querySelector<HTMLElement>("[data-testid=mc-entry]")!.click();
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe("media-item"));
    expect(router.currentRoute.value.params).toEqual({ type: "tv", id: "100088" });
    expect(router.currentRoute.value.query).toEqual({ from: "calendar", season: "2" });
  });

  it("没有内容、读不到时写明", async () => {
    api.calendar.mockRejectedValueOnce(new Error("TMDB 没配"));
    await mountAt("/media/calendar?date=2026-11-03");
    await vi.waitFor(() => expect(q("mc-state")!.textContent).toContain("TMDB 没配"));
    api.calendar.mockImplementation(async (from: string, to: string) => calendar(from, to));
    q("mc-state")!.querySelector("button")!.click();
    await vi.waitFor(() => expect(q("mc-empty")).not.toBeNull());
    expect(q("mc-empty")!.textContent).toContain("暂停的、待确认的订阅不列");
  });

  it("手机：按天列出有内容的天", async () => {
    api.calendar.mockImplementation(async (from: string, to: string) =>
      calendar(from, to, [ep(1, "2026-11-03", "library"), ep(2, "2026-11-04")]),
    );
    await mountAt("/media/calendar?view=week&date=2026-11-03", true);
    await vi.waitFor(() => expect(q("mc-days")).not.toBeNull());
    expect(q("mc-day-2026-11-03")!.textContent).toContain("11-03 周二 · 今天");
    expect(q("mc-day-2026-11-04")!.textContent).toContain("21:30 播出");
    expect(q("mc-day-2026-11-05"), "没有内容的天不列").toBeNull();
    expect(qa("mc-entry")).toHaveLength(2);

    // 卡头只放月、周切换；翻页在面板里第一行
    expect(q("mc-bar")!.querySelector("[data-testid=mc-prev]")).toBeNull();
    expect(q("mc-view")).not.toBeNull();
    expect(q("mc-mbar")!.textContent).toContain("11-02 – 11-08");
    q("mc-next")!.click();
    await vi.waitFor(() =>
      expect(api.calendar).toHaveBeenLastCalledWith("2026-11-09", "2026-11-15"),
    );
    await vi.waitFor(() => expect(q("mc-mbar")!.textContent).toContain("11-09 – 11-15"));
  });
});
