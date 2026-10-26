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

/** M-Team 的分区：综合区和成人区是两张列表（与 scheduler.BrushSections 一致） */
export const BRUSH_SECTIONS: readonly { value: string; label: string }[] = [
  { value: "normal", label: "综合区" },
  { value: "adult", label: "成人区" },
  { value: "all", label: "两个都读" },
];

/** 只有 M-Team 分区读列表；别的站点不显示「分区」，保存为空 */
export function hasBrushSection(siteName: string): boolean {
  return siteName.toLowerCase() === "mteam";
}

/** 保存时的分区：M-Team 空值按综合区，别的站点一律为空 */
export function brushSectionFor(siteName: string, section: string): string {
  return hasBrushSection(siteName) ? section || "normal" : "";
}

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
    section: "",
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
  if (c.max_total_size_gb > 0) parts.push(`任务总体积 ${c.max_total_size_gb} GB`);
  if (c.max_daily_download_gb > 0) parts.push(`每天加入 ${c.max_daily_download_gb} GB`);
  return parts.join(" · ");
}

/** 有 H&R、做种要求未知时的拒收原因（与后端 brushRejectHRUnknown 一致） */
export const HR_UNKNOWN_REASON = "H&R 要求的做种时长未知";

/** 「立即运行」之后弹的提示：结论在前，原因在后 */
export function runMessage(
  r: BrushRunResult,
  error?: string,
): { tone: "ok" | "warn" | "error"; text: string } {
  if (error) return { tone: "error", text: `运行失败：${error}` };
  const parts = [`列表 ${r.listed} 个，符合条件 ${r.eligible} 个，加入 ${r.added} 个`];
  const rejected = r.rejected ?? [];
  if (!r.eligible && rejected.length) {
    const top = rejected.slice(0, 2);
    let why = `不符合的原因：${top.map((x) => `${x.reason} ${x.count} 个`).join("、")}`;
    if (top.some((x) => x.reason === HR_UNKNOWN_REASON))
      why += "（可以在站点设置里填写 H&R 做种要求）";
    parts.push(why);
  }
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
