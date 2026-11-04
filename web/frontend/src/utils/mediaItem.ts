/*
 * 条目详情页（媒体中心 M3，画板 04、08）用到的纯函数：面包屑从哪里来、资料行、「你的 NAS 上」四行、集卡与种子行的文字。
 */
import type {
  ItemDetail,
  ItemEpisodeState,
  ItemInfo,
  ItemServer,
  ItemSubscription,
  ItemTorrent,
  MediaKind,
  SeasonDetail,
  SeasonEpisode,
  SubscriptionStatus,
} from "@/api";
import { shortSize } from "@/utils/entrySearch";
import { formatShortDateTime } from "@/utils/format";
import { CLASSIFY_COUNTRIES, mediaKindLabel } from "@/utils/media";
import type { RouteLocationRaw } from "vue-router";

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

/** 详情页从哪里进来的：面包屑中间那段与导航的高亮 */
export const ITEM_FROM = {
  explore: { label: "探索", path: "/media/explore" },
  subscriptions: { label: "追剧与订阅", path: "/media/subscriptions" },
  history: { label: "整理历史", path: "/media/history" },
  search: { label: "种子搜索", path: "/search" },
  calendar: { label: "订阅日历", path: "/media/calendar" },
} as const;

export type ItemFrom = keyof typeof ITEM_FROM;

/** 路由里的 from（没有或不认识时是探索） */
export function itemFrom(q: unknown): ItemFrom {
  return typeof q === "string" && q in ITEM_FROM ? (q as ItemFrom) : "explore";
}

/** 详情页的地址；season 是剧集默认打开的季 */
export function itemLink(
  kind: MediaKind,
  id: number,
  from: ItemFrom,
  season?: number,
): RouteLocationRaw {
  const query: Record<string, string> = { from };
  if (kind === "tv" && season !== undefined && season > 0) query.season = String(season);
  return { path: `/media/item/${kind}/${id}`, query };
}

/** 按条目搜索的地址（剧集带上季） */
export function searchLink(kind: MediaKind, id: number, season?: number): RouteLocationRaw {
  const query: Record<string, string> = { media_type: kind, tmdb_id: String(id) };
  if (kind === "tv" && season) query.season = String(season);
  return { path: "/search", query };
}

const TV_STATUS: Record<string, string> = {
  "Returning Series": "连载中",
  Ended: "已完结",
  Canceled: "已取消",
  "In Production": "制作中",
  Planned: "计划中",
  Pilot: "试播",
};

const MOVIE_STATUS: Record<string, string> = {
  Released: "已上映",
  "Post Production": "后期制作",
  "In Production": "拍摄中",
  Planned: "计划中",
  Rumored: "计划中",
  Canceled: "已取消",
};

/** TMDB 的状态写成中文；不认识的照原文 */
export function statusLabel(kind: MediaKind, status?: string): string {
  if (!status) return "";
  return (kind === "tv" ? TV_STATUS : MOVIE_STATUS)[status] ?? status;
}

export function countryLabel(code: string): string {
  return CLASSIFY_COUNTRIES.find((c) => c.value === code)?.label ?? code;
}

/** 片长：电影写整部（2 小时 46 分），剧集写每集（每集 55 分钟） */
export function runtimeText(kind: MediaKind, minutes?: number): string {
  if (!minutes || minutes <= 0) return "";
  if (kind === "tv") return `每集 ${minutes} 分钟`;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (!h) return `${m} 分钟`;
  return m ? `${h} 小时 ${m} 分` : `${h} 小时`;
}

/** 名字下面那一行：剧集「原名 · 年份 · 2 季 24 集」，电影「原名 · 年份 · 片长」；原名和名字一样时不写 */
export function originalLine(info: ItemInfo): string {
  const parts: string[] = [];
  if (info.original_title && info.original_title !== info.title) parts.push(info.original_title);
  if (info.year) parts.push(String(info.year));
  if (info.media_type === "tv") {
    if (info.seasons_count) {
      parts.push(
        info.episodes_count
          ? `${info.seasons_count} 季 ${info.episodes_count} 集`
          : `${info.seasons_count} 季`,
      );
    }
  } else {
    const rt = runtimeText("movie", info.runtime);
    if (rt) parts.push(rt);
  }
  return parts.join(" · ");
}

