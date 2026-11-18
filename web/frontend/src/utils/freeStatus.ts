import { formatShortDateTime } from "@/utils/format";

/** 任务记录里和免费资格、到期暂停有关的字段（见 models.TorrentInfo） */
export interface FreeStatusFields {
  isExpired: boolean;
  isPushed: boolean;
  freeEndTime: string;
  freeValidity?: string;
  freeVerifiedAt?: string | null;
  freeStatusError?: string;
  requireFree?: boolean;
  pauseOnFreeEnd?: boolean;
  pausePlannedAt?: string | null;
  freeEndFailures?: number;
}

export interface FreeEndLabel {
  text: string;
  /** dang：免费已结束；warn：结束时间未知；mute：普通 */
  tone: "dang" | "warn" | "mute";
  /** 悬停说明：完整的结束时间、读取时间、计划暂停时间与读取问题 */
  title: string;
}

function fullTime(s?: string | null): string {
  if (!s || s === "0001-01-01T00:00:00Z") return "";
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString("zh-CN");
}

/**
 * 「免费结束」列的显示：免费到某时显示时间；长期免费、结束时间未知分别写明；
 * 站点没给出结束时间不等于长期免费（要求免费的推送会等下一轮重新读取）。
 */
export function freeEndLabel(t: FreeStatusFields, now = new Date()): FreeEndLabel {
  const notes: string[] = [];
  const end = fullTime(t.freeEndTime);
  if (end) notes.push(`免费结束：${end}`);
  const verified = fullTime(t.freeVerifiedAt);
  if (verified) notes.push(`资格读取于 ${verified}`);
  const planned = fullTime(t.pausePlannedAt);
  if (planned && t.pauseOnFreeEnd) notes.push(`计划暂停：${planned}`);
  if (t.freeStatusError) notes.push(`读取问题：${t.freeStatusError}`);
  const title = notes.join("\n");

  switch (t.freeValidity) {
    case "free_permanent":
      return { text: "长期免费", tone: "mute", title };
    case "unknown":
      return { text: "结束时间未知", tone: "warn", title: title || "站点没有给出免费结束时间" };
  }
  return {
    text: formatShortDateTime(t.freeEndTime, now),
    tone: t.isExpired ? "dang" : "mute",
    title: title || end,
  };
}

/** 「需要关注」里和免费资格、到期暂停有关的原因 */
export function freeStatusReasons(t: FreeStatusFields): string[] {
  const out: string[] = [];
  if (!t.isPushed && t.requireFree && t.freeValidity === "unknown") {
    out.push("免费结束时间未知，要求免费的暂不推送");
  }
  if ((t.freeEndFailures ?? 0) > 0) {
    out.push(`到期暂停连续失败 ${t.freeEndFailures} 次，仍在重试`);
  }
  return out;
}
