import { describe, expect, it } from "vitest";

import {
  extractCsrfToken,
  extractOwnUsername,
  extractPhpEndpoints,
  extractSigninEntries,
  findDownloadLink,
  findSameOriginScripts,
  findSeedingHints,
  isForbiddenRequest,
  looksLikeLoginPage,
  looksLikeRss,
  scriptKeywords,
} from "./page-scan";

const ORIGIN = "https://pt.example";

describe("findSameOriginScripts", () => {
  it("只要同源脚本，跳过公共库，按出现顺序去重，带签到字样的文件名排在前面", () => {
    const html = `
      <script src="/js/jquery-3.6.0.min.js"></script>
      <script src="js/common.js?v=2"></script>
      <script type="text/javascript" src='https://pt.example/vendor/fullcalendar/main.min.js'></script>
      <script src="//cdn.other.example/echarts.min.js"></script>
      <script src="https://pt.example/js/bootstrap.bundle.js"></script>
      <script src="/js/attendance.js"></script>
      <script src="js/common.js?v=3"></script>
      <script>var inline = 1;</script>`;
    expect(findSameOriginScripts(html, `${ORIGIN}/index.php`, ORIGIN)).toEqual([
      `${ORIGIN}/js/attendance.js`,
      `${ORIGIN}/js/common.js?v=2`,
    ]);
  });

  it("不要 .js 以外、地址像签到的脚本", () => {
    const html = `<script src="/attendance.php?ajax=1"></script><script src="/js/ok.js"></script>`;
    expect(findSameOriginScripts(html, `${ORIGIN}/`, ORIGIN)).toEqual([`${ORIGIN}/js/ok.js`]);
  });
});

describe("scriptKeywords", () => {
  it("列出命中的关键词", () => {
    expect(
      scriptKeywords(`function doSign(){ $.get("attendance.php"); } // 签到 sp_state`),
    ).toEqual(["签到", "attendance", "sp_state"]);
  });

  it("没有命中时为空", () => {
    expect(scriptKeywords("function hello() { return 1; }")).toEqual([]);
  });
});

describe("extractSigninEntries", () => {
  it("记下签到入口的 href 与 onclick 摘要，地址脱敏", () => {
    const html = `
      <a href="javascript:void(0)" id="do-sign" class="faqlink" onclick="doSign('tok')">[签到得魔力]</a>
      <a href="attendance.php?sign=SIGN123">簽到</a>
      <a href="torrents.php">种子</a>
      <span class="checkin-btn" onclick="checkin()">打卡</span>`;
    const entries = extractSigninEntries(html, "index");
    expect(entries).toHaveLength(3);
    expect(entries[0]).toMatchObject({
      page: "index",
      tag: "a",
      text: "[签到得魔力]",
      href: "javascript:void(0)",
      onclick: "doSign('tok')",
      id: "do-sign",
    });
    expect(entries[1].href).toBe("attendance.php?sign=REMOVED");
    expect(entries[2]).toMatchObject({ tag: "span", text: "打卡", onclick: "checkin()" });
  });

  it("签到入口在 info_block 这类容器里面也认得出（容器本身不算）", () => {
    const html = `<div id="info_block">欢迎回来，<a href="userdetails.php?id=1"><b>u</b></a>
      <span class="nowrap"><a href="javascript:void(0)" onclick="doSign()">签到</a></span> 其它文字</div>`;
    expect(extractSigninEntries(html, "index")).toEqual([
      { page: "index", tag: "a", text: "签到", href: "javascript:void(0)", onclick: "doSign()" },
    ]);
  });

  it("没有签到入口时为空", () => {
    expect(extractSigninEntries(`<a href="signup.php">注册</a>`, "index")).toEqual([]);
  });
});

describe("findSeedingHints", () => {
  it("认出脚本里 POST 的列表接口和参数名", () => {
    const js = `
      function getusertorrentlist(type, blockid) {
        $.post("ajax_getusertorrentlist.php", {userid: CURUSER_ID, type: type, csrf: $('meta[name="x-csrf"]').attr('content'), page: 0}, function (d) {
          $('#' + blockid).html(d.message);
        }, "json");
      }`;
    expect(findSeedingHints(js)).toEqual([
      {
        path: "ajax_getusertorrentlist.php",
        method: "POST",
        params: [
          { name: "userid" },
          { name: "type" },
          { name: "csrf" },
          { name: "page", literal: "0" },
        ],
      },
    ]);
  });

  it("认出 $.ajax 的写法与请求头里的 CSRF", () => {
    const js = `$.ajax({ url: '/api/torrentlist.php', type: 'POST', headers: {'X-CSRF-TOKEN': token}, data: { uid: id, type: 'seeding' } });`;
    expect(findSeedingHints(js)).toEqual([
      {
        path: "/api/torrentlist.php",
        method: "POST",
        params: [{ name: "uid" }, { name: "type", literal: "seeding" }],
        csrfHeader: "X-CSRF-TOKEN",
      },
    ]);
  });

  it("GET 的地址里带参数", () => {
    const js = `xhr.open("GET", "getseedlist_torrentlist.php?userid=" + uid + "&type=seeding");`;
    expect(findSeedingHints(js)).toEqual([
      {
        path: "getseedlist_torrentlist.php",
        method: "GET",
        params: [{ name: "userid" }, { name: "type" }],
      },
    ]);
  });

  it("上游通用接口不算别的接口", () => {
    expect(
      findSeedingHints(`ajax.get("getusertorrentlistajax.php?userid=1&type=seeding")`),
    ).toEqual([]);
  });
});

