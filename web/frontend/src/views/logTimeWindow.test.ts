/*
 * 钉子：画板 29 的「最近 1 小时」。
 *
 * 最要紧的一条是最后那个 case —— 一批行里一个时间戳都解析不出来时，这枚 chip
 * 必须停用而不是把整页筛空。「点了之后什么都没有」会被读成「最近一小时没日志」，
 * 那是一句假话。
 */
import { describe, expect, it } from "vitest";

import { hasParsableTime, lineTime, withinWindow } from "./logTimeWindow";

const at = (iso: string, msg = "hello") => `{"level":"info","time":"${iso}","msg":"${msg}"}`;

describe("lineTime", () => {
  it("读 JSON 编码器的 time 字段", () => {
    expect(lineTime(at("2026-09-19T10:00:00.000+0800"))).toBe(
      Date.parse("2026-09-19T10:00:00.000+0800"),
    );
  });

  it("没有 time 字段就返回 null，不返回 0", () => {
    expect(lineTime("panic: runtime error")).toBeNull();
    expect(lineTime("")).toBeNull();
  });

  it("time 字段解析不出来也返回 null", () => {
    expect(lineTime('{"time":"昨天下午"}')).toBeNull();
  });
});

describe("withinWindow", () => {
  const base = Date.parse("2026-09-19T12:00:00.000Z");
  const lines = [
    at("2026-09-19T09:00:00.000Z", "三小时前"),
    at("2026-09-19T11:30:00.000Z", "半小时前"),
    "    /go/src/main.go:42 +0x1f  <- 没有时间戳的续行",
  ];

  it("不开窗就一行不动", () => {
    expect(withinWindow(lines, null)).toEqual(lines);
  });

  it("开一小时窗：滤掉三小时前那条", () => {
    const got = withinWindow(lines, base - 3600_000);
    expect(got).toHaveLength(2);
    expect(got[0]).toContain("半小时前");
  });

  it("解析不出时间戳的续行要留着 —— 丢掉它等于藏起报错的上下文", () => {
    expect(withinWindow(lines, base - 3600_000).at(-1)).toContain("+0x1f");
  });
});

describe("hasParsableTime", () => {
  it("有一行能解析就够", () => {
    expect(hasParsableTime(["plain", at("2026-09-19T10:00:00Z")])).toBe(true);
  });

  it("一行都解析不出来 → false（chip 必须停用，而不是把整页筛空）", () => {
    expect(hasParsableTime(["plain text log", "another one"])).toBe(false);
    expect(hasParsableTime([])).toBe(false);
  });
});
