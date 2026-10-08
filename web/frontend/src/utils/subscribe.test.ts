/*
 * 钉子：订阅页共用的文字。订阅的名字带年份与季；进度写成一句话（剧集写还没播的集）；种子里的集；质量档案的一句话说明。
 */
import { describe, expect, it } from "vitest";

import {
  episodeState,
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
