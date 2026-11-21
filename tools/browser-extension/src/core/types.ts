import type { KnownSite } from "./constants";

export type SiteSchema =
  | "NexusPHP"
  | "mTorrent"
  | "Gazelle"
  | "Unit3D"
  | "HDDolby"
  | "Rousi"
  | "IPTorrents"
  | "Unknown";

export type AuthMethod = "cookie" | "api_key" | "cookie_and_api_key" | "passkey";

export type PageType =
  | "search"
  | "search_free"
  | "detail"
  | "detail_nopromo"
  | "userinfo"
  | "hr"
  | "index"
  | "bonus"
  | "api_response"
  | "rules"
  | "faq"
  | "rss"
  /** 不带 Cookie 再取一次首页：Cookie 失效（未登录）时站点回的页面 */
  | "logged_out"
  /** 本人的做种列表（接口回的片段或 JSON） */
  | "seeding"
  /** 页面引用的同源脚本里提到签到、做种列表、优惠状态的（可以有多个） */
  | "script"
  /** 后台标签页里页面脚本跑完以后的 DOM */
  | "rendered_search"
  | "rendered_detail"
  | "rendered_userinfo"
  /** 后台标签页里页面脚本发出的同源请求 */
  | "requests_search"
  | "requests_detail"
  | "requests_userinfo"
  /** 下载种子回的是网页（第一次下载的提示、登录页） */
  | "download_notice"
  | "unknown";

/** 采集内容的格式，决定导出的扩展名与脱敏方式；没写时按 html */
export type PageFormat = "html" | "js" | "xml" | "json";

export type SiteMode = "known" | "unknown" | "none";

export interface SiteInfo {
  name: string;
  url: string;
  schema: SiteSchema;
  authMethod: AuthMethod;
}

export interface KnownSiteStatus {
  site: KnownSite;
  /** unverified：有像登录用的 Cookie、格式认不出，同步时由 pt-tools 先验证再保存（见 modules/sync/cookie-reader） */
  cookieStatus: "valid" | "expiring" | "expired" | "unverified" | "missing";
  cookieExpireDays: number | null;
  hasCookie: boolean;
  lastSync: string | null;
  autoSync: boolean;
  /**
   * pt-tools 验证后拒绝保存**现在这组** Cookie 的原因（pt-tools 写给用户的原话）；没被拒绝、Cookie 变了或换了 pt-tools 地址时为空。
   * 上次存下的站点状态里没有这一项（旧版本存的）。
   */
  syncRejected?: string | null;
}

export interface UnknownSiteStatus {
  detectedSchema: SiteSchema;
  pageType: PageType;
  url: string;
}

export interface TabSiteStatus {
  mode: SiteMode;
  known?: KnownSiteStatus;
  unknown?: UnknownSiteStatus;
}

export interface CapturedPage {
  pageType: PageType;
  url: string;
  /** 采到的内容（脱敏后）；脚本、RSS、JSON 也放在这里，format 说明是什么 */
  html: string;
  capturedAt: string;
  detectedSchema: SiteSchema;
  /** 没写时是 html（旧版本存下的页面都没有这一项） */
  format?: PageFormat;
}

/** 做种列表实际用的请求：只记方法、路径和参数名，不记参数值 */
export interface SeedingRequestInfo {
  /** GET、POST（点「当前做种」时记到的照页面实际发的写） */
  method: string;
  path: string;
  params: string[];
  /** upstream：上游通用接口；script：照页面脚本里写的；click：在后台标签页点「当前做种」时页面发的 */
  source: "upstream" | "script" | "click";
  status?: number;
  contentType?: string;
}

/** 页面上的签到入口：只记元素的摘要，扩展从不请求签到地址 */
export interface SigninEntry {
  page: PageType;
  tag: string;
  text: string;
  href?: string;
  onclick?: string;
  id?: string;
  className?: string;
}

/** 采到的脚本：地址、大小、是否截断、命中的关键词 */
export interface CollectedScript {
  url: string;
  bytes: number;
  truncated: boolean;
  keywords: string[];
}

/** 一键采集记下的站点信息，导出时写进 site-info.json（都已脱敏，不含 Cookie 值） */
export interface CollectedFacts {
  /** 站点 Cookie 的名字（只有名字，没有值）；没有 Cookie 权限时为 null */
  cookieNames?: string[] | null;
  /** 种子文件里 announce、announce-list 的主机名 */
  trackerHosts?: string[];
  seedingRequest?: SeedingRequestInfo | null;
  /** 试过的做种列表请求（方法、路径、参数名、状态码） */
  seedingAttempts?: SeedingRequestInfo[];
  signinEntries?: SigninEntry[];
  /** 脚本里出现的同源 .php 接口 */
  scriptEndpoints?: string[];
  scripts?: CollectedScript[];
  /** 不带 Cookie 取首页时的最终地址与状态码 */
  loggedOut?: {
    requestedUrl: string;
    finalUrl: string;
    status: number;
    redirected: boolean;
  } | null;
  /** 采集时给用户看的提示（缺了哪些样本） */
  warnings?: string[];
}

