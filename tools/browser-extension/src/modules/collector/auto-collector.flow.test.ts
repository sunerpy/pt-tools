import { describe, expect, it } from "vitest";

import { t } from "../../core/i18n";
import type { CapturedPage } from "../../core/types";
import { REQUESTS_LOG_MAX_CHARS } from "../../recorder/constants";
import { autoCollect, type CollectorDeps } from "./auto-collector";
import type { RecordedRequest, TabJob, TabRecording } from "./tab-recorder";

/*
 * 模仿 HDWinG（issue #910）的假站点：列表与详情只有 sp_state_placeholder，优惠由页面脚本 POST /ajax_promotion.php 取回；
 * 个人页的做种列表由脚本带 x-csrf POST /ajax_getusertorrentlist.php；首页签到入口由脚本 GET attendance.php；
 * 下载链接是签名的 download.php?hash=；没有 H&R 页；没登录时回登录页。
 */
const ORIGIN = "https://hdw.example";
const UID = "4242";
const USERNAME = "hdwing_tester";
const CSRF = "CSRF_TOKEN_0001";
const PASSKEY = "a1b2c3d4e5f60718293a4b5c6d7e8f90";
const HASH = "SIGNEDHASH0001";
const IPV4 = "203.0.113.77";
const IPV6 = "2001:db8:85a3::8a2e:370:7334";
const SECRETS = [USERNAME, CSRF, PASSKEY, HASH, IPV4, IPV6, UID];

const enc = new TextEncoder();
const bstr = (s: string) => `${enc.encode(s).length}:${s}`;
const TORRENT = enc.encode(
  `d${bstr("announce")}${bstr(`https://tracker.hdw.example/announce.php?passkey=${PASSKEY}`)}` +
    `${bstr("announce-list")}ll${bstr(`https://tracker.hdw.example/announce.php?passkey=${PASSKEY}`)}e` +
    `l${bstr("udp://backup.hdw.example:6969/announce")}ee` +
    `${bstr("info")}d${bstr("length")}i42e${bstr("name")}${bstr("a.mkv")}ee`,
);

const INDEX = `<html><head><meta name="x-csrf" content="${CSRF}"><title>HDWinG :: 首页</title>
<script src="/js/jquery.min.js"></script><script src="/js/sign.js?v=2"></script>
<script src="https://cdn.other.example/lib.js"></script></head>
<body><div id="info_block">欢迎回来，<a href="userdetails.php?id=${UID}" class='User_Name'><b>${USERNAME}</b></a>
<a href="javascript:void(0)" id="do-sign" onclick="doSign()">签到</a> 最近 IP：${IPV4} / ${IPV6}
<a href="torrentrss.php?passkey=${PASSKEY}">RSS</a></div>
<script>var csrf = "${CSRF}"; $.post("ajax_promotion.php", {csrf: csrf});</script></body></html>`;

const LIST = `<html><body><table class="torrent_list"><tr class="tab_header"><td>名称</td></tr>
<tr><td class="t_name"><h3><a href="details.php?id=101&amp;hit=1">A</a></h3><span class="sp_state_placeholder" data-id="101"></span></td></tr>
<tr><td class="t_name"><h3><a href="details.php?id=102&amp;hit=1">B</a></h3><span class="sp_state_placeholder" data-id="102"></span></td></tr>
</table><script src="/js/promotion.js"></script></body></html>`;

const DETAIL = (id: string) =>
  `<html><body><h2 id="top">种子 ${id} <span class="sp_state_placeholder"></span></h2>
<a href="download.php?hash=${HASH}">下载种子</a><script src="/js/promotion.js"></script></body></html>`;

const USERINFO = `<html><head><title>${USERNAME} 的个人信息</title></head><body>
<table><tr><td class="rowhead">用户名</td><td class="rowfollow">${USERNAME}</td></tr>
<tr><td class="rowhead">当前做种</td><td class="rowfollow"><a href="javascript:void(0)" onclick="getusertorrentlist('seeding','ka1')">当前做种</a><div id="ka1"></div></td></tr></table>
<script>var uid = ${UID};</script><script src="/js/userdetails.js"></script></body></html>`;

