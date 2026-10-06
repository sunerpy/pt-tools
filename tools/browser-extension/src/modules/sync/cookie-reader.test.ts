import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { KnownSite } from "../../core/constants";
import { checkCookieHealth } from "./cookie-reader";

/** 按域名装好 Cookie 的 chrome.cookies 替身 */
let jar: Record<string, chrome.cookies.Cookie[]> = {};

function cookie(name: string, value: string, expiresInDays = 30): chrome.cookies.Cookie {
  return {
    name,
    value,
    domain: "",
    path: "/",
    secure: true,
    httpOnly: true,
    sameSite: "lax",
    session: false,
    hostOnly: false,
    storeId: "0",
    expirationDate: Date.now() / 1000 + expiresInDays * 86400,
  } as chrome.cookies.Cookie;
}

const SITE: KnownSite = {
  id: "multi",
  name: "Multi",
  domains: ["old.example", "new.example"],
  schema: "NexusPHP",
  authMethod: "cookie",
  cookieNames: ["c_secure_uid", "c_secure_pass"],
  syncField: "cookie",
} as KnownSite;

beforeEach(() => {
  jar = {};
  vi.stubGlobal("chrome", {
    cookies: { getAll: vi.fn(async ({ domain }: { domain: string }) => jar[domain] ?? []) },
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

/*
 * 多域站点：健康检查原来把所有域的必需 Cookie 合在一起判定，发送的却是「第一个有任意 Cookie 的域」的全部 Cookie。
 * 旧域只剩统计 Cookie、新域有完整登录态时，状态显示有效，同步却把统计 Cookie 写进后端，替换掉原来有效的凭证。
 */
describe("checkCookieHealth: 多域站点", () => {
  it("用同时具备全部必需 Cookie 的那个域", async () => {
    jar["old.example"] = [cookie("_ga", "GA1.2.3")];
    jar["new.example"] = [
      cookie("c_secure_uid", "1"),
      cookie("c_secure_pass", "p"),
      cookie("cf_clearance", "cf"),
    ];

    const health = await checkCookieHealth(SITE);

    expect(health.status).toBe("valid");
    expect(health.cookieString).toContain("c_secure_uid=1");
    expect(health.cookieString).toContain("c_secure_pass=p");
    expect(health.cookieString).not.toContain("_ga");
  });

  it("必需 Cookie 分散在不同域时判为缺失，不拼凑、不给出可同步的 Cookie", async () => {
    jar["old.example"] = [cookie("c_secure_uid", "1")];
    jar["new.example"] = [cookie("c_secure_pass", "p")];

    const health = await checkCookieHealth(SITE);

    expect(health.status).toBe("missing");
    expect(health.cookieString).toBe("");
  });

  it("几个域都齐全时选最晚过期的那个，剩余天数也按它算", async () => {
    jar["old.example"] = [cookie("c_secure_uid", "1", -3), cookie("c_secure_pass", "p", -3)];
    jar["new.example"] = [cookie("c_secure_uid", "2", 20), cookie("c_secure_pass", "q", 20)];

    const health = await checkCookieHealth(SITE);

    expect(health.status).toBe("valid");
    expect(health.expireDays).toBeGreaterThanOrEqual(19);
    expect(health.cookieString).toContain("c_secure_uid=2");
    expect(health.cookieString).not.toContain("c_secure_uid=1");
  });
});