/** 登录用户自己的用户名与 ID：只存在本地，导出时用来把它们换成示例值，本身不导出 */
export interface UserIdentity {
  username?: string | null;
  userId?: string | null;
}

export interface CollectionSession {
  id: string;
  site: SiteInfo;
  pages: CapturedPage[];
  createdAt: string;
  status: "collecting" | "complete" | "exported";
  /** 一键采集记下的站点信息；旧版本存下的会话没有 */
  facts?: CollectedFacts;
  /** 只在本地，不导出 */
  identity?: UserIdentity;
}

export interface PtToolsConnection {
  baseUrl: string;
  sessionId: string;
  connected: boolean;
  lastSync: string | null;
}

export interface SiteCookieData {
  siteName: string;
  domain: string;
  cookies: string;
  capturedAt: string;
}

/** SyncSchedule 是定时同步的设置。 */
export interface SyncSchedule {
  enabled: boolean;
  /** intervalMinutes 在 MIN_SYNC_INTERVAL 与 MAX_SYNC_INTERVAL 之间 */
  intervalMinutes: number;
  /** allSites 为 true 时同步 pt-tools 里已启用的全部 Cookie 站点；false 时只同步打开了「自动同步」的站点 */
  allSites: boolean;
}

/** SyncRunSummary 是一次定时（或「立即同步一次」）的结果。 */
export interface SyncRunSummary {
  at: string;
  trigger: "schedule" | "manual";
  ptTools: {
    synced: string[];
    unchanged: number;
    failed: Array<{ siteId: string; error: string }>;
    /**
     * missing 是要同步、但浏览器里没有可用 Cookie 的站点名（KnownSite.name；没登录、Cookie 不全或过期了），这些站点没有推。
     * 这一项以前存下的结果里没有，读的时候按空列表算。
     */
    missing?: string[];
    /** skipped 是这次没同步 pt-tools 的原因（没连上、没有要同步的站点） */
    skipped?: string;
    /** error 是这一轮同步了、但没能照设置做全的原因（例如读不到 pt-tools 的站点列表） */
    error?: string;
  };
  cookieCloud: {
    uploaded: boolean;
    domains: number;
    cookies: number;
    error?: string;
    skipped?: string;
  };
}

/** CookieCloudSettingsView 是弹窗看到的 CookieCloud 设置：不带密码，只说有没有设置。 */
export interface CookieCloudSettingsView {
  enabled: boolean;
  serverUrl: string;
  uuid: string;
  cryptoType: "aes-128-cbc-fixed" | "legacy";
  hasPassword: boolean;
  lastUploadAt: string | null;
  domains: number;
  cookies: number;
  lastError: string | null;
  lastErrorAt: string | null;
}

export interface SyncSettingsView {
  schedule: SyncSchedule;
  cookieCloud: CookieCloudSettingsView;
  lastRun: SyncRunSummary | null;
}

export interface SaveCookieCloudPayload {
  enabled: boolean;
  serverUrl: string;
  uuid: string;
  cryptoType: "aes-128-cbc-fixed" | "legacy";
  /** password 不传时保留原来的 */
  password?: string;
}

export type MessageType =
  | "CAPTURE_PAGE"
  | "PAGE_CAPTURED"
  | "DETECT_SITE"
  | "SITE_DETECTED"
  | "SYNC_COOKIES"
  | "SYNC_SITE_COOKIES"
  | "BATCH_SYNC_SITES"
  | "BATCH_OPEN_TABS"
  | "CHECK_EXTENSION_UPDATE"
  | "AUTO_COLLECT"
  | "COOKIES_SYNCED"
  | "TOGGLE_AUTO_SYNC"
  | "GET_TAB_STATUS"
  | "GET_ALL_SITES_STATUS"
  | "GET_SITE_LOGIN_STATE"
  | "CHECK_CONNECTION"
  | "EXPORT_ZIP"
  | "CREATE_ISSUE"
  | "GET_STATUS"
  | "STATUS_UPDATE"
  | "GET_SYNC_SETTINGS"
  | "SAVE_SYNC_SCHEDULE"
  | "RUN_SYNC_NOW"
  | "SAVE_COOKIECLOUD"
  | "TEST_COOKIECLOUD"
  | "UPLOAD_COOKIECLOUD";

export interface Message<T = unknown> {
  type: MessageType;
  payload: T;
  timestamp: number;
}

export interface CapturePagePayload {
  sessionId?: string;
}

