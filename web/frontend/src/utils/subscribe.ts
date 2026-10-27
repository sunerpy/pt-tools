/*
 * 订阅（路线图 M11）页面共用的文字与小函数：订阅状态、来源、质量档案的选项、进度、单集与种子状态。
 */
import type {
  EpisodeRule,
  FollowSchedule,
  QualityPref,
  Subscription,
  SubscriptionProgress,
  SubscriptionRun,
  SubscriptionRunStep,
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
    { manual: "手动", explore: "探索", douban: "豆瓣想看", chatops: "机器人", app: "App" }[
      source
    ] ?? source
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
  if (p.missing?.length) {
    // 在等锁住的版本的也算缺
    parts.push(
      p.waiting ? `缺 ${p.missing.length}（等版本 ${p.waiting}）` : `缺 ${p.missing.length}`,
    );
  }
  if (p.skipped) parts.push(`不追 ${p.skipped}`);
  // aired 只数要追的集（起始集以前的、跳过的不算）
  const required = p.required ?? p.total;
  if (p.aired < required) parts.push(`${required - p.aired} 集还没播`);
  return parts.join(" · ");
}

const EPISODE_STATE: Record<string, { label: string; tone: Tone }> = {
  library: { label: "已入库", tone: "ok" },
  downloading: { label: "下载中", tone: "info" },
  missing: { label: "缺", tone: "warn" },
  waiting_version: { label: "等版本", tone: "primary" },
  upcoming: { label: "还没播", tone: "neutral" },
  skipped: { label: "不追", tone: "neutral" },
};

/** 台账格子的图例顺序。 */
export const EPISODE_STATES = [
  "library",
  "downloading",
  "missing",
  "waiting_version",
  "upcoming",
  "skipped",
] as const;

export function episodeState(state: string): { label: string; tone: Tone } {
  return EPISODE_STATE[state] ?? { label: state, tone: "neutral" };
}

const TORRENT_STATUS: Record<SubscriptionTorrent["status"], { label: string; tone: Tone }> = {
  downloading: { label: "下载中", tone: "info" },
  done: { label: "已入库", tone: "ok" },
  failed: { label: "失败", tone: "dang" },
  replaced: { label: "已替换", tone: "neutral" },
};

const DOUBAN_ITEM: Record<string, { label: string; tone: Tone }> = {
  subscribed: { label: "已订阅", tone: "ok" },
  removed: { label: "订阅已删除", tone: "neutral" },
  unmatched: { label: "没找到条目", tone: "warn" },
};

/** 豆瓣想看里一个条目的结果：建了订阅、订阅后来删掉了（不会再建）、TMDB 上没找到。 */
export function doubanItemState(status: string): { label: string; tone: Tone } {
  return DOUBAN_ITEM[status] ?? { label: status, tone: "neutral" };
}

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

// ---- 追剧 ----

/** 是不是追剧：剧集、按自己的定时搜索（升级以前的订阅定时为空，和以前一样按全局间隔）。 */
export function isFollow(s: Pick<Subscription, "media_type" | "schedule">): boolean {
  return s.media_type === "tv" && Boolean(s.schedule?.kind);
}

const WEEKDAYS = ["日", "一", "二", "三", "四", "五", "六"];

export const SCHEDULE_KINDS: readonly { value: FollowSchedule["kind"]; label: string }[] = [
  { value: "airtime", label: "按播出时间" },
  { value: "interval", label: "每隔一段时间" },
  { value: "daily", label: "每天" },
  { value: "every_days", label: "每隔几天" },
  { value: "weekly", label: "每周" },
  { value: "monthly", label: "每月" },
  { value: "cron", label: "cron 表达式（自定义）" },
  { value: "", label: "按全局间隔" },
];

/** cron 的常用写法：点一下填进去（综艺按播出的那几天、一天几次） */
export const CRON_PRESETS: readonly { label: string; cron: string }[] = [
  { label: "每天 20:00", cron: "0 20 * * *" },
  { label: "每天 8:00 与 22:00", cron: "0 8,22 * * *" },
  { label: "周五、周六 22:00", cron: "0 22 * * 5,6" },
  { label: "周五晚上 + 周六白天", cron: "0 22 * * 5; 0 9,13 * * 6" },
  { label: "工作日每 2 小时", cron: "0 */2 * * 1-5" },
  { label: "每月 1 号 9:00", cron: "0 9 1 * *" },
];

/** 间隔的说法：整天写天、整小时写小时，其余写分钟（和后端 intervalText 一样） */
export function intervalText(minutes: number | undefined): string {
  const m = minutes || 0;
  if (m >= 1440 && m % 1440 === 0) return `${m / 1440} 天`;
  if (m >= 60 && m % 60 === 0) return `${m / 60} 小时`;
  return `${m} 分钟`;
}

export const WEEKDAY_OPTIONS = WEEKDAYS.map((d, i) => ({ value: i, label: `周${d}` }));

