import type {
  MediaHistoryItem,
  MediaKind,
  MediaMeta,
  MediaMode,
  MediaRecognizeResult,
  MediaServerKind,
  MediaTransferStatus,
  MediaWordKind,
  MediaWordRule,
  OrganizeItemStatus,
} from "@/api";

/** TMDB 的语言（与后端 recognize.Languages 一致，第一个是默认值） */
export const MEDIA_LANGUAGES: readonly { value: string; label: string }[] = [
  { value: "zh-CN", label: "简体中文" },
  { value: "zh-TW", label: "繁体中文（台湾）" },
  { value: "zh-HK", label: "繁体中文（香港）" },
  { value: "en-US", label: "English" },
  { value: "ja-JP", label: "日本語" },
];

export function mediaKindLabel(kind?: string): string {
  if (kind === "movie") return "电影";
  if (kind === "tv") return "剧集";
  return "类型未知";
}

const SOURCE_LABELS: Record<MediaRecognizeResult["source"], string> = {
  override: "手动纠正",
  imdb: "按 IMDb 编号",
  search: "按名字搜索",
  none: "没有匹配",
};

export function mediaSourceLabel(source: MediaRecognizeResult["source"]): string {
  return SOURCE_LABELS[source] ?? source;
}

const WORD_KIND_LABELS: Record<MediaWordKind, string> = {
  block: "屏蔽",
  replace: "替换",
  offset: "集数偏移",
};

export const MEDIA_WORD_KINDS: readonly { value: MediaWordKind; label: string }[] = (
  Object.keys(WORD_KIND_LABELS) as MediaWordKind[]
).map((value) => ({ value, label: WORD_KIND_LABELS[value] }));

export function wordKindLabel(kind: MediaWordKind): string {
  return WORD_KIND_LABELS[kind] ?? kind;
}

/** 一条识别词做什么（给列表看） */
export function wordEffect(r: Pick<MediaWordRule, "kind" | "replacement" | "offset">): string {
  switch (r.kind) {
    case "block":
      return "从标题与副标题里去掉";
    case "replace":
      return r.replacement ? `换成「${r.replacement}」` : "换成空";
    case "offset":
      return `集数 ${r.offset > 0 ? "+" : ""}${r.offset}`;
  }
  return "";
}

const pad = (n: number) => String(n).padStart(2, "0");

/** 季与集：S01E01-E03、S01-S05、S02（全 30 集）、第 5 集；不是剧集时为空串 */
export function episodeText(m: MediaMeta): string {
  const parts: string[] = [];
  if (m.season)
    parts.push(m.season_end ? `S${pad(m.season)}-S${pad(m.season_end)}` : `S${pad(m.season)}`);
  if (m.episode) {
    const ep = m.episode_end ? `E${pad(m.episode)}-E${pad(m.episode_end)}` : `E${pad(m.episode)}`;
    if (parts.length) parts[0] += ep;
    else parts.push(m.episode_end ? `第 ${m.episode}-${m.episode_end} 集` : `第 ${m.episode} 集`);
  }
  if (m.total_episodes) parts.push(`全 ${m.total_episodes} 集`);
  else if (m.complete && m.season && !m.episode) parts.push("整季");
  return parts.length > 1 ? `${parts[0]}（${parts.slice(1).join("，")}）` : (parts[0] ?? "");
}

/** 解析结果里的各项，按 类型、年份、季集、画质、音轨、版本、语言、制作组 排列 */
export function metaTags(m: MediaMeta): string[] {
  const tags: string[] = [];
  if (m.type) tags.push(mediaKindLabel(m.type));
  if (m.year) tags.push(String(m.year));
  const ep = episodeText(m);
  if (ep) tags.push(ep);
  if (m.resolution) tags.push(m.resolution);
  if (m.source) tags.push(m.remux ? `${m.source} Remux` : m.source);
  else if (m.remux) tags.push("Remux");
  if (m.platform) tags.push(m.platform);
  if (m.video_codec) tags.push(m.bit_depth ? `${m.video_codec} ${m.bit_depth}bit` : m.video_codec);
  if (m.fps) tags.push(`${m.fps}fps`);
  tags.push(...(m.hdr ?? []));
  const audio = (m.audio ?? []).join(" ");
  if (audio || m.channels) tags.push([audio, m.channels].filter(Boolean).join(" "));
  tags.push(...(m.edition ?? []));
  if (m.version) tags.push(m.version);
  if (m.three_d) tags.push("3D");
  if (m.chinese_subs) tags.push("中字");
  if (m.mandarin) tags.push("国语");
  if (m.cantonese) tags.push("粤语");
  if (m.group) tags.push(`制作组 ${m.group}`);
  return tags;
}

/** TMDB 网页上这个条目的地址 */
export function tmdbPageURL(kind: MediaKind, id: number): string {
  return `https://www.themoviedb.org/${kind}/${id}`;
}

/** TMDB 海报地址；没有海报时为空串 */
export function posterURL(path?: string, size = "w185"): string {
  if (!path) return "";
  return `https://image.tmdb.org/t/p/${size}/${path.replace(/^\//, "")}`;
}

// ---- 整理入库（M10） ----

export const MEDIA_MODES: readonly { value: MediaMode; label: string; hint: string }[] = [
  {
    value: "hardlink",
    label: "硬链接",
    hint: "不多占空间，种子照常做种；下载目录与媒体库要在同一个分区（Docker 里是同一个卷）",
  },
  { value: "copy", label: "复制", hint: "多占一份空间，跨分区也能用" },
  {
    value: "symlink",
    label: "软链接",
    hint: "不占空间；媒体服务器要能用同一个路径读到下载目录，删种后链接失效",
  },
  { value: "move", label: "移动", hint: "文件搬进库里，种子不能再做种；只能整理已暂停的种子" },
];

