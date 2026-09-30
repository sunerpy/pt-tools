/*
 * 钉子：运行日志的级别判定。
 *
 * 之前先对整行跑 `\berror\b`，JSON 行里真正的 level 字段排在后面 —— 一条
 * `{"level":"warn",…,"msg":"retry after error"}` 被归成 ERROR：筛 WARN 看不到它，左栏计数也不准。
 * 规则：有 JSON level 字段就只认它；没有才按纯文本里的级别词猜。
 */
import { describe, expect, it } from "vitest";

import { levelOf } from "./logLevel";

describe("levelOf", () => {
  it("JSON 行只认 level 字段，不被消息里的字样带偏", () => {
    expect(levelOf('{"level":"warn","msg":"retry after error"}')).toBe("warn");
    expect(levelOf('{"level":"info","msg":"warning: disk almost full"}')).toBe("info");
    expect(levelOf('{"level":"debug","msg":"got error=nil"}')).toBe("debug");
    expect(levelOf('{"level":"error","msg":"ok"}')).toBe("error");
  });

  it("zap 的 dpanic / panic / fatal 归到 ERROR，warning 归到 WARN", () => {
    expect(levelOf('{"level":"dpanic","msg":"x"}')).toBe("error");
    expect(levelOf('{"level":"fatal","msg":"x"}')).toBe("error");
    expect(levelOf('{"level":"warning","msg":"x"}')).toBe("warn");
  });

  it("level 字段允许冒号后有空格、大小写不敏感", () => {
    expect(levelOf('{"level": "WARN", "msg": "error"}')).toBe("warn");
  });

  it("没有 JSON level 字段时按纯文本级别词猜", () => {
    expect(levelOf("2026-09-30T08:00:00.000+0800\tERROR\tsomething broke")).toBe("error");
    expect(levelOf("2026-09-30T08:00:00.000+0800\tWARN\tslow")).toBe("warn");
    expect(levelOf("2026-09-30T08:00:00.000+0800\tINFO\tstarted")).toBe("info");
    expect(levelOf("plain line without a level")).toBe("other");
  });

  it("未知的 level 值不硬猜", () => {
    expect(levelOf('{"level":"trace","msg":"error"}')).toBe("other");
  });
});
