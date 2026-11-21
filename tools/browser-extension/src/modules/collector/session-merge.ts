import type { CapturedPage, PageType } from "../../core/types";

/** 一次采集里可以有多个的页面类型（同源脚本）：按批换，不按类型逐个换 */
const MULTI_INSTANCE = new Set<PageType>(["script"]);

/** 只有一键采集才会产生的页面类型：手动逐页采集不会采到它们 */
const AUTO_ONLY = new Set<PageType>([
  "logged_out",
  "seeding",
  "script",
  "rss",
  "rendered_search",
  "rendered_detail",
  "rendered_userinfo",
  "requests_search",
  "requests_detail",
  "requests_userinfo",
  "download_notice",
]);

/**
 * 把一次采集的页面并进会话：同类页面换成新的；脚本这类可以有多个的整批换掉。
 * replaceAutoOnly（重新一键采集时）：上次一键采集才有、这次没采到的页面去掉，
 * 免得导出里同时有上次的「下载提示页」和这次读到的 tracker 域名这类互相矛盾的样本。
 */
export function mergeCollectedPages(
  existing: CapturedPage[],
  incoming: CapturedPage[],
  options: { replaceAutoOnly?: boolean } = {},
): CapturedPage[] {
  const incomingTypes = new Set(incoming.map((page) => page.pageType));
  const pages = existing.filter((page) => {
    if (MULTI_INSTANCE.has(page.pageType) && incomingTypes.has(page.pageType)) return false;
    if (options.replaceAutoOnly && AUTO_ONLY.has(page.pageType)) return false;
    return true;
  });
  for (const page of incoming) {
    if (MULTI_INSTANCE.has(page.pageType)) {
      pages.push(page);
      continue;
    }
    const index = pages.findIndex((item) => item.pageType === page.pageType);
    if (index >= 0) {
      pages[index] = page;
    } else {
      pages.push(page);
    }
  }
  return pages;
}
