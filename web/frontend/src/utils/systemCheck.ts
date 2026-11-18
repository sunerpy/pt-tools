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
  { key: "nas", label: "NAS", icon: "database" },
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
    return "一次检查 TMDB、站点、下载器、媒体服务器、路径映射、媒体库、磁盘、NAS、时间、备份与密钥";
  const parts = [`${formatShortDateTime(rep.at, now)} 体检`, took(rep)];
  const bad: string[] = [];
  if (rep.fail) bad.push(`${rep.fail} 项失败`);
  if (rep.warn) bad.push(`${rep.warn} 项提醒`);
  parts.push(bad.length ? bad.join("、") : "都正常");
  return parts.join(" · ");
}

/** 用时：不到 0.1 秒时写「用时不到 0.1 秒」，10 秒以内留一位小数（用时 3.2 秒），再长取整（用时 12 秒） */
function took(rep: HealthReport): string {
  if (rep.duration_ms < 100) return "用时不到 0.1 秒";
  const secs = rep.duration_ms / 1000;
  return `用时 ${secs < 10 ? secs.toFixed(1) : String(Math.round(secs))} 秒`;
}

/** 总览卡标题下的一行：「上次 11-04 09:30 体检 · 用时 3.2 秒 · 共 15 项」 */
export function checkMeta(rep: HealthReport, now = new Date()): string {
  return `上次 ${formatShortDateTime(rep.at, now)} 体检 · ${took(rep)} · 共 ${rep.items.length} 项`;
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

/* ------------------------------------------------------------------ 卡片式体检页（首页 NAS 概览 H1） */

/** 环形图里的一段：起点与长度都按圆周总长 100 算（svg 的 pathLength="100"） */
export interface RingSegment {
  status: HealthStatus;
  start: number;
  length: number;
}

type StatusCounts = Pick<HealthReport, "ok" | "warn" | "fail" | "skip">;

const RING_ORDER: readonly HealthStatus[] = ["fail", "warn", "ok", "skip"];
/** 段与段之间的缝（圆周的 1.2%） */
const RING_GAP = 1.2;

/**
 * 环形图的各段：失败、提醒、正常、跳过依次排，按项数占圆周；不止一段时每段的尾巴扣掉一道缝，
 * 很小的一段至少留它一半的长度，不会被缝吃没。没有项时没有段，只画整圈灰底。
 */
export function ringSegments(counts: StatusCounts | null): RingSegment[] {
  if (!counts) return [];
  const total = RING_ORDER.reduce((n, s) => n + (counts[s] || 0), 0);
  if (!total) return [];
  const present = RING_ORDER.filter((s) => (counts[s] || 0) > 0);
  const gap = present.length > 1 ? RING_GAP : 0;
  const out: RingSegment[] = [];
  let start = 0;
  for (const status of present) {
    const len = (counts[status] / total) * 100;
    out.push({ status, start, length: Math.max(len - gap, len / 2) });
    start += len;
  }
  return out;
}

/** 「需要处理」的问题卡：失败在前、提醒在后，同状态按报告里的顺序（就是后端各项检查的顺序） */
export function problemItems(rep: HealthReport | null): HealthItem[] {
  if (!rep) return [];
  const fails = rep.items.filter((it) => it.status === "fail");
  const warns = rep.items.filter((it) => it.status === "warn");
  return [...fails, ...warns];
}

const FIX_ROUTES: Partial<Record<HealthGroup, string>> = {
  tmdb: "/media/recognize",
  time: "/media/recognize",
  sites: "/sites",
  downloaders: "/downloaders",
  servers: "/media/library",
  paths: "/media/library",
  libraries: "/media/library",
  nas: "/system/nas",
  backup: "/global",
};

/**
 * 问题卡上「去处理」去哪：按组给页面，单个站点（key 是 `site:<名字>`）到它的详情页；
 * 一组的汇总项（没有配、读取失败，key 就是组名）到那一组的列表页。磁盘与密钥没有能在页面上改的地方，不给链接。
 */
export function fixRoute(item: Pick<HealthItem, "group" | "key">): string | null {
  if (item.group === "sites" && item.key.startsWith("site:")) {
    return `/sites/${encodeURIComponent(item.key.slice("site:".length))}`;
  }
  return FIX_ROUTES[item.group] ?? null;
}

/**
 * 这次新出现的问题：这次是提醒或失败，而上一次没有这一项、或者上一次比这次好。
 * 没有上一次（第一次体检、页面刚打开）时什么都不标。
 */
export function changedKeys(prev: HealthReport | null, next: HealthReport | null): Set<string> {
  const out = new Set<string>();
  if (!prev || !next) return out;
  const before = new Map(prev.items.map((it) => [it.key, it.status as HealthStatus]));
  for (const it of next.items) {
    const now = it.status as HealthStatus;
    if (now !== "fail" && now !== "warn") continue;
    const was = before.get(it.key);
    if (was === undefined || RANK[now] > RANK[was]) out.add(it.key);
  }
  return out;
}

/** 分组卡要显示的：整组（标题、最坏状态按整组算）与按筛选留下的项 */
export interface ShownGroup {
  group: CheckGroup;
  items: HealthItem[];
}

/**
 * 分组卡按筛选显示：「只看有问题的」只留提醒与失败，点了计数块时只留那个状态的项，两个同时开时都要满足；
 * 留不下项的组不列。组的最坏状态仍按整组算，筛掉的失败照样在卡头上看得到。
 */
export function filterGroups(
  rep: HealthReport | null,
  opts: { onlyProblems: boolean; status: HealthStatus | "" },
): ShownGroup[] {
  const out: ShownGroup[] = [];
  for (const group of checkGroups(rep)) {
    const items = group.items.filter(
      (it) =>
        (!opts.onlyProblems || it.status === "fail" || it.status === "warn") &&
        (!opts.status || it.status === opts.status),
    );
    if (items.length) out.push({ group, items });
  }
  return out;
}
