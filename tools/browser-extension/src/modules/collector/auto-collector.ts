import type {
  CapturedPage,
  CollectedFacts,
  CollectedScript,
  PageFormat,
  PageType,
  SeedingRequestInfo,
  SigninEntry,
  SiteSchema,
  UserIdentity,
} from "../../core/types";
import { t } from "../../core/i18n";
import { REQUESTS_LOG_MAX_CHARS } from "../../recorder/constants";
import { parseTorrentAnnounces, trackerHostsOf } from "./bencode";
import {
  UPSTREAM_SEEDING_ENDPOINT,
  extractCsrfToken,
  extractOwnUsername,
  extractPhpEndpoints,
  extractSigninEntries,
  findDownloadLink,
  findSameOriginScripts,
  findSeedingHints,
  inlineScripts,
  isForbiddenRequest,
  looksLikeLoginPage,
  looksLikeRss,
  mergeScriptCandidates,
  scriptKeywords,
  type SeedingHint,
} from "./page-scan";
import {
  type IdentityPlan,
  planIdentity,
  redactIdentity,
  sanitizeContent,
  sanitizeFacts,
  sanitizeParams,
  sanitizeUrl,
} from "./sanitizer";
import type { RecordTabs, RecordedRequest, TabJob, TabKey, TabRecording } from "./tab-recorder";

/**
 * Delay between consecutive page fetches. Go-side site definitions run at
 * RateLimit 0.5 (one request every two seconds); this collector fetches a few
 * dozen pages per run on NexusPHP sites, so it must not burst them.
 */
export const THROTTLE_MS = 1500;

/** 新增步骤读回应时的上限：普通页面、脚本、种子文件 */
const MAX_PAGE_BYTES = 2 * 1024 * 1024;
const MAX_SCRIPT_BYTES = 300 * 1024;
const MAX_TORRENT_BYTES = 4 * 1024 * 1024;
/** 最多保存几个脚本、最多试取几个候选脚本 */
const MAX_SCRIPTS = 6;
const MAX_SCRIPT_FETCHES = 10;
const MAX_SIGNIN_ENTRIES = 12;

/** Window scanned after an `id="info_block"` / nav marker when looking for the signed-in uid. */
const UID_WINDOW = 8000;

