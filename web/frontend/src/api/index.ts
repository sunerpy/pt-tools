const BASE_URL = "";

interface ApiOptions extends RequestInit {
  body?: string;
}

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(path: string, options: ApiOptions = {}): Promise<T> {
  const response = await fetch(`${BASE_URL}${path}`, {
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...options.headers,
    },
    ...options,
  });

  const contentType = response.headers.get("content-type") || "";
  const isJSON = contentType.includes("application/json");

  if (!response.ok) {
    const msg = await response.text();
    throw new ApiError(response.status, msg || `HTTP ${response.status}`);
  }

  if (isJSON) {
    return response.json() as Promise<T>;
  }
  return response.text() as unknown as T;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, data?: unknown) =>
    request<T>(path, {
      method: "POST",
      body: data ? JSON.stringify(data) : undefined,
    }),
  delete: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};

// API 类型定义
export interface GlobalSettings {
  default_interval_minutes: number;
  download_dir: string;
  download_limit_enabled: boolean;
  download_speed_limit: number;
  torrent_size_gb: number;
  min_free_minutes: number;
  auto_start: boolean;
  cleanup_enabled?: boolean;
  cleanup_interval_min?: number;
  cleanup_scope?: string;
  cleanup_scope_tags?: string;
  cleanup_remove_data?: boolean;
  cleanup_condition_mode?: string;
  cleanup_max_seed_time_h?: number;
  cleanup_min_ratio?: number;
  cleanup_max_inactive_h?: number;
  cleanup_slow_seed_time_h?: number;
  cleanup_slow_max_ratio?: number;
  cleanup_del_free_expired?: boolean;
  cleanup_disk_protect?: boolean;
  cleanup_min_disk_space_gb?: number;
  cleanup_protect_dl?: boolean;
  cleanup_protect_hr?: boolean;
  cleanup_min_retain_h?: number;
  cleanup_protect_tags?: string;
  auto_delete_on_free_end?: boolean;
}

export interface QbitSettings {
  enabled: boolean;
  url: string;
  user: string;
  password: string;
}

export interface RSSConfig {
  id?: number;
  name: string;
  url: string;
  category: string;
  tag: string;
  interval_minutes: number;
  downloader_id?: number; // 指定下载器，undefined 表示使用默认下载器
  download_path?: string; // 下载器中下载任务的目标下载路径（可选）
  filter_rule_ids?: number[]; // 关联的过滤规则 ID 列表
  pause_on_free_end?: boolean; // 免费结束时是否暂停未完成的下载
  is_example?: boolean; // 是否为示例配置
}

export interface SiteConfig {
  enabled: boolean;
  auth_method: string;
  cookie: string;
  api_key: string;
  api_url: string;
  passkey?: string;
  rss: RSSConfig[];
  urls?: string[];
  unavailable?: boolean;
  unavailable_reason?: string;
  is_builtin?: boolean;
}

export interface TaskItem {
  siteName: string;
  title: string;
  category: string;
  tag: string;
  torrentHash: string;
  isFree: boolean;
  freeLevel: string;
  freeEndTime: string;
  isDownloaded: boolean;
  isPushed: boolean;
  createdAt: string;
  lastCheckTime: string;
  isExpired: boolean;
  retryCount: number;
  lastError: string;
  pushTime: string;
  progress: number; // 下载进度 0-100
  torrentSize: number; // 种子大小（字节）
}

export interface TaskListResponse {
  items: TaskItem[];
  total: number;
  page: number;
  page_size: number;
}

export interface LogsResponse {
  lines: string[];
  path: string;
  truncated: boolean;
}

// API 方法
export const globalApi = {
  get: () => api.get<GlobalSettings>("/api/global"),
  save: (data: GlobalSettings) => api.post<void>("/api/global", data),
};

export const qbitApi = {
  get: () => api.get<QbitSettings>("/api/qbit"),
  save: (data: QbitSettings) => api.post<void>("/api/qbit", data),
};

export const sitesApi = {
  list: () => api.get<Record<string, SiteConfig>>("/api/sites"),
  get: (name: string) => api.get<SiteConfig>(`/api/sites/${name}`),
  save: (name: string, data: SiteConfig) => api.post<void>(`/api/sites/${name}`, data),
  delete: (name: string) => api.delete<void>(`/api/sites?name=${encodeURIComponent(name)}`),
  deleteRss: (name: string, id: number) =>
    api.delete<void>(`/api/sites/${name}?id=${encodeURIComponent(id.toString())}`),
};

