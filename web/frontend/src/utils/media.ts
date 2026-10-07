import type {
  MediaKind,
  MediaMeta,
  MediaRecognizeResult,
  MediaWordKind,
  MediaWordRule,
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
