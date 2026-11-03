/*
 * 订阅日历（媒体中心 M3）的纯函数：月、周的日期范围（周一开始），按天分组，一项的写法。日期都按本地时区的 YYYY-MM-DD。
 */
import type { CalendarEntry } from "@/api";

export type CalendarView = "month" | "week";

const WEEKDAYS = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];

/** 列头的星期（周一开始） */
export const WEEK_HEAD = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];

export function dayKey(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** YYYY-MM-DD 读成本地时间的那天 0 点；写法不对时为 null */
export function parseDay(s: unknown): Date | null {
  if (typeof s !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(s)) return null;
  const [y, m, d] = s.split("-").map(Number);
  const out = new Date(y, m - 1, d);
  return dayKey(out) === s ? out : null;
}

function addDays(d: Date, n: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}

/** 这一天所在的周的周一 */
export function startOfWeek(d: Date): Date {
  const offset = (d.getDay() + 6) % 7;
  return addDays(d, -offset);
}

/** 一段日子：月视图是这个月所在的整周（周一到周日，35 或 42 天），周视图是这一周 */
export function rangeDays(view: CalendarView, anchor: Date): string[] {
  let start: Date;
  let count: number;
  if (view === "week") {
    start = startOfWeek(anchor);
    count = 7;
  } else {
    const first = new Date(anchor.getFullYear(), anchor.getMonth(), 1);
    const last = new Date(anchor.getFullYear(), anchor.getMonth() + 1, 0);
    start = startOfWeek(first);
    const end = addDays(startOfWeek(last), 6);
    count = Math.round((end.getTime() - start.getTime()) / 86400000) + 1;
  }
  return Array.from({ length: count }, (_, i) => dayKey(addDays(start, i)));
}

/** 往前、往后翻一个月或一周 */
export function shiftAnchor(view: CalendarView, anchor: Date, n: number): Date {
  if (view === "week") return addDays(anchor, 7 * n);
  // 翻月时停在 1 号，免得 31 号翻到没有 31 号的月份跳过一个月
  return new Date(anchor.getFullYear(), anchor.getMonth() + n, 1);
}

/** 工具条上的范围：「2026 年 11 月」「11-02 – 11-08」（跨年时带年份） */
export function rangeLabel(view: CalendarView, anchor: Date): string {
  if (view === "month") return `${anchor.getFullYear()} 年 ${anchor.getMonth() + 1} 月`;
  const days = rangeDays("week", anchor);
  const a = days[0];
  const b = days[6];
  if (a.slice(0, 4) !== b.slice(0, 4)) return `${a} – ${b}`;
  return `${a.slice(5)} – ${b.slice(5)}`;
}

export function weekdayOf(day: string): string {
  const d = parseDay(day);
  return d ? WEEKDAYS[d.getDay()] : "";
}

/** 按天分组（同一天里按接口给的顺序：播出时刻、名字、集） */
export function groupByDay(entries: readonly CalendarEntry[]): Record<string, CalendarEntry[]> {
  const out: Record<string, CalendarEntry[]> = {};
  for (const e of entries) (out[e.air_date] ??= []).push(e);
  return out;
}

/** 一项的集号：剧集「S02E03」，电影「上映」 */
export function entryCode(e: CalendarEntry): string {
  if (e.media_type !== "tv") return "上映";
  const p = (n?: number) => String(n ?? 0).padStart(2, "0");
  return `S${p(e.season)}E${p(e.episode)}`;
}

/** 播出时刻（按播出时间追剧的才有）：「21:30」 */
export function entryTime(e: CalendarEntry): string {
  if (!e.air_at) return "";
  const d = new Date(e.air_at);
  if (Number.isNaN(d.getTime())) return "";
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/** 一项的说明行：剧集「第 2 季 E03 · 集名」，电影「电影上映」 */
export function entryDetail(e: CalendarEntry): string {
  if (e.media_type !== "tv") return "电影上映";
  const code = `第 ${e.season ?? 0} 季 E${String(e.episode ?? 0).padStart(2, "0")}`;
  return e.name ? `${code} · ${e.name}` : code;
}
