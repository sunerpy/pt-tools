import { describe, expect, it } from "vitest";

import {
  EXAMPLE_USERNAME,
  EXAMPLE_USER_ID,
  MAX_USERNAME_HITS_PER_PAGE,
  countUsername,
  isCommonUsername,
  planIdentity,
  redactIdentity,
  sanitizeHtml,
  sanitizeJsonText,
  sanitizeParams,
  sanitizeUrl,
} from "./sanitizer";

/**
 * Counts `<tr` start tags that a real HTML tokenizer would see, i.e. tags that are
 * NOT sitting inside an attribute value. Mirrors how a parser handles quoted
 * attributes: once a quote opens inside a tag, everything up to the matching quote
 * belongs to the attribute value, `<tr>` included.
 *
 * Used instead of DOMParser because this package's vitest environment is plain
 * node (no jsdom/happy-dom dependency). Sanitization must never change this count.
 */
function countRowsOutsideAttributes(html: string): number {
  let count = 0;
  let inTag = false;
  let quote: string | null = null;

  for (let i = 0; i < html.length; i++) {
    const ch = html[i];

    if (quote !== null) {
      if (ch === quote) {
        quote = null;
      }
      continue;
    }

    if (inTag) {
      if (ch === '"' || ch === "'") {
        quote = ch;
      } else if (ch === ">") {
        inTag = false;
      }
      continue;
    }

    if (ch === "<") {
      inTag = true;
      if (/^<tr[\s>]/i.test(html.slice(i, i + 4))) {
        count++;
      }
    }
  }

  return count;
}

type SanitizeCase = {
  name: string;
  html: string;
  /** Secret fragments that must not survive sanitization (privacy control). */
  mustNotContain: string[];
  /** Structural fragments that must survive sanitization (document integrity). */
  mustContain: string[];
};

const cases: SanitizeCase[] = [
  {
    name: "single-quoted attribute keeps its closing quote and bracket",
    html: `<a class="index" href='download.php?id=113372&passkey=abcdef0123456789'>torrent</a>`,
    mustNotContain: ["abcdef0123456789"],
    mustContain: ["'>", "torrent</a>"],
  },
  {
    name: "double-quoted attribute keeps its closing quote and bracket",
    html: `<a class="index" href="download.php?id=113372&passkey=abcdef0123456789">torrent</a>`,
    mustNotContain: ["abcdef0123456789"],
    mustContain: ['">', "torrent</a>"],
  },
  {
    name: "unquoted attribute keeps its closing bracket",
    html: `<a href=download.php?id=113372&passkey=abcdef0123456789>torrent</a>`,
    mustNotContain: ["abcdef0123456789"],
    mustContain: [">torrent</a>"],
  },
  {
    name: "json passkey keeps its surrounding quotes",
    html: `{"passkey":"abcdef0123456789","id":113372}`,
    mustNotContain: ["abcdef0123456789"],
    mustContain: [`"passkey":"REMOVED"`, `"id":113372`],
  },
  {
    name: "json passkey value containing an apostrophe is fully redacted",
    html: `{"passkey":"ab'cdef0123456789","id":113372}`,
    mustNotContain: ["ab'cdef0123456789", "cdef0123456789"],
    mustContain: [`"passkey":"REMOVED"`, `"id":113372`],
  },
  {
    name: "bare query string keeps the following parameters",
    html: `<a href='rss.php?passkey=abcdef0123456789&other=1'>rss</a>`,
    mustNotContain: ["abcdef0123456789"],
    mustContain: ["&other=1", "'>", "rss</a>"],
  },
  {
    name: "authkey in a single-quoted attribute keeps its closing quote",
    html: `<a href='download.php?id=1&authkey=deadbeefcafe1234'>dl</a>`,
    mustNotContain: ["deadbeefcafe1234"],
    mustContain: ["'>", "dl</a>"],
  },
  {
    name: "json authkey keeps its surrounding quotes",
    html: `{"authkey":"deadbeefcafe1234"}`,
    mustNotContain: ["deadbeefcafe1234"],
    mustContain: [`"authkey":"REMOVED"`],
  },
  {
    name: "apikey in a single-quoted attribute keeps its closing quote",
    html: `<a href='api.php?apikey=deadbeefcafe1234'>api</a>`,
    mustNotContain: ["deadbeefcafe1234"],
    mustContain: ["'>", "api</a>"],
  },
  {
    name: "json apikey keeps its surrounding quotes",
    html: `{"apikey":"deadbeefcafe1234"}`,
    mustNotContain: ["deadbeefcafe1234"],
    mustContain: [`"apikey":"REMOVED"`],
  },
  {
    name: "json token keeps its surrounding quotes",
    html: `{"token":"deadbeefcafe1234","id":1}`,
    mustNotContain: ["deadbeefcafe1234"],
    mustContain: [`"token":"REMOVED"`, `"id":1`],
  },
];

