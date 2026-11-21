import JSZip from "jszip";
import { describe, expect, it } from "vitest";

import { getMessages, t } from "../../core/i18n";
import type { CapturedPage, CollectionSession, PageFormat, PageType } from "../../core/types";
import { MAX_USERNAME_HITS_PER_PAGE } from "../collector/sanitizer";
import { generateIssueBody } from "./github";
import { exportNotes } from "./identity";
import { createExportZip, exportFileNames } from "./zipper";

function page(pageType: PageType, html: string, url: string, format?: PageFormat): CapturedPage {
  return {
    pageType,
    url,
    html,
    capturedAt: "2026-11-21T00:00:00Z",
    detectedSchema: "NexusPHP",
    ...(format ? { format } : {}),
  };
}

const ORIGIN = "https://hdw.example";

describe("exportFileNames", () => {
  it("按格式给扩展名，计数后缀加在扩展名前面", () => {
    const pages = [
      page("script", "a", `${ORIGIN}/a.js`, "js"),
      page("script", "b", `${ORIGIN}/b.js`, "js"),
      page("script", "c", `${ORIGIN}/c.js`, "js"),
      page("rss", "<rss/>", `${ORIGIN}/torrentrss.php`, "xml"),
      page("requests_search", "{}", `${ORIGIN}/torrents.php`, "json"),
      page("seeding", "{}", `${ORIGIN}/ajax_getusertorrentlist.php`, "json"),
      page("logged_out", "<html/>", `${ORIGIN}/login.php`),
      page("unknown", "<html/>", `${ORIGIN}/x.php`),
      page("unknown", "<html/>", `${ORIGIN}/y.php`),
      page("rendered_userinfo", "<html/>", `${ORIGIN}/userdetails.php?id=1`),
      page("download_notice", "<html/>", `${ORIGIN}/download.php`),
      page("bonus", "<html/>", `${ORIGIN}/mybonus.php`),
    ];
    expect(exportFileNames(pages)).toEqual([
      "script.js",
      "script-2.js",
      "script-3.js",
      "rss.xml",
      "requests-search.json",
      "seeding.json",
      "logged-out.html",
      "unknown.html",
      "unknown-2.html",
      "rendered-userinfo.html",
      "download-notice.html",
      "bonus.html",
    ]);
  });

  it("旧版本存下的页面没有 format：脚本、RSS、请求记录按类型给扩展名，其余是 .html", () => {
    expect(
      exportFileNames([
        page("script", "a", `${ORIGIN}/a.js`),
        page("rss", "<rss/>", `${ORIGIN}/r`),
        page("requests_detail", "{}", `${ORIGIN}/d`),
        page("seeding", "<table/>", `${ORIGIN}/s`),
      ]),
    ).toEqual(["script.js", "rss.xml", "requests-detail.json", "seeding.html"]);
  });
});

/** 一键采集存下的会话：页面已脱敏，用户身份只在本地 */
function collectedSession(): CollectionSession {
  return {
    id: "s1",
    site: { name: "hdw.example", url: ORIGIN, schema: "NexusPHP", authMethod: "cookie" },
    createdAt: "2026-11-21T00:00:00Z",
    status: "complete",
    pages: [
      page(
        "index",
        `<div id="info_block"><a href="userdetails.php?id=4242">hdwing_tester</a></div>`,
        `${ORIGIN}/index.php`,
      ),
      page(
        "userinfo",
        "<title>hdwing_tester 的个人信息</title>",
        `${ORIGIN}/userdetails.php?id=4242`,
      ),
      page("script", "function doSign(){} // 签到", `${ORIGIN}/js/sign.js`, "js"),
      page(
        "seeding",
        JSON.stringify({ message: "hdwing_tester 做种", uid: 4242 }),
        `${ORIGIN}/ajax.php`,
        "json",
      ),
    ],
    identity: { username: "hdwing_tester", userId: "4242" },
    facts: {
      cookieNames: ["c_secure_pass", "c_secure_uid"],
      trackerHosts: ["tracker.hdw.example"],
      seedingRequest: {
        method: "POST",
        path: "/ajax_getusertorrentlist.php",
        params: ["userid", "type", "csrf"],
        source: "script",
        status: 200,
      },
      seedingAttempts: [],
      signinEntries: [
        { page: "index", tag: "a", text: "签到", href: "javascript:void(0)", onclick: "doSign()" },
      ],
      scriptEndpoints: ["/ajax_getusertorrentlist.php", "/attendance.php"],
      scripts: [{ url: `${ORIGIN}/js/sign.js`, bytes: 24, truncated: false, keywords: ["签到"] }],
      loggedOut: {
        requestedUrl: `${ORIGIN}/index.php`,
        finalUrl: `${ORIGIN}/login.php`,
        status: 200,
        redirected: true,
      },
      warnings: ["⚠ 没有 H&R 页"],
    },
  };
}

async function unzip(session: CollectionSession): Promise<Record<string, string>> {
  const zip = await JSZip.loadAsync(await (await createExportZip(session)).arrayBuffer());
  const files: Record<string, string> = {};
  for (const name of Object.keys(zip.files)) {
    files[name] = await zip.file(name)!.async("string");
  }
  return files;
}

