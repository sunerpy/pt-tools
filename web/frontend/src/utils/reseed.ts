import type { ReseedRecord, ReseedSiteMapItem } from "@/api";
import { transferStateLabel } from "@/utils/transfer";

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

/** 一条辅种记录的结果：没建成任务的是失败；建成的看任务状态 */
export function reseedRecordResult(r: Pick<ReseedRecord, "state" | "job_state">): {
  label: string;
  tone: Tone;
} {
  if (r.state === "failed") return { label: "失败", tone: "dang" };
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

/** 记录对应的任务还没结束 */
export function reseedRecordActive(r: Pick<ReseedRecord, "state" | "job_state">): boolean {
  if (r.state !== "queued") return false;
  return !["source_removed", "rolled_back", "failed", "canceled"].includes(r.job_state ?? "");
}

/** IYUU 站点在 pt-tools 里的情况 */
export function reseedSiteStatus(s: ReseedSiteMapItem): { label: string; tone: Tone } {
  if (!s.site_name) return { label: "pt-tools 不支持", tone: "neutral" };
  if (!s.configured) return { label: "没有配置", tone: "warn" };
  if (!s.selected) return { label: "没有选中", tone: "neutral" };
  return { label: "参与辅种", tone: "ok" };
}
