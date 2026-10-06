import { describe, expect, it } from "vitest";

import { sanitizeHtml, sanitizeUrl } from "./sanitizer";

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

  it("解析不了的地址也做字符串脱敏", () => {
    expect(sanitizeUrl("not a url passkey=0123456789abcdef0123")).not.toContain(
      "0123456789abcdef0123",
    );
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
