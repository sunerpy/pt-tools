// @vitest-environment happy-dom
/*
 * 钉子：用户统计页上账号的下载量都写明口径（设计 free-expiry-safety B.2.14 / B.2.17）。
 * 站点报的下载量是计费口径（按免费、折扣折算以后计入分享率），实际传输另算：
 *   - 站点表的表头是「上传 / 下载（计费）」，实际传输一列叫「实际传输」；
 *   - 实际传输只要站点报了就显示 —— 原来只在实际上传与上传不同时才显示，
 *     M-Team 免费下载时实际下载在涨、实际上传与上传相同，实际下载就看不到；
 *   - 账户总量写「总下载量（计费）」，周期增量的下载写「下载（计费）」并说明只有计费口径；
 *   - 手机行卡在下载量后面标「计费」，站点报了实际传输时多出「实际上传 / 实际下载」。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const GiB = 1024 ** 3;

// 站点头像会去请求 /api/favicon/*，测试里没有服务端，换成空壳
vi.mock("@/components/SiteAvatar.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return { default: defineComponent({ render: () => h("span") }) };
});

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    userInfoApi: {
      ...real.userInfoApi,
      getAggregated: vi.fn(() =>
        Promise.resolve({
          totalUploaded: 100 * GiB,
          totalDownloaded: 10 * GiB,
          averageRatio: 10,
          totalSeeding: 10,
          totalLeeching: 0,
          totalBonus: 1000,
          siteCount: 2,
          lastUpdate: 1758000000,
          perSiteStats: [
            {
              site: "mteam",
              username: "u",
              uploaded: 60 * GiB,
              downloaded: 5 * GiB,
              // 免费下载：实际下载比计费的多，实际上传与上传相同
              trueUploaded: 60 * GiB,
              trueDownloaded: 40 * GiB,
              ratio: 12,
              bonus: 600,
              seeding: 6,
              leeching: 0,
              rank: "",
              lastUpdate: 1758000000,
            },
            {
              site: "hdsky",
              username: "u",
              uploaded: 40 * GiB,
              downloaded: 5 * GiB,
              ratio: 8,
              bonus: 400,
              seeding: 4,
              leeching: 0,
              rank: "",
              lastUpdate: 1758000000,
            },
          ],
        }),
      ),
      getSummary: vi.fn(() =>
        Promise.resolve({
          range: "7d",
          from: "2026-09-30",
          to: "2026-10-06",
          sites: [
            {
              site: "mteam",
              from: "2026-09-29",
              to: "2026-10-06",
              uploaded: 3 * GiB,
              downloaded: 1 * GiB,
              bonus: 10,
              hasBaseline: true,
              negative: false,
            },
          ],
          totalUploaded: 3 * GiB,
          totalDownloaded: 1 * GiB,
          totalBonus: 10,
        }),
      ),
      getTrends: vi.fn(() =>
        Promise.resolve({ days: 7, from: "", to: "", dates: [], totals: [], sites: {} }),
      ),
    },
    sitesApi: {
      ...real.sitesApi,
      list: vi.fn(() => Promise.resolve({})),
      listLoginStates: vi.fn(() => Promise.resolve([])),
    },
    tasksApi: { ...real.tasksApi, stats: vi.fn(() => Promise.resolve(null)) },
    siteLevelsApi: { ...real.siteLevelsApi, getAll: vi.fn(() => Promise.resolve({ sites: {} })) },
  };
});

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

let view: MountedView | null = null;

beforeEach(() => {
  setActivePinia(createPinia());
  window.localStorage.clear();
});

afterEach(() => {
  view?.unmount();
  view = null;
  window.localStorage.clear();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

async function mountDashboard() {
  const Dash = (await import("./UserInfoDashboard.vue")).default;
  view = mountView(Dash);
}

const text = (el: Element | null | undefined) => (el?.textContent ?? "").replace(/\s+/g, " ");

/** 站点表的表头文字 */
function headers(): string[] {
  return [...document.querySelectorAll(".el-table__header th .cell")].map((c) => text(c).trim());
}

/** 站点表里某一站那一行的各格文字（按表头顺序） */
function rowCells(site: string): Record<string, string> {
  const hs = headers();
  const row = [...document.querySelectorAll(".el-table__body tr.el-table__row")].find((tr) =>
    text(tr.querySelector("td")).includes(site),
  );
  const out: Record<string, string> = {};
  [...(row?.querySelectorAll("td") ?? [])].forEach((td, i) => {
    out[hs[i] ?? String(i)] = text(td).trim();
  });
  return out;
}

describe("用户统计：下载量的口径", () => {
  it("桌面：表头写明计费，站点报了实际传输就显示（只有实际下载不同也显示）", async () => {
    // 列偏好存成「什么都不收起」，实际传输这一列才在
    window.localStorage.setItem("pt-tools-userinfo-cols-v1", "[]");
    await mountDashboard();

    await vi.waitFor(() => expect(rowCells("mteam")["实际传输"]).toBeTruthy());
    expect(headers()).toContain("上传 / 下载（计费）");
    expect(headers()).not.toContain("数据量");
    expect(headers()).not.toContain("真实数据");

    const mteam = rowCells("mteam");
    expect(mteam["上传 / 下载（计费）"]).toContain("↓5 GB");
    expect(mteam["实际传输"]).toContain("↑60 GB");
    expect(mteam["实际传输"]).toContain("↓40 GB");
    expect(rowCells("hdsky")["实际传输"], "站点不报实际传输时是「-」").toBe("-");
  });

  it("没存过列偏好时实际传输一列照旧收起，列设置里的名字是「实际传输」", async () => {
    await mountDashboard();

    await vi.waitFor(() => expect(headers()).toContain("上传 / 下载（计费）"));
    expect(headers()).not.toContain("实际传输");
    document.querySelector<HTMLElement>("[data-testid=userinfo-cols-btn]")!.click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("实际传输"));
  });

  it("账户总量与周期增量：下载写明计费，周期增量说明只有计费口径", async () => {
    await mountDashboard();

    await vi.waitFor(() =>
      expect(text(document.querySelector("[data-testid=userinfo-totals]"))).toContain(
        "总下载量（计费）",
      ),
    );
    await vi.waitFor(() => expect(document.body.textContent).toContain("下载（计费）+1 GB"));
    expect(document.body.textContent).toContain("下载增量只有计费口径");
  });

  it("手机行卡：下载量后面标「计费」，站点报了实际传输时多出实际上传、实际下载", async () => {
    mobileViewport();
    await mountDashboard();

    await vi.waitFor(() => expect(document.querySelectorAll("article.card").length).toBe(2));
    const card = (site: string) =>
      [...document.querySelectorAll("article.card")].find((c) =>
        text(c.querySelector(".site__name")).includes(site),
      );
    /** 行卡统计区的「标签 → 数值」 */
    const stats = (site: string) =>
      Object.fromEntries(
        [...(card(site)?.querySelectorAll(".stat") ?? [])].map((s) => [
          text(s.querySelector(".stat__l")).trim(),
          text(s.querySelector(".stat__v")).trim(),
        ]),
      );
    expect(text(card("mteam")?.querySelector(".card__io"))).toContain("5 GB 计费");
    expect(stats("mteam")["实际上传"]).toBe("60 GB");
    expect(stats("mteam")["实际下载"]).toBe("40 GB");
    expect(stats("hdsky")).not.toHaveProperty("实际下载");
    expect(stats("hdsky")).not.toHaveProperty("实际上传");
  });
});
