import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { KNOWN_SITES, type KnownSite } from "../../core/constants";
import { checkCookieHealth } from "./cookie-reader";

/**
 * 按 Cookie 的域装好 Cookie 的 chrome.cookies 替身：键是 Cookie 自己的域，只给这个主机的写主机名（如 old.example），
 * 设在某一级域名上的写带点的（如 .example）。getAll({ url }) 和浏览器一样：主机名相同，或者网址的主机在那个域名之下。
 */
let jar: Record<string, chrome.cookies.Cookie[]> = {};

function sentTo(url: string): chrome.cookies.Cookie[] {
  const host = new URL(url).hostname;
  return Object.entries(jar).flatMap(([domain, list]) =>
    domain === host ||
    (domain.startsWith(".") && (host === domain.slice(1) || host.endsWith(domain)))
      ? list
      : [],
  );
}

/** getAll({ domain }) 的语义：只认这个域名和它的子域名（Chromium 的 MatchFilter::MatchesDomain），不含上一级域名的 */
function underDomain(filter: string): chrome.cookies.Cookie[] {
  const f = filter.replace(/^\./, "");
  return Object.entries(jar).flatMap(([domain, list]) => {
    const d = domain.replace(/^\./, "");
    return d === f || d.endsWith(`.${f}`) ? list : [];
  });
}

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
    cookies: {
      getAll: vi.fn(async ({ url, domain }: { url?: string; domain?: string }) =>
        url ? sentTo(url) : underDomain(domain ?? ""),
      ),
    },
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

/*
 * 把登录 Cookie 设在上一级域名上的站点：以前按 domain 过滤只认本域名和子域名，登录着也显示「缺失」、
 * 没有同步按钮。现在按网址取，和浏览器实际发给站点的一致。
 */
describe("checkCookieHealth: Cookie 设在上一级域名上", () => {
  const SUB: KnownSite = { ...SITE, id: "sub", domains: ["pt.example"] } as KnownSite;

  it("算作这个站点的 Cookie，可以同步", async () => {
    jar[".example"] = [cookie("c_secure_uid", "1"), cookie("c_secure_pass", "p")];
    jar["pt.example"] = [cookie("cf_clearance", "cf")];
    jar["other.example2"] = [cookie("c_secure_pass", "x")];

    const health = await checkCookieHealth(SUB);

    expect(health.status).toBe("valid");
    expect(health.cookieString).toContain("c_secure_uid=1");
    expect(health.cookieString).toContain("c_secure_pass=p");
    expect(health.cookieString).toContain("cf_clearance=cf");
    expect(health.cookieString).not.toContain("=x");
    expect(health.missingCookies).toEqual([]);
  });

  it("不算别的网站同名后缀的 Cookie", async () => {
    jar[".ample"] = [cookie("c_secure_uid", "1"), cookie("c_secure_pass", "p")];
    const health = await checkCookieHealth({ ...SUB, domains: ["pt.example"] } as KnownSite);
    expect(health.status).toBe("missing");
  });
});

describe("checkCookieHealth: 缺失时写明缺哪几个", () => {
  it("按凑得最多的那个域名说还差的", async () => {
    jar["old.example"] = [cookie("_ga", "GA1.2.3")];
    jar["new.example"] = [cookie("c_secure_uid", "1"), cookie("cf_clearance", "cf")];

    const health = await checkCookieHealth(SITE);

    expect(health.status).toBe("missing");
    expect(health.hasCookie).toBe(true);
    expect(health.missingCookies).toEqual(["c_secure_pass"]);
    expect(health.cookieString).toBe("");
  });

  it("一个必需 Cookie 也没有时列出全部", async () => {
    jar["old.example"] = [cookie("_ga", "GA1.2.3")];
    const health = await checkCookieHealth(SITE);
    expect(health.missingCookies).toEqual(["c_secure_uid", "c_secure_pass"]);
  });
});

