import { describe, expect, it } from "vitest";

import type { KnownSite } from "../core/constants";
import type { KnownSiteStatus } from "../core/types";
import type { SiteLoginStateRecord } from "../modules/sync";
import { buildMySites, cookieHint, countMine, loginLabel, loginTone } from "./mySites";

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

  it("只有 Cookie 有效、快过期或待验证、并且按 Cookie 同步的站点能同步", () => {
    const rows = buildMySites([...sites, status(site("byrpt"), "unverified")], logins, true);
    const byId = Object.fromEntries(rows.map((r) => [r.status.site.id, r]));
    expect(byId.hdsky!.syncable).toBe(true);
    expect(byId.ssd!.syncable).toBe(false);
    expect(byId.byrpt!.syncable).toBe(true);
    expect(byId.byrpt!.attention).toBe(false);
  });

  it("pt-tools 里没启用、扩展推过 Cookie 的站点标成「Cookie 已存，待启用」", () => {
    const pushed = { ...status(site("audiences"), "valid"), lastSync: "2026-10-23T08:00:00Z" };
    const neverPushed = status(site("hdtime"), "valid");
    const enabledPushed = { ...status(site("hdsky"), "valid"), lastSync: "2026-10-23T08:00:00Z" };
    const rows = buildMySites(
      [pushed, neverPushed, enabledPushed],
      [login("audiences", "none", false), login("hdtime", "none", false), login("hdsky", "none")],
      false,
    );
    const byId = Object.fromEntries(rows.map((r) => [r.status.site.id, r]));
    expect(byId.audiences!.savedNotEnabled).toBe(true);
    expect(byId.hdtime!.savedNotEnabled).toBe(false); // 没推过
    expect(byId.hdsky!.savedNotEnabled).toBe(false); // 已启用
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

describe("loginLabel", () => {
  const hdsky = site("hdsky");
  const [enabled] = buildMySites([status(hdsky, "valid")], [login("hdsky", "none")], false);
  const [absent] = buildMySites([status(hdsky, "valid")], [], false);

  it("没读到过 pt-tools 的登录状态时是状态未知，不写「pt-tools 未启用」", () => {
    expect(loginLabel(absent!, false)).toEqual({ kind: "unknown" });
    expect(loginLabel(enabled!, false)).toEqual({ kind: "unknown" });
  });

  it("读到了、pt-tools 里没有这个站点时才是未启用", () => {
    expect(loginLabel(absent!, true)).toEqual({ kind: "notInPtTools" });
    expect(loginLabel(enabled!, true)).toMatchObject({ kind: "tier" });
  });
});

/*
 * 行上不再写「缺 c_secure_uid…」：站点换了登录 Cookie 的格式（BYR）也这么写，看着像扩展坏了。
 * 2026-10-29 用户：「不能做一个通用适配处理吗…而不是报其缺少 id 之类的」。
 */
describe("cookieHint", () => {
  const s = site("byrpt");
  it("格式认不出时说同步时由 pt-tools 验证", () => {
    expect(cookieHint(status(s, "unverified", true))).toBe("unverified");
  });
  it("有 Cookie、没有登录用的时说多半没登录；一个 Cookie 也没有时不说", () => {
    expect(cookieHint(status(s, "missing", true))).toBe("noLogin");
    expect(cookieHint(status(s, "missing", false))).toBeNull();
  });
  it("Cookie 正常、或者不是按 Cookie 同步的站点不说", () => {
    expect(cookieHint(status(s, "valid"))).toBeNull();
    expect(cookieHint(status(site("mteam", "api_key"), "missing", true))).toBeNull();
  });
});
