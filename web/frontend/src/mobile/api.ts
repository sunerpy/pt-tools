import type { MediaKind, MediaMeta, MediaSearchMember, MediaSearchResult } from "@/api";
import { type CallOptions, request } from "./bridge";
export interface NamedOption {
  id: number;
  name: string;
}
/** 下载器：旧主机没有 enabled（当作启用着） */
export interface DownloaderOption extends NamedOption {
  enabled?: boolean;
}
/** 下载器的一个预设保存目录（下载器里看到的路径）；旧主机没有 */
export interface DirectoryOption {
  downloader_id: number;
  path: string;
  alias?: string;
  is_default?: boolean;
}
export interface MobileOptions {
  sites: { name: string; display_name: string }[];
  downloaders: DownloaderOption[];
  directories?: DirectoryOption[];
  profiles: NamedOption[];
  libraries: NamedOption[];
  errors?: Record<string, string>;
}
/** /media/libraries 的一个库：新建订阅时带出的四项下载设置旧主机没有 */
export interface AppLibrary {
  id: number;
  name: string;
  kind: string;
  anime?: boolean;
  enabled: boolean;
  download_downloader_id?: number;
  download_save_path?: string;
  download_category?: string;
  download_tags?: string;
}
export interface AppMeta {
  version?: string;
  features?: string[];
}
/** 新建订阅前的条目（只读）：season 是这一季（请求的 0 换成最新一季），seasons 一共几季 */
export interface AppDraft {
  media_type: string;
  tmdb_id: number;
  season: number;
  seasons: number;
  total_episodes: number;
  title: string;
  original_title: string;
  year: number;
  anime: boolean;
  /** 媒体库选「自动」时整理会选到的库（没有能用的库时是 0） */
  auto_library_id: number;
  /** 这一季（这部电影）已有的订阅（没有时是 0） */
  subscription_id: number;
}
export interface Schedule {
  kind: string;
  interval_minutes?: number;
  cron?: string;
  at?: string;
  every?: number;
  air_delay_hours?: number;
  air_retry_hours?: number;
  air_window_days?: number;
  days?: number[];
}
export interface SubscriptionForm {
  media_type: string;
  tmdb_id: number;
  season: number;
  profile_id: number;
  sites: string[];
  downloader_id: number;
  category: string;
  tags: string;
  save_path: string;
  schedule: Schedule;
  lock_version: boolean;
  start_episode: number;
  follow_next_season: boolean;
  library_id: number;
  upgrade: boolean;
  include_keywords: string;
  exclude_keywords: string;
}
/** GET /subscriptions/{id}：表单的字段，加上片名与锁住的版本 */
export type SubscriptionDetail = Partial<SubscriptionForm> & {
  title: string;
  year?: number;
  lock_group?: string;
  lock_resolution?: string;
};
/** 试搜索：按表单里还没保存的季、站点、质量档案、标题包含与排除搜一次（只读） */
export interface PreviewRequest {
  media_type: string;
  tmdb_id: number;
  season: number;
  profile_id: number;
  sites: string[];
  include_keywords: string;
  exclude_keywords: string;
  include_unmatched: boolean;
}
export interface PreviewVerdict {
  score: number;
  reject: string;
}
export interface PreviewMember {
  site: string;
  torrent_id: string;
  title: string;
  subtitle: string;
  size_bytes: number;
  seeders: number;
  leechers: number;
  snatched: number;
  uploaded_at: number;
  discount: string;
  free: boolean;
  free_left_seconds: number;
  hr: boolean;
  hr_hours: number;
  category: string;
  by_imdb: boolean;
  blocked: boolean;
  verdict?: PreviewVerdict;
}
export interface PreviewGroup {
  group_id: string;
  meta: MediaMeta;
  verdict?: PreviewVerdict;
  span?: { start: number; end: number; whole: boolean };
  covers: number[];
  downloading: number[];
  in_library: boolean;
  blocked: boolean;
  members: PreviewMember[];
}
/** POST /subscriptions/search/preview 的回应（种子不带页面与下载地址） */
export interface AppPreview {
  item: {
    media_type: string;
    tmdb_id: number;
    season: number;
    title: string;
    original_title: string;
    year: number;
    seasons: number;
    total_episodes: number;
  };
  subscription?: { id: number; status: string; lock?: { group: string; resolution: string } };
  profile?: { id: number; name: string };
  keywords: string[];
  episodes: { number: number; state: string; site?: string; air_date?: string; name?: string }[];
  movie?: { state: string; site?: string };
  groups: PreviewGroup[];
  total: number;
  blocked_count: number;
  unmatched_count: number;
  unmatched: {
    site: string;
    torrent_id: string;
    title: string;
    subtitle: string;
    reason: string;
  }[];
  errors: { site: string; error: string; timed_out: boolean }[];
  duration_ms: number;
}

