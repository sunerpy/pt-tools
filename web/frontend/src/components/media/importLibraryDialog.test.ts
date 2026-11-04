// @vitest-environment happy-dom
/*
 * 钉子（M5）：导入已有文件的弹窗。路径默认是库目录，可以在库目录里浏览；开始扫描以后每秒看一次进度，扫描完了列出每一项，
 * 能导入的默认选上；没认出条目的可以填 TMDB 编号（改好以后选上）；导入排进队列，做完以后写明结果；扫描可以取消。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import type { MediaImportItem, MediaImportScan, MediaLibrary } from "@/api";
import { type MountedView, buttonByText, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  importScan: vi.fn(),
  importScanStatus: vi.fn(),
  cancelImportScan: vi.fn(),
  setImportItem: vi.fn(),
  importItems: vi.fn(),
  fsList: vi.fn(),
}));
const ui = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, organizeApi: api };
});
vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      warning: ui.warning,
      error: ui.error,
    }),
  };
});

let view: MountedView | null = null;
beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
});
afterEach(() => {
  view?.unmount();
  view = null;
  vi.useRealTimers();
  vi.resetAllMocks();
});

const flush = () => vi.advanceTimersByTimeAsync(0);
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);
const qs = (id: string) => [...document.querySelectorAll<HTMLElement>(`[data-testid=${id}]`)];
const findButton = (text: string, scope: ParentNode) =>
  [...scope.querySelectorAll("button")].find((b) => (b.textContent ?? "").trim() === text) ?? null;

const library: MediaLibrary = {
  id: 3,
  name: "电影",
  kind: "movie",
  anime: false,
  path: "/media/movies",
  template: "",
  mode: "hardlink",
  scrape: true,
  scrape_overwrite: false,
  scrape_items: [],
  scrape_image_lang: "",
  scrape_sources: {
    plot: "",
    rating: "",
    episode: "",
    artwork: "",
  },
  classify_dir: false,
  overwrite_mode: "",
  scrape_actor_zh: false,
  scrape_fill: false,
  site_subtitles: false,
  csf_url: "",
  csf_path: "",
  has_csf_key: false,
  enabled: true,
  created_at: "",
  updated_at: "",
  effective_template: "",
  preview: "",
};

const ready: MediaImportItem = {
  path: "/media/movies/Oppenheimer (2023)",
  name: "Oppenheimer (2023)",
  match: { media_type: "movie", tmdb_id: 872585, title: "奥本海默", year: 2023 },
  source: "nfo",
  files: [{ rel: "Oppenheimer.2023.mkv", size: 1 << 30 }],
  file_count: 1,
  size: 1 << 30,
  ready: 1,
  recorded: 0,
};
const unknown: MediaImportItem = {
  path: "/media/movies/Nothing Here (1999)",
  name: "Nothing Here (1999)",
  problem: "没有识别出来：TMDB 上没有可靠的匹配，填 TMDB 编号",
  files: [{ rel: "n.mkv", size: 1 << 20 }],
  file_count: 1,
  size: 1 << 20,
  ready: 1,
  recorded: 0,
};
const recorded: MediaImportItem = {
  path: "/media/movies/已整理 (2023)",
  name: "已整理 (2023)",
  match: { media_type: "movie", tmdb_id: 872585, title: "奥本海默", year: 2023 },
  source: "history",
  files: [{ rel: "o.mkv", size: 1 << 20, recorded: true }],
  file_count: 1,
  size: 1 << 20,
  ready: 0,
  recorded: 1,
};

const scanOf = (over: Partial<MediaImportScan>): MediaImportScan => ({
  id: "s1",
  library_id: 3,
  path: "/media/movies",
  status: "running",
  scanned: 0,
  total: 0,
  started_at: "2026-11-03T12:00:00Z",
  ...over,
});

async function mountDialog(lib = library) {
  const Dialog = (await import("./ImportLibraryDialog.vue")).default;
  const done = vi.fn();
  const Host = defineComponent({
    setup() {
      const open = ref(true);
      return () =>
        h(Dialog, {
          modelValue: open.value,
          "onUpdate:modelValue": (v: boolean) => (open.value = v),
          library: lib,
          onDone: done,
        });
    },
  });
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:p(.*)*", component: { render: () => null } }],
  });
  view = mountView(Host, { router });
  await flush();
  return done;
}

/** 开始扫描，进度走一次，扫描完了列出每一项 */
async function scanned(items: MediaImportItem[]) {
  api.importScan.mockResolvedValue(scanOf({}));
  api.importScanStatus
    .mockResolvedValueOnce(scanOf({ total: 3, scanned: 1 }))
    .mockResolvedValueOnce(scanOf({ status: "done", total: 3, scanned: 3 }))
    .mockResolvedValueOnce(scanOf({ status: "done", total: 3, scanned: 3, items }));
  q("import-scan")!.click();
  await flush();
  await vi.advanceTimersByTimeAsync(1000);
  expect(q("import-progress")!.textContent).toContain("已认 1 / 3 项");
  await vi.advanceTimersByTimeAsync(1000);
  await flush();
}