/** 资料胶囊：类型、题材（最多 3 个）、国家（最多 2 个）、剧集的每集片长、状态 */
export function infoPills(info: ItemInfo): string[] {
  const pills = [mediaKindLabel(info.media_type)];
  pills.push(...(info.genres ?? []).slice(0, 3));
  pills.push(...(info.countries ?? []).slice(0, 2).map(countryLabel));
  if (info.media_type === "tv") {
    const rt = runtimeText("tv", info.runtime);
    if (rt) pills.push(rt);
  }
  const st = statusLabel(info.media_type, info.status);
  if (st) pills.push(st);
  return pills;
}

/** 主创：「导演 陈默 · 主演 周屿、林晚晴、沈川、许曼」（导演最多 2 位，主演最多 4 位） */
export function creditsLine(info: ItemInfo): string {
  const parts: string[] = [];
  if (info.directors?.length) parts.push(`导演 ${info.directors.slice(0, 2).join("、")}`);
  const cast = (info.cast ?? []).slice(0, 4).map((c) => c.name);
  if (cast.length) parts.push(`主演 ${cast.join("、")}`);
  return parts.join(" · ");
}

/** 评分人数：「TMDB 1,204 人评分」 */
export function votesText(votes?: number): string {
  if (!votes) return "TMDB 评分";
  return `TMDB ${votes.toLocaleString("en-US")} 人评分`;
}

/** 还差多久（不到一天写小时，其余按天四舍五入） */
export function leftText(seconds: number): string {
  if (seconds <= 0) return "";
  if (seconds < 24 * 3600) return `${Math.max(1, Math.ceil(seconds / 3600))} 小时`;
  return `${Math.round(seconds / 86400)} 天`;
}

/** 还要多久下完：「12 分钟」「1 小时 5 分钟」「2 天」 */
export function etaText(seconds?: number): string {
  if (!seconds || seconds <= 0) return "";
  if (seconds < 60) return "不到 1 分钟";
  if (seconds < 3600) return `${Math.round(seconds / 60)} 分钟`;
  if (seconds < 86400) {
    const h = Math.floor(seconds / 3600);
    const m = Math.round((seconds % 3600) / 60);
    return m ? `${h} 小时 ${m} 分钟` : `${h} 小时`;
  }
  return `${Math.round(seconds / 86400)} 天`;
}

/** 做种时长：「18 天」「5 小时」 */
export function seedingText(seconds: number): string {
  if (seconds <= 0) return "";
  if (seconds < 86400) return `${Math.max(1, Math.floor(seconds / 3600))} 小时`;
  return `${Math.floor(seconds / 86400)} 天`;
}

const SUB_STATUS: Record<SubscriptionStatus, string> = {
  active: "",
  paused: "已暂停",
  pending: "待确认",
  done: "已完成",
};

/** 订阅的说法：「追剧中 · 第 2 季」「已暂停 · 第 2 季」「订阅中」 */
export function subscriptionLabel(kind: MediaKind, s: ItemSubscription): string {
  const head =
    SUB_STATUS[s.status] || (kind === "tv" ? (s.follow ? "追剧中" : "订阅中") : "订阅中");
  return kind === "tv" ? `${head} · 第 ${s.season} 季` : head;
}

/** 版本锁的说法：「版本锁 HHWEB 2160p」；没开时为空 */
export function lockText(lock?: { group: string; resolution: string }): string {
  if (!lock) return "";
  const v = [lock.group, lock.resolution].filter(Boolean).join(" ");
  return v ? `版本锁 ${v}` : "";
}

/** 页面上看的订阅：选中的这一季的订阅；没有时用最新的一季（电影只有一个） */
export function pickSubscription(
  subs: readonly ItemSubscription[],
  season: number,
): ItemSubscription | null {
  return subs.find((s) => s.season === season) ?? subs[0] ?? null;
}

export interface NasLine {
  key: "library" | "seeding" | "subscription" | "server";
  icon: string;
  title: string;
  desc: string;
  /** 媒体服务器那一行：有 pt-tools 整理过、服务器里没了的（说明用 warn 色，后面是「忽略」）；issues 是要忽略的问题 */
  warn?: boolean;
  issues?: number[];
}

/** 「你的 NAS 上」第 4 行看的那台服务器：有没处理的问题的、服务器里有这一部的、第一台 */
export function nasServer(d: ItemDetail): ItemServer | null {
  const list = d.servers ?? [];
  return list.find((s) => s.removed.length) ?? list.find((s) => s.have) ?? list[0] ?? null;
}

/** 「在 Jellyfin 播放」：第一台有这一部、给了打开链接的服务器 */
export function playTarget(d: ItemDetail): { name: string; url: string } | null {
  const s = (d.servers ?? []).find((x) => x.have && x.open_url);
  return s ? { name: s.name, url: s.open_url! } : null;
}

