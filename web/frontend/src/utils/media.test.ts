import { describe, expect, it } from "vitest";

import {
  MEDIA_LANGUAGES,
  MEDIA_WORD_KINDS,
  episodeText,
  mediaKindLabel,
  mediaSourceLabel,
  metaTags,
  posterURL,
  tmdbPageURL,
  wordEffect,
  wordKindLabel,
} from "./media";

describe("媒体识别的显示工具", () => {
  it("语言与识别词种类和后端一致", () => {
    expect(MEDIA_LANGUAGES.map((l) => l.value)).toEqual([
      "zh-CN",
      "zh-TW",
      "zh-HK",
      "en-US",
      "ja-JP",
    ]);
    expect(MEDIA_WORD_KINDS.map((k) => k.value)).toEqual(["block", "replace", "offset"]);
    expect(wordKindLabel("offset")).toBe("集数偏移");
  });

  it("类型、来源", () => {
    expect(mediaKindLabel("movie")).toBe("电影");
    expect(mediaKindLabel("tv")).toBe("剧集");
    expect(mediaKindLabel("")).toBe("类型未知");
    expect(mediaSourceLabel("override")).toBe("手动纠正");
    expect(mediaSourceLabel("none")).toBe("没有匹配");
  });

  it("识别词做什么", () => {
    expect(wordEffect({ kind: "block", replacement: "", offset: 0 })).toBe("从标题与副标题里去掉");
    expect(wordEffect({ kind: "replace", replacement: "Joy.of.Life", offset: 0 })).toBe(
      "换成「Joy.of.Life」",
    );
    expect(wordEffect({ kind: "replace", replacement: "", offset: 0 })).toBe("换成空");
    expect(wordEffect({ kind: "offset", replacement: "", offset: -12 })).toBe("集数 -12");
    expect(wordEffect({ kind: "offset", replacement: "", offset: 3 })).toBe("集数 +3");
  });

  it("季与集", () => {
    expect(episodeText({ season: 1, episode: 1, episode_end: 3 })).toBe("S01E01-E03");
    expect(episodeText({ season: 1, season_end: 5, complete: true })).toBe("S01-S05（整季）");
    expect(episodeText({ season: 2, total_episodes: 30, complete: true })).toBe("S02（全 30 集）");
    expect(episodeText({ episode: 5 })).toBe("第 5 集");
    expect(episodeText({ episode: 1, episode_end: 12 })).toBe("第 1-12 集");
    expect(episodeText({ year: 2020 })).toBe("");
  });

  it("解析结果的标签", () => {
    expect(
      metaTags({
        type: "movie",
        year: 2023,
        resolution: "2160p",
        source: "UHD BluRay",
        remux: true,
        video_codec: "H.265",
        bit_depth: 10,
        hdr: ["DV", "HDR"],
        audio: ["TrueHD", "Atmos"],
        channels: "7.1",
        edition: ["IMAX"],
        version: "REPACK",
        chinese_subs: true,
        mandarin: true,
        group: "FGT",
      }),
    ).toEqual([
      "电影",
      "2023",
      "2160p",
      "UHD BluRay Remux",
      "H.265 10bit",
      "DV",
      "HDR",
      "TrueHD Atmos 7.1",
      "IMAX",
      "REPACK",
      "中字",
      "国语",
      "制作组 FGT",
    ]);
    expect(metaTags({})).toEqual([]);
    expect(
      metaTags({
        remux: true,
        channels: "5.1",
        three_d: true,
        cantonese: true,
        fps: 60,
        platform: "Netflix",
      }),
    ).toEqual(["Remux", "Netflix", "60fps", "5.1", "3D", "粤语"]);
  });

  it("TMDB 地址与海报", () => {
    expect(tmdbPageURL("tv", 1399)).toBe("https://www.themoviedb.org/tv/1399");
    expect(posterURL("/p.jpg")).toBe("https://image.tmdb.org/t/p/w185/p.jpg");
    expect(posterURL("p.jpg", "w92")).toBe("https://image.tmdb.org/t/p/w92/p.jpg");
    expect(posterURL()).toBe("");
  });
});
