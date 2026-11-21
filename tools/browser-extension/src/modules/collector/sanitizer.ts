import { SENSITIVE_PATTERNS } from "../../core/constants";
import type { CollectedFacts, PageFormat, UserIdentity } from "../../core/types";

/**
 * 参数名、表单字段名、JSON 字段名等于这些时，值换成 REMOVED，名字本身留着（适配时要看结构）。
 * 地址里的凭证（RSS 的 passkey、HDDolby 的 downhash、Gazelle 的 torrent_pass、签名下载链接的 hash 等）都在这里。
 */
const SENSITIVE_NAME =
  /^(passkey|authkey|apikey|api_key|rsskey|key|token|access_token|refresh_token|auth|auth_token|secret|sign|signature|downhash|hash|torrent_pass|tp|sid|sessionid|phpsessid|csrf|csrf_token|csrftoken|_csrf|xsrf|xsrf_token|_token|nonce|password|passwd|pwd)$/i;

/** 名字里出现这些词也算：csrf_hash、session_token 这类 */
const SENSITIVE_NAME_PART = /csrf|xsrf|passkey|password|passwd|token|secret|session/i;

/** 隐藏字段名里出现这些词就清空值：CSRF、一次性令牌、会话、签名等 */
const SECRET_FIELD_NAME = /csrf|xsrf|token|nonce|secret|pass|auth|session|sign|key/i;

/** meta 的 name / http-equiv / property 里出现这些词就清空 content */
const SECRET_META_NAME = /csrf|xsrf|token/i;

/**
 * 正文里地址的查询参数：? 或 &（含 &amp;、&#38;、JSON 转义的 &）后面的 名字=值。
 * 值到 &、#、空白、引号、尖括号或反斜杠为止，所以属性的引号、标签的 > 和 JSON 的转义都留着。
 */
