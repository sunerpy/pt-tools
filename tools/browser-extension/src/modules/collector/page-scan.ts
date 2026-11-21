import type { PageType, SigninEntry } from "../../core/types";
import { isReplaceableUsername, sanitizeHtml } from "./sanitizer";

/*
 * 一键采集从已经取回的页面里找线索：同源脚本、签到入口、做种列表接口、下载链接、CSRF 令牌、本人的用户名。
 * 都是纯函数，只读文本，不发请求。
 */

/** 脚本内容里提到签到、做种列表、优惠状态的关键词（按这个顺序列出命中的） */
const SCRIPT_KEYWORDS = [
  "签到",
  "簽到",
  "attendance",
  "signin",
  "sign_in",
  "qiandao",
  "checkin",
  "getusertorrentlist",
  "promotion",
  "sp_state",
];

/** 页面上签到入口的字样 */
const SIGNIN_HINT =
  /签到|簽到|attendance|signin|sign_in|sign-in|qiandao|checkin|check_in|check-in|打卡/i;

/**
 * 公共库与 NexusPHP 自带的界面小工具：不会有站点自己的签到、做种列表逻辑，不取。
 * 按路径里的一段匹配（/vendor/ 下的都算）。
 */
const COMMON_LIBRARY =
  /(?:^|[/._-])(?:jquery|bootstrap|echarts|vue|react|react-dom|angular|lodash|underscore|moment|dayjs|layui|layer|swiper|chart|highcharts|popper|select2|sweetalert2?|font-?awesome|polyfill|axios|clipboard|lazyload|ckeditor|tinymce|prism|highlight|fancybox|viewer|datatables|qtip|ueditor|kindeditor|zepto|backbone|d3|three|socket\.io|webuploader|fullcalendar|flatpickr|crypto-js|md5|sha1|base64|json2|html5shiv|respond|modernizr|require|plupload|nprogress|toastr|masonry|mootools|prototype|swfobject|overlib|wz_tooltip|domlib|domtt|domtt_drag|fadomatic|curtain_imageresizer|vendor|libs?|node_modules)(?=[._/-]|$)/i;

/** 文件名像签到、做种列表、优惠的脚本先取 */
const SCRIPT_NAME_PRIORITY =
  /sign|attendance|qiandao|checkin|torrentlist|seed|promotion|promo|userdetails/i;

/**
 * 会改账号状态的地址：签到、登录登出、删除、认领。一键采集从不请求它们；
 * 静态的 .js 文件除外（attendance.js 这类脚本只是代码，取它不会签到）。
 */
const FORBIDDEN_REQUEST =
  /attendance|sign[_-]?in|qiandao|check[_-]?in|签到|簽到|打卡|logout|logoff|takelogin|delete|claim/i;

/** 上游 NexusPHP 的通用做种列表接口：已经单独试过，脚本里写的是它时不算别的接口 */
export const UPSTREAM_SEEDING_ENDPOINT = "getusertorrentlistajax.php";

const ENTITY: Record<string, string> = {
  amp: "&",
  lt: "<",
  gt: ">",
  quot: '"',
  apos: "'",
  nbsp: " ",
};

export function decodeEntities(text: string): string {
  return text.replace(/&(#x[0-9a-f]+|#\d+|[a-z]+);/gi, (match, code: string) => {
    if (code[0] === "#") {
      const n =
        code[1] === "x" || code[1] === "X" ? parseInt(code.slice(2), 16) : Number(code.slice(1));
      return Number.isFinite(n) && n > 0 && n < 0x110000 ? String.fromCodePoint(n) : match;
    }
    return ENTITY[code.toLowerCase()] ?? match;
  });
}

function stripTags(html: string): string {
  return decodeEntities(html.replace(/<[^>]*>/g, " "))
    .replace(/\s+/g, " ")
    .trim();
}

