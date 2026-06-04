import type { KnownSite } from "./constants";

export type SiteSchema =
  | "NexusPHP"
  | "mTorrent"
  | "Gazelle"
  | "Unit3D"
  | "HDDolby"
  | "Rousi"
  | "Unknown";

export type AuthMethod = "cookie" | "api_key" | "cookie_and_api_key" | "passkey";

export type PageType =
  | "search"
  | "detail"
  | "userinfo"
  | "index"
  | "bonus"
  | "api_response"
  | "unknown";

export type SiteMode = "known" | "unknown" | "none";

export interface SiteInfo {
  name: string;
  url: string;
  schema: SiteSchema;
  authMethod: AuthMethod;
}

export interface KnownSiteStatus {
  site: KnownSite;
  cookieStatus: "valid" | "expiring" | "expired" | "missing";
  cookieExpireDays: number | null;
  hasCookie: boolean;
  lastSync: string | null;
  autoSync: boolean;
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
  html: string;
  capturedAt: string;
  detectedSchema: SiteSchema;
}

export interface CollectionSession {
  id: string;
  site: SiteInfo;
  pages: CapturedPage[];
  createdAt: string;
  status: "collecting" | "complete" | "exported";
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
  | "EXPORT_ZIP"
  | "CREATE_ISSUE"
  | "GET_STATUS"
  | "STATUS_UPDATE";

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
}

export interface BatchSyncSitesPayload {
  siteIds: string[];
}

export interface BatchSyncResult {
  synced: string[];
  failed: Array<{ siteId: string; error: string }>;
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
  EXPORT_ZIP: ExportZipPayload;
  CREATE_ISSUE: CreateIssuePayload;
  GET_STATUS: Record<string, never>;
  STATUS_UPDATE: StatusPayload | TabSiteStatus;
}

export interface MessageSender {
  tabId?: number;
  frameId?: number;
  url?: string;
}
