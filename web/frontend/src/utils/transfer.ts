import type { TransferJob, TransferPreviewItem, TransferRuleConfig, TransferState } from "@/api";

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

export const TRANSFER_STATE_LABEL: Record<TransferState, string> = {
  pending: "等待导出",
  exported: "等待加入",
  adding: "正在加入",
  checking: "校验中",
  verified: "校验完成",
  source_removed: "已完成",
  rolled_back: "已回滚",
  failed: "失败",
  canceled: "已取消",
};

const TONE: Record<TransferState, Tone> = {
  pending: "neutral",
  exported: "neutral",
  adding: "info",
  checking: "info",
  verified: "info",
  source_removed: "ok",
  rolled_back: "warn",
  failed: "dang",
  canceled: "neutral",
};

export function transferStateLabel(state: TransferState | string): string {
  return TRANSFER_STATE_LABEL[state as TransferState] ?? state;
}

export function transferStateTone(state: TransferState | string): Tone {
  return TONE[state as TransferState] ?? "neutral";
}

/** 还没加入目标的任务可以取消 */
export function transferCancelable(job: Pick<TransferJob, "state">): boolean {
  return job.state === "pending" || job.state === "exported";
}

/** 校验进度的百分比文字；不在校验时为空 */
export function transferProgressText(job: Pick<TransferJob, "state" | "progress">): string {
  if (job.state !== "checking" || !job.progress) return "";
  return `${(job.progress * 100).toFixed(1)}%`;
}

/** 种子文件从哪来 */
export function transferSourceLabel(item: Pick<TransferPreviewItem, "source">): string {
  if (item.source === "export") return "从下载器导出";
  if (item.source === "site") return "从站点重新下载";
  return "";
}

/** 预览的一句话：能转几个、跳过几个 */
export function transferPreviewSummary(items: TransferPreviewItem[]): string {
  const ok = items.filter((i) => i.ok).length;
  const skipped = items.length - ok;
  return skipped ? `能转 ${ok} 个，跳过 ${skipped} 个` : `能转 ${ok} 个`;
}

/** 规则条件的一句话；没有条件时写「全部已下完的种子」 */
export function transferRuleSummary(rule: {
  category: string;
  tag: string;
  site_name: string;
  min_seeding_hours: number;
}): string {
  const parts: string[] = [];
  if (rule.category) parts.push(`分类 ${rule.category}`);
  if (rule.tag) parts.push(`标签含 ${rule.tag}`);
  if (rule.site_name) parts.push(`站点 ${rule.site_name}`);
  if (rule.min_seeding_hours > 0) parts.push(`做种满 ${rule.min_seeding_hours} 小时`);
  return parts.length ? parts.join("、") : "全部已下完的种子";
}

/** 新规则的默认值：关闭、每轮 10 个、每 60 分钟 */
export function defaultTransferRule(): TransferRuleConfig {
  return {
    name: "",
    enabled: false,
    source_downloader_id: 0,
    target_downloader_id: 0,
    category: "",
    tag: "",
    site_name: "",
    min_seeding_hours: 0,
    max_per_run: 10,
    interval_min: 60,
  };
}