/** 定时写成一句话：按播出时间（20:00 播出，2 小时后开始搜）、每天 20:00、周六 21:00…… */
export function scheduleText(sc: FollowSchedule | undefined): string {
  const at = sc?.at || "20:00";
  switch (sc?.kind) {
    case "airtime":
      return `按播出时间（${at} 播出，${sc.air_delay_hours || 2} 小时后开始搜）`;
    case "daily":
      return `每天 ${at}`;
    case "every_days":
      return `每 ${sc.every || 1} 天 ${at}`;
    case "weekly":
      return `${(sc.days ?? []).map((d) => `周${WEEKDAYS[d] ?? d}`).join("、")} ${at}`;
    case "monthly":
      return `每月 ${(sc.days ?? []).map((d) => `${d} 号`).join("、")} ${at}`;
    case "interval":
      return `每隔 ${intervalText(sc.interval_minutes)}`;
    case "cron":
      return `cron「${sc.cron ?? ""}」`;
  }
  return "按全局间隔";
}

const PREVIEW_WEEKDAYS = ["日", "一", "二", "三", "四", "五", "六"];

/** 预览里的一个时间：10-24 周五 20:00（不是今年的带上年份） */
export function previewTime(iso: string, now = new Date()): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n: number) => String(n).padStart(2, "0");
  const md = `${p(d.getMonth() + 1)}-${p(d.getDate())} 周${PREVIEW_WEEKDAYS[d.getDay()]} ${p(d.getHours())}:${p(d.getMinutes())}`;
  return d.getFullYear() === now.getFullYear() ? md : `${d.getFullYear()}-${md}`;
}

/** 版本锁写成一句话：没开时为空串。 */
/** 订阅的标题包含词、排除词（和服务端一样按行或逗号拆开）；都没填时是空数组。 */
export function keywordLines(
  s: Pick<Subscription, "include_keywords" | "exclude_keywords">,
): string[] {
  const split = (v?: string) =>
    (v ?? "")
      .split(/[\n,，]/)
      .map((x) => x.trim())
      .filter(Boolean);
  const out: string[] = [];
  const inc = split(s.include_keywords);
  if (inc.length) out.push(`标题包含 ${inc.join(" 或 ")}`);
  const exc = split(s.exclude_keywords);
  if (exc.length) out.push(`标题排除 ${exc.join("、")}`);
  return out;
}

export function lockText(
  s: Pick<Subscription, "lock_version" | "lock_group" | "lock_resolution">,
): string {
  if (!s.lock_version) return "";
  const v = [s.lock_group, s.lock_resolution].filter(Boolean).join(" ");
  return v ? `版本锁 ${v}` : "版本锁（按第一个下载的版本定）";
}

const RUN_TRIGGER: Record<SubscriptionRun["trigger"], string> = {
  create: "建立",
  schedule: "定时搜索",
  rss: "RSS",
  manual: "手动",
  ai: "AI",
};

export function runTrigger(trigger: string): string {
  return RUN_TRIGGER[trigger as SubscriptionRun["trigger"]] ?? trigger;
}

const RUN_STATUS: Record<SubscriptionRun["status"], { label: string; tone: Tone }> = {
  ok: { label: "成功", tone: "ok" },
  nothing: { label: "没有新的", tone: "neutral" },
  waiting: { label: "在等", tone: "primary" },
  failed: { label: "失败", tone: "dang" },
  partial: { label: "部分成功", tone: "warn" },
  changed: { label: "改动", tone: "neutral" },
};

export function runStatus(status: string): { label: string; tone: Tone } {
  return RUN_STATUS[status as SubscriptionRun["status"]] ?? { label: status, tone: "neutral" };
}

const STEP_KIND: Record<SubscriptionRunStep["kind"], string> = {
  search: "搜索",
  reject: "不要",
  lock: "版本锁",
  push: "推送",
  fail: "失败",
  adopt: "认领",
  wait: "等",
  info: "说明",
};

export function stepKind(kind: string): string {
  return STEP_KIND[kind as SubscriptionRunStep["kind"]] ?? kind;
}

export const RULE_MODES: readonly { value: EpisodeRule["mode"]; label: string; tip: string }[] = [
  {
    value: "tmdb",
    label: "按 TMDB 的集号",
    tip: "标题里的集号就是 TMDB 的集号，不按期号与上下换算",
  },
  {
    value: "period_part",
    label: "按第几期与上下",
    tip: "综艺把一期分上下：第 1 期下、E01 Part 2 是 TMDB 的第 2 集",
  },
  {
    value: "date",
    label: "按播出日期",
    tip: "标题里写的是播出日期（20261002），按 TMDB 的播出日期对",
  },
  { value: "offset", label: "集号偏移", tip: "TMDB 的集号 = 标题里的集号 + 偏移" },
];

/** 一条集号规则写成一句话。 */
export function ruleText(r: Pick<EpisodeRule, "mode" | "offset">): string {
  if (r.mode === "offset") return `集号偏移 ${r.offset > 0 ? "+" : ""}${r.offset}`;
  return RULE_MODES.find((m) => m.value === r.mode)?.label ?? r.mode;
}

export function ruleSource(source: string): string {
  return { manual: "手动", ai: "AI", auto: "自动" }[source] ?? source;
}
