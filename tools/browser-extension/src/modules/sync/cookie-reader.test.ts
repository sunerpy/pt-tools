import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { KNOWN_SITES, type KnownSite } from "../../core/constants";
import { checkCookieHealth } from "./cookie-reader";

/**
 * 按 Cookie 的域装好 Cookie 的 chrome.cookies 替身：键是 Cookie 自己的域，只给这个主机的写主机名（如 old.example），
 * 设在某一级域名上的写带点的（如 .example）。getAll({ url }) 和浏览器一样：主机名相同，或者网址的主机在那个域名之下。
 * 键前面写 http: 的是在 http 下设的、按协议区分时只发给 http 网址的 Cookie（如 http:open.example）。
 */
let jar: Record<string, chrome.cookies.Cookie[]> = {};

function sentTo(url: string): chrome.cookies.Cookie[] {
  const { hostname: host, protocol } = new URL(url);
  return Object.entries(jar).flatMap(([key, list]) => {
    const httpOnly = key.startsWith("http:");
    const domain = httpOnly ? key.slice("http:".length) : key;
    if (httpOnly && protocol !== "http:") return [];
    return domain === host ||
      (domain.startsWith(".") && (host === domain.slice(1) || host.endsWith(domain)))
      ? list
      : [];
  });
}

/** getAll({ domain }) 的语义：只认这个域名和它的子域名（Chromium 的 MatchFilter::MatchesDomain），不含上一级域名的 */
function underDomain(filter: string): chrome.cookies.Cookie[] {
  const f = filter.replace(/^\./, "");
  return Object.entries(jar).flatMap(([domain, list]) => {
    const d = domain.replace(/^\./, "");
    return d === f || d.endsWith(`.${f}`) ? list : [];
  });
}

function cookie(
  name: string,
  value: string,
  expiresInDays = 30,
  domain = "",
): chrome.cookies.Cookie {
  return {
    name,
    value,
    domain,
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

  it("必需 Cookie 分散在不同域时不拼凑：认不出格式，只给出一个域的，要 pt-tools 验证过才保存", async () => {
    jar["old.example"] = [cookie("c_secure_uid", "1")];
    jar["new.example"] = [cookie("c_secure_pass", "p")];

    const health = await checkCookieHealth(SITE);

    expect(health.status).toBe("unverified");
    expect(health.cookieString).toBe("c_secure_uid=1");
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
    expect(health.format).toBe("declared");
  });

  it("不算别的网站同名后缀的 Cookie", async () => {
    jar[".ample"] = [cookie("c_secure_uid", "1"), cookie("c_secure_pass", "p")];
    const health = await checkCookieHealth({ ...SUB, domains: ["pt.example"] } as KnownSite);
    expect(health.status).toBe("missing");
  });
});

/*
 * 认不出格式时不再报「缺 c_secure_uid…」：有像登录用的 Cookie 就整组交给 pt-tools 验证；只有统计、Cloudflare、偏好这类的算没登录。
 * 2026-10-29 用户：「byrbt 中扩展显示缺 c_secure_uid, c_secure_pass, c_secure_tracker_ssl…不能做一个通用适配处理吗」。
 * BYR 现在登录后是 auth_token（JWT，15 分钟）、refresh_token、session_id，都在 .byr.pt 上。
 */
