// @vitest-environment happy-dom
/*
 * 钉子：整理历史。按状态与关键字筛选；失败与跳过的可以重试并写明结果；删除时可选连库里的文件一起删，
 * 移动整理的与没整理出文件的不能选；读不到时写明原因。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ history: vi.fn(), retry: vi.fn(), deleteHistory: vi.fn() }));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}));

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
      error: ui.error,
      warning: ui.warning,
      info: ui.info,
    }),
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

const row = (over: Record<string, unknown>) => ({
  id: 1,
  downloader_id: 1,
  downloader_name: "qb",
  info_hash: "abc",
  task_id: "abc",
  torrent_name: "The.Last.of.Us.S01.2160p",
  library_id: 2,
  library_name: "剧集",
  source_path: "/data/downloads/The.Last.of.Us.S01/E01.mkv",
  target_path: "/media/tv/最后生还者 (2023)/Season 1/最后生还者 - S01E01.mkv",
  mode: "hardlink",
  media_type: "tv",
  tmdb_id: 100088,
  title: "最后生还者",
  year: 2023,
  season: 1,
  episode: 1,
  episode_end: 0,
  size: 1 << 30,
  status: "done",
  message: "",
  attempts: 0,
  trigger: "auto",
  subtitles: 1,
  has_files: true,
  created_at: "2026-10-07T12:00:00Z",
  updated_at: "2026-10-07T12:00:00Z",
  ...over,
});

async function mountPage(fail = false) {
  if (fail) api.history.mockRejectedValue(new Error("读取整理记录失败"));
  else
    api.history.mockResolvedValue({
      items: [
        row({}),
        row({
          id: 2,
          status: "failed",
          message: "识别失败：TMDB 暂时不能访问",
          attempts: 1,
          next_retry_at: "2026-10-07T12:10:00Z",
          target_path: "",
          has_files: false,
        }),
        row({ id: 3, mode: "move" }),
        row({
          id: 4,
          status: "skipped",
          message: "库里的文件已经不是当初整理出的那个（换成了别的文件或改过），不会覆盖",
        }),
      ],
      total: 4,
    });
  const Page = (await import("./MediaHistory.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.history).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("整理历史", () => {
  it("列表：条目、文件、方式与状态；失败的写明原因与下次重试", async () => {
    await mountPage();
    expect(api.history).toHaveBeenCalledWith({ status: "", q: "", limit: 50, offset: 0 });
    const text = q("mh-table")!.textContent ?? "";
    expect(text).toContain("最后生还者 (2023) S01E01");
    expect(text).toContain("E01.mkv");
    expect(text).toContain("1 个字幕");
    expect(text).toContain("硬链接");
    expect(text).toContain("已整理");
    expect(text).toContain("下载完成后自动");
    expect(text).toContain("识别失败：TMDB 暂时不能访问");
    expect(text).toContain("自动重试（第 1 次失败）");
    expect(q("mh-retry-2")).not.toBeNull();
    expect(q("mh-retry-1"), "整理成功的不能重试").toBeNull();
  });

  it("筛选：按状态与关键字重新读，回到第一页", async () => {
    await mountPage();
    const failed = [...q("mh-status")!.querySelectorAll("label")].find((l) =>
      l.textContent?.includes("失败"),
    )!;
    failed.querySelector("input")!.click();
    await vi.waitFor(() => expect(api.history).toHaveBeenCalledTimes(2));
    expect(api.history.mock.calls[1]![0]).toMatchObject({ status: "failed" });
    const el = q("mh-keyword")!;
    const kw = (el instanceof HTMLInputElement ? el : el.querySelector("input"))!;
    kw.value = " 最后生还者 ";
    kw.dispatchEvent(new Event("input"));
    kw.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter" }));
    await vi.waitFor(() => expect(api.history).toHaveBeenCalledTimes(3));
    expect(api.history.mock.calls[2]![0]).toMatchObject({
      q: "最后生还者",
      status: "failed",
      offset: 0,
    });
  });

  it("重试：写明结果并刷新", async () => {
    await mountPage();
    api.retry.mockResolvedValue({ created: 2, done: 0, skipped: 0, failed: 0 });
    q("mh-retry-2")!.click();
    await vi.waitFor(() => expect(api.retry).toHaveBeenCalledWith(2));
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("新入库 2 个，已在库里 0 个"));
    expect(api.history).toHaveBeenCalledTimes(2);
    // 刷新完、按钮退出加载态以后才能再点
    await vi.waitFor(() => expect(q("mh-retry-2")!.classList.contains("is-loading")).toBe(false));

    api.retry.mockResolvedValue({
      created: 0,
      done: 0,
      skipped: 0,
      failed: 1,
      plan: { problem: "没有启用的剧集媒体库" },
    });
    q("mh-retry-2")!.click();
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalledWith("没有启用的剧集媒体库"));
  });

  it("删除：可选连库里的文件一起删；移动整理的不能选", async () => {
    await mountPage();
    api.deleteHistory.mockResolvedValue({ ok: true });
    q("mh-del-1")!.click();
    await flush();
    const box = q("mh-del-files")!;
    expect(box.classList.contains("is-disabled")).toBe(false);
    box.querySelector("input")!.click();
    await flush();
    q("mh-del-confirm")!.click();
    await vi.waitFor(() => expect(api.deleteHistory).toHaveBeenCalledWith(1, true));
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("已删除记录与库里的文件"));

    q("mh-del-3")!.click();
    await flush();
    expect(q("mh-del-files")!.classList.contains("is-disabled"), "移动整理的不能连文件删").toBe(
      true,
    );
    expect(document.body.textContent).toContain("移动整理的文件是唯一的一份");
    q("mh-del-confirm")!.click();
    await vi.waitFor(() => expect(api.deleteHistory).toHaveBeenCalledWith(3, false));

    q("mh-del-2")!.click();
    await flush();
    expect(q("mh-del-files")!.classList.contains("is-disabled"), "没整理出文件的不能选").toBe(true);
    expect(document.body.textContent).toContain("这条记录没有整理出文件");
  });

  it("删除：跳过的记录记着之前整理出的文件时也能连文件删，换掉的文件写明留着", async () => {
    await mountPage();
    api.deleteHistory.mockResolvedValue({
      ok: true,
      kept: ["库里的 最后生还者 - S01E01.mkv 已经换成了别的文件或改过，没有删除"],
    });
    q("mh-del-4")!.click();
    await flush();
    const box = q("mh-del-files")!;
    expect(box.classList.contains("is-disabled")).toBe(false);
    box.querySelector("input")!.click();
    await flush();
    q("mh-del-confirm")!.click();
    await vi.waitFor(() => expect(api.deleteHistory).toHaveBeenCalledWith(4, true));
    await vi.waitFor(() =>
      expect(ui.warning).toHaveBeenCalledWith(expect.stringContaining("这些文件留着")),
    );
    expect(ui.success).not.toHaveBeenCalled();
  });

  it("读不到时写明原因", async () => {
    await mountPage(true);
    const state = q("mh-state")!.textContent ?? "";
    expect(state).toContain("读取整理记录失败");
    expect(state).not.toContain("还没有整理记录");
  });
});
