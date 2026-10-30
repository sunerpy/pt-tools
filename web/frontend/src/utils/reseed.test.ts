import { describe, expect, it } from "vitest";

import {
  applySiteSelection,
  reseedAttemptResult,
  reseedRecordActive,
  reseedRecordMessage,
  reseedRecordResult,
  reseedRunDuration,
  reseedRunProgress,
  reseedRunStatus,
  reseedRunTrigger,
  reseedSiteStatus,
  reseedStepKind,
} from "./reseed";

describe("reseed utils", () => {
  it("labels records by their job", () => {
    expect(reseedRecordResult({ state: "failed" }).label).toBe("失败");
    // 站点上已经删了的种子不算失败
    expect(reseedRecordResult({ state: "gone" })).toEqual({ label: "站点已删除", tone: "neutral" });
    expect(reseedRecordResult({ state: "queued", job_state: "source_removed" })).toEqual({
      label: "已辅种",
      tone: "ok",
    });
    expect(reseedRecordResult({ state: "queued", job_state: "rolled_back" }).tone).toBe("warn");
    expect(reseedRecordResult({ state: "queued", job_state: "failed" }).tone).toBe("dang");
    expect(reseedRecordResult({ state: "queued", job_state: "canceled" }).label).toBe("已取消");
    expect(reseedRecordResult({ state: "queued", job_state: "checking" }).label).toBe("校验中");
    expect(reseedRecordResult({ state: "queued" }).label).toBe("已建任务");
    // 任务记录清除后留下的结果
    expect(reseedRecordResult({ state: "done" })).toEqual({ label: "已辅种", tone: "ok" });
    expect(reseedRecordResult({ state: "rolled_back" }).label).toBe("已撤回");
    expect(reseedRecordResult({ state: "canceled" }).label).toBe("已取消");
    expect(reseedRecordResult({ state: "queued", job_missing: true }).label).toBe("任务已删除");
  });

  it("tells messages and activity", () => {
    expect(reseedRecordMessage({ message: "文件不一致", job_message: "x" })).toBe("文件不一致");
    expect(reseedRecordMessage({ message: "", job_message: "正在校验" })).toBe("正在校验");
    expect(reseedRecordActive({ state: "queued", job_state: "checking" })).toBe(true);
    expect(reseedRecordActive({ state: "queued", job_state: "source_removed" })).toBe(false);
    expect(reseedRecordActive({ state: "failed" })).toBe(false);
    expect(reseedRecordActive({ state: "done" })).toBe(false);
    // 任务不在了不再轮询
    expect(reseedRecordActive({ state: "queued", job_missing: true })).toBe(false);
  });

  it("applies a site selection like the server does", () => {
    const base = { iyuu_site: "x", nickname: "x", host: "x", selected: false };
    const items = [
      { ...base, sid: 1, site_name: "hdsky", configured: true },
      { ...base, sid: 2, site_name: "ourbits", configured: true },
      { ...base, sid: 3, site_name: "mteam", configured: false },
      { ...base, sid: 4, configured: false },
    ];
    expect(
      applySiteSelection(items, { site_mode: "all", site_names: [] }).map((i) => i.selected),
    ).toEqual([true, true, false, false]);
    expect(
      applySiteSelection(items, { site_mode: "selected", site_names: ["ourbits", "mteam"] }).map(
        (i) => i.selected,
      ),
    ).toEqual([false, true, false, false]);
    // 全部关掉：空名单不是「全部」
    expect(
      applySiteSelection(items, { site_mode: "selected", site_names: [] }).some((i) => i.selected),
    ).toBe(false);
  });

  it("describes site status", () => {
    const base = { sid: 1, iyuu_site: "x", nickname: "x", host: "x" };
    expect(reseedSiteStatus({ ...base, configured: false, selected: false }).label).toBe(
      "pt-tools 不支持",
    );
    expect(
      reseedSiteStatus({ ...base, site_name: "hdsky", configured: false, selected: false }).label,
    ).toBe("没有配置");
    expect(
      reseedSiteStatus({ ...base, site_name: "hdsky", configured: true, selected: false }).label,
    ).toBe("没有选中");
    expect(
      reseedSiteStatus({ ...base, site_name: "hdsky", configured: true, selected: true }).tone,
    ).toBe("ok");
  });

  it("describes runs and attempts", () => {
    expect(reseedRunStatus("limited")).toEqual({ label: "提前停下", tone: "warn" });
    expect(reseedRunStatus("paused").label).toBe("已暂停");
    expect(reseedRunStatus("weird").label).toBe("weird");
    expect(reseedRunTrigger("continue")).toBe("继续");
    expect(reseedRunTrigger("retry")).toBe("重试");
    expect(reseedStepKind("query")).toBe("查询");
    // 用时不含暂停的时间；没跑完的（被打断）不写
    expect(
      reseedRunDuration({
        started_at: "2026-10-30T10:00:00Z",
        finished_at: "2026-10-30T10:05:00Z",
        status: "ok",
        paused_seconds: 60,
      }),
    ).toBe("4 分");
    expect(
      reseedRunDuration({
        started_at: "2026-10-30T10:00:00Z",
        finished_at: "2026-10-30T10:05:00Z",
        status: "interrupted",
        paused_seconds: 0,
      }),
    ).toBe("");
    const cur = {
      id: 1,
      trigger: "manual" as const,
      status: "running" as const,
      started_at: "",
      hashes: 10,
      batches: 3,
      batches_done: 3,
      candidates: 2,
      created: 1,
      failed: 1,
      gone: 0,
      skipped: 0,
    };
    expect(reseedRunProgress(cur)).toBe("第 3/3 批 · 加入 1 · 失败 1");
    expect(reseedRunProgress({ ...cur, trigger: "retry", batches: 0 })).toBe(
      "重试 10 条 · 加入 1 · 失败 1",
    );
    expect(reseedAttemptResult({ outcome: "active" }).label).toBe("已有任务");
    expect(reseedAttemptResult({ outcome: "created", job_state: "checking" }).label).toBe("校验中");
    expect(reseedAttemptResult({ outcome: "created", job_missing: true }).label).toBe("任务已删除");
    expect(reseedAttemptResult({ outcome: "created" }).label).toBe("已建任务");
  });
});
