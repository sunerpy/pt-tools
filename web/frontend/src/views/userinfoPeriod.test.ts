// @vitest-environment happy-dom
/*
 * 钉子：用户统计页的「周期」chip（画板 10 的 chip-0）由每日快照支撑。
 * 选周期之后 KPI 带「总上传量」那一格显示该周期的上传增量，「上传构成」按周期增量排；
 * 切换周期时旧请求晚回来不能把上一档的数字配到这一档的标签上。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  getSummary: vi.fn(),
  getTrends: vi.fn(),
}));

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
          averageRatio: 5,
          totalSeeding: 10,
          totalLeeching: 0,
          totalBonus: 1000,
          siteCount: 2,
          lastUpdate: 1758000000,
          perSiteStats: [
            {
              site: "hdsky",
              username: "u",
              uploaded: 60 * GiB,
              downloaded: 5 * GiB,
              ratio: 12,
              bonus: 600,
              seeding: 6,
              leeching: 0,
              rank: "",
              lastUpdate: 1758000000,
            },
            {
              site: "pter",
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
      getSummary: api.getSummary,
      getTrends: api.getTrends,
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

function summaryOf(range: string, hdsky: number, pter: number) {
  return {
    range,
    from: "2026-09-30",
    to: "2026-10-06",
    sites: [
      {
        site: "hdsky",
        from: "2026-09-29",
        to: "2026-10-06",
        uploaded: hdsky,
        downloaded: 0,
        bonus: 0,
        hasBaseline: true,
        negative: false,
      },
      {
        site: "pter",
        from: "2026-09-29",
        to: "2026-10-06",
        uploaded: pter,
        downloaded: 0,
        bonus: 0,
        hasBaseline: true,
        negative: false,
      },
    ],
    totalUploaded: hdsky + pter,
    totalDownloaded: 0,
    totalBonus: 0,
  };
}

let view: MountedView | null = null;

beforeEach(() => {
  setActivePinia(createPinia());
});

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

async function mountDashboard() {
  const Dash = (await import("./UserInfoDashboard.vue")).default;
  view = mountView(Dash);
}

async function choosePeriod(label: string) {
  const chip = document.querySelector("[data-testid=userinfo-period]")!;
  (chip.querySelector<HTMLElement>(".el-select__wrapper") ?? (chip as HTMLElement)).click();
  await vi.waitFor(() => {
    const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
      (o) => (o.textContent ?? "").trim() === label,
    );
    expect(opt).toBeTruthy();
    opt!.click();
  });
}

describe("用户统计：周期", () => {
  it("默认本周：KPI 总上传量显示本周增量，上传构成按增量排", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", 3 * GiB, 1 * GiB));
    api.getTrends.mockResolvedValue({
      days: 8,
      from: "",
      to: "",
      dates: [],
      totals: [],
      sites: {},
    });
    await mountDashboard();

    await vi.waitFor(() => expect(api.getSummary).toHaveBeenCalledWith("7d"));
    await vi.waitFor(() => expect(document.body.textContent).toContain("本周 +4 GB"));
    expect(document.body.textContent).toContain("+3 GB");
  });

  it("切到今日：重新取今日的增量；上一档晚回来的结果被丢掉", async () => {
    let releaseWeek!: (v: unknown) => void;
    api.getSummary.mockImplementation((range: string) =>
      range === "7d"
        ? new Promise((resolve) => {
            releaseWeek = resolve;
          })
        : Promise.resolve(summaryOf("today", 2 * GiB, 0)),
    );
    api.getTrends.mockResolvedValue({
      days: 8,
      from: "",
      to: "",
      dates: [],
      totals: [],
      sites: {},
    });
    await mountDashboard();
    await vi.waitFor(() => expect(api.getSummary).toHaveBeenCalledWith("7d"));

    await choosePeriod("周期: 今日");
    await vi.waitFor(() => expect(api.getSummary).toHaveBeenCalledWith("today"));
    await vi.waitFor(() => expect(document.body.textContent).toContain("今日 +2 GB"));

    releaseWeek(summaryOf("7d", 9 * GiB, 9 * GiB));
    await new Promise((r) => setTimeout(r, 0));
    expect(document.body.textContent).not.toContain("+18 GB");
    expect(document.body.textContent).toContain("今日 +2 GB");
  });

  /** 「总上传量」那一格的柱：每根的高度（px）与读屏说明 */
  function uploadKpiBars() {
    const cell = [...document.querySelectorAll<HTMLElement>(".pt-kpi__cell")].find((c) =>
      (c.textContent ?? "").includes("总上传量"),
    );
    const bars = cell?.querySelector<HTMLElement>(".pt-kpi__bars");
    return {
      hint: bars?.getAttribute("aria-label") ?? null,
      heights: [...(bars?.querySelectorAll<HTMLElement>("i") ?? [])].map((i) =>
        Number.parseInt(i.style.height, 10),
      ),
    };
  }

  it("最近 7 天一天增量都没有：柱照画（每根 2px 的底），不丢掉画板要求的柱图", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", 0, 0));
    const dates = [
      "2026-09-30",
      "2026-10-01",
      "2026-10-02",
      "2026-10-03",
      "2026-10-04",
      "2026-10-05",
      "2026-10-06",
    ];
    api.getTrends.mockResolvedValue({
      days: 7,
      from: dates[0],
      to: dates.at(-1),
      dates,
      totals: dates.map((date) => ({ date, uploaded: 0, downloaded: 0, bonus: 0 })),
      sites: {},
    });
    await mountDashboard();

    await vi.waitFor(() => expect(document.body.textContent).toContain("本周 +0 B"));
    await vi.waitFor(() => expect(uploadKpiBars().heights.length).toBe(7));
    expect(uploadKpiBars().heights).toEqual([2, 2, 2, 2, 2, 2, 2]);
    expect(uploadKpiBars().hint).toContain("每天全部站点的上传增量");
  });

  it("走势接口没拿到：那一格回落到按站点的上传构成，柱图仍在", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", 3 * GiB, 1 * GiB));
    api.getTrends.mockRejectedValue(new Error("boom"));
    await mountDashboard();

    await vi.waitFor(() => expect(document.body.textContent).toContain("本周 +4 GB"));
    await vi.waitFor(() => expect(uploadKpiBars().heights.length).toBeGreaterThan(0));
    expect(uploadKpiBars().hint).toContain("上传量构成");
  });
});
