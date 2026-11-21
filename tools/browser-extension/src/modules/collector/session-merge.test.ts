import { describe, expect, it } from "vitest";

import type { CapturedPage, PageType } from "../../core/types";
import { mergeCollectedPages } from "./session-merge";

function page(pageType: PageType, url: string, html = ""): CapturedPage {
  return { pageType, url, html, capturedAt: "2026-11-21T00:00:00Z", detectedSchema: "NexusPHP" };
}

describe("mergeCollectedPages", () => {
  it("同类页面换成新的，脚本这类可以有多个的整批换掉", () => {
    const existing = [
      page("search", "s-old"),
      page("userinfo", "u-old"),
      page("script", "a.js"),
      page("script", "b.js"),
    ];
    const incoming = [page("search", "s-new"), page("script", "c.js"), page("script", "d.js")];
    expect(mergeCollectedPages(existing, incoming).map((p) => `${p.pageType}:${p.url}`)).toEqual([
      "search:s-new",
      "userinfo:u-old",
      "script:c.js",
      "script:d.js",
    ]);
  });

  it("重新一键采集时，上次一键采集才有的页面这次没采到就去掉（不留互相矛盾的样本）", () => {
    const existing = [
      page("search", "s-old"),
      page("download_notice", "dl"),
      page("rendered_search", "r"),
      page("script", "a.js"),
    ];
    const incoming = [page("search", "s-new"), page("seeding", "seed")];
    expect(
      mergeCollectedPages(existing, incoming, { replaceAutoOnly: true }).map((p) => p.pageType),
    ).toEqual(["search", "seeding"]);
  });

  it("手动采集的页面不受影响", () => {
    const existing = [page("hr", "hr-manual")];
    expect(mergeCollectedPages(existing, [page("search", "s")], { replaceAutoOnly: true })).toEqual(
      [page("hr", "hr-manual"), page("search", "s")],
    );
  });
});
