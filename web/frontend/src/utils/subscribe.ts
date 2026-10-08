/*
 * 订阅（路线图 M11）页面共用的文字与小函数：订阅状态、来源、质量档案的选项、进度、单集与种子状态。
 */
import type {
  QualityPref,
  Subscription,
  SubscriptionProgress,
  SubscriptionStatus,
  SubscriptionTorrent,
} from "@/api";

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

const SUB_STATUS: Record<SubscriptionStatus, { label: string; tone: Tone }> = {
  active: { label: "在找", tone: "info" },
  paused: { label: "暂停", tone: "neutral" },
  pending: { label: "待确认", tone: "warn" },
  done: { label: "完成", tone: "ok" },
};

export function subStatus(status: string): { label: string; tone: Tone } {
  return SUB_STATUS[status as SubscriptionStatus] ?? { label: status, tone: "neutral" };
}

export const SUB_FILTERS: readonly { value: "" | SubscriptionStatus; label: string }[] = [
  { value: "", label: "全部" },
  { value: "active", label: "在找" },
  { value: "pending", label: "待确认" },
  { value: "paused", label: "暂停" },
  { value: "done", label: "完成" },
];

export function sourceLabel(source: string): string {
  return (
    { manual: "手动", explore: "探索", douban: "豆瓣想看", chatops: "机器人" }[source] ?? source
  );
}

/** 订阅的名字：名字加年份，剧集加季。 */
export function subName(s: Pick<Subscription, "title" | "year" | "media_type" | "season">): string {
  let name = s.title;
  if (s.year > 0) name += ` (${s.year})`;
  if (s.media_type === "tv") name += ` 第 ${s.season} 季`;
  return name;
}

/** 进度写成一句话：电影是已入库 / 下载中 / 还没下载；剧集是已入库几集、下载中几集、缺几集。 */
export function progressText(kind: string, p: SubscriptionProgress | undefined): string {
  if (!p) return "";
  if (kind === "movie") {
    if (p.in_library > 0) return "已入库";
    if (p.downloading > 0) return "下载中";
    return "还没下载";
  }
  const parts = [`已入库 ${p.in_library}/${p.total}`];
  if (p.downloading > 0) parts.push(`下载中 ${p.downloading}`);
  if (p.missing?.length) parts.push(`缺 ${p.missing.length}`);
  if (p.aired < p.total) parts.push(`${p.total - p.aired} 集还没播`);
  return parts.join(" · ");
}

const EPISODE_STATE: Record<string, { label: string; tone: Tone }> = {
  library: { label: "已入库", tone: "ok" },
  downloading: { label: "下载中", tone: "info" },
  missing: { label: "缺", tone: "warn" },
  upcoming: { label: "还没播", tone: "neutral" },
};

export function episodeState(state: string): { label: string; tone: Tone } {
  return EPISODE_STATE[state] ?? { label: state, tone: "neutral" };
}

const TORRENT_STATUS: Record<SubscriptionTorrent["status"], { label: string; tone: Tone }> = {
  downloading: { label: "下载中", tone: "info" },
  done: { label: "已入库", tone: "ok" },
  failed: { label: "失败", tone: "dang" },
  replaced: { label: "已替换", tone: "neutral" },
};

export function torrentStatus(status: string): { label: string; tone: Tone } {
  return (
    TORRENT_STATUS[status as SubscriptionTorrent["status"]] ?? { label: status, tone: "neutral" }
  );
}

/** 种子里的集：整季、第 a–b 集、第 n 集；电影为空串。 */
export function torrentSpan(
  t: Pick<SubscriptionTorrent, "complete" | "episode" | "episode_end">,
): string {
  if (t.complete) return "整季";
  if (t.episode > 0 && t.episode_end > t.episode) return `第 ${t.episode}–${t.episode_end} 集`;
  if (t.episode > 0) return `第 ${t.episode} 集`;
  return "";
}

export interface PrefOption {
  value: QualityPref;
  label: string;
}

/** 质量档案里「要不要」的选项；avoid 只有 Remux 与 HDR 有。 */
export function prefOptions(avoid: boolean): PrefOption[] {
  const out: PrefOption[] = [
    { value: "", label: "不限" },
    { value: "prefer", label: "优先" },
    { value: "require", label: "必须有" },
  ];
  if (avoid) out.push({ value: "avoid", label: "不要" });
  return out;
}

export function prefLabel(v: string): string {
  return { "": "不限", prefer: "优先", require: "必须有", avoid: "不要" }[v] ?? v;
}

/** 质量档案的一句话说明：分辨率、来源、编码与几个要求。 */
export function profileSummary(p: {
  resolutions: string[];
  sources: string[];
  codecs: string[];
  remux: string;
  hdr: string;
  chinese_subs: string;
  free: string;
  min_size_gb: number;
  max_size_gb: number;
  exclude_hr: boolean;
}): string {
  const parts: string[] = [];
  parts.push(p.resolutions.length ? p.resolutions.join(" > ") : "分辨率不限");
  if (p.sources.length) parts.push(p.sources.join(" > "));
  if (p.codecs.length) parts.push(p.codecs.join(" > "));
  for (const [name, v] of [
    ["Remux", p.remux],
    ["HDR", p.hdr],
    ["中字", p.chinese_subs],
    ["免费", p.free],
  ] as const) {
    if (v) parts.push(`${name}${prefLabel(v)}`);
  }
  if (p.min_size_gb > 0 || p.max_size_gb > 0) {
    parts.push(`${p.min_size_gb || 0}–${p.max_size_gb || "∞"} GB`);
  }
  if (p.exclude_hr) parts.push("不要 H&R");
  return parts.join(" · ");
}
