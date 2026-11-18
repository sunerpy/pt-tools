/*
 * 钉子（M5）：导入已有媒体库的几句话：一项能不能导入、剧集的季集、导入的结果。
 */
import { describe, expect, it } from "vitest";

import type { MediaImportItem } from "@/api";
import { matchSourceLabel, modeLabel, triggerLabel } from "@/utils/media";
import { importEpisodes, importItemState, importRunText } from "@/utils/mediaImport";

const item = (over: Partial<MediaImportItem>): MediaImportItem => ({
  path: "/m/a",
  name: "a",
  files: [],
  file_count: 1,
  size: 1,
  ready: 1,
  recorded: 0,
  match: { media_type: "movie", tmdb_id: 1, title: "A" },
  ...over,
});

describe("导入已有媒体库", () => {
  it("一项能导入、已经有记录、导入不了", () => {
    expect(importItemState(item({}))).toBe("ready");
    expect(importItemState(item({ ready: 0, recorded: 1 }))).toBe("recorded");
    expect(importItemState(item({ problem: "原盘暂不导入" }))).toBe("problem");
    expect(importItemState(item({ match: undefined }))).toBe("problem");
  });

  it("剧集的季集：连续的写成区间，几集合在一个文件里的展开，特别篇放最后，没认出集数的不算", () => {
    expect(
      importEpisodes([
        { rel: "a", size: 1, season: 2, episode: 1 },
        { rel: "b", size: 1, season: 1, episode: 3 },
        { rel: "c", size: 1, season: 1, episode: 1, episode_end: 2 },
        { rel: "d", size: 1, season: 1, episode: 5 },
        { rel: "e", size: 1, season: 0, episode: 1 },
        { rel: "f", size: 1, season: 1, episode: 0, problem: "没认出集数" },
        { rel: "g", size: 1, season: 1, episode: 3 },
      ]),
    ).toBe("第 1 季 1–3、5 集、第 2 季 1 集、特别篇 1 集");
    expect(importEpisodes([{ rel: "a", size: 1, season: 1, episode: 1 }], 300)).toBe(
      "第 1 季 1 集（只列了前面的视频）",
    );
    expect(importEpisodes([{ rel: "a", size: 1 }])).toBe("");
  });

  it("导入的结果", () => {
    const run = {
      status: "done" as const,
      items: 3,
      done: 3,
      created: 10,
      skipped: 0,
      failed: 0,
      scraped: 0,
    };
    expect(importRunText(run)).toBe("导入了 10 个视频");
    expect(importRunText({ ...run, skipped: 2, failed: 1, scraped: 3 })).toBe(
      "导入了 10 个视频，2 个已经有整理记录，跳过了，1 处导入不了（原因见下面），补刮了 3 部",
    );
    expect(importRunText({ ...run, status: "failed" })).toBe("没导入完：导入了 10 个视频");
    expect(importRunText({ ...run, status: "queued" })).toBe(
      "排进了整理队列，等前面的整理做完就导入（3 项）",
    );
    expect(importRunText({ ...run, status: "running", done: 1 })).toBe("正在导入：1 / 3 项");
  });

  it("整理历史里写明导入的记录", () => {
    expect(modeLabel("existing")).toBe("已有文件");
    expect(triggerLabel("import")).toBe("导入已有文件");
    expect(matchSourceLabel("nfo")).toBe("按 NFO 里的 TMDB 编号");
    expect(matchSourceLabel("nfo-imdb")).toBe("按 NFO 里的 IMDb 编号");
    expect(matchSourceLabel("dirname")).toBe("按目录名里的 TMDB 编号");
  });
});