export interface SiteDetectedPayload {
  mode: SiteMode;
  knownSiteId?: string;
  detectedSchema?: SiteSchema;
  pageType: PageType;
  url: string;
  tabId?: number;
}

export interface SyncSiteCookiesPayload {
  siteId: string;
}

/**
 * PUT /api/sites/{id} payload sent by syncSiteCredential.
 * `cookie` / `api_key` / `passkey` is keyed dynamically by `KnownSite.syncField`.
 * `last_visit_at` is an optional RFC3339 ISO timestamp pulled from
 * `getLastVisitMap()` and forwarded so the backend can refresh
 * `site_login_state.last_visit_at` in the same request.
 */
export interface SyncSiteCredentialPayload {
  cookie?: string;
  api_key?: string;
  passkey?: string;
  last_visit_at?: string;
  /** true：不改站点的启用状态，停用的站点只存凭据（定时同步推没启用的站点时用） */
  keep_enabled?: boolean;
  /** true：pt-tools 先拿这组 Cookie 实际登录一次，登录上了才保存（格式认不出的 Cookie 用） */
  verify_cookie?: boolean;
}

export interface BatchSyncSitesPayload {
  siteIds: string[];
}

export interface BatchSyncResult {
  synced: string[];
  failed: Array<{ siteId: string; error: string }>;
  /** 同步成功、但 pt-tools 说还要处理的站点（比如 HDDolby 还缺 API Key，Cookie 已存下、站点暂不启用） */
  warnings?: Array<{ siteId: string; warning: string }>;
}

export interface BatchOpenTabsPayload {
  siteIds: string[];
  timeoutMs?: number;
}

export interface BatchOpenTabsResult {
  ok: string[];
  failed: Array<{ siteId: string; reason: string }>;
  skipped: string[];
}

export interface ExtensionUpdateCheckResult {
  currentVersion: string;
  status: "checking" | "no_update" | "update_available" | "throttled" | "unavailable";
  releaseUrl: string;
}

export interface AutoCollectPayload {
  siteOrigin: string;
  schema: SiteSchema;
}

export interface ToggleAutoSyncPayload {
  siteId: string;
  enabled: boolean;
}

export interface SyncCookiesPayload {
  baseUrl: string;
  username?: string;
  password?: string;
}

export interface CookiesSyncedPayload {
  synced: number;
  failed: Array<{ siteName: string; error: string }>;
  syncedAt: string;
}

export interface ExportZipPayload {
  sessionId: string;
}

export interface CreateIssuePayload {
  sessionId: string;
}

export interface StatusPayload {
  activeSession: CollectionSession | null;
  pageCount: number;
  currentSite: SiteInfo | null;
  connection: PtToolsConnection;
}

export interface MessagePayloadMap {
  CAPTURE_PAGE: CapturePagePayload;
  PAGE_CAPTURED: CapturedPage;
  DETECT_SITE: Record<string, never>;
  SITE_DETECTED: SiteDetectedPayload;
  SYNC_COOKIES: SyncCookiesPayload;
  SYNC_SITE_COOKIES: SyncSiteCookiesPayload;
  BATCH_SYNC_SITES: BatchSyncSitesPayload;
  BATCH_OPEN_TABS: BatchOpenTabsPayload;
  CHECK_EXTENSION_UPDATE: Record<string, never>;
  AUTO_COLLECT: AutoCollectPayload;
  COOKIES_SYNCED: CookiesSyncedPayload;
  TOGGLE_AUTO_SYNC: ToggleAutoSyncPayload;
  GET_TAB_STATUS: Record<string, never>;
  GET_ALL_SITES_STATUS: Record<string, never>;
  GET_SITE_LOGIN_STATE: Record<string, never>;
  CHECK_CONNECTION: Record<string, never>;
  EXPORT_ZIP: ExportZipPayload;
  CREATE_ISSUE: CreateIssuePayload;
  GET_STATUS: Record<string, never>;
  STATUS_UPDATE: StatusPayload | TabSiteStatus;
  GET_SYNC_SETTINGS: Record<string, never>;
  SAVE_SYNC_SCHEDULE: SyncSchedule;
  RUN_SYNC_NOW: Record<string, never>;
  SAVE_COOKIECLOUD: SaveCookieCloudPayload;
  TEST_COOKIECLOUD: Record<string, never>;
  UPLOAD_COOKIECLOUD: { overwrite?: boolean };
}

export interface MessageSender {
  tabId?: number;
  frameId?: number;
  url?: string;
}

/**
 * RejectedPush 是 pt-tools 验证后拒绝保存的那组 Cookie：指纹（同 pushFingerprint，不是 Cookie 本身）和 pt-tools 给的原因。
 * 指纹没变就不再推（不再让 pt-tools 去访问站点），原因照样写进同步结果。
 */
export interface RejectedPush {
  hash: string;
  reason: string;
}
