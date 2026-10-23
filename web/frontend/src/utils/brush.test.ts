import { describe, expect, it } from "vitest";

import type { BrushTask } from "@/api";

import {
  brushSectionFor,
  configOf,
  defaultBrushConfig,
  discountLabel,
  hasBrushSection,
  limitSummary,
  nextRunAt,
  removalSummary,
  runMessage,
  taskDiscounts,
} from "./brush";

describe("brushSectionFor", () => {
  it("只有 M-Team 有分区：空值按综合区，别的站点一律为空", () => {
    expect(hasBrushSection("mteam")).toBe(true);
    expect(hasBrushSection("hdsky")).toBe(false);
    expect(brushSectionFor("mteam", "")).toBe("normal");
    expect(brushSectionFor("mteam", "all")).toBe("all");
    expect(brushSectionFor("hdsky", "adult")).toBe("");
  });
});

describe("brush utils", () => {
  it("默认配置：关闭、只收免费、排除 H&R、免费到期未完成就删", () => {
    const c = defaultBrushConfig();
    expect(c.enabled).toBe(false);
    expect(taskDiscounts(c.discounts)).toEqual(["FREE", "2XFREE"]);
    expect(c.exclude_hr).toBe(true);
    expect(c.remove_free_expired_incomplete).toBe(true);
    expect(c.max_downloading).toBe(3);
  });

  it("优惠类型：空串按只收免费，大小写不敏感", () => {
    expect(taskDiscounts("")).toEqual(["FREE", "2XFREE"]);
    expect(taskDiscounts(" free, percent_50 ")).toEqual(["FREE", "PERCENT_50"]);
    expect(discountLabel("2xfree")).toBe("2x 免费");
    expect(discountLabel("WEIRD")).toBe("WEIRD");
  });

  it("删种与限额的说明", () => {
    const c = defaultBrushConfig();
    expect(removalSummary(c)).toBe("免费到期时尚未下完");
    c.remove_free_expired_incomplete = false;
    expect(removalSummary(c)).toBe("不会自动删种");
    c.remove_ratio = 3;
    c.remove_low_speed_kbs = 50;
    expect(removalSummary(c)).toBe("分享率到 3、30 分钟平均上传低于 50 KB/s");
    c.max_total_size_gb = 500;
    expect(limitSummary(c)).toBe("同时下载 3 个 · 总体积 500 GB");
  });

  it("运行结果的提示：出错时优先报错误", () => {
    const r = {
      task_id: 1,
      sampled: 2,
      removed: 1,
      gone: 0,
      listed: 40,
      eligible: 3,
      added: 2,
      stopped: "同时下载已有 3 个，达到上限",
    };
    expect(runMessage(r)).toEqual({
      tone: "ok",
      text: "列表 40 个，符合条件 3 个，加入 2 个；删除 1 个；同时下载已有 3 个，达到上限",
    });
    expect(runMessage({ ...r, errors: ["下载失败"] }).tone).toBe("warn");
    expect(runMessage(r, "站点 502").tone).toBe("error");
  });

  it("一个都不符合时写出前两条原因，H&R 要求未知时提示去站点设置填写（#727）", () => {
    const r = {
      task_id: 1,
      sampled: 0,
      removed: 0,
      gone: 0,
      listed: 38,
      eligible: 0,
      added: 0,
      rejected: [
        { reason: "H&R 要求的做种时长未知", count: 36 },
        { reason: "优惠类型 PERCENT_50 不在允许范围", count: 1 },
        { reason: "优惠已到期", count: 1 },
      ],
    };
    expect(runMessage(r).text).toBe(
      "列表 38 个，符合条件 0 个，加入 0 个；不符合的原因：H&R 要求的做种时长未知 36 个、优惠类型 PERCENT_50 不在允许范围 1 个（可以在站点设置里填写 H&R 做种要求）",
    );
    expect(runMessage({ ...r, eligible: 2 }).text).toBe("列表 38 个，符合条件 2 个，加入 0 个");
  });

  it("配置从任务里取回，下次运行时间按间隔算", () => {
    const task = {
      ...defaultBrushConfig(),
      id: 3,
      name: "x",
      last_run_at: "2026-10-06T10:00:00Z",
      interval_min: 10,
    } as BrushTask;
    expect(configOf(task).name).toBe("x");
    expect(configOf(task)).not.toHaveProperty("id");
    expect(nextRunAt(task)).toBe(Date.parse("2026-10-06T10:10:00Z"));
    expect(nextRunAt({ ...task, last_run_at: undefined })).toBeNull();
  });
});