const SIGN_JS = `function doSign(){ $.get("/attendance.php?action=sign", function(r){ alert(r); }); } // 签到`;
const USERDETAILS_JS = `function getusertorrentlist(type, blockid) {
  $.post("ajax_getusertorrentlist.php", {userid: uid, type: type, csrf: $('meta[name="x-csrf"]').attr('content')}, function (d) {
    $('#' + blockid).html(d.message);
  }, "json");
}`;
const PROMOTION_JS = `$(function(){ var ids = []; $.post("/ajax_promotion.php", {csrf: $('meta[name="x-csrf"]').attr('content'), ids: ids}, function(d){ /* sp_state */ }, "json"); });`;

const RSS = `<?xml version="1.0" encoding="utf-8"?><rss version="2.0"><channel><title>HDWinG Torrents</title>
<item><title>A</title><link>${ORIGIN}/details.php?id=101</link>
<enclosure url="${ORIGIN}/download.php?id=101&amp;passkey=${PASSKEY}" length="42" type="application/x-bittorrent" /></item>
</channel></rss>`;

const SEEDING_JSON = JSON.stringify({
  status: 1,
  message: `<table><tr><td><a href="details.php?id=101">A</a></td><td>${USERNAME}</td></tr></table>`,
});

interface Call {
  url: string;
  method: string;
  body: string;
  credentials: RequestCredentials | undefined;
}

type Override = (call: Call) => Response | undefined;

function respond(
  body: BodyInit,
  init: { status?: number; type?: string; url?: string; redirected?: boolean } = {},
): Response {
  const res = new Response(body, {
    status: init.status ?? 200,
    headers: { "content-type": init.type ?? "text/html; charset=utf-8" },
  });
  if (init.url) Object.defineProperty(res, "url", { value: init.url });
  if (init.redirected) Object.defineProperty(res, "redirected", { value: true });
  return res;
}

function fakeSite(override?: Override): { calls: Call[]; fetch: CollectorDeps["fetch"] } {
  const calls: Call[] = [];
  const fetch: CollectorDeps["fetch"] = async (input, init) => {
    const call: Call = {
      url: String(input),
      method: (init?.method ?? "GET").toUpperCase(),
      body: typeof init?.body === "string" ? init.body : init?.body ? String(init.body) : "",
      credentials: init?.credentials,
    };
    calls.push(call);
    const custom = override?.(call);
    if (custom) return custom;

    const url = new URL(call.url);
    const route = url.pathname + url.search;
    if (call.credentials === "omit") {
      return respond(
        `<html><title>HDWinG :: 登录</title><form action="takelogin.php"><input type="password" name="password"></form></html>`,
        {
          url: `${ORIGIN}/login.php`,
          redirected: true,
        },
      );
    }
    switch (url.pathname) {
      case "/index.php":
        return respond(INDEX);
      case "/torrents.php":
        return respond(LIST);
      case "/details.php":
        return respond(DETAIL(url.searchParams.get("id") ?? ""));
      case "/userdetails.php":
        return respond(USERINFO);
      case "/js/sign.js":
        return respond(SIGN_JS, { type: "application/javascript" });
      case "/js/userdetails.js":
        return respond(USERDETAILS_JS, { type: "application/javascript" });
      case "/js/promotion.js":
        return respond(PROMOTION_JS, { type: "application/javascript" });
      case "/mybonus.php":
        return respond(`<html><title>魔力值</title><td>每小时 12.3</td></html>`);
      case "/rules.php":
        return respond(`<html><title>规则</title><h2>网站等级规则</h2></html>`);
      case "/faq.php":
        return respond(`<html><title>常见问题</title></html>`);
      case "/torrentrss.php":
        return route === "/torrentrss.php?rows=10"
          ? respond(RSS, { type: "application/xml" })
          : respond("bad", { status: 400 });
      case "/ajax_getusertorrentlist.php": {
        const form = new URLSearchParams(call.body);
        const ok =
          call.method === "POST" &&
          form.get("csrf") === CSRF &&
          form.get("userid") === UID &&
          form.get("type") === "seeding";
        return ok
          ? respond(SEEDING_JSON, { type: "application/json" })
          : respond(`{"status":0}`, { status: 403, type: "application/json" });
      }
      case "/download.php":
        return url.searchParams.get("hash") === HASH
          ? new Response(TORRENT, { headers: { "content-type": "application/x-bittorrent" } })
          : respond("not found", { status: 404 });
      case "/attendance.php":
        return respond("签到成功");
      default:
        return respond("<html>404</html>", { status: 404 });
    }
  };
  return { calls, fetch };
}