function attr(tag: string, name: string): string | undefined {
  const m = new RegExp(`\\s${name}\\s*=\\s*(?:"([^"]*)"|'([^']*)'|([^\\s"'>]+))`, "i").exec(tag);
  const raw = m?.[1] ?? m?.[2] ?? m?.[3];
  return raw === undefined ? undefined : decodeEntities(raw).trim();
}

function truncate(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max)}…` : text;
}

/** 页面引用的同源外部 .js 脚本：跳过公共库，按路径去重；文件名像签到、做种、优惠的排在前面 */
export function findSameOriginScripts(html: string, pageUrl: string, origin: string): string[] {
  const seen = new Set<string>();
  const first: string[] = [];
  const rest: string[] = [];
  for (const match of html.matchAll(/<script\b[^>]*>/gi)) {
    const src = attr(match[0], "src");
    if (!src) continue;
    let url: URL;
    try {
      url = new URL(src, pageUrl);
    } catch {
      continue;
    }
    if (url.origin !== origin) continue;
    const path = url.pathname.toLowerCase();
    if (!/\.m?js$/.test(path) || COMMON_LIBRARY.test(path) || FORBIDDEN_REQUEST.test(url.search)) {
      continue;
    }
    if (seen.has(path)) continue;
    seen.add(path);
    url.hash = "";
    (SCRIPT_NAME_PRIORITY.test(path.split("/").pop() ?? "") ? first : rest).push(url.toString());
  }
  return [...first, ...rest];
}

/** 几个页面的候选脚本合到一起：按路径去重，文件名像签到、做种、优惠的排在前面，其余按页面顺序 */
export function mergeScriptCandidates(lists: string[][]): string[] {
  const seen = new Set<string>();
  const first: string[] = [];
  const rest: string[] = [];
  for (const url of lists.flat()) {
    const path = new URL(url).pathname.toLowerCase();
    if (seen.has(path)) continue;
    seen.add(path);
    (SCRIPT_NAME_PRIORITY.test(path.split("/").pop() ?? "") ? first : rest).push(url);
  }
  return [...first, ...rest];
}

/** 脚本内容命中的关键词，按 SCRIPT_KEYWORDS 的顺序 */
export function scriptKeywords(text: string): string[] {
  const lower = text.toLowerCase();
  return SCRIPT_KEYWORDS.filter((keyword) => lower.includes(keyword.toLowerCase()));
}

/** 页面里没有 src 的内联脚本 */
export function inlineScripts(html: string): string[] {
  return [...html.matchAll(/<script\b(?![^>]*\bsrc\s*=)[^>]*>([\s\S]*?)<\/script\s*>/gi)].map(
    (match) => match[1],
  );
}

/**
 * 页面上的签到入口：属性（href、onclick、id、class、title、value）里有签到字样的元素，
 * 或者文字是签到、并且带 href / onclick 的元素。只记摘要，地址和代码脱敏。
 */
export function extractSigninEntries(html: string, page: PageType): SigninEntry[] {
  const entries: SigninEntry[] = [];
  const seen = new Set<string>();
  // 只匹配开始标签，元素里的文字另外往后看：一次匹配吞掉整个容器（info_block 的 div）会漏掉里面的链接
  const element = /<(a|button|span|input|img|li|div|i)\b([^>]*)>/gi;
  for (const match of html.matchAll(element)) {
    const tag = match[1].toLowerCase();
    const attrs = ` ${match[2]}`;
    const from = (match.index ?? 0) + match[0].length;
    const close = html.slice(from, from + 300).search(new RegExp(`</${tag}\\s*>`, "i"));
    const inner = close >= 0 ? html.slice(from, from + close) : "";
    const href = attr(attrs, "href");
    const onclick = attr(attrs, "onclick");
    const id = attr(attrs, "id");
    const className = attr(attrs, "class");
    const label = [attr(attrs, "title"), attr(attrs, "value"), attr(attrs, "alt")]
      .filter(Boolean)
      .join(" ");
    const text = stripTags(inner) || label;
    const attrHit = [href, onclick, id, className, label].some((v) => v && SIGNIN_HINT.test(v));
    const textHit = SIGNIN_HINT.test(text) && Boolean(href || onclick);
    if (!attrHit && !textHit) continue;

    const entry: SigninEntry = { page, tag, text: truncate(text, 60) };
    if (href) entry.href = truncate(sanitizeHtml(href), 200);
    if (onclick) entry.onclick = truncate(sanitizeHtml(onclick), 160);
    if (id) entry.id = truncate(id, 60);
    if (className) entry.className = truncate(className, 80);
    const key = `${tag}|${entry.href ?? ""}|${entry.onclick ?? ""}|${entry.text}`;
    if (seen.has(key)) continue;
    seen.add(key);
    entries.push(entry);
  }
  return entries;
}

/** 做种列表接口的线索：路径、方法、参数名（参数值是脚本里直接写的字面量时一并记下） */
export interface SeedingHint {
  path: string;
  method: "GET" | "POST";
  params: Array<{ name: string; literal?: string }>;
  /** 脚本把 CSRF 令牌放在这个请求头里 */
  csrfHeader?: string;
}

/** 从 text[open]（一个 {）读到配对的 }，回中间的内容；字符串里的括号不算 */
function readBalanced(text: string, open: number): string | null {
  let depth = 0;
  let quote: string | null = null;
  for (let i = open; i < text.length && i < open + 2000; i++) {
    const ch = text[i];
    if (quote) {
      if (ch === "\\") i++;
      else if (ch === quote) quote = null;
      continue;
    }
    if (ch === '"' || ch === "'" || ch === "`") quote = ch;
    else if (ch === "{" || ch === "(" || ch === "[") depth++;
    else if (ch === "}" || ch === ")" || ch === "]") {
      depth--;
      if (depth === 0) return text.slice(open + 1, i);
    }
  }
  return null;
}

