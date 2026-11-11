import { headersToText, parseHeadersText } from "@/utils/webhook";

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
  patch: <T>(path: string, data?: unknown) =>
    request<T>(path, {
      method: "PATCH",
      body: data ? JSON.stringify(data) : undefined,
    }),
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
  /** 空间低于下限时紧急清理（多挑种子删、推送遇到空间不足时提前开始一轮）；只在 cleanup_disk_protect 开着时起作用 */
  cleanup_emergency?: boolean;
  cleanup_min_disk_space_gb?: number;
  cleanup_protect_dl?: boolean;
  cleanup_protect_hr?: boolean;
  cleanup_min_retain_h?: number;
  cleanup_protect_tags?: string;
  free_end_advance_minutes?: number;
  /** 做种竞争度：自动删种的一条删除条件，S/L 超过 peer_ratio_max_sl 时删除（peer_ratio_remove_data）或只暂停 */
  peer_ratio_enabled?: boolean;
  peer_ratio_max_sl?: number;
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
  pause_on_free_end?: boolean; // 免费结束时处理没下完的种子（暂停，free_end_delete 为真时删除）
  free_end_delete?: boolean; // 免费结束时删除而不是暂停，只在 pause_on_free_end 为真时有意义
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
  /** 站点规则里没有 H&R 做种要求时用户填的（小时），0 = 不知道 */
  hr_seed_time_h?: number;
  /** 站点规则里已经写了 H&R 做种要求（这时不用填 hr_seed_time_h） */
  hr_rule_builtin?: boolean;
  /** H&R 上传量达标（#542）：下完以后上传量达到体积的几倍也算 H&R 达标，0 = 不用 */
  hr_upload_ratio?: number;
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
  /** 免费资格：not_free / free_until / free_permanent / unknown（结束时间缺失或解析失败） */
  freeValidity?: string;
  freeVerifiedAt?: string | null; // 最近一次读到资格的时间
  freeStatusError?: string; // 读取资格时的问题（不含原始响应）
  requireFree?: boolean; // 推送是否以免费为前提
  pauseOnFreeEnd?: boolean;
  pausePlannedAt?: string | null; // 计划暂停时间（结束时间减提前量）
  freeEndFailures?: number; // 到期暂停连续失败次数
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

/** 站点代理的模式：全部站点、只有选中的站点、除了选中的站点 */
export type SiteProxyMode = "all" | "include" | "exclude";

/** 站点多选里的一项（注册表里能发请求的站点）：id 是小写的站点 ID */
export interface SiteProxyOption {
  id: string;
  name: string;
  enabled: boolean;
}

/** 保存站点代理的参数：proxy_url 与读到的（密码换成 *** 的）地址相同表示不改，空串表示清除 */
export interface SiteProxyInput {
  enabled: boolean;
  proxy_url: string;
  mode: SiteProxyMode;
  sites: string[];
}

/** 读到的站点代理设置：proxy_url 里的密码换成了 ***；site_options 是站点多选的唯一来源 */
export interface SiteProxySettings extends SiteProxyInput {
  updated_at: string | null;
  site_options: SiteProxyOption[];
  /** 存着的代理地址解不开（加密密钥换过），要重新填 */
  proxy_unreadable?: boolean;
}

// API 方法
export const globalApi = {
  get: () => api.get<GlobalSettings>("/api/global"),
  save: (data: GlobalSettings) => api.post<void>("/api/global", data),
};

/** 全局设置里的「站点代理」：保存后立即生效，不用重启 */
export const siteProxyApi = {
  get: () => api.get<SiteProxySettings>("/api/site-proxy"),
  save: (data: SiteProxyInput) => api.put<SiteProxySettings>("/api/site-proxy", data),
};

/** 备份的原因：定时、升级前、手动（网页上点的）、命令行 */
export type BackupReason = "scheduled" | "pre-upgrade" | "manual" | "cli";

/** 备份设置（全局设置 → 备份）。dir 为空表示用默认目录 default_dir */
export interface BackupSettingsInput {
  enabled: boolean;
  /** 每天定时备份的时刻（HH:MM，服务器时区） */
  time: string;
  /** 定时备份保留几份（min_keep–max_keep） */
  keep: number;
  dir: string;
  include_secret: boolean;
}

export interface BackupSettings extends BackupSettingsInput {
  default_dir: string;
  min_keep: number;
  max_keep: number;
}

/** 备份目录里的一份备份；error 非空表示读不出它的说明（坏文件），只能删掉 */
export interface BackupItem {
  name: string;
  size: number;
  created_at: string | null;
  reason: BackupReason | "";
  reason_label: string;
  app_version: string;
  schema_version: number;
  has_secret: boolean;
  /** 带上的密钥从哪来：file（secret.key）或 env（PT_TOOLS_SECRET_KEY） */
  secret_source?: "file" | "env";
  error?: string;
}

/** 最近一次备份的结果：error 为空表示成功 */
export interface BackupResult {
  at: string;
  reason: BackupReason;
  reason_label: string;
  name: string;
  error?: string;
  last_ok_at: string | null;
}

/** 最近一次启动时恢复的结果：error 为空表示换上了；dir 是换下来的原文件放在哪 */
export interface RestoreResult {
  at: string;
  name: string;
  dir?: string;
  error?: string;
}

/**
 * 放好了、重启 pt-tools 以后生效的恢复。key_action：keep-env 仍用环境变量的密钥，
 * use-backup 换成备份里的密钥，keep-file 不动 secret.key（no_cipher 时备份里没有密文，不需要密钥）
 */
export interface PendingRestore {
  name: string;
  staged_at: string;
  created_at: string;
  reason: BackupReason | "";
  reason_label: string;
  app_version: string;
  schema_version: number;
  key_action: "keep-env" | "use-backup" | "keep-file";
  no_cipher: boolean;
}

/** 备份页：设置、最近一次备份与恢复、下一次定时备份、备份列表与待恢复 */
export interface BackupsView {
  settings: BackupSettings;
  /** 现在生效的备份目录 */
  dir: string;
  /** 密钥来自环境变量 PT_TOOLS_SECRET_KEY */
  env_key: boolean;
  /** 下一次定时备份大约什么时候；关着时为 null */
  next_at: string | null;
  last: BackupResult | null;
  restore: RestoreResult | null;
  pending: PendingRestore | null;
  backups: BackupItem[];
  /** 读不出备份目录的原因（设置照常返回） */
  list_error?: string;
}

/** 下载一份备份的地址（同源，带着网页会话） */
export function backupFileURL(name: string): string {
  return `/api/backups/file/${encodeURIComponent(name)}`;
}

/** 全局设置里的「备份」与备份文件：恢复只是放进待恢复，重启 pt-tools 以后才换上 */
export const backupApi = {
  get: () => api.get<BackupsView>("/api/backups"),
  saveSettings: (data: BackupSettingsInput) => api.put<BackupsView>("/api/backups/settings", data),
  /** 立即备份（手动备份不会被自动删掉） */
  create: () => api.post<BackupsView>("/api/backups"),
  remove: (name: string) => api.delete<BackupsView>(backupFileURL(name)),
  restore: (name: string) =>
    api.post<BackupsView>(`/api/backups/file/${encodeURIComponent(name)}/restore`),
  cancelRestore: () => api.delete<BackupsView>("/api/backups/pending"),
  /**
   * 上传一个备份文件并放进待恢复：请求体就是文件本身（最大 2 GiB）。
   * 用 XMLHttpRequest 而不是 fetch：fetch 拿不到上传进度，大文件在慢的线路上会像卡住了一样。
   */
  upload: (file: File, onProgress?: (ratio: number) => void) =>
    new Promise<BackupsView>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open("POST", `/api/backups/upload?name=${encodeURIComponent(file.name)}`);
      xhr.withCredentials = true;
      xhr.setRequestHeader("Content-Type", "application/octet-stream");
      if (onProgress) {
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable && e.total > 0) onProgress(e.loaded / e.total);
        };
      }
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            resolve(JSON.parse(xhr.responseText) as BackupsView);
          } catch {
            reject(new ApiError(xhr.status, "服务返回的内容读不懂"));
          }
          return;
        }
        reject(new ApiError(xhr.status, xhr.responseText.trim() || `HTTP ${xhr.status}`));
      };
      xhr.onerror = () => reject(new ApiError(0, "上传没有完成：连接断开了"));
      xhr.send(file);
    }),
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
  /** 签到要过的一关：hdsky_captcha、opencd_captcha 要识别验证码，baka_quiz 要答题（都要用 AI）；不用时没有 */
  challenge?: AttendanceChallenge;
  day: string;
  status: AttendanceStatus;
  attempts: number;
  message?: string;
  last_error?: string;
  scheduled_at?: number;
  next_attempt_at?: number;
  last_attempt_at?: number;
}

export type AttendanceChallenge = "hdsky_captcha" | "opencd_captcha" | "baka_quiz";

/** 每日签到设置：时间窗（HH:MM，服务端时区）与 AI 签到 */
export interface AttendanceSettings {
  window_start: string;
  window_end: string;
  /** 让 AI 过签到的验证码与答题 */
  ai_enabled?: boolean;
  /** 识别验证码用的模型（要能看图），空时用 AI 助手的模型 */
  vision_model?: string;
  /** 只读：AI 助手是否启用并配好、用的哪个模型 */
  assistant_ready?: boolean;
  assistant_model?: string;
  /** 每天所有站点签完后推一条签到结果（默认开） */
  notify_enabled?: boolean;
  /** 推到哪些通知通道；空表示所有启用的通道 */
  notify_channel_ids?: number[];
}

/** 「一键签到」的进度：running 为 false 且有 finished_at 时是上一轮的结果（时间为 Unix 秒） */
export interface AttendanceRunStatus {
  running: boolean;
  started_at?: number;
  finished_at?: number;
  done: number;
  total: number;
  error?: string;
}

/** 签到记录的一行：某站某天的结果 */
export interface AttendanceHistoryRow {
  site_name: string;
  day: string;
  status: AttendanceStatus;
  attempts: number;
  message?: string;
  last_error?: string;
  last_attempt_at?: number;
}

/** 最近几天的签到记录：days 从今天往前排 */
export interface AttendanceHistory {
  days: string[];
  rows: AttendanceHistoryRow[];
}

/** 签到用的方式：直接签到，或者借助 AI 识别验证码、答题 */
export type AttendanceMethod = "direct" | "ai_captcha" | "ai_quiz";

/** 一次签到尝试的结果：retry 是这次没成、之后还会再试；failed 是当天不再试（或者手动签到失败） */
export type AttendanceEventResult = "signed" | "already" | "retry" | "failed" | "unsupported";

/** 一次签到尝试（时间为 Unix 秒） */
export interface AttendanceEvent {
  id: number;
  trigger: "auto" | "manual";
  result: AttendanceEventResult;
  method: AttendanceMethod;
  ai_model?: string;
  ai_calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  duration_ms: number;
  message?: string;
  error?: string;
  started_at: number;
  finished_at: number;
}

/** 某天一个站点的签到：当天的结果加上每次尝试；AI 的调用与 token 是这一天的合计 */
export interface AttendanceDaySite {
  site_name: string;
  status: AttendanceStatus;
  attempts: number;
  message?: string;
  last_error?: string;
  last_attempt_at?: number;
  next_attempt_at?: number;
  method: AttendanceMethod;
  ai_model?: string;
  ai_calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  events: AttendanceEvent[];
}

/** 某天所有站点签到的合计 */
export interface AttendanceDaySummary {
  sites: number;
  signed: number;
  already: number;
  failed: number;
  pending: number;
  unsupported: number;
  /** 借助 AI 签到（调过模型）的站点数 */
  ai_sites: number;
  ai_calls: number;
  prompt_tokens: number;
  completion_tokens: number;
}

/** 签到详情：某天每个站点怎么签的 */
export interface AttendanceDay {
  day: string;
  today: string;
  summary: AttendanceDaySummary;
  sites: AttendanceDaySite[];
}

/** 「测试看图」的结果：现画一张验证码让模型认 */
export interface AttendanceVisionTest {
  model: string;
  expected: string;
  got: string;
  ok: boolean;
  /** 认错了几个字 */
  mismatch: number;
  latency_ms: number;
}

export const attendanceApi = {
  list: () => api.get<SiteAttendance[]>("/api/sites/attendance"),
  signNow: (name: string) => api.post<SiteAttendance>(`/api/sites/${name}/attendance`),
  setEnabled: (name: string, enabled: boolean) =>
    api.put<SiteAttendance>(`/api/sites/${name}/attendance`, { enabled }),
  getSettings: () => api.get<AttendanceSettings>("/api/sites/attendance/settings"),
  saveSettings: (data: AttendanceSettings) =>
    api.put<AttendanceSettings>("/api/sites/attendance/settings", data),
  /** 用现画的验证码试一下看图模型（为空时用 AI 助手的模型）认不认得出 */
  testVision: (visionModel: string) =>
    api.post<AttendanceVisionTest>("/api/sites/attendance/ai-test", { vision_model: visionModel }),
  /** 一键签到：后台给开启了自动签到、今天还没有结果的站点各签一次，马上返回进度 */
  runAll: () => api.post<AttendanceRunStatus>("/api/sites/attendance/run"),
  runStatus: () => api.get<AttendanceRunStatus>("/api/sites/attendance/run"),
  history: (days = 14) => api.get<AttendanceHistory>(`/api/sites/attendance/history?days=${days}`),
  /** 签到详情：某天（YYYY-MM-DD，不给时是今天）每个站点的结果与每次尝试 */
  events: (day?: string) =>
    api.get<AttendanceDay>(
      `/api/sites/attendance/events${day ? `?day=${encodeURIComponent(day)}` : ""}`,
    ),
};

/** 试一下 RSS 链接（只拉一次，不存）的结果 */
export interface RSSLinkTestResult {
  title: string;
  count: number;
  /** 最新的几条标题 */
  titles: string[];
}

