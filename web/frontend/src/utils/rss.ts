/**
 * RSS 链接的检查：示例里的占位地址（hdsky.xxx、springxxx.xxx、rss.m-team.xxx 这类，顶级域是 xxx）永远取不到，
 * 和后端 models.IsPlaceholderRSSHost 是同一个判断；域名和站点对不上时多半是贴错了链接（只提醒，不拦）。
 */

/** 链接里的主机名（小写，去掉末尾的点）；不是 http(s) 链接时为空串 */
export function rssHost(raw: string): string {
  try {
    const u = new URL(raw.trim());
    if (u.protocol !== "http:" && u.protocol !== "https:") return "";
    return u.hostname.toLowerCase().replace(/\.$/, "");
  } catch {
    return "";
  }
}

/** 是不是示例里的占位地址 */
export function isPlaceholderRssUrl(raw: string): boolean {
  const host = rssHost(raw);
  return host === "xxx" || host.endsWith(".xxx");
}

/** 主机名的「站点域」：去掉 www、rss、api 这类前缀后的最后两段（hdsky.me、m-team.cc）；两段以内的原样 */
function siteDomain(host: string): string {
  const parts = host.split(".").filter(Boolean);
  if (parts.length <= 2) return parts.join(".");
  // co.uk、com.cn 这类二级后缀：留三段
  const sld = parts[parts.length - 2] ?? "";
  const keep = ["co", "com", "net", "org", "gov", "edu", "ac"].includes(sld) ? 3 : 2;
  return parts.slice(-keep).join(".");
}

/**
 * RSS 链接的域名属不属于站点：siteUrls 是站点的地址（站点地址、网页地址等）。
 * 站点没有地址可比时返回 true（不提醒）。
 */
export function rssBelongsToSite(raw: string, siteUrls: (string | undefined)[]): boolean {
  const host = rssHost(raw);
  if (!host) return true;
  const domains = siteUrls
    .map((u) => (u ? rssHost(u.includes("://") ? u : `https://${u}`) : ""))
    .filter(Boolean)
    .map(siteDomain);
  if (domains.length === 0) return true;
  return domains.includes(siteDomain(host));
}
