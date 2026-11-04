/*
 * 首页（媒体中心 M8，画板 02、07）用到的纯函数：页头的一句话、今天播出卡、继续观看与最近入库的副行、这周还要播、
 * NAS 状态（速度、磁盘、状态行）的文字与颜色。
 */
import type {
  CalendarEntry,
  HomeBackupLine,
  HomeDisk,
  HomeDownloader,
  HomeHealthLine,
  HomeServerLine,
  HomeTodayItem,
  HomeTodoGroup,
  HomeTodoItem,
  MediaKind,
  MediaResumeGroup,
  MediaResumeItem,
  SpeedSample,
} from "@/api";
import { entryCode, entryTime, weekdayOf } from "@/utils/calendar";
import { formatShortDateTime } from "@/utils/format";
import { serverKindLabel } from "@/utils/media";
import { etaText, itemLink, searchLink } from "@/utils/mediaItem";
import { episodeState } from "@/utils/subscribe";
import type { RouteLocationRaw } from "vue-router";

export type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

const pad = (n: number) => String(n).padStart(2, "0");

/** 「3 集」「1 部」「3 集、1 部」；给了动词时写成「3 集播出、1 部上映」 */
function countText(list: readonly HomeTodayItem[], tvVerb = "", movieVerb = ""): string {
  const eps = list.filter((e) => e.media_type === "tv").length;
  const movies = list.length - eps;
  return [eps ? `${eps} 集${tvVerb}` : "", movies ? `${movies} 部${movieVerb}` : ""]
    .filter(Boolean)
    .join("、");
}

/**
 * 页头的一句话：「今天 5 集播出 · 其中 2 集已入库 · 6 件事要处理」；各块没取到的那段不写。
 * short 是手机顶栏（画板 07：「今天 5 集播出 · 6 件事要处理」），不写入库的那段。
 */
export function headline(
  today: readonly HomeTodayItem[] | null,
  todo: number | null,
  short = false,
): string {
  const parts: string[] = [];
  if (today) {
    if (!today.length) parts.push("今天没有要播的");
    else {
      parts.push(`今天 ${countText(today, "播出", "上映")}`);
      const inLib = today.filter((e) => e.state === "library");
      if (inLib.length && !short) parts.push(`其中 ${countText(inLib)}已入库`);
    }
  }
  if (todo !== null) parts.push(todo ? `${todo} 件事要处理` : "没有要处理的事");
  return parts.join(" · ");
}

/** 今天播出卡的动作行：播放（去媒体服务器）、搜资源、看版本，或者一句说明 */
export interface TodayAction {
  kind: "play" | "search" | "versions" | "note";
  text: string;
  icon?: string;
  /** 播放：媒体服务器里打开这一集（新标签页） */
  href?: string;
  /** 搜资源、看版本：站内跳转 */
  to?: RouteLocationRaw;
}

export interface TodayCard {
  state: { label: string; tone: Tone };
  /** 时刻牌：还没播写「20:00 播出」，别的写「19:30」；不是按播出时间追剧的为空 */
  time: string;
  /** 副行：「S02E09 · 第 9 集」「S01E14 · 2160p HHWEB」「S03E05 · HDSky 63%」「S01E21 · 播出 6 小时」「电影 · 今天上映」 */
  sub: string;
  action: TodayAction;
  /** 剧照，没有时用这一部的背景图 */
  image?: string;
}

/** 条目详情的地址（从首页进来）；episode 是要滚到的那一集 */
export function homeItemLink(
  kind: MediaKind,
  id: number,
  season?: number,
  episode?: number,
): RouteLocationRaw {
  const to = itemLink(kind, id, "home", season) as { path: string; query: Record<string, string> };
  if (kind === "tv" && episode && episode > 0) to.query.episode = String(episode);
  return to;
}

/** 播出了多久：「播出 6 小时」「刚播出」 */
function airedText(airAt: string | undefined, now: Date): string {
  if (!airAt) return "";
  const t = new Date(airAt).getTime();
  if (Number.isNaN(t)) return "";
  const hours = Math.floor((now.getTime() - t) / 3_600_000);
  if (hours < 0) return "";
  return hours < 1 ? "刚播出" : `播出 ${hours} 小时`;
}

