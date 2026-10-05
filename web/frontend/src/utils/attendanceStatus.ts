import type { SiteAttendance } from "@/api";
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
