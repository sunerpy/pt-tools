/*
 * 钉子：订阅页共用的文字。订阅的名字带年份与季；进度写成一句话（剧集写还没播的集）；种子里的集；质量档案的一句话说明。
 */
import { describe, expect, it } from "vitest";

import {
  episodeState,
  keywordLines,
  prefOptions,
  profileSummary,
  progressText,
  sourceLabel,
  subName,
  subStatus,
  torrentSpan,
  torrentStatus,
} from "./subscribe";

describe("订阅的文字", () => {
  it("名字、状态与来源", () => {
    expect(subName({ title: "沙丘2", year: 2024, media_type: "movie", season: 0 })).toBe(
      "沙丘2 (2024)",
    );
    expect(subName({ title: "最后生还者", year: 0, media_type: "tv", season: 2 })).toBe(
      "最后生还者 第 2 季",
    );
    expect(subStatus("pending")).toEqual({ label: "待确认", tone: "warn" });
    expect(subStatus("weird").label).toBe("weird");
    expect(sourceLabel("douban")).toBe("豆瓣想看");
    expect(episodeState("missing").label).toBe("缺");
    expect(torrentStatus("replaced").label).toBe("已替换");
  });

  it("进度", () => {
    expect(progressText("movie", { total: 1, aired: 1, in_library: 1, downloading: 0 })).toBe(
      "已入库",
    );
    expect(progressText("movie", { total: 1, aired: 1, in_library: 0, downloading: 1 })).toBe(
      "下载中",
    );
    expect(progressText("movie", undefined)).toBe("");
    expect(
      progressText("tv", { total: 10, aired: 6, in_library: 3, downloading: 1, missing: [5, 6] }),
    ).toBe("已入库 3/10 · 下载中 1 · 缺 2 · 4 集还没播");
    // 跳过的集不在要追的里：还没播的按要追的数（6 集跳过 1 集，已播 3 集，还没播 2 集）
    expect(
      progressText("tv", {
        total: 6,
        required: 5,
        aired: 3,
        in_library: 0,
        downloading: 3,
        skipped: 1,
      }),
    ).toBe("已入库 0/6 · 下载中 3 · 不追 1 · 2 集还没播");
  });

  it("种子里的集", () => {
    expect(torrentSpan({ complete: true, episode: 0, episode_end: 0 })).toBe("整季");
    expect(torrentSpan({ complete: false, episode: 1, episode_end: 4 })).toBe("第 1–4 集");
    expect(torrentSpan({ complete: false, episode: 3, episode_end: 0 })).toBe("第 3 集");
    expect(torrentSpan({ complete: false, episode: 0, episode_end: 0 })).toBe("");
  });

  it("质量档案", () => {
    expect(prefOptions(false).map((o) => o.value)).toEqual(["", "prefer", "require"]);
    expect(prefOptions(true).map((o) => o.label)).toContain("不要");
    expect(
      profileSummary({
        resolutions: ["2160p", "1080p"],
        sources: ["UHD BluRay"],
        codecs: [],
        remux: "prefer",
        hdr: "avoid",
        chinese_subs: "",
        free: "require",
        min_size_gb: 0,
        max_size_gb: 60,
        exclude_hr: true,
      }),
    ).toBe("2160p > 1080p · UHD BluRay · Remux优先 · HDR不要 · 免费必须有 · 0–60 GB · 不要 H&R");
    expect(
      profileSummary({
        resolutions: [],
        sources: [],
        codecs: [],
        remux: "",
        hdr: "",
        chinese_subs: "",
        free: "",
        min_size_gb: 0,
        max_size_gb: 0,
        exclude_hr: false,
      }),
    ).toBe("分辨率不限");
  });
});

describe("追剧定时的说法（每隔一段时间、cron）", () => {
  it("间隔：整天写天、整小时写小时，其余写分钟", async () => {
    const { intervalText, scheduleText } = await import("./subscribe");
    expect(intervalText(30)).toBe("30 分钟");
    expect(intervalText(90)).toBe("90 分钟");
    expect(intervalText(120)).toBe("2 小时");
    expect(intervalText(2880)).toBe("2 天");
    expect(scheduleText({ kind: "interval", interval_minutes: 360 })).toBe("每隔 6 小时");
    expect(scheduleText({ kind: "cron", cron: "0 22 * * 5; 0 9 * * 6" })).toBe(
      "cron「0 22 * * 5; 0 9 * * 6」",
    );
  });

  it("预览里的时间带星期，不是今年的带年份", async () => {
    const { previewTime } = await import("./subscribe");
    const now = new Date(2026, 9, 22, 10, 0);
    expect(previewTime(new Date(2026, 9, 23, 22, 0).toISOString(), now)).toBe("10-23 周五 22:00");
    expect(previewTime(new Date(2027, 0, 1, 0, 5).toISOString(), now)).toBe(
      "2027-01-01 周五 00:05",
    );
  });
});

describe("标题包含与排除的说法", () => {
  it("按行或逗号拆开；都没填时没有", () => {
    expect(
      keywordLines({ include_keywords: "CHDWEB\nHHWEB", exclude_keywords: "DV，60fps" }),
    ).toEqual(["标题包含 CHDWEB 或 HHWEB", "标题排除 DV、60fps"]);
    expect(keywordLines({ include_keywords: " , ", exclude_keywords: "" })).toEqual([]);
    expect(keywordLines({})).toEqual([]);
  });
});
