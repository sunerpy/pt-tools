import { describe, expect, it } from "vitest";

import { isPlaceholderRssUrl, rssBelongsToSite, rssHost } from "./rss";

describe("rss 链接检查", () => {
  it("认出示例里的占位地址", () => {
    expect(isPlaceholderRssUrl("https://hdsky.xxx/torrentrss.php?xxx")).toBe(true);
    expect(isPlaceholderRssUrl("https://springxxx.xxx")).toBe(true);
    expect(isPlaceholderRssUrl(" https://rss.m-team.xxx/api/rss/xxx ")).toBe(true);
    expect(isPlaceholderRssUrl("https://hdsky.me/torrentrss.php?passkey=k")).toBe(false);
    expect(isPlaceholderRssUrl("https://example.com/rss")).toBe(false);
    expect(isPlaceholderRssUrl("不是链接")).toBe(false);
  });

  it("主机名", () => {
    expect(rssHost("https://RSS.M-Team.cc./api/rss")).toBe("rss.m-team.cc");
    expect(rssHost("ftp://hdsky.me")).toBe("");
    expect(rssHost("")).toBe("");
  });

  it("域名和站点对得上", () => {
    expect(rssBelongsToSite("https://rss.m-team.cc/api/rss/x", ["https://api.m-team.cc"])).toBe(
      true,
    );
    expect(rssBelongsToSite("https://hdsky.me/torrentrss.php", ["hdsky.me"])).toBe(true);
    expect(rssBelongsToSite("https://springsunday.net/torrentrss.php", ["https://hdsky.me"])).toBe(
      false,
    );
    expect(rssBelongsToSite("https://a.example.co.uk/rss", ["https://b.example.co.uk"])).toBe(true);
    // 站点没有地址可比：不提醒
    expect(rssBelongsToSite("https://x.y/rss", [])).toBe(true);
    expect(rssBelongsToSite("https://x.y/rss", [undefined, ""])).toBe(true);
  });
});
