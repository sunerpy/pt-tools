/**
 * 数据导出海报的数据：把用户统计接口的数字整理成五个样式都用得上的一份。
 *
 * 只做取值与格式化，不碰 DOM：样式组件只管排版。数字的写法沿用页面上的 formatBytes / formatNumber /
 * formatRatio，海报上的数与用户统计页对得上。打码在渲染时做（PosterMask、PosterLogo），这里保留原文。
 */
import type { AggregatedStatsResponse, DeltaSummary, UserInfoResponse } from "@/api";
import {
  formatBytes,
  formatDate,
  formatJoinDuration,
  formatNumber,
  formatRatio,
  getAvatarColor,
  getSiteBonusName,
} from "@/utils/format";

/** 带单位的数：「186.42 TB」拆成数与单位，排版时单位可以小一号 */
export interface PosterNumber {
  /** 完整写法，例如「186.42 TB」「328.45万」 */
  text: string;
  /** 数的部分，例如「186.42」 */
  value: string;
  /** 单位，例如「TB」「万」；没有单位时是空串 */
  unit: string;
}

/** 站点图标：站点的 favicon，以及打码时用的 4×4 色块（从图标缩出来的，没有图标时用站点的颜色） */
export interface PosterLogoData {
  /** favicon 的 data URL；取不到时是 null，显示首字母 */
  src: string | null;
  /** 16 个 CSS 颜色，按行排列 */
  cells: string[];
}

export interface PosterSite {
  id: string;
  /** 站点名（打码时只留首字） */
  name: string;
  /** 用户名，没有时是空串 */
  user: string;
  /** 等级名，没有时是空串 */
  level: string;
  up: PosterNumber;
  down: PosterNumber;
  ratio: string;
  bonus: PosterNumber;
  /** 魔力在这个站点叫什么（魔力、茉莉、鲸币……） */
  bonusName: string;
  /** 时魔，没有时是空串 */
  bph: string;
  /** 入站日期与已入站时长，没有时是空串 */
  join: string;
  joinDur: string;
  /** 上传量占这张图里上传最多的站点的比例，0–1，画条形用 */
  upShare: number;
  /** 上传在「上传 + 计费下载」里的比例，0–1 */
  upFrac: number;
  /** 站点的颜色（与站点头像的底色同源） */
  color: string;
  logo: PosterLogoData;
}

export interface PosterIncrement {
  /** 今日、本周、本月 */
  label: string;
  up: string;
  down: string;
  bonus: string;
  /** 增量里上传占「上传 + 下载」的比例，0–1，画进度条用 */
  upFrac: number;
}

export interface PosterModel {
  title: string;
  /** 「2026/10/22」 */
  date: string;
  /** 「2026/10/22 20:30」 */
  generated: string;
  /** 小票上的编号：日期 + 站点数，例如「20261022-0008」 */
  serial: string;
  siteCount: number;
  /** 最早的入站日期与已入站时长；没有任何站点报入站日期时是空串 */
  join: string;
  joinDur: string;
  up: PosterNumber;
  down: PosterNumber;
  /** 平均分享率（各站分享率的平均，与用户统计页同一个数） */
  ratio: string;
  /** 总上传在「总上传 + 计费下载」里的比例，0–1 */
  upFrac: number;
  bonus: PosterNumber;
  /** 时魔（每小时魔力），例如「1.29K」 */
  bph: PosterNumber;
  /** 做种数，带千位分隔 */
  seeding: string;
  seedSize: PosterNumber;
  /** 做种积分合计；没有站点报做种积分时是 null，样式里不放这一项 */
  seedBonus: PosterNumber | null;
  /** 平均每站上传 */
  avgUp: PosterNumber;
  /** 周期增量；关掉开关或还没有可比快照时是 null */
  inc: PosterIncrement | null;
  /** 入图的站点，按上传从多到少；关掉「附上逐站明细」时是空数组 */
  sites: PosterSite[];
  mask: PosterMaskOptions;
}

export interface PosterMaskOptions {
  names: boolean;
  users: boolean;
  logos: boolean;
}

