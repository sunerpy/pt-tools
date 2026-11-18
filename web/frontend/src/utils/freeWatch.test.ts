import { describe, expect, it } from "vitest";

import {
  checkText,
  expiryDisabled,
  expiryProblem,
  FREE_WATCH_FILTERS,
  filterLabel,
  freeWatchStatus,
} from "./freeWatch";

describe("等免费", () => {
  it("状态的叫法与颜色；不认识的原样写", () => {
    expect(freeWatchStatus("watching")).toEqual({ label: "等免费中", tone: "info" });
    expect(freeWatchStatus("pushed")).toEqual({ label: "已推送", tone: "ok" });
    expect(freeWatchStatus("failed")).toEqual({ label: "不再等", tone: "dang" });
    expect(freeWatchStatus("bogus")).toEqual({ label: "bogus", tone: "neutral" });
  });

  it("筛选档位带条数，全部写总数，没有时不写数", () => {
    const counts = { watching: 2, pushed: 3 };
    const label = (v: string) =>
      filterLabel(
        FREE_WATCH_FILTERS.find((f) => f.value === v)!,
        counts,
      );
    expect(label("")).toBe("全部 5");
    expect(label("watching")).toBe("等免费中 2");
    expect(label("expired")).toBe("到期了");
  });

  it("到期时间：今天以前、30 天以后不能选；要晚于现在、不晚于 30 天", () => {
    const now = new Date(2026, 10, 3, 12, 0);
    expect(expiryDisabled(new Date(2026, 10, 2), now)).toBe(true);
    expect(expiryDisabled(new Date(2026, 10, 3), now)).toBe(false);
    expect(expiryDisabled(new Date(2026, 11, 3), now)).toBe(false);
    expect(expiryDisabled(new Date(2026, 11, 4), now)).toBe(true);
    expect(expiryProblem(null, now)).toBe("选一个到期时间");
    expect(expiryProblem(new Date(2026, 10, 3, 11, 0), now)).toBe("到期时间要晚于现在");
    expect(expiryProblem(new Date(2026, 11, 3, 12, 1), now)).toBe("最多再等 30 天");
    expect(expiryProblem(new Date(2026, 10, 10, 12, 0), now)).toBe("");
  });

  it("看过几次", () => {
    expect(checkText({ check_count: 0 })).toBe("还没看过");
    expect(checkText({ check_count: 4 })).toBe("看过 4 次");
  });
});
