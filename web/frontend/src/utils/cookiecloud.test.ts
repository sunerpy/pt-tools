import { describe, expect, it } from "vitest";

import {
  cookieCloudImportSummary,
  cookieCloudImportTone,
  cookieCloudItemStatus,
} from "./cookiecloud";

describe("cookiecloud utils", () => {
  it("labels preview items", () => {
    expect(cookieCloudItemStatus({ enabled: false, changed: true }).label).toBe("没有启用");
    expect(cookieCloudItemStatus({ enabled: true, changed: true })).toEqual({
      label: "Cookie 有变化",
      tone: "warn",
    });
    expect(cookieCloudItemStatus({ enabled: true, changed: false }).tone).toBe("neutral");
  });

  it("summarizes import results", () => {
    const empty = { imported: [], unchanged: [], missing: [], failed: [] };
    expect(cookieCloudImportSummary(empty)).toBe("没有可以更新的站点");
    expect(cookieCloudImportTone(empty)).toBe("info");
    const r = {
      imported: ["hdsky", "ourbits"],
      unchanged: ["audiences"],
      missing: ["x"],
      failed: [{ site: "y", error: "保存失败" }],
    };
    expect(cookieCloudImportSummary(r)).toBe(
      "更新了 2 个站点的 Cookie（hdsky、ourbits），1 个没有变化，1 个在 CookieCloud 里没有找到（x），1 个写入失败（y）",
    );
    expect(cookieCloudImportTone(r)).toBe("warning");
    expect(cookieCloudImportTone({ ...empty, imported: ["hdsky"] })).toBe("success");
  });
});