export const sitesApi = {
  list: (signal?: AbortSignal) => api.get<Record<string, SiteConfig>>("/api/sites", { signal }),
  /** 添加、编辑 RSS 时先试一下链接：失败时错误文本说清楚是哪一步出的问题 */
  /** site 是这条 RSS 所属的站点：站点代理按它判定走不走代理 */
  testRssLink: (url: string, site?: string) =>
    api.post<RSSLinkTestResult>("/api/rss/test", site ? { url, site } : { url }),
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

/**
 * RSS 任务的状态（GET /api/control/status；POST /api/control/auto-start 也回它）。
 * 文案与提示条规则见 utils/rssTask.ts。
 */
export type RssTaskState =
  | "running"
  | "starting"
  | "paused"
  | "manual"
  | "not_ready"
  | "no_downloader"
  | "downloader_unhealthy"
  | "no_subscriptions"
  | "stopped";

export interface RssTaskStatus {
  state: RssTaskState;
  /** 正在运行的 RSS 任务数（每个订阅一个） */
  jobs: number;
  /** 已启用站点下可以运行的订阅数 */
  subscriptions: number;
  auto_start: boolean;
  /** 最早一个在运行的任务的启动时间；没有任务在运行时不给 */
  started_at?: string;
}

export const controlApi = {
  stop: () => api.post<void>("/api/control/stop"),
  start: () => api.post<void>("/api/control/start"),
  status: (signal?: AbortSignal) => api.get<RssTaskStatus>("/api/control/status", { signal }),
  /** 只切换「自动启动 RSS 任务」，其余全局设置不动 */
  setAutoStart: (enabled: boolean) =>
    api.post<RssTaskStatus>("/api/control/auto-start", { enabled }),
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
  /** RSS 试跑时这个种子的详情没取到：标题按 RSS 里的，免费状态不知道（按非免费算） */
  detail_missing?: boolean;
}

export interface FilterRuleTestResponse {
  match_count: number;
  total_count: number;
  matches: FilterRuleTestMatch[];
  /** 这次用的数据：history 是最近入库的种子记录，rss 是现拉的 RSS */
  source?: "history" | "rss";
  /** 数据源的说明：RSS 的名称与站点，或「最近入库的 N 条种子记录」 */
  source_name?: string;
  /** RSS 里详情没取到的条目数 */
  detail_failed?: number;
}

// 过滤规则 API
export const filterRulesApi = {
  list: () => api.get<FilterRule[]>("/api/filter-rules"),
  get: (id: number) => api.get<FilterRule>(`/api/filter-rules/${id}`),
  create: (data: Omit<FilterRule, "id" | "created_at" | "updated_at">) =>
    api.post<FilterRule>("/api/filter-rules", data),
  /**
   * 整条替换：仅免费、启用、大小、站点与 RSS 范围没给就按 false / 0 / 不限存，
   * 只有名称、模式、匹配范围、优先级与用途空着时保留原值。所以要发完整的规则。
   */
  update: (id: number, data: FilterRule) =>
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
/** 每日战报的统计范围：前一天整天，或当天截至发送时刻 */
export type DailyReportScope = "previous_day" | "today";

export interface DailyReportSettings {
  enabled: boolean;
  /** HH:MM，服务器时区 */
  time: string;
  channel_ids: number[];
  /** 旧版本的后端不回这一项，按前一天 */
  scope?: DailyReportScope;
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
  /** 标题与副标题解析出的信息（分辨率、来源、平台、编码、HDR、制作组、中字、季集），按它筛选 */
  meta?: MediaMeta;
  /** 对所有条目拉黑了（关键字搜索里点的「不要」）；id 用来移出黑名单 */
  block?: { id: number; reason?: string };
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
  /** 条目（按条目搜索的结果里推送时带上）：写进种子记录，整理入库按它整理 */
  mediaType?: MediaKind;
  tmdbId?: number;
  season?: number;
  /** 按条目建子目录（要带条目）：保存路径（空的是每台下载器的默认保存路径）下按订阅设置里的模板建一级子目录 */
  itemSubdir?: boolean;
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

/**
 * 一台下载器的做种情况（用户统计页的「下载器做种」卡）。体积是种子体积相加：辅种和原种子各算一次，
 * 和站点上报的做种体积口径一样。`reachable` 为假时这一台没读到种子列表，数都是 0，原因在 `error`。
 */
export interface DownloaderSeedingItem {
  downloader_id: number;
  downloader_name: string;
  downloader_type: string;
  reachable: boolean;
  error?: string;
  /** 状态是做种的种子 */
  seeding_count: number;
  seeding_size: number;
  /** 下载完了、没在做种的种子（暂停、停止、排队、校验中、出错） */
  idle_count: number;
  idle_size: number;
}

/** 各启用下载器的做种情况；合计只算读到了的那几台 */
export interface DownloaderSeedingResponse {
  total_seeding_count: number;
  total_seeding_size: number;
  downloaders: DownloaderSeedingItem[];
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
  /** 能一键加、去「保留」标签（pt-tools-keep）；老后端不返回这个字段，当作不能 */
  can_keep?: boolean;
  categories: string[];
  tags: string[];
}

/** 批量操作里一个种子的结果（对应后端 BatchTorrentActionResult）。 */
export interface BatchTorrentActionResult {
  downloader_id: number;
  downloader_name: string;
  task_id: string;
  success: boolean;
  message?: string;
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
      /** 最近的总速度采样（首页的速度柱，旧的在前）；老后端不返回 */
      speed_history?: SpeedSample[];
    }>("/api/downloader-torrents/transfer-stats", { signal }),

  /** 各启用下载器正在做种的种子数与体积（用户统计页） */
  seedingStats: () => api.get<DownloaderSeedingResponse>("/api/downloader-torrents/seeding-stats"),

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
    api.post<{
      success_count: number;
      failed_count: number;
      results?: BatchTorrentActionResult[];
    }>("/api/downloader-torrents/batch-action", payload),

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
  /** Webhook 的自定义请求头：表单里是 JSON 文本，提交时转成对象（utils/webhook） */
  headers?: string;
  /** Webhook 的请求方法、请求体格式与请求体模板（M21） */
  method?: string;
  content_type?: string;
  body_template?: string;
  /** Webhook 按响应判断成败的模板（可选）：结果是 true 才算送到 */
  success_template?: string;
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
  "method",
  "content_type",
  "body_template",
  "success_template",
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
  // 请求头在表单里是 JSON 文本，后端要对象；解析不了的原样发出，由后端回 400 写明原因
  if (typeof config.headers === "string") {
    const parsed = parseHeadersText(config.headers);
    if (parsed.ok) config.headers = parsed.value;
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
    if (key === "headers") {
      sink[key] = headersToText(v);
    } else if (typeof v === "string") {
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
      if (key === "headers") {
        // 对象写成 JSON 文本给编辑框；不然对象会被丢掉，保存时再把原来的请求头覆盖掉
        if (v !== undefined) sink[key] = headersToText(v);
      } else if (typeof v === "string") {
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

/** 绑定页「通道」一栏里的一个账号（internal/app/binding_channels.go 的 ChannelBindingUser） */
export interface ChatOpsChannelUser {
  user_id: string;
  /** 在通道名单里的身份：admin、allowed；不在名单里时为空数组 */
  roles: string[];
  bound: boolean;
  binding_id?: number;
  /** 最近一次和机器人的往来（操作审计里最近的一条命令） */
  last?: { command: string; result: string; at: string };
}

/** 绑定页「通道」一栏里的一个能收命令的通道（ChannelBindingView） */
export interface ChatOpsBindingChannel {
  conf_id: number;
  name: string;
  channel_type: string;
  enabled: boolean;
  runtime_state?: "connected" | "running" | "error" | "disabled";
  /** 没有指定收件人的通知（每日战报、签到…）发给谁；没有时看 notify_error */
  notify_target?: string;
  notify_via?: "default_chat_id" | "admin" | "allowed" | "binding";
  notify_error?: string;
  users: ChatOpsChannelUser[];
  config_error?: string;
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

/**
 * 一个通道最近几天的投递：RSS 通知与后台监控通知（保号、签到、战报……）合计，按来源分。
 * sent 是已发出：RSS 通知实时发不出去时转进重发队列也算，重发的结果在 requeued 里（按消息数，合并发送的一组算一条）。
 * pending 是还在等（安静时段挂起、等重试）；canceled 是在 RSS 通知日志里取消的、或被同一种子后来的通知取代的；
 * throttled 是超出 RSS 每小时上限没发的。
 */
export interface NotifyStatsChannel {
  conf_id: number;
  total: number;
  sent: number;
  failed: number;
  pending: number;
  canceled: number;
  throttled: number;
  requeued: { total: number; sent: number; pending: number; dead: number };
  sources: { source: string; total: number; sent: number }[];
}

/**
 * 「最近的通知」里的一条：RSS 通知与后台监控通知合在一起按时间倒序。title 是通知的内置标题（RSS 是种子名），
 * 投递以前存下的：配了消息模板时实际收到的标题按模板，几条 RSS 合并成一条发出时这里仍逐条列出；
 * 没存下内容的（超出每小时上限没发的）为空。result：sent 已发出、failed 失败、pending 等待发送、canceled 取消或被取代、
 * throttled 超出每小时上限。
 */
export interface RecentNotification {
  source: string;
  log_id: number;
  conf_id: number;
  title: string;
  site?: string;
  torrent_id?: string;
  kind: string;
  result: "sent" | "failed" | "pending" | "canceled" | "throttled";
  attempts: number;
  /** 最近一次投递失败的原因（遮掉凭证）；没失败过时没有 */
  error?: string;
  created_at: string;
}

export interface NotifyStatsView {
  since: string;
  days: number;
  channels: NotifyStatsChannel[];
}

/** 消息模板的一个变量（路线图 M21）。列表的每一项是对象，字段在 item 里。 */
export interface NotifyTemplateVar {
  key: string;
  type: "string" | "int" | "float" | "bool" | "list";
  desc: string;
  sample?: unknown;
  item?: NotifyTemplateVar[];
}

/** 事件的组：notify（通知）、reply（ChatOps 命令回复）、assistant（AI 助手回复）。 */
export interface NotifyTemplateGroup {
  key: string;
  name: string;
  desc: string;
}

/**
 * 一种事件：所在的组、说明、变量、内置标题与正文的示例，以及当前的模板（没有自定义时 title、text 为空）。
 * no_title 的事件（命令回复、助手回复）只有正文。
 */
export interface NotifyTemplateView {
  type: string;
  group: string;
  name: string;
  desc: string;
  no_title?: boolean;
  vars: NotifyTemplateVar[];
  sample_title: string;
  sample_text: string;
  title: string;
  text: string;
  customized: boolean;
  updated_at?: string;
}

export interface NotifyTemplateCatalog {
  groups: NotifyTemplateGroup[];
  common_vars: NotifyTemplateVar[];
  events: NotifyTemplateView[];
}

export interface NotifyTemplateInput {
  title: string;
  text: string;
}

/** Webhook 请求的预览输入（M21）：和通道配置的同名字段一样，type 是用哪种事件的示例数据 */
export interface WebhookPreviewInput {
  type: string;
  method: string;
  endpoint_url: string;
  content_type: string;
  headers: Record<string, string>;
  body_template: string;
  /** 成功条件与拿来试它的一份响应（状态码默认 200） */
  success_template?: string;
  sample_response?: string;
  sample_status?: number;
}

/** 按示例数据渲染出来的请求：只渲染、不发送，不含签名 */
export interface WebhookPreviewResult {
  method: string;
  url: string;
  content_type: string;
  body: string;
  /** 写了成功条件时：按示例响应算出的成败与失败原因 */
  success?: boolean;
  success_reason?: string;
}

export const chatopsApi = {
  templates: {
    list: () => api.get<NotifyTemplateCatalog>("/api/chatops/templates"),
    save: (type: string, data: NotifyTemplateInput) =>
      api.put<NotifyTemplateView>(`/api/chatops/templates/${encodeURIComponent(type)}`, data),
    reset: (type: string) =>
      api.delete<{ success: boolean }>(`/api/chatops/templates/${encodeURIComponent(type)}`),
    preview: (type: string, data: NotifyTemplateInput) =>
      api.post<NotifyTemplateInput>("/api/chatops/templates/preview", { type, ...data }),
  },
  webhook: {
    preview: (data: WebhookPreviewInput) =>
      api.post<WebhookPreviewResult>("/api/chatops/webhook/preview", data),
  },
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
    stats: () => api.get<NotifyStatsView>("/api/chatops/notifications/stats"),
    recent: (limit = 8) =>
      api.get<{ items: RecentNotification[] }>(`/api/chatops/notifications/recent?limit=${limit}`),
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
    /** 作废一个没用掉的绑定码（按待绑定列表里的编号） */
    revokeCode: (id: number) => api.delete<{ status: string }>(`/api/chatops/bindings/codes/${id}`),
    channels: () =>
      api.get<{ channels: ChatOpsBindingChannel[] }>("/api/chatops/bindings/channels"),
    /** 在网页上直接绑定通道名单里的账号（效果与绑定码相同） */
    bindDirect: (confId: number, channelUserId: string) =>
      api.post<ChatOpBinding>("/api/chatops/bindings", {
        conf_id: confId,
        channel_user_id: channelUserId,
      }),
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
// ---- 自动删种的预览与上次运行 ----

/** 一个要删（或要暂停）的种子与为什么 */
export interface CleanupItem {
  name: string;
  info_hash: string;
  size: number;
  seeding_h: number;
  ratio: number;
  upload_kbs: number;
  /** 对上了的删种条件（紧急清理多挑的写剩余空间不足）；受保护的种子写命中的保护规则 */
  reasons: string[];
  /** 数据还被别的种子用着：只删种子、保留数据 */
  keep_data?: boolean;
  /** delete（删除）或 pause（做种竞争度过高、设置成只暂停）；受保护的条目没有 */
  action?: "delete" | "pause";
}

/** 一台下载器一轮自动删种（或预览）的结果 */
export interface CleanupResult {
  downloader: string;
  /** 下载器里的种子数（管理范围以外的也算） */
  total?: number;
  managed: number;
  protected: number;
  candidates: number;
  /** 受保护、这一轮不删的种子与原因 */
  protected_items?: CleanupItem[] | null;
  /** 候选里刷流任务的种子：按刷流任务的规则删，自动删种的删除条件都不碰 */
  brush?: number;
  /** 剩余空间低于下限、按紧急清理多挑了种子 */
  emergency?: boolean;
  free_gb?: number;
  /** 旧版本里连不上的下载器的结果没有列表（null） */
  items: CleanupItem[] | null;
  notes?: string[];
  /** 真的删掉、因做种竞争度暂停的个数（预览时为 0） */
  deleted: number;
  paused?: number;
  error?: string;
}

export interface CleanupRun {
  at: string;
  results: CleanupResult[];
}

export interface CleanupStatus {
  enabled: boolean;
  interval_min: number;
  /** 最近一轮（重启以后没有） */
  last_run?: CleanupRun;
  /** 下一轮大约什么时候（没启用时没有） */
  next_run_at?: string;
}

/** 预览用的删种规则（自动删种页上的那几项，可以还没保存） */
export interface CleanupPreviewInput {
  cleanup_scope: string;
  cleanup_scope_tags: string;
  cleanup_remove_data: boolean;
  cleanup_condition_mode: string;
  cleanup_max_seed_time_h: number;
  cleanup_min_ratio: number;
  cleanup_max_inactive_h: number;
  cleanup_slow_seed_time_h: number;
  cleanup_slow_max_ratio: number;
  cleanup_del_free_expired: boolean;
  cleanup_disk_protect: boolean;
  cleanup_emergency: boolean;
  cleanup_min_disk_space_gb: number;
  cleanup_protect_dl: boolean;
  cleanup_protect_hr: boolean;
  cleanup_min_retain_h: number;
  cleanup_protect_tags: string;
  peer_ratio_enabled: boolean;
  peer_ratio_max_sl: number;
  peer_ratio_remove_data: boolean;
}

export const cleanupApi = {
  status: () => api.get<CleanupStatus>("/api/cleanup/status"),
  preview: (data: CleanupPreviewInput) =>
    api.post<{ results: CleanupResult[] }>("/api/cleanup/preview", data),
};

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
  /** 读哪个分区的列表，只对 M-Team：normal 综合区、adult 成人区、all 两个都读；空 = 综合区。别的站点为空 */
  section: string;
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
  /** 不符合入场条件的原因与个数（多的在前） */
  rejected?: { reason: string; count: number }[];
}

export interface BrushRunStep {
  kind: "list" | "reject" | "add" | "remove" | "gone" | "skip" | "stop" | "error" | "info";
  text: string;
}

/** 刷流任务的一轮运行（定时或「立即运行」）：每个任务保留最近 500 条、不超过 14 天 */
export interface BrushRun {
  id: number;
  task_id: number;
  trigger: "schedule" | "manual";
  started_at: string;
  finished_at?: string;
  /** running 还在跑；partial 跑完了但个别种子出错；interrupted 是这一轮没跑完 pt-tools 就退出了 */
  status: "running" | "ok" | "partial" | "failed" | "interrupted";
  summary: string;
  error: string;
  listed: number;
  eligible: number;
  added: number;
  removed: number;
  gone: number;
  steps: BrushRunStep[];
  steps_truncated: boolean;
}

export interface BrushRunPage {
  items: BrushRun[];
  total: number;
}

export interface BrushStatsPoint extends BrushStat {
  /** 这一段的第一天（YYYY-MM-DD） */
  date: string;
  /** 这一段的最后一天（自然的结束日：当前这一段可能晚于今天，收益只算到今天） */
  end?: string;
}

/** 收益统计的分段：按日、周（周一开始）、月、季度、年，主机时区 */
export type BrushStatsGranularity = "day" | "week" | "month" | "quarter" | "year";

export interface BrushStatsResponse {
  granularity: BrushStatsGranularity;
  /** 段数；最后一段是当前这一段 */
  periods: number;
  /** 窗口覆盖的天数 */
  days: number;
  from: string;
  to: string;
  /** 每一段的第一天 */
  dates: string[];
  tasks: { task_id: number; name: string; series: BrushStatsPoint[] }[];
  totals: BrushStatsPoint[];
}

/** 删除刷流任务时留下的种子：交给自动删种（handover，默认），或者保留（keep：加 pt-tools-keep） */
export type BrushDeleteTorrents = "handover" | "keep";

/** 删除任务时选了保留、保留失败的一个种子 */
export interface BrushKeepFailure {
  title: string;
  info_hash: string;
  downloader_id: number;
  downloader_name?: string;
  message: string;
}

/**
 * 删除刷流任务的结果。选了保留时 kept 是加上保留标签并交还给你的种子数，missing 是已经不在下载器里的；
 * 有种子保留失败时任务不删，接口回 502，正文也是这个形状（success 为 false，failed 列出那几个）。
 */
export interface BrushDeleteResult {
  success: boolean;
  error?: string;
  kept?: number;
  missing?: number;
  failed?: BrushKeepFailure[];
}

export const brushApi = {
  list: () => api.get<BrushTask[]>("/api/brush/tasks"),
  get: (id: number) => api.get<BrushTask>(`/api/brush/tasks/${id}`),
  create: (data: BrushTaskConfig) => api.post<BrushTask>("/api/brush/tasks", data),
  update: (id: number, data: BrushTaskConfig) => api.put<BrushTask>(`/api/brush/tasks/${id}`, data),
  /** 删除任务；torrents 不传时按接口的默认值（交给自动删种） */
  remove: (id: number, torrents?: BrushDeleteTorrents) =>
    api.delete<BrushDeleteResult>(
      `/api/brush/tasks/${id}${torrents ? `?torrents=${torrents}` : ""}`,
    ),
  run: (id: number) =>
    api.post<{ result: BrushRunResult; error?: string }>(`/api/brush/tasks/${id}/run`),
  /** 运行记录，新的在前 */
  runs: (id: number, q: { limit?: number; offset?: number } = {}) => {
    const params = new URLSearchParams();
    if (q.limit) params.set("limit", String(q.limit));
    if (q.offset) params.set("offset", String(q.offset));
    const qs = params.toString();
    return api.get<BrushRunPage>(`/api/brush/tasks/${id}/runs${qs ? `?${qs}` : ""}`);
  },
  torrents: (id: number, state: BrushTorrentState | "" = "", page = 1, pageSize = 50) =>
    api.get<{ items: BrushTorrent[]; total: number; page: number; page_size: number }>(
      `/api/brush/tasks/${id}/torrents?state=${state}&page=${page}&page_size=${pageSize}`,
    ),
  /** 最近 periods 段的收益；不传 periods 时按周期的默认段数（日 30、周 12、月 12、季度 8、年 5） */
  stats: (granularity: BrushStatsGranularity = "day", periods?: number) => {
    const params = new URLSearchParams({ granularity });
    if (periods) params.set("periods", String(periods));
    return api.get<BrushStatsResponse>(`/api/brush/stats?${params.toString()}`);
  },
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

/** 读回的定时扫描设置还带上一轮的时间与结果、下一轮大约什么时候（在内存里，重启以后从头算） */
export interface DeadTorrentScanView extends DeadTorrentScanSettings {
  last_scan_at?: string;
  last_result?: string;
  next_scan_at?: string;
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
  getDeadScan: () => api.get<DeadTorrentScanView>("/api/downloader-assistant/dead-scan"),
  saveDeadScan: (data: DeadTorrentScanSettings) =>
    api.put<DeadTorrentScanView>("/api/downloader-assistant/dead-scan", data),
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
  /** 站点范围：all 是全部已配置、IYUU 也支持的站点（以后新配置的也算），selected 是只算 site_names */
  site_mode: ReseedSiteMode;
  max_per_site_per_day: number;
  last_run_at?: string;
  last_result: string;
  /** 下一轮大约什么时候（开着、有 token 时才有） */
  next_run_at?: string;
  running: boolean;
  /** 正在跑（或暂停中）的那一轮 */
  current_run?: ReseedCurrentRun;
  /** 两次请求 IYUU 之间至少隔几秒（没设置过是默认的 3） */
  iyuu_min_interval_seconds: number;
  /** 每天最多请求 IYUU 几次，0 是不限 */
  iyuu_max_calls_per_day: number;
  /** 限流冷却到什么时候（冷却中才有） */
  iyuu_cooldown_until?: string;
  /** 最近一次限流 */
  iyuu_last_limit?: ReseedIYUULimit;
  /** 今天各接口的请求计数 */
  iyuu_today: ReseedIYUUCalls[];
  /** 每轮结果推送：关闭、每一轮、只在有变化时 */
  notify_mode: ReseedNotifyMode;
  /** 接收每轮结果的通知通道 */
  notify_channel_ids: number[];
  /** 每日战报里带不带今天的辅种 */
  daily_report: boolean;
}

export type ReseedNotifyMode = "off" | "always" | "changes";

export type ReseedIYUUEndpoint = "sites" | "report" | "query";

/** 一天里一个 IYUU 接口的请求计数（calls = ok + rate_limited + failed） */
export interface ReseedIYUUCalls {
  day: string;
  endpoint: ReseedIYUUEndpoint;
  calls: number;
  ok: number;
  rate_limited: number;
  failed: number;
  /** 查询的 info hash 数（只有 query 有） */
  hashes: number;
  last_at?: string;
}

/** 最近一次 IYUU 限流：retry_after 是 IYUU 给的秒数，reset_at、limit 是 X-RateLimit-Reset 与 X-RateLimit-Limit */
export interface ReseedIYUULimit {
  at: string;
  endpoint: ReseedIYUUEndpoint;
  message: string;
  retry_after: number;
  reset_at?: string;
  limit?: number;
  /** 冷却到什么时候 */
  until: string;
}

/** 一段时间里的辅种计数：尝试按开始的那一天，任务结果按完成的那一天，跳过与 IYUU 请求按天 */
export interface ReseedStatsCounts {
  /** 建成的辅种任务 */
  created: number;
  /** 下载或核对没通过 */
  failed: number;
  /** 站点上已经删了 */
  gone: number;
  /** 已有进行中的任务（下载、核对以后才发现，算跳过） */
  active: number;
  /** 校验通过、开始做种 */
  succeeded: number;
  rolled_back: number;
  job_failed: number;
  canceled: number;
  skip_present: number;
  skip_tried: number;
  skip_daily_limit: number;
  skip_not_selected: number;
  iyuu_calls: number;
  iyuu_rate_limited: number;
  iyuu_failed: number;
  iyuu_hashes: number;
}

export interface ReseedStatsPoint extends ReseedStatsCounts {
  /** 这一段的第一天与最后一天（自定义范围时裁到范围以内） */
  date: string;
  end: string;
}

/** 一个站点在范围内的合计；site_name 为空的是不在辅种范围的 IYUU 站点 */
export interface ReseedStatsSite extends ReseedStatsCounts {
  site_name: string;
}

export interface ReseedStatsIYUU {
  endpoint: ReseedIYUUEndpoint;
  calls: number;
  ok: number;
  rate_limited: number;
  failed: number;
  hashes: number;
}

export interface ReseedStats {
  granularity: "day" | "week" | "month" | "quarter" | "year";
  custom: boolean;
  from: string;
  to: string;
  series: ReseedStatsPoint[];
  totals: ReseedStatsCounts;
  sites: ReseedStatsSite[];
  iyuu: ReseedStatsIYUU[];
  /** 最近一次请求 IYUU 的时间（不限范围） */
  last_iyuu_at?: string;
  /** 最早有数据的那天 */
  first_day?: string;
}

/** 统计的范围：最近 periods 段，或者自定义的 from、to（YYYY-MM-DD，含两头） */
export interface ReseedStatsQuery {
  granularity: string;
  periods?: number;
  from?: string;
  to?: string;
}

export type ReseedRunTrigger = "schedule" | "manual" | "retry" | "continue";

export type ReseedRunStatus =
  | "running"
  | "paused"
  | "ok"
  | "partial"
  | "stopped"
  | "limited"
  | "failed"
  | "interrupted";

/** 正在跑的那一轮的实时进度 */
export interface ReseedCurrentRun {
  id: number;
  trigger: ReseedRunTrigger;
  continued_from?: number;
  status: "running" | "paused";
  started_at: string;
  paused_at?: string;
  hashes: number;
  batches: number;
  batches_done: number;
  candidates: number;
  created: number;
  failed: number;
  gone: number;
  skipped: number;
}

/** 一轮辅种（轮次列表的一行） */
export interface ReseedRun {
  id: number;
  trigger: ReseedRunTrigger;
  started_at: string;
  finished_at?: string;
  status: ReseedRunStatus;
  summary: string;
  error: string;
  downloaders: number;
  hashes: number;
  batches: number;
  batches_done: number;
  candidates: number;
  created: number;
  failed: number;
  gone: number;
  interrupted: number;
  skipped: number;
  skipped_detail?: Record<string, number>;
  iyuu_calls: number;
  continued_from?: number;
  paused_at?: string;
  paused_seconds: number;
  steps_truncated: boolean;
  /** 最近一个提前停下的轮次（重试的除外）可以接着往下查 */
  continuable?: boolean;
}

export interface ReseedRunStep {
  kind: "list" | "query" | "add" | "fail" | "gone" | "skip" | "stop" | "error" | "info";
  text: string;
}

export interface ReseedRunDetail extends ReseedRun {
  steps: ReseedRunStep[];
}

/** 一次辅种尝试（只追加：每站每天的名额、轮次里的尝试与统计都按它算） */
export interface ReseedAttempt {
  id: number;
  run_id?: number;
  started_at: string;
  finished_at?: string;
  site_name: string;
  info_hash: string;
  torrent_id: string;
  name: string;
  downloader_id: number;
  outcome: "running" | "created" | "failed" | "gone" | "active" | "interrupted";
  message: string;
  job_id?: number;
  job_state?: TransferState;
  job_message?: string;
  job_finished_at?: string;
  job_missing?: boolean;
  backfilled: boolean;
}

export interface ReseedPage<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export interface ReseedSettingsUpdate {
  enabled: boolean;
  /** 不传时保留原来的 token，空串清除 */
  token?: string;
  interval_hours: number;
  downloader_ids: number[];
  /** 不传时不动站点范围（站点在「站点」页用开关改） */
  site_names?: string[];
  max_per_site_per_day: number;
  /** 不传时保留原来的；0 是不隔 */
  iyuu_min_interval_seconds?: number;
  /** 0 是不限 */
  iyuu_max_calls_per_day: number;
  /** 以下三项不传时保留原来的；推送开着时至少要选一个通道 */
  notify_mode?: ReseedNotifyMode;
  notify_channel_ids?: number[];
  daily_report?: boolean;
}

export type ReseedSiteMode = "all" | "selected";

/** 站点开关的结果：all 是全部站点，selected 是只算 site_names */
export interface ReseedSiteSelection {
  site_mode: ReseedSiteMode;
  site_names: string[];
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

/** 记录在页面上的结果（服务端按它筛选）：记录自己的状态说了算的直接用，建了任务的看任务现在的状态 */
export type ReseedRecordResult =
  | "active"
  | "done"
  | "failed"
  | "rolled_back"
  | "canceled"
  | "gone"
  | "missing";

export interface ReseedRecord {
  id: number;
  info_hash: string;
  site_name: string;
  torrent_id: string;
  source_hash: string;
  downloader_id: number;
  name: string;
  /** queued 时结果看任务；done / rolled_back / canceled 是任务记录清除后留下的结果；gone 是站点上已经删了这个种子（不再尝试） */
  state: "queued" | "failed" | "gone" | "done" | "rolled_back" | "canceled";
  message: string;
  job_id?: number;
  job_state?: TransferState;
  job_message?: string;
  /** 记录指向的任务已经不在了，结果不知道 */
  job_missing?: boolean;
  result: ReseedRecordResult;
  created_at: string;
  /** 最后一次尝试的时间 */
  updated_at: string;
}

/** 记录列表的筛选（都可以不填）：q 匹配种子名或 info hash */
export interface ReseedRecordFilter {
  result?: ReseedRecordResult | "";
  site?: string;
  q?: string;
}

export interface ReseedRecordPage extends ReseedPage<ReseedRecord> {
  /** 记录里出现过的站点（筛选用） */
  sites: string[];
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
  /** 下一次定时同步大约什么时候（打开定时同步、填全了设置才有） */
  next_sync_at?: string;
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
  /** 自己填了 API Key */
  has_tmdb_key: boolean;
  /** 识别现在用哪个 Key：自己填的、内置的（没填时），或者没有能用的 */
  key_source: "user" | "builtin" | "none";
  /** 这个版本内置了 Key（发布的 Release 与 Docker 镜像有，从源码编译的没有） */
  builtin_key: boolean;
  language: string;
  proxy_url: string;
  /** 换掉的 TMDB 接口地址（镜像或自建中转）；空 = 官方地址 */
  api_base_url: string;
  /** 换掉的 TMDB 图片地址（到 /t/p/ 为止）；空 = 官方地址 */
  image_base_url: string;
  /** 刮削用的 Fanart.tv（项目的 Key）、Fanart.tv 个人 Key 与 OMDb 的 API Key 填了没有（只写不读） */
  has_fanart_key: boolean;
  has_fanart_client_key: boolean;
  has_omdb_key: boolean;
}

/** 保存设置：语言以外的项不传时不改，传空串时清除 */
export interface MediaSettingsUpdate {
  tmdb_api_key?: string;
  language: string;
  proxy_url?: string;
  api_base_url?: string;
  image_base_url?: string;
  /** Fanart.tv 项目的 API Key（填了才用 Fanart.tv） */
  fanart_api_key?: string;
  /** Fanart.tv 的个人 Key（可选，和项目的 Key 一起发，新上传的图片更快能用） */
  fanart_client_key?: string;
  omdb_api_key?: string;
}

/** 测试连接的来源：TMDB、Fanart.tv、OMDb */
export type MediaTestSource = "tmdb" | "fanart" | "omdb";

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
  /** 出品国家或地区（ISO 3166-1）：详情里有，剧集的搜索结果里也有 */
  countries?: string[];
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
  /** 用当前设置访问一次这个来源，确认 API Key 与代理可用 */
  testSource: (source: MediaTestSource) =>
    api.post<{ ok: boolean }>(`/api/media/settings/test?source=${source}`),
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
  /** 扫描监控目录的间隔（分钟） */
  watch_interval_min: number;
  /** 实时目录监控：监控目录里有改动时提前扫描那一项（默认开，按间隔扫描照旧兜底） */
  watch_realtime: boolean;
  /** 定期扫描与目录监控扫描上一次、下一次大约什么时候（在内存里，重启以后从头算；只在读设置时回） */
  runs?: OrganizeRunTimes;
}

export interface OrganizeRunTimes {
  scan_last?: string;
  scan_next?: string;
  watch_last?: string;
  watch_next?: string;
}

export type OrganizeSettingsInput = Omit<OrganizeSettings, "auto_since" | "runs">;

/** 刮削内容（和后端 scrape.AllItems 一致） */
export type MediaScrapeItem =
  | "nfo"
  | "poster"
  | "fanart"
  | "logo"
  | "clearart"
  | "banner"
  | "landscape"
  | "discart"
  | "season_poster"
  | "thumb";
/** 海报与 Logo 优先用的语言：空串用 TMDB 给的那张；none 是没有文字的 */
export type MediaImageLang = "" | "zh" | "en" | "ja" | "none";
/** 旧版本的「简介与评分」：空串用 TMDB 的；douban 用豆瓣的。新的设置看 scrape_sources */
export type MediaScrapeSource = "" | "douban";

/** 每一项的数据来源（和后端 metadata.Sources 一致）：空串是 TMDB，选的来源找不到时用 TMDB 的 */
export interface MediaScrapeSources {
  /** 简介：TMDB、豆瓣、Bangumi（只用于动画） */
  plot: "" | "douban" | "bangumi";
  /** 主要的评分：TMDB、豆瓣、Bangumi、IMDb（经 OMDb，要填它的 Key） */
  rating: "" | "douban" | "bangumi" | "omdb";
  /** 每一集的标题与简介（只有剧集）：TMDB、Bangumi */
  episode: "" | "bangumi";
  /** 海报、背景、Logo 与季海报先用哪个来源的：TMDB、Fanart.tv（要填它的 Key） */
  artwork: "" | "fanart";
}

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
  /** 刮削要写的内容；没选过的库是默认的那几项（Logo 以外） */
  scrape_items: MediaScrapeItem[];
  scrape_image_lang: MediaImageLang;
  /** 旧版本的「简介与评分」；带了 scrape_sources 时不看它 */
  scrape_source?: MediaScrapeSource;
  scrape_sources: MediaScrapeSources;
  /** 按二级分类放进库目录下的分类目录 */
  classify_dir: boolean;
  /** 目标位置已经有 pt-tools 之前放进库的同一部（同一集）的旧版本时：空串不替换，larger 新文件更大时替换 */
  overwrite_mode: MediaOverwriteMode;
  /** 刮削的演员名用 TMDB 上的中文译名（前 15 位，没有译名的用原名；要开着刮削） */
  scrape_actor_zh: boolean;
  /** 每周对刮削结果是部分成功或失败的记录补刮一次（只补没有的文件；要开着刮削） */
  scrape_fill: boolean;
  /** 整理出新视频以后到站点上取字幕（种子里没有字幕、种子是从 NexusPHP 站点推送的时） */
  site_subtitles: boolean;
  /** ChineseSubFinder 的地址；空串是不用（保存时一起清掉 Key） */
  csf_url: string;
  /** ChineseSubFinder 的 API Key：只写不读，不带或空串时保留原来的 */
  csf_key?: string;
  /** ChineseSubFinder 里看到的这个库的目录；空串是和库目录一样 */
  csf_path: string;
  /**
   * 新建订阅时带出的下载设置：订阅弹窗与手机的追剧编辑页在新建时按媒体库把它们填进表单（已有的订阅不跟着变）。
   * 下载器 0 是不指定；保存目录是下载器里看到的目录，不是库目录；标签逗号分开；都可以空着
   */
  download_downloader_id: number;
  download_save_path: string;
  download_category: string;
  download_tags: string;
  enabled: boolean;
}

export type MediaOverwriteMode = "" | "larger";

/** 一条二级分类：条件之间是「且」，同一个条件里的几个值是「或」，空的条件不限 */
export interface ClassifyRule {
  name: string;
  /** TMDB 的类型编号 */
  genres?: number[];
  /** 原始语言（ISO 639-1，例如 zh、ja；TMDB 把粤语记作 cn） */
  languages?: string[];
  /** 出品国家或地区（ISO 3166-1，例如 CN、US） */
  countries?: string[];
  /**
   * max_runtime 与 keywords 合成一个条件（两个都填时是「或」）：片长（分钟，剧集是一集）不超过 max_runtime，
   * 或者标题（种子名、站点标题）里有其中一个关键字。片长未知时片长这一半对不上
   */
  max_runtime?: number;
  keywords?: string[];
}

/** 这次放进哪个二级分类、为什么：rule 是对上的第几条（从 1 数），0 是都没对上；其余是对上的条件 */
export interface ClassifyNote {
  rule: number;
  name?: string;
  /** 这一类（电影或剧集）的规则数 */
  rules: number;
  genres?: number[];
  languages?: string[];
  countries?: string[];
  /** 对上「片长不超过」时条目的片长与规则里的上限（分钟） */
  runtime?: number;
  max_runtime?: number;
  /** 标题里对上的关键字 */
  keyword?: string;
}

/** 电影与剧集各自的二级分类，按顺序取第一条对得上的 */
export interface ClassifyRules {
  movie: ClassifyRule[];
  tv: ClassifyRule[];
}

export interface ClassifyView {
  rules: ClassifyRules;
  /** 常用的二级分类，「填入常用分类」用 */
  presets: ClassifyRules;
}

export interface MediaLibrary extends MediaLibraryInput {
  id: number;
  /** 上一次每周补刮的时间 */
  scrape_filled_at?: string;
  created_at: string;
  updated_at: string;
  /** 实际用的模板与示例渲染的结果 */
  effective_template: string;
  preview: string;
  /** 有没有保存 ChineseSubFinder 的 API Key */
  has_csf_key: boolean;
}

export interface MediaCheckItem {
  name: string;
  ok: boolean;
  message?: string;
}

/** 监控的目录：放进来的视频（不是下载器里的种子）按媒体库整理，目录下的每一项当作一个种子 */
export interface MediaWatchDirInput {
  path: string;
  enabled: boolean;
}

/** 监控目录的实时监控：on 在监听；off 关着（设置里关了、目录关着或后台没在跑）；unavailable 加不了监听，只按间隔扫描 */
export type MediaWatchRealtime = "on" | "off" | "unavailable";

export interface MediaWatchDir extends MediaWatchDirInput {
  id: number;
  created_at: string;
  updated_at: string;
  realtime?: MediaWatchRealtime;
  /** 实时监控不可用的原因 */
  realtime_note?: string;
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
  /* 下面几项不传时不改（新建时用默认值） */
  /** 打开链接用的地址（浏览器能打开的那个）；空串用上面的地址 */
  play_url?: string;
  /** 索引多久同步一次（小时，最多 168）；0 关掉索引。新建时 6 */
  index_hours?: number;
  /** 追剧时把这台服务器里有的集算作已入库 */
  count_library?: boolean;
  /** 继续观看用的用户（Emby、Jellyfin）；空串不取继续观看 */
  resume_user_id?: string;
  resume_user_name?: string;
  /** 播放通知发往的通道；空的不发 */
  playback_channels?: number[];
}

export interface MediaServer extends Required<Omit<MediaServerInput, "token">> {
  id: number;
  has_token: boolean;
  last_error: string;
  last_refresh_at?: string;
  /** 上一次同步成功的时间；index_error 是最近一次同步失败的原因（成功时清空） */
  indexed_at?: string;
  index_error: string;
  index_movies: number;
  index_series: number;
  index_episodes: number;
  /** 开了 Webhook；webhook_at、webhook_event 是最近一次收到的时间与事件 */
  webhook: boolean;
  webhook_at?: string;
  webhook_event: string;
  created_at: string;
  updated_at: string;
}

/** 同步一次索引的结果（POST /api/media/servers/{id}/sync） */
export interface MediaServerSyncResult {
  movies: number;
  series: number;
  episodes: number;
  added: number;
  updated: number;
  removed: number;
  restored: number;
  purged: number;
  issues: number;
  /** 同步的时候服务器里的条目变了：这一次没有标删除 */
  partial?: boolean;
  /** 换了一台服务器：清掉了旧的索引 */
  reset?: boolean;
}

export interface MediaServerUser {
  id: string;
  name: string;
}

/** 生成的 Webhook 令牌：只在这一次给；path 前面加上媒体服务器访问得到的 pt-tools 地址 */
export interface MediaServerWebhookToken {
  token: string;
  path: string;
}

/** 继续观看的一项（GET /api/media/resume）；剧集的 title 是剧名 */
export interface MediaResumeItem {
  server_id: number;
  item_id: string;
  media_type: string;
  tmdb_id?: number;
  title: string;
  season?: number;
  episode?: number;
  episode_name?: string;
  still_path?: string;
  backdrop_path?: string;
  poster_path?: string;
  percent: number;
  remaining_seconds?: number;
  open_url?: string;
}

/** 一台服务器的继续观看；error 是这一台没取到的原因 */
export interface MediaResumeGroup {
  server_id: number;
  name: string;
  kind: MediaServerKind;
  user: string;
  items: MediaResumeItem[];
  error?: string;
}

export type MediaIssueKind = "server_removed" | "library_missing";
export type MediaIssueStatus = "open" | "resolved" | "dismissed";

/**
 * 待处理问题：server_removed 是 pt-tools 整理过的片子在媒体服务器里没了；library_missing 是每天巡检发现库里的文件不见了
 * （history_id 是那条整理记录）
 */
export interface MediaIssue {
  id: number;
  kind: MediaIssueKind;
  status: MediaIssueStatus;
  server_id: number;
  server_name?: string;
  server_item_id: string;
  history_id: number;
  media_type: MediaKind;
  tmdb_id: number;
  season: number;
  episode: number;
  title: string;
  detail: string;
  created_at: string;
  updated_at: string;
  resolved_at?: string;
}

export interface MediaIssueQuery {
  status?: MediaIssueStatus;
  kind?: MediaIssueKind;
  media_type?: MediaKind;
  tmdb_id?: number;
}

export interface MediaServerInfo {
  name: string;
  version: string;
  libraries?: number;
}

/**
 * 两种整理请求共用的可选项：指定条目（类型与 TMDB 编号一起给）与媒体库；不给时自动识别、按类型选库。
 * mode 是这次用的整理方式，不给时跟媒体库（只有网页会话的两个整理路由收，App API 不收）
 */
interface OrganizeRequestOptions {
  media_type?: MediaKind;
  tmdb_id?: number;
  library_id?: number;
  mode?: MediaMode;
}

/** 整理下载器里的一个种子 */
export interface TorrentOrganizeRequest extends OrganizeRequestOptions {
  downloader_id: number;
  hash: string;
}

/** 手动整理 pt-tools 里看到的一个文件或文件夹（只有网页会话能用，App API 不收） */
export interface PathOrganizeRequest extends OrganizeRequestOptions {
  source_path: string;
}

/** 整理监控目录里的一项（只有网页会话能用，App API 不收） */
export interface WatchOrganizeRequest extends OrganizeRequestOptions {
  watch_dir_id: number;
  entry: string;
}

export type OrganizeRequest = TorrentOrganizeRequest | PathOrganizeRequest | WatchOrganizeRequest;

export type OrganizeItemStatus = "pending" | "done" | "exists" | "replace" | "failed";

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
  /** 这一项是一张原盘（BDMV 蓝光、VIDEO_TS DVD）：source 是原盘根目录，target 是电影目录 */
  disc?: { kind: "BDMV" | "VIDEO_TS"; files: number };
}

/**
 * 整理时条目从哪来：manual 是手动指定的，history 是沿用之前整理这些文件时用的，subscription 是订阅的，
 * push 是推送时说明的（按条目搜索、订阅推送）；都没有时是媒体识别的来源
 */
export type MediaMatchSource =
  | MediaRecognizeResult["source"]
  | "manual"
  | "history"
  | "subscription"
  | "push"
  | "nfo"
  | "nfo-imdb"
  | "dirname";

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
  source?: MediaMatchSource;
  library?: MediaLibraryInput & { id: number };
  /** 放进的二级分类（媒体库开了「按二级分类放进子目录」、规则对得上时才有） */
  category?: string;
  /** 二级分类怎么定的（媒体库开了「按二级分类放进子目录」、这一类有规则时才有） */
  classify?: ClassifyNote;
  mode?: MediaMode;
  items: OrganizePlanItem[];
  skipped?: { file: { rel: string; path: string; size: number }; reason: string }[];
  /** 整个种子整理不了的原因 */
  problem?: string;
  /** 整理不了的分类（有 problem 时才有） */
  fail_kind?: MediaFailKind;
  /** 手动指定路径整理时是规整后的路径（改成按种子整理时没有） */
  source_path?: string;
  /** 不挡整理的说明（例如指定的路径是下载器里的种子，改成了按种子整理） */
  notes?: string[];
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

/**
 * 失败与跳过的整理记录的分类：没识别出来、媒体库的问题、要加路径映射、文件系统（跨文件系统、权限）、目标已有别的文件、
 * 源文件不在了、整理不了的原盘与镜像、其他。整理成的记录为空；升级前的失败、跳过记录也为空（当作其他）
 */
export type MediaFailKind =
  | "recognize"
  | "library"
  | "path"
  | "filesystem"
  | "exists"
  | "source"
  | "disc"
  | "other";

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
  /** existing 是导入已有媒体库时记下的文件（导入时就在库里，不是 pt-tools 放进去的） */
  mode: MediaMode | "existing";
  /** 手动整理时选的整理方式（空串是跟媒体库）：重试按它整理 */
  chosen_mode: "" | MediaMode;
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
  /** 失败与跳过的分类（见 MediaFailKind） */
  fail_kind: "" | MediaFailKind;
  attempts: number;
  next_retry_at?: string;
  trigger: "auto" | "scan" | "manual" | "watch" | "import";
  /** 条目从哪来（升级前的记录为空） */
  match_source?: "" | MediaMatchSource;
  /** 目录监控的目录（不是下载器里的种子时） */
  watch_dir_id?: number;
  /** 手动指定路径整理的记录当时填的路径（别的记录没有） */
  manual_path?: string;
  subtitles: number;
  /** 原盘的记录放进库的文件数（不是原盘时没有） */
  disc_files?: number;
  /** 记录里记着 pt-tools 放进库的文件（跳过、失败的记录也可能有），可以连文件删 */
  has_files: boolean;
  /** 上一次刮削的结果：空串是没刮削过（媒体库关着刮削，或升级前整理的） */
  scrape_status: "" | MediaScrapeStatus;
  /** 刮削出的问题与说明（用了 TMDB 代替的一类） */
  scrape_note: string;
  scraped_at?: string;
  created_at: string;
  updated_at: string;
}

/** 刮削的结果：成功；部分成功（有的文件没写成，或有来源出错、用了 TMDB 的）；失败 */
export type MediaScrapeStatus = "ok" | "partial" | "failed";

/** 重新刮削的结果 */
export interface RescrapeResult {
  /** 刮削了的整理记录数（这个媒体库里同一部电影或剧集） */
  records: number;
  /** 写了的文件数（内容一样的不重写） */
  written: number;
  messages?: string[];
  /** NFO 里文字的每一项用了哪个来源 */
  sources?: string;
  /** 还在后台刮削，稍后到整理历史里看 */
  queued?: boolean;
}

export interface MediaHistoryPage {
  items: MediaHistoryItem[];
  total: number;
}

/** 按条目分的一组整理记录：识别出条目的按类型、TMDB 编号与季分，没识别出的按种子名（torrent）分 */
export interface MediaHistoryGroup {
  media_type?: MediaKind;
  tmdb_id?: number;
  season?: number;
  torrent?: string;
  title?: string;
  year?: number;
  total: number;
  done: number;
  failed: number;
  skipped: number;
  removed: number;
  /** 组里最近更新的那条记录的时间 */
  latest_at: string;
}

export interface MediaHistoryGroupPage {
  groups: MediaHistoryGroup[];
  total: number;
}

/** 整理历史的查询：kind 只看失败与跳过的记录；media_type、tmdb_id、season 或 torrent 选一组（见 MediaHistoryGroup） */
export interface MediaHistoryQuery {
  status?: string;
  kind?: MediaFailKind;
  q?: string;
  limit?: number;
  offset?: number;
  media_type?: MediaKind;
  tmdb_id?: number;
  season?: number;
  torrent?: string;
}

export type MediaHistoryBatchAction = "retry" | "delete" | "delete_files" | "rescrape";

/** 批量操作的结果：每条记录一项；重试与重新刮削排进整理队列就回 */
export interface MediaHistoryBatchResult {
  items: { id: number; ok: boolean; message?: string }[];
  ok: number;
  failed: number;
}

/**
 * 「让 AI 认」的结果：chosen 是选中的候选下标，-1 是都不是（或没有候选、没调模型）；选中时 request 是带上这个条目的整理请求，
 * preview 是按它算出的整理预览（不整理）
 */
export interface MediaAIRecognizeResult {
  /** 认的标题（种子名与站点标题）：「记成纠正」用它 */
  input: { title: string; subtitle?: string };
  candidates: MediaCandidate[];
  chosen: number;
  reason?: string;
  model?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  request?: OrganizeRequest & { media_type: MediaKind; tmdb_id: number };
  preview?: OrganizePlan;
}

/** 「手动整理」浏览文件时能进的根目录：kinds 是它从哪来（媒体库、监控目录、路径映射、下载器的下载目录） */
export interface MediaFSRoot {
  path: string;
  kinds: ("library" | "watch" | "map" | "downloader")[];
  labels?: string[];
}

export interface MediaFSEntry {
  name: string;
  path: string;
  dir: boolean;
  size?: number;
  mod_time: string;
}

/** 一层目录：目录在前、按名字排；root 是所在的根目录，parent 是上一级（已经是根目录时没有） */
export interface MediaFSList {
  path: string;
  root: string;
  parent?: string;
  entries: MediaFSEntry[];
  truncated?: boolean;
}

function historyParams(q: MediaHistoryQuery, group = false): string {
  const params = new URLSearchParams();
  if (q.status) params.set("status", q.status);
  if (q.kind) params.set("kind", q.kind);
  if (q.q) params.set("q", q.q);
  if (q.limit) params.set("limit", String(q.limit));
  if (q.offset) params.set("offset", String(q.offset));
  if (q.tmdb_id) {
    params.set("media_type", q.media_type ?? "");
    params.set("tmdb_id", String(q.tmdb_id));
    params.set("season", String(q.season ?? 0));
  } else if (q.torrent) {
    params.set("torrent", q.torrent);
  }
  if (group) params.set("group", "item");
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}

export const organizeApi = {
  settings: () => api.get<OrganizeSettings>("/api/media/organize/settings"),
  saveSettings: (data: OrganizeSettingsInput) =>
    api.put<OrganizeSettings>("/api/media/organize/settings", data),
  classify: () => api.get<ClassifyView>("/api/media/organize/classify"),
  watchDirs: () => api.get<MediaWatchDir[]>("/api/media/watch-dirs"),
  createWatchDir: (data: MediaWatchDirInput) =>
    api.post<MediaWatchDir>("/api/media/watch-dirs", data),
  updateWatchDir: (id: number, data: MediaWatchDirInput) =>
    api.put<MediaWatchDir>(`/api/media/watch-dirs/${id}`, data),
  deleteWatchDir: (id: number) => api.delete<{ ok: boolean }>(`/api/media/watch-dirs/${id}`),
  saveClassify: (data: ClassifyRules) =>
    api.put<ClassifyView>("/api/media/organize/classify", data),
  preview: (data: OrganizeRequest) => api.post<OrganizePlan>("/api/media/organize/preview", data),
  organize: (data: OrganizeRequest) => api.post<OrganizeResult>("/api/media/organize", data),
  libraries: () => api.get<MediaLibrary[]>("/api/media/libraries"),
  createLibrary: (data: MediaLibraryInput) => api.post<MediaLibrary>("/api/media/libraries", data),
  updateLibrary: (id: number, data: MediaLibraryInput) =>
    api.put<MediaLibrary>(`/api/media/libraries/${id}`, data),
  deleteLibrary: (id: number) => api.delete<{ ok: boolean }>(`/api/media/libraries/${id}`),
  checkLibrary: (data: { path: string; mode: MediaMode }) =>
    api.post<MediaCheckItem[]>("/api/media/libraries/check", data),
  /** 访问一次 ChineseSubFinder（不建任务）；带了媒体库编号、没填 Key 时用这个库保存的 Key */
  testCSF: (data: { id?: number; url: string; key?: string }) =>
    api.post<{ ok: boolean }>("/api/media/libraries/csf-test", data),
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
  /** 马上同步这台服务器的索引，等同步做完（大的库要几分钟） */
  syncServer: (id: number) => api.post<MediaServerSyncResult>(`/api/media/servers/${id}/sync`),
  /** 服务器的用户（选继续观看的用户；请求同测试连接） */
  serverUsers: (data: { id?: number; kind: MediaServerKind; url: string; token?: string }) =>
    api.post<MediaServerUser[]>("/api/media/servers/users", data),
  /** 生成（或重新生成）Webhook 令牌：原来的作废 */
  createWebhook: (id: number) =>
    api.post<MediaServerWebhookToken>(`/api/media/servers/${id}/webhook`),
  deleteWebhook: (id: number) => api.delete<{ ok: boolean }>(`/api/media/servers/${id}/webhook`),
  resume: () => api.get<{ servers: MediaResumeGroup[] }>("/api/media/resume"),
  issues: (q: MediaIssueQuery = {}) => {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(q)) if (v !== undefined && v !== "") p.set(k, String(v));
    const qs = p.toString();
    return api.get<{ items: MediaIssue[] }>(`/api/media/issues${qs ? `?${qs}` : ""}`);
  },
  dismissIssue: (id: number) => api.post<MediaIssue>(`/api/media/issues/${id}/dismiss`),
  history: (q: MediaHistoryQuery = {}) =>
    api.get<MediaHistoryPage>(`/api/media/history${historyParams(q)}`),
  /** 按条目分组（组的条件不看） */
  historyGroups: (q: MediaHistoryQuery = {}) =>
    api.get<MediaHistoryGroupPage>(`/api/media/history${historyParams(q, true)}`),
  /** allowExisting：连文件删除时也删导入的已有文件（页面上另外确认过） */
  historyBatch: (ids: number[], action: MediaHistoryBatchAction, allowExisting = false) =>
    api.post<MediaHistoryBatchResult>("/api/media/history/batch", {
      ids,
      action,
      ...(allowExisting ? { allow_existing: true } : {}),
    }),
  /** 让 AI 从 TMDB 的候选里认这个种子（请求同预览）：只认、不整理 */
  aiRecognize: (data: OrganizeRequest) =>
    api.post<MediaAIRecognizeResult>("/api/media/organize/ai-recognize", data),
  aiRecognizeHistory: (id: number) =>
    api.post<MediaAIRecognizeResult>(`/api/media/history/${id}/ai-recognize`),
  /** 「让 AI 认」能不能用（AI 助手启用并配好了没有），不能用时写明原因 */
  aiStatus: () => api.get<{ ready: boolean; reason?: string }>("/api/media/organize/ai-status"),
  fsRoots: () => api.get<{ roots: MediaFSRoot[] }>("/api/media/fs/roots"),
  fsList: (path: string) =>
    api.get<MediaFSList>(`/api/media/fs/list?path=${encodeURIComponent(path)}`),
  retry: (id: number) => api.post<OrganizeResult>(`/api/media/history/${id}/retry`),
  /** 按媒体库现在的设置重新刮削这条记录所在的电影或剧集 */
  rescrape: (id: number) => api.post<RescrapeResult>(`/api/media/history/${id}/scrape`),
  /** kept 是换掉或改过、没有删的文件的说明；allowExisting：连文件删除导入的已有文件（页面上另外确认过） */
  deleteHistory: (id: number, files = false, allowExisting = false) =>
    api.delete<{ ok: boolean; kept?: string[] }>(
      `/api/media/history/${id}${files ? `?files=1${allowExisting ? "&allow_existing=1" : ""}` : ""}`,
    ),
  /** 开始扫描媒体库里已有的文件（后台进行；path 空串是库目录），回扫描的状态 */
  importScan: (libraryId: number, path: string) =>
    api.post<MediaImportScan>(`/api/media/libraries/${libraryId}/import/scan`, { path }),
  /** 扫描的进度与结果；brief 时不带每一项（轮询进度用） */
  importScanStatus: (id: string, brief = false) =>
    api.get<MediaImportScan>(`/api/media/import/scans/${id}${brief ? "?brief=1" : ""}`),
  /** 取消还在进行的扫描；扫描完了的丢掉结果 */
  cancelImportScan: (id: string) => api.delete<{ ok: boolean }>(`/api/media/import/scans/${id}`),
  /** 改扫描里一项的条目（类型跟媒体库） */
  setImportItem: (id: string, path: string, tmdbId: number) =>
    api.put<MediaImportItem>(`/api/media/import/scans/${id}/item`, { path, tmdb_id: tmdbId }),
  /** 导入选中的项（排进整理的队列）；scrape 时每个条目补刮一次（只补没有的 NFO 与图片） */
  importItems: (id: string, paths: string[], scrape: boolean) =>
    api.post<MediaImportRun>(`/api/media/import/scans/${id}/import`, { paths, scrape }),
};

// ---- 导入已有媒体库 ----

/** 扫描与导入的状态 */
export type MediaImportStatus = "running" | "done" | "failed" | "canceled" | "queued";

/** 扫描出的一项里的一个视频 */
export interface MediaImportFile {
  /** 相对这一项的路径（库目录里直接放的视频是文件名） */
  rel: string;
  size: number;
  season?: number;
  episode?: number;
  episode_end?: number;
  /** 已经有整理记录（导入时跳过） */
  recorded?: boolean;
  /** 这个视频导入不了的原因（例如没认出集数） */
  problem?: string;
}

/** 一项认出的条目 */
export interface MediaImportMatch {
  media_type: MediaKind;
  tmdb_id: number;
  title: string;
  original_title?: string;
  year?: number;
  poster_path?: string;
}

/** 扫描出的一项：一部电影（一个文件夹，或库目录里直接放的一个视频）或一部剧（一个剧集目录） */
export interface MediaImportItem {
  path: string;
  /** 相对库目录的路径 */
  name: string;
  file?: boolean;
  match?: MediaImportMatch;
  source?: MediaMatchSource;
  /** 条目是页面上改的 */
  edited?: boolean;
  /** 视频（最多 200 个）；file_count 与 size 按全部视频算 */
  files: MediaImportFile[];
  file_count: number;
  size: number;
  /** 能导入的视频数（没有整理记录、没有问题的） */
  ready: number;
  /** 已经有整理记录的视频数 */
  recorded: number;
  /** 这一项导入不了的原因 */
  problem?: string;
  notes?: string[];
}

/** 一次导入：排进整理的队列，和整理一样一次只做一件 */
export interface MediaImportRun {
  status: MediaImportStatus;
  items: number;
  done: number;
  created: number;
  skipped: number;
  failed: number;
  scraped: number;
  messages?: string[];
  finished_at?: string;
}

/** 一次扫描：进度、每一项（扫描完了、不是 brief 时才有）与之后的导入 */
export interface MediaImportScan {
  id: string;
  library_id: number;
  path: string;
  status: MediaImportStatus;
  scanned: number;
  total: number;
  /** 多于 1000 项：只列了前这么多项，其余的分子目录扫描 */
  truncated?: boolean;
  error?: string;
  items?: MediaImportItem[];
  started_at: string;
  finished_at?: string;
  import?: MediaImportRun;
}

// ---- 订阅（路线图 M11） ----

export type SubscriptionStatus = "active" | "paused" | "pending" | "done";
export type SubscriptionSource = "manual" | "explore" | "douban" | "chatops" | "app";
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
  /** 每天 today_time（HH:MM）把今天播出的集推送到 today_channels（空的用 notify_channels） */
  today_enabled: boolean;
  today_time: string;
  today_channels: number[];
  /** 下载时按条目建子目录：订阅推送的种子放进保存路径（没填时是下载器的默认保存路径）下按 subdir_template 起名的子目录 */
  subdir_enabled: boolean;
  /** 子目录名：{title}、{original_title}、{year}、{season}、{tmdb_id}；空着保存时是 {title} ({year}) */
  subdir_template: string;
  /** 下载前让 AI 复核：推送订阅的候选以前问 AI 助手的模型是不是要的条目，说不是的不推 */
  ai_review_enabled: boolean;
  /** name 只复核按名字对上的（按 IMDb 编号对上的不问），all 全部 */
  ai_review_scope: "name" | "all";
  /** 追剧开着版本锁时，一集等锁住的版本满几天以后改用别的版本（同「这一集换别的版本」）；0 是一直等 */
  unlock_after_days: number;
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

/** 剧集一集的情况：已入库、下载中、缺、在等锁住的版本、还没播、不追（起始集以前的或跳过的）。 */
export type EpisodeStateKind =
  | "library"
  | "downloading"
  | "missing"
  | "waiting_version"
  | "upcoming"
  | "skipped";

export interface SubscriptionEpisode {
  number: number;
  name?: string;
  air_date?: string;
  state: EpisodeStateKind;
  /** 状态从哪里来：媒体库、订阅推送的种子、下载器里已有的种子、媒体服务器里有（开了「算作已入库」的服务器） */
  source?: "library" | "subscription" | "downloader" | "server";
  /** 现在的版本（制作组与分辨率） */
  version?: string;
  manual_skip?: boolean;
  /** 这一集不受版本锁限制 */
  manual_unlock?: boolean;
}

export interface SubscriptionProgress {
  total: number;
  /** 要追的集数（起始集以后、没跳过的） */
  required?: number;
  aired: number;
  in_library: number;
  downloading: number;
  /** 在等锁住的版本的集数（也算在 missing 里） */
  waiting?: number;
  skipped?: number;
  missing?: number[];
  /** 每一集的状态（按集号，列表里画格子用） */
  states?: EpisodeStateKind[];
  episodes?: SubscriptionEpisode[];
}

/** 追剧主动搜索的定时：空的沿用全局间隔。 */
export type FollowScheduleKind =
  | ""
  | "airtime"
  | "interval"
  | "daily"
  | "every_days"
  | "weekly"
  | "monthly"
  | "cron";

export interface FollowSchedule {
  kind: FollowScheduleKind;
  /** interval：每隔几分钟（30 分钟到 7 天），从上次搜索算起 */
  interval_minutes?: number;
  /** cron：分 时 日 月 星期；几条用分号分开，到点的取最早的 */
  cron?: string;
  /** HH:MM；按播出时间时是播出的时刻 */
  at?: string;
  /** every_days 的天数 */
  every?: number;
  /** weekly 的星期几（0 是星期日）或 monthly 的几号 */
  days?: number[];
  /** 按播出时间：播出后几小时开始搜、没找到隔几小时再搜、播出后几天里集中重试（0 用默认的 2、6、7） */
  air_delay_hours?: number;
  air_retry_hours?: number;
  air_window_days?: number;
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
  /** 标题（连同副标题）的包含词、排除词：按行或逗号分开；更早的版本没有 */
  include_keywords?: string;
  exclude_keywords?: string;
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
  // 以下是追剧（剧集）的设置；更早的版本没有
  schedule?: FollowSchedule;
  /** 从第几集开始追（0 与 1 是整季） */
  start_episode?: number;
  /** 集数是手动填的 */
  total_episodes_manual?: boolean;
  /** 版本锁：同一个制作组、同一个分辨率 */
  lock_version?: boolean;
  lock_group?: string;
  lock_resolution?: string;
  /** 最近一次搜索找到的、能补上缺的集的版本（剧集；没搜过时没有）：「修改」里改锁时给用户选 */
  seen_versions?: { at: string; versions: SeenVersion[] };
  follow_next_season?: boolean;
  next_season_id?: number;
  /** 整理进哪个媒体库（0 按类型自动选） */
  library_id?: number;
  /** 种子按 TMDB 的哪个剧集组写季集（剧集；空串是按 TMDB 默认的季集） */
  episode_group_id?: string;
  completed_at?: string;
}

/** 最近一次搜索找到的一个版本：能补上哪几个缺的集、在哪些站点上 */
export interface SeenVersion {
  group: string;
  resolution: string;
  episodes: number[];
  sites: string[];
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
  /** 这个种子整理进媒体库的情况：每个视频文件一条整理记录（还没整理过时为空） */
  files?: SubscriptionTorrentFile[];
  /** 能重新整理：有整理没成、也不在自动重试的文件，或者种子标成了失败、还没有整理记录 */
  can_organize?: boolean;
  /** 能重新刮削：有整理好的文件 */
  can_rescrape?: boolean;
  /** 对这个订阅的条目拉黑了（不要这个种子） */
  blocked?: boolean;
}

/** 订阅下载的种子里一个视频文件的整理记录 */
export interface SubscriptionTorrentFile {
  /** 整理记录的编号（整理历史里同一条） */
  id: number;
  name: string;
  episode?: number;
  episode_end?: number;
  status: MediaTransferStatus;
  message?: string;
  scrape_status?: "" | MediaScrapeStatus;
  scrape_note?: string;
  next_retry_at?: string;
  /** 整理队列正在重试它 */
  retrying?: boolean;
}

export interface SubscriptionDetail extends Subscription {
  torrent_list: SubscriptionTorrent[];
}

/** 新建订阅前读的条目（GET /api/media/subscriptions/draft，只读） */
export interface SubscriptionDraft {
  media_type: MediaKind;
  tmdb_id: number;
  /** 这一季：请求的 0 换成最新一季；电影是 0 */
  season: number;
  /** 一共几季、这一季的集数（电影都是 0） */
  seasons: number;
  total_episodes: number;
  title: string;
  original_title: string;
  /** 年份（剧集是这一季的） */
  year: number;
  poster_path: string;
  /** TMDB 类型里有「动画」 */
  anime: boolean;
  /** 媒体库选「自动」时整理会选到的库（选法同整理）；没有能用的库时是 0 */
  auto_library_id: number;
  /** 这一季已有的订阅；没有时是 0 */
  subscription_id: number;
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
  /** 标题的包含词（含其中任一个才要）与排除词（含任何一个都不要）：按行或逗号分开；不给时修改不变 */
  include_keywords?: string;
  exclude_keywords?: string;
  // 追剧的设置：不给时新建按默认（按播出时间、开着版本锁），修改时不变
  schedule?: FollowSchedule;
  lock_version?: boolean;
  /** lock_group 与 lock_resolution 都给空串是清掉锁住的版本 */
  lock_group?: string;
  lock_resolution?: string;
  start_episode?: number;
  follow_next_season?: boolean;
  library_id?: number;
  /** 这一季的集数：大于 0 时以它为准，0 改回按 TMDB（只在修改时用） */
  total_episodes?: number;
  /** 种子按 TMDB 的哪个剧集组写季集：空串是不用剧集组；不给时新建不用、修改不变 */
  episode_group_id?: string;
  /** 从站点上的这个种子开始追（只在新建时用） */
  from_torrent?: { site: string; torrent_id: string; title?: string };
}

export interface SubscriptionRunStep {
  kind: "search" | "reject" | "lock" | "push" | "fail" | "adopt" | "wait" | "info";
  text: string;
}

/** 订阅的一次运行：主动搜索、RSS 命中、建立、手动操作。 */
export interface SubscriptionRun {
  id: number;
  subscription_id: number;
  trigger: "create" | "schedule" | "rss" | "manual" | "ai";
  started_at: string;
  finished_at?: string;
  /** changed 是手动改动的记录（改设置、跳过一集、换版本、集号规则），不是一次搜索 */
  status: "ok" | "nothing" | "waiting" | "failed" | "partial" | "changed";
  summary: string;
  steps: SubscriptionRunStep[];
  steps_truncated: boolean;
}

export interface SubscriptionRunPage {
  items: SubscriptionRun[];
  total: number;
}

/** 集号规则：tmdb 按 TMDB 的集号、period_part 按第几期与上下、date 按播出日期、offset 加偏移。 */
export type EpisodeRuleMode = "tmdb" | "period_part" | "date" | "offset";

export interface EpisodeRule {
  id: number;
  subscription_id: number;
  /** 制作组；空的对所有组 */
  group: string;
  mode: EpisodeRuleMode;
  offset: number;
  source: "manual" | "ai" | "auto";
  note: string;
  created_at: string;
  updated_at: string;
}

export interface EpisodeRuleInput {
  group: string;
  mode: EpisodeRuleMode;
  offset: number;
  note: string;
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
  status: "subscribed" | "unmatched" | "removed";
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

/**
 * 探索页的列表：两边都有本周趋势、热门、高分；电影另有正在上映、即将上映，剧集另有正在播出、今天播出；
 * discover 按出品国家与类型发现，search 按名字搜
 */
export type ExploreList =
  | "trending"
  | "popular"
  | "top_rated"
  | "now_playing"
  | "upcoming"
  | "on_the_air"
  | "airing_today"
  | "discover"
  | "search";

/** 番剧日历的一天：weekday 1 是星期一，7 是星期日 */
export interface BangumiDay {
  weekday: number;
  name: string;
  items: BangumiCalendarItem[];
}

/** 番剧日历里的一部动画（Bangumi 的条目） */
export interface BangumiCalendarItem {
  id: number;
  name: string;
  name_cn?: string;
  air_date?: string;
  /** 封面（Bangumi 的 https 图片地址） */
  image?: string;
  score?: number;
  votes?: number;
  rank?: number;
  /** 在看的人数 */
  watching?: number;
}

/** 按条目搜索的请求（POST /api/media/search，只读） */
export interface MediaSearchInput {
  media_type: MediaKind;
  tmdb_id: number;
  /** 剧集的季，0 或不填是最新的一季 */
  season?: number;
  /** 只影响这一次：大于 0 用这个档案，0 用订阅的（没有时用默认档案），-1 不按档案 */
  profile_id?: number;
  sites?: string[];
  include_unmatched?: boolean;
  /**
   * 试搜索：还没保存的表单（质量档案、标题包含与排除），给了时按它对种子，不按库里的订阅；不能和 profile_id 同时给。
   * profile_id 为 0 时是设置里的默认档案（和保存以后一样）
   */
  draft?: MediaSearchDraft;
}

/** 试搜索时还没保存的表单 */
export interface MediaSearchDraft {
  profile_id: number;
  include_keywords: string;
  exclude_keywords: string;
}

/** 一集现在的情况：已入库、在下载、等锁住的版本、缺、还没播 */
export type MediaSearchEpisodeState =
  | "library"
  | "downloading"
  | "waiting_version"
  | "missing"
  | "upcoming";

export interface MediaSearchEpisode {
  episode: number;
  state: MediaSearchEpisodeState;
  /** 在下载时的来源站点（下载器里认领的种子没有） */
  site?: string;
  air_date?: string;
  name?: string;
}

/** 按档案的结果：reject 不为空时不符合（全部原因），score 是 0–100 的分 */
export interface MediaSearchVerdict {
  score: number;
  reject?: string;
}

export interface MediaSearchMember {
  site: string;
  torrent_id: string;
  title: string;
  subtitle?: string;
  info_hash?: string;
  size_bytes: number;
  uploaded_at?: number;
  seeders: number;
  leechers: number;
  snatched?: number;
  /** 站点给的优惠（FREE、2XFREE、PERCENT_50、NONE……） */
  discount: string;
  free: boolean;
  /** 免费还剩几秒；不知道结束时间时没有 */
  free_left_seconds?: number;
  /** 有 H&R 要求（种子自己有，或者站点整站有）；hr_hours 是要做种几小时，不知道时没有 */
  hr: boolean;
  hr_hours?: number;
  url?: string;
  download_url?: string;
  category?: string;
  by_imdb?: boolean;
  meta: MediaMeta;
  /** 不按档案时为 null */
  verdict: MediaSearchVerdict | null;
  /** 对不上这个条目的原因（只在 unmatched 里） */
  reason?: string;
  /** 拉黑了（对这个条目，或 all 为真时对所有条目）；没拉黑时没有 */
  block?: { id: number; reason?: string; all?: boolean };
}

/** 同一个资源：members 的第一个是首选 */
export interface MediaSearchGroup {
  group_id: string;
  members: MediaSearchMember[];
  meta: MediaMeta;
  verdict: MediaSearchVerdict | null;
  /** 剧集里有的集（电影没有） */
  span?: { start?: number; end?: number; whole?: boolean };
  /** 能补上的集（缺的与等版本的） */
  covers: number[];
  /** 这一组里正在下载的集 */
  downloading: number[];
  /** 这一组的集都已入库（电影是库里有） */
  in_library: boolean;
  /** 这一组的成员都拉黑了（排在最后） */
  blocked?: boolean;
}

export interface MediaSearchResult {
  item: {
    media_type: MediaKind;
    tmdb_id: number;
    season?: number;
    title: string;
    original_title?: string;
    year?: number;
    poster_path?: string;
    /** 剧集一共几季、这一季几集 */
    seasons?: number;
    total_episodes?: number;
  };
  subscription: {
    id: number;
    status: SubscriptionStatus;
    lock: { group: string; resolution: string } | null;
  } | null;
  /** 这次打分用的档案；不按档案时为 null，没有档案（按分辨率与来源比）时 id 为 0 */
  profile: { id: number; name: string } | null;
  keywords: string[];
  episodes: MediaSearchEpisode[];
  movie?: { state: "library" | "downloading" | "missing"; site?: string };
  groups: MediaSearchGroup[];
  total: number;
  /** 对上了、但拉黑了的结果数（成员数） */
  blocked_count: number;
  unmatched_count: number;
  unmatched?: MediaSearchMember[];
  /** 站点（site 为空时是一次搜索）的错误；timed_out 是到了时限还没有回应 */
  errors: { site: string; error: string; timed_out?: boolean }[];
}

// ---- 条目详情与订阅日历（媒体中心 M3） ----

/** 一集或一部电影现在的情况：已入库、在下载、等锁住的版本、缺、还没播、不追 */
export type ItemEpisodeState = MediaSearchEpisodeState | "skipped";

/** 演职员里的一位 */
export interface CastMember {
  name: string;
  character?: string;
  order: number;
  profile_path?: string;
}

/** 条目的资料：TMDB 详情与演职员 */
export interface ItemInfo {
  media_type: MediaKind;
  tmdb_id: number;
  title: string;
  original_title?: string;
  year?: number;
  /** 电影的上映日期、剧集的首播日期 */
  date?: string;
  overview?: string;
  genres: string[];
  /** 出品国家或地区（ISO 3166-1） */
  countries: string[];
  /** 片长（分钟）：电影是整部，剧集是一集 */
  runtime?: number;
  /** TMDB 的状态原文（Returning Series、Ended、Released……） */
  status?: string;
  poster_path?: string;
  backdrop_path?: string;
  rating?: number;
  votes?: number;
  imdb_id?: string;
  tvdb_id?: number;
  studios: string[];
  directors: string[];
  cast: CastMember[];
  /** 剧集的季数与总集数（不算特别篇） */
  seasons_count?: number;
  episodes_count?: number;
}

/** 剧集的一季：have 是这一季已入库的集数；n 为 0 是特别篇 */
export interface ItemSeason {
  n: number;
  name?: string;
  episode_count: number;
  air_date?: string;
  poster_path?: string;
  have: number;
}

/** 关联的种子在下载器里的状态；unknown 是下载器没读到 */
export type ItemTorrentState = "seeding" | "downloading" | "paused" | "error" | "unknown";

/** 和这个条目有关、还在下载器里的一个种子 */
export interface ItemTorrent {
  site: string;
  torrent_id?: string;
  title: string;
  /** 剧集的季（0 是不知道）；episodes 是种子里的集（E01–E04、整季） */
  season: number;
  episodes?: string;
  version?: string;
  size: number;
  downloader_id: number;
  downloader: string;
  hash: string;
  state: ItemTorrentState;
  percent: number;
  eta_seconds?: number;
  ratio: number;
  seeding_seconds: number;
  hr: boolean;
  hr_hours?: number;
  /** 能不能删（按自动删种的保护规则）：reason 是「能不能删」那一列的短字，protection 是受保护时的完整原因 */
  deletable: boolean;
  reason: string;
  protection?: string;
}

/** 这个条目的一个订阅（剧集每季一个，新的季在前）；follow 是追剧 */
export interface ItemSubscription {
  id: number;
  status: SubscriptionStatus;
  season: number;
  follow: boolean;
  lock?: { group: string; resolution: string };
}

/** 条目详情（GET /api/media/items/{type}/{tmdb_id}）；errors 是没取到的部分，其余照样给 */
export interface ItemDetail {
  item: ItemInfo;
  seasons: ItemSeason[];
  nas: {
    library: {
      /** 剧集按季写入库了几集（只列有入库的季） */
      seasons: { n: number; have: number; total: number }[];
      files: number;
      size: number;
      libraries: string[];
      /** 电影最新入库的那个文件的版本（分辨率与制作组） */
      version?: string;
      /** 最新的一条整理记录：「重新刮削」刮削它所在的电影或剧集 */
      record_id?: number;
      /** 还开着的「库里的文件不见了」（每天巡检发现的） */
      missing: { id: number; season?: number; episode?: number }[];
    };
    seeding: {
      count: number;
      /** 有 H&R 要求还没做够的个数，与其中离做够最近的还差几秒 */
      hr_pending: number;
      hr_left_seconds?: number;
      lowest_ratio?: number;
    };
  };
  subscriptions: ItemSubscription[];
  torrents: ItemTorrent[];
  recommendations: ExploreItem[];
  /** 开着、同步过索引的媒体服务器里有没有这一部（按服务器编号排） */
  servers?: ItemServer[];
  /** 删除时按哪几条保护（开着的保护规则）；读不到保护规则时为空 */
  protect_rules: string[];
  errors: string[];
}

/** 一台媒体服务器里的这一部：剧集写有几集与各季；removed 是 pt-tools 整理过、服务器里没了的（还没处理） */
export interface ItemServer {
  server_id: number;
  name: string;
  kind: MediaServerKind;
  synced_at?: string;
  have: boolean;
  episodes?: number;
  seasons?: { n: number; have: number; total: number }[];
  open_url?: string;
  removed: { id: number; season?: number; episode?: number }[];
}

/** 剧集一季里的一集 */
export interface SeasonEpisode {
  episode: number;
  name?: string;
  air_date?: string;
  /** 播出时刻：按播出时间追剧的才有 */
  air_at?: string;
  still_path?: string;
  runtime?: number;
  state: ItemEpisodeState;
  site?: string;
  /** 入库的：版本（分辨率与制作组）与体积 */
  version?: string;
  size?: number;
  /** 下载中的：进度与还要多久 */
  percent?: number;
  eta_seconds?: number;
  /** 整理记录里没有、算作已入库的媒体服务器里有：source 是 server，server 是服务器名 */
  source?: "server";
  server?: string;
  /** 已入库、媒体服务器里有这一集时的打开链接 */
  open_url?: string;
  /** 已入库、媒体服务器里被删了（还没处理的问题） */
  server_issue?: { id: number; server: string };
  /** 已入库、库里的文件不见了（还没处理的问题） */
  library_issue?: { id: number };
}

/** TMDB 上一部剧的一个剧集组（GET /api/media/items/tv/{tmdb_id}/episode-groups）：另一种排法，例如绝对集号、DVD 的分季 */
export interface EpisodeGroup {
  id: string;
  name: string;
  /** 1 播出日期、2 绝对集号、3 DVD、4 数字发行、5 故事线、6 制作顺序、7 电视台 */
  type: number;
  description?: string;
  group_count: number;
  episode_count: number;
  /** 这种排法所属的电视台或平台 */
  network?: string;
}

/** 剧集的一季（GET /api/media/items/tv/{tmdb_id}/seasons/{n}） */
export interface SeasonDetail {
  tmdb_id: number;
  season: number;
  name?: string;
  air_date?: string;
  /** 这一季的订阅；airtime 为真时按播出时间追剧，search_delay_hours 是播出后几小时开始搜 */
  subscription?: {
    id: number;
    status: SubscriptionStatus;
    follow: boolean;
    lock?: { group: string; resolution: string };
    airtime?: boolean;
    search_delay_hours?: number;
  };
  episodes: SeasonEpisode[];
  errors: string[];
}

/** 日历上的一项：剧集的一集，或电影的上映 */
export interface CalendarEntry {
  subscription_id: number;
  media_type: MediaKind;
  tmdb_id: number;
  title: string;
  year?: number;
  season?: number;
  episode?: number;
  name?: string;
  air_date: string;
  /** 播出时刻与开始搜的时刻：按播出时间追剧的才有 */
  air_at?: string;
  search_at?: string;
  state: ItemEpisodeState;
  poster_path?: string;
  still_path?: string;
}

/** 订阅日历（GET /api/media/calendar）：from 到 to 含两头；errors 是没读到 TMDB 的订阅 */
export interface MediaCalendar {
  from: string;
  to: string;
  today: string;
  entries: CalendarEntry[];
  errors: string[];
}

/** 删除一部片选的范围：一部电影；剧集的一季，episodes 不为空时只是其中几集 */
export interface DeleteSelector {
  media_type: MediaKind;
  tmdb_id: number;
  season: number;
  episodes?: number[];
}

/** 删除预览里的一条库文件（整理记录）：state 是 ours（当初放的）、missing（不在了）、changed（换成了别的，删的时候留着） */
export interface DeleteFile {
  id: number;
  path: string;
  library?: string;
  mode: string;
  existing: boolean;
  size: number;
  season: number;
  episode: number;
  episode_end: number;
  state: "ours" | "missing" | "changed";
  deletable: boolean;
  note?: string;
  downloader_id?: number;
  info_hash?: string;
}

/**
 * 删除预览里的一个关联种子：rule 是命中的保护规则，why 是受保护的短句（写在「受保护：」后面）；covers_more 是种子里还有这次没选的集
 * （默认不勾）；shares 是同一台下载器里和它共用数据的种子
 */
export interface DeleteTorrent extends ItemTorrent {
  rule?: string;
  why?: string;
  covers_more: boolean;
  more?: string;
  shares: { hash: string; name: string }[];
}

/** 删除预览（POST /api/media/items/delete/preview） */
export interface DeletePreview {
  files: DeleteFile[];
  torrents: DeleteTorrent[];
  subscriptions: ItemSubscription[];
  issues: MediaIssue[];
  errors: string[];
}

export interface TorrentKey {
  downloader_id: number;
  hash: string;
}

/** 订阅怎么办：暂停（默认）、删除、保留继续追 */
export type DeleteSubscriptionAction = "pause" | "delete" | "keep";

/** 删除请求（POST /api/media/items/delete） */
export interface DeleteRequest extends DeleteSelector {
  history_ids: number[];
  allow_existing: boolean;
  torrents: (TorrentKey & { delete_data: boolean })[];
  confirm_protected: TorrentKey[];
  subscription: DeleteSubscriptionAction;
}

/** 删除的一步：订阅、种子或一条整理记录（连库里的文件） */
export interface DeleteStep {
  kind: "subscription" | "torrent" | "file";
  target: string;
  id?: number;
  downloader_id?: number;
  hash?: string;
  ok: boolean;
  message: string;
}

export interface DeleteResult {
  steps: DeleteStep[];
  failed: number;
}

/** 拉黑的一个种子：tmdb_id 为 0 时对所有条目生效（关键字搜索里拉黑的） */
export interface TorrentBlock {
  id: number;
  site_name: string;
  torrent_id: string;
  media_type: "" | MediaKind;
  tmdb_id: number;
  season: number;
  /** 拉黑时条目的名字（作用于哪个条目） */
  media_title: string;
  title: string;
  reason: string;
  created_at: string;
}

/** 一次拉黑：同一资源在几个站点上的种子一起拉黑；tmdb_id 为 0 时对所有条目 */
export interface TorrentBlockInput {
  media_type?: MediaKind;
  tmdb_id?: number;
  season?: number;
  media_title?: string;
  reason?: string;
  items: { site: string; torrent_id: string; title: string }[];
  /** 把第一个种子的制作组加进这一季订阅的标题排除词（要有订阅） */
  exclude_group?: boolean;
}

export interface TorrentBlockResult {
  blocks: TorrentBlock[];
  /** 加进标题排除词的制作组（没有加时没有） */
  excluded_group?: string;
}

export const subscribeApi = {
  settings: () => api.get<SubscribeSettings>("/api/media/subscribe/settings"),
  /** 新建订阅前读条目（只读）：剧集不带季或 0 是最新一季 */
  draft: (q: { media_type: MediaKind; tmdb_id: number; season?: number }) => {
    const params = new URLSearchParams({ media_type: q.media_type, tmdb_id: String(q.tmdb_id) });
    if (q.season) params.set("season", String(q.season));
    return api.get<SubscriptionDraft>(`/api/media/subscriptions/draft?${params.toString()}`);
  },
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
  /** 重新整理订阅下载的一个种子（整季包与单集一样）：按订阅的条目与媒体库，已经整理好的文件不动 */
  organizeTorrent: (id: number, tid: number) =>
    api.post<OrganizeResult>(`/api/media/subscriptions/${id}/torrents/${tid}/organize`),
  /** 重新刮削订阅下载的一个种子整理进的电影或剧集 */
  rescrapeTorrent: (id: number, tid: number) =>
    api.post<RescrapeResult>(`/api/media/subscriptions/${id}/torrents/${tid}/scrape`),
  /** 定时接下来几次什么时候搜（只读）；定时不对时报错，错误文本说清楚哪里不对 */
  previewSchedule: (data: {
    schedule: FollowSchedule;
    media_type?: MediaKind;
    subscription_id?: number;
    count?: number;
  }) =>
    api.post<{ next: string[]; note?: string }>("/api/media/subscriptions/schedule-preview", data),
  episodes: (id: number) =>
    api.get<SubscriptionEpisode[]>(`/api/media/subscriptions/${id}/episodes`),
  skipEpisode: (id: number, n: number, skip: boolean) =>
    api.post<SubscriptionEpisode[]>(`/api/media/subscriptions/${id}/episodes/${n}/skip`, { skip }),
  unlockEpisode: (id: number, n: number, unlock: boolean) =>
    api.post<SubscriptionEpisode[]>(`/api/media/subscriptions/${id}/episodes/${n}/unlock`, {
      unlock,
    }),
  runs: (id: number, q: { limit?: number; offset?: number } = {}) => {
    const params = new URLSearchParams();
    if (q.limit) params.set("limit", String(q.limit));
    if (q.offset) params.set("offset", String(q.offset));
    const qs = params.toString();
    return api.get<SubscriptionRunPage>(`/api/media/subscriptions/${id}/runs${qs ? `?${qs}` : ""}`);
  },
  episodeRules: (id: number) =>
    api.get<EpisodeRule[]>(`/api/media/subscriptions/${id}/episode-rules`),
  saveEpisodeRule: (id: number, data: EpisodeRuleInput) =>
    api.put<EpisodeRule[]>(`/api/media/subscriptions/${id}/episode-rules`, data),
  deleteEpisodeRule: (id: number, group: string) =>
    api.delete<EpisodeRule[]>(
      `/api/media/subscriptions/${id}/episode-rules?group=${encodeURIComponent(group)}`,
    ),
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
    list: ExploreList;
    page?: number;
    q?: string;
    /** 按条件发现（list 是 discover）时的出品国家（例如 CN）与类型编号（几个用 | 隔开） */
    country?: string;
    genres?: string;
  }) => {
    const params = new URLSearchParams({ kind: q.kind, list: q.list });
    if (q.page) params.set("page", String(q.page));
    if (q.q) params.set("q", q.q);
    if (q.country) params.set("country", q.country);
    if (q.genres) params.set("genres", q.genres);
    return api.get<ExplorePage>(`/api/media/explore?${params.toString()}`);
  },
  /** 番剧日历：Bangumi 的每日放送（星期一到星期日，缓存 6 小时） */
  bangumiCalendar: () => api.get<{ days: BangumiDay[] }>("/api/media/explore/bangumi"),
  /** 番剧日历里的一部在 TMDB 上对应的剧集；找不到时 404，错误里写明搜过的名字 */
  bangumiTMDB: (id: number) =>
    api.get<{ media_type: MediaKind; tmdb_id: number; title: string }>(
      `/api/media/explore/bangumi/${id}`,
    ),
  /** 按条目搜索：只读，不推送；站点多时要十几秒到一分钟 */
  searchMedia: (data: MediaSearchInput) => api.post<MediaSearchResult>("/api/media/search", data),
  /** 黑名单：全部拉黑的种子，新的在前 */
  blocks: () => api.get<{ items: TorrentBlock[] }>("/api/media/blocks"),
  addBlocks: (data: TorrentBlockInput) => api.post<TorrentBlockResult>("/api/media/blocks", data),
  removeBlock: (id: number) => api.delete<{ ok: boolean }>(`/api/media/blocks/${id}`),
  /** 追剧详情里「不要这个种子」：还在下载的改成失败、马上再搜；下载器里的种子不删 */
  blockTorrent: (id: number, tid: number, data: { reason?: string; exclude_group?: boolean }) =>
    api.post<TorrentBlockResult>(`/api/media/subscriptions/${id}/torrents/${tid}/block`, data),
  /** 条目详情：资料、库里与做种的情况、订阅、关联的种子与推荐（只读） */
  item: (kind: MediaKind, id: number) => api.get<ItemDetail>(`/api/media/items/${kind}/${id}`),
  /** 剧集一季的每一集（只读） */
  itemSeason: (id: number, n: number) =>
    api.get<SeasonDetail>(`/api/media/items/tv/${id}/seasons/${n}`),
  /** 剧集在 TMDB 上的剧集组（追剧里选种子按哪个剧集组写季集） */
  episodeGroups: (id: number) =>
    api.get<{ groups: EpisodeGroup[] }>(`/api/media/items/tv/${id}/episode-groups`),
  /** 删除一部片的预览：库里的文件、关联的种子与能不能删、订阅、问题 */
  deletePreview: (sel: DeleteSelector) =>
    api.post<DeletePreview>("/api/media/items/delete/preview", sel),
  /** 按勾选删除：每一步单独回结果 */
  deleteItem: (req: DeleteRequest) => api.post<DeleteResult>("/api/media/items/delete", req),
  /** 订阅日历：from 到 to（YYYY-MM-DD，含两头，最多 62 天） */
  calendar: (from: string, to: string) =>
    api.get<MediaCalendar>(
      `/api/media/calendar?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
    ),
};

/** 等免费的状态 */
export type FreeWatchStatus = "watching" | "pushed" | "expired" | "canceled" | "failed";

/** 一条等免费：免费了才推送到记下的下载器与路径 */
export interface FreeWatch {
  id: number;
  site_name: string;
  torrent_id: string;
  title: string;
  subtitle: string;
  size_bytes: number;
  has_hr: boolean;
  hr_seed_time_h: number;
  downloader_id: number;
  save_path: string;
  category: string;
  tags: string;
  media_type: "" | MediaKind;
  tmdb_id: number;
  season: number;
  status: FreeWatchStatus;
  next_check_at?: string;
  last_check_at?: string;
  check_count: number;
  failures: number;
  expires_at: string;
  info_hash: string;
  message: string;
  created_at: string;
  updated_at: string;
}

export interface FreeWatchInput {
  site: string;
  torrent_id: string;
  title: string;
  subtitle?: string;
  size_bytes?: number;
  has_hr?: boolean;
  hr_seed_time_h?: number;
  downloader_id: number;
  save_path?: string;
  category?: string;
  tags?: string;
  media_type?: MediaKind;
  tmdb_id?: number;
  season?: number;
  /** 等多久（1–30 天，默认 7） */
  days?: number;
}

export const freeWatchApi = {
  list: (status?: FreeWatchStatus) =>
    api.get<{ items: FreeWatch[]; counts: Partial<Record<FreeWatchStatus, number>> }>(
      `/api/free-watches${status ? `?status=${status}` : ""}`,
    ),
  create: (data: FreeWatchInput) => api.post<FreeWatch>("/api/free-watches", data),
  /** 能等免费的站点（启用着、能读种子详情） */
  sites: () => api.get<{ sites: string[] }>("/api/free-watches/sites"),
  setExpiry: (id: number, expiresAt: string) =>
    api.patch<FreeWatch>(`/api/free-watches/${id}`, { expires_at: expiresAt }),
  cancel: (id: number) => api.post<FreeWatch>(`/api/free-watches/${id}/cancel`),
};

export const reseedApi = {
  settings: () => api.get<ReseedSettings>("/api/reseed/settings"),
  saveSettings: (data: ReseedSettingsUpdate) =>
    api.put<ReseedSettings>("/api/reseed/settings", data),
  sites: () => api.get<{ items: ReseedSiteMapItem[] }>("/api/reseed/sites"),
  /** 打开或关掉一个站点 */
  setSite: (site: string, selected: boolean) =>
    api.put<ReseedSiteSelection>(`/api/reseed/sites/${encodeURIComponent(site)}`, { selected }),
  /** 全部打开（以后新配置的站点也自动参与）或全部关掉 */
  setAllSites: (selected: boolean) =>
    api.put<ReseedSiteSelection>("/api/reseed/sites", { selected }),
  run: () => api.post<{ started: boolean; run_id: number }>("/api/reseed/run", {}),
  runs: (page = 1, pageSize = 20) =>
    api.get<ReseedPage<ReseedRun>>(`/api/reseed/runs?page=${page}&page_size=${pageSize}`),
  runDetail: (id: number) => api.get<ReseedRunDetail>(`/api/reseed/runs/${id}`),
  runAttempts: (id: number, page = 1, pageSize = 50) =>
    api.get<ReseedPage<ReseedAttempt>>(
      `/api/reseed/runs/${id}/attempts?page=${page}&page_size=${pageSize}`,
    ),
  pauseRun: (id: number) =>
    api.post<{ ok: boolean; current_run?: ReseedCurrentRun }>(`/api/reseed/runs/${id}/pause`, {}),
  resumeRun: (id: number) =>
    api.post<{ ok: boolean; current_run?: ReseedCurrentRun }>(`/api/reseed/runs/${id}/resume`, {}),
  /** cancelPending：这一轮结束以后一起取消它建的、还没加入下载器的任务 */
  stopRun: (id: number, cancelPending = false) =>
    api.post<{ ok: boolean }>(`/api/reseed/runs/${id}/stop`, { cancel_pending: cancelPending }),
  continueRun: (id: number) =>
    api.post<{ started: boolean; run_id: number }>(`/api/reseed/runs/${id}/continue`, {}),
  records: (filter: ReseedRecordFilter = {}, page = 1, pageSize = 20) => {
    const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
    if (filter.result) params.set("result", filter.result);
    if (filter.site) params.set("site", filter.site);
    if (filter.q) params.set("q", filter.q);
    return api.get<ReseedRecordPage>(`/api/reseed/records?${params}`);
  },
  /** 在后台重试选中的记录（一轮触发方式为「重试」的轮次） */
  retryRecords: (ids: number[]) =>
    api.post<{ started: boolean; run_id: number }>("/api/reseed/records/retry", { ids }),
  /** 删除记录；任务还没结束的不删（active 是没删的条数） */
  deleteRecords: (ids: number[]) =>
    api.post<{ deleted: number; active: number }>("/api/reseed/records/delete", { ids }),
  stats: (q: ReseedStatsQuery) => {
    const params = new URLSearchParams({ granularity: q.granularity });
    if (q.periods) params.set("periods", String(q.periods));
    if (q.from) params.set("from", q.from);
    if (q.to) params.set("to", q.to);
    return api.get<ReseedStats>(`/api/reseed/stats?${params}`);
  },
  /** 取消记录对应的、还在等待加入下载器的任务 */
  cancelRecord: (id: number) => api.post<{ ok: boolean }>(`/api/reseed/records/${id}/cancel`, {}),
  jobs: (status: "" | "active" | "finished" = "") =>
    api.get<{ items: TransferJob[] }>(
      `/api/reseed/jobs${status ? `?status=${encodeURIComponent(status)}` : ""}`,
    ),
  clearFinished: () => api.delete<{ deleted: number }>("/api/reseed/jobs"),
};

// ---- API 令牌（路线图 M12） ----

export type ApiTokenScope =
  | "app:read"
  | "app:write"
  | "app:config"
  | "mcp:read"
  | "mcp:write"
  | "qbit:compat"
  | "servarr";

export interface ApiToken {
  id: number;
  name: string;
  scopes: ApiTokenScope[];
  expires_at?: string;
  last_used_at?: string;
  created_by: string;
  created_at: string;
}

export interface ApiTokenInput {
  name: string;
  scopes: ApiTokenScope[];
  /** 0 表示不过期 */
  expires_in_days: number;
}

export interface ApiTokenCreated {
  token: ApiToken;
  /** 明文只在新建时给这一次 */
  plaintext: string;
}

export const tokensApi = {
  list: () => api.get<ApiToken[]>("/api/tokens"),
  create: (data: ApiTokenInput) => api.post<ApiTokenCreated>("/api/tokens", data),
  revoke: (id: number) => api.delete<{ ok: boolean }>(`/api/tokens/${id}`),
};

/** MCP 接入的开关与路径：关着时外部的 AI 客户端连不上（内置 AI 助手不受影响） */
export interface McpSettings {
  enabled: boolean;
  /** MCP 的路径（/mcp 这样） */
  path: string;
  default_path: string;
  /** 现在能连 MCP 的令牌个数（没过期、有 MCP 权限）；读不到时是 -1 */
  tokens: number;
  updated_at?: string;
}

export const mcpApi = {
  settings: () => api.get<McpSettings>("/api/mcp/settings"),
  save: (data: { enabled: boolean; path: string }) =>
    api.put<McpSettings>("/api/mcp/settings", data),
};

/** qB 兼容入口（路线图 M13）：监听地址是启动参数，这里只读 */
/** qB 兼容入口的一次登录：结果是 ok、bad_credentials（令牌不对）、scope（令牌没有 qB 兼容权限）、locked（失败太多次被锁了） */
export interface QbitCompatLogin {
  at: string;
  ip: string;
  username?: string;
  /** 用的令牌的名字（成功时） */
  token?: string;
  result: "ok" | "bad_credentials" | "scope" | "locked";
}

export interface QbitCompatStatus {
  /** 正在监听的地址；没在监听时是空的 */
  listen_addr: string;
  listening: boolean;
  /** 要监听的地址从哪来：flag 启动参数、env 环境变量、settings 这里设的；都没设时是空的 */
  listen_source: "" | "flag" | "env" | "settings";
  /** 要监听的地址；listen_error 是它监听失败的原因 */
  configured_addr: string;
  listen_error?: string;
  /** 这里存的监听地址（listen_source 是 flag、env 时不管用） */
  settings_addr: string;
  /** 0 表示默认下载器 */
  downloader_id: number;
  full_control: boolean;
  /** 实际用的下载器名字；没有可用的下载器时是空的 */
  downloader: string;
  /** 选的下载器为什么用不了（停用了、删掉了、没有启用的下载器） */
  downloader_issue?: string;
  compat_torrents: number;
  /** 登录着的客户端会话数；最近一次成功、失败的登录（重启以后没有） */
  sessions: number;
  last_login?: QbitCompatLogin;
  last_failure?: QbitCompatLogin;
}

export interface QbitCompatInput {
  downloader_id: number;
  full_control: boolean;
  /** 监听地址（地址:端口，空串是不开）；不带时不改 */
  listen_addr?: string;
}

export const qbitCompatApi = {
  get: () => api.get<QbitCompatStatus>("/api/qbit-compat"),
  save: (data: QbitCompatInput) => api.put<QbitCompatStatus>("/api/qbit-compat", data),
};

// ---- 远程访问（路线图 M15） ----

/** 设备的权限：只读是 app:read，完全控制是 app:read + app:write */
export type RemoteScope = "app:read" | "app:write";

export interface RemoteSettings {
  enabled: boolean;
  /** 自己填的 relay 地址（ws:// 或 wss://），最多 4 个；留空时用构建时内置的托管 relay */
  relays: string[];
  /** App 直连用的地址（http:// 或 https://）；空的时候配对链接里不带 */
  direct_url: string;
  /** 不使用 relay（只直连）；填过的 relay 地址留着 */
  relay_disabled: boolean;
}

export interface RemoteRelayStatus {
  url: string;
  state: "connecting" | "online" | "offline";
  error?: string;
  since: string;
  streams: number;
}

export interface RemoteOverview extends RemoteSettings {
  /** 第一次打开远程访问以后才有 */
  host_id?: string;
  host_key?: string;
  /** 构建时内置的托管 relay；没有时为空 */
  default_relay?: string;
  /** 实际在用的 relay：自己填的，或者没填时的托管 relay；关掉 relay 时为空 */
  active_relays: string[];
  relay_status: RemoteRelayStatus[];
  sessions: number;
  stream_path: string;
}

export interface RemoteDevice {
  id: number;
  name: string;
  scopes: RemoteScope[];
  created_at: string;
  last_seen_at?: string;
  last_seen_via?: "direct" | "relay" | "";
  revoked_at?: string;
  online: boolean;
  online_via?: "direct" | "relay";
}

export interface RemotePairing {
  /** pttools://pair?…，里面有一次性的配对密钥：只在这里给一次 */
  link: string;
  expires_at: string;
  scopes: RemoteScope[];
  host_id: string;
  /** 链接的二维码（SVG 文本） */
  qr_svg: string;
}

export interface RemotePairingStatus {
  state: "none" | "waiting" | "paired" | "expired" | "closed";
  expires_at?: string;
  scopes?: RemoteScope[];
  failures: number;
  device?: Omit<RemoteDevice, "online" | "online_via">;
}

export const remoteApi = {
  get: () => api.get<RemoteOverview>("/api/remote"),
  save: (data: RemoteSettings) => api.put<RemoteOverview>("/api/remote", data),
  rotateKeys: () => api.post<RemoteOverview>("/api/remote/keys/rotate"),
  startPairing: (data: { scopes: RemoteScope[]; direct_url: string }) =>
    api.post<RemotePairing>("/api/remote/pairings", data),
  pairingStatus: () => api.get<RemotePairingStatus>("/api/remote/pairings/current"),
  cancelPairing: () => api.delete<RemotePairingStatus>("/api/remote/pairings/current"),
  devices: () => api.get<RemoteDevice[]>("/api/remote/devices"),
  updateDevice: (id: number, data: { name?: string; scopes?: RemoteScope[] }) =>
    api.put<RemoteDevice>(`/api/remote/devices/${id}`, data),
  revokeDevice: (id: number) => api.post<RemoteDevice>(`/api/remote/devices/${id}/revoke`),
  deleteDevice: (id: number) => api.delete<{ ok: boolean }>(`/api/remote/devices/${id}`),
};

// AI 助手（路线图 M19）：设置只在网页上改（API Key 只写不读）；对话的进度经 SSE 推给页面
export interface AssistantSettings {
  enabled: boolean;
  /** 界面上选的服务商预设（deepseek、openai、custom 等） */
  provider: string;
  base_url: string;
  model: string;
  has_api_key: boolean;
  /** 密码换成了 *** */
  proxy_url: string;
  chatops: boolean;
  max_steps: number;
  confirm_minutes: number;
  /** 0 表示不清理 */
  retention_days: number;
  /** 上下文窗口（token），0 是自动 */
  context_window: number;
  /** 上下文用到 compact_percent% 时自动压缩 */
  auto_compact: boolean;
  compact_percent: number;
  /** 语音输入（Voltip）：访问令牌只写不读 */
  voice_base_url: string;
  has_voice_token: boolean;
  voice_model: string;
  voice_language: string;
}

/** 保存设置的参数：api_key、proxy_url 不给时不改，给空串时清除；换了接口地址又没给新 Key 时存着的 Key 清掉 */
export interface AssistantSettingsInput {
  enabled: boolean;
  provider: string;
  base_url: string;
  model: string;
  api_key?: string;
  proxy_url?: string;
  chatops: boolean;
  max_steps: number;
  confirm_minutes: number;
  retention_days?: number;
  /** 下面三项不给时不改；context_window 为 0 是自动 */
  context_window?: number;
  auto_compact?: boolean;
  compact_percent?: number;
  /** 语音输入：不给时不改；voice_token 给空串时清除，换了地址又没给新令牌时存着的清掉 */
  voice_base_url?: string;
  voice_token?: string;
  voice_model?: string;
  voice_language?: string;
}

/** 测试语音输入：Voltip 列出的处理方式（id 原样填进模型，name 是 Voltip 给的名字） */
export interface AssistantVoiceTest {
  models: { id: string; name: string }[];
  latency_ms: number;
}

export interface AssistantStatus {
  enabled: boolean;
  model: string;
  can_confirm: boolean;
  chatops: boolean;
  /** 配了语音输入（Voltip） */
  voice?: boolean;
}

export interface AssistantTestResult {
  model: string;
  reply: string;
  latency_ms: number;
}

/** 服务商 GET /models 列出的、能拿来对话的模型（total 是去掉不能对话的之前的数量） */
export interface AssistantModelList {
  models: string[];
  total: number;
}

export interface AssistantConversation {
  id: number;
  title: string;
  entry: "web" | "app" | "chatops" | string;
  running: boolean;
  created_at: string;
  updated_at: string;
}

export interface AssistantToolCall {
  id: string;
  name: string;
  /** 规范化脱敏以后的参数（JSON 文本） */
  arguments: string;
  write?: boolean;
}

export type AssistantRole = "user" | "assistant" | "tool" | "notice";

/** 推理等级（reasoning_effort）：空串是不指定 */
export type AssistantEffort = "" | "none" | "minimal" | "low" | "medium" | "high" | "xhigh";

/** 用户消息带的附件（内容经 assistantApi.attachmentURL 取） */
export interface AssistantAttachment {
  id: number;
  name: string;
  kind: "image" | "text";
  mime: string;
  size: number;
}

/** 对话页的模型下拉框：default 是设置里的模型（第一个）；error 是读服务商模型列表失败的原因 */
export interface AssistantChatModels {
  default: string;
  models: string[];
  error?: string;
}

export interface AssistantMessage {
  id: number;
  run_id?: string;
  role: AssistantRole;
  content: string;
  /** 模型的推理内容（思考过程），只有助手消息有 */
  reasoning?: string;
  /** 这一轮的模型与推理等级（没有时是设置里的模型、不指定） */
  model?: string;
  effort?: AssistantEffort;
  attachments?: AssistantAttachment[];
  tool_calls?: AssistantToolCall[];
  tool_name?: string;
  tool_call_id?: string;
  is_error?: boolean;
  created_at: string;
}

export type AssistantConfirmStatus = "pending" | "approved" | "denied" | "expired";

/** 写操作的一项改动：改以前、改以后（新增的没有 before，删掉的没有 after）；secret 是密钥，不写原文 */
export interface AssistantChange {
  label: string;
  before?: string;
  after?: string;
  secret?: boolean;
}

export interface AssistantConfirmation {
  id: number;
  run_id: string;
  tool_call_id: string;
  tool: string;
  summary: string;
  arguments: Record<string, unknown>;
  /** 会改什么、可能有什么影响（写工具的预览给的，没有时为空） */
  changes?: AssistantChange[];
  impact?: string[];
  status: AssistantConfirmStatus;
  /** 属主，或者 timeout、cancel、restart、shutdown */
  decided_by?: string;
  decided_at?: string;
  expires_at: string;
  created_at: string;
}

/** 上下文窗口从哪儿来的：设置里填的、服务商的模型列表、按模型名的内置表、默认 128K */
export type AssistantWindowSource = "setting" | "provider" | "table" | "default";

/**
 * 会话的上下文用量（单位都是 token）：context 是下一次请求大约要带的（estimated 为真时按字数估的），
 * window 是窗口，到 compact_percent% 自动压缩；input、output 是本会话所有调用服务商报的合计。
 */
export interface AssistantUsage {
  context: number;
  estimated?: boolean;
  window: number;
  window_source: AssistantWindowSource;
  auto_compact: boolean;
  compact_percent: number;
  input: number;
  output: number;
  compacted?: boolean;
}

export interface AssistantConversationDetail extends AssistantConversation {
  messages: AssistantMessage[];
  confirmations: AssistantConfirmation[];
  run_id?: string;
  partial?: string;
  /** 进行中的运行这一步到现在的推理内容 */
  partial_reasoning?: string;
  /** 读事件的 after：从它接着读不会漏 */
  last_seq: number;
  /** 上下文用量（读不出来时没有） */
  usage?: AssistantUsage;
}

export type AssistantEventType =
  | "run_started"
  | "delta"
  | "reasoning"
  | "message"
  | "tool_started"
  | "confirmation"
  | "run_finished"
  | "usage"
  | "compacting"
  | "reset";

export interface AssistantEvent {
  seq: number;
  type: AssistantEventType;
  run_id?: string;
  /** delta、reasoning 的文字 */
  text?: string;
  /** 为真时 text 是这一步到现在的全部文字，整段替换 */
  replace?: boolean;
  message?: AssistantMessage;
  tool?: AssistantToolCall;
  confirmation?: AssistantConfirmation;
  /** run_finished：done、canceled、failed */
  status?: "done" | "canceled" | "failed";
  error?: string;
  /** usage：新的上下文用量 */
  usage?: AssistantUsage;
  at?: string;
  /** reset：重新读会话以后从这里接着读 */
  last?: number;
}

export interface AssistantSendResult {
  run_id: string;
  message: AssistantMessage;
}

/** 发消息的选项：model 为空用设置里的模型，effort 为空不指定，attachment_ids 是先上传的附件 */
export interface AssistantSendOptions {
  model?: string;
  effort?: AssistantEffort;
  attachment_ids?: number[];
}

/** 把一段 WAV 录音交给 Voltip 识别（经 pt-tools，令牌不出后端）；失败时抛 ApiError（正文是原因） */
async function transcribeVoice(wav: Blob, language?: string): Promise<string> {
  const q = language ? `?language=${encodeURIComponent(language)}` : "";
  const res = await fetch(`/api/assistant/transcribe${q}`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "audio/wav" },
    body: wav,
  });
  if (!res.ok) {
    const msg = await res.text();
    throw new ApiError(res.status, msg.trim() || `HTTP ${res.status}`);
  }
  return ((await res.json()) as { text: string }).text;
}

/** 上传一个附件（还没发的只有自己看得到）；失败时抛 ApiError（正文是原因） */
async function uploadAssistantAttachment(file: File): Promise<AssistantAttachment> {
  const form = new FormData();
  form.append("file", file, file.name);
  const res = await fetch("/api/assistant/attachments", {
    method: "POST",
    credentials: "same-origin",
    body: form,
  });
  if (!res.ok) {
    const msg = await res.text();
    throw new ApiError(res.status, msg || `HTTP ${res.status}`);
  }
  return (await res.json()) as AssistantAttachment;
}

export const assistantApi = {
  settings: () => api.get<AssistantSettings>("/api/assistant/settings"),
  saveSettings: (data: AssistantSettingsInput) =>
    api.put<AssistantSettings>("/api/assistant/settings", data),
  testSettings: (data: AssistantSettingsInput) =>
    api.post<AssistantTestResult>("/api/assistant/settings/test", data),
  listModels: (data: AssistantSettingsInput) =>
    api.post<AssistantModelList>("/api/assistant/settings/models", data),
  /** 按输入（还没保存的也行）读 Voltip 的模型列表，看语音输入能不能用 */
  voiceTest: (data: AssistantSettingsInput) =>
    api.post<AssistantVoiceTest>("/api/assistant/settings/voice-test", data),
  transcribe: transcribeVoice,
  status: () => api.get<AssistantStatus>("/api/assistant/status"),
  conversations: () => api.get<AssistantConversation[]>("/api/assistant/conversations"),
  create: (data: { title?: string; message?: string } & AssistantSendOptions) =>
    api.post<{ conversation: AssistantConversation; run?: AssistantSendResult }>(
      "/api/assistant/conversations",
      data,
    ),
  conversation: (id: number) =>
    api.get<AssistantConversationDetail>(`/api/assistant/conversations/${id}`),
  remove: (id: number) => api.delete<{ ok: boolean }>(`/api/assistant/conversations/${id}`),
  send: (id: number, text: string, opts: AssistantSendOptions = {}) =>
    api.post<AssistantSendResult>(`/api/assistant/conversations/${id}/messages`, {
      text,
      ...opts,
    }),
  /** 对话页的模型下拉框（服务商的模型列表缓存 10 分钟） */
  models: () => api.get<AssistantChatModels>("/api/assistant/models"),
  upload: uploadAssistantAttachment,
  /** 删掉一个还没发的附件 */
  removeAttachment: (id: number) => api.delete<{ ok: boolean }>(`/api/assistant/attachments/${id}`),
  /** 附件的内容：图片在页面上显示，文本文件下载 */
  attachmentURL: (id: number) => `/api/assistant/attachments/${id}`,
  cancel: (id: number) => api.post<{ ok: boolean }>(`/api/assistant/conversations/${id}/cancel`),
  decide: (id: number, approve: boolean) =>
    api.post<AssistantConfirmation>(`/api/assistant/confirmations/${id}`, { approve }),
  /** SSE 地址：浏览器断线重连时自己带 Last-Event-ID */
  eventsURL: (id: number, after: number) =>
    `/api/assistant/conversations/${id}/events?after=${after}`,
};

// ---- 首页与系统体检（媒体中心 M8） ----

/** 一拍全部下载器合成的总速度（字节/秒）；answered < total 时这一拍有下载器没取到 */
export interface SpeedSample {
  at: string;
  down: number;
  up: number;
  answered: number;
  total: number;
}

/** 首页的一块：data 是这一块的数据，error 是没取到的原因（别的块照样有） */
export interface HomeBlock<T> {
  data: T | null;
  error?: string;
}

/** 今天播出的一项：日历的一项，加上卡片要写的 */
export interface HomeTodayItem extends CalendarEntry {
  /** 没有剧照时用的背景图（这一部的） */
  backdrop_path?: string;
  /** 已入库的版本（例如 2160p HHWEB） */
  version?: string;
  /** 下载中的站点、进度（0–100）与预计还要多久（秒） */
  site?: string;
  percent?: number;
  eta_seconds?: number;
  /** 等版本时锁住的版本（例如 HHWEB 2160p） */
  lock?: string;
  /** 在媒体服务器里打开这一集（电影是这一部）；服务器里没有时不给 */
  open_url?: string;
}

/** 今天播出与这周还要播（明天起 6 天）；errors 是没读到 TMDB 的订阅 */
export interface HomeCalendar {
  today: HomeTodayItem[];
  week: CalendarEntry[];
  errors: string[];
  /** 订阅的个数：一个都没有时写「还没有追剧」 */
  subscriptions: number;
}

export type HomeTodoKind =
  | "organize_failed"
  | "recognize"
  | "waiting_version"
  | "attendance_failed"
  | "login_failed"
  | "server_removed"
  | "library_missing"
  | "backup"
  | "health";

export type HomeTone = "dang" | "warn" | "primary" | "info";

/** 待处理的一类：target 是去哪个页面（带着筛选），drawer 是在首页的抽屉里处理 */
export interface HomeTodoGroup {
  kind: HomeTodoKind;
  count: number;
  label: string;
  target: string;
  tone: HomeTone;
}

/** 待处理抽屉里的一行；issue_ids 是媒体服务器里删了的与库文件不见了的问题 */
export interface HomeTodoItem {
  kind: HomeTodoKind;
  title: string;
  detail?: string;
  target?: string;
  issue_ids?: number[];
  media_type?: MediaKind;
  tmdb_id?: number;
  season?: number;
  episode?: number;
  /** 问题说的那几集（剧集；有说整季的问题时没有，删除弹窗按整季） */
  episodes?: number[];
}

export interface HomeTodo {
  groups: HomeTodoGroup[];
  items: HomeTodoItem[];
}

/** 最近入库的一项：一部电影，或剧的一季（写最新那一集）；source 是 pt-tools 整理的或服务器里新加的 */
export interface HomeRecentItem {
  media_type: MediaKind;
  tmdb_id: number;
  title: string;
  season?: number;
  episode?: number;
  poster_path?: string;
  quality?: string;
  source: "pt-tools" | "server";
  at: string;
}

export interface HomeRecent {
  today_count: number;
  items: HomeRecentItem[];
}

export interface HomeDownloader {
  id: number;
  name: string;
  type: string;
  online: boolean;
  down: number;
  up: number;
  downloading: number;
  seeding: number;
  error?: string;
}

export interface HomeDownloaders {
  downloaders: HomeDownloader[];
  down: number;
  up: number;
  downloading: number;
  seeding: number;
  speed_history: SpeedSample[];
}

/** 存储里的一块盘：total 为 0 是不知道总量；low 同体检的警告线（剩不到 5% 或 20 GB） */
export interface HomeDisk {
  kind: "download" | "library";
  label: string;
  path: string;
  free: number;
  total: number;
  low: boolean;
  error?: string;
}

export interface HomeHealthLine {
  at: string;
  ok: number;
  warn: number;
  fail: number;
  skip: number;
  /** 第一项失败（没有时第一项提醒）的「标题：说明」 */
  problem?: string;
}

export interface HomeBackupLine {
  enabled: boolean;
  last_at?: string;
  last_ok_at?: string;
  error?: string;
  late: boolean;
  /** 最近一次成功距今几天；-1 是没成功过 */
  days: number;
}

export interface HomeServerLine {
  id: number;
  name: string;
  kind: MediaServerKind;
  version?: string;
  online: boolean;
  error?: string;
  indexed_at?: string;
  index_error?: string;
  movies: number;
  series: number;
  episodes: number;
  open_url?: string;
}

export interface HomeStatus {
  health: HomeHealthLine | null;
  backup: HomeBackupLine | null;
  servers: HomeServerLine[];
}

/** 首页（GET /api/home）：每块单独给数据或错误 */
export interface HomeData {
  at: string;
  calendar: HomeBlock<HomeCalendar>;
  todo: HomeBlock<HomeTodo>;
  resume: HomeBlock<MediaResumeGroup[]>;
  recent: HomeBlock<HomeRecent>;
  downloaders: HomeBlock<HomeDownloaders>;
  storage: HomeBlock<HomeDisk[]>;
  status: HomeBlock<HomeStatus>;
  /** NAS 概览（首页 NAS 概览 H2）：首页显示的那个来源与其余能用的来源 */
  nas: HomeBlock<HomeNas>;
}

export const homeApi = {
  /** refresh 为真时不用服务端 30 秒的缓存 */
  get: (refresh = false) => api.get<HomeData>(refresh ? "/api/home?refresh=1" : "/api/home"),
};

/*
 * NAS 监控（首页 NAS 概览 H2）：本机来源与接入的 NAS 来源的状态、最新数据与最近的几拍；来源的保存、测试连接与设置。
 * 数据都在 pt-tools 的内存里（30 秒一拍），GET /api/nas 不对外发请求。
 */

/** 一个来源现在的情况 */
export type NasState =
  | "ok"
  | "connecting"
  | "error"
  | "auth_failed"
  | "cert_untrusted"
  | "cert_changed"
  | "disabled"
  | "unsupported";

/** 快照里各类数据的键：来源读不到的那类按 facets 写原因，不把没有当成 0 */
export type NasFacetKey =
  | "system"
  | "cpu"
  | "cpu_temp"
  | "memory"
  | "network"
  | "pools"
  | "disks"
  | "alerts"
  | "apps";

export interface NasFacet {
  status: "ok" | "unsupported" | "denied" | "error";
  reason?: string;
}

export interface NasSystem {
  hostname: string;
  platform: string;
  version: string;
  model: string;
  cores: number;
  uptime_seconds: number;
}

/** 处理器：percent、iowait 是百分比（0–100），温度读不到时没有 */
export interface NasCPU {
  percent: number;
  iowait?: number;
  load1: number;
  load5: number;
  load15: number;
  temp_c?: number;
}

/** 内存（字节） */
export interface NasMemory {
  total: number;
  used: number;
  available: number;
}

/** 各网卡合计的速度（字节/秒） */
export interface NasNetwork {
  rx_bps: number;
  tx_bps: number;
  interfaces: string[] | null;
}

/** 存储池：size、used、free 是可用容量（字节）；healthy 读不到时没有 */
export interface NasPool {
  name: string;
  status: string;
  healthy?: boolean;
  size: number;
  used: number;
  free: number;
  disks: number;
}

export type NasDiskKind = "hdd" | "ssd" | "nvme" | "unknown";

export interface NasDisk {
  name: string;
  model: string;
  kind: NasDiskKind;
  size: number;
  temp_c?: number;
  pool?: string;
}

export interface NasAlert {
  level: "critical" | "warning" | "info";
  title: string;
  at?: string;
}

export interface NasApps {
  running: number;
  total: number;
  upgradable: number;
  /** 来源不知道哪些能更新（Glances 只看得到容器）；为真时 upgradable 不算数 */
  updates_unknown?: boolean;
}

export interface NasSnapshot {
  at: string;
  system: NasSystem;
  cpu: NasCPU;
  memory: NasMemory;
  network: NasNetwork;
  pools: NasPool[] | null;
  disks: NasDisk[] | null;
  alerts: NasAlert[] | null;
  apps: NasApps;
  facets: Partial<Record<NasFacetKey, NasFacet>> | null;
}

/** 历史的一拍：cpu、mem 是百分比，rx、tx 是字节/秒 */
export interface NasSample {
  at: string;
  cpu: number;
  mem: number;
  rx: number;
  tx: number;
}

/** 一个来源（本机的 id 是 local，接入的来源是数字编号）；不回密钥，只回有没有 */
export interface NasSource {
  id: string;
  name: string;
  kind: string;
  enabled: boolean;
  experimental: boolean;
  url?: string;
  has_secret: boolean;
  tls_pin_sha256?: string;
  insecure_ack: boolean;
  state: NasState;
  message?: string;
  last_ok_at?: string;
  next_try_at?: string;
  snapshot: NasSnapshot | null;
  history: NasSample[];
}

/**
 * 能接入的一种来源：username 是用户名一栏，secret 是要不要密钥（with_username：填了用户名才要），
 * plain 是 http:// 地址的规则（never：只收 https；without_credentials：不带账号密码时可以；with_ack：要确认明文风险）。
 * verified 是实验性的类型在哪个版本的真机上验证过（例如「fnOS 1.2.1000」），没验证过时不带。
 */
export interface NasKind {
  name: string;
  label: string;
  experimental: boolean;
  verified?: string;
  username: "none" | "optional" | "required";
  secret: "required" | "with_username" | "none";
  plain: "never" | "without_credentials" | "with_ack";
}

/** GET /api/nas：home 是首页按设置选出的来源（空是没有能显示的） */
export interface NasView {
  home_source: string;
  home: string;
  local_enabled: boolean;
  local_supported: boolean;
  kinds: NasKind[];
  sources: NasSource[];
}

/** 编辑一个来源用的字段：不回密钥与长期凭据，只回有没有 */
export interface NasSourceEdit {
  id: number;
  name: string;
  kind: string;
  enabled: boolean;
  url: string;
  username: string;
  has_secret: boolean;
  has_extra: boolean;
  tls_pin_sha256: string;
  insecure_ack: boolean;
}

/** 保存一个来源：secret 为空表示不改（只在类型、地址、账号都没变时）；pending 是测试连接回的暂存凭据 */
export interface NasSourceInput {
  name: string;
  kind: string;
  enabled: boolean;
  url: string;
  username: string;
  secret?: string;
  clear_secret?: boolean;
  tls_pin_sha256: string;
  insecure_ack: boolean;
  pending?: string;
}

/** 测试连接：id 是编辑中的来源（新建时不给），trust_sha256 是这一次确认信任的证书指纹 */
export interface NasTestInput {
  id?: number;
  kind: string;
  url: string;
  username?: string;
  secret?: string;
  clear_secret?: boolean;
  otp?: string;
  trust_sha256?: string;
  insecure_ack?: boolean;
}

export interface NasCertInfo {
  sha256: string;
  subject: string;
  issuer: string;
  not_before: string;
  not_after: string;
  dns_names: string[] | null;
}

/** 测试连接的结果：certificate 是证书不受信任（还没发出凭据），otp_required 是要验证码 */
export interface NasTestResult {
  status: "ok" | "certificate" | "otp_required" | "auth_failed" | "error";
  system?: NasSystem;
  certificate?: NasCertInfo;
  message?: string;
  pending?: string;
}

/** 首页的 nas 块：首页显示的那个来源（没有时为空）与其余能用的来源 */
export interface HomeNas {
  source: NasSource | null;
  others: { id: string; name: string; kind: string; state: NasState }[];
}

export const nasApi = {
  /** 各来源的状态与最新数据（只读 pt-tools 的内存）；signal 用于轮询时放弃太慢的一次 */
  get: (signal?: AbortSignal) => api.get<NasView>("/api/nas", { signal }),
  getSource: (id: number) => api.get<NasSourceEdit>(`/api/nas/sources/${id}`),
  create: (input: NasSourceInput) => api.post<NasSourceEdit>("/api/nas/sources", input),
  update: (id: number, input: NasSourceInput) =>
    api.put<NasSourceEdit>(`/api/nas/sources/${id}`, input),
  remove: (id: number) => api.delete<{ ok: boolean }>(`/api/nas/sources/${id}`),
  test: (input: NasTestInput) => api.post<NasTestResult>("/api/nas/sources/test", input),
  /** 重新连接（只试一次）：id 是 local 或数字编号 */
  retry: (id: string) => api.post<NasSource>(`/api/nas/sources/${id}/retry`),
  settings: (input: { local_enabled?: boolean; home_source?: string }) =>
    api.put<NasView>("/api/nas/settings", input),
};

export type HealthStatus = "ok" | "warn" | "fail" | "skip";

export type HealthGroup =
  | "tmdb"
  | "sites"
  | "downloaders"
  | "servers"
  | "paths"
  | "libraries"
  | "disks"
  | "nas"
  | "time"
  | "backup"
  | "secret";

/** 体检的一项：hint 是怎么修（没问题时没有） */
export interface HealthItem {
  key: string;
  group: HealthGroup;
  title: string;
  status: HealthStatus;
  detail?: string;
  hint?: string;
}

export interface HealthReport {
  at: string;
  duration_ms: number;
  items: HealthItem[];
  ok: number;
  warn: number;
  fail: number;
  skip: number;
}

export const systemCheckApi = {
  /** 上一次体检的结果；没体检过时 report 为 null */
  last: () => api.get<{ report: HealthReport | null }>("/api/system/check"),
  /** 马上体检一次（最多 60 秒；跑着的时候再点等它跑完） */
  run: () => api.post<{ report: HealthReport }>("/api/system/check"),
};