describe("sanitizeHtml", () => {
  it.each(cases)("$name", ({ html, mustNotContain, mustContain }) => {
    const sanitized = sanitizeHtml(html);

    for (const secret of mustNotContain) {
      expect(sanitized).not.toContain(secret);
    }
    expect(sanitized).toContain("REMOVED");
    for (const fragment of mustContain) {
      expect(sanitized).toContain(fragment);
    }
  });

  // Regression for the pt.eastgame.org capture (issue #502): the passkey regex
  // consumed the closing `'>` of the download link, so the parser absorbed the
  // rows that followed and 副标题 / 基本信息 / 行为 disappeared from the export.
  const detailPage = [
    `<table class="main">`,
    `<tr><td class="rowhead">下载直链</td><td class="rowfollow"><a class="index" href='download.php?id=113372&passkey=abcdef0123456789'>Witch.from.Nepal.1986.mkv.torrent</a></td></tr>`,
    `<tr><td class="rowhead">副标题</td><td class="rowfollow">奇缘</td></tr>`,
    `<tr><td class="rowhead">基本信息</td><td class="rowfollow"><b>大小：</b>1.95 GB</td></tr>`,
    `<tr><td class="rowhead">行为</td><td class="rowfollow"><input name="torrent_name" value="Witch.from.Nepal.1986.mkv" /></td></tr>`,
    `</table>`,
  ].join("\n");

  it("does not swallow the rows following a single-quoted passkey link", () => {
    const sanitized = sanitizeHtml(detailPage);

    expect(countRowsOutsideAttributes(sanitized)).toBe(countRowsOutsideAttributes(detailPage));
    expect(countRowsOutsideAttributes(sanitized)).toBe(4);
    expect(sanitized).toContain("副标题");
    expect(sanitized).toContain("基本信息");
    expect(sanitized).toContain(`<input name="torrent_name"`);
    expect(sanitized).not.toContain("abcdef0123456789");
  });
});

/*
 * 页面地址原样进 site-info.json 和 Issue 正文：详情、下载页的地址常带 passkey 等凭证。
 * HTML 原来只按固定的几种写法脱敏，密码框、CSRF 隐藏字段与 meta 里的值照样留着。
 */
