/*
 * 等免费（媒体中心 M2b）页面共用的文字与小函数：状态、筛选、到期时间能改到哪天。
 */
import type { FreeWatch, FreeWatchStatus } from "@/api";

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

const STATUS: Record<FreeWatchStatus, { label: string; tone: Tone }> = {
  watching: { label: "等免费中", tone: "info" },
  pushed: { label: "已推送", tone: "ok" },
  expired: { label: "到期了", tone: "neutral" },
  canceled: { label: "已取消", tone: "neutral" },
  failed: { label: "不再等", tone: "dang" },
};

export function freeWatchStatus(status: string): { label: string; tone: Tone } {
  return STATUS[status as FreeWatchStatus] ?? { label: status, tone: "neutral" };
}

export const FREE_WATCH_FILTERS: readonly { value: "" | FreeWatchStatus; label: string }[] = [
  { value: "", label: "全部" },
  { value: "watching", label: "等免费中" },
  { value: "pushed", label: "已推送" },
  { value: "expired", label: "到期了" },
  { value: "failed", label: "不再等" },
  { value: "canceled", label: "已取消" },
];

/** 筛选档位上的字：带条数（「等免费中 3」）；全部写总数 */
export function filterLabel(
  f: { value: "" | FreeWatchStatus; label: string },
  counts: Partial<Record<FreeWatchStatus, number>>,
): string {
  const n = f.value
    ? (counts[f.value] ?? 0)
    : Object.values(counts).reduce((a, b) => a + (b ?? 0), 0);
  return n > 0 ? `${f.label} ${n}` : f.label;
}

/** 到期时间最晚能改到哪天（同服务端：不晚于现在加 30 天） */
export const MAX_DAYS = 30;

/** 日期选择器里哪些天不能选：今天以前、30 天以后 */
export function expiryDisabled(d: Date, now = new Date()): boolean {
  const day = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const last = day + MAX_DAYS * 86400_000;
  return d.getTime() < day || d.getTime() > last;
}

/** 改到期时间时选的时刻：要晚于现在、不晚于现在加 30 天，不行时说明原因 */
export function expiryProblem(at: Date | null, now = new Date()): string {
  if (!at || Number.isNaN(at.getTime())) return "选一个到期时间";
  if (at.getTime() <= now.getTime()) return "到期时间要晚于现在";
  if (at.getTime() > now.getTime() + MAX_DAYS * 86400_000) return "最多再等 30 天";
  return "";
}

/** 看过几次优惠 */
export function checkText(w: Pick<FreeWatch, "check_count">): string {
  return w.check_count > 0 ? `看过 ${w.check_count} 次` : "还没看过";
}