/*
 * 新版 NexusPHP（xiaomlove，include/functions.php 的 logincookie）登录只设一个 c_secure_pass：
 * 值是 base64 的 {"user_id":…,"expires":…}.签名，没有 c_secure_uid、c_secure_tracker_ssl。星陨阁就是这样，
 * 2026-10-25 用户：「缺 c_secure_uid, c_secure_tracker_ssl」「实际上 chrome 中我已登录星陨阁啊」。
 */
describe("checkCookieHealth: 新版 NexusPHP 只有一个 c_secure_pass", () => {
  const NEXUS: KnownSite = {
    ...SITE,
    id: "xingyunge",
    domains: ["pt.example"],
    cookieNames: ["c_secure_uid", "c_secure_pass", "c_secure_tracker_ssl"],
  } as KnownSite;
  // PHP 的 setcookie 会把 base64 里的 = + / 编成 %3D %2B %2F
  const token = encodeURIComponent(
    btoa('{"user_id":12345,"expires":1792000000}.') + "0f".repeat(32),
  );

  it("c_secure_pass 是签名令牌时只要它一个，可以同步", async () => {
    jar[".example"] = [cookie("c_secure_pass", token, 300), cookie("c_lang_folder", "chs")];

    const health = await checkCookieHealth(NEXUS);

    expect(health.status).toBe("valid");
    expect(health.missingCookies).toEqual([]);
    expect(health.cookieString).toContain(`c_secure_pass=${token}`);
    expect(health.expireDays).toBeGreaterThanOrEqual(299);
  });

  it("旧版的 c_secure_pass（md5）单独在时仍算缺失，不拿残缺的一组去同步", async () => {
    jar[".example"] = [cookie("c_secure_pass", "0123456789abcdef0123456789abcdef")];

    const health = await checkCookieHealth(NEXUS);

    expect(health.status).toBe("missing");
    expect(health.missingCookies).toEqual(["c_secure_uid", "c_secure_tracker_ssl"]);
    expect(health.cookieString).toBe("");
  });

  it("令牌解不开、不是 JSON 开头时按旧版算", async () => {
    jar[".example"] = [cookie("c_secure_pass", "%E0%A4%A")];
    expect((await checkCookieHealth(NEXUS)).status).toBe("missing");
    jar[".example"] = [cookie("c_secure_pass", btoa("not a token"))];
    expect((await checkCookieHealth(NEXUS)).status).toBe("missing");
  });
});

/*
 * OurBits 现在登录只设一个 ourbits_jwt（JWT，HS256，30 天到期），没有 c_secure_* 三件套；另有 Cloudflare 的 cf_clearance。
 * 2026-10-27 用户：「OurBits 缺失 … 缺 c_secure_uid, c_secure_pass, c_secure_tracker_ssl」「ourbits通过扩展无法同步cookie」。
 * 带上 ourbits_jwt 的请求不会被 Cloudflare 质询（2026-10-27 在服务器上试过：没有它是 403 质询页，有它 200 并且登录着）。
 */
describe("checkCookieHealth: OurBits 只有 ourbits_jwt", () => {
  const OURBITS = KNOWN_SITES.find((site) => site.id === "ourbits") as KnownSite;

  it("有 ourbits_jwt 就算登录着，可以同步（连同 cf_clearance 一起）", async () => {
    jar["ourbits.club"] = [cookie("ourbits_jwt", "header.payload.signature", 30)];
    jar[".ourbits.club"] = [cookie("cf_clearance", "clearance", 365)];

    const health = await checkCookieHealth(OURBITS);

    expect(health.status).toBe("valid");
    expect(health.missingCookies).toEqual([]);
    expect(health.cookieString).toContain("ourbits_jwt=header.payload.signature");
    expect(health.cookieString).toContain("cf_clearance=clearance");
    expect(health.expireDays).toBeGreaterThanOrEqual(29);
  });

  it("只有 Cloudflare 的 Cookie 时算缺失，写明缺 ourbits_jwt", async () => {
    jar[".ourbits.club"] = [cookie("cf_clearance", "clearance", 365)];

    const health = await checkCookieHealth(OURBITS);

    expect(health.status).toBe("missing");
    expect(health.missingCookies).toEqual(["ourbits_jwt"]);
    expect(health.cookieString).toBe("");
  });
});
