/*
 * 手机追剧编辑页的文字（追剧方案 F3，3.3.1）：编辑页跟着 App 的语言显示中文或英文，网页共用的规则
 * （utils/subscribeDefaults.ts、utils/entrySearch.ts）只有中文，这里按同样的规则写两种文字。
 * 中文与网页一字不差的地方直接调网页的函数。
 */
import type {
  MediaSearchEpisode,
  MediaSearchGroup,
  MediaSearchMember,
  MediaSearchResult,
} from "@/api";
import {
  type Pill,
  coverPill,
  discountText,
  epList,
  episodeNote,
  hrText,
  memberLine,
} from "@/utils/entrySearch";
import type {
  DownloadField,
  DownloadValues,
  FieldSource,
  LibraryDefaults,
  Sources,
} from "@/utils/subscribeDefaults";
import type { NamedOption, Schedule } from "./api";

type T = (zh: string, en: string) => string;

const word = (en: boolean) => (zh: string, english: string) => (en ? english : zh);

/** 标题包含、排除的文字拆成原样的词（按换行与逗号），给编辑页显示成标签 */
export function rawWords(text: string): string[] {
  return (text ?? "")
    .split(/[\n,，]/)
    .map((w) => w.trim())
    .filter(Boolean);
}

const WEEKDAYS_ZH = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];
const WEEKDAYS_EN = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

/** 搜索定时的一行摘要：「按播出时间 · 20:00」「每周 周二、周四 · 21:30」 */
export function scheduleSummary(s: Schedule, en: boolean): string {
  const t = word(en);
  const at = s.at?.trim() ? ` · ${s.at.trim()}` : "";
  const days = [...(s.days ?? [])].sort((a, b) => a - b);
  switch (s.kind) {
    case "":
      return t("跟随全局间隔", "Global interval");
    case "airtime":
      return t("按播出时间", "Airtime") + at;
    case "daily":
      return t("每天", "Daily") + at;
    case "every_days": {
      const n = s.every && s.every > 0 ? s.every : 1;
      return t(`每隔 ${n} 天`, `Every ${n} days`) + at;
    }
    case "weekly": {
      const list = days.length
        ? ` ${days.map((d) => (en ? WEEKDAYS_EN : WEEKDAYS_ZH)[d] ?? String(d)).join(en ? ", " : "、")}`
        : "";
      return t("每周", "Weekly") + list + at;
    }
    case "monthly": {
      const list = days.length ? ` ${days.join(en ? ", " : "、")}` : "";
      return (
        (en ? `Monthly${list ? ` on day${list}` : ""}` : `每月${list}${list ? " 日" : ""}`) + at
      );
    }
    case "interval": {
      const m = s.interval_minutes && s.interval_minutes > 0 ? s.interval_minutes : 360;
      return m % 60 === 0
        ? t(`每 ${m / 60} 小时`, `Every ${m / 60} h`)
        : t(`每 ${m} 分钟`, `Every ${m} min`);
    }
    case "cron":
      return `Cron ${s.cron?.trim() || "—"}`;
  }
  return s.kind;
}

function downloaderName(
  id: number,
  downloaders: readonly (NamedOption & { enabled?: boolean })[],
  t: T,
) {
  if (!id) return t("默认下载器", "Default downloader");
  const d = downloaders.find((x) => x.id === id);
  if (!d) return t(`下载器 ${id}（已删除）`, `Downloader ${id} (deleted)`);
  return d.enabled === false ? t(`${d.name}（已停用）`, `${d.name} (disabled)`) : d.name;
}

/** 整理与下载的一行摘要：「默认下载器 · /downloads/tv · 分类 tv · 标签 pt-tools」 */
export function downloadSummary(
  v: DownloadValues,
  downloaders: readonly (NamedOption & { enabled?: boolean })[],
  en: boolean,
): string {
  const t = word(en);
  return [
    downloaderName(v.downloader_id, downloaders, t),
    v.save_path.trim() || t("下载器的默认目录", "Downloader's default folder"),
    v.category.trim()
      ? t(`分类 ${v.category.trim()}`, `Category ${v.category.trim()}`)
      : t("不加分类", "No category"),
    v.tags.trim() ? t(`标签 ${v.tags.trim()}`, `Tags ${v.tags.trim()}`) : t("不加标签", "No tags"),
  ].join(" · ");
}