export interface PosterInput {
  /** Defaults to the desktop Chinese format. */
  locale?: string;
  stats: AggregatedStatsResponse;
  /** 入图的站点（顺序不限，这里按上传排） */
  sites: UserInfoResponse[];
  showSites: boolean;
  /** 已经按开关与「有没有可比快照」筛过：不放增量时传 null */
  increments: DeltaSummary | null;
  incrementLabel: string;
  title: string;
  mask: PosterMaskOptions;
  logos: ReadonlyMap<string, PosterLogoData>;
  now: Date;
}

/** 「186.42 TB」→ 数与单位；formatBytes 的数与单位之间是一个空格 */
export function bytesNumber(bytes: number): PosterNumber {
  const text = formatBytes(bytes);
  const at = text.lastIndexOf(" ");
  return at < 0
    ? { text, value: text, unit: "" }
    : { text, value: text.slice(0, at), unit: text.slice(at + 1) };
}

/** 「328.45万」→ 数与单位；formatNumber 的单位是紧跟在数后面的 K、万、亿 */
export function countNumber(n: number, locale = "zh"): PosterNumber {
  if (locale.startsWith("en")) {
    const abs = Math.abs(n);
    const scale =
      abs >= 1e12
        ? ([1e12, "T"] as const)
        : abs >= 1e9
          ? ([1e9, "B"] as const)
          : abs >= 1e6
            ? ([1e6, "M"] as const)
            : abs >= 1e3
              ? ([1e3, "K"] as const)
              : ([1, ""] as const);
    const value = (n / scale[0]).toFixed(scale[0] === 1 ? 0 : 2);
    return { text: value + scale[1], value, unit: scale[1] };
  }
  const text = formatNumber(n);
  const m = /^(-?[\d.]+)(K|万|亿)?$/.exec(text);
  return m ? { text, value: m[1]!, unit: m[2] ?? "" } : { text, value: text, unit: "" };
}

/** 增量的写法：不是负数时带「+」 */
function signed(text: string): string {
  return text.startsWith("-") ? text : `+${text}`;
}

function frac(up: number, down: number): number {
  const total = Math.max(0, up) + Math.max(0, down);
  return total > 0 ? Math.max(0, up) / total : 0;
}

const pad2 = (n: number) => String(n).padStart(2, "0");

/** 与 toLocaleDateString("zh-CN") 一样的写法，但不受浏览器语言影响：「2026/10/22」 */
export function posterDate(d: Date): string {
  return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()}`;
}

export function posterDateTime(d: Date): string {
  return `${posterDate(d)} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

/**
 * 站点头像的底色是「#rrggbb」或「hsl(h, s%, l%)」：取出色相，打码色块按它变深浅。
 * 解析不了时退回一个中性的蓝色调。
 */