describe("sanitizeUrl", () => {
  it("把查询参数里的凭证换成 REMOVED，其余参数保留", () => {
    const out = sanitizeUrl(
      "https://tracker.example/download.php?id=42&passkey=0123456789abcdef0123&token=t0k&downhash=dh",
    );
    expect(out).toContain("id=42");
    expect(out).not.toContain("0123456789abcdef0123");
    expect(out).not.toContain("t0k");
    expect(out).not.toContain("=dh");
    expect(out).toContain("passkey=REMOVED");
  });

  it("去掉地址里的用户名与密码", () => {
    expect(sanitizeUrl("https://alice:hunter2@tracker.example/index.php")).not.toContain("hunter2");
  });

  it("参数里套着的地址、IP 也脱敏，站点本身的主机与路径不动", () => {
    const out = sanitizeUrl(
      "http://198.51.100.7:8080/go.php?to=https%3A%2F%2Ft.example%2Fdl.php%3Fid%3D1%26torrent_pass%3DTP0021&from=203.0.113.9",
    );
    expect(out.startsWith("http://198.51.100.7:8080/go.php?")).toBe(true);
    expect(out).not.toContain("TP0021");
    expect(out).not.toContain("203.0.113.9");
  });

  it("解析不了的地址也做字符串脱敏", () => {
    expect(sanitizeUrl("not a url passkey=0123456789abcdef0123")).not.toContain(
      "0123456789abcdef0123",
    );
  });

  // 清空凭证参数时整串查询参数会重新编码，IPv6 的冒号、邮箱的 @ 变成 %3A、%40：要在编码前脱敏
  it("同一地址里有凭证参数时，别的参数里的 IPv6 与邮箱也脱敏", () => {
    const out = sanitizeUrl(
      "https://t.example/ajax.php?csrf=CSRF0201&ip=2001:db8:85a3::8a2e:370:7334&mail=qa@tester.example&ids[]=1&ids[]=2",
    );
    expect(out).not.toContain("CSRF0201");
    expect(out).not.toContain("8a2e");
    expect(out).not.toContain("tester.example");
    expect(out.startsWith("https://t.example/ajax.php?")).toBe(true);
    // 同名参数都留着
    expect(new URL(out).searchParams.getAll("ids[]")).toEqual(["1", "2"]);
  });
});

describe("sanitizeHtml: 表单与 meta 里的秘密", () => {
  it("清空密码框与可疑隐藏字段的值，普通隐藏字段保留", () => {
    const out = sanitizeHtml(
      '<form><input type="password" name="pw" value="hunter2">' +
        "<input type='hidden' name='csrf_token' value='abc123xyz'>" +
        '<input type="hidden" name="id" value="42"></form>',
    );
    expect(out).not.toContain("hunter2");
    expect(out).not.toContain("abc123xyz");
    expect(out).toContain('value="42"');
    expect(out).toContain("</form>");
  });

  it("清空 CSRF meta 的 content", () => {
    const out = sanitizeHtml(
      '<head><meta name="csrf-token" content="s3cr3tvalue"><title>x</title></head>',
    );
    expect(out).not.toContain("s3cr3tvalue");
    expect(out).toContain("<title>x</title>");
  });
});

/*
 * HDWinG（issue #910）的附件里漏了报告人的 IPv6 地址：原来只认 IPv4。
 * 时间（12:34:56）、CSS 伪元素（::before）、NexusPHP 标题里的「 :: 」不能被当成 IPv6 换掉。
 */
describe("sanitizeHtml: IPv6", () => {
  it.each([
    ["完整写法", "最近 IP：2001:0db8:85a3:0000:0000:8a2e:0370:7334 。", "2001:0db8"],
    ["简写", "<td>2001:db8:85a3::8a2e:370:7334</td>", "8a2e:370:7334"],
    ["只有后半段", "地址 ::ffff:c000:280 记下了", "c000:280"],
    ["链路本地带网卡名", "fe80::1ff:fe23:4567:890a%eth0", "fe23:4567"],
    ["带方括号和端口", "http://[2001:db8::42]:8080/announce", "2001:db8::42"],
    ["IPv4 映射", "来自 ::ffff:192.0.2.128 的访问", "192.0.2.128"],
    ["大写", "2001:DB8:0:0:8:800:200C:417A", "200C:417A"],
  ])("%s", (_name, html, secret) => {
    const out = sanitizeHtml(html);
    expect(out).not.toContain(secret);
    expect(out).toContain("::1");
  });

  it("带方括号的地址替换后方括号与端口还在", () => {
    expect(sanitizeHtml("http://[2001:db8::42]:8080/announce")).toBe("http://[::1]:8080/announce");
  });

  it.each([
    ["时间", "发布于 2026-11-21 12:34:56，剩余 01:02:03"],
    ["CSS 伪元素", "a.tag::before{content:''} p::after{color:red} li::marker{}"],
    ["NexusPHP 标题分隔符", "<title>HDWinG :: 首页 - Powered by NexusPHP</title>"],
    ["C++ 作用域", "std::string name; Foo::Bar();"],
    ["比例与端口", "分享率 1:2，端口 :8080"],
    ["长的冒号十六进制串", "fingerprint aa:bb:cc:dd:ee:ff:00:11:22:33:44:55"],
  ])("不误伤%s", (_name, html) => {
    expect(sanitizeHtml(html)).toBe(html);
  });
});

