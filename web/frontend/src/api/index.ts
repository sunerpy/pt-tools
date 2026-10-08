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

export { ApiError };

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
  /** options 用于给轮询类请求挂 AbortSignal：拿不到就主动放弃，别一直占着浏览器的连接槽 */
  get: <T>(path: string, options?: ApiOptions) => request<T>(path, options),
  post: <T>(path: string, data?: unknown) =>
    request<T>(path, {
      method: "POST",
      body: data ? JSON.stringify(data) : undefined,
    }),
  put: <T>(path: string, data?: unknown) =>
    request<T>(path, {
      method: "PUT",
      body: data ? JSON.stringify(data) : undefined,
    }),
  delete: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};

// API 类型定义
export type FilterMode = "" | "auto_free" | "filter_only" | "free_only";

export interface GlobalSettings {
  default_interval_minutes: number;
  download_dir: string;
  download_limit_enabled: boolean;
  download_speed_limit: number;
  torrent_size_gb: number;
  torrent_min_size_gb: number;
  min_free_minutes: number;
  auto_start: boolean;
  retain_hours: number;
  max_retry: number;
  default_concurrency: number;
  default_enabled: boolean;
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
  free_end_advance_minutes?: number;
  peer_ratio_enabled?: boolean;
  peer_ratio_max_sl?: number;
  peer_ratio_interval_min?: number;
  peer_ratio_remove_data?: boolean;
  default_filter_mode?: FilterMode;
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
  filter_mode?: FilterMode; // 空字符串表示继承全局
  notify_mode?: "" | "all" | "filtered" | "both"; // RSS 上新通知模式
  notify_conf_ids?: string; // JSON 数组字符串，例如 "[1,2]"
  max_notifications_per_hour?: number; // 每小时最多通知数（0 = 不限制）
}

export interface SiteConfig {
  enabled: boolean;
  auth_method: string;
  cookie: string;
  has_cookie?: boolean;
  api_key: string;
  api_url: string;
  passkey?: string;
  upload_limit_kbs?: number;
  download_limit_kbs?: number;
  seeding_capacity_gb?: number;
  rss: RSSConfig[];
  urls?: string[];
  web_url?: string;
  unavailable?: boolean;
  unavailable_reason?: string;
  is_builtin?: boolean;
  api_last_login_at?: number;
  cookie_last_login_at?: number;
  last_access_at?: number;
  last_consistency_check?: number;
  probe_mode?: "auto" | "manual" | "disabled";
  ban_threshold_days?: number;
  remind_before_days?: number;
}