const code = (season?: number, episode?: number) =>
  `S${String(season ?? 0).padStart(2, "0")}E${String(episode ?? 0).padStart(2, "0")}`;

/** 媒体服务器那一行：「Jellyfin 已入库 17 集」；服务器里删了的写「S02E03 在服务器里被删了」，没有时写同步时间 */
export function serverLine(kind: MediaKind, s: ItemServer, now = new Date()): NasLine {
  let title = `${s.name} 里没有`;
  if (s.have) title = kind === "tv" ? `${s.name} 已入库 ${s.episodes ?? 0} 集` : `${s.name} 已入库`;
  const removed = s.removed;
  if (removed.length) {
    let desc = "在服务器里被删了";
    if (kind === "tv") {
      const first = code(removed[0].season, removed[0].episode);
      desc =
        removed.length > 1
          ? `${first} 等 ${removed.length} 集在服务器里被删了`
          : `${first} 在服务器里被删了`;
    }
    return {
      key: "server",
      icon: "monitor",
      title,
      desc,
      warn: true,
      issues: removed.map((r) => r.id),
    };
  }
  const desc = s.synced_at ? `${formatShortDateTime(s.synced_at, now)} 同步` : "";
  return { key: "server", icon: "monitor", title, desc };
}

/** 「你的 NAS 上」的四行：库里、做种、订阅、媒体服务器（同步过索引的服务器才有第 4 行） */
export function nasLines(d: ItemDetail, sub: ItemSubscription | null): NasLine[] {
  const kind = d.item.media_type;
  const lib = d.nas.library;
  let libTitle = "库里没有";
  if (kind === "tv" && lib.seasons.length) {
    libTitle = `库里 ${lib.seasons
      .map((s) => `S${String(s.n).padStart(2, "0")} ${s.have}/${s.total || "?"}`)
      .join(" · ")}`;
  } else if (kind === "movie" && lib.files > 0) {
    libTitle = lib.version ? `库里有 · ${lib.version}` : "库里有";
  }
  const libDesc =
    lib.files > 0
      ? [lib.libraries.join("、"), `${lib.files} 个文件`, lib.size > 0 ? shortSize(lib.size) : ""]
          .filter(Boolean)
          .join(" · ")
      : "";
  const seed = d.nas.seeding;
  let seedDesc = "";
  if (seed.hr_pending > 0 && seed.hr_left_seconds) {
    seedDesc = `${seed.hr_pending} 个 H&R 还差 ${leftText(seed.hr_left_seconds)}`;
  } else if (seed.lowest_ratio !== undefined && seed.count > 0) {
    seedDesc = `分享率最低 ${seed.lowest_ratio.toFixed(2)}`;
  }
  const lines: NasLine[] = [
    { key: "library", icon: "folder-open", title: libTitle, desc: libDesc },
    {
      key: "seeding",
      icon: "upload",
      title: seed.count > 0 ? `做种 ${seed.count} 个种子` : "没有做种",
      desc: seedDesc,
    },
    {
      key: "subscription",
      icon: "bookmark",
      title: sub ? subscriptionLabel(kind, sub) : kind === "tv" ? "没有追剧" : "没有订阅",
      desc: sub ? lockText(sub.lock) || "没开版本锁" : "",
    },
  ];
  const srv = nasServer(d);
  if (srv) lines.push(serverLine(kind, srv));
  return lines;
}

const EP_STATE: Record<ItemEpisodeState, { label: string; tone: Tone }> = {
  library: { label: "已入库", tone: "ok" },
  downloading: { label: "下载中", tone: "info" },
  waiting_version: { label: "等版本", tone: "primary" },
  missing: { label: "缺", tone: "warn" },
  upcoming: { label: "还没播", tone: "neutral" },
  skipped: { label: "不追", tone: "neutral" },
};

export function episodeStatePill(state: string): { label: string; tone: Tone } {
  return EP_STATE[state as ItemEpisodeState] ?? { label: state, tone: "neutral" };
}

/** 图例：已入库、下载中、等版本、还没播的集数（画板的顺序；缺的也算进来） */
export function legendCounts(
  eps: readonly SeasonEpisode[],
): { state: ItemEpisodeState; label: string; count: number }[] {
  const order: ItemEpisodeState[] = [
    "library",
    "downloading",
    "waiting_version",
    "missing",
    "upcoming",
  ];
  return order
    .map((state) => ({
      state,
      label: EP_STATE[state].label,
      count: eps.filter((e) => e.state === state).length,
    }))
    .filter((x) => x.count > 0);
}

