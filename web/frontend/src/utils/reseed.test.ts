import { describe, expect, it } from "vitest";

import {
  reseedRecordActive,
  reseedRecordMessage,
  reseedRecordResult,
  reseedSiteStatus,
} from "./reseed";

describe("reseed utils", () => {
  it("labels records by their job", () => {
    expect(reseedRecordResult({ state: "failed" }).label).toBe("失败");
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
});
