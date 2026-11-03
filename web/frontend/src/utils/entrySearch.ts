import type {
  MediaMeta,
  MediaSearchEpisode,
  MediaSearchGroup,
  MediaSearchMember,
  MediaSearchResult,
  MediaSearchVerdict,
} from "@/api";

import { applyFacets, type FacetRecord, type FacetSelection, facetLabel } from "./searchFacets";

/** 按条目搜索（画板 06、09）结果的写法与排法：只用接口给的数据，不重新搜。 */

const pad = (n: number) => String(n).padStart(2, "0");

export const epCode = (n: number) => `E${pad(n)}`;

/**
 * 一组覆盖的集（服务端按集号规则对过的 span，第几期上下这种也已换成 TMDB 的集号）：E08、E01–E06、整季。
 * 卡片的副行与「季集」筛选用它，不用种子标题直接解析出来的集号（只写期号的种子解析不出集号）。电影没有 span，回空串
 */
export function spanText(span: MediaSearchGroup["span"]): string {
  if (!span) return "";
  if (span.whole) return "整季";
  const start = span.start ?? 0;
  if (!start) return "";
  const end = span.end ?? start;
  return end > start ? `${epCode(start)}–${epCode(end)}` : epCode(start);
}

/** 集号列表：连着三集以上写成区间（E01–E06、E08） */
export function epList(ns: readonly number[]): string {
  const sorted = [...new Set(ns)].sort((a, b) => a - b);
  const out: string[] = [];
  for (let i = 0; i < sorted.length;) {
    let j = i;
    while (j + 1 < sorted.length && sorted[j + 1] === sorted[j]! + 1) j++;
    if (j - i >= 2) out.push(`${epCode(sorted[i]!)}–${epCode(sorted[j]!)}`);
    else for (let k = i; k <= j; k++) out.push(epCode(sorted[k]!));
    i = j + 1;
  }
  return out.join("、");
}

/** 每集格子下面那一句（画板 06 左栏 ep note）：缺的、等版本的、在下载的 */
export function episodeNote(episodes: readonly MediaSearchEpisode[]): string {
  const of = (state: MediaSearchEpisode["state"]) =>
    episodes.filter((e) => e.state === state).map((e) => e.episode);
  const missing = of("missing");
  const waiting = of("waiting_version");
  const downloading = of("downloading");
  const parts: string[] = [];
  if (missing.length && waiting.length)
    parts.push(`缺 ${epList(missing)}`, `${epList(waiting)} 等版本`);
  else if (missing.length) parts.push(`缺 ${epList(missing)}`);
  else if (waiting.length) parts.push(`缺 ${epList(waiting)}（等版本）`);
  if (downloading.length) parts.push(`${epList(downloading)} 下载中`);
  if (parts.length === 0 && episodes.some((e) => e.state === "library")) return "播出了的都已入库";
  // 全角括号后面不再空一格（画板「缺 E08（等版本）· E07 下载中」）
  return parts.join(" · ").replaceAll("） · ", "）· ");
}

export const EPISODE_STATE_LABELS: Record<MediaSearchEpisode["state"], string> = {
  library: "已入库",
  downloading: "下载中",
  waiting_version: "等版本",
  missing: "缺",
  upcoming: "还没播",
};

export interface Pill {
  text: string;
  tone: "ok" | "warn" | "info" | "neutral";
  dot?: boolean;
  icon?: string;
}

/**
 * 分组卡片右上角的那一枚（画板 06 的 cover）：整组库里都有 → 库里都有了；不符合档案 → 原因；
 * 能补上缺的集 → 集号；这一组的集在下载 → 集号与来源站点。都不是时不显示。
 */
export function coverPill(
  g: MediaSearchGroup,
  episodes: readonly MediaSearchEpisode[],
  movie?: MediaSearchResult["movie"],
): Pill | null {
  if (g.in_library) return { text: "库里都有了", tone: "neutral", dot: true };
  if (g.verdict?.reject)
    return { text: `不符合：${g.verdict.reject}`, tone: "warn", icon: "triangle-alert" };
  if (g.covers.length) return { text: `能补上 ${epList(g.covers)}`, tone: "ok", dot: true };
  if (g.downloading.length) {
    const site = episodes.find((e) => e.episode === g.downloading[0])?.site;
    return {
      text: `${epList(g.downloading)} 正在下载${site ? `（${site}）` : ""}`,
      tone: "info",
      dot: true,
    };
  }
  if (movie?.state === "downloading") {
    return { text: `正在下载${movie.site ? `（${movie.site}）` : ""}`, tone: "info", dot: true };
  }
  return null;
}

