import { describe, expect, it } from "vitest";

import { freeEndLabel, freeStatusReasons, type FreeStatusFields } from "./freeStatus";

const base: FreeStatusFields = { isExpired: false, isPushed: false, freeEndTime: "" };
const now = new Date("2026-10-16T12:00:00+08:00");

describe("freeEndLabel", () => {
  it("免费到某时显示时间，悬停带读取时间与计划暂停", () => {
    const got = freeEndLabel(
      {
        ...base,
        freeValidity: "free_until",
        freeEndTime: "2026-10-17T20:00:00+08:00",
        freeVerifiedAt: "2026-10-16T11:58:00+08:00",
        pauseOnFreeEnd: true,
        pausePlannedAt: "2026-10-17T19:55:00+08:00",
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
    expect(
      freeEndLabel({ ...base, isExpired: true, freeEndTime: "2026-10-16T08:00:00+08:00" }, now)
        .tone,
    ).toBe("dang");
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
