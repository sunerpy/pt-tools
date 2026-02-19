import type { KnownSite } from "../../core/constants";

interface LoginResponse {
  success?: boolean;
  username?: string;
}

interface PingResponse {
  status?: string;
  version?: string;
}

interface SiteConfigEntry {
  enabled?: boolean;
  auth_method?: string;
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
    const response = await fetch(`${this.baseUrl}/login`, {
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

  async syncSiteCredential(site: KnownSite, credential: string): Promise<void> {
    const siteId = site.id.toLowerCase();
    const response = await fetch(`${this.baseUrl}/api/sites/${encodeURIComponent(siteId)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ [site.syncField]: credential }),
      credentials: "include",
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `HTTP ${response.status}`);
    }
  }

  async getSites(): Promise<Array<{ name: string; enabled: boolean }>> {
    const response = await fetch(`${this.baseUrl}/api/sites`, {
      method: "GET",
      credentials: "include",
    });

    if (!response.ok) {
      throw new Error("Failed to get configured sites");
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

  async ping(): Promise<boolean> {
    try {
      const response = await fetch(`${this.baseUrl}/api/ping`, {
        method: "GET",
        credentials: "include",
      });
      if (!response.ok) return false;
      const result = (await this.safeJson(response)) as PingResponse | null;
      return result?.status === "ok";
    } catch {
      return false;
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