describe("checkCookieHealth: 认不出格式", () => {
  const BYR: KnownSite = {
    ...SITE,
    id: "byrpt",
    domains: ["byr.example"],
    cookieNames: ["c_secure_uid", "c_secure_pass", "c_secure_tracker_ssl"],
  } as KnownSite;

  it("有像登录用的 Cookie：待验证，整组可以同步，不算剩余天数", async () => {
    jar[".byr.example"] = [
      cookie("auth_token", "header.payload.sig", 30),
      cookie("refresh_token", "f".repeat(64), 30),
      cookie("session_id", "0b1c2d3e-0000-4000-8000-000000000000", 30),
      cookie("_ga", "GA1.2.3"),
    ];

    const health = await checkCookieHealth(BYR);

    expect(health.status).toBe("unverified");
    expect(health.format).toBeNull();
    expect(health.expireDays).toBeNull();
    expect(health.hasCookie).toBe(true);
    for (const name of ["auth_token", "refresh_token", "session_id", "_ga"]) {
      expect(health.cookieString).toContain(`${name}=`);
    }
  });

  it("几个域都认不出时取像登录用的 Cookie 最多的那个", async () => {
    jar["old.example"] = [cookie("PHPSESSID", "s1"), cookie("_ga", "GA1.2.3")];
    jar["new.example"] = [cookie("auth_token", "t"), cookie("refresh_token", "r")];

    const health = await checkCookieHealth(SITE);

    expect(health.status).toBe("unverified");
    expect(health.cookieString).toBe("auth_token=t; refresh_token=r");
  });

  it("认得格式的域优先于认不出的", async () => {
    jar["old.example"] = [
      cookie("auth_token", "t"),
      cookie("refresh_token", "r"),
      cookie("x", "1"),
    ];
    jar["new.example"] = [cookie("c_secure_uid", "1"), cookie("c_secure_pass", "p")];

    const health = await checkCookieHealth(SITE);

    expect(health.status).toBe("valid");
    expect(health.cookieString).toBe("c_secure_uid=1; c_secure_pass=p");
  });

  it("只有统计、Cloudflare、偏好这类 Cookie 时算没登录，不给出可同步的 Cookie", async () => {
    jar["old.example"] = [
      cookie("_ga", "GA1.2.3"),
      cookie("_ga_ABC123", "GS1"),
      cookie("Hm_lvt_abc", "1"),
      cookie("cf_clearance", "cf"),
      cookie("c_lang_folder", "chs"),
      cookie("c_secure_tracker_ssl", "bm9wZQ%3D%3D"),
    ];

    const health = await checkCookieHealth(SITE);

    expect(health.status).toBe("missing");
    expect(health.hasCookie).toBe(true);
    expect(health.cookieString).toBe("");
  });

  it("浏览器里一个 Cookie 都没有时 hasCookie 为假", async () => {
    const health = await checkCookieHealth(SITE);
    expect(health.status).toBe("missing");
    expect(health.hasCookie).toBe(false);
  });
});

/*
 * 旧版 NexusPHP 登录要的是 c_secure_uid 和 c_secure_pass；c_secure_tracker_ssl 只是「tracker 用 https」的偏好。
 * 以前六十个站点都要三件套，少了它就显示缺失、不能同步。
 */
describe("checkCookieHealth: 旧版 NexusPHP 只要 uid 和 pass", () => {
  const NEXUS: KnownSite = {
    ...SITE,
    domains: ["pt.example"],
    cookieNames: ["c_secure_uid", "c_secure_pass", "c_secure_tracker_ssl"],
  } as KnownSite;

  it("没有 c_secure_tracker_ssl 也算登录着，剩余天数按 uid、pass 算", async () => {
    jar["pt.example"] = [
      cookie("c_secure_uid", "MTIz", 100),
      cookie("c_secure_pass", "0123456789abcdef0123456789abcdef", 100),
      cookie("c_secure_ssl", "eWVhaA%3D%3D", 1),
    ];

    const health = await checkCookieHealth(NEXUS);

    expect(health.status).toBe("valid");
    expect(health.format).toBe("nexus");
    expect(health.expireDays).toBeGreaterThanOrEqual(99);
    expect(health.cookieString).toContain("c_secure_ssl=");
  });

  it("三件套齐全时按站点声明的算", async () => {
    jar["pt.example"] = [
      cookie("c_secure_uid", "MTIz"),
      cookie("c_secure_pass", "0123456789abcdef0123456789abcdef"),
      cookie("c_secure_tracker_ssl", "eWVhaA%3D%3D"),
    ];
    expect((await checkCookieHealth(NEXUS)).format).toBe("declared");
  });
});