function recordingFor(
  job: TabJob,
  requests: RecordedRequest[],
  click: TabRecording["click"] = null,
): TabRecording {
  return {
    key: job.key,
    url: job.url,
    finalUrl: job.url,
    html: `<html><body>rendered ${job.key} ${USERNAME} <meta name="x-csrf" content="${CSRF}"></body></html>`,
    requests,
    click,
  };
}

function fakeTabs(options: { clickWorks?: boolean } = {}) {
  const seen: { origin: string; jobs: TabJob[] }[] = [];
  const recordTabs: NonNullable<CollectorDeps["recordTabs"]> = async (origin, jobs) => {
    seen.push({ origin, jobs });
    return jobs.map((job) => {
      if (job.key === "search") {
        return recordingFor(job, [
          {
            kind: "xhr",
            method: "POST",
            url: `${ORIGIN}/ajax_promotion.php`,
            status: 200,
            contentType: "application/json",
            requestBody: `csrf=${CSRF}&ids%5B%5D=101&ids%5B%5D=102`,
            responseBody: JSON.stringify({
              status: 1,
              message: { "101": { sp_state: `<img class="pro_free" alt="Free">`, timeout: "" } },
            }),
            startedAt: 1000,
            endedAt: 1100,
          },
        ]);
      }
      if (job.key === "userinfo") {
        return recordingFor(
          job,
          options.clickWorks === false
            ? []
            : [
                {
                  kind: "xhr",
                  method: "POST",
                  url: `${ORIGIN}/ajax_getusertorrentlist.php`,
                  status: 200,
                  contentType: "application/json",
                  requestBody: `userid=${UID}&type=seeding&csrf=${CSRF}`,
                  responseBody: SEEDING_JSON,
                  startedAt: 2000,
                  endedAt: 2100,
                },
              ],
          {
            clicked: true,
            text: "当前做种",
            code: "getusertorrentlist('seeding','ka1')",
            at: 1500,
          },
        );
      }
      return recordingFor(job, []);
    });
  };
  return { seen, recordTabs };
}

function deps(
  site: ReturnType<typeof fakeSite>,
  extra: Partial<CollectorDeps> = {},
): CollectorDeps {
  return {
    fetch: site.fetch,
    sleep: async () => undefined,
    recordTabs: fakeTabs().recordTabs,
    cookieNames: async () => ["c_secure_pass", "c_secure_uid", "cf_clearance"],
    ...extra,
  };
}

function typesOf(pages: CapturedPage[]): string[] {
  return pages.map((page) => page.pageType);
}

function expectNoSecrets(text: string): void {
  for (const secret of SECRETS) {
    expect(text, `leaked ${secret}`).not.toContain(secret);
  }
}

