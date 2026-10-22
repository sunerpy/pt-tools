// @vitest-environment happy-dom
/*
 * 钉子：搜索结果的「追剧」。先用这个种子的标题识别，认出剧集与季以后打开追剧弹窗（季预先填好、带着这个种子），
 * 开始追时把站点、种子编号与标题一起交给服务端；认不出剧集时写明怎么办，不打开弹窗。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ multiSite: vi.fn() }));
const media = vi.hoisted(() => ({
  recognize: vi.fn(),
  settings: vi.fn(async () => ({ image_base_url: "" })),
}));
const subs = vi.hoisted(() => ({
  create: vi.fn(),
  profiles: vi.fn(async () => ({ items: [], resolutions: [], sources: [], codecs: [] })),
}));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    searchApi: {
      getSites: vi.fn(() => Promise.resolve(["mteam"])),
      multiSite: api.multiSite,
      clearCache: vi.fn(),
    },
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    siteCategoriesApi: { getAll: vi.fn(() => Promise.resolve({})) },
    torrentPushApi: { push: vi.fn(), batchPush: vi.fn() },
    sitesApi: { list: vi.fn(() => Promise.resolve({ mteam: { enabled: true } })) },
    organizeApi: { libraries: vi.fn(() => Promise.resolve([])) },
    mediaApi: media,
    subscribeApi: subs,
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), ui),
  };
});

const TITLE = "Natural High S04E05 2026 1080p WEB-DL H264 AAC-HHWEB";
const results = {
  items: [
    {
      id: "1234",
      title: TITLE,
      subtitle: "现在就出发 第四季 第3期上",
      category: "综艺",
      isFree: true,
      sizeBytes: 1024 ** 3,
      seeders: 9,
      leechers: 0,
      snatched: 3,
      sourceSite: "mteam",
    },
  ],
  totalResults: 1,
  siteResults: { mteam: 1 },
  errors: [],
  durationMs: 10,
};
const show = (id: number, title: string) => ({
  id,
  media_type: "tv",
  title,
  original_title: title,
  year: 2023,
});

let view: MountedView | null = null;
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);

async function search() {
  sessionStorage.clear();
  api.multiSite.mockResolvedValue(results);
  const TorrentSearch = (await import("./TorrentSearch.vue")).default;
  view = mountView(TorrentSearch);
  const box = "input[placeholder='输入关键词，回车即搜']";
  await vi.waitFor(() => expect(document.querySelector(box)).not.toBeNull());
  const input = document.querySelector<HTMLInputElement>(box)!;
  input.value = "现在就出发";
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await Promise.resolve();
  input.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
  await vi.waitFor(() => expect(q("ts-follow-mteam:1234")).not.toBeNull());
}

afterEach(() => {
  view?.unmount();
  view = null;
  sessionStorage.clear();
  vi.clearAllMocks();
});

describe("种子搜索：追剧", () => {
  it("认出剧集与季：弹窗里预先填好，开始追时带上这个种子", async () => {
    media.recognize.mockResolvedValue({
      meta: { type: "tv", season: 4, episode: 5 },
      summary: "",
      source: "search",
      match: show(231620, "现在就出发"),
      candidates: [
        { ...show(231620, "现在就出发"), score: 1 },
        { ...show(1, "出发吧"), score: 0.5 },
        { id: 9, media_type: "movie", title: "出发", original_title: "", score: 0.4 },
      ],
    });
    await search();
    q("ts-follow-mteam:1234")!.click();
    await vi.waitFor(() =>
      expect(media.recognize).toHaveBeenCalledWith({
        title: TITLE,
        subtitle: "现在就出发 第四季 第3期上",
      }),
    );
    await vi.waitFor(() => expect(q("sd-from")).not.toBeNull());
    expect(q("sd-from")!.textContent).toContain(TITLE);
    expect(q("sd-name")!.textContent).toContain("现在就出发 (2023)");
    expect(q("sd-candidate"), "有别的剧集候选可以改选").not.toBeNull();
    const season = q("sd-season")!.querySelector("input")!;
    expect(season.value).toBe("4");
    subs.create.mockResolvedValue({ id: 5, title: "现在就出发", message: "推送了第 5 集" });
    q("sd-save")!.click();
    await vi.waitFor(() => expect(subs.create).toHaveBeenCalledTimes(1));
    const body = subs.create.mock.calls[0]![0];
    expect(body).toMatchObject({
      media_type: "tv",
      tmdb_id: 231620,
      season: 4,
      lock_version: true,
      schedule: { kind: "airtime" },
      from_torrent: { site: "mteam", torrent_id: "1234", title: TITLE },
    });
    expect(body.start_episode, "起始集交给服务端按这个种子定").toBeUndefined();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("开始追剧：现在就出发"));
    expect(ui.info).toHaveBeenCalledWith("推送了第 5 集");
  });

  it("认不出剧集：写明怎么办，不打开弹窗", async () => {
    media.recognize.mockResolvedValue({
      meta: { type: "movie" },
      summary: "",
      source: "search",
      match: { id: 9, media_type: "movie", title: "出发", original_title: "" },
    });
    await search();
    q("ts-follow-mteam:1234")!.click();
    await vi.waitFor(() =>
      expect(ui.warning).toHaveBeenCalledWith(
        "认不出这个种子是哪部剧：可以先在「媒体识别」里纠正，或者去「探索」搜这部剧",
      ),
    );
    expect(q("sd-from")).toBeNull();
  });
});
