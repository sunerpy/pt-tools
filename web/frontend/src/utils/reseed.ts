import type {
  ReseedAttempt,
  ReseedCurrentRun,
  ReseedRecord,
  ReseedRun,
  ReseedRunStatus,
  ReseedRunStep,
  ReseedRunTrigger,
  ReseedSiteMapItem,
  ReseedSiteSelection,
} from "@/api";
import { transferStateLabel } from "@/utils/transfer";

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

/** 一条辅种记录的结果：没建成任务的是失败；建成的看任务状态（任务记录清除后看记录里留下的结果） */
export function reseedRecordResult(r: Pick<ReseedRecord, "state" | "job_state" | "job_missing">): {
  label: string;
  tone: Tone;
} {
  switch (r.state) {
    case "failed":
      return { label: "失败", tone: "dang" };
    case "gone":
      // 站点上删了这个种子（IYUU 的索引里还有）：不是失败，不再尝试
      return { label: "站点已删除", tone: "neutral" };
    case "done":
      return { label: "已辅种", tone: "ok" };
    case "rolled_back":
      return { label: "已撤回", tone: "warn" };
    case "canceled":
      return { label: "已取消", tone: "neutral" };
  }
  if (r.job_missing) return { label: "任务已删除", tone: "neutral" };
  switch (r.job_state) {
    case "source_removed":
      return { label: "已辅种", tone: "ok" };
    case "rolled_back":
      return { label: "已撤回", tone: "warn" };
    case "failed":
      return { label: "失败", tone: "dang" };
    case "canceled":
      return { label: "已取消", tone: "neutral" };
    case undefined:
      return { label: "已建任务", tone: "info" };
    default:
      return { label: transferStateLabel(r.job_state), tone: "info" };
  }
}

/** 记录的说明：失败原因，或者任务的消息 */
export function reseedRecordMessage(r: Pick<ReseedRecord, "message" | "job_message">): string {
  return r.message || r.job_message || "";
}

/** 记录对应的任务还没结束（任务已经不在了的不算） */
export function reseedRecordActive(
  r: Pick<ReseedRecord, "state" | "job_state" | "job_missing">,
): boolean {
  if (r.state !== "queued" || r.job_missing) return false;
  return !["source_removed", "rolled_back", "failed", "canceled"].includes(r.job_state ?? "");
}

/** IYUU 站点在 pt-tools 里的情况 */
export function reseedSiteStatus(s: ReseedSiteMapItem): { label: string; tone: Tone } {
  if (!s.site_name) return { label: "pt-tools 不支持", tone: "neutral" };
  if (!s.configured) return { label: "没有配置", tone: "warn" };
  if (!s.selected) return { label: "没有选中", tone: "neutral" };
  return { label: "参与辅种", tone: "ok" };
}

/** 按站点开关的结果重算每个 IYUU 站点在不在辅种范围（与服务端 mapSites 同一规则：只有已配置的站点能参与） */
export function applySiteSelection(
  items: ReseedSiteMapItem[],
  sel: ReseedSiteSelection,
): ReseedSiteMapItem[] {
  const pick = new Set(sel.site_names);
  return items.map((it) => ({
    ...it,
    selected: it.configured && (sel.site_mode === "all" || pick.has(it.site_name ?? "")),
  }));
}

const RUN_STATUS: Record<ReseedRunStatus, { label: string; tone: Tone }> = {
  running: { label: "运行中", tone: "info" },
  paused: { label: "已暂停", tone: "warn" },
  ok: { label: "完成", tone: "ok" },
  partial: { label: "部分出错", tone: "warn" },
  stopped: { label: "已停止", tone: "neutral" },
  limited: { label: "提前停下", tone: "warn" },
  failed: { label: "没能开始", tone: "dang" },
  interrupted: { label: "被打断", tone: "neutral" },
};

/** 轮次的状态 */
export function reseedRunStatus(status: string): { label: string; tone: Tone } {
  return RUN_STATUS[status as ReseedRunStatus] ?? { label: status, tone: "neutral" };
}

const RUN_TRIGGER: Record<ReseedRunTrigger, string> = {
  schedule: "定时",
  manual: "立即运行",
  retry: "重试",
  continue: "继续",
};

/** 轮次是怎么开始的 */
export function reseedRunTrigger(trigger: string): string {
  return RUN_TRIGGER[trigger as ReseedRunTrigger] ?? trigger;
}

const STEP_KIND: Record<ReseedRunStep["kind"], string> = {
  list: "下载器",
  query: "查询",
  add: "加入",
  fail: "失败",
  gone: "已删除",
  stop: "停下",
  error: "出错",
  info: "说明",
};

export function reseedStepKind(kind: string): string {
  return STEP_KIND[kind as ReseedRunStep["kind"]] ?? kind;
}

/**
 * 这一轮用了多久（不含暂停的时间）：「8 秒」「2 分 5 秒」；还在跑、或者没跑完（结束时间是下次启动补记的时刻）时为空。
 */
export function reseedRunDuration(
  run: Pick<ReseedRun, "started_at" | "finished_at" | "status" | "paused_seconds">,
): string {
  if (!run.finished_at || run.status === "interrupted") return "";
  const ms = Date.parse(run.finished_at) - Date.parse(run.started_at) - run.paused_seconds * 1000;
  if (!Number.isFinite(ms) || ms < 0) return "";
  const sec = Math.round(ms / 1000);
  if (sec < 60) return `${sec} 秒`;
  const min = Math.floor(sec / 60);
  return sec % 60 ? `${min} 分 ${sec % 60} 秒` : `${min} 分`;
}

/** 正在跑的那一轮的进度：「第 2/5 批 · 加入 1 · 失败 0」 */
export function reseedRunProgress(cur: ReseedCurrentRun): string {
  const parts: string[] = [];
  if (cur.trigger === "retry") parts.push(`重试 ${cur.hashes} 条`);
  else if (cur.batches > 0)
    parts.push(`第 ${Math.min(cur.batches_done + 1, cur.batches)}/${cur.batches} 批`);
  parts.push(`加入 ${cur.created}`);
  if (cur.failed) parts.push(`失败 ${cur.failed}`);
  if (cur.gone) parts.push(`站点已删除 ${cur.gone}`);
  if (cur.skipped) parts.push(`跳过 ${cur.skipped}`);
  return parts.join(" · ");
}

/** 一次尝试的结果：建成任务的看任务现在的状态 */
export function reseedAttemptResult(
  a: Pick<ReseedAttempt, "outcome" | "job_state" | "job_missing">,
): { label: string; tone: Tone } {
  switch (a.outcome) {
    case "running":
      return { label: "尝试中", tone: "info" };
    case "failed":
      return { label: "失败", tone: "dang" };
    case "gone":
      return { label: "站点已删除", tone: "neutral" };
    case "active":
      return { label: "已有任务", tone: "neutral" };
    case "interrupted":
      return { label: "被打断", tone: "neutral" };
  }
  if (a.job_missing) return { label: "任务已删除", tone: "neutral" };
  if (!a.job_state) return { label: "已建任务", tone: "info" };
  switch (a.job_state) {
    case "source_removed":
      return { label: "已辅种", tone: "ok" };
    case "rolled_back":
      return { label: "已撤回", tone: "warn" };
    case "failed":
      return { label: "任务失败", tone: "dang" };
    case "canceled":
      return { label: "已取消", tone: "neutral" };
    default:
      return { label: transferStateLabel(a.job_state), tone: "info" };
  }
}
