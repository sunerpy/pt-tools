/*
 * 分类归桶的钉子。
 *
 * 用的都是**驱动真实会回的值**，不是造出来刚好能命中的字符串：
 *   site/v2/mtorrent_driver.go 的 mteamCategoryMap
 *   site/v2/hddolby_driver.go 的 getCategoryName
 * 一次 review 指出上一版的归桶「漏收 `影剧/综艺/HD` 与 `Animation`、
 * 误收 `TV游戏` 进剧集」，而当时的假数据只给了刚好能命中的「电影/剧集/动漫」，
 * 所以零偏离是假证明。这条测试就是补这个洞：漏收与误收各覆盖到。
 */
import { describe, expect, it } from "vitest";

import { bucketOf, bucketOfItem, categoryLabel, CATEGORY_OPTIONS } from "./category";

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

describe("bucketOfItem：分类优先、标签兜底", () => {
  it("有分类就用分类，不看标签", () => {
    expect(bucketOfItem({ category: "电影/HD", tags: ["anime"] })).toBe("movie");
  });

  it("分类为空时看标签 —— Gazelle 的搜索响应没有分类字段", () => {
    expect(bucketOfItem({ category: "", tags: ["movie", "1990s"] })).toBe("movie");
    expect(bucketOfItem({ tags: ["tv.show"] })).toBe("tv");
    expect(bucketOfItem({ tags: ["anime"] })).toBe("anime");
  });

  it("标签里归不进任何桶的（音乐站的流派标签）返回空串", () => {
    expect(bucketOfItem({ tags: ["electronic", "jazz", "1980s"] })).toBe("");
  });

  it("分类与标签都没有时返回空串", () => {
    expect(bucketOfItem({})).toBe("");
    expect(bucketOfItem({ category: null, tags: null })).toBe("");
  });

  it("多个标签时取第一个能归桶的", () => {
    expect(bucketOfItem({ tags: ["1990s", "comedy", "movie"] })).toBe("movie");
  });
});