/** 还没播的什么时候开始搜：「播出后 2 小时开始搜」（按播出时间追剧的）或「按追剧计划搜」 */
function searchText(e: HomeTodayItem): string {
  if (!e.search_at) return "按追剧计划搜";
  const search = new Date(e.search_at);
  if (Number.isNaN(search.getTime())) return "按追剧计划搜";
  if (e.air_at) {
    const air = new Date(e.air_at).getTime();
    if (!Number.isNaN(air)) {
      const h = Math.round((search.getTime() - air) / 3_600_000);
      return h > 0 ? `播出后 ${h} 小时开始搜` : "播出就开始搜";
    }
  }
  return `${pad(search.getHours())}:${pad(search.getMinutes())} 开始搜`;
}

export function todayCard(e: HomeTodayItem, now = new Date()): TodayCard {
  const st = episodeState(e.state);
  const tv = e.media_type === "tv";
  // 电影的卡写「电影 · 今天上映」（日历里写「上映」，放在卡上和后半句重复）
  const code = tv ? entryCode(e) : "电影";
  const time = entryTime(e);
  const named = e.name || (tv ? `第 ${e.episode ?? 0} 集` : "今天上映");
  let detail = named;
  let action: TodayAction = { kind: "note", text: "" };
  switch (e.state) {
    case "upcoming":
      action = { kind: "note", text: searchText(e) };
      break;
    case "library":
      detail = e.version || e.name || "已入库";
      action = e.open_url
        ? { kind: "play", text: "播放", icon: "play", href: e.open_url }
        : { kind: "note", text: "已入库" };
      break;
    case "downloading": {
      const parts = [
        e.site,
        e.percent !== undefined && e.percent > 0 ? `${Math.floor(e.percent)}%` : "",
      ].filter(Boolean);
      detail = parts.join(" ") || "下载中";
      const eta = etaText(e.eta_seconds);
      action = { kind: "note", text: eta ? `下载中，预计 ${eta}` : "下载中" };
      break;
    }
    case "missing":
      detail = airedText(e.air_at, now) || named;
      action = {
        kind: "search",
        text: "搜资源",
        icon: "search",
        to: searchLink(e.media_type, e.tmdb_id, e.season),
      };
      break;
    case "waiting_version":
      detail = e.lock ? `锁定 ${e.lock}` : "等锁住的版本";
      action = {
        kind: "versions",
        text: "看版本",
        icon: "layers",
        to: homeItemLink(e.media_type, e.tmdb_id, e.season, e.episode),
      };
      break;
    default:
      action = { kind: "note", text: "这一集不追" };
  }
  return {
    state: st,
    time: time ? (e.state === "upcoming" ? `${time} 播出` : time) : "",
    sub: `${code} · ${detail}`,
    action,
    image: e.still_path || e.backdrop_path || undefined,
  };
}

/** 手机上整张卡点了去哪：已入库去媒体服务器播放（没有时进详情），缺去按条目搜索，其余进详情 */
export function todayTap(e: HomeTodayItem): { href?: string; to?: RouteLocationRaw } {
  if (e.state === "library" && e.open_url) return { href: e.open_url };
  if (e.state === "missing") return { to: searchLink(e.media_type, e.tmdb_id, e.season) };
  return { to: homeItemLink(e.media_type, e.tmdb_id, e.season, e.episode) };
}

/** 还剩多久：「18 分钟」「1 小时 12 分」 */
export function remainingText(seconds?: number): string {
  if (!seconds || seconds <= 0) return "";
  const minutes = Math.max(1, Math.round(seconds / 60));
  if (minutes < 60) return `${minutes} 分钟`;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m ? `${h} 小时 ${m} 分` : `${h} 小时`;
}