const BODY_PARAM = /(\?|&amp;|&#0*38;|&|\\u0026)([A-Za-z_][\w.\-[\]%]{0,40})=([^&#\s"'<>\\]*)/g;

/** 编码进另一个地址里的参数（%3F、%26 分隔，%3D 是等号），比如跳转地址里套着的下载地址 */
const ENCODED_PARAM =
  /(%3F|%26)([A-Za-z_][\w.-]{0,40})%3D((?:[^&%\s"'<>\\]|%(?!26)(?!3[Ff])[0-9A-Fa-f]{2})*)/gi;

/**
 * 脚本里直接写的凭证：var passkey = "…"、{csrf: '…'}、data-token="…"，
 * 键名带引号的也算：{"csrf": "…"}、{'X-CSRF-TOKEN': '…'}、截断后解析不了的 JSON 里的 "downhash":"…"
 */
const SCRIPT_SECRET =
  /((?:pass|auth|api|rss)key|api_key|torrent_pass|downhash|csrf(?:[_-]?token)?|xsrf(?:[_-]?token)?|_token|token|secret)(["']?\s*[:=]\s*)(["'])([^"'\n]{1,512})\3/gi;

export function isSensitiveName(name: string): boolean {
  const bare = name.replace(/\[.*$/, "");
  return SENSITIVE_NAME.test(bare) || SENSITIVE_NAME_PART.test(bare);
}

function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value.replace(/\+/g, " "));
  } catch {
    return value;
  }
}

/** 把标签里某个属性的值换成 "REMOVED"；值可以带双引号、单引号或不带引号 */
function redactAttr(tag: string, attr: string): string {
  const re = new RegExp(`(\\s${attr}\\s*=\\s*)("[^"]*"|'[^']*'|[^\\s"'>]+)`, "i");
  return tag.replace(re, '$1"REMOVED"');
}

function attrValue(tag: string, attr: string): string {
  const m = new RegExp(`\\s${attr}\\s*=\\s*(?:"([^"]*)"|'([^']*)'|([^\\s"'>]+))`, "i").exec(tag);
  return (m?.[1] ?? m?.[2] ?? m?.[3] ?? "").trim();
}

export function sanitizeHtml(html: string): string {
  let sanitized = html;

  for (const { pattern, replacement } of SENSITIVE_PATTERNS) {
    sanitized = sanitized.replace(pattern, replacement);
  }

  // Two value terminators, one per form. `key=value`: any quote, `&`, whitespace, `<`
  // or `>` ends it, so the tag's closing `'>` / `">` / `>` survives (swallowing it left
  // the attribute open and the parser absorbed the following rows). `"key":"value"`:
  // only the closing double quote ends it, so `'` and `&` inside JSON are redacted too.
  sanitized = sanitized.replace(
    /(passkey|authkey|apikey)(?:(=)[^"'&\s<>]+|("\s*:\s*")[^"<>]*("))/gi,
    "$1$2$3REMOVED$4",
  );
  sanitized = sanitized.replace(/("token"\s*:\s*")([^"]+)(")/gi, "$1REMOVED$3");

  // 正文里出现的地址：按参数名清空凭证的值（RSS 的 link / enclosure、脚本里的字符串、JSON 里的地址）
  sanitized = sanitized.replace(BODY_PARAM, (match, sep: string, name: string, value: string) =>
    value && value !== "REMOVED" && isSensitiveName(safeDecode(name))
      ? `${sep}${name}=REMOVED`
      : match,
  );
  sanitized = sanitized.replace(ENCODED_PARAM, (match, sep: string, name: string, value: string) =>
    value && value !== "REMOVED" && isSensitiveName(name) ? `${sep}${name}%3DREMOVED` : match,
  );
  sanitized = sanitized.replace(SCRIPT_SECRET, "$1$2$3REMOVED$3");

  // 服务端回填进表单的秘密：密码框的值，以及名字像 CSRF / 令牌 / 会话的隐藏字段。
  // 普通隐藏字段（如种子 id）保留，适配站点时要用。
  sanitized = sanitized.replace(/<input\b[^>]*>/gi, (tag) => {
    const type = attrValue(tag, "type").toLowerCase();
    const name = attrValue(tag, "name");
    if (type === "password" || (type === "hidden" && SECRET_FIELD_NAME.test(name))) {
      return redactAttr(tag, "value");
    }
    return tag;
  });
  sanitized = sanitized.replace(/<meta\b[^>]*>/gi, (tag) => {
    const name =
      attrValue(tag, "name") || attrValue(tag, "http-equiv") || attrValue(tag, "property");
    return SECRET_META_NAME.test(name) ? redactAttr(tag, "content") : tag;
  });

  return sanitized;
}

/**
 * 页面地址进 site-info.json 和 Issue 正文之前脱敏：详情、下载页的地址常带 passkey、token 等凭证。
 * 解析不了的地址退回字符串脱敏。
 */
export function sanitizeUrl(raw: string): string {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return sanitizeHtml(raw);
  }
  url.username = "";
  url.password = "";
  // 凭证参数清空，其余参数的值解码后按正文脱敏：有参数要改时整串会重新编码，
  // IPv6 的冒号、邮箱的 @ 编成 %3A、%40 以后正文规则就认不出了。同名参数逐个保留
  const params = [...url.searchParams];
  const cleaned = params.map(([name, value]): [string, string] => [
    name,
    isSensitiveName(name) ? "REMOVED" : sanitizeHtml(value),
  ]);
  if (cleaned.some(([, value], index) => value !== params[index][1])) {
    url.search = new URLSearchParams(cleaned).toString();
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    return sanitizeHtml(url.toString());
  }
  // 查询参数与 # 后面再按正文脱敏（套在参数里的地址、IP、邮箱）；协议、主机、路径原样留着：站点用 IP 访问时主机不能换成 127.0.0.1
  return `${url.origin}${url.pathname}${sanitizeHtml(url.search + url.hash)}`;
}

function sanitizeJsonValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => sanitizeJsonValue(item));
  }
  if (value !== null && typeof value === "object") {
    const out: Record<string, unknown> = {};
    for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
      out[key] =
        isSensitiveName(key) && (typeof item === "string" || typeof item === "number")
          ? "REMOVED"
          : sanitizeJsonValue(item);
    }
    return out;
  }
  if (typeof value === "string") {
    return sanitizeHtml(value);
  }
  return value;
}

/**
 * JSON 正文（接口回应、记录到的请求）：凭证字段清空，字符串里的 HTML 与地址照正文脱敏，结果仍是 JSON。
 * 解析不了时按正文脱敏。
 */
export function sanitizeJsonText(text: string): string {
  let data: unknown;
  try {
    data = JSON.parse(text);
  } catch {
    return sanitizeHtml(text);
  }
  return JSON.stringify(sanitizeJsonValue(data), null, 2);
}

