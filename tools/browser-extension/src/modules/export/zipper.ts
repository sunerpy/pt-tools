import JSZip from "jszip";

import type { CapturedPage, CollectionSession, PageFormat, PageType } from "../../core/types";
import {
  redactIdentity,
  sanitizeContent,
  sanitizeFacts,
  sanitizeUrl,
} from "../collector/sanitizer";
import { exportWarnings, planExportIdentity } from "./identity";

/** 每种页面导出的文件名（不带扩展名）；扩展名按格式给 */
const PAGE_FILE_STEMS: Record<PageType, string> = {
  search: "search",
  search_free: "search-free",
  detail: "detail",
  detail_nopromo: "detail-nopromo",
  userinfo: "userinfo",
  hr: "hr",
  index: "index",
  bonus: "bonus",
  api_response: "api-response",
  rules: "rules",
  faq: "faq",
  rss: "rss",
  logged_out: "logged-out",
  seeding: "seeding",
  script: "script",
  rendered_search: "rendered-search",
  rendered_detail: "rendered-detail",
  rendered_userinfo: "rendered-userinfo",
  requests_search: "requests-search",
  requests_detail: "requests-detail",
  requests_userinfo: "requests-userinfo",
  download_notice: "download-notice",
  unknown: "unknown",
};

/** 页面没写格式时（旧版本存下的）按类型推 */
const DEFAULT_FORMAT: Partial<Record<PageType, PageFormat>> = {
  script: "js",
  rss: "xml",
  requests_search: "json",
  requests_detail: "json",
  requests_userinfo: "json",
};

export function pageFormat(page: CapturedPage): PageFormat {
  return page.format ?? DEFAULT_FORMAT[page.pageType] ?? "html";
}

/** 导出的文件名：同名的第二个起加 -2、-3，加在扩展名前面（script-2.js、unknown-2.html） */
export function exportFileNames(pages: CapturedPage[]): string[] {
  const counters = new Map<string, number>();
  return pages.map((page) => {
    const stem = PAGE_FILE_STEMS[page.pageType] ?? "page";
    const ext = pageFormat(page);
    const base = `${stem}.${ext}`;
    const count = counters.get(base) ?? 0;
    counters.set(base, count + 1);
    return count === 0 ? base : `${stem}-${count + 1}.${ext}`;
  });
}

/**
 * Which semantic samples this session actually contains. Site adapters read this
 * from site-info.json instead of grepping the HTML to discover what is missing.
 */
function describeCapabilities(session: CollectionSession): Record<string, boolean> {
  const present = new Set<PageType>(session.pages.map((page) => page.pageType));
  return {
    index: present.has("index"),
    search: present.has("search"),
    freeSearch: present.has("search_free"),
    detail: present.has("detail"),
    noPromoDetail: present.has("detail_nopromo"),
    userInfo: present.has("userinfo"),
    hr: present.has("hr"),
    bonus: present.has("bonus"),
    rules: present.has("rules"),
    faq: present.has("faq"),
    rss: present.has("rss"),
    loggedOut: present.has("logged_out"),
    seeding: present.has("seeding"),
    scripts: present.has("script"),
    renderedSearch: present.has("rendered_search"),
    renderedDetail: present.has("rendered_detail"),
    renderedUserInfo: present.has("rendered_userinfo"),
    requestsSearch: present.has("requests_search"),
    requestsDetail: present.has("requests_detail"),
    requestsUserInfo: present.has("requests_userinfo"),
    downloadNotice: present.has("download_notice"),
  };
}

export async function createExportZip(session: CollectionSession): Promise<Blob> {
  const zip = new JSZip();
  // 用户名是常见词或在页面里出现得太多时不换用户名（只换 ID），warnings 里提示用户自己检查
  const plan = planExportIdentity(session);
  const identity = plan.identity;
  const warnings = exportWarnings(session, plan);
  const names = exportFileNames(session.pages);
  const exportUrl = (url: string): string => redactIdentity(sanitizeUrl(url), identity, "json");

  session.pages.forEach((page, index) => {
    const format = pageFormat(page);
    // 导出前再脱敏一次：旧版本采集、存在本地的页面没经过新的脱敏规则；登录用户的用户名与 ID 换成示例值
    zip.file(names[index], redactIdentity(sanitizeContent(page.html, format), identity, format));
  });

  const facts = session.facts ? sanitizeFacts({ ...session.facts, warnings }, identity) : null;
  const metadata = {
    id: session.id,
    site: session.site,
    status: session.status,
    createdAt: session.createdAt,
    pages: session.pages.map((page, index) => ({
      file: names[index],
      pageType: page.pageType,
      format: pageFormat(page),
      url: exportUrl(page.url),
      capturedAt: page.capturedAt,
      detectedSchema: page.detectedSchema,
    })),
    capabilities: describeCapabilities(session),
    // 一键采集记下的站点信息（NexusPHP 一族才有）：Cookie 只有名字，tracker 只有主机名，请求只有方法、路径和参数名
    ...(facts
      ? {
          cookieNames: facts.cookieNames ?? null,
          trackerHosts: facts.trackerHosts ?? [],
          seedingRequest: facts.seedingRequest ?? null,
          seedingAttempts: facts.seedingAttempts ?? [],
          signinEntries: facts.signinEntries ?? [],
          scriptEndpoints: facts.scriptEndpoints ?? [],
          scripts: facts.scripts ?? [],
          loggedOut: facts.loggedOut ?? null,
          warnings: facts.warnings ?? [],
        }
      : warnings.length > 0
        ? { warnings }
        : {}),
  };

  zip.file("site-info.json", JSON.stringify(metadata, null, 2));
  return zip.generateAsync({ type: "blob" });
}

export function downloadZip(blob: Blob, filename: string): void {
  const objectUrl = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectUrl;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(objectUrl);
}
