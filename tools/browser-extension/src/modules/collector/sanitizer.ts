import { SENSITIVE_PATTERNS } from "../../core/constants";

/** 查询参数里会带凭证的参数名：值换成 REMOVED，参数本身留着（适配时要看地址结构） */
const SENSITIVE_QUERY_PARAM =
  /^(passkey|authkey|apikey|api_key|rsskey|key|token|access_token|auth|secret|sign|signature|downhash|torrent_pass|sid|sessionid|phpsessid)$/i;

/** 隐藏字段名里出现这些词就清空值：CSRF、一次性令牌、会话、签名等 */
const SECRET_FIELD_NAME = /csrf|xsrf|token|nonce|secret|pass|auth|session|sign|key/i;

/** meta 的 name / http-equiv / property 里出现这些词就清空 content */
const SECRET_META_NAME = /csrf|xsrf|token/i;

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
  for (const name of new Set(url.searchParams.keys())) {
    if (SENSITIVE_QUERY_PARAM.test(name)) {
      url.searchParams.set(name, "REMOVED");
    }
  }
  return url.toString();
}
