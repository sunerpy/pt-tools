/*
 * 钉子：画板 29 的「最近 1 小时」。
 *
 * 最要紧的一条是最后那个 case —— 一批行里一个时间戳都解析不出来时，这枚 chip
 * 必须停用而不是把整页筛空。「点了之后什么都没有」会被读成「最近一小时没日志」，
 * 那是一句假话。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { hasParsableTime, lineTime, withinWindow } from "./logTimeWindow";

const at = (iso: string, msg = "hello") => `{"level":"info","time":"${iso}","msg":"${msg}"}`;

/*
 * Safari（JavaScriptCore）的 Date.parse 只认 ECMAScript 规定的日期时间格式，
 * 时区偏移必须写成 `+08:00`；zap 的 ISO8601 编码器写的是 `+0800`，在 Safari 上解析成 NaN。
 * V8 宽松地认了 `+0800`，所以 Node 里直接跑看不出问题 —— 这里把 Date.parse 换成
 * 只认规范格式的版本，模拟 Safari 的行为。
 */
const realParse = Date.parse.bind(Date);
const ES_DATE_TIME =
  /^\d{4}-\d{2}-\d{2}(?:T\d{2}:\d{2}(?::\d{2}(?:\.\d{1,3})?)?(?:Z|[+-]\d{2}:\d{2})?)?$/;
function strictParse(s: string): number {
  return ES_DATE_TIME.test(s) ? realParse(s) : NaN;
}

describe("lineTime", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("读 JSON 编码器的 time 字段", () => {
    expect(lineTime(at("2026-09-19T10:00:00.000+0800"))).toBe(
      Date.parse("2026-09-19T10:00:00.000+0800"),
    );
  });

  it("zap 写的 +0800 先规范成 +08:00 再解析 —— 只认规范格式的引擎（Safari）也读得出来", () => {
    vi.spyOn(Date, "parse").mockImplementation(strictParse);
    /* 东八区 10:00 = UTC 02:00 */
    expect(lineTime(at("2026-09-19T10:00:00.000+0800"))).toBe(Date.UTC(2026, 8, 19, 2, 0, 0));
    expect(lineTime(at("2026-09-19T10:00:00.000-0130"))).toBe(Date.UTC(2026, 8, 19, 11, 30, 0));
  });

  it("本来就合规的写法（Z、+08:00）原样解析，不被规范化改坏", () => {
    vi.spyOn(Date, "parse").mockImplementation(strictParse);
    expect(lineTime(at("2026-09-19T02:00:00.000Z"))).toBe(Date.UTC(2026, 8, 19, 2, 0, 0));
    expect(lineTime(at("2026-09-19T10:00:00.000+08:00"))).toBe(Date.UTC(2026, 8, 19, 2, 0, 0));
  });

  it("Safari 上「最近 1 小时」照样能筛：一批 +0800 的行不会因为全解析失败而停用 chip", () => {
    vi.spyOn(Date, "parse").mockImplementation(strictParse);
    const lines = [
      at("2026-09-19T09:00:00.000+0800", "两小时前"),
      at("2026-09-19T10:30:00.000+0800"),
    ];
    expect(hasParsableTime(lines)).toBe(true);
    /* 窗口起点：东八区 10:00 */
    expect(withinWindow(lines, Date.UTC(2026, 8, 19, 2, 0, 0))).toHaveLength(1);
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
