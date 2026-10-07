import { describe, expect, it } from "vitest";

import {
  HISTORY_FILTERS,
  MEDIA_MODES,
  MEDIA_SERVER_KINDS,
  TEMPLATE_VARS,
  historyEntry,
  historyStatus,
  itemStatus,
  modeLabel,
  seasonEpisode,
  serverKindLabel,
  templatePresets,
  triggerLabel,
} from "./media";

describe("整理入库的显示工具", () => {
  it("整理方式、媒体服务器种类和后端一致", () => {
    expect(MEDIA_MODES.map((m) => m.value)).toEqual(["hardlink", "copy", "symlink", "move"]);
    expect(modeLabel("")).toBe("硬链接");
    expect(modeLabel("symlink")).toBe("软链接");
    expect(MEDIA_SERVER_KINDS.map((k) => k.value)).toEqual(["emby", "jellyfin", "plex"]);
    expect(serverKindLabel("jellyfin")).toBe("Jellyfin");
    expect(serverKindLabel("kodi")).toBe("kodi");
  });

  it("模板预设：默认是空串；带编号的按各家写法", () => {
    const movie = templatePresets("movie");
    expect(movie[0]).toEqual({ label: "默认", template: "" });
    expect(movie[1]!.template).toContain("[tmdbid={{.TMDBID}}]");
    expect(movie[2]!.template).toContain("[tmdbid-{{.TMDBID}}]");
    expect(movie[3]!.template).toContain("{tmdb-{{.TMDBID}}}");
    expect(templatePresets("tv")[1]!.template).toContain("Season {{.Season}}/");
    expect(TEMPLATE_VARS.map((v) => v.name)).toContain(".SeasonEpisode");
  });

  it("记录状态、计划状态、触发方式", () => {
    expect(historyStatus("done")).toEqual({ label: "已整理", tone: "ok" });
    expect(historyStatus("failed").tone).toBe("dang");
    expect(historyStatus("removed").label).toBe("已删除");
    expect(historyStatus("weird")).toEqual({ label: "weird", tone: "neutral" });
    expect(HISTORY_FILTERS.map((f) => f.value)).toEqual([
      "",
      "done",
      "failed",
      "skipped",
      "removed",
    ]);
    expect(itemStatus("exists")).toEqual({ label: "目标已有文件", tone: "warn" });
    expect(itemStatus("pending").label).toBe("待整理");
    expect(triggerLabel("auto")).toBe("下载完成后自动");
    expect(triggerLabel("scan")).toBe("定期扫描");
    expect(triggerLabel("x")).toBe("x");
  });

  it("季集与条目", () => {
    expect(seasonEpisode(1, 2)).toBe("S01E02");
    expect(seasonEpisode(1, 2, 5)).toBe("S01E02-E05");
    expect(seasonEpisode(2)).toBe("S02");
    const base = {
      title: "最后生还者",
      year: 2023,
      media_type: "tv" as const,
      season: 1,
      episode: 2,
      episode_end: 0,
    };
    expect(historyEntry(base)).toBe("最后生还者 (2023) S01E02");
    expect(historyEntry({ ...base, media_type: "movie", year: 0 })).toBe("最后生还者");
    expect(historyEntry({ ...base, title: "" })).toBe("");
  });
});
