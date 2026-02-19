import { PAGE_PATTERNS } from "../../core/constants";
import type { PageType, SiteSchema } from "../../core/types";
import { getMetaContent, hasSelector, textContentIncludes } from "../../utils";

export function detectSiteSchema(doc: Document): SiteSchema {
  if (
    textContentIncludes(doc, /NexusPHP/i) ||
    hasSelector(doc, "table.torrents, table#torrenttable")
  ) {
    return "NexusPHP";
  }

  if (
    textContentIncludes(doc, /UNIT3D/i) ||
    getMetaContent(doc, "generator").toLowerCase().includes("unit3d") ||
    hasSelector(doc, ".torrent-listings, .panelV2")
  ) {
    return "Unit3D";
  }

  if (
    textContentIncludes(doc, /Gazelle/i) ||
    hasSelector(doc, "#content .thin") ||
    getMetaContent(doc, "generator").toLowerCase().includes("gazelle")
  ) {
    return "Gazelle";
  }

  if (
    location.pathname.includes("/api/") ||
    hasSelector(doc, "[data-v-app]") ||
    textContentIncludes(doc, /m-?team|mTorrent/i)
  ) {
    return "mTorrent";
  }

  if (textContentIncludes(doc, /HDDolby/i)) {
    return "HDDolby";
  }

  if (textContentIncludes(doc, /Rousi/i)) {
    return "Rousi";
  }

  return "Unknown";
}

export function detectPageType(url: string, doc: Document): PageType {
  const normalized = url.toLowerCase();
  const matched = PAGE_PATTERNS.find((item) => item.pattern.test(normalized));
  if (matched) {
    return matched.pageType;
  }

  if (hasSelector(doc, "table.torrents, .torrent-list, .torrent-table")) {
    return "search";
  }

  if (hasSelector(doc, "#kdescr, .torrent-description, .details")) {
    return "detail";
  }

  return "unknown";
}