/** 按最外层的逗号切开对象字面量的内容 */
function splitTopLevel(inner: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let quote: string | null = null;
  let start = 0;
  for (let i = 0; i < inner.length; i++) {
    const ch = inner[i];
    if (quote) {
      if (ch === "\\") i++;
      else if (ch === quote) quote = null;
      continue;
    }
    if (ch === '"' || ch === "'" || ch === "`") quote = ch;
    else if (ch === "{" || ch === "(" || ch === "[") depth++;
    else if (ch === "}" || ch === ")" || ch === "]") depth--;
    else if (ch === "," && depth === 0) {
      parts.push(inner.slice(start, i));
      start = i + 1;
    }
  }
  parts.push(inner.slice(start));
  return parts.map((part) => part.trim()).filter(Boolean);
}

function objectParams(inner: string): Array<{ name: string; literal?: string }> {
  const params: Array<{ name: string; literal?: string }> = [];
  for (const entry of splitTopLevel(inner)) {
    const m = /^(?:(["'])([^"']+)\1|([A-Za-z_$][\w$[\]]*))\s*:\s*([\s\S]*)$/.exec(entry);
    if (!m) continue;
    const name = m[2] ?? m[3];
    const value = m[4].trim();
    const str = /^(["'])([^"'\\]*)\1$/.exec(value);
    const literal = str ? str[2] : /^-?\d+(?:\.\d+)?$/.test(value) ? value : undefined;
    params.push(literal === undefined ? { name } : { name, literal });
  }
  return params;
}

/** 字符串字面量里像查询参数的名字：?userid=、&type= */
function queryNames(fragment: string): string[] {
  return [...fragment.matchAll(/(?:^|[?&])([A-Za-z_][\w[\]]*)=/g)].map((m) => m[1]);
}

/**
 * 脚本或页面里写的做种列表接口（路径里有 torrentlist 的 .php，上游通用的 getusertorrentlistajax.php 除外）：
 * 照它前后的代码认请求方法（$.post、type: 'POST'、xhr.open("POST"…)）和参数名
 * （紧跟着的对象字面量、data: {…}、地址里和接着拼上的查询参数）。
 */
export function findSeedingHints(text: string): SeedingHint[] {
  const hints: SeedingHint[] = [];
  const seen = new Set<string>();
  const endpoint =
    /["'`]((?:https?:)?(?:\/\/[^/"'`\s]+)?\/?(?:[\w.-]+\/)*[\w.-]*torrentlist[\w.-]*\.php)(\?[^"'`]*)?["'`]/gi;
  for (const match of text.matchAll(endpoint)) {
    const path = match[1];
    const base = path.split("/").pop()?.toLowerCase() ?? "";
    if (base === UPSTREAM_SEEDING_ENDPOINT || seen.has(path)) continue;
    seen.add(path);

    const start = match.index ?? 0;
    const end = start + match[0].length;
    const before = text.slice(Math.max(0, start - 200), start);
    const after = text.slice(end, end + 400);
    const around = before + after;

    const isPost =
      /\.post\s*\(\s*$/i.test(before) ||
      /\.open\s*\(\s*["']post["']\s*,\s*$/i.test(before) ||
      /\b(?:type|method)\s*:\s*["']post["']/i.test(around);

    const params: Array<{ name: string; literal?: string }> = [];
    const add = (param: { name: string; literal?: string }): void => {
      if (!params.some((p) => p.name === param.name)) params.push(param);
    };

    const immediate = /^\s*,\s*\{/.exec(after);
    const dataObject = /\bdata\s*:\s*\{/.exec(after) ?? /\bdata\s*:\s*\{/.exec(before);
    if (immediate) {
      const inner = readBalanced(after, immediate[0].length - 1);
      if (inner !== null) objectParams(inner).forEach(add);
    } else if (dataObject) {
      const source = /\bdata\s*:\s*\{/.test(after) ? after : before;
      const inner = readBalanced(source, dataObject.index + dataObject[0].length - 1);
      if (inner !== null) objectParams(inner).forEach(add);
    }

    for (const name of queryNames(match[2] ?? "")) add({ name });
    // 地址后面接着拼上的查询参数："…php?userid=" + uid + "&type=seeding"
    const tail = after.split(/[;\n]/)[0];
    for (const literal of tail.matchAll(/(["'`])((?:\\.|(?!\1)[^\\])*)\1/g)) {
      if (/^[?&]/.test(literal[2])) queryNames(literal[2]).forEach((name) => add({ name }));
    }

    const hint: SeedingHint = { path, method: isPost ? "POST" : "GET", params };
    const headers = /headers\s*:\s*\{([^}]*)\}/i.exec(around);
    const headerName =
      (headers && /["']?([\w-]*(?:csrf|xsrf)[\w-]*)["']?\s*:/i.exec(headers[1])?.[1]) ||
      /setRequestHeader\s*\(\s*["']([^"']*(?:csrf|xsrf)[^"']*)["']/i.exec(around)?.[1];
    if (headerName) hint.csrfHeader = headerName;
    hints.push(hint);
  }
  return hints;
}

/** 脚本里出现的同源 .php 接口：去掉参数，统一成 / 开头的路径，排序去重 */
export function extractPhpEndpoints(text: string, origin: string): string[] {
  const host = new URL(origin).host.toLowerCase();
  const found = new Set<string>();
  const ref =
    /(?:https?:)?(?:\/\/([^/"'`\s?#]+))?((?:\.{0,2}\/)?(?:[\w.-]+\/)*[\w-][\w.-]*\.php)(?![\w.])/gi;
  for (const match of text.matchAll(ref)) {
    if (match[1] && match[1].toLowerCase() !== host) continue;
    let path: string;
    try {
      path = new URL(match[2], `${origin}/`).pathname;
    } catch {
      continue;
    }
    found.add(path);
    if (found.size >= 80) break;
  }
  return [...found].sort();
}

/** 详情页里的种子下载链接（download.php?…，同源），解成绝对地址 */
export function findDownloadLink(html: string, pageUrl: string): string | null {
  const origin = new URL(pageUrl).origin;
  for (const match of html.matchAll(/\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))/gi)) {
    const href = decodeEntities((match[1] ?? match[2] ?? match[3] ?? "").trim());
    if (!/download\.php\?/i.test(href)) continue;
    try {
      const url = new URL(href, pageUrl);
      if (url.origin === origin) return url.toString();
    } catch {
      // 不是地址
    }
  }
  return null;
}

/** 页面里的 CSRF 令牌：<meta name="x-csrf" content>、<meta name="csrf-token">，或隐藏字段 csrf / _token */
export function extractCsrfToken(html: string): string | null {
  for (const match of html.matchAll(/<meta\b[^>]*>/gi)) {
    const name = attr(match[0], "name") ?? attr(match[0], "http-equiv") ?? "";
    if (/^(?:x-csrf|x-csrf-token|csrf|csrf-token|csrf_token|_csrf|x-xsrf-token)$/i.test(name)) {
      const content = attr(match[0], "content");
      if (content) return content;
    }
  }
  for (const match of html.matchAll(/<input\b[^>]*>/gi)) {
    const name = attr(match[0], "name") ?? "";
    if (/^(?:csrf|_csrf|csrf_token|csrftoken|_token)$/i.test(name)) {
      const value = attr(match[0], "value");
      if (value) return value;
    }
  }
  return null;
}

/** 首页 info_block（或导航块）里指向本人资料页的链接文字：登录用户自己的用户名 */
export function extractOwnUsername(html: string, uid: string): string | null {
  if (!/^\d+$/.test(uid)) return null;
  const link = new RegExp(
    `<a\\b[^>]*userdetails\\.php\\?id=${uid}(?!\\d)[^>]*>([\\s\\S]{0,400}?)<\\/a>`,
    "i",
  );
  const marker = /id=["'](?:info_block|nav_block|user(?:bar|info|menu))["']/i.exec(html);
  const scopes = marker ? [html.slice(marker.index, marker.index + 8000), html] : [html];
  for (const scope of scopes) {
    const found = link.exec(scope);
    const name = found ? stripTags(found[1]) : "";
    if (isReplaceableUsername(name)) return name;
  }
  return null;
}

/** 回的是 RSS / Atom：XML 开头，或内容类型是 XML 且有 <rss>/<feed>；网页不算 */
export function looksLikeRss(text: string, contentType: string): boolean {
  const head = text.slice(0, 2000).trimStart();
  if (/^<!doctype\s+html|<html[\s>]/i.test(head)) return false;
  return (
    /^(?:<\?xml|<rss\b|<feed\b)/i.test(head) ||
    (/xml|rss/i.test(contentType) && /<rss\b|<feed\b|<channel\b/i.test(head))
  );
}

/** 登录页：NexusPHP 的 takelogin.php 表单、loginform，或有密码框 */
export function looksLikeLoginPage(html: string): boolean {
  return /takelogin\.php|<form\b[^>]*(?:id|name)=["']?loginform|type=["']?password["']?/i.test(
    html,
  );
}

/** 会改账号状态的请求（签到、登出、删除、认领），一键采集一律不发；静态 .js 文件除外 */
export function isForbiddenRequest(rawUrl: string): boolean {
  let url: URL;
  try {
    url = new URL(rawUrl);
  } catch {
    return true;
  }
  let path = url.pathname;
  try {
    path = decodeURIComponent(path);
  } catch {
    // 留原样
  }
  if (/\.m?js$/i.test(path)) return false;
  return FORBIDDEN_REQUEST.test(path + decodeEntities(url.search));
}
