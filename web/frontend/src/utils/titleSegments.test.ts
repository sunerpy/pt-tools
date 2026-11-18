import { describe, expect, it } from "vitest";

import { hasEpisode, segmentText, splitTitle } from "./titleSegments";

/*
 * 钉子：试搜索「选标题里的一段」（追剧方案 3.2.5）。切出的块不含分隔符；起止两块之间取的是原标题里连着的一段，
 * 括号、连字符、点照原样，所以一定在这个种子的标题里。
 */
const SEPARATORS = /^[\s._()[\]{}【】（）「」《》<>,，|/]|[\s._()[\]{}【】（）「」《》<>,，|/]$/u;

describe("splitTitle / segmentText", () => {
  const title = "The.Last.of.Us.S02E03.2160p.MAX.WEB-DL.DDP5.1.Atmos.H.265-[CHDWEB]（中字）";

  it("按空格、点、下划线、括号切，连字符不切", () => {
    const tokens = splitTitle(title).map((t) => t.text);
    expect(tokens).toEqual([
      "The",
      "Last",
      "of",
      "Us",
      "S02E03",
      "2160p",
      "MAX",
      "WEB-DL",
      "DDP5",
      "1",
      "Atmos",
      "H",
      "265-",
      "CHDWEB",
      "中字",
    ]);
    expect(splitTitle("A_B C|D/E,F，G")).toHaveLength(7);
    expect(splitTitle("...")).toEqual([]);
  });

  it("任取起止，都是原标题里连着的一段，不以分隔符开头或结尾", () => {
    const tokens = splitTitle(title);
    for (let i = 0; i < tokens.length; i++) {
      for (let j = 0; j < tokens.length; j++) {
        const seg = segmentText(title, tokens, i, j);
        expect(title.includes(seg)).toBe(true);
        expect(seg).not.toMatch(SEPARATORS);
        expect(seg.length).toBeGreaterThan(0);
      }
    }
  });

  it("原样保留中间的点、连字符与括号；先点后面一块也行", () => {
    const tokens = splitTitle(title);
    expect(segmentText(title, tokens, 8, 9)).toBe("DDP5.1");
    expect(segmentText(title, tokens, 13, 10)).toBe("Atmos.H.265-[CHDWEB");
    expect(segmentText(title, tokens, 0, 99)).toBe("");
  });
});

describe("hasEpisode", () => {
  it.each([
    "S02E03",
    "s1e10",
    "S02 E03",
    "E03",
    "EP12",
    "Ep.3 E5",
    "E01-E06",
    "E03中字",
    "第 3 集",
    "第三话",
    "第2期",
  ])("%s 有集号", (s) => {
    expect(hasEpisode(s)).toBe(true);
  });

  it.each(["S02", "Atmos-CHDWEB", "WEB-DL", "E-AC3", "x265-E1A", "H.265", "第二季", "2160p"])(
    "%s 没有集号",
    (s) => {
      expect(hasEpisode(s)).toBe(false);
    },
  );
});
