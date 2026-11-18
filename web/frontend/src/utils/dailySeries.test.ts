import { describe, expect, it } from "vitest";

import type { DailyPoint } from "@/api";

import { dailyUploadSeries, nextDay } from "./dailySeries";

function point(date: string, deltaUploaded: number, spanDays: number): DailyPoint {
  return {
    date,
    uploaded: 0,
    downloaded: 0,
    bonus: 0,
    seeding: 0,
    deltaUploaded,
    deltaDownloaded: 0,
    deltaBonus: 0,
    spanDays,
    negative: false,
  };
}

describe("dailyUploadSeries", () => {
  it("缺快照的日子补 0，跨天的增量记在快照那天，没有基线的第一份不算", () => {
    const series = dailyUploadSeries(
      [point("2026-10-01", 0, 0), point("2026-10-02", 5, 1), point("2026-10-05", 9, 3)],
      "2026-10-01",
      "2026-10-06",
    );
    expect(series).toEqual([0, 5, 0, 0, 9, 0]);
  });

  it("跨月、跨年按日历走", () => {
    expect(nextDay("2026-02-28")).toBe("2026-03-01");
    expect(nextDay("2026-12-31")).toBe("2027-01-01");
    expect(dailyUploadSeries([point("2027-01-01", 3, 1)], "2026-12-31", "2027-01-01")).toEqual([
      0, 3,
    ]);
  });

  it("日期不合法或倒置时返回空数组", () => {
    expect(dailyUploadSeries([], "", "2026-10-06")).toEqual([]);
    expect(dailyUploadSeries([], "2026-10-07", "2026-10-06")).toEqual([]);
  });
});