export const tasksApi = {
  list: (params: URLSearchParams) => api.get<TaskListResponse>(`/api/tasks?${params.toString()}`),
};

export const logsApi = {
  get: () => api.get<LogsResponse>("/api/logs"),
};

export const controlApi = {
  stop: () => api.post<void>("/api/control/stop"),
  start: () => api.post<void>("/api/control/start"),
};

export const passwordApi = {
  change: (data: { username: string; old: string; new: string }) =>
    api.post<void>("/api/password", data),
};

// 下载器相关类型
export interface DownloaderSetting {
  id?: number;
  name: string;
  type: string; // qbittorrent, transmission
  url: string;
  username: string;
  password?: string;
  is_default: boolean;
  enabled: boolean;
  auto_start?: boolean;
  extra_config?: string;
}

export interface DownloaderHealthResponse {
  name: string;
  is_healthy: boolean;
  message?: string;
}

// 动态站点相关类型
export interface DynamicSiteSetting {
  id?: number;
  name: string;
  display_name: string;
  base_url: string;
  enabled: boolean;
  auth_method: string;
  cookie?: string;
  api_key?: string;
  api_url?: string;
  passkey?: string;
  downloader_id?: number;
  parser_config?: string;
  is_builtin: boolean;
}

export interface SiteValidationRequest {
  name: string;
  base_url: string;
  auth_method: string;
  cookie?: string;
  api_key?: string;
  api_url?: string;
  passkey?: string;
}

export interface SiteValidationResponse {
  valid: boolean;
  message: string;
  free_torrents?: string[];
}

export interface SiteTemplate {
  id: number;
  name: string;
  display_name: string;
  base_url: string;
  auth_method: string;
  description?: string;
  version?: string;
  author?: string;
}

export interface TemplateImportRequest {
  template: unknown;
  cookie?: string;
  api_key?: string;
}