describe("导入已有文件", () => {
  it("扫描：路径默认是库目录；扫描完了列出每一项，能导入的默认选上，写明条目从哪来、导入不了的原因", async () => {
    await mountDialog();
    expect((q("import-path") as HTMLInputElement).value).toBe("/media/movies");
    await scanned([ready, unknown, recorded]);
    expect(api.importScan).toHaveBeenCalledWith(3, "/media/movies");
    expect(api.importScanStatus).toHaveBeenNthCalledWith(1, "s1", true);
    expect(api.importScanStatus).toHaveBeenLastCalledWith("s1");
    expect(q("import-counts")!.textContent).toContain("能导入 1 项（1 个视频）");
    expect(q("import-counts")!.textContent).toContain("已有记录 1 项");
    expect(q("import-counts")!.textContent).toContain("导入不了 1 项");
    const rows = qs("import-item");
    expect(rows).toHaveLength(3);
    expect(rows[0]!.textContent).toContain("奥本海默 (2023)");
    expect(rows[0]!.textContent).toContain("按 NFO 里的 TMDB 编号");
    expect(rows[1]!.textContent).toContain("没有识别出来");
    expect(rows[2]!.textContent).toContain("1 个已经有整理记录");
    expect(q("import-run")!.textContent).toContain("导入 1 项（1 个视频）");

    // 只看导入不了的
    const label = [...q("import-filter")!.querySelectorAll("label")].find((l) =>
      l.textContent?.includes("导入不了"),
    )!;
    label.querySelector("input")!.click();
    await flush();
    expect(qs("import-item")).toHaveLength(1);
  });

  it("填 TMDB 编号：改好以后选上；导入排进队列，做完以后写明结果", async () => {
    const done = await mountDialog();
    await scanned([ready, unknown, recorded]);
    const row = qs("import-item")[1]!;
    buttonByText("填 TMDB 编号", row).click();
    await flush();
    const input = q("import-edit-id")!.querySelector("input")!;
    input.value = "872585";
    input.dispatchEvent(new Event("input"));
    input.dispatchEvent(new Event("change"));
    await flush();
    api.setImportItem.mockResolvedValue({
      ...unknown,
      problem: undefined,
      match: ready.match,
      source: "manual",
      edited: true,
    });
    q("import-edit-save")!.click();
    await flush();
    expect(api.setImportItem).toHaveBeenCalledWith("s1", unknown.path, 872585);
    expect(q("import-run")!.textContent).toContain("导入 2 项（2 个视频）");
    expect(qs("import-item")[1]!.textContent).toContain("手动指定");

    api.importItems.mockResolvedValue({
      status: "queued",
      items: 2,
      done: 0,
      created: 0,
      skipped: 0,
      failed: 0,
      scraped: 0,
    });
    const run = {
      status: "done" as const,
      items: 2,
      done: 2,
      created: 2,
      skipped: 0,
      failed: 0,
      scraped: 2,
    };
    api.importScanStatus.mockResolvedValue(scanOf({ status: "done", import: run }));
    q("import-run")!.click();
    await flush();
    expect(api.importItems).toHaveBeenCalledWith("s1", [ready.path, unknown.path], true);
    expect(q("import-result")!.textContent).toContain("排进了整理队列");
    await vi.advanceTimersByTimeAsync(1000);
    await flush();
    expect(q("import-result")!.textContent).toContain("导入了 2 个视频，补刮了 2 部");
    expect(ui.success).toHaveBeenCalledWith("导入了 2 个视频，补刮了 2 部");
    expect(done).toHaveBeenCalledTimes(1);
    // 导入了的项算作已有记录：写「已导入」、不能再选，导入按钮不能点
    const rows = qs("import-item");
    expect(rows[0]!.textContent).toContain("已导入");
    expect(rows[1]!.textContent).toContain("已导入");
    expect(rows[0]!.querySelector(".el-checkbox")!.classList.contains("is-disabled")).toBe(true);
    expect(findButton("改编号", rows[0]!), "导入了的不能再改编号").toBeNull();
    expect(q("import-counts")!.textContent).toContain("能导入 0 项（0 个视频）");
    expect(q("import-counts")!.textContent).toContain("已有记录 3 项");
    expect((q("import-run") as HTMLButtonElement).disabled).toBe(true);
  });

  it("原盘这类导入不了的项：只写原因，不能填编号", async () => {
    await mountDialog();
    const disc: MediaImportItem = {
      path: "/media/movies/Disc (2012)",
      name: "Disc (2012)",
      problem: "原盘暂不导入",
      files: [],
      file_count: 0,
      size: 0,
      ready: 0,
      recorded: 0,
    };
    await scanned([disc, unknown]);
    const [discRow, unknownRow] = qs("import-item");
    expect(discRow!.textContent).toContain("原盘暂不导入");
    expect(discRow!.textContent, "没有视频：不写 0 个视频").not.toContain("0 个视频");
    expect(findButton("填 TMDB 编号", discRow!)).toBeNull();
    expect(buttonByText("填 TMDB 编号", unknownRow!)).not.toBeNull();
  });

  it("导入时出错写明；改编号出错写明", async () => {
    await mountDialog();
    await scanned([ready, unknown]);
    buttonByText("改编号", qs("import-item")[0]!).click();
    await flush();
    api.setImportItem.mockRejectedValue(new Error("TMDB 上没有编号是 1 的电影"));
    q("import-edit-save")!.click();
    await flush();
    expect(ui.error).toHaveBeenCalledWith("TMDB 上没有编号是 1 的电影");
    api.importItems.mockRejectedValue(new Error("整理队列已满，稍后再试"));
    q("import-run")!.click();
    await flush();
    expect(ui.error).toHaveBeenCalledWith("整理队列已满，稍后再试");
  });

  it("取消扫描；开始不了时写明原因；媒体库关着刮削时不能补刮", async () => {
    await mountDialog({ ...library, scrape: false });
    expect(q("import-scrape")!.querySelector<HTMLInputElement>("input")!.disabled, "关着刮削").toBe(
      true,
    );
    api.importScan.mockRejectedValueOnce(new Error("TMDB 不能用：代理连不上"));
    q("import-scan")!.click();
    await flush();
    expect(q("import-error")!.textContent).toContain("TMDB 不能用");

    api.importScan.mockResolvedValue(scanOf({}));
    api.importScanStatus.mockResolvedValue(scanOf({ total: 5, scanned: 2 }));
    q("import-scan")!.click();
    await flush();
    await vi.advanceTimersByTimeAsync(1000);
    api.cancelImportScan.mockResolvedValue({ ok: true });
    api.importScanStatus.mockResolvedValue(scanOf({ status: "canceled" }));
    q("import-cancel")!.click();
    await flush();
    await flush();
    expect(api.cancelImportScan).toHaveBeenCalledWith("s1");
    expect(q("import-canceled")).not.toBeNull();
  });

  it("浏览：只在库目录里，上一级到库目录为止，选一个文件夹", async () => {
    await mountDialog();
    api.fsList
      .mockResolvedValueOnce({
        path: "/media/movies",
        root: "/media/movies",
        entries: [
          { name: "华语电影", path: "/media/movies/华语电影", dir: true, mod_time: "" },
          { name: "a.mkv", path: "/media/movies/a.mkv", dir: false, size: 1, mod_time: "" },
        ],
      })
      .mockResolvedValueOnce({
        path: "/media/movies/华语电影",
        root: "/media/movies",
        parent: "/media/movies",
        entries: [],
      });
    q("import-browse")!.click();
    await flush();
    expect(api.fsList).toHaveBeenCalledWith("/media/movies");
    expect(q("import-fs-dirs")!.textContent).toContain("华语电影");
    expect(q("import-fs-dirs")!.textContent, "只列文件夹").not.toContain("a.mkv");
    expect((q("import-fs-up") as HTMLButtonElement).disabled, "库目录里不能再往上").toBe(true);
    buttonByText("华语电影", q("import-fs-dirs")!).click();
    await flush();
    expect(api.fsList).toHaveBeenLastCalledWith("/media/movies/华语电影");
    expect((q("import-fs-up") as HTMLButtonElement).disabled).toBe(false);
    q("import-fs-pick")!.click();
    await flush();
    expect((q("import-path") as HTMLInputElement).value).toBe("/media/movies/华语电影");
  });

  it("剧集库写明季集；看每个视频", async () => {
    await mountDialog({ ...library, kind: "tv" });
    const show: MediaImportItem = {
      path: "/media/tv/最后生还者",
      name: "最后生还者",
      match: { media_type: "tv", tmdb_id: 100088, title: "最后生还者", year: 2023 },
      source: "search",
      files: [
        { rel: "Season 1/S01E01.mkv", size: 1, season: 1, episode: 1 },
        { rel: "Season 1/S01E02.mkv", size: 1, season: 1, episode: 2 },
        { rel: "x.mkv", size: 1, season: 1, problem: "文件名里没认出集数" },
      ],
      file_count: 3,
      size: 3,
      ready: 2,
      recorded: 0,
    };
    await scanned([show]);
    const row = qs("import-item")[0]!;
    expect(row.textContent).toContain("第 1 季 1–2 集");
    buttonByText("看视频（1 个导入不了）", row).click();
    await flush();
    const files = q("import-files")!.textContent ?? "";
    expect(files).toContain("S01E02");
    expect(files).toContain("文件名里没认出集数");
  });
});
