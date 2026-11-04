/*
 * 系统体检页（媒体中心 M8）用到的纯函数：分组的名字与图标、状态的图标与颜色、页头的一句话、按组排好的各项。
 */
import type { HealthGroup, HealthItem, HealthReport, HealthStatus } from "@/api";
import { formatShortDateTime } from "@/utils/format";

type Tone = "ok" | "warn" | "dang" | "neutral";

/** 分组按体检页上的顺序 */
export const HEALTH_GROUPS: readonly { key: HealthGroup; label: string; icon: string }[] = [
  { key: "tmdb", label: "TMDB", icon: "scan" },
  { key: "sites", label: "站点", icon: "globe" },
  { key: "downloaders", label: "下载器", icon: "server" },
  { key: "servers", label: "媒体服务器", icon: "monitor" },
  { key: "paths", label: "路径映射", icon: "arrow-right-left" },
  { key: "libraries", label: "媒体库", icon: "folder-open" },
  { key: "disks", label: "磁盘空间", icon: "hard-drive" },
  { key: "time", label: "时间", icon: "clock" },
  { key: "backup", label: "备份", icon: "archive" },
  { key: "secret", label: "密钥", icon: "key-round" },
];

const STATUS: Record<HealthStatus, { label: string; icon: string; tone: Tone }> = {
  ok: { label: "正常", icon: "circle-check", tone: "ok" },
  warn: { label: "提醒", icon: "triangle-alert", tone: "warn" },
  fail: { label: "失败", icon: "circle-x", tone: "dang" },
  skip: { label: "跳过", icon: "minus", tone: "neutral" },
};

export function healthStatus(status: string): { label: string; icon: string; tone: Tone } {
  return STATUS[status as HealthStatus] ?? STATUS.skip;
}

/** 页头的一句话：「11-04 09:30 体检 · 用时 3.2 秒 · 2 项失败、1 项提醒」；没体检过时说一次查什么 */
export function checkHeadline(rep: HealthReport | null, now = new Date()): string {
  if (!rep)
    return "一次检查 TMDB、站点、下载器、媒体服务器、路径映射、媒体库、磁盘、时间、备份与密钥";
  const secs = rep.duration_ms / 1000;
  const took = secs < 10 ? secs.toFixed(1) : String(Math.round(secs));
  const parts = [`${formatShortDateTime(rep.at, now)} 体检`, `用时 ${took} 秒`];
  const bad: string[] = [];
  if (rep.fail) bad.push(`${rep.fail} 项失败`);
  if (rep.warn) bad.push(`${rep.warn} 项提醒`);
  parts.push(bad.length ? bad.join("、") : "都正常");
  return parts.join(" · ");
}

export interface CheckGroup {
  key: HealthGroup;
  label: string;
  icon: string;
  items: HealthItem[];
  /** 这一组最坏的状态（失败 > 提醒 > 正常 > 跳过） */
  worst: HealthStatus;
}

const RANK: Record<HealthStatus, number> = { fail: 3, warn: 2, ok: 1, skip: 0 };

/** 按组排好的各项；onlyProblems 时只留提醒与失败的项（没有这种项的组不列） */
export function checkGroups(rep: HealthReport | null, onlyProblems = false): CheckGroup[] {
  if (!rep) return [];
  const out: CheckGroup[] = [];
  for (const g of HEALTH_GROUPS) {
    const items = rep.items.filter(
      (it) => it.group === g.key && (!onlyProblems || it.status === "fail" || it.status === "warn"),
    );
    if (!items.length) continue;
    const worst = items.reduce<HealthStatus>(
      (w, it) => (RANK[it.status as HealthStatus] > RANK[w] ? (it.status as HealthStatus) : w),
      "skip",
    );
    out.push({ ...g, items, worst });
  }
  return out;
}
