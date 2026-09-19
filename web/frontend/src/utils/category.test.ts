/*
 * 分类归桶的钉子。
 *
 * 用的都是**驱动真实会回的值**，不是造出来刚好能命中的字符串：
 *   site/v2/mtorrent_driver.go 的 mteamCategoryMap
 *   site/v2/hddolby_driver.go 的 getCategoryName
 * 一次 review 指出上一版的归桶「漏收 `影剧/综艺/HD` 与 `Animation`、
 * 误收 `TV游戏` 进剧集」，而当时的假数据只给了刚好能命中的「电影/剧集/动漫」，
 * 所以零偏离是假证明。这条测试就是补这个洞：漏收与误收各覆盖到。
 *
 * 第三个洞同理：标签兜底原来是用 `["movie","1990s"]` 证明的，那是造出来刚好能命中的形状。
 * Gazelle 站点的真实形状在 site/v2/definitions/mooko_fixture_test.go：
 * tags 是 `["剧情","悬疑","传记"]` 这种题材词，一个都归不进桶。
 * 所以这里钉的是真实形状 → unknown；而 unknown 在筛选里**默认丢掉、可显式请回** ——
 * 一度改成「无条件留下」，评审指出那只是把「所有档位下消失」换成「所有档位下都出现」，
 * 分段器对这类站点照样不起作用。
 */
import { describe, expect, it } from "vitest";

import {
  bucketOf,
  categoryLabel,
  CATEGORY_OPTIONS,
  matchesCategory,
  verdictOfItem,
} from "./category";

describe("bucketOf：M-Team 的真实分类名", () => {
  it.each([
    ["电影/SD", "movie"],
    ["电影/HD", "movie"],
    ["电影/Blu-Ray", "movie"],
    ["电影/Remux", "movie"],
    // 漏收过：这一串里没有「剧集」二字
    ["影剧/综艺/SD", "tv"],
    ["影剧/综艺/HD", "tv"],
    ["影剧/综艺/BD", "tv"],
    ["动画", "anime"],
    ["H-Anime", "anime"],
    ["H-Comic", "anime"],
    ["音乐(无损)", "music"],
  ])("%s → %s", (input, want) => {
    expect(bucketOf(input)).toBe(want);
  });

  it.each([
    // 误收过：含 tv，但它是 TV 游戏
    ["TV游戏"],
    ["PC游戏"],
    ["H-Game"],
    // 画板没有这些档位，归不进去比硬塞进某一桶好
    ["纪录片"],
    ["运动"],
    ["电子书"],
    ["软件"],
    ["有声书"],
    ["教育影片"],
    ["其他"],
    ["演唱会"],
  ])("%s 不归任何桶", (input) => {
    expect(bucketOf(input)).toBe("");
  });
});

describe("bucketOf：HDDolby 的真实分类名", () => {
  it.each([
    ["Movies/SD", "movie"],
    ["Movies/UHD", "movie"],
    ["Movies/Remux", "movie"],
    ["TV/SD", "tv"],
    ["TV/HD", "tv"],
    ["TV/BluRay", "tv"],
    // 漏收过：里面没有「动漫」二字
    ["Animation", "anime"],
    ["Music", "music"],
  ])("%s → %s", (input, want) => {
    expect(bucketOf(input)).toBe(want);
  });

  it.each([["Documentary"], ["Sports"], ["Games"], ["Software"]])("%s 不归任何桶", (input) => {
    expect(bucketOf(input)).toBe("");
  });
});

describe("bucketOf：边界", () => {
  it("空值与空白归到空桶", () => {
    expect(bucketOf(undefined)).toBe("");
    expect(bucketOf(null)).toBe("");
    expect(bucketOf("   ")).toBe("");
  });

  it("末尾的 TV 算剧集，紧跟中文的 TV 不算", () => {
    expect(bucketOf("综合/TV")).toBe("tv");
    expect(bucketOf("TV游戏")).toBe("");
  });

  it("动漫排在剧集之前：TV动画 归动漫", () => {
    expect(bucketOf("TV动画")).toBe("anime");
  });

  it("大小写无关", () => {
    expect(bucketOf("movies/hd")).toBe("movie");
    expect(bucketOf("ANIME")).toBe("anime");
  });
});

describe("categoryLabel", () => {
  it("桶 ID 回中文标签 —— 保存的搜索条件要给人看，不能显示 movie", () => {
    expect(categoryLabel("movie")).toBe("电影");
    expect(categoryLabel("tv")).toBe("剧集");
    expect(categoryLabel("")).toBe("全部分类");
    expect(categoryLabel(undefined)).toBe("全部分类");
  });

  it("认不出的值原样返回，不吞掉", () => {
    expect(categoryLabel("综艺")).toBe("综艺");
  });
});

