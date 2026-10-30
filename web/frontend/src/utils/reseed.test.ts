import { describe, expect, it } from "vitest";

import {
  applySiteSelection,
  RECORD_RESULT_OPTIONS,
  reseedAttemptResult,
  reseedRecordActive,
  reseedRecordCancelable,
  reseedRecordMessage,
  reseedRecordResult,
  reseedRecordRetryable,
  reseedIYUUCallsText,
  reseedIYUUEndpoint,
  reseedRunDuration,
  reseedRunProgress,
  reseedRunStatus,
  reseedRunTrigger,
  reseedSiteStatus,
  reseedStepKind,
} from "./reseed";

describe("reseed utils", () => {
  it("labels records by the server's result", () => {
    expect(reseedRecordResult({ result: "failed" }).label).toBe("失败");
    // 站点上已经删了的种子不算失败
    expect(reseedRecordResult({ result: "gone" })).toEqual({
      label: "站点已删除",
      tone: "neutral",
    });
    expect(reseedRecordResult({ result: "done", job_state: "source_removed" })).toEqual({
      label: "已辅种",
      tone: "ok",
    });
    expect(reseedRecordResult({ result: "rolled_back" }).tone).toBe("warn");
    expect(reseedRecordResult({ result: "canceled" }).label).toBe("已取消");
    expect(reseedRecordResult({ result: "missing" }).label).toBe("任务已删除");
    // 任务还没结束：写走到哪一步
    expect(reseedRecordResult({ result: "active", job_state: "checking" })).toEqual({
      label: "校验中",
      tone: "info",
    });
    expect(reseedRecordResult({ result: "active" }).label).toBe("已建任务");
    expect(RECORD_RESULT_OPTIONS.map((o) => o.value)).toEqual([
      "active",
      "done",
      "failed",
      "rolled_back",
      "canceled",
      "gone",
      "missing",
    ]);
  });

  it("tells messages and which actions a record allows", () => {
    expect(reseedRecordMessage({ message: "文件不一致", job_message: "x" })).toBe("文件不一致");
    expect(reseedRecordMessage({ message: "", job_message: "正在校验" })).toBe("正在校验");
    expect(reseedRecordActive({ result: "active" })).toBe(true);
    expect(reseedRecordActive({ result: "done" })).toBe(false);
    expect(reseedRecordActive({ result: "missing" })).toBe(false);
    for (const result of ["failed", "gone", "rolled_back", "canceled", "missing"] as const)
      expect(reseedRecordRetryable({ result })).toBe(true);
    expect(reseedRecordRetryable({ result: "done" })).toBe(false);
    expect(reseedRecordRetryable({ result: "active" })).toBe(false);
    // 只有还没加入下载器的任务能取消
    expect(reseedRecordCancelable({ result: "active", job_state: "pending" })).toBe(true);
    expect(reseedRecordCancelable({ result: "active", job_state: "exported" })).toBe(true);
    expect(reseedRecordCancelable({ result: "active", job_state: "checking" })).toBe(false);
    expect(reseedRecordCancelable({ result: "failed" })).toBe(false);
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
    expect(reseedStepKind("skip")).toBe("跳过");
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

  it("describes IYUU calls", () => {
    expect(reseedIYUUEndpoint("report")).toBe("报告站点");
    expect(reseedIYUUEndpoint("x")).toBe("x");
    const row = { day: "2026-10-30", ok: 0, rate_limited: 0, failed: 0, hashes: 0 };
    expect(reseedIYUUCallsText([])).toBe("");
    expect(
      reseedIYUUCallsText([
        { ...row, endpoint: "sites", calls: 1 },
        { ...row, endpoint: "report", calls: 0 },
        { ...row, endpoint: "query", calls: 2, hashes: 400, failed: 1 },
      ]),
    ).toBe("查询 2 次（400 个种子，失败 1 次） · 站点列表 1 次");
  });
});