describe("autoCollect（NexusPHP，HDWinG 那样的站点）", () => {
  it("补采魔力值、规则、常见问题、RSS、未登录首页、做种列表、脚本，并记下站点信息", async () => {
    const site = fakeSite();
    const { pages, warnings, facts, identity } = await autoCollect(
      ORIGIN,
      "NexusPHP",
      undefined,
      deps(site),
    );

    const types = typesOf(pages);
    for (const type of [
      "index",
      "search",
      "search_free",
      "detail",
      "userinfo",
      "bonus",
      "rules",
      "faq",
      "rss",
      "logged_out",
      "seeding",
      "rendered_search",
      "rendered_detail",
      "rendered_userinfo",
      "requests_search",
      "requests_detail",
      "requests_userinfo",
    ]) {
      expect(types, type).toContain(type);
    }
    expect(types).not.toContain("hr");
    expect(types).not.toContain("download_notice");

    const scripts = pages.filter((page) => page.pageType === "script");
    expect(scripts.map((page) => page.url).sort()).toEqual([
      `${ORIGIN}/js/promotion.js`,
      `${ORIGIN}/js/sign.js?v=2`,
      `${ORIGIN}/js/userdetails.js`,
    ]);
    expect(scripts.every((page) => page.format === "js")).toBe(true);
    expect(pages.find((page) => page.pageType === "rss")?.format).toBe("xml");
    expect(pages.find((page) => page.pageType === "seeding")?.format).toBe("json");
    expect(pages.find((page) => page.pageType === "requests_search")?.format).toBe("json");

    expect(identity).toEqual({ username: USERNAME, userId: UID });
    expect(facts.cookieNames).toEqual(["c_secure_pass", "c_secure_uid", "cf_clearance"]);
    expect(facts.trackerHosts).toEqual(["backup.hdw.example", "tracker.hdw.example"]);
    expect(facts.seedingRequest).toMatchObject({
      method: "POST",
      path: "/ajax_getusertorrentlist.php",
      params: ["userid", "type", "csrf"],
      source: "script",
      status: 200,
    });
    expect(facts.seedingAttempts?.[0]).toMatchObject({
      method: "GET",
      path: "/getusertorrentlistajax.php",
      params: ["userid", "type"],
      source: "upstream",
      status: 404,
    });
    expect(facts.loggedOut).toEqual({
      requestedUrl: `${ORIGIN}/index.php`,
      finalUrl: `${ORIGIN}/login.php`,
      status: 200,
      redirected: true,
    });
    expect(facts.signinEntries).toContainEqual(
      expect.objectContaining({ page: "index", tag: "a", text: "签到", onclick: "doSign()" }),
    );
    expect(facts.scriptEndpoints).toEqual(
      expect.arrayContaining([
        "/ajax_getusertorrentlist.php",
        "/ajax_promotion.php",
        "/attendance.php",
      ]),
    );
    expect(facts.scripts?.find((s) => s.url.endsWith("sign.js?v=2"))?.keywords).toEqual([
      "签到",
      "attendance",
    ]);
    expect(warnings.some((w) => /H&R/.test(w))).toBe(true);
    expect(facts.warnings).toEqual(warnings);

    // 做种列表照脚本 POST，带页面的 x-csrf
    const seedingCall = site.calls.find((call) =>
      call.url.endsWith("/ajax_getusertorrentlist.php"),
    );
    expect(seedingCall?.method).toBe("POST");
    expect(new URLSearchParams(seedingCall?.body).get("csrf")).toBe(CSRF);
    // 未登录首页不带 Cookie
    expect(
      site.calls.some((call) => call.credentials === "omit" && call.url === `${ORIGIN}/index.php`),
    ).toBe(true);
  });

  it("从不请求签到地址，jquery 和别的网站的脚本不取", async () => {
    const site = fakeSite();
    await autoCollect(ORIGIN, "NexusPHP", undefined, deps(site));
    const urls = site.calls.map((call) => call.url);
    expect(urls.some((url) => /attendance\.php/i.test(url))).toBe(false);
    expect(urls.some((url) => url.includes("jquery"))).toBe(false);
    expect(urls.some((url) => url.includes("cdn.other.example"))).toBe(false);
  });

  it("后台标签页：列表页、详情页、个人页，只在个人页点「当前做种」", async () => {
    const site = fakeSite();
    const tabs = fakeTabs();
    const { pages } = await autoCollect(
      ORIGIN,
      "NexusPHP",
      undefined,
      deps(site, { recordTabs: tabs.recordTabs }),
    );
    expect(tabs.seen).toHaveLength(1);
    expect(tabs.seen[0].origin).toBe(ORIGIN);
    expect(tabs.seen[0].jobs).toEqual([
      { key: "search", url: `${ORIGIN}/torrents.php` },
      { key: "detail", url: `${ORIGIN}/details.php?id=101` },
      { key: "userinfo", url: `${ORIGIN}/userdetails.php?id=${UID}`, clickSeeding: true },
    ]);
    const log = JSON.parse(pages.find((page) => page.pageType === "requests_search")!.html);
    expect(log.requests[0]).toMatchObject({
      method: "POST",
      url: `${ORIGIN}/ajax_promotion.php`,
      status: 200,
      params: "csrf=REMOVED&ids%5B%5D=101&ids%5B%5D=102",
    });
    expect(log.requests[0].response.message["101"].sp_state).toContain("pro_free");
  });

  it("导出前的页面与站点信息里没有用户名、ID、passkey、csrf、签名哈希和 IP", async () => {
    const site = fakeSite();
    const { pages, facts } = await autoCollect(ORIGIN, "NexusPHP", undefined, deps(site));
    for (const page of pages) {
      expectNoSecrets(page.html);
      expectNoSecrets(page.url);
    }
    expectNoSecrets(JSON.stringify(facts));
  });

  it("做种列表照脚本也取不到时，用后台标签页里点「当前做种」记到的请求", async () => {
    const site = fakeSite((call) =>
      call.url.endsWith("/ajax_getusertorrentlist.php")
        ? respond("denied", { status: 403 })
        : undefined,
    );
    const { pages, facts } = await autoCollect(ORIGIN, "NexusPHP", undefined, deps(site));
    expect(facts.seedingRequest).toMatchObject({
      method: "POST",
      path: "/ajax_getusertorrentlist.php",
      params: ["userid", "type", "csrf"],
      source: "click",
      status: 200,
    });
    const seeding = pages.find((page) => page.pageType === "seeding");
    expect(seeding?.format).toBe("json");
    expect(seeding?.html).toContain("details.php?id=101");
  });

  it("脚本里写的做种列表接口在别的网站时不去请求（不把 Cookie 和 CSRF 令牌带过去）", async () => {
    const site = fakeSite((call) =>
      call.url.endsWith("/js/userdetails.js")
        ? respond(
            USERDETAILS_JS.replace(
              '"ajax_getusertorrentlist.php"',
              '"https://other.example/ajax_getusertorrentlist.php"',
            ),
            {
              type: "application/javascript",
            },
          )
        : undefined,
    );
    const { facts } = await autoCollect(ORIGIN, "NexusPHP", undefined, deps(site));
    expect(site.calls.some((call) => !call.url.startsWith(`${ORIGIN}/`))).toBe(false);
    expect(facts.seedingAttempts?.some((attempt) => attempt.source === "script")).toBe(false);
  });

  it("三种办法都取不到做种列表时记一条提示", async () => {
    const site = fakeSite((call) =>
      call.url.endsWith("/ajax_getusertorrentlist.php")
        ? respond("denied", { status: 403 })
        : undefined,
    );
    const { pages, warnings, facts } = await autoCollect(
      ORIGIN,
      "NexusPHP",
      undefined,
      deps(site, { recordTabs: fakeTabs({ clickWorks: false }).recordTabs }),
    );
    expect(typesOf(pages)).not.toContain("seeding");
    expect(facts.seedingRequest).toBeNull();
    expect(warnings.some((w) => w.includes("做种") || /seeding/i.test(w))).toBe(true);
  });

  it("RSS 回的不是 RSS 时记提示、不保存", async () => {
    const site = fakeSite((call) =>
      call.url.includes("/torrentrss.php") ? respond("require passkey") : undefined,
    );
    const { pages, warnings } = await autoCollect(ORIGIN, "NexusPHP", undefined, deps(site));
    expect(typesOf(pages)).not.toContain("rss");
    expect(warnings.some((w) => /RSS/.test(w))).toBe(true);
  });

  it("下载种子回的是网页时按 download_notice 保存、记提示，不记 tracker", async () => {
    const site = fakeSite((call) =>
      call.url.includes("/download.php")
        ? respond(
            `<html><title>下载提示</title><a href="download.php?id=101&amp;letdown=1&amp;passkey=${PASSKEY}">继续下载</a></html>`,
          )
        : undefined,
    );
    const { pages, warnings, facts } = await autoCollect(ORIGIN, "NexusPHP", undefined, deps(site));
    const notice = pages.find((page) => page.pageType === "download_notice");
    expect(notice?.html).toContain("下载提示");
    expectNoSecrets(notice?.html ?? "");
    expect(facts.trackerHosts ?? []).toEqual([]);
    expect(warnings.some((w) => /种子|torrent/i.test(w))).toBe(true);
  });

  it("单个页面失败只记提示，不中断", async () => {
    const site = fakeSite((call) =>
      /\/(rules|faq|mybonus)\.php/.test(call.url) ? respond("err", { status: 500 }) : undefined,
    );
    const { pages, warnings } = await autoCollect(ORIGIN, "NexusPHP", undefined, deps(site));
    const types = typesOf(pages);
    expect(types).not.toContain("rules");
    expect(types).not.toContain("faq");
    expect(types).not.toContain("bonus");
    expect(types).toContain("seeding");
    expect(warnings.length).toBeGreaterThanOrEqual(3);
  });

  it("后台标签页出错、没有 Cookie 权限时只记提示", async () => {
    const site = fakeSite();
    const { pages, warnings, facts } = await autoCollect(
      ORIGIN,
      "NexusPHP",
      undefined,
      deps(site, {
        recordTabs: async () => {
          throw new Error("tabs boom");
        },
        cookieNames: async () => null,
      }),
    );
    expect(typesOf(pages)).toContain("seeding");
    expect(typesOf(pages)).not.toContain("rendered_search");
    expect(facts.cookieNames).toBeNull();
    expect(warnings.some((w) => w.includes("tabs boom"))).toBe(true);
    expect(warnings.some((w) => /cookie/i.test(w))).toBe(true);
  });
});