/*
 * 正文里出现的地址（RSS 的 link / enclosure、脚本里的字符串、JSON 转义过的地址）都按凭证参数名清空值，
 * 不只是 passkey：downhash、torrent_pass、tp、rsskey、sign、hash、csrf 等。
 */
describe("sanitizeHtml: 正文地址里的凭证参数", () => {
  it.each([
    [
      "RSS 的 enclosure",
      `<enclosure url="https://t.example/download.php?id=1&amp;downhash=DH0001&amp;foo=1" length="1" type="application/x-bittorrent" />`,
      "DH0001",
      ["&amp;foo=1", `type="application/x-bittorrent"`],
    ],
    [
      "RSS 的 link 带 rsskey",
      "<link>https://t.example/torrentrss.php?rows=10&amp;rsskey=RSSKEY0002&amp;linktype=dl</link>",
      "RSSKEY0002",
      ["linktype=dl", "</link>"],
    ],
    [
      "脚本字符串里的 torrent_pass 与 tp",
      `var u = "/download.php?id=1&torrent_pass=TP0003&tp=TP0004";`,
      "TP0003",
      ["id=1"],
    ],
    ["签到地址的 sign", `<a href="attendance.php?sign=SIGN0005">x</a>`, "SIGN0005", [`">x</a>`]],
    [
      "签名下载链接的 hash",
      `<a href='download.php?hash=HASH0006'>dl</a>`,
      "HASH0006",
      ["'>dl</a>"],
    ],
    ["csrf 参数", `fetch("/ajax.php?csrf=CSRF0007&x=1")`, "CSRF0007", ["&x=1"]],
    [
      "JSON 转义的 & ",
      `{"url":"https:\\/\\/t.example\\/dl.php?id=1\\u0026tp=TP0008"}`,
      "TP0008",
      [`"url":`],
    ],
    [
      "编码进另一个地址里的参数",
      "go.php?to=https%3A%2F%2Ft.example%2Fdl.php%3Fid%3D1%26torrent_pass%3DTP0009",
      "TP0009",
      ["go.php?to="],
    ],
  ])("%s", (_name, html, secret, keep) => {
    const out = sanitizeHtml(html);
    expect(out).not.toContain(secret);
    for (const fragment of keep) {
      expect(out).toContain(fragment);
    }
  });

  it("TP0003 与 TP0004 都清掉", () => {
    const out = sanitizeHtml(`var u = "/download.php?id=1&torrent_pass=TP0003&tp=TP0004";`);
    expect(out).not.toContain("TP0004");
  });

  it("普通参数和名字里带 key 的普通属性不动", () => {
    const html = `<a href="torrents.php?cat=401&amp;type=seeding&amp;id=42&amp;monkey=1" data-key="row-1" class="token-list">x</a>`;
    expect(sanitizeHtml(html)).toBe(html);
  });
});

describe("sanitizeHtml: 脚本里直接写的凭证", () => {
  it("清空 passkey、csrf 这类变量和对象字段的字符串值", () => {
    const out = sanitizeHtml(
      `<script>var passkey = "abcdef123456"; $.post("x.php", {csrf: 'CSRF0010', userid: 1});</script>`,
    );
    expect(out).not.toContain("abcdef123456");
    expect(out).not.toContain("CSRF0010");
    expect(out).toContain("userid: 1");
  });

  it("普通变量不动", () => {
    const html = `<script>var title = "hello"; var type = 'seeding';</script>`;
    expect(sanitizeHtml(html)).toBe(html);
  });

  /*
   * 键名带引号的写法：JSON 风格的对象字面量、jQuery 的 ajaxSetup 请求头，
   * 以及记录到的回应超过上限被截断、解析不了只能按正文脱敏的 JSON。
   */
  it.each([
    ["JSON 风格的对象", `<script>var cfg = {"csrf": "TOK0101"};</script>`],
    ["ajaxSetup 请求头", `<script>$.ajaxSetup({headers: {'X-CSRF-TOKEN': 'TOK0102'}});</script>`],
    ["带下划线的 _token", `<script>var data = {'_token': 'TOK0103'};</script>`],
    ["键名前后有空格", `<script>config = { 'csrf_token' : "TOK0104" }</script>`],
    ["请求头 X-CSRF", `<script>let headers = {"X-CSRF": "TOK0105"}</script>`],
    ["截断的 JSON", `{"status":1,"downhash":"TOK0106","id":101,"title":"A`],
  ])("键名带引号时也清空：%s", (_name, html) => {
    const out = sanitizeHtml(html);
    expect(out).not.toMatch(/TOK01\d\d/);
    expect(out).toContain("REMOVED");
  });

  it("键名带引号的普通字段不动", () => {
    const html = `<script>var row = {"title": "hello", 'type': 'seeding', "tokens": "a,b"};</script>`;
    expect(sanitizeHtml(html)).toBe(html);
  });
});

