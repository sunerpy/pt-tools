import type { KnownSite } from "../../core/constants";
import { getLastVisitMap } from "../../core/storage";
import type { SyncSiteCredentialPayload } from "../../core/types";
import { PtToolsApiError } from "./errors";
import { classifyHttpResponse, fetchWithTimeout } from "./http";

interface LoginResponse {
  success?: boolean;
  username?: string;
}

interface PingResponse {
  status?: string;
  version?: string;
  /** 这个 pt-tools 认得的可选能力（rc.38 起）；旧版本没有这个字段 */
  features?: unknown;
}

/** pt-tools 认得 PUT /api/sites/{id} 的 keep_enabled：停用的站点只存凭据、不启用（见 web/api_credential.go） */
export const FEATURE_CREDENTIAL_KEEP_ENABLED = "site_credential_keep_enabled";

/**
 * pt-tools 认得 PUT /api/sites/{id} 的 verify_cookie：先拿这组 Cookie 实际登录一次，登录上了才保存（见 web/credential_verify.go）。
 * 扩展认不出格式的 Cookie 只推给认得它的 pt-tools：旧版本会不验证直接保存，可能换掉能用的那组。
 */
export const FEATURE_CREDENTIAL_VERIFY = "site_credential_verify";

interface SiteConfigEntry {
  enabled?: boolean;
  auth_method?: string;
}

/**
 * Response from GET /api/sites/login-state. Mirrors
 * web.SiteLoginStateResponse — never includes cookie / cookie_encrypted.
 * Times are unix seconds; null/undefined means "never".
 */
export interface SiteLoginStateRecord {
  site_name: string;
  display_name?: string;
  base_url?: string;
  enabled: boolean;
  last_login_at?: number;
  last_access_at?: number;
  last_visit_at?: number;
  effective_last_active_at?: number;
  last_probe_at?: number;
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
}

function trimSlash(value: string): string {
  return value.replace(/\/+$/, "");
}