const FREE_RE = /pro_free2up|pro_free|class=["']free["']/i;

/** Any promotion marker — used to find a torrent that carries no promotion at all. */
const ANY_PROMO_RE =
  /pro_free2up|pro_free|pro_2up|pro_50pctdown2up|pro_50pctdown|pro_30pctdown|class=["'](?:free|twoup|twoupfree|halfdown|thirtypercent|twouphalfdown)["']/i;

export type TorrentPick = "freePreferred" | "free" | "nopromo";

/**
 * NexusPHP 一族（含共用 nexusPHPUrls 的 HDDolby、Rousi）才补采的样本。别的架构没有这一项，采集行为不变。
 */
interface NexusExtras {
  bonusPath: string;
  rulesPath: string;
  faqPath: string;
  rssPath: string;
  /** 上游 NexusPHP 通用的做种列表接口 */
  seedingListPath: (uid: string) => string;
}

interface SchemaUrls {
  /** Page the signed-in user id is read from. Never a torrent listing. */
  userIdSourcePath: string;
  searchPath: string;
  /** Optional promotion-filtered listing (NexusPHP `spstate=2` = free). */
  freeSearchPath?: string;
  /** Optional H&R page. */
  hrPath?: string;
  detailUrlBuilder: (baseUrl: string, id: string) => string;
  userInfoUrlBuilder: (baseUrl: string, id: string) => string;
  extractTorrentId: (html: string) => string | null;
  extractUserId: (html: string) => string | null;
  extras?: NexusExtras;
}

function idOf(row: string): string | null {
  const match = row.match(/(?:^|[^A-Za-z])details\.php\?id=(\d+)/);
  return match ? match[1] : null;
}

/**
 * Split a listing page into per-torrent chunks. Table layouts yield `<tr>` rows;
 * div layouts (cspt, hhanclub) fall back to the span between consecutive
 * `details.php` links.
 */
function rowCandidates(html: string): string[] {
  const rows = [...html.matchAll(/<tr[^>]*>[\s\S]*?<\/tr>/gi)]
    .map((match) => match[0])
    .filter((row) => /details\.php\?id=\d+/.test(row));
  if (rows.length > 0) {
    return rows;
  }

  const links = [...html.matchAll(/(?:^|[^A-Za-z])details\.php\?id=(\d+)/gi)];
  return links.map((link, index) => html.slice(link.index, links[index + 1]?.index ?? html.length));
}

/**
 * Pick a torrent id from a listing page.
 *
 * `freePreferred` keeps the historical behaviour (first free torrent, else the
 * first torrent). `free` and `nopromo` return null instead of silently falling
 * back, so the caller can report that the site offered no such sample.
 */
export function findTorrentId(html: string, want: TorrentPick): string | null {
  let first: string | null = null;

  for (const row of rowCandidates(html)) {
    const id = idOf(row);
    if (!id) continue;
    first ??= id;

    if (want === "nopromo") {
      if (!ANY_PROMO_RE.test(row)) return id;
      continue;
    }

    if (FREE_RE.test(row)) return id;
  }

  return want === "freePreferred" ? first : null;
}

export function extractNexusPHPTorrentId(html: string): string | null {
  return findTorrentId(html, "freePreferred");
}

function uidNear(html: string, marker: RegExp, window: number): string | null {
  const found = marker.exec(html);
  if (!found) return null;
  const slice = html.slice(found.index, found.index + window);
  const uid = slice.match(/userdetails\.php\?id=(\d+)/);
  return uid ? uid[1] : null;
}

/**
 * Read the signed-in user's id from a NexusPHP-family page.
 *
 * `info_block` is matched by id alone because sites render it as either a `<div>`
 * or a `<table>` (piggo uses the latter), and nested markup makes closing-tag
 * matching unreliable.
 *
 * There is deliberately **no** whole-page frequency fallback: on a torrent
 * listing the most frequent `userdetails.php?id=` link is the busiest uploader,
 * which made the collector capture a stranger's profile and ship it inside a
 * public issue attachment. Returning null lets the caller ask for a manual
 * capture instead.
 */
export function extractOwnUserId(html: string): string | null {
  return (
    uidNear(html, /id=["']info_block["']/i, UID_WINDOW) ??
    uidNear(html, /id=["'](?:nav_block|user(?:bar|info|menu))["']/i, UID_WINDOW / 2)
  );
}

/** True when a fetched profile page actually exposes the detail rows we need. */
export function userInfoHasDetailRows(html: string): boolean {
  return /class=["']rowhead/i.test(html);
}

const nexusPHPUrls: SchemaUrls = {
  userIdSourcePath: "/index.php",
  searchPath: "/torrents.php",
  freeSearchPath: "/torrents.php?spstate=2",
  hrPath: "/myhr.php",
  detailUrlBuilder: (base, id) => `${base}/details.php?id=${id}`,
  userInfoUrlBuilder: (base, id) => `${base}/userdetails.php?id=${id}`,
  extractTorrentId: (html) => findTorrentId(html, "freePreferred"),
  extractUserId: extractOwnUserId,
  extras: {
    bonusPath: "/mybonus.php",
    rulesPath: "/rules.php",
    faqPath: "/faq.php",
    rssPath: "/torrentrss.php?rows=10",
    seedingListPath: (uid) => `/${UPSTREAM_SEEDING_ENDPOINT}?userid=${uid}&type=seeding`,
  },
};

const SCHEMA_URL_MAP: Record<string, SchemaUrls> = {
  NexusPHP: nexusPHPUrls,
  HDDolby: nexusPHPUrls,
  Rousi: nexusPHPUrls,
  Unit3D: {
    userIdSourcePath: "/",
    searchPath: "/torrents",
    detailUrlBuilder: (base, id) => `${base}/torrents/${id}`,
    userInfoUrlBuilder: (base, slug) => `${base}/users/${slug}`,
    extractTorrentId(html: string): string | null {
      const match = html.match(/\/torrents\/(\d+)/);
      return match ? match[1] : null;
    },
    extractUserId(html: string): string | null {
      const match = html.match(/\/users\/([\w-]+)/);
      return match ? match[1] : null;
    },
  },
  Gazelle: {
    userIdSourcePath: "/index.php",
    searchPath: "/torrents.php",
    detailUrlBuilder: (base, id) => `${base}/torrents.php?id=${id}`,
    userInfoUrlBuilder: (base, id) => `${base}/user.php?id=${id}`,
    extractTorrentId(html: string): string | null {
      const match = html.match(/torrents\.php\?id=(\d+)/);
      return match ? match[1] : null;
    },
    extractUserId(html: string): string | null {
      const match = html.match(/user\.php\?id=(\d+)/);
      return match ? match[1] : null;
    },
  },
};

function getSchemaUrls(schema: SiteSchema): SchemaUrls | null {
  return SCHEMA_URL_MAP[schema] ?? null;
}

export interface AutoCollectProgress {
  step: PageType | "tracker" | "tabs" | "cookies" | "done" | "error";
  completed: number;
  total: number;
  message: string;
}

export type ProgressCallback = (progress: AutoCollectProgress) => void;

export interface AutoCollectOutcome {
  pages: CapturedPage[];
  /** Human-readable notes about samples this site could not provide. */
  warnings: string[];
  /** 记进 site-info.json 的站点信息（已脱敏） */
  facts: CollectedFacts;
  /** 登录用户自己的用户名与 ID：只留在本地，导出时换成示例值 */
  identity: UserIdentity | null;
}

/** 采集要用的外部能力：测试换成假的。后台标签页记录与 Cookie 名不传时跳过这两步 */
export interface CollectorDeps {
  fetch: (input: string, init?: RequestInit) => Promise<Response>;
  sleep: (ms: number) => Promise<void>;
  /** 在后台标签页里打开页面、记录页面脚本发的请求 */
  recordTabs?: RecordTabs;
  /** 站点 Cookie 的名字（不碰值）；没有 Cookie 权限时回 null */
  cookieNames?: (origin: string) => Promise<string[] | null>;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

const DEFAULT_DEPS: CollectorDeps = {
  fetch: (input, init) => fetch(input, init),
  sleep,
};

/** 一键采集发出的每个请求都经过这里：签到、登出这类会改账号状态的地址一律不发 */
async function guardedFetch(
  deps: CollectorDeps,
  url: string,
  init?: RequestInit,
): Promise<Response> {
  if (isForbiddenRequest(url)) {
    throw new Error(`refused: ${sanitizeUrl(url)}`);
  }
  return deps.fetch(url, init);
}

async function fetchPageHtml(deps: CollectorDeps, url: string): Promise<string> {
  const response = await guardedFetch(deps, url, { credentials: "include", redirect: "follow" });
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${sanitizeUrl(url)}`);
  }
  return response.text();
}

/** Fetch a page whose absence is tolerable; returns null instead of throwing. */
async function tryFetchPageHtml(deps: CollectorDeps, url: string): Promise<string | null> {
  try {
    return await fetchPageHtml(deps, url);
  } catch {
    return null;
  }
}

/** 读回应，最多读 maxBytes：超过的部分不读（不下载完整的大文件） */
async function readCapped(
  response: Response,
  maxBytes: number,
): Promise<{ bytes: Uint8Array; truncated: boolean }> {
  if (!response.body) {
    const all = new Uint8Array(await response.arrayBuffer());
    return { bytes: all.subarray(0, maxBytes), truncated: all.length > maxBytes };
  }
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  let truncated = false;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    if (total + value.length > maxBytes) {
      chunks.push(value.subarray(0, maxBytes - total));
      total = maxBytes;
      truncated = true;
      await reader.cancel().catch(() => undefined);
      break;
    }
    chunks.push(value);
    total += value.length;
  }
  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.length;
  }
  return { bytes, truncated };
}

const utf8 = new TextDecoder("utf-8");

interface Fetched {
  finalUrl: string;
  status: number;
  ok: boolean;
  redirected: boolean;
  contentType: string;
  text: string;
  bytes: number;
  truncated: boolean;
}

/** 新增步骤用的取法：不因状态码抛错（调用方自己判断），网络错误或被拒绝时回 null */
async function tryFetchText(
  deps: CollectorDeps,
  url: string,
  init: RequestInit = { credentials: "include", redirect: "follow" },
  maxBytes = MAX_PAGE_BYTES,
): Promise<Fetched | null> {
  try {
    const response = await guardedFetch(deps, url, init);
    const { bytes, truncated } = await readCapped(response, maxBytes);
    return {
      finalUrl: response.url || url,
      status: response.status,
      ok: response.ok,
      redirected: response.redirected,
      contentType: response.headers.get("content-type") ?? "",
      text: utf8.decode(bytes),
      bytes: bytes.length,
      truncated,
    };
  } catch {
    return null;
  }
}

function isJsonText(text: string, contentType: string): boolean {
  if (/json/i.test(contentType)) return true;
  const trimmed = text.trim();
  if (!/^[{[]/.test(trimmed)) return false;
  try {
    JSON.parse(trimmed);
    return true;
  } catch {
    return false;
  }
}

/** 做种列表接口回的像列表：有内容，不是登录页，也不是一整个网页（404 页、首页） */
function isSeedingListResponse(fetched: { ok: boolean; text: string }): boolean {
  const text = fetched.text.trim();
  return (
    fetched.ok &&
    text.length > 0 &&
    !looksLikeLoginPage(text) &&
    !/^(?:<!doctype|<html[\s>])/i.test(text)
  );
}

/** 表单或 JSON 请求参数里的参数名（不要值） */
function paramNames(body: string | undefined): string[] {
  const text = (body ?? "").trim();
  if (!text) return [];
  if (/^[{[]/.test(text)) {
    try {
      const data: unknown = JSON.parse(text);
      return data !== null && typeof data === "object" && !Array.isArray(data)
        ? Object.keys(data)
        : [];
    } catch {
      return [];
    }
  }
  if (!text.includes("=")) return [];
  return [
    ...new Set(
      text
        .replace(/^\?/, "")
        .split("&")
        .map((pair) => pair.split("=")[0])
        .filter(Boolean)
        .map((name) => {
          try {
            return decodeURIComponent(name);
          } catch {
            return name;
          }
        }),
    ),
  ];
}

/** path 按站点地址解析以后还在这个站点上 */
function isSameOrigin(path: string, baseUrl: string): boolean {
  try {
    return new URL(path, `${baseUrl}/`).origin === new URL(baseUrl).origin;
  } catch {
    return false;
  }
}

const USER_ID_PARAM = /^(?:userid|uid|user_id|id)$/i;
const CSRF_PARAM = /csrf|xsrf|^_?token$/i;

/** 照脚本里写的接口拼请求：用户 ID、type=seeding、页面里的 CSRF 令牌，脚本里直接写的字面量照用 */
function buildHintRequest(
  hint: SeedingHint,
  baseUrl: string,
  uid: string,
  csrf: string | null,
): { url: string; init: RequestInit; info: SeedingRequestInfo } {
  const target = new URL(hint.path, `${baseUrl}/`);
  const values = new URLSearchParams();
  const names: string[] = [];
  const params = hint.params.length > 0 ? hint.params : [{ name: "userid" }, { name: "type" }];
  for (const param of params) {
    let value: string | undefined = param.literal;
    if (USER_ID_PARAM.test(param.name)) value = uid;
    else if (/^type$/i.test(param.name)) value = "seeding";
    else if (CSRF_PARAM.test(param.name)) value = csrf ?? undefined;
    if (value === undefined) continue;
    values.append(param.name, value);
    names.push(param.name);
  }
  const headers: Record<string, string> = { "X-Requested-With": "XMLHttpRequest" };
  if (hint.csrfHeader && csrf) headers[hint.csrfHeader] = csrf;

  let init: RequestInit;
  if (hint.method === "POST") {
    headers["Content-Type"] = "application/x-www-form-urlencoded; charset=UTF-8";
    init = { method: "POST", credentials: "include", headers, body: values.toString() };
  } else {
    values.forEach((value, name) => target.searchParams.set(name, value));
    init = { method: "GET", credentials: "include", headers };
  }
  return {
    url: target.toString(),
    init,
    info: { method: hint.method, path: target.pathname, params: names, source: "script" },
  };
}

/** 采到的页面先只脱敏；登录用户的用户名与 ID 等全部页面采完再一起换（见 redactPages） */
function makeCapturedPage(
  url: string,
  content: string,
  pageType: PageType,
  schema: SiteSchema,
  format: PageFormat = "html",
): CapturedPage {
  const page: CapturedPage = {
    pageType,
    url: sanitizeUrl(url),
    html: sanitizeContent(content, format),
    capturedAt: new Date().toISOString(),
    detectedSchema: schema,
  };
  if (format !== "html") {
    page.format = format;
  }
  return page;
}

/** 按 planIdentity 的决定换掉页面与地址里登录用户的用户名和 ID */
function redactPages(pages: CapturedPage[], plan: IdentityPlan): CapturedPage[] {
  if (!plan.identity) {
    return pages;
  }
  return pages.map((page) => ({
    ...page,
    url: redactIdentity(page.url, plan.identity, "json"),
    html: redactIdentity(page.html, plan.identity, page.format ?? "html"),
  }));
}

/** requests_*.json 里的一条请求 */
interface RequestLogEntry {
  kind: RecordedRequest["kind"];
  method: string;
  url: string;
  status?: number;
  contentType?: string;
  params?: string;
  response?: unknown;
  afterClick?: true;
  error?: string;
  /** 整份记录超出 REQUESTS_LOG_MAX_CHARS，这一条的参数和回应正文没有保存 */
  omitted?: true;
}

/**
 * 后台标签页记到的请求整理成 requests_*.json：地址、参数、回应正文脱敏，JSON 回应解开方便看。
 * 整份超出 REQUESTS_LOG_MAX_CHARS 时，从最后一条往前去掉参数和回应正文（方法、地址、状态等照留，记 omitted），
 * 打开页面时最先发的请求留得最全；点「当前做种」取到的做种列表另存在 seeding 页里，不受影响。
 */
function requestsLog(recording: TabRecording): string {
  const clickAt = recording.click?.clicked ? recording.click.at : undefined;
  const entries: RequestLogEntry[] = recording.requests.map((request: RecordedRequest) => {
    let response: unknown = request.responseBody;
    if (request.responseBody && isJsonText(request.responseBody, request.contentType ?? "")) {
      try {
        response = JSON.parse(sanitizeContent(request.responseBody, "json"));
      } catch {
        response = sanitizeContent(request.responseBody, "html");
      }
    } else if (request.responseBody) {
      response = sanitizeContent(request.responseBody, "html");
    }
    return {
      kind: request.kind,
      method: request.method,
      url: sanitizeUrl(request.url),
      status: request.status,
      contentType: request.contentType ?? undefined,
      params: request.requestBody ? sanitizeParams(request.requestBody) : undefined,
      response,
      ...(clickAt !== undefined && request.startedAt >= clickAt
        ? { afterClick: true as const }
        : {}),
      ...(request.error ? { error: request.error } : {}),
    };
  });
  const render = (): string =>
    JSON.stringify(
      {
        page: sanitizeUrl(recording.finalUrl),
        timedOut: recording.timedOut === true,
        ...(recording.click
          ? {
              click: {
                clicked: recording.click.clicked,
                text: recording.click.text,
                code: recording.click.code,
              },
            }
          : {}),
        requests: entries,
      },
      null,
      2,
    );
  let text = render();
  for (let index = entries.length - 1; index >= 0; index -= 1) {
    if (text.length <= REQUESTS_LOG_MAX_CHARS) break;
    const { params, response, ...rest } = entries[index];
    if (params === undefined && response === undefined) continue;
    entries[index] = { ...rest, omitted: true };
    text = render();
  }
  return text;
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export async function autoCollect(
  siteOrigin: string,
  schema: SiteSchema,
  onProgress?: ProgressCallback,
  deps: CollectorDeps = DEFAULT_DEPS,
): Promise<AutoCollectOutcome> {
  const urls = getSchemaUrls(schema);
  if (!urls) {
    throw new Error(t("error.unsupportedSchema", schema));
  }

  const pages: CapturedPage[] = [];
  const warnings: string[] = [];
  const baseUrl = siteOrigin.replace(/\/+$/, "");
  const extras = urls.extras;

  // index / search / detail / detail_nopromo / userinfo, plus the optional
  // free listing and H&R page when the schema exposes them; NexusPHP-family
  // sites add bonus, rules, faq, rss, logged-out index, scripts, seeding list,
  // tracker hosts, background tabs and cookie names.
  const total = 5 + (urls.freeSearchPath ? 1 : 0) + (urls.hrPath ? 1 : 0) + (extras ? 11 : 0);
  const stepsDone = new Set<string>();
  let identity: UserIdentity | null = null;

  const report = (step: AutoCollectProgress["step"], message: string): void => {
    onProgress?.({ step, completed: stepsDone.size, total, message });
  };

  const push = (
    url: string,
    content: string,
    pageType: PageType,
    format: PageFormat = "html",
  ): void => {
    pages.push(makeCapturedPage(url, content, pageType, schema, format));
    stepsDone.add(pageType.replace(/^(?:rendered|requests)_.*/, "tabs"));
  };

  const pause = (): Promise<void> => deps.sleep(THROTTLE_MS);

  // 1. Index page: the only trustworthy source of the signed-in user id, and a
  //    sample the Go-side UserInfo pipeline reads directly.
  report("index", t("collector.index"));
  const indexUrl = `${baseUrl}${urls.userIdSourcePath}`;
  const indexHtml = await tryFetchPageHtml(deps, indexUrl);
  if (extras && indexHtml) {
    // 登录用户自己的用户名与 ID：采完以后在页面里换成示例值（用户名是常见词时不换，见 planIdentity）
    const uid = urls.extractUserId(indexHtml);
    if (uid) {
      identity = { userId: uid, username: extractOwnUsername(indexHtml, uid) };
    }
  }
  if (indexHtml) {
    push(indexUrl, indexHtml, "index");
  } else {
    warnings.push(t("collector.warnNoIndex"));
  }

  // 2. Torrent listing.
  await pause();
  report("search", t("collector.searching"));
  const searchUrl = `${baseUrl}${urls.searchPath}`;
  const searchHtml = await fetchPageHtml(deps, searchUrl);
  push(searchUrl, searchHtml, "search");

  // 3. Promotion-filtered listing, when the schema has one.
  let freeSearchHtml: string | null = null;
  if (urls.freeSearchPath) {
    await pause();
    report("search_free", t("collector.searchFree"));
    const freeSearchUrl = `${baseUrl}${urls.freeSearchPath}`;
    freeSearchHtml = await tryFetchPageHtml(deps, freeSearchUrl);
    if (freeSearchHtml) {
      push(freeSearchUrl, freeSearchHtml, "search_free");
    } else {
      warnings.push(t("collector.warnNoFreeSearch"));
    }
  }

  // 4. A free torrent's detail page. When the site currently has no free
  //    torrent we still capture one detail page, but say so explicitly rather
  //    than letting the sample look like a free one.
  await pause();
  report("detail", t("collector.detailing"));
  const freeId =
    (freeSearchHtml ? findTorrentId(freeSearchHtml, "free") : null) ??
    findTorrentId(searchHtml, "free");
  const detailId = freeId ?? urls.extractTorrentId(searchHtml);
  const detailUrl = detailId ? urls.detailUrlBuilder(baseUrl, detailId) : null;
  let detailHtml: string | null = null;
  if (detailUrl) {
    detailHtml = await tryFetchPageHtml(deps, detailUrl);
    if (detailHtml) {
      push(detailUrl, detailHtml, "detail");
    } else {
      warnings.push(t("collector.warnNoDetail"));
    }
    if (!freeId) {
      warnings.push(t("collector.warnNoFree"));
    }
  } else {
    warnings.push(t("collector.skipDetail"));
  }

  // 5. A torrent with no promotion at all — proves the detail parser does not
  //    mistake an unrelated element for a discount end time.
  await pause();
  report("detail_nopromo", t("collector.detailNoPromo"));
  const noPromoId = findTorrentId(searchHtml, "nopromo");
  if (noPromoId && noPromoId !== detailId) {
    const noPromoUrl = urls.detailUrlBuilder(baseUrl, noPromoId);
    const noPromoHtml = await tryFetchPageHtml(deps, noPromoUrl);
    if (noPromoHtml) {
      push(noPromoUrl, noPromoHtml, "detail_nopromo");
    } else {
      warnings.push(t("collector.warnNoNoPromo"));
    }
  } else {
    warnings.push(t("collector.warnNoNoPromo"));
  }

  // 6. The signed-in user's own profile page, resolved from the index page.
  await pause();
  report("userinfo", t("collector.userinfo"));
  const userId = indexHtml ? urls.extractUserId(indexHtml) : null;
  const userUrl = userId ? urls.userInfoUrlBuilder(baseUrl, userId) : null;
  let userHtml: string | null = null;
  if (userUrl) {
    userHtml = await tryFetchPageHtml(deps, userUrl);
    if (userHtml) {
      push(userUrl, userHtml, "userinfo");
      if (!userInfoHasDetailRows(userHtml)) {
        warnings.push(t("collector.warnUserinfoPrivacy"));
      }
    } else {
      warnings.push(t("collector.warnNoUserinfo"));
    }
  } else {
    warnings.push(t("collector.skipUserinfo"));
  }

  // 7. H&R page, when the schema has one.
  if (urls.hrPath) {
    await pause();
    report("hr", t("collector.hr"));
    const hrUrl = `${baseUrl}${urls.hrPath}`;
    const hrHtml = await tryFetchPageHtml(deps, hrUrl);
    if (hrHtml) {
      push(hrUrl, hrHtml, "hr");
    } else {
      warnings.push(t("collector.warnNoHR"));
    }
  }

  if (!extras) {
    report("done", t("collector.done", stepsDone.size, total));
    return { pages, warnings, facts: {}, identity: null };
  }

  const facts: CollectedFacts = {};

  // 8–10. 魔力值、规则、常见问题：取不到（或回的是登录页）只记提示
  const optionalPages: Array<[PageType, string, Parameters<typeof t>[0], Parameters<typeof t>[0]]> =
    [
      ["bonus", extras.bonusPath, "collector.bonus", "collector.warnNoBonus"],
      ["rules", extras.rulesPath, "collector.rules", "collector.warnNoRules"],
      ["faq", extras.faqPath, "collector.faq", "collector.warnNoFaq"],
    ];
  for (const [pageType, path, stepKey, warnKey] of optionalPages) {
    await pause();
    report(pageType, t(stepKey));
    const fetched = await tryFetchText(deps, `${baseUrl}${path}`);
    if (fetched?.ok && !looksLikeLoginPage(fetched.text)) {
      push(fetched.finalUrl, fetched.text, pageType);
    } else {
      warnings.push(t(warnKey));
    }
  }

  // 11. RSS：带会话 Cookie 取；回的不是 RSS/XML（比如「require passkey」）时不保存
  await pause();
  report("rss", t("collector.rss"));
  const rss = await tryFetchText(deps, `${baseUrl}${extras.rssPath}`);
  if (rss?.ok && looksLikeRss(rss.text, rss.contentType)) {
    push(rss.finalUrl, rss.text, "rss", "xml");
  } else if (rss) {
    warnings.push(
      t("collector.warnRssNotXml", rss.ok ? rss.contentType || "?" : `HTTP ${rss.status}`),
    );
  } else {
    warnings.push(t("collector.warnNoRss"));
  }

  // 12. 未登录时的首页：同一地址不带 Cookie 再取一次（不收它回的 Cookie），记下最终地址与状态码
  await pause();
  report("logged_out", t("collector.loggedOut"));
  const loggedOut = await tryFetchText(deps, indexUrl, { credentials: "omit", redirect: "follow" });
  if (loggedOut) {
    push(loggedOut.finalUrl, loggedOut.text, "logged_out");
    facts.loggedOut = {
      requestedUrl: sanitizeUrl(indexUrl),
      finalUrl: sanitizeUrl(loggedOut.finalUrl),
      status: loggedOut.status,
      redirected: loggedOut.redirected,
    };
  } else {
    warnings.push(t("collector.warnNoLoggedOut"));
    facts.loggedOut = null;
  }

  // 13. 页面引用的同源脚本：内容提到签到、做种列表、优惠状态的才保存
  report("script", t("collector.scripts"));
  const scanned: Array<{ type: PageType; url: string; html: string }> = [];
  if (indexHtml) scanned.push({ type: "index", url: indexUrl, html: indexHtml });
  if (userHtml && userUrl) scanned.push({ type: "userinfo", url: userUrl, html: userHtml });
  scanned.push({ type: "search", url: searchUrl, html: searchHtml });
  if (detailHtml && detailUrl) scanned.push({ type: "detail", url: detailUrl, html: detailHtml });

  const candidates = mergeScriptCandidates(
    scanned.map((page) => findSameOriginScripts(page.html, page.url, baseUrl)),
  );
  const fetchedScripts: string[] = [];
  const scripts: CollectedScript[] = [];
  for (const scriptUrl of candidates.slice(0, MAX_SCRIPT_FETCHES)) {
    if (scripts.length >= MAX_SCRIPTS) break;
    await pause();
    const fetched = await tryFetchText(deps, scriptUrl, undefined, MAX_SCRIPT_BYTES);
    if (!fetched?.ok || /^\s*(?:<!doctype|<html[\s>])/i.test(fetched.text)) continue;
    fetchedScripts.push(fetched.text);
    const keywords = scriptKeywords(fetched.text);
    if (keywords.length === 0) continue;
    push(scriptUrl, fetched.text, "script", "js");
    scripts.push({
      url: sanitizeUrl(scriptUrl),
      bytes: fetched.bytes,
      truncated: fetched.truncated,
      keywords,
    });
  }
  facts.scripts = scripts;
  facts.scriptEndpoints = extractPhpEndpoints(
    [...fetchedScripts, ...scanned.flatMap((page) => inlineScripts(page.html))].join("\n"),
    baseUrl,
  );

  // 14. 自己的做种列表：先试上游通用的 GET；个人页或脚本里写的是别的接口时，照脚本里的方法与参数取
  report("seeding", t("collector.seeding"));
  const attempts: SeedingRequestInfo[] = [];
  let seedingRequest: SeedingRequestInfo | null = null;
  let seeding: { url: string; text: string; format: PageFormat } | null = null;
  if (userId) {
    await pause();
    const upstreamUrl = `${baseUrl}${extras.seedingListPath(userId)}`;
    const upstream = await tryFetchText(deps, upstreamUrl);
    const upstreamInfo: SeedingRequestInfo = {
      method: "GET",
      path: `/${UPSTREAM_SEEDING_ENDPOINT}`,
      params: ["userid", "type"],
      source: "upstream",
      ...(upstream ? { status: upstream.status, contentType: upstream.contentType } : {}),
    };
    attempts.push(upstreamInfo);
    if (upstream && isSeedingListResponse(upstream)) {
      seedingRequest = upstreamInfo;
      seeding = {
        url: upstreamUrl,
        text: upstream.text,
        format: isJsonText(upstream.text, upstream.contentType) ? "json" : "html",
      };
    }

    // 只照本站的接口取：脚本里写的是别的网站时，不把 Cookie 和 CSRF 令牌带过去
    const hint = findSeedingHints([userHtml ?? "", ...fetchedScripts].join("\n")).find((item) =>
      isSameOrigin(item.path, baseUrl),
    );
    if (hint) {
      const csrf =
        extractCsrfToken(userHtml ?? "") ??
        extractCsrfToken(indexHtml ?? "") ??
        extractCsrfToken(searchHtml);
      const request = buildHintRequest(hint, baseUrl, userId, csrf);
      await pause();
      const fetched = await tryFetchText(deps, request.url, request.init);
      const info: SeedingRequestInfo = {
        ...request.info,
        ...(fetched ? { status: fetched.status, contentType: fetched.contentType } : {}),
      };
      attempts.push(info);
      if (fetched && isSeedingListResponse(fetched)) {
        seedingRequest = info;
        seeding = {
          url: request.url,
          text: fetched.text,
          format: isJsonText(fetched.text, fetched.contentType) ? "json" : "html",
        };
      }
    }
  }

  // 15. tracker 域名：从详情页的下载链接下载一个种子文件，只读 announce 的主机名，文件不保存
  await pause();
  report("tracker", t("collector.tracker"));
  const downloadUrl = detailHtml && detailUrl ? findDownloadLink(detailHtml, detailUrl) : null;
  if (!downloadUrl) {
    warnings.push(t("collector.warnNoDownloadLink"));
  } else {
    try {
      const response = await guardedFetch(deps, downloadUrl, {
        credentials: "include",
        redirect: "follow",
      });
      const { bytes } = await readCapped(response, MAX_TORRENT_BYTES);
      const announces = response.ok ? parseTorrentAnnounces(bytes) : null;
      if (announces) {
        facts.trackerHosts = trackerHostsOf(announces);
        if (facts.trackerHosts.length === 0) {
          warnings.push(t("collector.warnTorrentNoTracker"));
        }
      } else {
        const text = utf8.decode(bytes);
        if (/<(?:!doctype|html|head|body|form|table|div|a)\b/i.test(text.slice(0, 4000))) {
          // 第一次下载的提示页、登录页：保存这一页（脱敏），不绕过
          push(response.url || downloadUrl, text, "download_notice");
          warnings.push(t("collector.warnTorrentNotTorrent", response.status));
        } else {
          warnings.push(t("collector.warnTorrentFailed", response.status));
        }
      }
    } catch (error: unknown) {
      warnings.push(t("collector.warnTorrentFailed", errorText(error)));
    }
  }

  // 16. 后台标签页：列表页、详情页、个人页（点「当前做种」），记下页面脚本另取的内容
  let recordings: TabRecording[] = [];
  if (deps.recordTabs) {
    await pause();
    report("tabs", t("collector.tabs"));
    const jobs: TabJob[] = [{ key: "search", url: searchUrl }];
    if (detailUrl) jobs.push({ key: "detail", url: detailUrl });
    if (userUrl) jobs.push({ key: "userinfo", url: userUrl, clickSeeding: true });
    try {
      recordings = await deps.recordTabs(baseUrl, jobs);
    } catch (error: unknown) {
      warnings.push(t("collector.warnTabsFailed", errorText(error)));
    }
    for (const recording of recordings) {
      if (recording.error) {
        warnings.push(t("collector.warnTabFailed", recording.key, recording.error));
      }
      if (!recording.html && recording.requests.length === 0) continue;
      const key: TabKey = recording.key;
      if (recording.html) {
        push(recording.finalUrl, recording.html, `rendered_${key}`);
      }
      push(recording.finalUrl, requestsLog(recording), `requests_${key}`, "json");
    }

    // 第 14 步取不到时的通用兜底：个人页上点「当前做种」触发的请求
    const userRecording = recordings.find((recording) => recording.key === "userinfo");
    const clickAt = userRecording?.click?.clicked ? userRecording.click.at : undefined;
    const clicked = userRecording?.requests.find(
      (request) =>
        (clickAt === undefined || request.startedAt >= clickAt) &&
        /torrentlist|seeding/i.test(`${request.url} ${request.requestBody ?? ""}`) &&
        request.status !== undefined &&
        request.status < 400 &&
        Boolean(request.responseBody),
    );
    if (!seeding && clicked?.responseBody) {
      const target = new URL(clicked.url);
      seedingRequest = {
        method: clicked.method,
        path: target.pathname,
        params: [...new Set([...paramNames(target.search), ...paramNames(clicked.requestBody)])],
        source: "click",
        status: clicked.status,
        ...(clicked.contentType ? { contentType: clicked.contentType } : {}),
      };
      seeding = {
        url: clicked.url,
        text: clicked.responseBody,
        format: isJsonText(clicked.responseBody, clicked.contentType ?? "") ? "json" : "html",
      };
    }
  }

  if (seeding) {
    push(seeding.url, seeding.text, "seeding", seeding.format);
  } else {
    warnings.push(t("collector.warnNoSeeding"));
  }
  facts.seedingRequest = seedingRequest;
  facts.seedingAttempts = attempts;

  // 签到入口：取回的页面与后台标签页里渲染后的页面
  const entries: SigninEntry[] = [];
  const seenEntries = new Set<string>();
  const entrySources: Array<{ type: PageType; html: string }> = [
    ...scanned,
    ...recordings
      .filter((recording) => recording.html)
      .map((recording) => ({
        type: `rendered_${recording.key}` as PageType,
        html: recording.html ?? "",
      })),
  ];
  for (const source of entrySources) {
    for (const entry of extractSigninEntries(source.html, source.type)) {
      const key = `${entry.tag}|${entry.href ?? ""}|${entry.onclick ?? ""}|${entry.text}`;
      if (seenEntries.has(key) || entries.length >= MAX_SIGNIN_ENTRIES) continue;
      seenEntries.add(key);
      entries.push(entry);
    }
  }
  facts.signinEntries = entries;

  // 17. 站点 Cookie 的名字（只要名字，不碰值）
  if (deps.cookieNames) {
    report("cookies", t("collector.cookies"));
    try {
      facts.cookieNames = await deps.cookieNames(baseUrl);
    } catch {
      facts.cookieNames = null;
    }
    if (facts.cookieNames === null) {
      warnings.push(t("collector.warnNoCookiePermission"));
    } else {
      stepsDone.add("cookies");
    }
  }

  // 用户名是常见词（free、admin 这类）或在页面里出现得太多时不换用户名，只换 ID，提示用户提交前自己检查
  const plan = planIdentity(
    identity,
    pages.map((page) => page.html),
  );
  if (plan.usernameKept) {
    warnings.push(t("collector.warnUsernameCommon"));
  }

  facts.warnings = [...warnings];
  report("done", t("collector.done", stepsDone.size, total));
  return {
    pages: redactPages(pages, plan),
    warnings,
    facts: sanitizeFacts(facts, plan.identity),
    identity,
  };
}