function memberOf(m: PreviewMember, meta: MediaMeta, groupVerdict?: PreviewVerdict) {
  const verdict = m.verdict ?? groupVerdict;
  return {
    ...m,
    meta,
    verdict: verdict ? { score: verdict.score, reject: verdict.reject } : null,
    block: m.blocked ? { id: 0 } : undefined,
  } satisfies MediaSearchMember;
}

/**
 * 试搜索的回应换成网页按条目搜索的写法：结果卡片、能补哪几集、版本锁对不对得上都和网页共用
 * utils/entrySearch.ts，手机不另写一套。成员没有自己的解析结果，用所在组的。
 */
export function previewResult(p: AppPreview): MediaSearchResult {
  return {
    item: { ...p.item, media_type: p.item.media_type as MediaKind },
    subscription: p.subscription
      ? {
          id: p.subscription.id,
          status: p.subscription.status as NonNullable<MediaSearchResult["subscription"]>["status"],
          lock: p.subscription.lock ?? null,
        }
      : null,
    profile: p.profile ?? null,
    keywords: p.keywords ?? [],
    episodes: (p.episodes ?? []).map((e) => ({
      episode: e.number,
      state: e.state as MediaSearchResult["episodes"][number]["state"],
      site: e.site,
      air_date: e.air_date,
      name: e.name,
    })),
    movie: p.movie as MediaSearchResult["movie"],
    groups: (p.groups ?? []).map((g) => ({
      group_id: g.group_id,
      meta: g.meta ?? {},
      verdict: g.verdict ? { score: g.verdict.score, reject: g.verdict.reject } : null,
      span: g.span,
      covers: g.covers ?? [],
      downloading: g.downloading ?? [],
      in_library: g.in_library,
      blocked: g.blocked,
      members: (g.members ?? []).map((m) => memberOf(m, g.meta ?? {}, g.verdict)),
    })),
    total: p.total,
    blocked_count: p.blocked_count,
    unmatched_count: p.unmatched_count,
    unmatched: (p.unmatched ?? []).map((u) => ({
      site: u.site,
      torrent_id: u.torrent_id,
      title: u.title,
      subtitle: u.subtitle,
      reason: u.reason,
      size_bytes: 0,
      seeders: 0,
      leechers: 0,
      discount: "",
      free: false,
      hr: false,
      meta: {},
      verdict: null,
    })),
    errors: p.errors ?? [],
  };
}

export const mobileApi = {
  meta: () => request<AppMeta>("/api/app/v1/meta"),
  options: () => request<MobileOptions>("/api/app/v1/mobile/options"),
  libraries: () => request<AppLibrary[]>("/api/app/v1/media/libraries"),
  subscription: (id: number) => request<SubscriptionDetail>(`/api/app/v1/subscriptions/${id}`),
  draft: (mediaType: string, tmdbID: number, season: number) =>
    request<AppDraft>(
      `/api/app/v1/subscriptions/draft?${new URLSearchParams({
        media_type: mediaType,
        tmdb_id: String(tmdbID),
        season: String(season),
      })}`,
    ),
  preview: (body: PreviewRequest, options: CallOptions = {}) =>
    request<AppPreview>("/api/app/v1/subscriptions/search/preview", "POST", body, options),
};
