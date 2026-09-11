import type { CapturedPage, PageType, SiteSchema } from "../../core/types";
import { t } from "../../core/i18n";
import { sanitizeHtml } from "./sanitizer";

/**
 * Delay between consecutive page fetches. Go-side site definitions run at
 * RateLimit 0.5 (one request every two seconds); this collector now fetches up
 * to seven pages per run, so it must not burst them.
 */
const THROTTLE_MS = 1500;

/** Window scanned after an `id="info_block"` / nav marker when looking for the signed-in uid. */
const UID_WINDOW = 8000;

const FREE_RE = /pro_free2up|pro_free|class=["']free["']/i;

/** Any promotion marker — used to find a torrent that carries no promotion at all. */
const ANY_PROMO_RE =
  /pro_free2up|pro_free|pro_2up|pro_50pctdown2up|pro_50pctdown|pro_30pctdown|class=["'](?:free|twoup|twoupfree|halfdown|thirtypercent|twouphalfdown)["']/i;

export type TorrentPick = "freePreferred" | "free" | "nopromo";

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
  step: PageType | "done" | "error";
  completed: number;
  total: number;
  message: string;
}

export type ProgressCallback = (progress: AutoCollectProgress) => void;

export interface AutoCollectOutcome {
  pages: CapturedPage[];
  /** Human-readable notes about samples this site could not provide. */
  warnings: string[];
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

async function fetchPageHtml(url: string): Promise<string> {
  const response = await fetch(url, { credentials: "include", redirect: "follow" });
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${url}`);
  }
  return response.text();
}

/** Fetch a page whose absence is tolerable; returns null instead of throwing. */
async function tryFetchPageHtml(url: string): Promise<string | null> {
  try {
    return await fetchPageHtml(url);
  } catch {
    return null;
  }
}

function makeCapturedPage(
  url: string,
  html: string,
  pageType: PageType,
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
): Promise<AutoCollectOutcome> {
  const urls = getSchemaUrls(schema);
  if (!urls) {
    throw new Error(t("error.unsupportedSchema", schema));
  }

  const pages: CapturedPage[] = [];
  const warnings: string[] = [];
  const baseUrl = siteOrigin.replace(/\/+$/, "");

  // index / search / detail / detail_nopromo / userinfo, plus the optional
  // free listing and H&R page when the schema exposes them.
  const total = 5 + (urls.freeSearchPath ? 1 : 0) + (urls.hrPath ? 1 : 0);
  let completed = 0;

  const report = (step: AutoCollectProgress["step"], message: string): void => {
    onProgress?.({ step, completed, total, message });
  };

  const push = (url: string, html: string, pageType: PageType): void => {
    pages.push(makeCapturedPage(url, html, pageType, schema));
    completed += 1;
  };

  // 1. Index page: the only trustworthy source of the signed-in user id, and a
  //    sample the Go-side UserInfo pipeline reads directly.
  report("index", t("collector.index"));
  const indexUrl = `${baseUrl}${urls.userIdSourcePath}`;
  const indexHtml = await tryFetchPageHtml(indexUrl);
  if (indexHtml) {
    push(indexUrl, indexHtml, "index");
  } else {
    warnings.push(t("collector.warnNoIndex"));
  }

  // 2. Torrent listing.
  await sleep(THROTTLE_MS);
  report("search", t("collector.searching"));
  const searchUrl = `${baseUrl}${urls.searchPath}`;
  const searchHtml = await fetchPageHtml(searchUrl);
  push(searchUrl, searchHtml, "search");

  // 3. Promotion-filtered listing, when the schema has one.
  let freeSearchHtml: string | null = null;
  if (urls.freeSearchPath) {
    await sleep(THROTTLE_MS);
    report("search_free", t("collector.searchFree"));
    const freeSearchUrl = `${baseUrl}${urls.freeSearchPath}`;
    freeSearchHtml = await tryFetchPageHtml(freeSearchUrl);
    if (freeSearchHtml) {
      push(freeSearchUrl, freeSearchHtml, "search_free");
    } else {
      warnings.push(t("collector.warnNoFreeSearch"));
    }
  }

  // 4. A free torrent's detail page. When the site currently has no free
  //    torrent we still capture one detail page, but say so explicitly rather
  //    than letting the sample look like a free one.
  await sleep(THROTTLE_MS);
  report("detail", t("collector.detailing"));
  const freeId =
    (freeSearchHtml ? findTorrentId(freeSearchHtml, "free") : null) ??
    findTorrentId(searchHtml, "free");
  const detailId = freeId ?? urls.extractTorrentId(searchHtml);
  if (detailId) {
    const detailUrl = urls.detailUrlBuilder(baseUrl, detailId);
    const detailHtml = await tryFetchPageHtml(detailUrl);
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
  await sleep(THROTTLE_MS);
  report("detail_nopromo", t("collector.detailNoPromo"));
  const noPromoId = findTorrentId(searchHtml, "nopromo");
  if (noPromoId && noPromoId !== detailId) {
    const noPromoUrl = urls.detailUrlBuilder(baseUrl, noPromoId);
    const noPromoHtml = await tryFetchPageHtml(noPromoUrl);
    if (noPromoHtml) {
      push(noPromoUrl, noPromoHtml, "detail_nopromo");
    } else {
      warnings.push(t("collector.warnNoNoPromo"));
    }
  } else {
    warnings.push(t("collector.warnNoNoPromo"));
  }

  // 6. The signed-in user's own profile page, resolved from the index page.
  await sleep(THROTTLE_MS);
  report("userinfo", t("collector.userinfo"));
  const userId = indexHtml ? urls.extractUserId(indexHtml) : null;
  if (userId) {
    const userUrl = urls.userInfoUrlBuilder(baseUrl, userId);
    const userHtml = await tryFetchPageHtml(userUrl);
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
    await sleep(THROTTLE_MS);
    report("hr", t("collector.hr"));
    const hrUrl = `${baseUrl}${urls.hrPath}`;
    const hrHtml = await tryFetchPageHtml(hrUrl);
    if (hrHtml) {
      push(hrUrl, hrHtml, "hr");
    } else {
      warnings.push(t("collector.warnNoHR"));
    }
  }

  report("done", t("collector.done", pages.length, total));
  return { pages, warnings };
}
