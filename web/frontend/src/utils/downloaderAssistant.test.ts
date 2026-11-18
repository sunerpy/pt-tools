import { describe, expect, it } from "vitest";

import { applySummary, applyTone, deadReasonLabel } from "./downloaderAssistant";

describe("downloaderAssistant utils", () => {
  it("labels dead reasons and keeps unknown ones as is", () => {
    expect(deadReasonLabel("unregistered")).toBe("未注册");
    expect(deadReasonLabel("not_found")).toBe("不存在");
    expect(deadReasonLabel("other")).toBe("other");
  });

  it("summarizes apply results", () => {
    expect(applySummary("删除", { done: 3, skipped: [], failed: [] })).toBe("删除 3 个");
    expect(applySummary("删除", { done: 3, skipped: [], failed: [], kept_data: 1 })).toBe(
      "删除 3 个，其中 1 个的数据还被别的种子用着，只删了种子",
    );
    expect(
      applySummary("替换", {
        done: 1,
        skipped: [{ hash: "a", error: "x" }],
        failed: [{ hash: "b", name: "B", error: "409" }],
      }),
    ).toBe("替换 1 个，跳过 1 个（a：x），失败 1 个（B：409）");
    expect(
      applySummary("删除", {
        done: 0,
        skipped: [
          { hash: "c", name: "C", error: "tracker 又正常了，没有删除" },
          { hash: "d", error: "y" },
        ],
        failed: [],
      }),
    ).toBe("删除 0 个，跳过 2 个（C：tracker 又正常了，没有删除）");
  });

  it("picks a tone", () => {
    expect(applyTone({ done: 2, skipped: [], failed: [] })).toBe("success");
    expect(applyTone({ done: 0, skipped: [{ hash: "a", error: "x" }], failed: [] })).toBe("info");
    expect(applyTone({ done: 1, skipped: [], failed: [{ hash: "b", error: "e" }] })).toBe(
      "warning",
    );
  });
});