describe("autoCollect：用户名是常见词", () => {
  const LIST_WITH_FREE = `<html><body><table class="torrent_list"><tr class="tab_header"><td>名称</td></tr>
<tr><td class="t_name"><a href="details.php?id=101&amp;hit=1">A</a><img class="pro_free" alt="Free" /><span class="free">免费</span></td></tr>
<tr><td class="t_name"><a href="details.php?id=102&amp;hit=1">B</a></td></tr></table></body></html>`;

  it("用户名叫 free：不替换用户名（class、alt 不被换掉），ID 照换，提示用户提交前自己检查", async () => {
    const site = fakeSite((call) => {
      if (call.credentials === "omit") return undefined;
      const { pathname } = new URL(call.url);
      if (pathname === "/index.php") return respond(INDEX.replaceAll(USERNAME, "free"));
      if (pathname === "/userdetails.php") return respond(USERINFO.replaceAll(USERNAME, "free"));
      if (pathname === "/torrents.php") return respond(LIST_WITH_FREE);
      return undefined;
    });
    const { pages, warnings, facts, identity } = await autoCollect(
      ORIGIN,
      "NexusPHP",
      undefined,
      deps(site),
    );

    expect(identity).toEqual({ username: "free", userId: UID });
    expect(warnings).toContain(t("collector.warnUsernameCommon"));
    expect(facts.warnings).toEqual(warnings);
    const search = pages.find((page) => page.pageType === "search");
    expect(search?.html).toContain('class="free"');
    expect(search?.html).toContain('alt="Free"');
    expect(search?.html).not.toContain("ExampleUser");
    expect(pages.find((page) => page.pageType === "index")?.html).toContain("<b>free</b>");
    for (const page of pages) {
      expect(page.html, page.pageType).not.toContain(UID);
      expect(page.url, page.pageType).not.toContain(UID);
    }
  });

  it("正常用户名照旧替换，不给这条提示", async () => {
    const { pages, warnings } = await autoCollect(ORIGIN, "NexusPHP", undefined, deps(fakeSite()));
    expect(warnings).not.toContain(t("collector.warnUsernameCommon"));
    for (const page of pages) {
      expect(page.html, page.pageType).not.toContain(USERNAME);
    }
    expect(pages.find((page) => page.pageType === "index")?.html).toContain("ExampleUser");
  });
});

