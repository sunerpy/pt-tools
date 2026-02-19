import type { CapturedPage, SiteSchema } from "../../core/types";
import { t } from "../../core/i18n";
import { sanitizeHtml } from "./sanitizer";

interface SchemaUrls {
  searchPath: string;
  detailPattern: RegExp;
  userInfoPattern: RegExp;
  detailUrlBuilder: (baseUrl: string, id: string) => string;
  userInfoUrlBuilder: (baseUrl: string, id: string) => string;
  extractTorrentId: (html: string) => string | null;
  extractUserId: (html: string) => string | null;
}

const SCHEMA_URL_MAP: Record<string, SchemaUrls> = {
  NexusPHP: {
    searchPath: "/torrents.php",
    detailPattern: /details\.php\?id=(\d+)/,
    userInfoPattern: /userdetails\.php\?id=(\d+)/,
    detailUrlBuilder: (base, id) => `${base}/details.php?id=${id}`,
    userInfoUrlBuilder: (base, id) => `${base}/userdetails.php?id=${id}`,
    extractTorrentId(html: string): string | null {
      const rows = html.matchAll(/<tr[^>]*>[\s\S]*?<\/tr>/gi);
      let freeRowId: string | null = null;
      let firstRowId: string | null = null;

      for (const rowMatch of rows) {
        const row = rowMatch[0];
        const idMatch = row.match(/details\.php\?id=(\d+)/);
        if (!idMatch) continue;

        if (!firstRowId) {
          firstRowId = idMatch[1];
        }

        if (/pro_free|pro_free2up|class="free"/i.test(row)) {
          freeRowId = idMatch[1];
          break;
        }
      }

      return freeRowId ?? firstRowId;
    },
    extractUserId(html: string): string | null {
      const infoBlock = html.match(/<div\s+id=["']info_block["'][^>]*>[\s\S]*?<\/div>/i);
      if (infoBlock) {
        const uidMatch = infoBlock[0].match(/userdetails\.php\?id=(\d+)/);
        if (uidMatch) return uidMatch[1];
      }

      const navArea = html.match(
        /<div\s+id=["'](?:nav_block|user(?:bar|info|menu))["'][^>]*>[\s\S]*?<\/div>/i,
      );
      if (navArea) {
        const uidMatch = navArea[0].match(/userdetails\.php\?id=(\d+)/);
        if (uidMatch) return uidMatch[1];
      }

      const allUserLinks = html.matchAll(/userdetails\.php\?id=(\d+)/g);
      const ids = new Map<string, number>();
      for (const m of allUserLinks) {
        ids.set(m[1], (ids.get(m[1]) ?? 0) + 1);
      }
      if (ids.size > 0) {
        return [...ids.entries()].sort((a, b) => b[1] - a[1])[0][0];
      }

      return null;
    },
  },
  Unit3D: {
    searchPath: "/torrents",
    detailPattern: /\/torrents\/(\d+)/,
    userInfoPattern: /\/users\/([\w-]+)/,
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
    searchPath: "/torrents.php",
    detailPattern: /torrents\.php\?id=(\d+)/,
    userInfoPattern: /user\.php\?id=(\d+)/,
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
  HDDolby: {
    searchPath: "/torrents.php",
    detailPattern: /details\.php\?id=(\d+)/,
    userInfoPattern: /userdetails\.php\?id=(\d+)/,
    detailUrlBuilder: (base, id) => `${base}/details.php?id=${id}`,
    userInfoUrlBuilder: (base, id) => `${base}/userdetails.php?id=${id}`,
    extractTorrentId: (html) => SCHEMA_URL_MAP["NexusPHP"].extractTorrentId(html),
    extractUserId: (html) => SCHEMA_URL_MAP["NexusPHP"].extractUserId(html),
  },
};

SCHEMA_URL_MAP["Rousi"] = SCHEMA_URL_MAP["NexusPHP"];

function getSchemaUrls(schema: SiteSchema): SchemaUrls | null {
  return SCHEMA_URL_MAP[schema] ?? null;
}

export interface AutoCollectProgress {
  step: "search" | "detail" | "userinfo" | "done" | "error";
  completed: number;
  total: number;
  message: string;
}

export type ProgressCallback = (progress: AutoCollectProgress) => void;

async function fetchPageHtml(url: string): Promise<string> {
  const response = await fetch(url, { credentials: "include", redirect: "follow" });
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${url}`);
  }
  return response.text();
}

function makeCapturedPage(
  url: string,
  html: string,
  pageType: CapturedPage["pageType"],
  schema: SiteSchema,
): CapturedPage {
  return {
    pageType,
    url,
    html: sanitizeHtml(html),
    capturedAt: new Date().toISOString(),
    detectedSchema: schema,
  };
}

export async function autoCollect(
  siteOrigin: string,
  schema: SiteSchema,
  onProgress?: ProgressCallback,
): Promise<CapturedPage[]> {
  const urls = getSchemaUrls(schema);
  if (!urls) {
    throw new Error(t("error.unsupportedSchema", schema));
  }

  const pages: CapturedPage[] = [];
  const baseUrl = siteOrigin.replace(/\/+$/, "");

  onProgress?.({ step: "search", completed: 0, total: 3, message: t("collector.searching") });
  const searchUrl = `${baseUrl}${urls.searchPath}`;
  const searchHtml = await fetchPageHtml(searchUrl);
  pages.push(makeCapturedPage(searchUrl, searchHtml, "search", schema));

  const torrentId = urls.extractTorrentId(searchHtml);
  const userId = urls.extractUserId(searchHtml);

  onProgress?.({ step: "detail", completed: 1, total: 3, message: t("collector.detailing") });
  if (torrentId) {
    const detailUrl = urls.detailUrlBuilder(baseUrl, torrentId);
    const detailHtml = await fetchPageHtml(detailUrl);
    pages.push(makeCapturedPage(detailUrl, detailHtml, "detail", schema));
  } else {
    onProgress?.({ step: "detail", completed: 1, total: 3, message: t("collector.skipDetail") });
  }

  onProgress?.({ step: "userinfo", completed: 2, total: 3, message: t("collector.userinfo") });
  if (userId) {
    const userUrl = urls.userInfoUrlBuilder(baseUrl, userId);
    const userHtml = await fetchPageHtml(userUrl);
    pages.push(makeCapturedPage(userUrl, userHtml, "userinfo", schema));
  } else {
    onProgress?.({
      step: "userinfo",
      completed: 2,
      total: 3,
      message: t("collector.skipUserinfo"),
    });
  }

  onProgress?.({
    step: "done",
    completed: pages.length,
    total: 3,
    message: t("collector.done", pages.length),
  });
  return pages;
}