describe("CATEGORY_OPTIONS", () => {
  it("就是画板 bar-88 的五档，顺序照画板", () => {
    expect(CATEGORY_OPTIONS.map((o) => o.label)).toEqual(["全部", "电影", "剧集", "动漫", "音乐"]);
  });
});

describe("verdictOfItem：分类优先、标签兜底，且分「站点没给」与「给了但不在五桶」", () => {
  it("有分类就用分类，不看标签", () => {
    expect(verdictOfItem({ category: "电影/HD", tags: ["anime"] })).toBe("movie");
  });

  it("五桶各自照旧命中（M-Team / HDDolby 的真实分类名）", () => {
    expect(verdictOfItem({ category: "电影/HD" })).toBe("movie");
    expect(verdictOfItem({ category: "影剧/综艺/HD" })).toBe("tv");
    expect(verdictOfItem({ category: "动画" })).toBe("anime");
    expect(verdictOfItem({ category: "音乐(无损)" })).toBe("music");
    expect(verdictOfItem({ category: "Animation" })).toBe("anime");
  });

  it("站点给了分类但不在五桶 → other（M-Team 的纪录片、TV游戏）", () => {
    expect(verdictOfItem({ category: "纪录片" })).toBe("other");
    expect(verdictOfItem({ category: "TV游戏" })).toBe("other");
    expect(verdictOfItem({ category: "Documentary" })).toBe("other");
    expect(verdictOfItem({ category: "Sports" })).toBe("other");
  });

  it("MooKo 的真实形状：没有分类 + 题材标签 → unknown", () => {
    /* site/v2/definitions/mooko_fixture_test.go 的 mookoSearchFixture 就是这个 tags */
    expect(verdictOfItem({ tags: ["剧情", "悬疑", "传记"] })).toBe("unknown");
  });

  it("分类为空但标签能认出内容类型时照样归桶", () => {
    expect(verdictOfItem({ category: "", tags: ["anime", "2020s"] })).toBe("anime");
    expect(verdictOfItem({ tags: ["1990s", "comedy", "movie"] })).toBe("movie");
  });

  it("分类与标签都没有 → unknown，不是 other", () => {
    expect(verdictOfItem({})).toBe("unknown");
    expect(verdictOfItem({ category: null, tags: null })).toBe("unknown");
    expect(verdictOfItem({ category: "   ", tags: [] })).toBe("unknown");
  });

  it("音乐站的流派标签认不出内容类型 → unknown", () => {
    expect(verdictOfItem({ tags: ["electronic", "jazz", "1980s"] })).toBe("unknown");
  });
});

describe("matchesCategory：默认严格筛，unknown 要显式请回", () => {
  it("选「全部」时全留，连 other 也留", () => {
    expect(matchesCategory({ category: "纪录片" }, "")).toBe(true);
    expect(matchesCategory({ tags: ["剧情"] }, "")).toBe(true);
  });

  it("选具体档位时命中的留、别的桶丢", () => {
    expect(matchesCategory({ category: "电影/HD" }, "movie")).toBe(true);
    expect(matchesCategory({ category: "影剧/综艺/HD" }, "movie")).toBe(false);
  });

  it("站点给了分类但不在五桶的丢掉 —— 五档之外不显示是画板的原意", () => {
    expect(matchesCategory({ category: "纪录片" }, "movie")).toBe(false);
    expect(matchesCategory({ category: "TV游戏" }, "tv")).toBe(false);
  });

  /*
   * 默认丢掉没有分类的行：留着的话档位对 Gazelle 那类站点等于没作用 ——
   * 一条 MooKo 的结果会在电影、剧集、动漫、音乐四个档位下全部出现。
   */
  it("默认把「站点没给分类」的行也筛掉，四个档位都不出现", () => {
    const mooko = { category: "", tags: ["剧情", "悬疑", "传记"] };
    for (const bucket of ["movie", "tv", "anime", "music"] as const) {
      expect(matchesCategory(mooko, bucket)).toBe(false);
    }
  });

  it("includeUnknown 打开时请回来 —— 但 other 仍然不回来", () => {
    const mooko = { category: "", tags: ["剧情", "悬疑", "传记"] };
    expect(matchesCategory(mooko, "music", { includeUnknown: true })).toBe(true);
    /* 站点明确表过态的不属于「信息缺失」，开关管不到它 */
    expect(matchesCategory({ category: "纪录片" }, "movie", { includeUnknown: true })).toBe(false);
  });
});