export function hueOf(color: string): number {
  const hsl = /hsl\(\s*(-?[\d.]+)/i.exec(color);
  if (hsl) return ((Number(hsl[1]) % 360) + 360) % 360;
  const hex = /^#([0-9a-f]{6})$/i.exec(color.trim());
  if (!hex) return 210;
  const v = Number.parseInt(hex[1]!, 16);
  const r = ((v >> 16) & 255) / 255;
  const g = ((v >> 8) & 255) / 255;
  const b = (v & 255) / 255;
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  if (max === min) return 0;
  const d = max - min;
  let h: number;
  if (max === r) h = ((g - b) / d) % 6;
  else if (max === g) h = (b - r) / d + 2;
  else h = (r - g) / d + 4;
  return Math.round(((h * 60) % 360) + 360) % 360;
}

/**
 * 没有图标时的打码色块：站点颜色的同一色相、16 格深浅不一。按站点编号定下来（同一站点每次一样），
 * 不随机，导出两次的图一样。
 */
export function fallbackCells(siteId: string, color: string): string[] {
  const hue = hueOf(color);
  let seed = 0;
  for (const ch of siteId) seed = (seed * 31 + ch.charCodeAt(0)) >>> 0;
  return Array.from({ length: 16 }, (_, i) => {
    const s = (seed >>> (i % 16)) + i * 7 + (i % 4) * 5;
    const light = 38 + (s % 10) * 4.2;
    return `hsl(${hue} 46% ${light.toFixed(1)}%)`;
  });
}

export function buildPosterModel(input: PosterInput): PosterModel {
  const { stats, now } = input;
  const locale = input.locale ?? "zh";
  const english = locale.startsWith("en");
  const count = (n: number) => countNumber(n, locale);
  const duration = (timestamp: number) => {
    const value = formatJoinDuration(timestamp);
    return english
      ? value.replace(/年/g, "y ").replace(/月/g, "mo ").replace(/天/g, "d").trim()
      : value;
  };
  const joinTs = stats.perSiteStats
    .map((s) => s.joinDate ?? 0)
    .filter((t) => t > 0)
    .reduce((a, b) => (a === 0 ? b : Math.min(a, b)), 0);

  const ordered = input.showSites ? [...input.sites].sort((a, b) => b.uploaded - a.uploaded) : [];
  const maxUp = ordered.reduce((m, s) => Math.max(m, s.uploaded), 0);
  const sites: PosterSite[] = ordered.map((s) => {
    const color = getAvatarColor(s.site);
    const logo = input.logos.get(s.site.toLowerCase()) ?? {
      src: null,
      cells: fallbackCells(s.site, color),
    };
    return {
      id: s.site,
      name: s.site,
      user: s.username ?? "",
      level: s.levelName || s.rank || "",
      up: bytesNumber(s.uploaded),
      down: bytesNumber(s.downloaded),
      ratio: formatRatio(s.ratio),
      bonus: count(s.bonus ?? 0),
      bonusName: english ? "Bonus" : getSiteBonusName(s.site),
      bph: s.bonusPerHour && s.bonusPerHour > 0 ? count(s.bonusPerHour).text : "",
      join: s.joinDate && s.joinDate > 0 ? formatDate(s.joinDate) : "",
      joinDur: s.joinDate && s.joinDate > 0 ? duration(s.joinDate) : "",
      upShare: maxUp > 0 ? Math.max(0, s.uploaded) / maxUp : 0,
      upFrac: frac(s.uploaded, s.downloaded),
      color,
      logo,
    };
  });

  const inc = input.increments;
  const date = posterDate(now);
  return {
    title: input.title,
    date,
    generated: posterDateTime(now),
    serial: `${now.getFullYear()}${pad2(now.getMonth() + 1)}${pad2(now.getDate())}-${String(stats.siteCount).padStart(4, "0")}`,
    siteCount: stats.siteCount,
    join: joinTs > 0 ? formatDate(joinTs) : "",
    joinDur: joinTs > 0 ? duration(joinTs) : "",
    up: bytesNumber(stats.totalUploaded),
    down: bytesNumber(stats.totalDownloaded),
    ratio: formatRatio(stats.averageRatio),
    upFrac: frac(stats.totalUploaded, stats.totalDownloaded),
    bonus: count(stats.totalBonus),
    bph: count(stats.totalBonusPerHour ?? 0),
    seeding: stats.totalSeeding.toLocaleString("en-US"),
    seedSize: bytesNumber(stats.totalSeederSize ?? 0),
    seedBonus:
      stats.totalSeedingBonus && stats.totalSeedingBonus > 0
        ? count(stats.totalSeedingBonus)
        : null,
    avgUp: bytesNumber(stats.siteCount > 0 ? stats.totalUploaded / stats.siteCount : 0),
    inc: inc
      ? {
          label: input.incrementLabel,
          up: signed(formatBytes(inc.totalUploaded)),
          down: signed(formatBytes(inc.totalDownloaded)),
          bonus: signed(count(Math.round(inc.totalBonus)).text),
          upFrac: frac(inc.totalUploaded, inc.totalDownloaded),
        }
      : null,
    sites,
    mask: { ...input.mask },
  };
}

/** 打码的写法：首字 + 遮盖条，条宽随字数（em），一个字也给一点宽度，看得出是打了码 */
export function maskParts(text: string): { first: string; barEm: number } {
  const chars = Array.from(text);
  return { first: chars[0] ?? "", barEm: Math.max(1.4, (chars.length - 1) * 0.56) };
}