describe("sanitizeParams: 记录到的请求参数", () => {
  it("表单参数清空 csrf，其余保留", () => {
    const out = sanitizeParams("userid=4242&type=seeding&csrf=CSRF0011&ids%5B0%5D=7");
    expect(out).not.toContain("CSRF0011");
    expect(out).toContain("userid=4242");
    expect(out).toContain("type=seeding");
    expect(out).toContain("ids%5B0%5D=7");
  });

  it("JSON 参数清空凭证字段", () => {
    const out = sanitizeParams(`{"csrf":"CSRF0012","ids":[1,2],"passkey":"PK0013"}`);
    expect(out).not.toContain("CSRF0012");
    expect(out).not.toContain("PK0013");
    expect(JSON.parse(out).ids).toEqual([1, 2]);
  });

  it("空参数原样返回", () => {
    expect(sanitizeParams("")).toBe("");
  });
});

describe("sanitizeJsonText", () => {
  it("清空凭证字段，字符串里的 HTML 也脱敏，结果仍是 JSON", () => {
    const raw = JSON.stringify({
      status: 1,
      message: `<a href="download.php?id=1&passkey=PK0014">dl</a> 来自 203.0.113.9`,
      passkey: "PK0015",
      info: { downhash: "DH0016", size: 42 },
    });
    const out = sanitizeJsonText(raw);
    for (const secret of ["PK0014", "PK0015", "DH0016", "203.0.113.9"]) {
      expect(out).not.toContain(secret);
    }
    const parsed = JSON.parse(out);
    expect(parsed.status).toBe(1);
    expect(parsed.info.size).toBe(42);
  });

  it("不是 JSON 时按正文脱敏", () => {
    expect(sanitizeJsonText("oops passkey=0123456789abcdef0123")).not.toContain(
      "0123456789abcdef0123",
    );
  });
});

/*
 * 登录用户自己的用户名与用户 ID：导出的页面里换成固定的示例值。
 * 用户名只换文字和属性值（不碰标签名），ID 只在 userdetails.php?id=、userid=、uid= 这类上下文里换。
 */
