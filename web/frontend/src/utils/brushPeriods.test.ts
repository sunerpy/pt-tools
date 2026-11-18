// @vitest-environment happy-dom
import { afterEach, describe, expect, it } from "vitest";

import {
  brushPeriodLong,
  brushPeriodMeta,
  brushPeriodShort,
  loadBrushPeriod,
  saveBrushPeriod,
} from "./brushPeriods";

afterEach(() => localStorage.clear());

describe("刷流收益的统计周期", () => {
  it("每种周期的叫法与段数", () => {
    expect(brushPeriodMeta("week")).toMatchObject({
      current: "本周",
      previous: "上周",
      periods: 12,
    });
    expect(brushPeriodMeta("quarter").current).toBe("本季度");
    expect(brushPeriodMeta("bogus").value).toBe("day");
    expect(brushPeriodMeta(null).window).toBe("最近 30 天");
  });

  it("柱子下面的短标签与读数里的完整写法", () => {
    expect(brushPeriodShort("day", "2026-10-29")).toBe("10/29");
    expect(brushPeriodShort("week", "2026-10-26")).toBe("10/26");
    expect(brushPeriodShort("month", "2026-10-01")).toBe("10月");
    expect(brushPeriodShort("month", "2026-01-01")).toBe("2026年1月");
    expect(brushPeriodShort("quarter", "2026-10-01")).toBe("Q4");
    expect(brushPeriodShort("quarter", "2026-01-01")).toBe("2026 Q1");
    expect(brushPeriodShort("year", "2026-01-01")).toBe("2026");
    expect(brushPeriodLong("day", "2026-10-29", "2026-10-29")).toBe("10月29日");
    expect(brushPeriodLong("week", "2026-10-26", "2026-11-01")).toBe("10月26日–11月1日");
    expect(brushPeriodLong("month", "2026-10-01", "2026-10-31")).toBe("2026年10月");
    expect(brushPeriodLong("quarter", "2026-07-01", "2026-09-30")).toBe("2026年第3季度");
    expect(brushPeriodLong("year", "2026-01-01", "2026-12-31")).toBe("2026年");
    expect(brushPeriodShort("day", "bad")).toBe("bad");
  });

  it("记住上次选的周期", () => {
    expect(loadBrushPeriod()).toBe("day");
    saveBrushPeriod("month");
    expect(loadBrushPeriod()).toBe("month");
    localStorage.setItem("pt-brush-period", "hour");
    expect(loadBrushPeriod()).toBe("day");
  });
});