export class PtToolsApiClient {
  private readonly baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = trimSlash(baseUrl);
  }

  async login(username: string, password: string): Promise<boolean> {
    const response = await fetchWithTimeout(`${this.baseUrl}/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
      credentials: "include",
      redirect: "manual",
    });

    if (response.type === "opaqueredirect" || response.status === 302) {
      return true;
    }

    if (!response.ok) {
      return false;
    }

    const result = (await this.safeJson(response)) as LoginResponse | null;
    return result?.success !== false;
  }

  /**
   * 把凭据推给 pt-tools。成功时 pt-tools 可能带一句 warning：凭据存下了、但站点还缺别的凭据（HDDolby 还要
   * API Key）所以暂不启用 —— 调用方要把它告诉用户，不能当成什么都没发生。
   */
  async syncSiteCredential(
    site: KnownSite,
    credential: string,
    opts: { keepEnabled?: boolean; verifyCookie?: boolean } = {},
  ): Promise<{ warning?: string }> {
    const siteId = site.id.toLowerCase();
    const payload: SyncSiteCredentialPayload = {
      [site.syncField]: credential,
    };
    // 定时同步推没启用的站点时带上：只存 Cookie，不替用户启用（旧版 pt-tools 不认，调用方先看 features）
    if (opts.keepEnabled) {
      payload.keep_enabled = true;
    }
    // 格式认不出的 Cookie：要 pt-tools 先验证、登录上了才保存（调用方先看 features）
    if (opts.verifyCookie) {
      payload.verify_cookie = true;
    }

    const visitMap = await getLastVisitMap();
    const lastVisitAt = visitMap[siteId];
    if (lastVisitAt) {
      payload.last_visit_at = lastVisitAt;
    }

    const response = await fetch(`${this.baseUrl}/api/sites/${encodeURIComponent(siteId)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(payload),
      credentials: "include",
      redirect: "manual",
    });

    if (!response.ok) {
      const text = await response.text().catch(() => "");
      if (opts.verifyCookie && (response.status === 409 || response.status === 422)) {
        throw new PtToolsApiError(
          "verify_failed",
          text.trim() || `HTTP ${response.status}`,
          response.status,
        );
      }
      classifyHttpResponse(response, text);
    }
    const body = (await this.safeJson(response)) as { warning?: unknown } | null;
    return typeof body?.warning === "string" && body.warning.trim()
      ? { warning: body.warning }
      : {};
  }

  async reportVisit(siteId: string, ts: number | Date): Promise<void> {
    const lastVisitAt = (typeof ts === "number" ? new Date(ts) : ts).toISOString();
    try {
      const response = await fetch(`${this.baseUrl}/api/sites/visit`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ site_name: siteId, last_visit_at: lastVisitAt }),
        credentials: "include",
        redirect: "manual",
      });
      if (!response.ok && response.status !== 401 && response.status !== 0) {
        console.warn(`pt-tools reportVisit non-OK response for ${siteId}: HTTP ${response.status}`);
      }
    } catch (error: unknown) {
      console.warn(`pt-tools reportVisit failed for ${siteId}`, error);
    }
  }

  async getSites(): Promise<Array<{ name: string; enabled: boolean }>> {
    const response = await fetchWithTimeout(`${this.baseUrl}/api/sites`, {
      method: "GET",
      headers: { Accept: "application/json" },
      credentials: "include",
      redirect: "manual",
    });

    if (!response.ok) {
      const text = await response.text().catch(() => "");
      classifyHttpResponse(response, text);
    }

    const raw = await this.safeJson(response);
    if (raw === null || typeof raw !== "object") {
      return [];
    }

    if (Array.isArray(raw)) {
      return raw
        .filter(
          (item): item is Record<string, unknown> => typeof item === "object" && item !== null,
        )
        .filter((item) => typeof item.name === "string")
        .map((item) => ({ name: item.name as string, enabled: item.enabled === true }));
    }

    const siteMap = raw as Record<string, unknown>;
    return Object.entries(siteMap).map(([name, value]) => {
      const entry = value as SiteConfigEntry | null;
      return { name, enabled: entry?.enabled === true };
    });
  }

  async getSiteLoginStatus(): Promise<SiteLoginStateRecord[]> {
    const response = await fetchWithTimeout(`${this.baseUrl}/api/sites/login-state`, {
      method: "GET",
      headers: { Accept: "application/json" },
      credentials: "include",
      redirect: "manual",
    });

    if (!response.ok) {
      const text = await response.text().catch(() => "");
      classifyHttpResponse(response, text);
    }

    const raw = await this.safeJson(response);
    if (!Array.isArray(raw)) {
      return [];
    }
    return raw.filter((item): item is SiteLoginStateRecord => {
      if (typeof item !== "object" || item === null) return false;
      const obj = item as Record<string, unknown>;
      return typeof obj.site_name === "string";
    });
  }

  /** features 读 /api/ping 里这个 pt-tools 认得的可选能力；读不到、旧版本没有这个字段时返回空数组 */
  async features(timeoutMs?: number): Promise<string[]> {
    const response = await fetchWithTimeout(`${this.baseUrl}/api/ping`, {
      method: "GET",
      headers: { Accept: "application/json" },
      credentials: "include",
      redirect: "manual",
      timeoutMs,
    });
    if (!response.ok) {
      return [];
    }
    const result = (await this.safeJson(response)) as PingResponse | null;
    const features = result?.features;
    return Array.isArray(features)
      ? features.filter((f): f is string => typeof f === "string")
      : [];
  }

  async ping(timeoutMs?: number): Promise<void> {
    const response = await fetchWithTimeout(`${this.baseUrl}/api/ping`, {
      method: "GET",
      headers: { Accept: "application/json" },
      credentials: "include",
      redirect: "manual",
      timeoutMs,
    });
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      classifyHttpResponse(response, text);
    }
    const result = (await this.safeJson(response)) as PingResponse | null;
    if (result?.status !== "ok") {
      throw new PtToolsApiError("ping_failed", "ping returned non-ok status");
    }
  }

  private async safeJson(response: Response): Promise<unknown> {
    const text = await response.text();
    if (!text) return null;
    try {
      return JSON.parse(text) as unknown;
    } catch {
      return null;
    }
  }
}