describe("redactIdentity", () => {
  const identity = { username: "hdwing_tester", userId: "4242" };

  it("换掉首页 info_block、标题与列表请求里的用户名和 ID", () => {
    const html =
      `<title>hdwing_tester 的个人信息</title><div id="info_block">欢迎 ` +
      `<a href="userdetails.php?id=4242" class="User_Name" title="hdwing_tester"><b>hdwing_tester</b></a>` +
      ` <a href="javascript: getusertorrentlistajax('4242', 'seeding', 'ka1')">做种</a>` +
      ` <a href="getusertorrentlistajax.php?userid=4242&amp;type=seeding">x</a></div>`;
    const out = redactIdentity(html, identity, "html");
    expect(out).not.toContain("hdwing_tester");
    expect(out).not.toContain("4242");
    expect(out).toContain(`userdetails.php?id=${EXAMPLE_USER_ID}`);
    expect(out).toContain(`<b>${EXAMPLE_USERNAME}</b>`);
    expect(out).toContain(`title="${EXAMPLE_USERNAME}"`);
  });

  it.each([
    ["uid 参数", "announce.php?uid=4242&x=1", "uid=4242"],
    ["user_id 参数", "api.php?user_id=4242", "user_id=4242"],
    ["JSON 数字", `{"uid":4242,"size":1}`, `"uid":4242`],
    ["JSON 字符串", `{"userid":"4242"}`, `"userid":"4242"`],
    ["脚本变量", "var uid = 4242;", "uid = 4242"],
    ["属性", `<div data-uid="4242">`, `uid="4242"`],
  ])("ID 上下文：%s", (_name, text, gone) => {
    const out = redactIdentity(text, identity, "json");
    expect(out).not.toContain(gone);
    expect(out).toContain(EXAMPLE_USER_ID);
  });

  it("别人的 ID、更长的数字和别处的同样数字不动", () => {
    const html = `<a href="userdetails.php?id=42421">a</a><a href="userdetails.php?id=777">b</a> 大小 4242 MB uid=42420`;
    expect(redactIdentity(html, identity, "html")).toBe(html);
  });

  // 注释、脚本与样式的内容、CDATA 里没有标签，按文字换；不带引号的属性值也换
  it.each([
    ["HTML 注释", "<!-- logged in as hdwing_tester -->"],
    ["脚本里有小于号", `<script>for (var i=0;i<n;i++){} alert("hdwing_tester");</script>`],
    ["脚本里的对象字段", "<script>if (a<b) { CURUSER = {name: 'hdwing_tester'} }</script>"],
    ["不带引号的属性值", "<div title=hdwing_tester>x</div>"],
    ["RSS 的 CDATA", "<description><![CDATA[ hdwing_tester <b>发布</b> ]]></description>"],
  ])("用户名在%s里也换", (_name, html) => {
    const out = redactIdentity(html, identity, "html");
    expect(out).not.toContain("hdwing_tester");
    expect(out).toContain(EXAMPLE_USERNAME);
  });

  it("用户名正好是 script 时不碰脚本标签", () => {
    const out = redactIdentity(
      "<script>var who = 'script';</script>",
      { username: "script", userId: "1" },
      "html",
    );
    expect(out).toBe(`<script>var who = '${EXAMPLE_USERNAME}';</script>`);
  });

  it("用户名正好是标签名时不碰标签", () => {
    const out = redactIdentity(
      "<title>title 的个人信息</title>",
      { username: "title", userId: "1" },
      "html",
    );
    expect(out).toBe(`<title>${EXAMPLE_USERNAME} 的个人信息</title>`);
  });

  it("英文用户名不分大小写，只换整个词", () => {
    const out = redactIdentity(
      "Hdwing_Tester 和 hdwing_testers 以及 pre_hdwing_tester",
      identity,
      "html",
    );
    expect(out).toBe(`${EXAMPLE_USERNAME} 和 hdwing_testers 以及 pre_hdwing_tester`);
  });

  it("中文用户名连着别的字也换，地址里编码过的也换", () => {
    const out = redactIdentity(
      "张三丰的个人信息 search.php?user=%E5%BC%A0%E4%B8%89%E4%B8%B0",
      { username: "张三丰", userId: "9" },
      "html",
    );
    expect(out).not.toContain("张三丰");
    expect(out).not.toContain("%E5%BC%A0%E4%B8%89%E4%B8%B0");
  });

  it.each([
    ["太短", "ab"],
    ["纯数字", "12345"],
  ])("用户名%s时不换用户名", (_name, username) => {
    const text = `欢迎 ${username} 回来`;
    expect(redactIdentity(text, { username, userId: "4242" }, "html")).toBe(text);
  });
});

/*
 * 用户名是常见词时，按整词连属性值一起换会把页面换坏：叫 free 的用户导出的列表里 class="free"、alt="Free"
 * 都成了 ExampleUser，优惠认不出来。这种用户名不换，提示用户提交前自己检查。
 */