describe("createExportZip", () => {
  it("site-info.json 带上新字段、文件名与新样本，不带用户身份", async () => {
    const files = await unzip(collectedSession());
    expect(Object.keys(files).sort()).toEqual([
      "index.html",
      "script.js",
      "seeding.json",
      "site-info.json",
      "userinfo.html",
    ]);
    const info = JSON.parse(files["site-info.json"]);
    expect(info.pages.map((p: { file: string }) => p.file)).toEqual([
      "index.html",
      "userinfo.html",
      "script.js",
      "seeding.json",
    ]);
    expect(info.cookieNames).toEqual(["c_secure_pass", "c_secure_uid"]);
    expect(info.trackerHosts).toEqual(["tracker.hdw.example"]);
    expect(info.seedingRequest).toMatchObject({
      method: "POST",
      path: "/ajax_getusertorrentlist.php",
    });
    expect(info.signinEntries[0]).toMatchObject({ text: "签到", onclick: "doSign()" });
    expect(info.scriptEndpoints).toEqual(["/ajax_getusertorrentlist.php", "/attendance.php"]);
    expect(info.loggedOut).toMatchObject({ finalUrl: `${ORIGIN}/login.php`, status: 200 });
    expect(info.capabilities).toMatchObject({
      index: true,
      userInfo: true,
      scripts: true,
      seeding: true,
      rss: false,
      loggedOut: false,
      renderedSearch: false,
    });
    expect(info.identity).toBeUndefined();
  });

  it("导出的文件与 site-info.json 里没有登录用户的用户名和 ID", async () => {
    const files = await unzip(collectedSession());
    for (const [name, content] of Object.entries(files)) {
      expect(content, name).not.toContain("hdwing_tester");
      expect(content, name).not.toContain("4242");
    }
    expect(files["userinfo.html"]).toContain("ExampleUser");
    expect(JSON.parse(files["seeding.json"]).uid).toBe(123456);
  });

  it("手动采集、会话里没有身份时，从页面的 info_block 认出来再换", async () => {
    const session = collectedSession();
    delete session.identity;
    delete session.facts;
    const files = await unzip(session);
    for (const [name, content] of Object.entries(files)) {
      expect(content, name).not.toContain("hdwing_tester");
      expect(content, name).not.toContain("4242");
    }
    const info = JSON.parse(files["site-info.json"]);
    expect(info.cookieNames).toBeUndefined();
  });

  it("Issue 正文里的页面地址不带用户 ID", () => {
    expect(generateIssueBody(collectedSession())).not.toContain("4242");
  });
});

/** 登录用户名是常见词 free 的会话：采集时没换用户名，ID 已换成示例值 */
function freeSession(): CollectionSession {
  return {
    id: "s2",
    site: { name: "hdw.example", url: ORIGIN, schema: "NexusPHP", authMethod: "cookie" },
    createdAt: "2026-11-21T00:00:00Z",
    status: "complete",
    pages: [
      page(
        "index",
        `<div id="info_block"><a href="userdetails.php?id=4242"><b>free</b></a></div>`,
        `${ORIGIN}/index.php`,
      ),
      page(
        "search",
        `<tr><td><img class="pro_free" alt="Free" /><span class="free">免费</span></td></tr>`,
        `${ORIGIN}/torrents.php`,
      ),
    ],
    identity: { username: "free", userId: "4242" },
    facts: {
      cookieNames: ["c_secure_uid"],
      // 采集时按中文存下的同一条提示：导出时不能重复
      warnings: ["⚠ 没有 H&R 页", getMessages("zh-CN")["collector.warnUsernameCommon"]],
    },
  };
}

describe("createExportZip：用户名是常见词", () => {
  it("不替换用户名、ID 照换，site-info.json 的 warnings 里提示一次", async () => {
    const files = await unzip(freeSession());
    expect(files["search.html"]).toContain('class="free"');
    expect(files["search.html"]).toContain('alt="Free"');
    expect(files["index.html"]).toContain("<b>free</b>");
    expect(files["index.html"]).not.toContain("4242");
    const info = JSON.parse(files["site-info.json"]);
    const note = t("collector.warnUsernameCommon");
    expect(info.warnings).toEqual(["⚠ 没有 H&R 页", note]);
    expect(exportNotes(freeSession())).toEqual([note]);
  });

  it("手动采集、会话里没有站点信息时也写进 warnings", async () => {
    const session = freeSession();
    delete session.facts;
    delete session.identity;
    const info = JSON.parse((await unzip(session))["site-info.json"]);
    expect(info.warnings).toEqual([t("collector.warnUsernameCommon")]);
    expect(info.cookieNames).toBeUndefined();
  });

  it("在一页里出现太多次的用户名也不换，并提示", async () => {
    const session = collectedSession();
    session.pages.push(
      page(
        "search",
        `<td class="hdwing_tester">x</td>`.repeat(MAX_USERNAME_HITS_PER_PAGE + 1),
        `${ORIGIN}/torrents.php`,
      ),
    );
    const files = await unzip(session);
    expect(files["userinfo.html"]).toContain("hdwing_tester");
    expect(JSON.parse(files["site-info.json"]).warnings).toContain(
      t("collector.warnUsernameCommon"),
    );
  });

  it("正常用户名照旧替换，没有这条提示", async () => {
    const files = await unzip(collectedSession());
    const info = JSON.parse(files["site-info.json"]);
    expect(info.warnings).toEqual(["⚠ 没有 H&R 页"]);
    expect(exportNotes(collectedSession())).toEqual([]);
    expect(files["userinfo.html"]).toContain("ExampleUser");
  });

  it("采集时存下了提示、导出时用户名换得掉（页面已经换过）：去掉旧提示", async () => {
    const session = collectedSession();
    session.facts!.warnings = [
      ...(session.facts!.warnings ?? []),
      getMessages("en-US")["collector.warnUsernameCommon"],
    ];
    const info = JSON.parse((await unzip(session))["site-info.json"]);
    expect(info.warnings).toEqual(["⚠ 没有 H&R 页"]);
  });
});