/*
 * 有的站点（OpenCD）没登录时把 https 跳到 http，登录 Cookie 是在 http 下设的；浏览器按协议区分 Cookie 时，
 * 只按 https 取会漏掉。现在 https、http 两边都取，同一条 Cookie 不重复。
 */
describe("checkCookieHealth: http 下设的 Cookie", () => {
  const OPEN: KnownSite = { ...SITE, domains: ["open.example"] } as KnownSite;

  it("只在 http 下有的登录 Cookie 也取得到", async () => {
    jar["http:open.example"] = [cookie("c_secure_uid", "1"), cookie("c_secure_pass", "p")];
    jar[".open.example"] = [cookie("cf_clearance", "cf")];

    const health = await checkCookieHealth(OPEN);

    expect(health.status).toBe("valid");
    expect(health.cookieString).toContain("c_secure_uid=1");
    expect(health.cookieString).toContain("cf_clearance=cf");
  });

  it("两边都取得到的同一条 Cookie 只算一次", async () => {
    jar["open.example"] = [
      cookie("c_secure_uid", "1", 30, "open.example"),
      cookie("c_secure_pass", "p", 30, "open.example"),
    ];

    const health = await checkCookieHealth(OPEN);

    expect(health.cookieString).toBe("c_secure_uid=1; c_secure_pass=p");
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
    expect(health.format).toBe("nexus-token");
    expect(health.cookieString).toContain(`c_secure_pass=${token}`);
    expect(health.expireDays).toBeGreaterThanOrEqual(299);
  });

  it("旧版的 c_secure_pass（md5）单独在时认不出：不直接同步，要 pt-tools 验证过才保存", async () => {
    jar[".example"] = [cookie("c_secure_pass", "0123456789abcdef0123456789abcdef")];

    const health = await checkCookieHealth(NEXUS);

    expect(health.status).toBe("unverified");
    expect(health.format).toBeNull();
  });

  it("令牌解不开、不是 JSON 开头时按旧版算", async () => {
    jar[".example"] = [cookie("c_secure_pass", "%E0%A4%A")];
    expect((await checkCookieHealth(NEXUS)).status).toBe("unverified");
    jar[".example"] = [cookie("c_secure_pass", btoa("not a token"))];
    expect((await checkCookieHealth(NEXUS)).status).toBe("unverified");
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
    expect(health.format).toBe("declared");
    expect(health.cookieString).toContain("ourbits_jwt=header.payload.signature");
    expect(health.cookieString).toContain("cf_clearance=clearance");
    expect(health.expireDays).toBeGreaterThanOrEqual(29);
  });

  it("只有 Cloudflare 的 Cookie 时算没登录", async () => {
    jar[".ourbits.club"] = [cookie("cf_clearance", "clearance", 365)];

    const health = await checkCookieHealth(OURBITS);

    expect(health.status).toBe("missing");
    expect(health.hasCookie).toBe(true);
    expect(health.cookieString).toBe("");
  });
});

/*
 * BYR 2026-10 起登录后是 auth_token / refresh_token / session_id（都在 .byr.pt 上、30 天），扩展按站点声明的认，
 * 剩余天数按这三个算。2026-10-29 用户：「byrbt 中，扩展显示：缺 c_secure_uid, c_secure_pass, c_secure_tracker_ssl」。
 */
describe("checkCookieHealth: BYR 的登录 Cookie", () => {
  const BYR = KNOWN_SITES.find((site) => site.id === "byrpt") as KnownSite;

  it("三个都在就算登录着，可以直接同步", async () => {
    jar[".byr.pt"] = [
      cookie("auth_token", "header.payload.sig", 30),
      cookie("refresh_token", "f".repeat(64), 30),
      cookie("session_id", "0b1c2d3e-0000-4000-8000-000000000000", 30),
    ];

    const health = await checkCookieHealth(BYR);

    expect(health.status).toBe("valid");
    expect(health.format).toBe("declared");
    expect(health.expireDays).toBeGreaterThanOrEqual(29);
  });
});
