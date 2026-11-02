import type { MediaMeta } from "@/api";

import { episodeText } from "./media";

/**
 * 搜索结果的筛选（画板 06 左栏、10 手机筛选面板，关键字搜索的「筛选」浮层也用它）：
 * 同一组里多选是「或」，组与组之间是「且」；一个选项的计数是**其他组的筛选下**它还有几条，
 * 所以勾上一个选项不会让同组其他选项的数变成 0。计数为 0 的选项变灰、不能勾。
 * 只在前端按已经拿到的结果算，不重新搜。
 */

export type FacetKey =
  | "resolution"
  | "source"
  | "platform"
  | "hdr"
  | "group"
  | "flags"
  | "codec"
  | "episodes"
  | "site";

/** 一条可以筛的结果 */
export interface FacetRecord {
  meta?: MediaMeta;
  free: boolean;
  hr: boolean;
  /** 站点给的优惠（FREE、PERCENT_50……） */
  discount?: string;
  site: string;
  /** 「季集」一组的值：按条目搜索时是服务端对过集号的范围（见 entrySearch 的 spanText）；不给时按标题解析出的季集写 */
  episodes?: string;
}

export type FacetSelection = Partial<Record<FacetKey, string[]>>;

export interface FacetOption {
  value: string;
  label: string;
  count: number;
}

export interface FacetGroup {
  key: FacetKey;
  label: string;
  options: FacetOption[];
}

export const FACET_LABELS: Record<FacetKey, string> = {
  resolution: "分辨率",
  source: "来源",
  platform: "平台",
  hdr: "HDR",
  group: "制作组",
  flags: "中字与优惠",
  codec: "编码",
  episodes: "季集",
  site: "站点",
};

/** 桌面左栏直接列出的组；编码、季集、站点收在「更多筛选」里 */
export const MAIN_FACETS: readonly FacetKey[] = [
  "resolution",
  "source",
  "platform",
  "hdr",
  "group",
  "flags",
];
export const MORE_FACETS: readonly FacetKey[] = ["codec", "episodes", "site"];
/** 手机筛选面板的组（画板 10） */
export const MOBILE_FACETS: readonly FacetKey[] = [
  "resolution",
  "source",
  "platform",
  "hdr",
  "group",
  "flags",
  "site",
];

/** 「中字与优惠」的几个选项，顺序固定；50% 只在手机筛选面板里（画板 10） */
const FLAGS = [
  { value: "subs", label: "有中字" },
  { value: "free", label: "免费" },
  { value: "half", label: "50%" },
  { value: "nohr", label: "无 H&R" },
] as const;

/** 分辨率从好到差（同后端 subscribe.Resolutions） */
const RESOLUTION_ORDER = ["4320p", "2160p", "1440p", "1080p", "720p", "576p", "480p"];

/** 解析出的英文写法换成中文界面上习惯的叫法；没有列在这里的原样显示 */
const VALUE_LABELS: Partial<Record<FacetKey, Record<string, string>>> = {
  hdr: { DV: "杜比视界" },
  platform: {
    "Tencent Video": "腾讯视频",
    iQIYI: "爱奇艺",
    Youku: "优酷",
    MGTV: "芒果 TV",
    Bilibili: "哔哩哔哩",
  },
};

export function facetLabel(key: FacetKey, value: string): string {
  if (key === "flags") return FLAGS.find((f) => f.value === value)?.label ?? value;
  return VALUE_LABELS[key]?.[value] ?? value;
}

const isHalf = (discount?: string) => {
  const d = (discount ?? "").toUpperCase();
  return d === "PERCENT_50" || d === "2X50";
};

/** 一条结果在一组里的值（可以有几个，例如 HDR10 与杜比视界） */
export function facetValues(r: FacetRecord, key: FacetKey): string[] {
  const m = r.meta ?? {};
  switch (key) {
    case "resolution":
      return m.resolution ? [m.resolution] : [];
    case "source": {
      const out = m.source ? [m.source] : [];
      if (m.remux) out.push("Remux");
      return out;
    }
    case "platform":
      return m.platform ? [m.platform] : [];
    case "hdr":
      return m.hdr ?? [];
    case "group":
      return m.group ? [m.group] : [];
    case "codec":
      return m.video_codec ? [m.video_codec] : [];
    case "episodes": {
      const ep = r.episodes ?? episodeText(m);
      return ep ? [ep] : [];
    }
    case "site":
      return r.site ? [r.site] : [];
    case "flags": {
      const out: string[] = [];
      if (m.chinese_subs) out.push("subs");
      if (r.free) out.push("free");
      if (isHalf(r.discount)) out.push("half");
      if (!r.hr) out.push("nohr");
      return out;
    }
  }
}

/** 这条结果过不过筛选（except 那一组不算，用来数这一组里各选项的条数） */
export function matchesFacets(r: FacetRecord, sel: FacetSelection, except?: FacetKey): boolean {
  for (const [key, chosen] of Object.entries(sel) as [FacetKey, string[] | undefined][]) {
    if (key === except || !chosen?.length) continue;
    const values = facetValues(r, key);
    if (!chosen.some((v) => values.includes(v))) return false;
  }
  return true;
}

export function applyFacets<T extends FacetRecord>(
  records: readonly T[],
  sel: FacetSelection,
): T[] {
  return records.filter((r) => matchesFacets(r, sel));
}

/**
 * 各组的选项与计数。选项来自全部结果（筛选不会让选项消失，只会让它的数变成 0）；
 * 分辨率按从好到差，「中字与优惠」顺序固定，其余按条数从多到少、同数按名字。
 */
export function buildFacets(
  records: readonly FacetRecord[],
  sel: FacetSelection,
  keys: readonly FacetKey[],
  opts: { half?: boolean } = {},
): FacetGroup[] {
  const groups: FacetGroup[] = [];
  for (const key of keys) {
    const total = new Map<string, number>();
    for (const r of records)
      for (const v of facetValues(r, key)) total.set(v, (total.get(v) ?? 0) + 1);
    let values: string[];
    if (key === "flags") {
      values = FLAGS.filter((f) => f.value !== "half" || opts.half).map((f) => f.value);
    } else {
      values = [...total.keys()];
      if (key === "resolution") {
        const rank = (v: string) => {
          const i = RESOLUTION_ORDER.indexOf(v.toLowerCase());
          return i < 0 ? RESOLUTION_ORDER.length : i;
        };
        values.sort((a, b) => rank(a) - rank(b) || a.localeCompare(b));
      } else {
        values.sort((a, b) => (total.get(b) ?? 0) - (total.get(a) ?? 0) || a.localeCompare(b));
      }
    }
    if (values.length === 0) continue;
    const pool = records.filter((r) => matchesFacets(r, sel, key));
    const options = values.map((value) => ({
      value,
      label: facetLabel(key, value),
      count: pool.filter((r) => facetValues(r, key).includes(value)).length,
    }));
    groups.push({ key, label: FACET_LABELS[key], options });
  }
  return groups;
}

export function toggleFacet(sel: FacetSelection, key: FacetKey, value: string): FacetSelection {
  const cur = sel[key] ?? [];
  const next = cur.includes(value) ? cur.filter((v) => v !== value) : [...cur, value];
  const out = { ...sel };
  if (next.length) out[key] = next;
  else delete out[key];
  return out;
}

/** 已选的选项个数（「筛选 · N」里的 N） */
export function selectedCount(sel: FacetSelection): number {
  return Object.values(sel).reduce((n, v) => n + (v?.length ?? 0), 0);
}
