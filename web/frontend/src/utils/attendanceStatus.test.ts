import { describe, expect, it } from "vitest";
import type { SiteAttendance } from "@/api";
import { attendanceView } from "./attendanceStatus";

function att(partial: Partial<SiteAttendance>): SiteAttendance {
  return {
    site_name: "hdtime",
    site_enabled: true,
    attendance_enabled: true,
    supported: true,
    day: "2026-10-05",
    status: "",
    attempts: 0,
    ...partial,
  };
}

describe("attendanceView", () => {
  it("shows the reason for sites that cannot sign in", () => {
    const v = attendanceView(
      att({ supported: false, unsupported_reason: "签到需要验证码，暂不支持自动签到" }),
    );
    expect(v.label).toBe("不支持");
    expect(v.detail).toContain("验证码");
    expect(v.canSign).toBe(false);
  });

  it("maps today's results", () => {
    expect(attendanceView(att({ status: "signed", message: "这是您的第 9 次签到" }))).toMatchObject(
      {
        label: "已签到",
        tone: "ok",
        detail: "这是您的第 9 次签到",
      },
    );
    expect(attendanceView(att({ status: "already" })).label).toBe("已签到");
    expect(attendanceView(att({ status: "failed", last_error: "Cookie 已失效" }))).toMatchObject({
      label: "签到失败",
      tone: "dang",
      detail: "Cookie 已失效",
    });
    expect(
      attendanceView(att({ status: "unsupported", last_error: "签到需要答题" })).detail,
    ).toContain("答题");
  });

  it("explains a pending sign-in and its retry", () => {
    const planned = attendanceView(att({ status: "pending", scheduled_at: 1_759_626_000 }));
    expect(planned.label).toBe("待签到");
    expect(planned.detail).toMatch(/^计划 /);
    const retry = attendanceView(
      att({
        status: "pending",
        attempts: 1,
        last_error: "HTTP 502",
        next_attempt_at: 1_759_626_600,
      }),
    );
    expect(retry.tone).toBe("warn");
    expect(retry.detail).toContain("HTTP 502");
    expect(retry.detail).toContain("重试");
  });

  it("separates switched-off sites from sites not yet scheduled", () => {
    expect(attendanceView(att({ attendance_enabled: false })).label).toBe("未开启");
    expect(attendanceView(att({})).label).toBe("待安排");
    expect(attendanceView(undefined).label).toBe("-");
    expect(attendanceView(att({ site_enabled: false })).canSign).toBe(false);
  });
});
