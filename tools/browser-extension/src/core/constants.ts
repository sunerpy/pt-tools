import type { AuthMethod, PageType, SiteSchema } from "./types";

export interface KnownSite {
  id: string;
  name: string;
  domains: string[];
  schema: SiteSchema;
  authMethod: AuthMethod;
  cookieNames: string[];
  syncField: "cookie" | "api_key" | "passkey";
}

export const KNOWN_SITES: KnownSite[] = [
  {
    id: "hdsky",
    name: "HDSky",
    domains: ["hdsky.me"],
    schema: "NexusPHP",
    authMethod: "cookie",
    cookieNames: ["c_secure_uid", "c_secure_pass", "c_secure_tracker_ssl"],
    syncField: "cookie",
  },
  {
    id: "springsunday",
    name: "SpringSunday",
    domains: ["springsunday.net"],
    schema: "NexusPHP",
    authMethod: "cookie",
    cookieNames: ["c_secure_uid", "c_secure_pass", "c_secure_tracker_ssl"],
    syncField: "cookie",
  },
  {
    id: "mteam",
    name: "M-Team",
    domains: [
      "m-team.cc",
      "m-team.io",
      "kp.m-team.cc",
      "xp.m-team.cc",
      "ap.m-team.cc",
      "zp.m-team.io",
      "ob.m-team.cc",
      "next.m-team.cc",
    ],
    schema: "mTorrent",
    authMethod: "api_key",
    cookieNames: [],
    syncField: "api_key",
  },
  {
    id: "hddolby",
    name: "HDDolby",
    domains: ["www.hddolby.com"],
    schema: "HDDolby",
    authMethod: "cookie",
    cookieNames: ["c_secure_uid", "c_secure_pass", "c_secure_tracker_ssl", "c_secure_login"],
    syncField: "cookie",
  },
  {
    id: "novahd",
    name: "NovaHD",
    domains: ["pt.novahd.top"],
    schema: "NexusPHP",
    authMethod: "cookie",
    cookieNames: ["c_secure_uid", "c_secure_pass", "c_secure_tracker_ssl"],
    syncField: "cookie",
  },
  {
    id: "rousipro",
    name: "Rousi Pro",
    domains: ["rousi.pro"],
    schema: "Rousi",
    authMethod: "passkey",
    cookieNames: [],
    syncField: "passkey",
  },
];

export const PAGE_PATTERNS: Array<{ pattern: RegExp; pageType: PageType }> = [
  { pattern: /torrents\.php|browse\.php/i, pageType: "search" },
  { pattern: /details\.php\?id=/i, pageType: "detail" },
  { pattern: /userdetails\.php\?id=/i, pageType: "userinfo" },
  { pattern: /login\.php|signup\.php/i, pageType: "index" },
  { pattern: /upload\.php/i, pageType: "index" },
  { pattern: /\/index\.php$|\/$/i, pageType: "index" },
  { pattern: /mybonus\.php/i, pageType: "bonus" },
  { pattern: /\/api\//i, pageType: "api_response" },
];

export const SENSITIVE_PATTERNS: Array<{ pattern: RegExp; replacement: string }> = [
  { pattern: /passkey=[a-f0-9]{16,}/gi, replacement: "passkey=REMOVED" },
  { pattern: /PHPSESSID=[a-f0-9]+/gi, replacement: "PHPSESSID=REMOVED" },
  { pattern: /c_secure_uid=[^;]+/gi, replacement: "c_secure_uid=REMOVED" },
  { pattern: /c_secure_pass=[^;]+/gi, replacement: "c_secure_pass=REMOVED" },
  { pattern: /c_secure_tracker_ssl=[^;]+/gi, replacement: "c_secure_tracker_ssl=REMOVED" },
  { pattern: /c_secure_login=[^;]+/gi, replacement: "c_secure_login=REMOVED" },
  { pattern: /[\w.-]+@[\w.-]+\.\w{2,}/g, replacement: "user@example.com" },
  { pattern: /\b(?:\d{1,3}\.){3}\d{1,3}\b/g, replacement: "127.0.0.1" },
  { pattern: /Bearer\s+[A-Za-z0-9_-]{32,}/g, replacement: "Bearer REMOVED" },
  { pattern: /invite[_-]?(?:code|link|url)[=:][^\s&"'<>]+/gi, replacement: "invite_code=REMOVED" },
  { pattern: /api[_-]?key=[^\s&"'<>]+/gi, replacement: "api_key=REMOVED" },
];

export const STORAGE_KEYS = {
  connection: "pt_tools_connection",
  sessions: "pt_tools_sessions",
  activeSessionId: "pt_tools_active_session_id",
  siteStatus: "pt_tools_site_status",
  tabStatusMap: "pt_tools_tab_status_map",
  autoSyncMap: "pt_tools_auto_sync_map",
  lastSyncMap: "pt_tools_last_sync_map",
} as const;

export const GITHUB_REPO = "sunerpy/pt-tools";
export const GITHUB_NEW_ISSUE_URL = `https://github.com/${GITHUB_REPO}/issues/new`;
