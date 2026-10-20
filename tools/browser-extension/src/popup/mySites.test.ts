import { describe, expect, it } from "vitest";

import type { KnownSite } from "../core/constants";
import type { KnownSiteStatus } from "../core/types";
import type { SiteLoginStateRecord } from "../modules/sync";
import { buildMySites, countMine, loginTone } from "./mySites";

function site(id: string, syncField: KnownSite["syncField"] = "cookie"): KnownSite {
  return {
    id,
    name: id.toUpperCase(),
    domains: [`${id}.example`],
    schema: "NexusPHP",
    authMethod: syncField === "cookie" ? "cookie" : "api_key",
    cookieNames: syncField === "cookie" ? ["c_secure_uid"] : [],
    syncField,
  } as KnownSite;
}

function status(
  s: KnownSite,
  cookieStatus: KnownSiteStatus["cookieStatus"],
  hasCookie = cookieStatus !== "missing",
): KnownSiteStatus {
  return {
    site: s,
    cookieStatus,
    cookieExpireDays: null,
    hasCookie,
    lastSync: null,
    autoSync: false,
  };
}

function login(name: string, tier: string, enabled = true): SiteLoginStateRecord {
  return {
    site_name: name,
    enabled,
    consecutive_probe_failures: 0,
    ban_threshold_days: 30,
    remind_before_days: 10,
    reminder_cron: "",
    notification_channel_ids: [],
    last_reminder_tier: "",
    days_remaining: 20,
    tier,
  };
}

describe("buildMySites", () => {
  const sites = [
    status(site("hdsky"), "valid"),
    status(site("mteam", "api_key"), "missing"),
    status(site("audiences"), "missing"),
    status(site("ssd"), "expired"),
  ];
  const logins = [login("mteam", "none"), login("hdsky", "7d"), login("audiences", "none", false)];

  it("默认只列有 Cookie 或 pt-tools 里启用了的站点，要注意的排前面", () => {
    const rows = buildMySites(sites, logins, false);
    expect(rows.map((r) => r.status.site.id)).toEqual(["hdsky", "ssd", "mteam"]);
    expect(rows[0]!.attention).toBe(true); // 保号快到期
    expect(rows[1]!.attention).toBe(true); // Cookie 过期
    expect(rows[2]!.configured).toBe(true);
    expect(rows[2]!.syncable).toBe(false); // API Key 站点不同步 Cookie
  });

  it("showAll 时连没有 Cookie、pt-tools 里也没启用的站点都列出来", () => {
    const rows = buildMySites(sites, logins, true);
    expect(rows.map((r) => r.status.site.id)).toContain("audiences");
    expect(countMine(sites, logins)).toBe(3);
  });

  it("只有 Cookie 有效或快过期、并且按 Cookie 同步的站点能同步", () => {
    const rows = buildMySites(sites, logins, true);
    const byId = Object.fromEntries(rows.map((r) => [r.status.site.id, r]));
    expect(byId.hdsky!.syncable).toBe(true);
    expect(byId.ssd!.syncable).toBe(false);
  });
});

describe("loginTone", () => {
  it("按保号档位给颜色", () => {
    expect(loginTone(undefined)).toBe("muted");
    expect(loginTone(login("a", "none"))).toBe("ok");
    expect(loginTone(login("a", "30d"))).toBe("ok");
    expect(loginTone(login("a", "14d"))).toBe("warn");
    expect(loginTone(login("a", "banned-imminent"))).toBe("bad");
    expect(loginTone(login("a", "unknown"))).toBe("muted");
  });
});
