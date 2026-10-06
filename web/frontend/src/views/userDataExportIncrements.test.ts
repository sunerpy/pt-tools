// @vitest-environment happy-dom
/*
 * 钉子：分享图可以附上周期增量（预览与导出的 PNG 用同一份 summaryStats）。
 * 有可比快照时汇总区多出「本周上传 / 下载 / 魔力」三格；关掉开关、或还没有可比快照时不放；
 * 切换周期重新取该周期的增量。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ getSummary: vi.fn() }));

const GiB = 1024 ** 3;

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
          totalSeeding: 5,
          totalLeeching: 0,
          totalBonus: 1000,
          siteCount: 0,
          lastUpdate: 1758000000,
          perSiteStats: [],
        }),
      ),
      getSummary: api.getSummary,
    },
  };
});

function summaryOf(range: string, hasBaseline: boolean) {
  return {
    range,
    from: "2026-09-30",
    to: "2026-10-06",
    sites: [
      {
        site: "hdsky",
        from: "2026-09-29",
        to: "2026-10-06",
        uploaded: hasBaseline ? 4 * GiB : 0,
        downloaded: hasBaseline ? 1 * GiB : 0,
        bonus: hasBaseline ? 250 : 0,
        hasBaseline,
        negative: false,
      },
    ],
    totalUploaded: hasBaseline ? 4 * GiB : 0,
    totalDownloaded: hasBaseline ? 1 * GiB : 0,
    totalBonus: hasBaseline ? 250 : 0,
  };
}

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

async function mountExport() {
  const Page = (await import("./UserDataExport.vue")).default;
  view = mountView(Page);
}

/** 预览里汇总区每一格的「标签 → 数值」 */
function posterStats(): Record<string, string> {
  const out: Record<string, string> = {};
  for (const el of document.querySelectorAll(".pstat")) {
    const label = (el.querySelector(".pstat__l")?.textContent ?? "").replace(/\s+/g, " ").trim();
    out[label.replace(/^\S+\s/, "")] = (el.querySelector(".pstat__v")?.textContent ?? "").trim();
  }
  return out;
}

describe("分享图：周期增量", () => {
  it("有可比快照：汇总区多出本周上传、下载、魔力三格", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", true));
    await mountExport();

    await vi.waitFor(() => expect(posterStats()["本周上传"]).toBe("+4 GB"));
    expect(api.getSummary).toHaveBeenCalledWith("7d");
    expect(posterStats()["本周下载"]).toBe("+1 GB");
    expect(posterStats()["本周魔力"]).toBe("+250");
  });

  it("关掉开关：三格撤下", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", true));
    await mountExport();
    await vi.waitFor(() => expect(posterStats()).toHaveProperty("本周上传"));

    document.querySelector<HTMLElement>("[data-testid=export-increments]")!.click();
    await vi.waitFor(() => expect(posterStats()).not.toHaveProperty("本周上传"));
  });

  it("还没有可比快照：不放增量，免得在图上印一排 +0", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", false));
    await mountExport();
    await vi.waitFor(() => expect(api.getSummary).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 0));
    expect(Object.keys(posterStats()).some((k) => k.startsWith("本周"))).toBe(false);
  });

  it("切到本月：重新取本月的增量，标签跟着换", async () => {
    api.getSummary.mockImplementation((range: string) => Promise.resolve(summaryOf(range, true)));
    await mountExport();
    await vi.waitFor(() => expect(posterStats()).toHaveProperty("本周上传"));

    const chip = document.querySelector("[data-testid=export-increment-range]")!;
    (chip.querySelector<HTMLElement>(".el-select__wrapper") ?? (chip as HTMLElement)).click();
    await vi.waitFor(() => {
      const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
        (o) => (o.textContent ?? "").trim() === "周期: 本月",
      );
      expect(opt).toBeTruthy();
      opt!.click();
    });
    await vi.waitFor(() => expect(api.getSummary).toHaveBeenCalledWith("30d"));
    await vi.waitFor(() => expect(posterStats()).toHaveProperty("本月上传"));
    expect(posterStats()).not.toHaveProperty("本周上传");
  });
});