/** 记录到的请求参数：表单参数按名字清空凭证，JSON 参数同 sanitizeJsonText，其余按正文脱敏 */
export function sanitizeParams(body: string): string {
  const trimmed = body.trim();
  if (!trimmed) {
    return body;
  }
  if (/^[{[]/.test(trimmed)) {
    try {
      JSON.parse(trimmed);
      return sanitizeJsonText(trimmed);
    } catch {
      // 不是 JSON，往下按表单或正文处理
    }
  }
  if (/^[^=&\s]+=[^&\s]*(?:&[^=&\s]+=[^&\s]*)*$/.test(trimmed)) {
    const pairs = trimmed.split("&").map((pair) => {
      const eq = pair.indexOf("=");
      const name = pair.slice(0, eq);
      return isSensitiveName(safeDecode(name)) && eq < pair.length - 1 ? `${name}=REMOVED` : pair;
    });
    return sanitizeHtml(pairs.join("&"));
  }
  return sanitizeHtml(body);
}

/**
 * site-info.json 里的站点信息再脱敏一次：签到入口的文字与代码、脚本与未登录首页的地址、提示文字。
 * Cookie 名、tracker 主机名、接口路径与参数名本身不含凭证，原样保留（tracker 是 IP 时不能换成 127.0.0.1）。
 */
export function sanitizeFacts(
  facts: CollectedFacts,
  identity: UserIdentity | null | undefined,
): CollectedFacts {
  const clean = (value: string): string => redactIdentity(sanitizeHtml(value), identity, "json");
  const cleanUrl = (value: string): string => redactIdentity(sanitizeUrl(value), identity, "json");
  const out: CollectedFacts = { ...facts };
  if (facts.signinEntries) {
    out.signinEntries = facts.signinEntries.map((entry) => ({
      ...entry,
      text: clean(entry.text),
      ...(entry.href !== undefined ? { href: clean(entry.href) } : {}),
      ...(entry.onclick !== undefined ? { onclick: clean(entry.onclick) } : {}),
    }));
  }
  if (facts.scripts) {
    out.scripts = facts.scripts.map((script) => ({ ...script, url: cleanUrl(script.url) }));
  }
  if (facts.loggedOut) {
    out.loggedOut = {
      ...facts.loggedOut,
      requestedUrl: cleanUrl(facts.loggedOut.requestedUrl),
      finalUrl: cleanUrl(facts.loggedOut.finalUrl),
    };
  }
  if (facts.warnings) {
    out.warnings = facts.warnings.map(clean);
  }
  return out;
}

/** 按格式脱敏：JSON 走 sanitizeJsonText，HTML、脚本、RSS 走 sanitizeHtml */
export function sanitizeContent(text: string, format: PageFormat = "html"): string {
  return format === "json" ? sanitizeJsonText(text) : sanitizeHtml(text);
}

/** 导出页面里代替登录用户的用户名与用户 ID 的示例值 */
export const EXAMPLE_USERNAME = "ExampleUser";
export const EXAMPLE_USER_ID = "123456";

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/** 用户名够长（不少于 3 个字符）、不是纯数字才换：太短或纯数字的会误伤正文 */
export function isReplaceableUsername(name: string | null | undefined): name is string {
  const trimmed = (name ?? "").trim();
  return trimmed.length >= 3 && !/^\d+$/.test(trimmed);
}

/**
 * 用户名等于这些词（不分大小写）时不自动替换。英文用户名按整词换、连属性值和脚本一起换：
 * 叫 free 会把 class="free"、alt="Free" 换掉，叫 seed、torrent 会把表头、脚本里的变量和接口字段换掉，
 * 导出的样本就不能拿来适配了。收的是 PT 站点页面里常见的词（优惠、做种、分类、导航）、常见的账号名、
 * 脚本与接口回应里常见的字段名，以及 requests_*.json 自己的字段名；不到 3 个字符的本来就不换，不用收。
 * 没收进来的常用词靠出现次数认（MAX_USERNAME_HITS_PER_PAGE）。
 */
const COMMON_USERNAMES: ReadonlySet<string> = new Set([
  // 优惠与置顶
  "free",
  "freeleech",
  "twoup",
  "twoupfree",
  "halfdown",
  "twouphalfdown",
  "thirtypercent",
  "promotion",
  "sticky",
  "hot",
  "new",
  "top",
  // 做种与种子
  "seed",
  "seeds",
  "seeder",
  "seeders",
  "seeding",
  "leech",
  "leecher",
  "leechers",
  "leeching",
  "peer",
  "peers",
  "snatch",
  "snatched",
  "torrent",
  "torrents",
  "download",
  "downloads",
  "downloaded",
  "upload",
  "uploads",
  "uploaded",
  "uploader",
  "ratio",
  "bonus",
  "invite",
  "complete",
  "completed",
  // 分类
  "movie",
  "movies",
  "film",
  "films",
  "series",
  "anime",
  "music",
  "documentary",
  "sports",
  "game",
  "games",
  "software",
  "ebook",
  "other",
  // 账号与角色
  "user",
  "users",
  "member",
  "members",
  "admin",
  "administrator",
  "sysop",
  "staff",
  "moderator",
  "vip",
  "guest",
  "test",
  "tester",
  "testing",
  "demo",
  "anonymous",
  "system",
  "root",
  "bot",
  // 导航与页面
  "index",
  "home",
  "login",
  "logout",
  "signup",
  "forum",
  "forums",
  "rules",
  "faq",
  "help",
  "news",
  "search",
  "browse",
  "details",
  "detail",
  "profile",
  "messages",
  "rss",
  // 脚本与接口回应里常见的字段名，requests_*.json 的字段名
  "data",
  "type",
  "name",
  "value",
  "text",
  "code",
  "status",
  "message",
  "result",
  "success",
  "error",
  "list",
  "info",
  "size",
  "time",
  "date",
  "link",
  "url",
  "method",
  "kind",
  "params",
  "response",
  "page",
  "click",
  "clicked",
  "requests",
  "omitted",
  // 中文用户名连着别的字也换；两个字的本来就不换，这里只收几个三个字以上、站点页面里常见的词
  "管理员",
  "电视剧",
  "纪录片",
]);

/** 用户名是常见词（不分大小写）：见 COMMON_USERNAMES */
export function isCommonUsername(name: string | null | undefined): boolean {
  return COMMON_USERNAMES.has((name ?? "").trim().toLowerCase());
}

/**
 * 一个页面里用户名出现超过这么多次，就不像用户名，按常见词处理、不替换。
 * 真实用户名在一页里最多一百来次：NexusPHP 的种子列表一页最多 100 行（usercp.php 里 min(100, …)），
 * 列表里全是自己发布的种子时发布者一栏每行一次，再加页头几处。HTML、CSS、脚本里的常用词
 * （span、center、rowfollow 这类）在列表页一页就有几百上千次。取 200，比一页的行数上限多一倍。
 */
export const MAX_USERNAME_HITS_PER_PAGE = 200;

/** 英文用户名只认整个词，不分大小写：hdwing_testers、pre_hdwing_tester 不算 */
function wordPattern(name: string): RegExp {
  return new RegExp(`(?<![A-Za-z0-9_])${escapeRegExp(name)}(?![A-Za-z0-9_])`, "gi");
}

function isAsciiName(name: string): boolean {
  return /^[\x20-\x7e]+$/.test(name);
}

function occurrences(text: string, needle: string): number {
  return needle ? text.split(needle).length - 1 : 0;
}

/** 用户名在这段文字里出现几次，按替换时的认法数：英文按整词、不分大小写，中文等按原样，地址里编码过的也算 */
export function countUsername(text: string, name: string): number {
  let count = isAsciiName(name)
    ? (text.match(wordPattern(name)) ?? []).length
    : occurrences(text, name);
  const encoded = encodeURIComponent(name);
  if (encoded !== name) {
    count += occurrences(text, encoded);
    if (encoded.toLowerCase() !== encoded) {
      count += occurrences(text, encoded.toLowerCase());
    }
  }
  return count;
}

/** 一次导出（或一次采集）怎么换登录用户的身份 */
export interface IdentityPlan {
  /** 交给 redactIdentity 的身份：用户名不宜替换时去掉了用户名，用户 ID 照换 */
  identity: UserIdentity | null;
  /** 用户名是常见词、或在某一页里出现次数多得不像用户名，没有自动替换：要提示用户提交前自己检查 */
  usernameKept: boolean;
}

/**
 * 按全部页面决定换不换用户名：用户名是常见词（COMMON_USERNAMES），或在任何一页里作为整词出现超过
 * MAX_USERNAME_HITS_PER_PAGE 次，就不换用户名（整次采集一个决定，不会这页换那页不换），并给出提示。
 * 太短、纯数字的用户名本来就不换，不提示（和以前一样）。
 */
export function planIdentity(
  identity: UserIdentity | null | undefined,
  texts: readonly string[],
): IdentityPlan {
  const name = identity?.username?.trim();
  if (!identity || !isReplaceableUsername(name)) {
    return { identity: identity ?? null, usernameKept: false };
  }
  const common =
    isCommonUsername(name) ||
    texts.some((text) => countUsername(text, name) > MAX_USERNAME_HITS_PER_PAGE);
  return common
    ? { identity: { ...identity, username: null }, usernameKept: true }
    : { identity, usernameKept: false };
}

function replaceUsername(text: string, name: string): string {
  let out = text;
  if (isAsciiName(name)) {
    // 英文用户名只换整个词，不分大小写：hdwing_testers、pre_hdwing_tester 不动
    out = out.replace(wordPattern(name), EXAMPLE_USERNAME);
  } else {
    // 中文等用户名没有词边界，连着别的字也换
    out = out.split(name).join(EXAMPLE_USERNAME);
  }
  const encoded = encodeURIComponent(name);
  if (encoded !== name) {
    out = out.split(encoded).join(EXAMPLE_USERNAME);
    out = out.split(encoded.toLowerCase()).join(EXAMPLE_USERNAME);
  }
  return out;
}

/** 标签里只换属性值（带不带引号都算），标签名、属性名不碰 */
function replaceUsernameInTag(tag: string, name: string): string {
  return tag.replace(
    /(=\s*)("[^"]*"|'[^']*'|[^\s"'>]+)/g,
    (_attr, eq: string, value: string) => `${eq}${replaceUsername(value, name)}`,
  );
}

/**
 * HTML 里只换文字和属性值，标签名、属性名不碰（用户名正好叫 title、span 时不把标签换坏）。
 * 注释、CDATA 和 script / style 的内容里没有标签，整段按文字换：脚本里的 a<b 不能被当成标签开头。
 */
function replaceUsernameInMarkup(html: string, name: string): string {
  return html.replace(
    /(<!--[\s\S]*?(?:-->|$)|<!\[CDATA\[[\s\S]*?(?:\]\]>|$))|(<(script|style)\b[^>]*>)([\s\S]*?)(<\/\3\s*>|$)|(<\/?[A-Za-z!?][^>]*>)|([^<]+|<)/gi,
    (
      _match,
      raw?: string,
      open?: string,
      _rawTag?: string,
      body?: string,
      close?: string,
      tag?: string,
      text?: string,
    ) => {
      if (raw) {
        return replaceUsername(raw, name);
      }
      if (open) {
        return `${replaceUsernameInTag(open, name)}${replaceUsername(body ?? "", name)}${close ?? ""}`;
      }
      if (tag) {
        return replaceUsernameInTag(tag, name);
      }
      return replaceUsername(text ?? "", name);
    },
  );
}

function replaceUserId(text: string, uid: string): string {
  const id = escapeRegExp(uid);
  const end = "(?!\\d)";
  const sample = `$1${EXAMPLE_USER_ID}`;
  return text
    .replace(
      new RegExp(`(userdetails\\.php\\?(?:[^"'\\s<>]*?&(?:amp;)?)?id=)${id}${end}`, "gi"),
      sample,
    )
    .replace(new RegExp(`(\\b(?:userid|uid|user_id)\\s*[=:]\\s*["']?)${id}${end}`, "gi"), sample)
    .replace(new RegExp(`(\\b(?:userid|uid|user_id)%3D)${id}${end}`, "gi"), sample)
    .replace(new RegExp(`(["'](?:userid|uid|user_id)["']\\s*:\\s*["']?)${id}${end}`, "gi"), sample)
    .replace(new RegExp(`(getusertorrentlist\\w*\\(\\s*["']?)${id}${end}`, "gi"), sample);
}

/**
 * 把登录用户自己的用户名换成 EXAMPLE_USERNAME、用户 ID 换成 EXAMPLE_USER_ID（用户名是常见词时不换，见 planIdentity）。
 * ID 只在 userdetails.php?id=、userid= / uid= / user_id=（脚本里的 uid = 、属性 data-uid= 也算）、
 * JSON 同名字段与 getusertorrentlistajax('…') 这类上下文里换：
 * 正文里别处一样的数字（大小、别人的 ID）不动。
 */
export function redactIdentity(
  text: string,
  identity: UserIdentity | null | undefined,
  format: PageFormat = "html",
): string {
  if (!identity || !text) {
    return text;
  }
  let out = text;
  const uid = identity.userId?.trim();
  if (uid && /^\d+$/.test(uid)) {
    out = replaceUserId(out, uid);
  }
  const name = identity.username?.trim();
  // 常见词不换（换了会把 class="free" 这类一起换掉）；调用方用 planIdentity 决定并提示用户
  if (isReplaceableUsername(name) && !isCommonUsername(name)) {
    out =
      format === "html" || format === "xml"
        ? replaceUsernameInMarkup(out, name)
        : replaceUsername(out, name);
  }
  return out;
}
