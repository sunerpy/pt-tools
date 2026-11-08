import { describe, expect, it } from "vitest";
import type { SiteAttendance } from "@/api";
import { attendanceChallengeText, attendanceView } from "./attendanceStatus";

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
    expect(attendanceView(att({ site_enabled: false })).label).toBe("站点未启用");
  });

  // issue #862：自动签到关了（或站点停用了）以后，签到监控不再处理这个站点，今天的「待重试」不会发生
  it("does not promise a retry once automatic sign-in is switched off", () => {
    const off = attendanceView(
      att({
        status: "pending",
        attendance_enabled: false,
        attempts: 1,
        last_error: "签到页面里没有签到成功或今日已签到的文字",
        next_attempt_at: 1_759_626_600,
      }),
    );
    expect(off).toMatchObject({ label: "未开启", tone: "neutral", canSign: true });
    expect(off.detail).toContain("没有开启每日自动签到");
    expect(off.detail).toContain("签到页面里没有签到成功");
    expect(off.detail).not.toContain("重试");

    const disabled = attendanceView(
      att({ status: "pending", site_enabled: false, scheduled_at: 1_759_626_000 }),
    );
    expect(disabled).toMatchObject({ label: "站点未启用", tone: "neutral", canSign: false });
    expect(disabled.detail).not.toMatch(/计划/);

    // 今天已经有结果的照旧显示结果
    expect(attendanceView(att({ status: "failed", attendance_enabled: false })).label).toBe(
      "签到失败",
    );
    expect(attendanceView(att({ status: "signed", site_enabled: false })).label).toBe("已签到");
  });
});

describe("attendanceChallengeText", () => {
  it("要过验证码或答题的站点写明用 AI，其余站点不写", () => {
    expect(attendanceChallengeText("hdsky_captcha")).toContain("识别验证码");
    expect(attendanceChallengeText("opencd_captcha")).toContain("看图模型");
    expect(attendanceChallengeText("baka_quiz")).toContain("答题");
    expect(attendanceChallengeText(undefined)).toBe("");
  });
});