const WEEKDAYS = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];

function localDay(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** 今天的日期（本地时区，YYYY-MM-DD） */
export function todayOf(now: Date): string {
  return localDay(now);
}

function clockOf(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/**
 * 集卡的播出日：「10-26 周一」、按播出时间追剧的今天写「今天 20:00」；最后一集加「· 季终」。
 */
export function airDayText(ep: SeasonEpisode, now: Date, last: boolean): string {
  if (!ep.air_date) return "播出日期未定";
  const [y, m, d] = ep.air_date.split("-").map(Number);
  const date = new Date(y, (m || 1) - 1, d || 1);
  let text = `${ep.air_date.slice(5)} ${WEEKDAYS[date.getDay()]}`;
  if (ep.air_date === localDay(now)) text = ep.air_at ? `今天 ${clockOf(ep.air_at)}` : "今天";
  else if (ep.air_at) text += ` ${clockOf(ep.air_at)}`;
  return last ? `${text} · 季终` : text;
}

/** 播出了几天（按本地日期；还没播或没日期时为 0） */
export function daysSinceAir(airDate: string | undefined, now: Date): number {
  if (!airDate) return 0;
  const [y, m, d] = airDate.split("-").map(Number);
  const day = new Date(y, (m || 1) - 1, d || 1);
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  return Math.max(0, Math.round((today.getTime() - day.getTime()) / 86400000));
}

/**
 * 集卡状态后面的说明：入库的「2160p HHWEB · 6.8 GB」、下载中的「63% · 还要 12 分钟」、等版本的「锁定 HHWEB 2160p」、
 * 缺的「播出 3 天，还没找到」、还没播的下一集「播出后 2 小时开始搜」（按播出时间追剧才有）。
 */
export function episodeNote(
  ep: SeasonEpisode,
  season: SeasonDetail,
  now: Date,
  isNext: boolean,
): string {
  switch (ep.state) {
    case "library":
      if (ep.server_issue) return `${ep.server_issue.server} 里没有了`;
      if (ep.source === "server" && ep.server) return `${ep.server} 里有`;
      return [ep.version, ep.size ? shortSize(ep.size) : ""].filter(Boolean).join(" · ");
    case "downloading": {
      if (ep.percent === undefined) return ep.site ? `在 ${ep.site} 下载` : "";
      const eta = etaText(ep.eta_seconds);
      return eta ? `${Math.floor(ep.percent)}% · 还要 ${eta}` : `${Math.floor(ep.percent)}%`;
    }
    case "waiting_version": {
      const l = season.subscription?.lock;
      return l ? `锁定 ${[l.group, l.resolution].filter(Boolean).join(" ")}` : "";
    }
    case "missing": {
      const n = daysSinceAir(ep.air_date, now);
      return n > 0 ? `播出 ${n} 天，还没找到` : "今天播出，还没找到";
    }
    case "upcoming": {
      const sub = season.subscription;
      if (!isNext || !sub?.airtime) return "";
      return `播出后 ${sub.search_delay_hours || 2} 小时开始搜`;
    }
  }
  return "";
}

/** 集卡右上的动作：等版本的「看版本」、缺的「搜资源」、媒体服务器里删了的「忽略」 */
export function episodeAction(ep: SeasonEpisode): "versions" | "search" | "ignore" | "" {
  if (ep.state === "waiting_version") return "versions";
  if (ep.state === "missing") return "search";
  if (ep.state === "library" && ep.server_issue) return "ignore";
  return "";
}

const EP_ACTION_LABEL = { versions: "看版本", search: "搜资源", ignore: "忽略", "": "" } as const;

export function episodeActionLabel(ep: SeasonEpisode): string {
  return EP_ACTION_LABEL[episodeAction(ep)];
}

/** 集卡上的播放圆：已入库、媒体服务器里有这一集时 */
export function episodePlayURL(ep: SeasonEpisode): string {
  return ep.state === "library" && !ep.server_issue ? (ep.open_url ?? "") : "";
}

/**
 * 开头连续已入库的集收成一行：超过 limit 集（桌面一季超过 24 集时收、手机连续超过 3 集就收）时，返回收起的集数。
 */
export function leadingLibrary(eps: readonly SeasonEpisode[]): number {
  let n = 0;
  for (const e of eps) {
    if (e.state !== "library") break;
    n++;
  }
  return n;
}

export function collapsedCount(eps: readonly SeasonEpisode[], mobile: boolean): number {
  const lead = leadingLibrary(eps);
  if (mobile) return lead > 3 ? lead : 0;
  return eps.length > 24 && lead > 0 ? lead : 0;
}

/** 收起行的文字：「E01–E20 · 20 集都已入库」 */
export function collapsedText(eps: readonly SeasonEpisode[], n: number): string {
  if (n <= 0) return "";
  const first = eps[0].episode;
  const last = eps[n - 1].episode;
  const code = (x: number) => `E${String(x).padStart(2, "0")}`;
  const span = first === last ? code(first) : `${code(first)}–${code(last)}`;
  return `${span} · ${n} 集都已入库`;
}

const TORRENT_STATE: Record<string, { label: string; tone: Tone }> = {
  seeding: { label: "做种中", tone: "ok" },
  downloading: { label: "下载中", tone: "info" },
  paused: { label: "暂停", tone: "neutral" },
  error: { label: "出错", tone: "dang" },
  unknown: { label: "没读到", tone: "neutral" },
};

export function torrentStatePill(state: string): { label: string; tone: Tone } {
  return TORRENT_STATE[state] ?? { label: state, tone: "neutral" };
}

/** 种子那一列：「S02E01–E04 · 2160p HHWEB · 27.3 GB」（整季写「S02 整季」；电影只写版本与体积） */
export function torrentLine(t: ItemTorrent, opts: { version?: boolean } = {}): string {
  const parts: string[] = [];
  if (t.season > 0 || t.episodes) {
    const s = t.season > 0 ? `S${String(t.season).padStart(2, "0")}` : "";
    if (t.episodes === "整季") parts.push(s ? `${s} 整季` : "整季");
    else parts.push(`${s}${t.episodes ?? ""}`);
  }
  if (opts.version !== false && t.version) parts.push(t.version);
  if (t.size > 0) parts.push(shortSize(t.size));
  return parts.filter(Boolean).join(" · ") || t.title;
}

/** 数据那一列：下载中「63% · 还要 12 分钟」，其余「分享率 1.31 · 做种 18 天」 */
export function torrentData(t: ItemTorrent, short = false): string {
  if (t.state === "downloading") {
    const eta = etaText(t.eta_seconds);
    return eta ? `${Math.floor(t.percent)}% · 还要 ${eta}` : `${Math.floor(t.percent)}%`;
  }
  if (t.state === "unknown") return "下载器没读到";
  const seeding = seedingText(t.seeding_seconds);
  const parts = [`分享率 ${t.ratio.toFixed(2)}`];
  if (seeding) parts.push(short ? seeding : `做种 ${seeding}`);
  return parts.join(" · ");
}

/** 能不能删：受保护的（下载中、H&R 还差）写成 warn 带盾牌 */
export function deletableView(t: ItemTorrent): { text: string; warn: boolean } {
  return { text: t.reason || (t.deletable ? "可以删" : ""), warn: !t.deletable };
}

/** 这一季的种子：剧集按季筛（不知道是哪一季的也列上），电影全部 */
export function seasonTorrents(
  torrents: readonly ItemTorrent[],
  kind: MediaKind,
  season: number,
): ItemTorrent[] {
  if (kind !== "tv") return [...torrents];
  return torrents.filter((t) => t.season === season || t.season === 0);
}

/** 季 tab 的文字：「第 2 季  6/12」（特别篇写「特别篇」） */
export function seasonTabText(s: {
  n: number;
  name?: string;
  episode_count: number;
  have: number;
}): {
  name: string;
  count: string;
} {
  return {
    name: s.n === 0 ? "特别篇" : `第 ${s.n} 季`,
    count: s.episode_count ? `${s.have}/${s.episode_count}` : "",
  };
}

/** 默认打开哪一季：路由里给的、订阅的最新一季、最后一个有集数的季（特别篇不算） */
export function defaultSeason(d: ItemDetail, wanted: number): number {
  if (d.item.media_type !== "tv") return 0;
  if (wanted > 0 && d.seasons.some((s) => s.n === wanted)) return wanted;
  if (d.subscriptions.length) return d.subscriptions[0].season;
  const regular = d.seasons.filter((s) => s.n > 0);
  const aired = regular.filter((s) => s.episode_count > 0);
  return (aired.at(-1) ?? regular.at(-1) ?? d.seasons[0])?.n ?? 1;
}
