/*
 * 钉子：订阅日历的日期范围（周一开始，月视图是整周）、翻页、范围文字、分组与一项的写法。
 */
import { describe, expect, it } from "vitest";

import type { CalendarEntry } from "@/api";

import {
  dayKey,
  entryCode,
  entryDetail,
  entryTime,
  groupByDay,
  parseDay,
  rangeDays,
  rangeLabel,
  shiftAnchor,
  startOfWeek,
  weekdayOf,
} from "./calendar";

describe("日期", () => {
  it("读写 YYYY-MM-DD", () => {
    expect(dayKey(new Date(2026, 10, 3))).toBe("2026-11-03");
    expect(dayKey(parseDay("2026-02-28")!)).toBe("2026-02-28");
    expect(parseDay("2026-02-30"), "没有这一天").toBeNull();
    expect(parseDay("2026-2-3")).toBeNull();
    expect(parseDay(undefined)).toBeNull();
    expect(weekdayOf("2026-11-03")).toBe("周二");
    expect(weekdayOf("bad")).toBe("");
  });

  it("周一开始；月视图是这个月所在的整周", () => {
    expect(dayKey(startOfWeek(new Date(2026, 10, 3)))).toBe("2026-11-02");
    expect(dayKey(startOfWeek(new Date(2026, 10, 8))), "周日算这一周").toBe("2026-11-02");
    const week = rangeDays("week", new Date(2026, 10, 3));
    expect(week).toHaveLength(7);
    expect(week[0]).toBe("2026-11-02");
    expect(week[6]).toBe("2026-11-08");
    const nov = rangeDays("month", new Date(2026, 10, 15));
    expect(nov[0]).toBe("2026-10-26");
    expect(nov.at(-1)).toBe("2026-12-06");
    expect(nov).toHaveLength(42);
    // 2027 年 2 月 1 号是周一、28 号是周日：正好四周
    expect(rangeDays("month", new Date(2027, 1, 10))).toHaveLength(28);
  });

  it("翻页与范围文字", () => {
    expect(dayKey(shiftAnchor("week", new Date(2026, 10, 3), 1))).toBe("2026-11-10");
    expect(dayKey(shiftAnchor("month", new Date(2026, 0, 31), 1)), "31 号翻到 2 月不跳过").toBe(
      "2026-02-01",
    );
    expect(rangeLabel("month", new Date(2026, 10, 3))).toBe("2026 年 11 月");
    expect(rangeLabel("week", new Date(2026, 10, 3))).toBe("11-02 – 11-08");
    expect(rangeLabel("week", new Date(2026, 11, 30))).toBe("2026-12-28 – 2027-01-03");
  });
});

describe("一项", () => {
  const ep: CalendarEntry = {
    subscription_id: 1,
    media_type: "tv",
    tmdb_id: 100088,
    title: "最后生还者",
    season: 2,
    episode: 3,
    name: "路",
    air_date: "2026-11-03",
    air_at: new Date(2026, 10, 3, 21, 30).toISOString(),
    state: "upcoming",
  };
  const movie: CalendarEntry = {
    subscription_id: 2,
    media_type: "movie",
    tmdb_id: 1,
    title: "沙丘2",
    air_date: "2026-11-03",
    state: "missing",
  };

  it("集号、播出时刻与说明", () => {
    expect(entryCode(ep)).toBe("S02E03");
    expect(entryCode(movie)).toBe("上映");
    expect(entryTime(ep)).toBe("21:30");
    expect(entryTime(movie)).toBe("");
    expect(entryTime({ ...ep, air_at: "bad" })).toBe("");
    expect(entryDetail(ep)).toBe("第 2 季 E03 · 路");
    expect(entryDetail({ ...ep, name: "" })).toBe("第 2 季 E03");
    expect(entryDetail(movie)).toBe("电影上映");
  });

  it("按天分组", () => {
    const g = groupByDay([ep, movie, { ...ep, episode: 4, air_date: "2026-11-04" }]);
    expect(Object.keys(g)).toEqual(["2026-11-03", "2026-11-04"]);
    expect(g["2026-11-03"]).toHaveLength(2);
  });
});