// 下载器 API
export const downloadersApi = {
  list: () => api.get<DownloaderSetting[]>("/api/downloaders"),
  get: (id: number) => api.get<DownloaderSetting>(`/api/downloaders/${id}`),
  create: (data: DownloaderSetting) => api.post<DownloaderSetting>("/api/downloaders", data),
  update: (id: number, data: DownloaderSetting) =>
    request<DownloaderSetting>(`/api/downloaders/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  delete: (id: number) => api.delete<void>(`/api/downloaders/${id}`),
  health: (id: number) => api.get<DownloaderHealthResponse>(`/api/downloaders/${id}/health`),
  setDefault: (id: number) => api.post<DownloaderSetting>(`/api/downloaders/${id}/set-default`),
  applyToSites: (id: number, siteIds: number[]) =>
    api.post<{ updated_count: number }>(`/api/downloaders/${id}/apply-to-sites`, {
      site_ids: siteIds,
    }),
};

export interface SiteDownloaderSummaryItem {
  site_id: number;
  site_name: string;
  display_name: string;
  downloader_id?: number;
  downloader_name?: string;
}

export interface SiteDownloaderSummaryResponse {
  sites: SiteDownloaderSummaryItem[];
}

// 动态站点 API
export const dynamicSitesApi = {
  list: () => api.get<DynamicSiteSetting[]>("/api/sites/dynamic"),
  create: (data: Omit<DynamicSiteSetting, "id" | "is_builtin">) =>
    api.post<DynamicSiteSetting>("/api/sites/dynamic", data),
  validate: (data: SiteValidationRequest) =>
    api.post<SiteValidationResponse>("/api/sites/validate", data),
  getDownloaderSummary: () =>
    api.get<SiteDownloaderSummaryResponse>("/api/sites/downloader-summary"),
};

// 站点模板 API
export const templatesApi = {
  list: () => api.get<SiteTemplate[]>("/api/sites/templates"),
  import: (data: TemplateImportRequest) =>
    api.post<DynamicSiteSetting>("/api/sites/templates/import", data),
  export: (id: number) => api.get<unknown>(`/api/sites/templates/${id}/export`),
};

// 过滤规则相关类型
export interface FilterRule {
  id?: number;
  name: string;
  pattern: string;
  pattern_type: "keyword" | "wildcard" | "regex";
  match_field?: "title" | "tag" | "both";
  require_free: boolean;
  enabled: boolean;
  site_id?: number;
  rss_id?: number;
  priority: number;
  created_at?: string;
  updated_at?: string;
}

export interface FilterRuleTestRequest {
  pattern: string;
  pattern_type: string;
  match_field?: string;
  require_free?: boolean;
  site_id?: number;
  rss_id?: number;
  limit?: number;
}

export interface FilterRuleTestMatch {
  title: string;
  tag: string;
  is_free: boolean;
}

export interface FilterRuleTestResponse {
  match_count: number;
  total_count: number;
  matches: FilterRuleTestMatch[];
}

// 过滤规则 API
export const filterRulesApi = {
  list: () => api.get<FilterRule[]>("/api/filter-rules"),
  get: (id: number) => api.get<FilterRule>(`/api/filter-rules/${id}`),
  create: (data: Omit<FilterRule, "id" | "created_at" | "updated_at">) =>
    api.post<FilterRule>("/api/filter-rules", data),
  update: (id: number, data: Partial<FilterRule>) =>
    request<FilterRule>(`/api/filter-rules/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  delete: (id: number) => api.delete<void>(`/api/filter-rules/${id}`),
  test: (data: FilterRuleTestRequest) =>
    api.post<FilterRuleTestResponse>("/api/filter-rules/test", data),
};

// RSS-Filter 关联相关类型
export interface RSSFilterAssociationResponse {
  rss_id: number;
  filter_rule_ids: number[];
  filter_rules: FilterRule[];
}

export interface RSSFilterAssociationRequest {
  filter_rule_ids: number[];
}

// RSS-Filter 关联 API
export const rssFilterApi = {
  get: (rssId: number) => api.get<RSSFilterAssociationResponse>(`/api/rss/${rssId}/filter-rules`),
  update: (rssId: number, data: RSSFilterAssociationRequest) =>
    request<RSSFilterAssociationResponse>(`/api/rss/${rssId}/filter-rules`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
};

// 日志级别相关类型
export interface LogLevelResponse {
  level: string;
  levels: string[];
  message?: string;
}

export interface LogLevelRequest {
  level: string;
}

// 日志级别 API
export const logLevelApi = {
  get: () => api.get<LogLevelResponse>("/api/log-level"),
  set: (level: string) =>
    request<LogLevelResponse>("/api/log-level", {
      method: "PUT",
      body: JSON.stringify({ level }),
    }),
};

// 用户信息相关类型
export interface UserInfoResponse {
  site: string;
  username: string;
  userId: string;
  uploaded: number;
  downloaded: number;
  ratio: number;
  bonus: number;
  seeding: number;
  leeching: number;
  rank: string;
  joinDate?: number;
  lastAccess?: number;
  lastUpdate: number;
  // Extended fields
  levelName?: string; // 等级名称
  levelId?: number; // 等级 ID
  bonusPerHour?: number; // 时魔（每小时魔力值）
  seedingBonus?: number; // 做种积分
  seedingBonusPerHour?: number; // 每小时做种积分
  unreadMessageCount?: number; // 未读消息数
  totalMessageCount?: number; // 消息总数
  seederCount?: number; // 做种数量（来自 peer statistics）
  seederSize?: number; // 做种总大小（bytes）
  leecherCount?: number; // 下载数量
  leecherSize?: number; // 下载总大小（bytes）
  hnrUnsatisfied?: number; // 未满足的 H&R 数量
  hnrPreWarning?: number; // H&R 预警数量
  trueUploaded?: number; // 真实上传量
  trueDownloaded?: number; // 真实下载量
  uploads?: number; // 发布数量
}

export interface AggregatedStatsResponse {
  totalUploaded: number;
  totalDownloaded: number;
  averageRatio: number;
  totalSeeding: number;
  totalLeeching: number;
  totalBonus: number;
  siteCount: number;
  lastUpdate: number;
  perSiteStats: UserInfoResponse[];
  // Extended aggregated fields
  totalBonusPerHour?: number; // 所有站点时魔总和
  totalSeedingBonus?: number; // 所有站点做种积分总和
  totalUnreadMessages?: number; // 所有站点未读消息总和
  totalSeederSize?: number; // 所有站点做种总大小
  totalLeecherSize?: number; // 所有站点下载总大小
}

export interface SyncRequest {
  sites?: string[];
}

export interface SyncResponse {
  success: string[];
  failed?: { site: string; error: string }[];
}

// 用户信息 API
export const userInfoApi = {
  getAggregated: () => api.get<AggregatedStatsResponse>("/api/v2/userinfo/aggregated"),
  getSites: () => api.get<UserInfoResponse[]>("/api/v2/userinfo/sites"),
  getSite: (siteId: string) => api.get<UserInfoResponse>(`/api/v2/userinfo/sites/${siteId}`),
  syncSite: (siteId: string) => api.post<UserInfoResponse>(`/api/v2/userinfo/sites/${siteId}`),
  deleteSite: (siteId: string) => api.delete<void>(`/api/v2/userinfo/sites/${siteId}`),
  syncAll: (sites?: string[]) =>
    api.post<SyncResponse>("/api/v2/userinfo/sync", sites ? { sites } : {}),
  getRegisteredSites: () => api.get<{ sites: string[] }>("/api/v2/userinfo/registered"),
  clearCache: () => api.post<{ status: string }>("/api/v2/userinfo/cache/clear"),
};

// 批量下载相关类型
export interface FreeTorrentBatchRequest {
  archiveType: "tar.gz" | "zip";
}

export interface TorrentManifestItem {
  id: string;
  title: string;
  sizeBytes: number;
  discountLevel: string;
  downloadUrl: string;
  category?: string;
  seeders?: number;
  leechers?: number;
}

export interface FreeTorrentBatchResponse {
  archivePath: string;
  archiveType: string;
  torrentCount: number;
  totalSize: number;
  manifest: TorrentManifestItem[];
}

// 批量下载 API
export const batchDownloadApi = {
  // 获取免费种子列表
  listFreeTorrents: (siteId: string) =>
    api.get<TorrentManifestItem[]>(`/api/site/${siteId}/free-torrents`),
  // 下载免费种子压缩包
  downloadFreeTorrents: (siteId: string, archiveType: "tar.gz" | "zip" = "tar.gz") =>
    api.get<FreeTorrentBatchResponse>(
      `/api/site/${siteId}/free-torrents/download?type=${archiveType}`,
    ),
};

// 站点等级要求相关类型
export interface AlternativeRequirement {
  seedingBonus?: number;
  uploads?: number;
  bonus?: number;
  downloaded?: string;
  ratio?: number;
}

export interface SiteLevelRequirement {
  id: number;
  name: string;
  nameAka?: string[];
  groupType?: "user" | "vip" | "manager";
  interval?: string; // ISO 8601 duration, e.g., "P5W" for 5 weeks
  downloaded?: string; // e.g., "200GB"
  uploaded?: string;
  ratio?: number;
  bonus?: number;
  seedingBonus?: number;
  uploads?: number;
  seeding?: number;
  seedingSize?: string;
  alternative?: AlternativeRequirement[];
  privilege?: string;
}

export interface SiteLevelsResponse {
  siteId: string;
  siteName: string;
  levels: SiteLevelRequirement[];
}

export interface AllSiteLevelsResponse {
  sites: Record<string, SiteLevelsResponse>;
}

// 站点等级 API
export const siteLevelsApi = {
  get: (siteId: string) => api.get<SiteLevelsResponse>(`/api/v2/sites/${siteId}/levels`),
  getAll: () => api.get<AllSiteLevelsResponse>("/api/v2/sites/levels"),
};

// ============== 下载器目录管理 ==============

export interface DownloaderDirectory {
  id?: number;
  downloader_id: number;
  path: string;
  alias: string;
  is_default: boolean;
  created_at?: string;
  updated_at?: string;
}

// 下载器目录 API
export const downloaderDirectoriesApi = {
  list: (downloaderId: number) =>
    api.get<DownloaderDirectory[]>(`/api/downloaders/${downloaderId}/directories`),
  create: (
    downloaderId: number,
    data: Omit<DownloaderDirectory, "id" | "downloader_id" | "created_at" | "updated_at">,
  ) => api.post<DownloaderDirectory>(`/api/downloaders/${downloaderId}/directories`, data),
  update: (downloaderId: number, dirId: number, data: Partial<DownloaderDirectory>) =>
    request<DownloaderDirectory>(`/api/downloaders/${downloaderId}/directories/${dirId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  delete: (downloaderId: number, dirId: number) =>
    api.delete<void>(`/api/downloaders/${downloaderId}/directories/${dirId}`),
  setDefault: (downloaderId: number, dirId: number) =>
    api.post<DownloaderDirectory>(
      `/api/downloaders/${downloaderId}/directories/${dirId}/set-default`,
    ),
  listAll: () => api.get<Record<number, DownloaderDirectory[]>>("/api/downloaders/all-directories"),
};

// ============== 站点分类配置 ==============

export interface SiteCategoryOption {
  value: string;
  name: string;
}

export interface SiteCategory {
  key: string;
  name: string;
  options: SiteCategoryOption[];
}

export interface SiteCategoriesConfig {
  site_id: string;
  site_name: string;
  categories: SiteCategory[];
}

// 站点分类 API
export const siteCategoriesApi = {
  getAll: () => api.get<Record<string, SiteCategoriesConfig>>("/api/v2/sites/categories"),
  get: (siteId: string) => api.get<SiteCategoriesConfig>(`/api/v2/sites/${siteId}/categories`),
  getSupportedSites: () => api.get<string[]>("/api/v2/sites/supported"),
};

// ============== 种子搜索相关 ==============

export interface SearchTorrentItem {
  id: string;
  title: string;
  subtitle?: string;
  url?: string;
  downloadUrl?: string;
  magnetLink?: string;
  category?: string;
  sizeBytes: number;
  seeders: number;
  leechers: number;
  snatched: number;
  uploadedAt?: number;
  tags?: string[];
  isFree: boolean;
  discountLevel?: string;
  discountEndTime?: number;
  hasHR?: boolean;
  sourceSite: string;
}

export interface SiteSearchParams {
  [key: string]: string | string[];
}

export interface MultiSiteSearchRequest {
  keyword: string;
  sites: string[];
  page?: number;
  pageSize?: number;
  sortBy?: "sourceSite" | "publishTime" | "size" | "seeders" | "leechers" | "snatched";
  orderDesc?: boolean;
  siteParams?: Record<string, SiteSearchParams>;
  timeoutSecs?: number; // Timeout in seconds for the search
}

export interface SearchErrorItem {
  site: string;
  error: string;
}

export interface MultiSiteSearchResponse {
  items: SearchTorrentItem[];
  totalResults: number;
  siteResults: Record<string, number>;
  errors?: SearchErrorItem[];
  durationMs: number;
}

export interface SearchCacheStats {
  totalEntries: number;
  totalSize: number;
  hitCount: number;
  missCount: number;
}

// 搜索 API
export const searchApi = {
  multiSite: (req: MultiSiteSearchRequest) =>
    api.post<MultiSiteSearchResponse>("/api/v2/search/multi", req),
  getSites: async () => {
    const resp = await api.get<{ sites: string[] }>("/api/v2/search/sites");
    return resp.sites || [];
  },
  clearCache: () => api.post<{ status: string }>("/api/v2/search/cache/clear"),
  getCacheStats: () => api.get<SearchCacheStats>("/api/v2/search/cache/stats"),
};

// ============== 种子推送相关 ==============

export interface TorrentPushRequest {
  downloadUrl?: string;
  magnetLink?: string;
  downloaderIds: number[];
  savePath?: string;
  category?: string;
  tags?: string;
  autoStart?: boolean;
  torrentTitle?: string;
  sourceSite?: string;
  sizeBytes?: number;
}

export interface TorrentPushResultItem {
  downloaderId: number;
  downloaderName: string;
  success: boolean;
  skipped?: boolean; // 种子已存在时跳过
  message?: string;
  torrentHash?: string;
}

export interface TorrentPushResponse {
  success: boolean;
  results: TorrentPushResultItem[];
  message?: string;
}

export interface TorrentPushItem {
  downloadUrl?: string;
  magnetLink?: string;
  torrentTitle?: string;
  sourceSite?: string;
  sizeBytes?: number;
}

export interface BatchTorrentPushRequest {
  torrents: TorrentPushItem[];
  downloaderIds: number[];
  savePath?: string;
  category?: string;
  tags?: string;
  autoStart?: boolean;
}

export interface BatchTorrentPushResultItem {
  torrentTitle: string;
  sourceSite: string;
  success: boolean;
  skipped?: boolean; // 所有下载器都跳过时为 true
  message?: string;
  results?: TorrentPushResultItem[];
}

export interface BatchTorrentPushResponse {
  success: boolean;
  totalCount: number;
  successCount: number;
  skippedCount: number; // 跳过的数量（种子已存在）
  failedCount: number;
  results: BatchTorrentPushResultItem[];
}

// 种子推送 API
export const torrentPushApi = {
  push: (req: TorrentPushRequest) => api.post<TorrentPushResponse>("/api/v2/torrents/push", req),
  batchPush: (req: BatchTorrentPushRequest) =>
    api.post<BatchTorrentPushResponse>("/api/v2/torrents/batch-push", req),
};

// ============== 暂停种子管理相关 ==============

export interface PausedTorrent {
  id: number;
  site_name: string;
  title: string;
  torrent_hash?: string;
  progress: number;
  torrent_size: number;
  downloader_name: string;
  downloader_task_id: string;
  paused_at?: string;
  pause_reason: string;
  free_end_time?: string;
  created_at: string;
}

export interface PausedTorrentsResponse {
  items: PausedTorrent[];
  total: number;
  page: number;
  page_size: number;
}

export interface DeletePausedRequest {
  ids?: number[];
  remove_data: boolean;
}

export interface DeletePausedResponse {
  success: number;
  failed: number;
  failed_ids?: number[];
  failed_errors?: string[];
}

export interface ResumeTorrentResponse {
  success: boolean;
  message?: string;
}

export interface ArchiveTorrent {
  id: number;
  original_id: number;
  site_name: string;
  title: string;
  torrent_hash?: string;
  is_free: boolean;
  free_end_time?: string;
  is_completed: boolean;
  progress: number;
  is_paused_by_system: boolean;
  pause_reason?: string;
  downloader_name?: string;
  original_created_at: string;
  archived_at: string;
}

export interface ArchiveTorrentsResponse {
  items: ArchiveTorrent[];
  total: number;
  page: number;
  page_size: number;
}

export const pausedTorrentsApi = {
  list: (page = 1, pageSize = 50, site?: string) => {
    const params = new URLSearchParams();
    params.set("page", page.toString());
    params.set("page_size", pageSize.toString());
    if (site) params.set("site", site);
    return api.get<PausedTorrentsResponse>(`/api/torrents/paused?${params.toString()}`);
  },
  delete: (req: DeletePausedRequest) =>
    api.post<DeletePausedResponse>("/api/torrents/delete-paused", req),
  resume: (id: number) => api.post<ResumeTorrentResponse>(`/api/torrents/${id}/resume`),
  listArchive: (page = 1, pageSize = 50, site?: string) => {
    const params = new URLSearchParams();
    params.set("page", page.toString());
    params.set("page_size", pageSize.toString());
    if (site) params.set("site", site);
    return api.get<ArchiveTorrentsResponse>(`/api/torrents/archive?${params.toString()}`);
  },
};

// ============== 版本检查相关 ==============

export interface VersionInfo {
  version: string;
  build_time: string;
  commit_id: string;
}

export interface ReleaseAsset {
  name: string;
  download_url: string;
  size: number;
}

export interface ReleaseInfo {
  version: string;
  name: string;
  changelog: string;
  url: string;
  published_at: number;
  assets?: ReleaseAsset[];
}

export interface RuntimeEnvironment {
  is_docker: boolean;
  os: string;
  arch: string;
  executable: string;
  can_self_upgrade: boolean;
}

export interface UpgradeProgress {
  status: "idle" | "downloading" | "extracting" | "replacing" | "completed" | "failed";
  target_version?: string;
  progress: number;
  bytes_downloaded: number;
  total_bytes: number;
  error?: string;
  started_at?: number;
  completed_at?: number;
}

export interface RuntimeResponse {
  runtime: RuntimeEnvironment;
  upgrade_progress: UpgradeProgress;
}

export interface VersionCheckResult {
  current_version: string;
  has_update: boolean;
  new_releases?: ReleaseInfo[];
  changelog_url?: string;
  has_more_releases?: boolean;
  checked_at: number;
  error?: string;
}

export const versionApi = {
  getInfo: () => api.get<VersionInfo>("/api/version"),
  checkUpdate: (options?: { force?: boolean; proxy?: string }) => {
    const params = new URLSearchParams();
    if (options?.force) params.set("force", "true");
    if (options?.proxy) params.set("proxy", options.proxy);
    const query = params.toString();
    return api.get<VersionCheckResult>(`/api/version/check${query ? `?${query}` : ""}`);
  },
  getRuntime: () => api.get<RuntimeResponse>("/api/version/runtime"),

  getUpgradeProgress: () => api.get<UpgradeProgress>("/api/version/upgrade"),

  startUpgrade: (version: string, proxyUrl?: string) =>
    api.post<{ success: boolean; message: string }>("/api/version/upgrade", {
      version,
      proxy_url: proxyUrl,
    }),

  cancelUpgrade: () => api.delete<{ success: boolean; message: string }>("/api/version/upgrade"),
};