/** 一项来自媒体库时写在它下面的说明（规则同 utils/subscribeDefaults.ts 的 noteFor）；自己填的、没带出过的为空 */
export function fieldNote(
  source: FieldSource,
  d: LibraryDefaults | null,
  field: DownloadField,
  en: boolean,
): string {
  if (source.kind !== "library" || !d || d.libraryID !== source.libraryID) return "";
  const t = word(en);
  const lib = d.library;
  if (field === "downloader_id") {
    switch (d.downloader) {
      case "ok":
        return t(`来自媒体库「${lib}」`, `From library "${lib}"`);
      case "unset":
        return t(
          `媒体库「${lib}」没设下载器，用默认下载器`,
          `Library "${lib}" sets no downloader: the default one is used`,
        );
      case "disabled":
        return t(
          `媒体库「${lib}」设的下载器「${d.downloaderName}」已停用，按默认下载器推送`,
          `Downloader "${d.downloaderName}" set by library "${lib}" is disabled: sent to the default one`,
        );
      case "deleted":
        return t(
          `媒体库「${lib}」设的下载器已删除，按默认下载器推送`,
          `The downloader set by library "${lib}" was deleted: sent to the default one`,
        );
      default:
        return t(
          `读不到下载器列表，没有带出媒体库「${lib}」设的下载器`,
          `The downloader list could not be read: library "${lib}"'s downloader was not filled in`,
        );
    }
  }
  if (d.values[field]) return t(`来自媒体库「${lib}」`, `From library "${lib}"`);
  switch (field) {
    case "save_path":
      return t(
        `媒体库「${lib}」没设，按下载器的默认目录`,
        `Not set in library "${lib}": downloader's default folder`,
      );
    case "category":
      return t(`媒体库「${lib}」没设，不加分类`, `Not set in library "${lib}": no category`);
    default:
      return t(`媒体库「${lib}」没设，不加标签`, `Not set in library "${lib}": no tags`);
  }
}

/** 摘要下面那一行：四项都来自媒体库时写哪个库；只有几项来自库时写明其余是自己填的 */
export function sourceSummary(sources: Sources, d: LibraryDefaults | null, en: boolean): string {
  if (!d) return "";
  const t = word(en);
  const all = Object.values(sources);
  const fromLib = all.filter((s) => s.kind === "library" && s.libraryID === d.libraryID).length;
  if (!fromLib) return "";
  if (fromLib === all.length) return t(`来自媒体库「${d.library}」`, `From library "${d.library}"`);
  return t(
    `部分来自媒体库「${d.library}」，其余是自己填的`,
    `Partly from library "${d.library}", the rest set by you`,
  );
}

const list = (ns: readonly number[], en: boolean) =>
  en ? epList(ns).replaceAll("、", ", ") : epList(ns);

/** 这一季每集情况的一句（中文同网页的 episodeNote）：「缺 E02、E03」「E01 下载中」 */
export function episodeText(episodes: readonly MediaSearchEpisode[], en: boolean): string {
  if (!en) return episodeNote(episodes);
  const of = (state: MediaSearchEpisode["state"]) =>
    episodes.filter((e) => e.state === state).map((e) => e.episode);
  const missing = of("missing");
  const waiting = of("waiting_version");
  const downloading = of("downloading");
  const parts: string[] = [];
  if (missing.length && waiting.length)
    parts.push(`Missing ${list(missing, en)}`, `${list(waiting, en)} waiting for the version`);
  else if (missing.length) parts.push(`Missing ${list(missing, en)}`);
  else if (waiting.length) parts.push(`Missing ${list(waiting, en)} (waiting for the version)`);
  if (downloading.length) parts.push(`${list(downloading, en)} downloading`);
  if (parts.length === 0 && episodes.some((e) => e.state === "library"))
    return "Everything aired is in the library";
  return parts.join(" · ");
}

/** 一组能补什么（中文同网页的 coverPill）；都不是时为空 */
export function coverText(
  g: MediaSearchGroup,
  episodes: readonly MediaSearchEpisode[],
  movie: MediaSearchResult["movie"],
  en: boolean,
): Pill | null {
  if (!en) return coverPill(g, episodes, movie);
  if (g.in_library) return { text: "All in the library", tone: "neutral", dot: true };
  if (g.verdict?.reject) return { text: `Rejected: ${g.verdict.reject}`, tone: "warn" };
  if (g.covers.length) return { text: `Fills ${list(g.covers, en)}`, tone: "ok", dot: true };
  if (g.downloading.length) {
    const site = episodes.find((e) => e.episode === g.downloading[0])?.site;
    return {
      text: `${list(g.downloading, en)} downloading${site ? ` (${site})` : ""}`,
      tone: "info",
      dot: true,
    };
  }
  if (movie?.state === "downloading") {
    return { text: `Downloading${movie.site ? ` (${movie.site})` : ""}`, tone: "info", dot: true };
  }
  return null;
}

/** 一个种子的一行说明（中文同网页的 memberLine）：「做种 41 · 免费 18 时 · 无 H&R」 */
export function memberText(m: MediaSearchMember, en: boolean): string {
  if (!en) return memberLine(m);
  const d = discountText(m);
  const discount = d.text
    .replace("2x免费", "2x free")
    .replace("免费", "Free")
    .replace("无优惠", "No discount")
    .replace("2x上传", "2x upload")
    .replace(/ (\d+) 天$/, " $1d")
    .replace(/ (\d+) 时$/, " $1h")
    .replace(/ (\d+) 分$/, " $1m");
  const hr = hrText(m);
  const hrEn = !m.hr ? "No H&R" : m.hr_hours ? `H&R ${m.hr_hours}h` : "H&R";
  return [`${m.seeders} seeders`, discount, hr.warn ? hrEn : "No H&R"].join(" · ");
}