/** 订阅锁住的版本与这一组对不上时，写出对不上的那部分（画板 06「不是锁定的版本（HHWEB）」）；对得上或没锁时为空 */
export function lockMismatch(
  lock: { group: string; resolution: string } | null | undefined,
  m: Pick<MediaMeta, "group" | "resolution">,
): string {
  if (!lock) return "";
  const same = (a = "", b = "") => a.trim().toLowerCase() === b.trim().toLowerCase();
  const parts: string[] = [];
  if (lock.group && !same(lock.group, m.group)) parts.push(lock.group);
  if (lock.resolution && !same(lock.resolution, m.resolution)) parts.push(lock.resolution);
  return parts.join(" ");
}

/** 档案分牌（画板 06 score）：通过 ok 底、不符合与不按档案灰底「—」、整组库里都有时整块变淡 */
export function scoreCard(
  v: MediaSearchVerdict | null,
  inLibrary: boolean,
): { value: string; unit: string; tone: "ok" | "none" | "dim" } {
  const pass = v !== null && !v.reject;
  const value = pass ? String(v.score) : "—";
  const unit = v === null ? "不按档案" : pass ? "档案分" : "不符合";
  return { value, unit, tone: inLibrary ? "dim" : pass ? "ok" : "none" };
}

/** 优惠：免费带剩余时间（画板「免费 18 时」），50% 等用 info，没有优惠写「无优惠」 */
export function discountText(m: MediaSearchMember): Pill {
  const level = (m.discount || "").toUpperCase();
  const left = (s?: number) => {
    if (!s || s <= 0) return "";
    if (s >= 72 * 3600) return ` ${Math.floor(s / 86400)} 天`;
    if (s >= 3600) return ` ${Math.floor(s / 3600)} 时`;
    return ` ${Math.max(1, Math.floor(s / 60))} 分`;
  };
  switch (level) {
    case "FREE":
      return { text: `免费${left(m.free_left_seconds)}`, tone: "ok" };
    case "2XFREE":
      return { text: `2x免费${left(m.free_left_seconds)}`, tone: "ok" };
    case "PERCENT_50":
      return { text: "50%", tone: "info" };
    case "PERCENT_30":
      return { text: "30%", tone: "info" };
    case "PERCENT_70":
      return { text: "70%", tone: "info" };
    case "2XUP":
      return { text: "2x上传", tone: "info" };
    case "2X50":
      return { text: "2x50%", tone: "info" };
  }
  if (m.free) return { text: `免费${left(m.free_left_seconds)}`, tone: "ok" };
  return { text: "无优惠", tone: "neutral" };
}

export function hrText(m: MediaSearchMember): { text: string; warn: boolean } {
  if (!m.hr) return { text: "无 H&R", warn: false };
  return { text: m.hr_hours ? `H&R ${m.hr_hours} 时` : "有 H&R", warn: true };
}

export interface ParsedTag {
  label: string;
  tone: "neutral" | "info" | "kpi3" | "ok";
}

/**
 * 解析标签（画板 06 的 tag）：分辨率、来源、Remux、平台、编码、HDR、制作组、中字，按这个顺序；
 * 分辨率 info、HDR kpi-3、中字 ok，其余中性。
 */
export function parsedTags(m: MediaMeta | undefined): ParsedTag[] {
  if (!m) return [];
  const out: ParsedTag[] = [];
  const add = (label: string | undefined, tone: ParsedTag["tone"] = "neutral") => {
    if (label) out.push({ label, tone });
  };
  add(m.resolution, "info");
  add(m.source);
  if (m.remux) add("Remux");
  add(m.platform && facetLabel("platform", m.platform));
  add(m.video_codec);
  for (const h of m.hdr ?? []) add(facetLabel("hdr", h), "kpi3");
  add(m.group);
  if (m.chinese_subs) add("中字", "ok");
  return out;
}

/** 手机结果卡上成员的一行说明：「做种 41 · 免费 18 时 · 无 H&R」 */
export function memberLine(m: MediaSearchMember): string {
  return [`做种 ${m.seeders}`, discountText(m).text, hrText(m).text].join(" · ");
}

/** 体积：三位有效数字（4.71 GB、27.6 GB） */
export function shortSize(bytes: number): string {
  if (!bytes || bytes <= 0) return "—";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const v = bytes / 1024 ** i;
  return `${i === 0 ? v : Number(v.toPrecision(3))} ${units[i]}`;
}

export type EntrySort = "score" | "seeders" | "size" | "uploaded";

export const ENTRY_SORTS: readonly { value: EntrySort; label: string }[] = [
  { value: "score", label: "档案分" },
  { value: "seeders", label: "做种" },
  { value: "size", label: "体积" },
  { value: "uploaded", label: "发布时间" },
];

/** 一张结果卡片：合并时是一组，关掉合并时是一个成员 */
export interface EntryCard {
  key: string;
  group: MediaSearchGroup;
  /** 这张卡上显示的成员（过了筛选的），第一个是首选 */
  members: MediaSearchMember[];
  /** 过了档案（不按档案时都算过） */
  pass: boolean;
  /** 卡上的成员都拉黑了（只在显示拉黑的时候有，排在最后） */
  blocked: boolean;
}

