import type { BrushRunResult, BrushTask, BrushTaskConfig } from "@/api";

/** 刷流入场可选的优惠类型（与 site/v2 的 DiscountLevel 一致），按常用程度排 */
export const BRUSH_DISCOUNTS: readonly { value: string; label: string }[] = [
  { value: "FREE", label: "免费" },
  { value: "2XFREE", label: "2x 免费" },
  { value: "2X50", label: "2x 50%" },
  { value: "PERCENT_50", label: "50%" },
  { value: "PERCENT_30", label: "30%" },
  { value: "PERCENT_70", label: "70%" },
  { value: "2XUP", label: "2x 上传" },
];

const DISCOUNT_LABEL = new Map(BRUSH_DISCOUNTS.map((d) => [d.value, d.label]));

/** 优惠类型的中文名；不认识的原样返回 */
export function discountLabel(value: string): string {
  return DISCOUNT_LABEL.get(value.toUpperCase()) ?? value;
}

/** 任务允许的优惠类型；空 = 只收免费（与后端一致） */
export function taskDiscounts(discounts: string): string[] {
  const list = discounts
    .split(",")
    .map((d) => d.trim().toUpperCase())
    .filter(Boolean);
  return list.length ? list : ["FREE", "2XFREE"];
}

/** 新建任务的默认值：关闭、10 分钟一轮、同时下载 3 个、只收免费、排除 H&R、免费到期未完成就删 */
export function defaultBrushConfig(): BrushTaskConfig {
  return {
    name: "",
    enabled: false,
    site_name: "",
    downloader_id: 0,
    save_path: "",
    category: "",
    tags: "",
    interval_min: 10,
    discounts: "FREE,2XFREE",
    min_free_remain_min: 60,
    min_size_gb: 0,
    max_size_gb: 0,
    max_seeders: 0,
    min_leechers: 0,
    max_publish_age_min: 0,
    exclude_hr: true,
    include_keywords: "",
    exclude_keywords: "",
    max_downloading: 3,
    max_total_size_gb: 0,
    max_daily_download_gb: 0,
    remove_seed_time_h: 0,
    remove_ratio: 0,
    remove_low_speed_kbs: 0,
    remove_low_speed_window_min: 30,
    remove_inactive_h: 0,
    remove_free_expired_incomplete: true,
    remove_with_data: true,
  };
}

/** 从任务里取出可编辑的配置（编辑对话框用） */
export function configOf(task: BrushTask): BrushTaskConfig {
  const base = defaultBrushConfig();
  const out = { ...base };
  for (const key of Object.keys(base) as (keyof BrushTaskConfig)[]) {
    (out as Record<string, unknown>)[key] = task[key];
  }
  return out;
}

/** 删种规则的一句话说明；一条都没开时提醒「只在免费到期时删」或「不会自动删」 */
export function removalSummary(c: BrushTaskConfig): string {
  const parts: string[] = [];
  if (c.remove_seed_time_h > 0) parts.push(`做种满 ${c.remove_seed_time_h} 小时`);
  if (c.remove_ratio > 0) parts.push(`分享率到 ${c.remove_ratio}`);
  if (c.remove_low_speed_kbs > 0)
    parts.push(`${c.remove_low_speed_window_min} 分钟平均上传低于 ${c.remove_low_speed_kbs} KB/s`);
  if (c.remove_inactive_h > 0) parts.push(`${c.remove_inactive_h} 小时没有活动`);
  if (c.remove_free_expired_incomplete) parts.push("免费到期时尚未下完");
  if (!parts.length) return "不会自动删种";
  return parts.join("、");
}

/** 限额的一句话说明 */
export function limitSummary(c: BrushTaskConfig): string {
  const parts = [`同时下载 ${c.max_downloading} 个`];
  if (c.max_total_size_gb > 0) parts.push(`总体积 ${c.max_total_size_gb} GB`);
  if (c.max_daily_download_gb > 0) parts.push(`每天加入 ${c.max_daily_download_gb} GB`);
  return parts.join(" · ");
}

/** 「立即运行」之后弹的提示：结论在前，原因在后 */
export function runMessage(
  r: BrushRunResult,
  error?: string,
): { tone: "ok" | "warn" | "error"; text: string } {
  if (error) return { tone: "error", text: `运行失败：${error}` };
  const parts = [`列表 ${r.listed} 个，符合条件 ${r.eligible} 个，加入 ${r.added} 个`];
  if (r.removed) parts.push(`删除 ${r.removed} 个`);
  if (r.gone) parts.push(`${r.gone} 个已不在下载器里`);
  if (r.stopped) parts.push(r.stopped);
  const errs = r.errors ?? [];
  if (errs.length) parts.push(`${errs.length} 个错误：${errs[0]}`);
  return { tone: errs.length ? "warn" : "ok", text: parts.join("；") };
}

/** 下次运行时间（毫秒）；从没运行过时为 null（启动后一分钟内会跑） */
export function nextRunAt(task: Pick<BrushTask, "last_run_at" | "interval_min">): number | null {
  if (!task.last_run_at) return null;
  const t = Date.parse(task.last_run_at);
  if (Number.isNaN(t)) return null;
  return t + task.interval_min * 60_000;
}
