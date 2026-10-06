import type { DailyPoint } from "@/api";

/** YYYY-MM-DD 的后一天（按日历算，与时区无关） */
export function nextDay(date: string): string {
  const t = new Date(`${date}T00:00:00Z`);
  t.setUTCDate(t.getUTCDate() + 1);
  return t.toISOString().slice(0, 10);
}

/**
 * 把稀疏的每日快照铺成 [from, to] 每天一格的上传增量：没有快照的那天记 0（不插值），
 * 一份快照跨了好几天时增量记在它自己那天（与 /api/v2/userinfo/trends 同一口径）；
 * 没有基线的第一份快照（spanDays 为 0）不算增量。日期不合法时返回空数组。
 */
export function dailyUploadSeries(points: DailyPoint[], from: string, to: string): number[] {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(from) || !/^\d{4}-\d{2}-\d{2}$/.test(to) || from > to) return [];
  const byDate = new Map(
    points.filter((p) => p.spanDays > 0).map((p) => [p.date, p.deltaUploaded] as const),
  );
  const out: number[] = [];
  // 最多 400 格（接口的上限），日期再怎么不对也不会死循环
  for (let d = from; d <= to && out.length < 400; d = nextDay(d)) out.push(byDate.get(d) ?? 0);
  return out;
}