/** 继续观看的副行：剧集「S01E13 · 还剩 18 分钟」，电影「还剩 1 小时 12 分」 */
export function resumeSub(it: MediaResumeItem): string {
  const left = remainingText(it.remaining_seconds);
  const leftText = left ? `还剩 ${left}` : `看了 ${Math.round(it.percent)}%`;
  if (it.media_type !== "tv" || !it.season) return leftText;
  return `S${pad(it.season)}E${pad(it.episode ?? 0)} · ${leftText}`;
}

/** 继续观看卡的图：剧集用这一集的剧照，电影用背景图；都没有时用海报 */
export function resumeImage(it: MediaResumeItem): { path: string; poster: boolean } | null {
  const still =
    it.media_type === "tv" ? it.still_path || it.backdrop_path : it.backdrop_path || it.still_path;
  if (still) return { path: still, poster: false };
  return it.poster_path ? { path: it.poster_path, poster: true } : null;
}

/** 继续观看摊平成一列（各台服务器的，按接口给的顺序） */
export function resumeItems(groups: readonly MediaResumeGroup[] | null): MediaResumeItem[] {
  return (groups ?? []).flatMap((g) => g.items);
}

/** 继续观看的身份 chip：「Jellyfin · admin」（第一台服务器的名字与选定的用户） */
export function resumeIdentity(groups: readonly MediaResumeGroup[] | null): string {
  const g = groups?.[0];
  if (!g) return "";
  return g.user ? `${g.name} · ${g.user}` : g.name;
}

/** 最近入库的副行：「S01E14」或「电影」 */
export function recentSub(it: {
  media_type: MediaKind;
  season?: number;
  episode?: number;
}): string {
  if (it.media_type !== "tv") return "电影";
  if (!it.season) return "剧集";
  return it.episode ? `S${pad(it.season)}E${pad(it.episode)}` : `第 ${it.season} 季`;
}

/** 这周还要播的一项：日期「周二 11-03」、标题「长夜将尽 S02E10」、说明「20:00 播出 · 已追剧」 */
export function weekLine(e: CalendarEntry): { date: string; title: string; note: string } {
  const tv = e.media_type === "tv";
  const time = entryTime(e);
  const tail = tv ? "已追剧" : "已订阅";
  return {
    date: `${weekdayOf(e.air_date)} ${e.air_date.slice(5)}`,
    title: tv ? `${e.title} ${entryCode(e)}` : `${e.title} 上映`,
    note: time ? `${time} ${tv ? "播出" : "上映"} · ${tail}` : tail,
  };
}