export interface TaskItem {
  id: number;
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
  downloaderId?: number | null; // 推送到的下载器
  downloaderName?: string;
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

export interface DeleteTasksResponse {
  success: number;
  failed: number;
  failed_ids: number[];
  failed_errors: string[];
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

export interface SupportedSiteDefinition {
  id: string;
  name: string;
  aka?: string[];
  description?: string;
  schema: string;
  urls: string[];
  faviconUrl?: string;
  authMethod?: string;
  hrEnabled: boolean;
  unavailable?: boolean;
  unavailableReason?: string;
}

export interface SiteLoginState {
  site_name: string;
  display_name?: string;
  base_url?: string;
  enabled: boolean;
  last_login_at?: number;
  last_access_at?: number;
  last_visit_at?: number;
  effective_last_active_at?: number;
  /** 判定活跃取自哪一列；探测失败时可能是浏览器扩展上报的访问（last_visit） */
  effective_source?:
    | "last_access"
    | "api_last_login"
    | "cookie_last_login"
    | "last_login"
    | "last_visit"
    | "none"
    | "unknown";
  last_probe_at?: number;
  /** 下一次定时探测的时间（只对自动模式有意义） */
  next_probe_at?: number;
  last_success_at?: number;
  /** 当前失败连续段的开始时间，探测成功后清空 */
  first_failure_at?: number;
  /** 探测成功但站点的最近访问时间超过 48 小时没有前进：自动访问对该站无效 */
  access_stale_since?: number;
  last_probe_status?: string;
  last_probe_error?: string;
  consecutive_probe_failures: number;
  ban_threshold_days: number;
  remind_before_days: number;
  reminder_cron: string;
  notification_channel_ids: number[];
  last_reminder_tier: string;
  last_reminder_sent_at?: number;
  days_remaining: number;
  tier: string;
  probe_mode: string;
}

/** 当天的签到状态：pending 之外都是最终结果；没有记录时为空串 */
export type AttendanceStatus = "" | "pending" | "signed" | "already" | "failed" | "unsupported";

/** GET /api/sites/attendance 的一项：能否签到、开关与当天的结果（时间为 Unix 秒） */
export interface SiteAttendance {
  site_name: string;
  site_enabled: boolean;
  attendance_enabled: boolean;
  supported: boolean;
  unsupported_reason?: string;
  day: string;
  status: AttendanceStatus;
  attempts: number;
  message?: string;
  last_error?: string;
  scheduled_at?: number;
  next_attempt_at?: number;
  last_attempt_at?: number;
}

/** 每日签到时间窗（HH:MM，服务端时区） */
export interface AttendanceSettings {
  window_start: string;
  window_end: string;
}

export const attendanceApi = {
  list: () => api.get<SiteAttendance[]>("/api/sites/attendance"),
  signNow: (name: string) => api.post<SiteAttendance>(`/api/sites/${name}/attendance`),
  setEnabled: (name: string, enabled: boolean) =>
    api.put<SiteAttendance>(`/api/sites/${name}/attendance`, { enabled }),
  getSettings: () => api.get<AttendanceSettings>("/api/sites/attendance/settings"),
  saveSettings: (data: AttendanceSettings) =>
    api.put<AttendanceSettings>("/api/sites/attendance/settings", data),
};

export const sitesApi = {
  list: (signal?: AbortSignal) => api.get<Record<string, SiteConfig>>("/api/sites", { signal }),
  listLoginStates: () => api.get<SiteLoginState[]>("/api/sites/login-state"),
  get: (name: string) => api.get<SiteConfig>(`/api/sites/${name}`),
  save: (name: string, data: SiteConfig) => api.post<void>(`/api/sites/${name}`, data),
  delete: (name: string) => api.delete<void>(`/api/sites?name=${encodeURIComponent(name)}`),
  deleteRss: (name: string, id: number) =>
    api.delete<void>(`/api/sites/${name}?id=${encodeURIComponent(id.toString())}`),
  listDefinitions: () => api.get<SupportedSiteDefinition[]>("/api/sites/definitions"),
  validate: (name: string, data: SiteConfig) =>
    api.post<void>(`/api/sites/validate?name=${encodeURIComponent(name)}`, data),
  updateProbeMode: (name: string, mode: "auto" | "manual" | "disabled") =>
    api.put<void>(`/api/sites/${name}/login-state/config`, { probe_mode: mode }),
  updateLoginConfig: (
    name: string,
    data: Partial<{
      ban_threshold_days: number;
      remind_before_days: number;
      reminder_cron: string;
      notification_channel_ids: number[];
      probe_mode: "auto" | "manual" | "disabled";
    }>,
  ) => api.put<void>(`/api/sites/${name}/login-state/config`, data),
  probeNow: (name: string) =>
    api.post<{
      ok: boolean;
      last_probe_at?: number;
      last_probe_status?: string;
      last_probe_error?: string;
    }>(`/api/sites/${name}/login-state/probe`),
  testReminder: (name: string) =>
    api.post<{ success: boolean; message: string }>(`/api/sites/${name}/login-state/test-reminder`),
};

/** 全库任务计数 —— 画板 10 的 KPI 格「活跃任务 / 今日推送 / 免费种子」 */
export interface TaskStatsResponse {
  total: number;
  active: number;
  pushedToday: number;
  free: number;
  /** 最近 7 天按天计数，供 KPI 柱图与任务页的吞吐卡使用 */
  daily: { date: string; created: number; pushed: number; free: number }[];
}

export const tasksApi = {
  list: (params: URLSearchParams, signal?: AbortSignal) =>
    api.get<TaskListResponse>(`/api/tasks?${params.toString()}`, { signal }),
  stats: () => api.get<TaskStatsResponse>("/api/tasks/stats"),
  batchDelete: (ids: number[]) => api.post<DeleteTasksResponse>("/api/tasks/batch-delete", { ids }),
};

/** 日志目录清单 —— 画板 29 左栏的「文件清单 / 归档」 */
export interface LogFilesResponse {
  dir: string;
  files: { name: string; size: number; mod_time: number; rotated: boolean; is_active: boolean }[];
  max_age: number;
  max_backups: number;
}

export const logsApi = {
  get: () => api.get<LogsResponse>("/api/logs"),
  files: () => api.get<LogFilesResponse>("/api/logs/files"),
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
  /** 是否真的连接站点验证过凭证；目前只检查必填字段，恒为 false */
  verified?: boolean;
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
  min_size_gb?: number;
  max_size_gb?: number;
  enabled: boolean;
  site_id?: number;
  rss_id?: number;
  priority: number;
  purpose?: "download" | "notify" | "both";
  created_at?: string;
  updated_at?: string;
}

export interface FilterRuleTestRequest {
  pattern: string;
  pattern_type: string;
  match_field?: string;
  require_free?: boolean;
  min_size_gb?: number;
  max_size_gb?: number;
  test_size_gb?: number;
  test_is_free?: boolean | null;
  global_size?: number;
  filter_mode?: FilterMode;
  site_id?: number;
  rss_id?: number;
  limit?: number;
}

export interface FilterRuleTestMatch {
  title: string;
  tag: string;
  is_free: boolean;
  size_gb?: number;
  decision?: "downloaded" | "skipped";
  reason?: string;
  source?: "filter_rule" | "free_download" | "";
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
  /** 各规则真实命中过多少个种子（TorrentInfo.filter_rule_id 的分组计数） */
  hits: () => api.get<{ hits: Record<string, number> }>("/api/filter-rules/hits"),
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
/** 周期：今日、近 7 天（本周）、近 30 天（本月） */
export type UserInfoRange = "today" | "7d" | "30d";

/** 某站在区间内的增量；回退的那一项按 0 计、negative 为 true */
export interface SiteDelta {
  site: string;
  from: string;
  to: string;
  uploaded: number;
  downloaded: number;
  bonus: number;
  hasBaseline: boolean;
  negative: boolean;
}

export interface DeltaSummary {
  range: UserInfoRange;
  from: string;
  to: string;
  sites: SiteDelta[];
  totalUploaded: number;
  totalDownloaded: number;
  totalBonus: number;
}

/** 某站某天的数据与相对上一份快照的增量 */
export interface DailyPoint {
  date: string;
  uploaded: number;
  downloaded: number;
  bonus: number;
  seeding: number;
  deltaUploaded: number;
  deltaDownloaded: number;
  deltaBonus: number;
  /** 增量覆盖的天数：上一份快照不是前一天时大于 1；没有更早的快照时为 0 */
  spanDays: number;
  negative: boolean;
}

export interface UserInfoHistoryResponse {
  site: string;
  days: number;
  from: string;
  to: string;
  points: DailyPoint[];
}

export interface TrendPoint {
  date: string;
  uploaded: number;
  downloaded: number;
  bonus: number;
}

export interface UserInfoTrendsResponse {
  days: number;
  from: string;
  to: string;
  dates: string[];
  totals: TrendPoint[];
  sites: Record<string, TrendPoint[]>;
}

/** 每日战报设置 */
export interface DailyReportSettings {
  enabled: boolean;
  /** HH:MM，服务器时区 */
  time: string;
  channel_ids: number[];
}

export const userInfoApi = {
  getAggregated: () => api.get<AggregatedStatsResponse>("/api/v2/userinfo/aggregated"),
  getHistory: (site: string, days = 30) =>
    api.get<UserInfoHistoryResponse>(
      `/api/v2/userinfo/history?site=${encodeURIComponent(site)}&days=${days}`,
    ),
  getSummary: (range: UserInfoRange) =>
    api.get<DeltaSummary>(`/api/v2/userinfo/summary?range=${range}`),
  getTrends: (days = 8) => api.get<UserInfoTrendsResponse>(`/api/v2/userinfo/trends?days=${days}`),
  getDailyReport: () => api.get<DailyReportSettings>("/api/v2/userinfo/daily-report"),
  saveDailyReport: (data: DailyReportSettings) =>
    api.put<DailyReportSettings>("/api/v2/userinfo/daily-report", data),
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
  list: (page = 1, pageSize = 50, site?: string, signal?: AbortSignal) => {
    const params = new URLSearchParams();
    params.set("page", page.toString());
    params.set("page_size", pageSize.toString());
    if (site) params.set("site", site);
    return api.get<PausedTorrentsResponse>(`/api/torrents/paused?${params.toString()}`, { signal });
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
  prerelease?: boolean;
  prerelease_label?: string;
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
  checkUpdate: (options?: { force?: boolean; proxy?: string; includePrerelease?: boolean }) => {
    const params = new URLSearchParams();
    if (options?.force) params.set("force", "true");
    if (options?.proxy) params.set("proxy", options.proxy);
    if (options?.includePrerelease) params.set("include_prerelease", "true");
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

// ===================== Downloader Torrents (Hub) =====================

export interface TorrentActionTarget {
  downloader_id: number;
  task_id: string;
}

export interface DownloaderTorrentItem {
  downloader_id: number;
  downloader_name: string;
  downloader_type: string;
  task_id: string;
  title: string;
  info_hash: string;
  state: string;
  progress: number;
  size: number;
  upload_speed: number;
  download_speed: number;
  seeds: number;
  connections: number;
  ratio: number;
  added_at: number;
  completed_at: number;
  save_path: string;
  category: string;
  tags: string;
  eta: number;
}

/** 某台下载器没取到数据。非空即「部分失败」：列表里的数据是真的，但不完整 */
export interface DownloaderFailure {
  downloader_id: number;
  downloader_name: string;
  error: string;
}

export interface TorrentListResponse {
  items: DownloaderTorrentItem[];
  total: number;
  page: number;
  page_size: number;
  /** 后端逐台上报的失败，用来驱动 partial 态；全部成功时字段缺省 */
  failures?: DownloaderFailure[];
}

export interface TorrentFileInfo {
  index: number;
  name: string;
  size: number;
  progress: number;
  priority: number;
}

export interface TorrentTrackerInfo {
  url: string;
  status: number;
  seeds: number;
  peers: number;
  leeches: number;
  /** tracker 返回的失败原因。后端 TorrentDetailTracker 一直带着它，这里以前漏了声明 */
  message?: string;
}

export interface TorrentDetailResponse {
  torrent: DownloaderTorrentItem;
  files: TorrentFileInfo[];
  trackers: TorrentTrackerInfo[];
}

export interface DownloaderTransferStats {
  downloader_id: number;
  downloader_name: string;
  total_download_speed: number;
  total_upload_speed: number;
  total_downloaded: number;
  total_uploaded: number;
  free_space: number;
}

/**
 * transfer-stats 里每台下载器的明细（对应后端 `DownloaderTransferStatItem`）。
 *
 * **不要拿「在不在这个数组里」当连接态**：后端只要能从 manager 取到实例就会 append，
 * 取数失败时各字段留零值。实例可能是缓存来的，Transmission 的实现在普通 RPC 失败后
 * 也不清 healthy 标志 —— 一台断线的客户端照样会出现在数组里。
 * 连接态看 `reachable`（这一轮是否真的取到了状态或剩余空间），失败原因在 `error`。
 */
export interface DownloaderTransferStatItem {
  downloader_id: number;
  downloader_name: string;
  downloader_type: string;
  upload_speed: number;
  download_speed: number;
  uploaded: number;
  downloaded: number;
  session_uploaded: number;
  session_downloaded: number;
  free_space: number;
  /** 这一轮是否真的从这台取到了数据（状态或剩余空间任一成功） */
  reachable: boolean;
  /** 两个探测都失败时的原因；取到数据时不返回这个字段 */
  error?: string;
  /** 下载器自报的版本（画板 41 状态栏那格的第三段）。问不到就不返回这个字段 */
  client_version?: string;
}

export interface DownloaderCapability {
  downloader_id: number;
  downloader_name: string;
  supports_categories: boolean;
  supports_tags: boolean;
  can_pause: boolean;
  can_resume: boolean;
  can_delete: boolean;
  can_delete_with_data: boolean;
  can_set_location: boolean;
  can_recheck: boolean;
  can_add_torrent: boolean;
  can_export_torrent: boolean;
  can_edit_trackers: boolean;
  categories: string[];
  tags: string[];
}

export interface DownloaderMeta {
  downloader_id: number;
  downloader_name: string;
  downloader_type: string;
  version: string;
}

export const downloaderTorrentsApi = {
  list: (params: URLSearchParams) =>
    api.get<TorrentListResponse>(`/api/downloader-torrents?${params.toString()}`),

  detail: (downloaderId: number, taskId: string) =>
    api.get<TorrentDetailResponse>(
      `/api/downloader-torrents/${downloaderId}/${encodeURIComponent(taskId)}`,
    ),

  action: (downloaderId: number, taskIds: string[], action: string, deleteFiles?: boolean) =>
    api.post<{ success: boolean; message: string }>("/api/downloader-torrents/action", {
      downloader_id: downloaderId,
      task_ids: taskIds,
      action,
      delete_files: deleteFiles,
    }),

  transferStats: (signal?: AbortSignal) =>
    api.get<{
      total_upload_speed: number;
      total_download_speed: number;
      total_uploaded: number;
      total_downloaded: number;
      total_session_uploaded: number;
      total_session_downloaded: number;
      total_free_space: number;
      /* 后端一直在返回每台的明细，这里以前漏了声明 */
      downloaders: DownloaderTransferStatItem[];
    }>("/api/downloader-torrents/transfer-stats", { signal }),

  capabilities: () =>
    api.get<{ items: DownloaderCapability[] }>("/api/downloader-torrents/capabilities"),

  meta: () =>
    api.get<{ downloaders: DownloaderMeta[]; categories: string[]; tags: string[] }>(
      "/api/downloader-torrents/meta",
    ),

  batchAction: (payload: {
    action: string;
    targets: TorrentActionTarget[];
    save_path?: string;
    delete_files?: boolean;
  }) =>
    api.post<{ success_count: number; failed_count: number }>(
      "/api/downloader-torrents/batch-action",
      payload,
    ),

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  add: (payload: Record<string, any>) =>
    api.post<{ success_count: number; failed_count: number }>(
      "/api/downloader-torrents/add",
      payload,
    ),
};

// ChatOps API
export interface NotificationConfig {
  id: number;
  channel_type: string;
  name: string;
  enabled: boolean;
  quiet_hours_start?: string;
  quiet_hours_end?: string;
  /**
   * 进程内实时运行态，不落库，重启即重算。后端问不到这个信号时缺省（不是 "error"）。
   *
   * 四档而不是三档：`connected` 只给「适配器确认对端真的接上了」的通道（目前只有 QQ
   * 能给出这个判断）；其余在跑的是 `running` —— 四个适配器的 Healthy() 都只代表
   * 构造/启动成功，拿它写「已连接」是在界面上说假话。
   */
  runtime_state?: "connected" | "running" | "error" | "disabled";
  // Dynamic fields (frontend form shape; backend stores under encrypted config_json)
  bot_token?: string;
  allowed_users?: string;
  admin_users?: string;
  default_chat_id?: string;
  listen_addr?: string;
  access_token?: string;
  admin_qq_users?: string;
  allowed_qq_users?: string;
  endpoint_url?: string;
  hmac_secret?: string;
  headers?: string;
  webhook_key?: string;
  proxy_url?: string;
  // 只出站的通道（Bark、Server 酱、ntfy、钉钉、飞书），字段见 utils/notifyChannels
  server_url?: string;
  device_key?: string;
  group?: string;
  sound?: string;
  send_key?: string;
  topic?: string;
  token?: string;
  username?: string;
  password?: string;
  priority?: number;
  webhook_url?: string;
  secret?: string;
  msg_type?: string;
  allow_private?: boolean;
}

const NOTIFICATION_BASE_FIELDS = new Set<keyof NotificationConfig>([
  "id",
  "channel_type",
  "name",
  "enabled",
  "quiet_hours_start",
  "quiet_hours_end",
  "runtime_state",
]);

const NOTIFICATION_DYNAMIC_FIELDS = [
  "bot_token",
  "allowed_users",
  "admin_users",
  "default_chat_id",
  "listen_addr",
  "access_token",
  "admin_qq_users",
  "allowed_qq_users",
  "endpoint_url",
  "hmac_secret",
  "headers",
  "webhook_key",
  "proxy_url",
  "server_url",
  "device_key",
  "group",
  "sound",
  "send_key",
  "topic",
  "token",
  "username",
  "password",
  "priority",
  "webhook_url",
  "secret",
  "msg_type",
  "allow_private",
] as const;

interface NotificationWireBody {
  channel_type?: string;
  name?: string;
  enabled?: boolean;
  quiet_hours_start?: string;
  quiet_hours_end?: string;
  config_json?: Record<string, unknown>;
}

function packNotificationBody(
  data: Partial<NotificationConfig>,
  includeEmpty: boolean,
): NotificationWireBody {
  const body: NotificationWireBody = {};
  if (data.channel_type !== undefined) body.channel_type = data.channel_type;
  if (data.name !== undefined) body.name = data.name;
  if (data.enabled !== undefined) body.enabled = data.enabled;
  if (data.quiet_hours_start !== undefined) body.quiet_hours_start = data.quiet_hours_start;
  if (data.quiet_hours_end !== undefined) body.quiet_hours_end = data.quiet_hours_end;
  const config: Record<string, unknown> = {};
  let hasDynamic = false;
  for (const key of NOTIFICATION_DYNAMIC_FIELDS) {
    const v = data[key];
    if (v !== undefined) {
      config[key] = v;
      hasDynamic = true;
    }
  }
  if (hasDynamic || includeEmpty) {
    body.config_json = config;
  }
  return body;
}

function unpackNotificationResponse(
  raw: NotificationConfig & { config_json?: unknown },
): NotificationConfig {
  const result: NotificationConfig = {
    id: raw.id,
    channel_type: raw.channel_type,
    name: raw.name,
    enabled: raw.enabled,
    quiet_hours_start: raw.quiet_hours_start,
    quiet_hours_end: raw.quiet_hours_end,
    runtime_state: raw.runtime_state,
  };
  const sink = result as unknown as Record<string, unknown>;
  // Fields that may legitimately come back as arrays of user IDs (number[] / string[]).
  // Telegram admin_users / allowed_users are []int64 on the backend; QQ counterparts
  // are []int64 too (raw json.RawMessage tolerated). Preserve them so the form can
  // render array → comma-separated text via its own loadDetail transform.
  const arrayFields = new Set([
    "admin_qq_users",
    "allowed_qq_users",
    "admin_users",
    "allowed_users",
  ]);
  // Fields that may come back as numbers (default_chat_id can be int64 OR string; ntfy priority).
  const numericFields = new Set(["default_chat_id", "priority"]);
  // 只出站通道的开关（Bark、ntfy 的 allow_private）
  const booleanFields = new Set(["allow_private"]);
  for (const key of Object.keys(raw) as (keyof typeof raw)[]) {
    if (NOTIFICATION_BASE_FIELDS.has(key as keyof NotificationConfig)) continue;
    if (key === "config_json") continue;
    const v = raw[key];
    if (typeof v === "string") {
      sink[key as string] = v;
    } else if (Array.isArray(v) && arrayFields.has(key as string)) {
      sink[key as string] = v;
    } else if (typeof v === "number" && numericFields.has(key as string)) {
      sink[key as string] = v;
    } else if (typeof v === "boolean" && booleanFields.has(key as string)) {
      sink[key as string] = v;
    }
  }
  if (raw.config_json && typeof raw.config_json === "object") {
    const cfg = raw.config_json as Record<string, unknown>;
    for (const key of NOTIFICATION_DYNAMIC_FIELDS) {
      const v = cfg[key];
      if (typeof v === "string") {
        sink[key] = v;
      } else if (Array.isArray(v) && arrayFields.has(key)) {
        sink[key] = v;
      } else if (typeof v === "number" && numericFields.has(key)) {
        sink[key] = v;
      } else if (typeof v === "boolean" && booleanFields.has(key)) {
        sink[key] = v;
      }
    }
  }
  return result;
}

export interface ChatOpBinding {
  id: number;
  channel_user_id: string;
  channel_type: string;
  label?: string;
  reply_lang?: string;
  admin?: boolean;
  allowed?: boolean;
  last_active?: string;
  bind_code?: string;
  code?: string;
  conf_id?: number;
  created_at?: string;
  expires_at?: string;
}

export interface AuditLog {
  id: number;
  command: string;
  channel_type: string;
  channel_user_id: string;
  result: string;
  created_at: string;
  latency_ms: number;
  args_json?: string;
}

export interface RSSNotificationLog {
  id: number;
  rss_id: number;
  site_name: string;
  torrent_id: string;
  notify_kind: "all" | "filtered";
  notification_conf_id: number;
  matched_filter_rule_id?: number;
  result: "sent" | "failed" | "suppressed" | "pending" | "throttled";
  attempts: number;
  next_retry_at?: string;
  last_error?: string;
  payload_json?: string;
  delivered_at?: string;
  created_at: string;
  updated_at: string;
}

export const chatopsApi = {
  notifications: {
    list: async () => {
      const raw = await api.get<(NotificationConfig & { config_json?: unknown })[]>(
        "/api/chatops/notifications",
      );
      return (raw || []).map(unpackNotificationResponse);
    },
    get: async (id: number) => {
      const raw = await api.get<NotificationConfig & { config_json?: unknown }>(
        `/api/chatops/notifications/${id}`,
      );
      return unpackNotificationResponse(raw);
    },
    create: async (data: Omit<NotificationConfig, "id">) => {
      const raw = await api.post<NotificationConfig & { config_json?: unknown }>(
        "/api/chatops/notifications",
        packNotificationBody(data, true),
      );
      return unpackNotificationResponse(raw);
    },
    update: (id: number, data: Partial<NotificationConfig>) =>
      request<{ status: string }>(`/api/chatops/notifications/${id}`, {
        method: "PUT",
        body: JSON.stringify(packNotificationBody(data, false)),
      }),
    delete: (id: number) => api.delete<void>(`/api/chatops/notifications/${id}`),
    test: (id: number) => api.post<{ success: boolean }>(`/api/chatops/notifications/${id}/test`),
  },
  bindings: {
    list: () =>
      api.get<{ bindings: ChatOpBinding[]; pending: ChatOpBinding[] }>("/api/chatops/bindings"),
    generateCode: (confId: number, label?: string, ttlSeconds?: number) =>
      api.post<{ code: string; expires_at?: string }>("/api/chatops/bindings/issue-code", {
        conf_id: confId,
        label: label ?? "",
        ttl_seconds: ttlSeconds ?? 300,
      }),
    update: (id: number, data: { reply_lang: string }) =>
      request<ChatOpBinding>(`/api/chatops/bindings/${id}`, {
        method: "PATCH",
        body: JSON.stringify(data),
      }),
    delete: (id: number) => api.delete<void>(`/api/chatops/bindings/${id}`),
  },
  audit: {
    list: (params: URLSearchParams) =>
      api.get<{
        items: AuditLog[];
        total: number;
        today_count: number;
        success_rate: number;
        max_latency_ms: number;
      }>(`/api/chatops/audit?${params.toString()}`),
    stats: () =>
      api.get<{
        today_count: number;
        total_count: number;
        success_rate: number;
        max_latency_ms: number;
        avg_latency_ms: number;
      }>("/api/chatops/audit/stats"),
  },
  rssNotifications: {
    list: (params: URLSearchParams) =>
      api.get<{
        items: RSSNotificationLog[];
        total: number;
        page: number;
        page_size: number;
      }>(`/api/chatops/rss-notifications?${params.toString()}`),
    retry: (id: number) =>
      api.post<{ success: boolean; queued: boolean }>(`/api/chatops/rss-notifications/${id}/retry`),
    cancel: (id: number) =>
      api.post<{ success: boolean }>(`/api/chatops/rss-notifications/${id}/cancel`),
  },
};

export interface CloakConfig {
  endpoint: string;
  has_token: boolean;
  /** 登录探测后备使用的 profile；与端点、token 三项齐全才启用后备 */
  profile_id: string;
  manager_version: string | null;
}

export type CloakTestCategory =
  | "success"
  | "dns_fail"
  | "conn_refused"
  | "timeout"
  | "auth_fail"
  | "not_found"
  | "server_error"
  | "protocol_error"
  | "unknown";

export interface CloakTestResult {
  category: CloakTestCategory;
  message: string;
  manager_version?: string;
}

export const cloakApi = {
  getConfig: () => api.get<CloakConfig>("/api/cloak/config"),
  updateConfig: (data: { endpoint: string; token?: string; profile_id?: string }) =>
    api.put<void>("/api/cloak/config", data),
  testConnection: (data?: { endpoint?: string; token?: string }) =>
    api.post<CloakTestResult>("/api/cloak/test", data ?? {}),
};

// ============== 工作目录清理相关 ==============

export interface CleanCategoryResult {
  name: string;
  deletedCount: number;
  freedBytes: number;
  freedHuman: string;
  dirUsedBytes: number;
  dirUsedHuman: string;
  skippedCount: number;
  note?: string;
}

export interface CleanResult {
  dryRun: boolean;
  categories: CleanCategoryResult[];
  totalDeleted: number;
  totalFreedBytes: number;
  totalFreedHuman: string;
}

export interface CleanRequest {
  categories?: string[];
  dryRun: boolean;
  keepBackups?: number;
}

// 工作目录清理 API
export const maintenanceApi = {
  // 预览可清理项（dryRun，不删除任何文件）
  preview: () => api.get<CleanResult>("/api/maintenance/clean"),
  // 执行清理（dryRun:false 才会实际删除）
  clean: (data: CleanRequest) => api.post<CleanResult>("/api/maintenance/clean", data),
};

// ---------------------------------------------------------------- 刷流（M3）

/** 刷流任务的配置（与 models.BrushTask 同名字段；体积单位 GB，时长单位见字段名） */
export interface BrushTaskConfig {
  name: string;
  enabled: boolean;
  site_name: string;
  downloader_id: number;
  save_path: string;
  category: string;
  /** 额外标签，逗号分隔；站点名、pt-tools-brush 与任务标签总会带上 */
  tags: string;
  interval_min: number;
  /** 允许的优惠类型，逗号分隔（FREE,2XFREE…）；空 = 只收免费 */
  discounts: string;
  min_free_remain_min: number;
  min_size_gb: number;
  max_size_gb: number;
  max_seeders: number;
  min_leechers: number;
  max_publish_age_min: number;
  exclude_hr: boolean;
  include_keywords: string;
  exclude_keywords: string;
  max_downloading: number;
  max_total_size_gb: number;
  max_daily_download_gb: number;
  remove_seed_time_h: number;
  remove_ratio: number;
  remove_low_speed_kbs: number;
  remove_low_speed_window_min: number;
  remove_inactive_h: number;
  remove_free_expired_incomplete: boolean;
  remove_with_data: boolean;
}

export interface BrushStat {
  uploaded: number;
  downloaded: number;
  added: number;
  removed: number;
}

/** GET /api/brush/tasks 的一项：配置 + 运行状态 + 收益 */
export interface BrushTask extends BrushTaskConfig {
  id: number;
  last_run_at?: string;
  last_error: string;
  last_result: string;
  created_at: string;
  updated_at: string;
  downloader_name: string;
  site_enabled: boolean;
  active_count: number;
  downloading_count: number;
  active_size_bytes: number;
  today: BrushStat;
  total: BrushStat;
}

export type BrushTorrentState = "active" | "removed" | "gone";

export interface BrushTorrent {
  id: number;
  task_id: number;
  info_hash: string;
  site_name: string;
  torrent_id: string;
  title: string;
  size_bytes: number;
  discount: string;
  free_end_at?: string;
  has_hr: boolean;
  state: BrushTorrentState;
  added_at: string;
  removed_at?: string;
  remove_reason: string;
  uploaded: number;
  downloaded: number;
  progress: number;
  ratio: number;
  seeding_time_sec: number;
  last_activity_at?: string;
}

export interface BrushRunResult {
  task_id: number;
  sampled: number;
  removed: number;
  gone: number;
  listed: number;
  eligible: number;
  added: number;
  skipped?: string[];
  errors?: string[];
  stopped?: string;
}

export interface BrushStatsPoint extends BrushStat {
  date: string;
}

export interface BrushStatsResponse {
  days: number;
  from: string;
  to: string;
  dates: string[];
  tasks: { task_id: number; name: string; series: BrushStatsPoint[] }[];
  totals: BrushStatsPoint[];
}

export const brushApi = {
  list: () => api.get<BrushTask[]>("/api/brush/tasks"),
  get: (id: number) => api.get<BrushTask>(`/api/brush/tasks/${id}`),
  create: (data: BrushTaskConfig) => api.post<BrushTask>("/api/brush/tasks", data),
  update: (id: number, data: BrushTaskConfig) => api.put<BrushTask>(`/api/brush/tasks/${id}`, data),
  remove: (id: number) => api.delete<{ success: boolean }>(`/api/brush/tasks/${id}`),
  run: (id: number) =>
    api.post<{ result: BrushRunResult; error?: string }>(`/api/brush/tasks/${id}/run`),
  torrents: (id: number, state: BrushTorrentState | "" = "", page = 1, pageSize = 50) =>
    api.get<{ items: BrushTorrent[]; total: number; page: number; page_size: number }>(
      `/api/brush/tasks/${id}/torrents?state=${state}&page=${page}&page_size=${pageSize}`,
    ),
  stats: (days = 30) => api.get<BrushStatsResponse>(`/api/brush/stats?days=${days}`),
};

/** 下载器助手：缺站点标签的种子（按 tracker 认出了站点） */
export interface AssistantSiteTag {
  hash: string;
  name: string;
  size: number;
  site: string;
  site_name: string;
  tracker_host: string;
  tags: string;
  category: string;
}

/** 下载器助手：要替换的 tracker 地址（已脱敏）。id 是这条「原地址 → 新地址」的指纹，执行时按它核对 */
export interface AssistantTrackerMatch {
  /** 这条「原地址 → 新地址」的指纹；替换后不是有效地址时为空，不能执行 */
  id: string;
  hash: string;
  name: string;
  old: string;
  new: string;
  /** 替换后不是有效地址的原因 */
  error?: string;
}

export type AssistantDeadReason = "unregistered" | "not_found";

/** 下载器助手：tracker 报告未注册或不存在的种子 */
export interface AssistantDeadTorrent {
  hash: string;
  name: string;
  size: number;
  progress: number;
  site?: string;
  site_name?: string;
  tracker_host: string;
  reason: AssistantDeadReason;
  message: string;
}

export interface AssistantItemError {
  hash: string;
  name?: string;
  error: string;
}

/** 下载器助手：一次执行的结果 */
export interface AssistantApplyResult {
  done: number;
  skipped: AssistantItemError[];
  failed: AssistantItemError[];
  /** 要删数据、但数据还被别的种子用着（如辅种），所以只删了种子的个数（算在 done 里） */
  kept_data?: number;
}

/** 扫描检查了多少种子：scanned 小于 total 时说明种子太多，只检查了一部分 */
export interface AssistantScanInfo {
  total: number;
  scanned: number;
}

/** 失效种子定时扫描的设置（只通知、不删种） */
export interface DeadTorrentScanSettings {
  enabled: boolean;
  interval_hours: number;
  channel_ids: number[];
}

export const downloaderAssistantApi = {
  siteTags: (downloaderId: number) =>
    api.get<{ items: AssistantSiteTag[] } & AssistantScanInfo>(
      `/api/downloader-assistant/site-tags?downloader_id=${downloaderId}`,
    ),
  applySiteTags: (downloaderId: number, items: { hash: string; site: string }[]) =>
    api.post<AssistantApplyResult>("/api/downloader-assistant/site-tags", {
      downloader_id: downloaderId,
      items,
    }),
  previewTrackers: (downloaderId: number, from: string, to: string) =>
    api.get<{ items: AssistantTrackerMatch[]; supported: boolean } & AssistantScanInfo>(
      `/api/downloader-assistant/trackers?downloader_id=${downloaderId}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
    ),
  applyTrackers: (
    downloaderId: number,
    from: string,
    to: string,
    selections: { hash: string; id: string }[],
  ) =>
    api.post<AssistantApplyResult>("/api/downloader-assistant/trackers", {
      downloader_id: downloaderId,
      from,
      to,
      selections,
    }),
  dead: (downloaderId: number) =>
    api.get<{ items: AssistantDeadTorrent[] } & AssistantScanInfo>(
      `/api/downloader-assistant/dead?downloader_id=${downloaderId}`,
    ),
  deleteDead: (downloaderId: number, hashes: string[], removeData: boolean) =>
    api.post<AssistantApplyResult>("/api/downloader-assistant/dead", {
      downloader_id: downloaderId,
      hashes,
      remove_data: removeData,
    }),
  getDeadScan: () => api.get<DeadTorrentScanSettings>("/api/downloader-assistant/dead-scan"),
  saveDeadScan: (data: DeadTorrentScanSettings) =>
    api.put<DeadTorrentScanSettings>("/api/downloader-assistant/dead-scan", data),
};

// ---------- 转移做种（路线图 M5） ----------

export type TransferState =
  | "pending"
  | "exported"
  | "adding"
  | "checking"
  | "verified"
  | "source_removed"
  | "rolled_back"
  | "failed"
  | "canceled";

export interface TransferItem {
  source_id: number;
  hash: string;
}

export interface TransferPreviewItem {
  source_id: number;
  source_name: string;
  hash: string;
  name: string;
  size: number;
  save_path: string;
  target_path: string;
  /** 路径按映射换算过；为 false 时目标用同一个路径 */
  mapped: boolean;
  site_name?: string;
  /** 种子文件从哪来：export 从源下载器导出、site 按 pt-tools 的记录从站点重新下载 */
  source?: "export" | "site";
  ok: boolean;
  reason?: string;
}

export interface TransferJob {
  id: number;
  source_downloader_id: number;
  target_downloader_id: number;
  source_name: string;
  target_name: string;
  info_hash: string;
  name: string;
  total_size: number;
  site_name: string;
  source_save_path: string;
  target_save_path: string;
  state: TransferState;
  message: string;
  progress: number;
  rule_id?: number;
  final: boolean;
  created_at: string;
  updated_at: string;
  finished_at?: string;
}

export interface TransferPathMap {
  id?: number;
  source_downloader_id?: number;
  target_downloader_id?: number;
  source_prefix: string;
  target_prefix: string;
}

export interface TransferRuleConfig {
  name: string;
  enabled: boolean;
  source_downloader_id: number;
  target_downloader_id: number;
  category: string;
  tag: string;
  site_name: string;
  min_seeding_hours: number;
  max_per_run: number;
  interval_min: number;
}

export interface TransferRule extends TransferRuleConfig {
  id: number;
  source_name: string;
  target_name: string;
  last_run_at?: string;
  last_result: string;
}

export interface TransferRuleRunResult {
  result: { matched: number; created: number; skipped?: string[] };
  summary: string;
  error?: string;
}

export const transferApi = {
  preview: (targetId: number, items: TransferItem[]) =>
    api.post<{ items: TransferPreviewItem[] }>("/api/transfer/preview", {
      target_id: targetId,
      items,
    }),
  create: (targetId: number, items: TransferItem[]) =>
    api.post<{ created: TransferJob[]; skipped: TransferPreviewItem[] }>("/api/transfer/jobs", {
      target_id: targetId,
      items,
    }),
  jobs: (status: "" | "active" | "finished" = "") =>
    api.get<{ items: TransferJob[] }>(
      `/api/transfer/jobs${status ? `?status=${encodeURIComponent(status)}` : ""}`,
    ),
  cancel: (id: number) => api.post<{ success: boolean }>(`/api/transfer/jobs/${id}/cancel`, {}),
  clearFinished: () => api.delete<{ deleted: number }>("/api/transfer/jobs"),
  pathMaps: (sourceId: number, targetId: number) =>
    api.get<{ items: TransferPathMap[] }>(
      `/api/transfer/path-maps?source_id=${sourceId}&target_id=${targetId}`,
    ),
  savePathMaps: (sourceId: number, targetId: number, items: TransferPathMap[]) =>
    api.put<{ items: TransferPathMap[] }>("/api/transfer/path-maps", {
      source_id: sourceId,
      target_id: targetId,
      items: items.map((m) => ({ source_prefix: m.source_prefix, target_prefix: m.target_prefix })),
    }),
  rules: () => api.get<{ items: TransferRule[] }>("/api/transfer/rules"),
  createRule: (data: TransferRuleConfig) => api.post<TransferRule>("/api/transfer/rules", data),
  updateRule: (id: number, data: TransferRuleConfig) =>
    api.put<TransferRule>(`/api/transfer/rules/${id}`, data),
  deleteRule: (id: number) => api.delete<{ success: boolean }>(`/api/transfer/rules/${id}`),
  runRule: (id: number) => api.post<TransferRuleRunResult>(`/api/transfer/rules/${id}/run`, {}),
};

// ---------- IYUU 辅种（路线图 M6） ----------

export interface ReseedSettings {
  enabled: boolean;
  has_token: boolean;
  interval_hours: number;
  downloader_ids: number[];
  site_names: string[];
  max_per_site_per_day: number;
  last_run_at?: string;
  last_result: string;
  running: boolean;
}

export interface ReseedSettingsUpdate {
  enabled: boolean;
  /** 不传时保留原来的 token，空串清除 */
  token?: string;
  interval_hours: number;
  downloader_ids: number[];
  site_names: string[];
  max_per_site_per_day: number;
}

export interface ReseedSiteMapItem {
  sid: number;
  iyuu_site: string;
  nickname: string;
  host: string;
  site_name?: string;
  configured: boolean;
  selected: boolean;
}

export interface ReseedRecord {
  id: number;
  info_hash: string;
  site_name: string;
  torrent_id: string;
  source_hash: string;
  downloader_id: number;
  name: string;
  /** queued 时结果看任务；done / rolled_back / canceled 是任务记录清除后留下的结果 */
  state: "queued" | "failed" | "done" | "rolled_back" | "canceled";
  message: string;
  job_id?: number;
  job_state?: TransferState;
  job_message?: string;
  /** 记录指向的任务已经不在了，结果不知道 */
  job_missing?: boolean;
  created_at: string;
  /** 最后一次尝试的时间 */
  updated_at: string;
}

// ---------- CookieCloud 导入（路线图 M7） ----------

export interface CookieCloudSettings {
  server_url: string;
  uuid: string;
  /** 密码只写不读，只说有没有设置 */
  has_password: boolean;
  auto_sync: boolean;
  interval_hours: number;
  last_sync_at?: string;
  last_result: string;
}

export interface CookieCloudSettingsUpdate {
  server_url: string;
  uuid: string;
  /** 不传时保留原来的密码，空串清除 */
  password?: string;
  auto_sync: boolean;
  interval_hours: number;
}

/** CookieCloud 里有对这个站点的地址有效的 Cookie；不含 Cookie 的值 */
export interface CookieCloudPreviewItem {
  site: string;
  site_name: string;
  host: string;
  cookie_names: string[];
  /** 站点在 pt-tools 里已经启用 */
  enabled: boolean;
  /** 和 pt-tools 里现在的 Cookie 不同 */
  changed: boolean;
}

export interface CookieCloudPreview {
  items: CookieCloudPreviewItem[];
  /** CookieCloud 里一共有多少个域名 */
  domains: number;
}

export interface CookieCloudImportResult {
  imported: string[];
  unchanged: string[];
  missing: string[];
  failed: { site: string; error: string }[];
}

export const cookieCloudApi = {
  settings: () => api.get<CookieCloudSettings>("/api/cookiecloud/settings"),
  saveSettings: (data: CookieCloudSettingsUpdate) =>
    api.put<CookieCloudSettings>("/api/cookiecloud/settings", data),
  preview: () => api.post<CookieCloudPreview>("/api/cookiecloud/preview", {}),
  import: (sites: string[]) =>
    api.post<CookieCloudImportResult>("/api/cookiecloud/import", { sites }),
};

// 媒体识别（M9）：TMDB 的 API Key 与代理地址只写不读（代理地址里的密码换成 ***）
export interface MediaSettings {
  has_tmdb_key: boolean;
  language: string;
  proxy_url: string;
}

/** 保存设置：tmdb_api_key、proxy_url 不传时不改，传空串时清除 */
export interface MediaSettingsUpdate {
  tmdb_api_key?: string;
  language: string;
  proxy_url?: string;
}

/** 标题解析结果（internal/media/meta）：没解析出来的字段不出现 */
export interface MediaMeta {
  name_cn?: string;
  name_en?: string;
  year?: number;
  type?: "" | "movie" | "tv";
  season?: number;
  season_end?: number;
  episode?: number;
  episode_end?: number;
  total_episodes?: number;
  complete?: boolean;
  resolution?: string;
  source?: string;
  remux?: boolean;
  platform?: string;
  video_codec?: string;
  bit_depth?: number;
  fps?: number;
  hdr?: string[];
  audio?: string[];
  channels?: string;
  group?: string;
  edition?: string[];
  version?: string;
  three_d?: boolean;
  chinese_subs?: boolean;
  mandarin?: boolean;
  cantonese?: boolean;
}

export type MediaKind = "movie" | "tv";

/** TMDB 上的一个电影或剧集条目 */
export interface TMDBItem {
  id: number;
  media_type: MediaKind;
  title: string;
  original_title: string;
  year?: number;
  date?: string;
  overview?: string;
  poster_path?: string;
  popularity?: number;
  vote_average?: number;
  original_language?: string;
  imdb_id?: string;
  seasons?: number;
  backdrop_path?: string;
  /** TMDB 的类型编号（16 是动画）与名称 */
  genre_ids?: number[];
  genres?: string[];
  runtime?: number;
}

export interface MediaCandidate extends TMDBItem {
  score: number;
}

export interface MediaRecognizeResult {
  meta: MediaMeta;
  summary: string;
  rule_hits?: number[];
  match?: TMDBItem;
  /** 匹配从哪来：手动纠正、IMDb 编号、按名字搜索；none 是没有匹配 */
  source: "none" | "override" | "imdb" | "search";
  score?: number;
  candidates?: MediaCandidate[];
  override_id?: number;
  message?: string;
  /** TMDB 出错时的原因（解析结果照样有） */
  error?: string;
}

export interface MediaOverride {
  id: number;
  key: string;
  label: string;
  tmdb_id: number;
  media_type: MediaKind;
  title: string;
  created_at: string;
  updated_at: string;
}

export type MediaWordKind = "block" | "replace" | "offset";

export interface MediaWordRule {
  id: number;
  kind: MediaWordKind;
  pattern: string;
  replacement: string;
  offset: number;
  is_regex: boolean;
  enabled: boolean;
  note: string;
  created_at?: string;
  updated_at?: string;
}

export type MediaWordInput = Omit<MediaWordRule, "id" | "created_at" | "updated_at">;

export const mediaApi = {
  settings: () => api.get<MediaSettings>("/api/media/settings"),
  saveSettings: (data: MediaSettingsUpdate) => api.put<MediaSettings>("/api/media/settings", data),
  testTMDB: () => api.post<{ ok: boolean }>("/api/media/settings/test"),
  recognize: (data: { title: string; subtitle?: string; imdb_id?: string }) =>
    api.post<MediaRecognizeResult>("/api/media/recognize", data),
  overrides: () => api.get<MediaOverride[]>("/api/media/overrides"),
  setOverride: (data: {
    title: string;
    subtitle?: string;
    tmdb_id: number;
    media_type: MediaKind;
  }) => api.post<MediaOverride>("/api/media/overrides", data),
  deleteOverride: (id: number) => api.delete<{ ok: boolean }>(`/api/media/overrides/${id}`),
  words: () => api.get<MediaWordRule[]>("/api/media/words"),
  createWord: (data: MediaWordInput) => api.post<MediaWordRule>("/api/media/words", data),
  updateWord: (id: number, data: MediaWordInput) =>
    api.put<MediaWordRule>(`/api/media/words/${id}`, data),
  deleteWord: (id: number) => api.delete<{ ok: boolean }>(`/api/media/words/${id}`),
};

// 整理入库（M10）：媒体服务器的 Token 只写不读
export type MediaMode = "hardlink" | "copy" | "symlink" | "move";

export interface OrganizeSettings {
  auto_enabled: boolean;
  /** 打开自动整理的时间：只整理之后下载完成的种子 */
  auto_since?: string;
  scan_enabled: boolean;
  scan_interval_min: number;
  downloaders: number[];
  categories: string[];
  tags: string[];
  save_paths: string[];
  min_video_mb: number;
  notify_channels: number[];
  delete_links_on_remove: boolean;
}

export type OrganizeSettingsInput = Omit<OrganizeSettings, "auto_since">;

export interface MediaLibraryInput {
  name: string;
  kind: MediaKind;
  /** 只收动画（TMDB 类型里有「动画」的条目） */
  anime: boolean;
  path: string;
  /** 命名模板；空串用默认模板 */
  template: string;
  mode: MediaMode;
  scrape: boolean;
  scrape_overwrite: boolean;
  enabled: boolean;
}

export interface MediaLibrary extends MediaLibraryInput {
  id: number;
  created_at: string;
  updated_at: string;
  /** 实际用的模板与示例渲染的结果 */
  effective_template: string;
  preview: string;
}

export interface MediaCheckItem {
  name: string;
  ok: boolean;
  message?: string;
}

export interface MediaPathMapInput {
  downloader_id: number;
  downloader_prefix: string;
  local_prefix: string;
}

export interface MediaPathMap extends MediaPathMapInput {
  id: number;
  created_at: string;
  updated_at: string;
}

export type MediaServerKind = "emby" | "jellyfin" | "plex";
export type MediaRefreshMode = "path" | "library";

export interface MediaServerInput {
  name: string;
  kind: MediaServerKind;
  url: string;
  /** 不传时保留原来的 Token；新建时必须填 */
  token?: string;
  enabled: boolean;
  refresh_mode: MediaRefreshMode;
  local_prefix: string;
  server_prefix: string;
}

export interface MediaServer extends Omit<MediaServerInput, "token"> {
  id: number;
  has_token: boolean;
  last_error: string;
  last_refresh_at?: string;
  created_at: string;
  updated_at: string;
}

export interface MediaServerInfo {
  name: string;
  version: string;
  libraries?: number;
}

export interface OrganizeRequest {
  downloader_id: number;
  hash: string;
  media_type?: MediaKind;
  tmdb_id?: number;
  library_id?: number;
}

export type OrganizeItemStatus = "pending" | "done" | "exists" | "failed";

export interface OrganizePlanItem {
  rel: string;
  source: string;
  size: number;
  target?: string;
  season?: number;
  episode?: number;
  episode_end?: number;
  episode_title?: string;
  subtitles?: { source: string; target: string }[];
  status: OrganizeItemStatus;
  message?: string;
  history_id?: number;
}

export interface OrganizePlan {
  downloader_id: number;
  downloader_name: string;
  hash: string;
  task_id: string;
  name: string;
  save_path: string;
  local_path: string;
  mapped: boolean;
  match?: TMDBItem;
  /** manual 是手动指定的条目，history 是沿用之前整理这些文件时用的条目 */
  source?: MediaRecognizeResult["source"] | "manual" | "history";
  library?: MediaLibraryInput & { id: number };
  mode?: MediaMode;
  items: OrganizePlanItem[];
  skipped?: { file: { rel: string; path: string; size: number }; reason: string }[];
  /** 整个种子整理不了的原因 */
  problem?: string;
}

export interface OrganizeResult {
  plan?: OrganizePlan;
  created: number;
  done: number;
  skipped: number;
  failed: number;
  messages?: string[];
  /** 整理还在后台进行（复制大文件时），稍后在整理历史里看 */
  queued?: boolean;
}

export type MediaTransferStatus = "done" | "failed" | "skipped" | "removed";

export interface MediaHistoryItem {
  id: number;
  downloader_id: number;
  downloader_name: string;
  info_hash: string;
  task_id: string;
  torrent_name: string;
  library_id: number;
  library_name?: string;
  source_path: string;
  target_path: string;
  mode: MediaMode;
  media_type: "" | MediaKind;
  tmdb_id: number;
  title: string;
  year: number;
  season: number;
  episode: number;
  episode_end: number;
  size: number;
  status: MediaTransferStatus;
  message: string;
  attempts: number;
  next_retry_at?: string;
  trigger: "auto" | "scan" | "manual";
  subtitles: number;
  /** 记录里记着 pt-tools 放进库的文件（跳过、失败的记录也可能有），可以连文件删 */
  has_files: boolean;
  created_at: string;
  updated_at: string;
}

export interface MediaHistoryPage {
  items: MediaHistoryItem[];
  total: number;
}

export const organizeApi = {
  settings: () => api.get<OrganizeSettings>("/api/media/organize/settings"),
  saveSettings: (data: OrganizeSettingsInput) =>
    api.put<OrganizeSettings>("/api/media/organize/settings", data),
  preview: (data: OrganizeRequest) => api.post<OrganizePlan>("/api/media/organize/preview", data),
  organize: (data: OrganizeRequest) => api.post<OrganizeResult>("/api/media/organize", data),
  libraries: () => api.get<MediaLibrary[]>("/api/media/libraries"),
  createLibrary: (data: MediaLibraryInput) => api.post<MediaLibrary>("/api/media/libraries", data),
  updateLibrary: (id: number, data: MediaLibraryInput) =>
    api.put<MediaLibrary>(`/api/media/libraries/${id}`, data),
  deleteLibrary: (id: number) => api.delete<{ ok: boolean }>(`/api/media/libraries/${id}`),
  checkLibrary: (data: { path: string; mode: MediaMode }) =>
    api.post<MediaCheckItem[]>("/api/media/libraries/check", data),
  templatePreview: (data: { kind: MediaKind; template: string }) =>
    api.post<{ template: string; preview: string; error?: string }>(
      "/api/media/libraries/template-preview",
      data,
    ),
  pathMaps: () => api.get<MediaPathMap[]>("/api/media/path-maps"),
  createPathMap: (data: MediaPathMapInput) => api.post<MediaPathMap>("/api/media/path-maps", data),
  updatePathMap: (id: number, data: MediaPathMapInput) =>
    api.put<MediaPathMap>(`/api/media/path-maps/${id}`, data),
  deletePathMap: (id: number) => api.delete<{ ok: boolean }>(`/api/media/path-maps/${id}`),
  servers: () => api.get<MediaServer[]>("/api/media/servers"),
  createServer: (data: MediaServerInput) => api.post<MediaServer>("/api/media/servers", data),
  updateServer: (id: number, data: MediaServerInput) =>
    api.put<MediaServer>(`/api/media/servers/${id}`, data),
  deleteServer: (id: number) => api.delete<{ ok: boolean }>(`/api/media/servers/${id}`),
  testServer: (data: { id?: number; kind: MediaServerKind; url: string; token?: string }) =>
    api.post<MediaServerInfo>("/api/media/servers/test", data),
  history: (q: { status?: string; q?: string; limit?: number; offset?: number } = {}) => {
    const params = new URLSearchParams();
    if (q.status) params.set("status", q.status);
    if (q.q) params.set("q", q.q);
    if (q.limit) params.set("limit", String(q.limit));
    if (q.offset) params.set("offset", String(q.offset));
    const qs = params.toString();
    return api.get<MediaHistoryPage>(`/api/media/history${qs ? `?${qs}` : ""}`);
  },
  retry: (id: number) => api.post<OrganizeResult>(`/api/media/history/${id}/retry`),
  /** kept 是换掉或改过、没有删的文件的说明 */
  deleteHistory: (id: number, files = false) =>
    api.delete<{ ok: boolean; kept?: string[] }>(
      `/api/media/history/${id}${files ? "?files=1" : ""}`,
    ),
};

// ---- 订阅（路线图 M11） ----

export type SubscriptionStatus = "active" | "paused" | "pending" | "done";
export type SubscriptionSource = "manual" | "explore" | "douban" | "chatops";
/** 空串不限；prefer 优先；require 必须有；avoid 不要（只有 Remux 与 HDR 有） */
export type QualityPref = "" | "prefer" | "require" | "avoid";

export interface SubscribeSettings {
  enabled: boolean;
  search_interval_hours: number;
  search_skip_sites: string[];
  default_profile_id: number;
  default_downloader_id: number;
  notify_channels: number[];
  upgrade_old: "keep" | "delete";
}

export interface QualityProfile {
  id: number;
  name: string;
  resolutions: string[];
  sources: string[];
  codecs: string[];
  remux: QualityPref;
  hdr: QualityPref;
  chinese_subs: QualityPref;
  free: QualityPref;
  groups: string[];
  min_size_gb: number;
  max_size_gb: number;
  min_seeders: number;
  exclude_hr: boolean;
}

export type QualityProfileInput = Omit<QualityProfile, "id">;

export interface QualityProfileList {
  items: QualityProfile[];
  resolutions: string[];
  sources: string[];
  codecs: string[];
}

export interface SubscriptionProgress {
  total: number;
  aired: number;
  in_library: number;
  downloading: number;
  missing?: number[];
  episodes?: {
    number: number;
    name?: string;
    air_date?: string;
    state: "library" | "downloading" | "missing" | "upcoming";
  }[];
}

export interface Subscription {
  id: number;
  media_type: MediaKind;
  tmdb_id: number;
  season: number;
  title: string;
  original_title: string;
  year: number;
  imdb_id: string;
  poster_path: string;
  profile_id: number;
  sites: string[];
  downloader_id: number;
  category: string;
  tags: string;
  save_path: string;
  status: SubscriptionStatus;
  upgrade: boolean;
  source: SubscriptionSource;
  douban_id: string;
  total_episodes: number;
  last_search_at?: string;
  next_search_at?: string;
  message: string;
  created_at: string;
  updated_at: string;
  progress?: SubscriptionProgress;
  torrents: number;
}

export interface SubscriptionTorrent {
  id: number;
  subscription_id: number;
  site_name: string;
  torrent_id: string;
  info_hash: string;
  title: string;
  subtitle: string;
  score: number;
  episode: number;
  episode_end: number;
  complete: boolean;
  size_bytes: number;
  downloader_id: number;
  has_hr: boolean;
  adopted: boolean;
  status: "downloading" | "done" | "failed" | "replaced";
  message: string;
  created_at: string;
}

export interface SubscriptionDetail extends Subscription {
  torrent_list: SubscriptionTorrent[];
}

export interface SubscriptionInput {
  media_type?: MediaKind;
  tmdb_id?: number;
  season?: number;
  profile_id: number;
  sites: string[];
  downloader_id: number;
  category: string;
  tags: string;
  save_path: string;
  upgrade: boolean;
  pending?: boolean;
}

export interface DoubanSource {
  id: number;
  user_id: string;
  name: string;
  enabled: boolean;
  confirm: boolean;
  profile_id: number;
  last_fetch_at?: string;
  next_fetch_at?: string;
  failures: number;
  last_error: string;
  abnormal: boolean;
  subscribed?: number;
  unmatched?: number;
}

export type DoubanSourceInput = Pick<
  DoubanSource,
  "user_id" | "name" | "enabled" | "confirm" | "profile_id"
>;

export interface DoubanItem {
  id: number;
  source_id: number;
  douban_id: string;
  title: string;
  year: number;
  media_type: "" | MediaKind;
  tmdb_id: number;
  subscription_id: number;
  status: "subscribed" | "unmatched";
  created_at: string;
}

export interface ExploreItem extends TMDBItem {
  in_library: boolean;
  subscribed: boolean;
  subscription_id?: number;
}

export interface ExplorePage {
  items: ExploreItem[];
  page: number;
  total_pages: number;
}

export const subscribeApi = {
  settings: () => api.get<SubscribeSettings>("/api/media/subscribe/settings"),
  saveSettings: (data: SubscribeSettings) =>
    api.put<SubscribeSettings>("/api/media/subscribe/settings", data),
  profiles: () => api.get<QualityProfileList>("/api/media/quality-profiles"),
  createProfile: (data: QualityProfileInput) =>
    api.post<QualityProfile>("/api/media/quality-profiles", data),
  updateProfile: (id: number, data: QualityProfileInput) =>
    api.put<QualityProfile>(`/api/media/quality-profiles/${id}`, data),
  deleteProfile: (id: number) => api.delete<{ ok: boolean }>(`/api/media/quality-profiles/${id}`),
  list: (q: { status?: string; q?: string } = {}) => {
    const params = new URLSearchParams();
    if (q.status) params.set("status", q.status);
    if (q.q) params.set("q", q.q);
    const qs = params.toString();
    return api.get<Subscription[]>(`/api/media/subscriptions${qs ? `?${qs}` : ""}`);
  },
  get: (id: number) => api.get<SubscriptionDetail>(`/api/media/subscriptions/${id}`),
  create: (data: SubscriptionInput) => api.post<Subscription>("/api/media/subscriptions", data),
  update: (id: number, data: SubscriptionInput) =>
    api.put<Subscription>(`/api/media/subscriptions/${id}`, data),
  remove: (id: number) => api.delete<{ ok: boolean }>(`/api/media/subscriptions/${id}`),
  setStatus: (id: number, status: "active" | "paused") =>
    api.post<Subscription>(`/api/media/subscriptions/${id}/status`, { status }),
  search: (id: number) => api.post<{ message: string }>(`/api/media/subscriptions/${id}/search`),
  doubanSources: () => api.get<DoubanSource[]>("/api/media/douban-sources"),
  createDouban: (data: DoubanSourceInput) =>
    api.post<DoubanSource>("/api/media/douban-sources", data),
  updateDouban: (id: number, data: DoubanSourceInput) =>
    api.put<DoubanSource>(`/api/media/douban-sources/${id}`, data),
  deleteDouban: (id: number) => api.delete<{ ok: boolean }>(`/api/media/douban-sources/${id}`),
  fetchDouban: (id: number) =>
    api.post<{ created: number }>(`/api/media/douban-sources/${id}/fetch`),
  doubanItems: (id: number) => api.get<DoubanItem[]>(`/api/media/douban-sources/${id}/items`),
  explore: (q: {
    kind: MediaKind;
    list: "trending" | "popular" | "search";
    page?: number;
    q?: string;
  }) => {
    const params = new URLSearchParams({ kind: q.kind, list: q.list });
    if (q.page) params.set("page", String(q.page));
    if (q.q) params.set("q", q.q);
    return api.get<ExplorePage>(`/api/media/explore?${params.toString()}`);
  },
};

export const reseedApi = {
  settings: () => api.get<ReseedSettings>("/api/reseed/settings"),
  saveSettings: (data: ReseedSettingsUpdate) =>
    api.put<ReseedSettings>("/api/reseed/settings", data),
  sites: () => api.get<{ items: ReseedSiteMapItem[] }>("/api/reseed/sites"),
  run: () => api.post<{ started: boolean }>("/api/reseed/run", {}),
  records: () => api.get<{ items: ReseedRecord[] }>("/api/reseed/records"),
  jobs: (status: "" | "active" | "finished" = "") =>
    api.get<{ items: TransferJob[] }>(
      `/api/reseed/jobs${status ? `?status=${encodeURIComponent(status)}` : ""}`,
    ),
  clearFinished: () => api.delete<{ deleted: number }>("/api/reseed/jobs"),
};