describe("常见词用户名", () => {
  const listing = `<tr><td><img class="pro_free" alt="Free" /><span class="free">免费</span></td>
<td><a href="userdetails.php?id=4242"><b>free</b></a></td></tr>`;

  it.each(["free", "movie", "user", "admin", "test", "guest", "vip", "seed", "torrent"])(
    "%s 是常见词：不换用户名，用户 ID 照换，并要提示",
    (username) => {
      const plan = planIdentity({ username, userId: "4242" }, [listing]);
      expect(plan.usernameKept).toBe(true);
      expect(plan.identity).toEqual({ username: null, userId: "4242" });
    },
  );

  it("不分大小写", () => {
    expect(isCommonUsername("Free")).toBe(true);
    expect(isCommonUsername("ADMIN")).toBe(true);
    expect(planIdentity({ username: "Torrent", userId: "1" }, []).usernameKept).toBe(true);
  });

  it("直接调 redactIdentity 时常见词也不换：class 与 alt 留着，ID 照换", () => {
    const out = redactIdentity(listing, { username: "free", userId: "4242" }, "html");
    expect(out).toContain('class="free"');
    expect(out).toContain('alt="Free"');
    expect(out).toContain("<b>free</b>");
    expect(out).not.toContain("4242");
    expect(out).toContain(`userdetails.php?id=${EXAMPLE_USER_ID}`);
  });

  it("在一页里作为整词出现太多次的不换，并要提示", () => {
    const page = `<span class="zebra">x</span>`.repeat(MAX_USERNAME_HITS_PER_PAGE + 1);
    const plan = planIdentity({ username: "zebra", userId: "7" }, ["<b>zebra</b>", page]);
    expect(plan.usernameKept).toBe(true);
    expect(plan.identity?.username).toBeNull();
  });

  it("每页都没超过上限的照换：自己发布的种子占满一页列表（100 行），几页合起来再多也照换", () => {
    const fullListing = `<tr><td><a href="userdetails.php?id=7"><b>zebra</b></a></td></tr>`.repeat(
      100,
    );
    const texts = [fullListing, fullListing, fullListing, `<title>zebra 的个人信息</title>`];
    const plan = planIdentity({ username: "zebra", userId: "7" }, texts);
    expect(plan.usernameKept).toBe(false);
    expect(plan.identity).toEqual({ username: "zebra", userId: "7" });
    expect(redactIdentity(fullListing, plan.identity, "html")).not.toContain("zebra");
  });

  it("正好等于上限的还算用户名", () => {
    const page = "zebra ".repeat(MAX_USERNAME_HITS_PER_PAGE);
    expect(planIdentity({ username: "zebra", userId: "7" }, [page]).usernameKept).toBe(false);
  });

  it("正常用户名照旧替换，不提示", () => {
    const identity = { username: "hdwing_tester", userId: "4242" };
    const html = `<a href="userdetails.php?id=4242"><b>hdwing_tester</b></a> free movie`;
    const plan = planIdentity(identity, [html]);
    expect(plan).toEqual({ identity, usernameKept: false });
    const out = redactIdentity(html, plan.identity, "html");
    expect(out).not.toContain("hdwing_tester");
    expect(out).toContain(EXAMPLE_USERNAME);
    expect(out).toContain("free movie");
  });

  it("用户名里含常见词、但本身不是常见词的照换", () => {
    for (const username of ["free_rider", "admin2", "movielover"]) {
      expect(isCommonUsername(username)).toBe(false);
      expect(planIdentity({ username, userId: "1" }, [`<b>${username}</b>`]).usernameKept).toBe(
        false,
      );
    }
  });

  it("太短、纯数字的用户名本来就不换，也不提示（和以前一样）", () => {
    expect(planIdentity({ username: "ab", userId: "1" }, []).usernameKept).toBe(false);
    expect(planIdentity({ username: "12345", userId: "1" }, []).usernameKept).toBe(false);
    expect(planIdentity(null, []).identity).toBeNull();
  });

  it("countUsername 和替换时一样认：英文按整词不分大小写，中文按原样，编码过的也算", () => {
    expect(countUsername("Zebra zebra zebras pre_zebra zebra_x ZEBRA", "zebra")).toBe(3);
    expect(
      countUsername("张三丰来了，张三丰的种子 user=%E5%BC%A0%E4%B8%89%E4%B8%B0", "张三丰"),
    ).toBe(3);
  });
});