/** 速度的说法：「3.0 MB/s」「512 KB/s」「0 B/s」 */
export function speedText(bytesPerSec: number): string {
  if (!Number.isFinite(bytesPerSec) || bytesPerSec <= 0) return "0 B/s";
  if (bytesPerSec < 1024) return `${Math.round(bytesPerSec)} B/s`;
  const units = ["KB/s", "MB/s", "GB/s"];
  let v = bytesPerSec / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${v >= 100 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}

/** 大小的说法（磁盘）：「1.2 TB」「850 GB」 */
export function sizeText(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${v >= 100 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}

/** 速度柱的悬停提示：有哪一拍有下载器没取到时写出来 */
export function speedHint(history: readonly SpeedSample[]): string {
  const gaps = history.filter((s) => s.answered < s.total);
  if (!gaps.length) return history.length ? `最近 ${history.length} 拍的总速度（30 秒一拍）` : "";
  const last = gaps[gaps.length - 1];
  const miss = last.total - last.answered;
  return gaps.length === 1
    ? `有一拍有 ${miss} 台下载器没取到，那一拍只算了连上的`
    : `有 ${gaps.length} 拍有下载器没取到，那几拍只算了连上的`;
}

/** 一块盘的条：已用比例与颜色（已用 85% 以上 warn，剩得太少 dang）；不知道总量时 percent 为 null */
export function diskBar(d: HomeDisk): {
  percent: number | null;
  tone: "primary" | "warn" | "dang";
} {
  if (d.low) return { percent: d.total > 0 ? usedPercent(d) : null, tone: "dang" };
  if (d.total <= 0) return { percent: null, tone: "primary" };
  const used = usedPercent(d);
  return { percent: used, tone: used >= 85 ? "warn" : "primary" };
}

function usedPercent(d: HomeDisk): number {
  return Math.min(100, Math.max(0, ((d.total - d.free) / d.total) * 100));
}

/** 一块盘的剩余：「剩余 1.2 TB / 8.0 TB」（不知道总量时只写剩余）；取不到时写原因 */
export function diskText(d: HomeDisk, short = false): string {
  if (d.error) return `取不到：${d.error}`;
  const free = `剩余 ${sizeText(d.free)}`;
  return d.total > 0 && !short ? `${free} / ${sizeText(d.total)}` : free;
}

/** 手机只列下载盘与媒体库盘各一块：剩余最少的那块（读不到的排在后面） */
export function mobileDisks(disks: readonly HomeDisk[]): HomeDisk[] {
  const pick = (kind: HomeDisk["kind"]) =>
    disks
      .filter((d) => d.kind === kind)
      .sort((a, b) => Number(Boolean(a.error)) - Number(Boolean(b.error)) || a.free - b.free)[0];
  return [pick("download"), pick("library")].filter((d): d is HomeDisk => Boolean(d));
}

/** 速度标签：「下载 · 下载中 4」「上传 · 做种 1,284」（手机只写「下载」「上传」） */
export function speedLabels(
  dls: { downloading: number; seeding: number } | null,
  mobile: boolean,
): { down: string; up: string } {
  if (mobile || !dls) return { down: "下载", up: "上传" };
  return {
    down: `下载 · 下载中 ${dls.downloading.toLocaleString("en-US")}`,
    up: `上传 · 做种 ${dls.seeding.toLocaleString("en-US")}`,
  };
}

/** 下载器一行右侧的速度：「↓ 3.0 MB/s   ↑ 15.2 MB/s」；离线写原因 */
export function downloaderLine(d: HomeDownloader): string {
  if (!d.online) return d.error ? `离线：${d.error}` : "离线";
  return `↓ ${speedText(d.down)}   ↑ ${speedText(d.up)}`;
}

export interface StatusLine {
  key: "health" | "backup" | "server";
  icon: string;
  tone: Tone;
  title: string;
  desc: string;
  /** 右侧：时间（t3）或动作（主色） */
  right: string;
  rightKind: "time" | "action";
  to?: RouteLocationRaw;
  href?: string;
}

/** 状态行右侧的时刻：今天的只写「08:00」，别的天写「11-03 08:00」 */
export function whenText(at: string, now = new Date()): string {
  const d = new Date(at);
  if (Number.isNaN(d.getTime())) return "";
  if (d.toDateString() === now.toDateString()) return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  return formatShortDateTime(d, now);
}

/*
 * 三条状态行（画板 02、07）：标题写结论（手机只有标题），说明写第一个问题或细节（桌面才有）。
 */

/** 体检那一行：「体检：13 项通过，1 项提醒」，说明是第一项失败（没有时第一项提醒） */
export function healthLine(h: HomeHealthLine | null, now = new Date()): StatusLine {
  if (!h) {
    return {
      key: "health",
      icon: "activity",
      tone: "neutral",
      title: "体检：没体检过",
      desc: "查一遍 TMDB、下载器、媒体库与备份",
      right: "体检",
      rightKind: "action",
      to: "/system/check",
    };
  }
  const bad: string[] = [];
  if (h.fail) bad.push(`${h.fail} 项失败`);
  if (h.warn) bad.push(`${h.warn} 项提醒`);
  return {
    key: "health",
    icon: bad.length ? "triangle-alert" : "circle-check",
    tone: h.fail ? "dang" : bad.length ? "warn" : "ok",
    title: `体检：${h.ok} 项通过${bad.length ? `，${bad.join("、")}` : ""}`,
    desc: bad.length ? h.problem || "看系统体检里的说明" : "都正常",
    right: whenText(h.at, now),
    rightKind: "time",
    to: "/system/check",
  };
}

/** 备份那一行：「备份：3 天没成功」，说明是失败的原因 */
export function backupLine(b: HomeBackupLine | null, now = new Date()): StatusLine {
  const base = {
    key: "backup" as const,
    right: "设置",
    rightKind: "action" as const,
    to: "/global?section=backup",
  };
  if (!b)
    return { ...base, icon: "archive", tone: "neutral", title: "备份", desc: "读不到备份设置" };
  if (!b.enabled) {
    return {
      ...base,
      icon: "archive",
      tone: "neutral",
      title: "备份：每天定时备份关着",
      desc: "在全局设置里打开",
    };
  }
  const ok = b.last_ok_at || (b.error ? undefined : b.last_at);
  if (b.late) {
    const desc = b.error || (ok ? `最近一次成功在 ${whenText(ok, now)}` : "看备份与恢复里的原因");
    const title = b.days >= 2 ? `备份：${b.days} 天没成功` : "备份：最近一次失败";
    return { ...base, icon: "triangle-alert", tone: "warn", title, desc };
  }
  if (!ok)
    return {
      ...base,
      icon: "archive",
      tone: "neutral",
      title: "备份：没做过",
      desc: "每天定时备份开着，到点做第一份",
    };
  return {
    ...base,
    icon: "circle-check",
    tone: "ok",
    title: `备份：最近一次 ${whenText(ok, now)} 成功`,
    desc: "每天定时备份开着",
  };
}

/** 媒体服务器那一行（第一台）：「Jellyfin 12.2 · 14:00 同步」，说明是条目数与继续观看数；连不上、同步失败时标题写出来 */
export function serverLine(
  servers: readonly HomeServerLine[],
  resume: number | null,
  now = new Date(),
): StatusLine {
  const srv = servers[0];
  if (!srv) {
    return {
      key: "server",
      icon: "monitor",
      tone: "neutral",
      title: "媒体服务器",
      desc: "没有连媒体服务器",
      right: "去设置",
      rightKind: "action",
      to: "/media/library",
    };
  }
  const head = [serverKindLabel(srv.kind), srv.version].filter(Boolean).join(" ") || srv.name;
  const line = {
    key: "server" as const,
    icon: "monitor",
    right: srv.open_url ? "打开" : "",
    rightKind: "action" as const,
    href: srv.open_url,
  };
  if (!srv.online && srv.error)
    return { ...line, tone: "warn", title: `${head}：连不上`, desc: srv.error };
  if (srv.index_error)
    return { ...line, tone: "warn", title: `${head}：上次同步索引失败`, desc: srv.index_error };
  const parts = [
    `${srv.movies.toLocaleString("en-US")} 部电影`,
    `${srv.series.toLocaleString("en-US")} 部剧`,
  ];
  if (resume !== null) parts.push(`继续观看 ${resume}`);
  const synced = srv.indexed_at ? `${whenText(srv.indexed_at, now)} 同步` : "";
  return {
    ...line,
    tone: "neutral",
    title: [head, synced].filter(Boolean).join(" · "),
    desc: parts.join(" · "),
  };
}

/** 待处理那一行的图标（抽屉） */
export function todoIcon(kind: HomeTodoItem["kind"]): string {
  switch (kind) {
    case "organize_failed":
      return "circle-x";
    case "recognize":
      return "scan";
    case "waiting_version":
      return "layers";
    case "attendance_failed":
      return "calendar-check";
    case "login_failed":
      return "log-in";
    case "server_removed":
      return "monitor";
    case "library_missing":
      return "folder-open";
    case "backup":
      return "archive";
    default:
      return "activity";
  }
}

/** 待处理的总件数（各类的计数加起来） */
export function todoTotal(groups: readonly HomeTodoGroup[] | null): number | null {
  if (!groups) return null;
  return groups.reduce((n, g) => n + g.count, 0);
}

/** 是不是在首页抽屉里处理的那几类（每行有「处理」「忽略」） */
export function isIssueTodo(it: Pick<HomeTodoItem, "kind">): boolean {
  return it.kind === "server_removed" || it.kind === "library_missing";
}
