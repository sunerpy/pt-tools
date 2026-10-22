import type {
  AttendanceChallenge,
  AttendanceDaySite,
  AttendanceEvent,
  AttendanceMethod,
  SiteAttendance,
} from "@/api";
import { formatShortDateTime } from "@/utils/format";

/** 与 PtStatusPill 的语义色对应 */
export type AttendanceTone = "ok" | "warn" | "dang" | "info" | "neutral";

export interface AttendanceView {
  /** 胶囊里的短文案 */
  label: string;
  tone: AttendanceTone;
  /** tooltip / 详情里的一句说明 */
  detail: string;
  /** 现在能不能点「立即签到」：站点启用且支持签到 */
  canSign: boolean;
}

function at(unix?: number): string {
  return unix ? formatShortDateTime(unix * 1000) : "";
}

/**
 * 一个站点当天签到状态的展示。后端状态见 models.SiteAttendanceLog：
 * pending 之外都是当天的最终结果；没有记录时 status 为空。
 */
export function attendanceView(a?: SiteAttendance): AttendanceView {
  if (!a) return { label: "-", tone: "neutral", detail: "签到状态未知", canSign: false };
  const canSign = a.site_enabled && a.supported;
  if (!a.supported) {
    return {
      label: "不支持",
      tone: "neutral",
      detail: a.unsupported_reason || "该站点不支持自动签到",
      canSign,
    };
  }
  switch (a.status) {
    case "signed":
      return { label: "已签到", tone: "ok", detail: a.message || "今天签到成功", canSign };
    case "already":
      return { label: "已签到", tone: "ok", detail: a.message || "今天已经签到过", canSign };
    case "failed":
      return {
        label: "签到失败",
        tone: "dang",
        detail: a.last_error || "今天的签到没有成功",
        canSign,
      };
    case "unsupported":
      return {
        label: "不支持",
        tone: "neutral",
        detail: a.last_error || a.unsupported_reason || "该站点不支持自动签到",
        canSign: false,
      };
    case "pending": {
      if (a.last_error) {
        const next = at(a.next_attempt_at);
        return {
          label: "待重试",
          tone: "warn",
          detail: next ? `${a.last_error}；${next} 重试` : `${a.last_error}；稍后重试`,
          canSign,
        };
      }
      const planned = at(a.scheduled_at);
      return {
        label: "待签到",
        tone: "info",
        detail: planned ? `计划 ${planned}` : "今天待签到",
        canSign,
      };
    }
    default:
      return a.attendance_enabled
        ? { label: "待安排", tone: "info", detail: "今天还没安排签到时间", canSign }
        : { label: "未开启", tone: "neutral", detail: "没有开启每日自动签到", canSign };
  }
}

/**
 * 签到要过的一关写给用户看的一句话（要在「自动签到」页的签到设置里打开「用 AI 签到」）；不用过的返回空串。
 */
export function attendanceChallengeText(c?: AttendanceChallenge): string {
  switch (c) {
    case "hdsky_captcha":
    case "opencd_captcha":
      return "这个站点签到要识别验证码，用 AI 助手的看图模型";
    case "baka_quiz":
      return "这个站点签到要答题，用 AI 助手的模型";
    default:
      return "";
  }
}

/** 签到方式写给人看的名字 */
export const ATTENDANCE_METHOD_LABELS: Record<AttendanceMethod, string> = {
  direct: "直接签到",
  ai_captcha: "AI 识别验证码",
  ai_quiz: "AI 答题",
};

/** 一次签到尝试的结果：胶囊里的字与颜色 */
export function attendanceEventView(e: Pick<AttendanceEvent, "result">): {
  label: string;
  tone: AttendanceTone;
} {
  switch (e.result) {
    case "signed":
      return { label: "签上了", tone: "ok" };
    case "already":
      return { label: "已签过", tone: "ok" };
    case "retry":
      return { label: "失败，稍后重试", tone: "warn" };
    case "failed":
      return { label: "失败", tone: "dang" };
    default:
      return { label: "不支持", tone: "neutral" };
  }
}

/** 签到详情里一个站点当天的结果（和「今天的签到」一样的说法，没有开关的信息） */
export function attendanceDayView(s: AttendanceDaySite): { label: string; tone: AttendanceTone } {
  switch (s.status) {
    case "signed":
    case "already":
      return { label: "已签到", tone: "ok" };
    case "failed":
      return { label: "签到失败", tone: "dang" };
    case "unsupported":
      return { label: "不支持", tone: "neutral" };
    default:
      return s.last_error ? { label: "待重试", tone: "warn" } : { label: "待签到", tone: "info" };
  }
}

/** token 数写成「1,234」 */
export function tokenText(n: number): string {
  return Math.max(0, Math.round(n)).toLocaleString("zh-CN");
}

/** AI 用量的一句话，例如「vision-x · 调用 3 次 · 输入 1,200 / 输出 18 token」；没调 AI 时为空串 */
export function aiUsageText(u: {
  ai_model?: string;
  ai_calls: number;
  prompt_tokens: number;
  completion_tokens: number;
}): string {
  if (!u.ai_calls) return "";
  const parts = [`调用 ${u.ai_calls} 次`];
  if (u.prompt_tokens || u.completion_tokens) {
    parts.push(`输入 ${tokenText(u.prompt_tokens)} / 输出 ${tokenText(u.completion_tokens)} token`);
  } else {
    parts.push("服务商没有报用量");
  }
  return u.ai_model ? `${u.ai_model} · ${parts.join(" · ")}` : parts.join(" · ");
}

/** 用时写成「0.8 秒」「12 秒」「2 分 5 秒」 */
export function attendanceDuration(ms: number): string {
  if (!ms || ms < 0) return "";
  const sec = ms / 1000;
  if (sec < 10) return `${Math.max(0.1, Math.round(sec * 10) / 10)} 秒`;
  if (sec < 60) return `${Math.round(sec)} 秒`;
  const m = Math.floor(sec / 60);
  const rest = Math.round(sec - m * 60);
  return rest ? `${m} 分 ${rest} 秒` : `${m} 分钟`;
}