describe("extractPhpEndpoints", () => {
  it("只收同源的 .php 接口，去掉参数，排序去重", () => {
    const js = `
      $.post("ajax_promotion.php", {});
      $.get('/attendance.php?action=sign');
      fetch("https://pt.example/api/v2/user.php?x=1");
      fetch("https://other.example/evil.php");
      location.href = "details.php?id=" + id;
      $.post("ajax_promotion.php");`;
    expect(extractPhpEndpoints(js, ORIGIN)).toEqual([
      "/ajax_promotion.php",
      "/api/v2/user.php",
      "/attendance.php",
      "/details.php",
    ]);
  });
});

describe("findDownloadLink", () => {
  it.each([
    [
      `<a class="index" href="download.php?id=1&amp;passkey=abc">x</a>`,
      `${ORIGIN}/download.php?id=1&passkey=abc`,
    ],
    [`<a href='download.php?hash=HASH1'>下载</a>`, `${ORIGIN}/download.php?hash=HASH1`],
    [
      `<a href="https://pt.example/download.php?downhash=1.x">dl</a>`,
      `${ORIGIN}/download.php?downhash=1.x`,
    ],
  ])("%s", (html, expected) => {
    expect(findDownloadLink(html, `${ORIGIN}/details.php?id=1`)).toBe(expected);
  });

  it("没有下载链接或不同源时为 null", () => {
    expect(findDownloadLink(`<a href="details.php?id=1">x</a>`, `${ORIGIN}/`)).toBeNull();
    expect(
      findDownloadLink(`<a href="https://evil.example/download.php?id=1">x</a>`, `${ORIGIN}/`),
    ).toBeNull();
  });
});

describe("extractCsrfToken", () => {
  it.each([
    [`<meta name="x-csrf" content="META_TOKEN">`, "META_TOKEN"],
    [`<meta content='T2' name='csrf-token'>`, "T2"],
    [`<input type="hidden" name="csrf" value="HIDDEN_TOKEN">`, "HIDDEN_TOKEN"],
    [`<input value="T4" name="_token" type="hidden">`, "T4"],
  ])("%s", (html, expected) => {
    expect(extractCsrfToken(html)).toBe(expected);
  });

  it("没有令牌时为 null", () => {
    expect(extractCsrfToken(`<meta name="description" content="x">`)).toBeNull();
  });
});

describe("extractOwnUsername", () => {
  it("从 info_block 里本人的链接取用户名", () => {
    const html = `<div id="info_block">欢迎回来，<span class="nowrap"><a href="userdetails.php?id=4242" class='User_Name'><b>hdwing&amp;tester</b></a></span>
      <a href="userdetails.php?id=1">admin</a></div>`;
    expect(extractOwnUsername(html, "4242")).toBe("hdwing&tester");
  });

  it("找不到时为 null", () => {
    expect(extractOwnUsername(`<a href="userdetails.php?id=1">x</a>`, "4242")).toBeNull();
  });
});

describe("looksLikeRss / looksLikeLoginPage", () => {
  it("RSS 与网页分得开", () => {
    expect(looksLikeRss(`<?xml version="1.0"?><rss version="2.0"></rss>`, "text/xml")).toBe(true);
    expect(looksLikeRss(`<rss version="2.0"><channel></channel></rss>`, "text/html")).toBe(true);
    expect(looksLikeRss("require passkey", "text/html; charset=utf-8")).toBe(false);
    expect(looksLikeRss("<!DOCTYPE html><html><body>login</body></html>", "application/xml")).toBe(
      false,
    );
  });

  it("认出登录页", () => {
    expect(looksLikeLoginPage(`<form action="takelogin.php" method="post">`)).toBe(true);
    expect(looksLikeLoginPage(`<table class="torrents"></table>`)).toBe(false);
  });
});

describe("isForbiddenRequest", () => {
  it.each([
    [`${ORIGIN}/attendance.php`, true],
    [`${ORIGIN}/attendance.php?action=sign`, true],
    [`${ORIGIN}/plugin/sign_in`, true],
    [`${ORIGIN}/qiandao.php`, true],
    [`${ORIGIN}/logout.php`, true],
    [`${ORIGIN}/js/attendance.js?v=1`, false],
    [`${ORIGIN}/getusertorrentlistajax.php?userid=1&type=seeding`, false],
    [`${ORIGIN}/torrentrss.php?rows=10`, false],
    [`${ORIGIN}/download.php?hash=abc`, false],
  ])("%s → %s", (url, forbidden) => {
    expect(isForbiddenRequest(url)).toBe(forbidden);
  });
});