export function modeLabel(mode?: string): string {
  return MEDIA_MODES.find((m) => m.value === (mode || "hardlink"))?.label ?? mode ?? "";
}

/** 命名模板的预设：默认的三家都认；目录名里带上 TMDB 编号时按媒体服务器的写法 */
export function templatePresets(kind: MediaKind): { label: string; template: string }[] {
  const dir = "{{.Title}}{{if .Year}} ({{.Year}}){{end}}";
  const file =
    kind === "tv"
      ? "Season {{.Season}}/{{.Title}} - {{.SeasonEpisode}}{{if .EpisodeTitle}} - {{.EpisodeTitle}}{{end}}"
      : "{{.Title}}{{if .Year}} ({{.Year}}){{end}}{{if .Quality}} - {{.Quality}}{{end}}";
  return [
    { label: "默认", template: "" },
    { label: "Emby（带 tmdbid=）", template: `${dir} [tmdbid={{.TMDBID}}]/${file}` },
    { label: "Jellyfin（带 tmdbid-）", template: `${dir} [tmdbid-{{.TMDBID}}]/${file}` },
    { label: "Plex（带 {tmdb-}）", template: `${dir} {tmdb-{{.TMDBID}}}/${file}` },
  ];
}

/** 模板里能用的变量（给编辑框旁边的说明） */
export const TEMPLATE_VARS: readonly { name: string; desc: string }[] = [
  { name: ".Title", desc: "标题（TMDB，按设置的语言）" },
  { name: ".OriginalTitle", desc: "原名" },
  { name: ".Year", desc: "年份（剧集是首播年份）" },
  { name: ".TMDBID", desc: "TMDB 编号" },
  { name: ".IMDbID", desc: "IMDb 编号" },
  { name: ".Season", desc: "季" },
  { name: ".Episode", desc: "集" },
  { name: ".SeasonEpisode", desc: "S01E02，多集时 S01E02-E03" },
  { name: ".EpisodeTitle", desc: "这一集的标题" },
  { name: ".Quality", desc: "分辨率、来源、HDR、编码，如 2160p WEB-DL HDR10 H.265" },
  { name: ".Resolution", desc: "分辨率" },
  { name: ".Source", desc: "来源，如 BluRay、WEB-DL" },
  { name: ".VideoCodec", desc: "视频编码" },
  { name: ".HDR", desc: "HDR 格式" },
  { name: ".Audio", desc: "音轨" },
  { name: ".Group", desc: "制作组" },
  { name: ".Edition", desc: "版本说明" },
];

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

const HISTORY_STATUS: Record<MediaTransferStatus, { label: string; tone: Tone }> = {
  done: { label: "已整理", tone: "ok" },
  failed: { label: "失败", tone: "dang" },
  skipped: { label: "跳过", tone: "warn" },
  removed: { label: "已删除", tone: "neutral" },
};

export function historyStatus(status: string): { label: string; tone: Tone } {
  return HISTORY_STATUS[status as MediaTransferStatus] ?? { label: status, tone: "neutral" };
}

export const HISTORY_FILTERS: readonly { value: "" | MediaTransferStatus; label: string }[] = [
  { value: "", label: "全部" },
  { value: "done", label: "已整理" },
  { value: "failed", label: "失败" },
  { value: "skipped", label: "跳过" },
  { value: "removed", label: "已删除" },
];

export function triggerLabel(trigger: string): string {
  return { auto: "下载完成后自动", scan: "定期扫描", manual: "手动" }[trigger] ?? trigger;
}

const ITEM_STATUS: Record<OrganizeItemStatus, { label: string; tone: Tone }> = {
  pending: { label: "待整理", tone: "info" },
  done: { label: "已在库里", tone: "ok" },
  exists: { label: "目标已有文件", tone: "warn" },
  failed: { label: "整理不了", tone: "dang" },
};

export function itemStatus(status: OrganizeItemStatus): { label: string; tone: Tone } {
  return ITEM_STATUS[status] ?? { label: status, tone: "neutral" };
}

export const MEDIA_SERVER_KINDS: readonly { value: MediaServerKind; label: string }[] = [
  { value: "emby", label: "Emby" },
  { value: "jellyfin", label: "Jellyfin" },
  { value: "plex", label: "Plex" },
];

export function serverKindLabel(kind: string): string {
  return MEDIA_SERVER_KINDS.find((k) => k.value === kind)?.label ?? kind;
}

/** S01E02，多集时 S01E02-E03；没有集数时只写季 */
export function seasonEpisode(season?: number, episode?: number, end?: number): string {
  const s = `S${pad(season ?? 0)}`;
  if (!episode) return s;
  return end && end > episode ? `${s}E${pad(episode)}-E${pad(end)}` : `${s}E${pad(episode)}`;
}

/** 整理记录对应的条目：标题（年份），剧集再加季集 */
export function historyEntry(
  r: Pick<MediaHistoryItem, "title" | "year" | "media_type" | "season" | "episode" | "episode_end">,
): string {
  if (!r.title) return "";
  const name = r.year ? `${r.title} (${r.year})` : r.title;
  return r.media_type === "tv"
    ? `${name} ${seasonEpisode(r.season, r.episode, r.episode_end)}`
    : name;
}
