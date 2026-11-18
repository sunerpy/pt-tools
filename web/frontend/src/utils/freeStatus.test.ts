import { describe, expect, it } from "vitest";

import { freeEndLabel, freeStatusReasons, type FreeStatusFields } from "./freeStatus";

const base: FreeStatusFields = { isExpired: false, isPushed: false, freeEndTime: "" };
// 用本地时间构造：显示按浏览器所在时区，CI 跑在 UTC
const now = new Date(2026, 9, 16, 12, 0, 0);
const local = (d: number, h: number, m = 0) => new Date(2026, 9, d, h, m, 0).toISOString();

describe("freeEndLabel", () => {
  it("免费到某时显示时间，悬停带读取时间与计划暂停", () => {
    const got = freeEndLabel(
      {
        ...base,
        freeValidity: "free_until",
        freeEndTime: local(17, 20),
        freeVerifiedAt: local(16, 11, 58),
        pauseOnFreeEnd: true,
        pausePlannedAt: local(17, 19, 55),
      },
      now,
    );
    expect(got.text).toBe("10-17 20:00");
    expect(got.tone).toBe("mute");
    expect(got.title).toContain("资格读取于");
    expect(got.title).toContain("计划暂停");
  });

  it("结束时间未知不显示成长期免费", () => {
    const got = freeEndLabel(
      { ...base, freeValidity: "unknown", freeStatusError: "endTime: 时间格式无法解析" },
      now,
    );
    expect(got.text).toBe("结束时间未知");
    expect(got.tone).toBe("warn");
    expect(got.title).toContain("时间格式无法解析");
  });

  it("只有接口明确长期免费才写长期免费", () => {
    expect(freeEndLabel({ ...base, freeValidity: "free_permanent" }, now).text).toBe("长期免费");
  });

  it("免费已结束用醒目颜色；没有结束时间的普通记录显示 -", () => {
    expect(freeEndLabel({ ...base, isExpired: true, freeEndTime: local(16, 8) }, now).tone).toBe(
      "dang",
    );
    expect(freeEndLabel({ ...base, freeValidity: "not_free" }, now).text).toBe("-");
  });
});

describe("freeStatusReasons", () => {
  it("要求免费、结束时间未知、还没推送的提示原因", () => {
    expect(freeStatusReasons({ ...base, requireFree: true, freeValidity: "unknown" })).toEqual([
      "免费结束时间未知，要求免费的暂不推送",
    ]);
    expect(freeStatusReasons({ ...base, requireFree: false, freeValidity: "unknown" })).toEqual([]);
  });

  it("到期暂停失败计入需要关注", () => {
    expect(freeStatusReasons({ ...base, isPushed: true, freeEndFailures: 3 })).toEqual([
      "到期暂停连续失败 3 次，仍在重试",
    ]);
  });
});
