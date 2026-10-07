import { describe, expect, it } from "vitest";

import {
  defaultTransferRule,
  transferCancelable,
  transferPreviewSummary,
  transferProgressText,
  transferRuleSummary,
  transferSourceLabel,
  transferStateLabel,
  transferStateTone,
} from "./transfer";

describe("transfer utils", () => {
  it("labels states and keeps unknown ones as is", () => {
    expect(transferStateLabel("checking")).toBe("校验中");
    expect(transferStateLabel("source_removed")).toBe("已完成");
    expect(transferStateLabel("other")).toBe("other");
    expect(transferStateTone("failed")).toBe("dang");
    expect(transferStateTone("rolled_back")).toBe("warn");
    expect(transferStateTone("other")).toBe("neutral");
  });

  it("only allows canceling before the torrent is added", () => {
    expect(transferCancelable({ state: "pending" })).toBe(true);
    expect(transferCancelable({ state: "exported" })).toBe(true);
    expect(transferCancelable({ state: "checking" })).toBe(false);
  });

  it("formats progress and sources", () => {
    expect(transferProgressText({ state: "checking", progress: 0.4567 })).toBe("45.7%");
    expect(transferProgressText({ state: "checking", progress: 0 })).toBe("");
    expect(transferProgressText({ state: "verified", progress: 1 })).toBe("");
    expect(transferSourceLabel({ source: "export" })).toBe("从下载器导出");
    expect(transferSourceLabel({ source: "site" })).toBe("从站点重新下载");
    expect(transferSourceLabel({})).toBe("");
  });

  it("summarizes previews", () => {
    const base = {
      source_id: 1,
      source_name: "",
      hash: "",
      name: "",
      size: 0,
      save_path: "",
      target_path: "",
      mapped: false,
    };
    expect(
      transferPreviewSummary([
        { ...base, ok: true },
        { ...base, ok: false },
      ]),
    ).toBe("能转 1 个，跳过 1 个");
    expect(transferPreviewSummary([{ ...base, ok: true }])).toBe("能转 1 个");
  });

  it("summarizes rule conditions", () => {
    const r = {
      ...defaultTransferRule(),
      category: "movies",
      tag: "done",
      site_name: "hdsky",
      min_seeding_hours: 48,
    };
    expect(transferRuleSummary(r)).toBe("分类 movies、标签含 done、站点 hdsky、做种满 48 小时");
    expect(transferRuleSummary(defaultTransferRule())).toBe("全部已下完的种子");
    expect(defaultTransferRule().enabled).toBe(false);
  });
});