export interface CardOptions {
  merge: boolean;
  sort: EntrySort;
  sel: FacetSelection;
  /** 输入框里再写的字：按标题与副标题筛，不重新搜 */
  text: string;
  /** 点了每集格子里的一集：只看包含这一集的 */
  episode?: number;
  /** 显示拉黑的（默认不显示；显示时全拉黑的卡排在最后） */
  showBlocked?: boolean;
}

type Row = FacetRecord & { m: MediaSearchMember; g: MediaSearchGroup; gi: number; mi: number };

const verdictPass = (v: MediaSearchVerdict | null) => v === null || !v.reject;

function covers(g: MediaSearchGroup, n: number): boolean {
  const sp = g.span;
  if (!sp) return true;
  return Boolean(sp.whole) || (n >= (sp.start ?? 0) && n <= (sp.end ?? sp.start ?? 0));
}

/** 卡片里的成员先过筛选；按档案分排时是服务端排好的顺序（组间、组内都已按档案的比法排过） */
export function buildCards(res: MediaSearchResult, opts: CardOptions): EntryCard[] {
  const words = opts.text.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const rows: Row[] = [];
  res.groups.forEach((g, gi) => {
    if (opts.episode && !covers(g, opts.episode)) return;
    g.members.forEach((m, mi) => {
      if (m.block && !opts.showBlocked) return;
      const text = `${m.title} ${m.subtitle ?? ""}`.toLowerCase();
      if (words.some((w) => !text.includes(w))) return;
      rows.push({
        m,
        g,
        gi,
        mi,
        meta: m.meta,
        free: m.free,
        hr: m.hr,
        discount: m.discount,
        site: m.site,
        episodes: spanText(g.span),
      });
    });
  });
  const kept = applyFacets(rows, opts.sel);
  let cards: (EntryCard & { order: number })[];
  if (opts.merge) {
    const byGroup = new Map<number, Row[]>();
    for (const r of kept) byGroup.set(r.gi, [...(byGroup.get(r.gi) ?? []), r]);
    cards = [...byGroup.entries()].map(([gi, list]) => {
      const g = res.groups[gi]!;
      const members = list.sort((a, b) => a.mi - b.mi).map((r) => r.m);
      return {
        key: g.group_id,
        group: g,
        members,
        pass: verdictPass(members[0]!.verdict),
        blocked: members.every((m) => Boolean(m.block)),
        order: gi * 10000,
      };
    });
  } else {
    cards = kept.map((r) => ({
      key: `${r.g.group_id}|${r.m.site}|${r.m.torrent_id}`,
      group: r.g,
      members: [r.m],
      pass: verdictPass(r.m.verdict),
      blocked: Boolean(r.m.block),
      order: r.gi * 10000 + r.mi,
    }));
  }
  const best = (c: EntryCard) => c.members[0]!;
  const key: Record<EntrySort, (c: EntryCard) => number> = {
    score: () => 0,
    seeders: (c) => best(c).seeders,
    size: (c) => best(c).size_bytes,
    uploaded: (c) => best(c).uploaded_at ?? 0,
  };
  const by = key[opts.sort];
  cards.sort((a, b) => Number(a.blocked) - Number(b.blocked) || by(b) - by(a) || a.order - b.order);
  return cards.map(({ order: _order, ...c }) => c);
}

/** 拉黑的成员（按服务端的顺序） */
export function blockedMembers(res: MediaSearchResult): MediaSearchMember[] {
  return res.groups.flatMap((g) => g.members.filter((m) => m.block));
}

/**
 * 最后一行拉黑的那段（画板 06）：「已拉黑 1 条：标题（不要这个种子：原因）」；多于一条只写条数；没有拉黑的为空
 */
export function blockedLine(res: MediaSearchResult): { count: string; detail: string } | null {
  const list = blockedMembers(res);
  if (!list.length) return null;
  const count = `已拉黑 ${list.length} 条`;
  if (list.length > 1) return { count, detail: "" };
  const m = list[0]!;
  const reason = m.block?.reason ? `不要这个种子：${m.block.reason}` : "不要这个种子";
  return { count, detail: `：${m.title}（${reason}）` };
}

/**
 * 结果概况（画板 06 sum）：「14 条结果 · 合并成 9 组 · 符合档案 5」；关掉合并时不写组数，不按档案时不写符合档案。
 * 手机（画板 09）短一点：「14 条 · 合并成 9 组 · 符合档案 5」，和排序下拉排在一行
 */
export function summaryText(
  res: MediaSearchResult,
  cards: readonly EntryCard[],
  merge: boolean,
  short = false,
): string {
  const results = cards.reduce((n, c) => n + c.members.length, 0);
  const parts = [short ? `${results} 条` : `${results} 条结果`];
  if (merge) parts.push(`合并成 ${cards.length} 组`);
  if (res.profile) parts.push(`符合档案 ${cards.filter((c) => c.pass).length}`);
  return parts.join(" · ");
}