describe("autoCollect：请求记录的上限", () => {
  it("一页的请求记录超出上限时，从最后一条往前去掉参数和回应正文，前面的留全", async () => {
    // 30 个请求，每个回应几十 KB：整份远超上限
    const bigBody = (index: number) =>
      JSON.stringify({
        status: 1,
        rows: Array.from({ length: 400 }, (_, row) => ({
          tid: `t${index}-${row}`,
          sp_state: "pro_free",
        })),
      });
    const recordTabs: NonNullable<CollectorDeps["recordTabs"]> = async (origin, jobs) => {
      const base = await fakeTabs().recordTabs(origin, jobs);
      return base.map((recording) =>
        recording.key === "search"
          ? {
              ...recording,
              requests: Array.from({ length: 30 }, (_, index) => ({
                kind: "xhr" as const,
                method: "POST",
                url: `${ORIGIN}/ajax_promotion.php?page=${index}`,
                status: 200,
                contentType: "application/json",
                requestBody: `ids=${index}`,
                responseBody: bigBody(index),
                startedAt: 1000 + index,
              })),
            }
          : recording,
      );
    };
    const { pages } = await autoCollect(
      ORIGIN,
      "NexusPHP",
      undefined,
      deps(fakeSite(), { recordTabs }),
    );

    const searchLog = pages.find((page) => page.pageType === "requests_search")!;
    expect(searchLog.html.length).toBeLessThanOrEqual(REQUESTS_LOG_MAX_CHARS);
    const log = JSON.parse(searchLog.html);
    expect(log.requests).toHaveLength(30);
    expect(log.requests[0].omitted).toBeUndefined();
    expect(log.requests[0].response.rows).toHaveLength(400);
    expect(log.requests[0].params).toBe("ids=0");
    const last = log.requests.at(-1);
    expect(last).toMatchObject({
      omitted: true,
      method: "POST",
      url: `${ORIGIN}/ajax_promotion.php?page=29`,
      status: 200,
    });
    expect(last.response).toBeUndefined();
    expect(last.params).toBeUndefined();

    // 没超出上限的请求记录一条不少
    const userLog = JSON.parse(pages.find((page) => page.pageType === "requests_userinfo")!.html);
    expect(userLog.requests).toHaveLength(1);
    expect(userLog.requests[0].omitted).toBeUndefined();
    expect(userLog.requests[0].response).toBeDefined();
  });
});

describe("autoCollect：别的架构不变", () => {
  it("Unit3D 不补采这些页面，不开后台标签页，不读 Cookie 名", async () => {
    const calls: string[] = [];
    let tabs = 0;
    let cookies = 0;
    await autoCollect("https://u3d.example", "Unit3D", undefined, {
      fetch: async (input) => {
        calls.push(String(input));
        return respond(`<a href="/torrents/7">t</a><a href="/users/alice">alice</a>`);
      },
      sleep: async () => undefined,
      recordTabs: async () => {
        tabs += 1;
        return [];
      },
      cookieNames: async () => {
        cookies += 1;
        return [];
      },
    });
    expect(calls).toEqual([
      "https://u3d.example/",
      "https://u3d.example/torrents",
      "https://u3d.example/torrents/7",
      "https://u3d.example/users/alice",
    ]);
    expect(tabs).toBe(0);
    expect(cookies).toBe(0);
  });
});
