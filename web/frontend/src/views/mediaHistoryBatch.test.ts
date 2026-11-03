// @vitest-environment happy-dom
/*
 * 钉子（M4b）：整理历史的失败分类、批量操作、按条目分组与「让 AI 认」。
 * 失败与跳过的记录写明分类（升级前没有分类的算其他），按分类筛选只在看全部、失败、跳过时有；勾选几条出批量条，
 * 重试只发能重试的、重新刮削只发整理好的，删除先确认；「按条目」列分组，点一组看组里的记录，「返回分组」回去；
 * 没识别出来的记录「让 AI 认」打开整理弹窗并认一次。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  history: vi.fn(),
  historyGroups: vi.fn(),
  historyBatch: vi.fn(),
  retry: vi.fn(),
  rescrape: vi.fn(),
  deleteHistory: vi.fn(),
  preview: vi.fn(),
  organize: vi.fn(),
  libraries: vi.fn(),
  aiRecognize: vi.fn(),
  aiStatus: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    organizeApi: api,
    mediaApi: { ...real.mediaApi, settings: vi.fn(async () => ({ image_base_url: "" })) },
  };
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
    ElMessageBox: { ...real.ElMessageBox, confirm: ui.confirm },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.resetAllMocks();
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
  chosen_mode: "",
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
  fail_kind: "",
  attempts: 0,
  trigger: "auto",
  subtitles: 0,
  has_files: true,
  scrape_status: "",
  scrape_note: "",
  created_at: "2026-11-03T12:00:00Z",
  updated_at: "2026-11-03T12:00:00Z",
  ...over,
});

const rows = [
  row({}),
  row({
    id: 2,
    status: "failed",
    fail_kind: "recognize",
    message: "没有识别出来：TMDB 上没有可靠的匹配",
    media_type: "",
    tmdb_id: 0,
    title: "",
    torrent_name: "Unknown.2020",
    target_path: "",
    has_files: false,
  }),
  row({ id: 3, status: "skipped", fail_kind: "", message: "旧记录" }),
  row({ id: 4, status: "failed", fail_kind: "exists", message: "目标位置已经有同名文件" }),
];

async function mountPage(mobile = false, list = rows) {
  if (mobile) {
    vi.spyOn(window, "matchMedia").mockImplementation(
      (query: string) =>
        ({
          matches: query.includes("max-width: 768px"),
          media: query,
          onchange: null,
          addEventListener: () => {},
          removeEventListener: () => {},
          addListener: () => {},
          removeListener: () => {},
          dispatchEvent: () => false,
        }) as unknown as MediaQueryList,
    );
  }
  api.history.mockResolvedValue({ items: list, total: list.length });
  api.aiStatus.mockResolvedValue({ ready: true });
  const Page = (await import("./MediaHistory.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:p(.*)*", component: { render: () => null } }],
  });
  view = mountView(Page, { router });
  await vi.waitFor(() => expect(api.history).toHaveBeenCalled());
  await flush();
  await flush();
}

/** 勾选表格里第 i 行（从 0 数） */
async function tick(i: number) {
  document.querySelectorAll<HTMLInputElement>(".el-table__body .el-checkbox__original")[i]!.click();
  await flush();
}

async function pickKind(label: string) {
  q("mh-kind")!.click();
  await flush();
  const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
    (el) => el.textContent?.trim() === label,
  );
  if (!opt) throw new Error(`分类里没有「${label}」`);
  opt.click();
  await flush();
}

function radio(id: string, label: string) {
  const l = [...q(id)!.querySelectorAll("label")].find((x) => x.textContent?.includes(label))!;
  l.querySelector("input")!.click();
}

describe("整理历史：失败分类", () => {
  it("写明失败与跳过的分类，升级前没有分类的算其他；整理成的不写", async () => {
    await mountPage();
    expect(q("mh-kind-2")!.textContent).toBe("没识别出来");
    expect(q("mh-kind-3")!.textContent).toBe("其他");
    expect(q("mh-kind-4")!.textContent).toBe("目标已有文件");
    expect(q("mh-kind-1")).toBeNull();
  });

  it("按分类筛选；看已整理、已删除时没有分类筛选，换过去时清掉", async () => {
    await mountPage();
    await pickKind("没识别出来");
    await vi.waitFor(() => expect(api.history).toHaveBeenCalledTimes(2));
    expect(api.history.mock.calls[1]![0]).toEqual({
      status: "",
      q: "",
      limit: 50,
      offset: 0,
      kind: "recognize",
    });

    radio("mh-status", "已整理");
    await vi.waitFor(() => expect(api.history).toHaveBeenCalledTimes(3));
    expect(api.history.mock.calls[2]![0]).toEqual({ status: "done", q: "", limit: 50, offset: 0 });
    expect(q("mh-kind")).toBeNull();
    radio("mh-status", "失败");
    await vi.waitFor(() => expect(api.history).toHaveBeenCalledTimes(4));
    expect(q("mh-kind")).not.toBeNull();
  });
});

describe("整理历史：批量操作", () => {
  it("勾选以后出批量条；重试只发失败、跳过的，重新刮削只发整理好的，写明结果并刷新", async () => {
    await mountPage();
    expect(q("mh-band")).toBeNull();
    await tick(0);
    await tick(1);
    await tick(2);
    expect(q("mh-band")!.textContent).toContain("已选 3 条");
    expect(q("mh-batch-retry")!.textContent).toContain("重试 2 条");
    expect(q("mh-batch-rescrape")!.textContent).toContain("重新刮削 1 条");

    api.historyBatch.mockResolvedValue({
      items: [
        { id: 2, ok: true, message: "排进了整理队列" },
        { id: 3, ok: true, message: "排进了整理队列" },
      ],
      ok: 2,
      failed: 0,
    });
    q("mh-batch-retry")!.click();
    await vi.waitFor(() => expect(api.historyBatch).toHaveBeenCalledWith([2, 3], "retry"));
    await vi.waitFor(() =>
      expect(ui.success).toHaveBeenCalledWith("已重试 2 条（排进了整理队列，稍后刷新看结果）"),
    );
    expect(api.history).toHaveBeenCalledTimes(2);
    await vi.waitFor(() => expect(q("mh-band")).toBeNull());

    await tick(0);
    api.historyBatch.mockResolvedValue({
      items: [{ id: 1, ok: false, message: "媒体库「剧集」关着刮削：先在媒体库里打开" }],
      ok: 0,
      failed: 1,
    });
    q("mh-batch-rescrape")!.click();
    await vi.waitFor(() => expect(api.historyBatch).toHaveBeenCalledWith([1], "rescrape"));
    await vi.waitFor(() =>
      expect(ui.warning).toHaveBeenCalledWith(
        "已重新刮削 0 条，1 条没做成：媒体库「剧集」关着刮削：先在媒体库里打开",
      ),
    );
  });

  it("只选了整理好的：重试不能点；选了失败的：重新刮削不能点", async () => {
    await mountPage();
    await tick(0);
    expect((q("mh-batch-retry") as HTMLButtonElement).disabled).toBe(true);
    expect((q("mh-batch-rescrape") as HTMLButtonElement).disabled).toBe(false);
    await tick(0);
    await tick(1);
    expect((q("mh-batch-rescrape") as HTMLButtonElement).disabled).toBe(true);
  });

  it("删除先确认；取消时不发；连文件删写明移动整理的只删记录", async () => {
    await mountPage();
    await tick(0);
    await tick(3);
    ui.confirm.mockRejectedValueOnce("cancel");
    q("mh-batch-delete-files")!.click();
    await vi.waitFor(() => expect(ui.confirm).toHaveBeenCalledTimes(1));
    expect(ui.confirm.mock.calls[0]![0]).toContain("移动整理的文件是唯一的一份");
    expect(ui.confirm.mock.calls[0]![2], "删除用红色的确认按钮").toMatchObject({
      confirmButtonClass: "el-button--danger",
    });
    await flush();
    expect(api.historyBatch).not.toHaveBeenCalled();

    ui.confirm.mockResolvedValueOnce("confirm");
    api.historyBatch.mockResolvedValue({
      items: [
        { id: 1, ok: true },
        { id: 4, ok: true },
      ],
      ok: 2,
      failed: 0,
    });
    q("mh-batch-delete")!.click();
    await vi.waitFor(() => expect(api.historyBatch).toHaveBeenCalledWith([1, 4], "delete"));
    expect(ui.confirm.mock.calls[1]![0]).toContain("库里与下载目录里的文件都不动");
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("已删除记录 2 条"));
  });

  it("批量操作出错时写明原因", async () => {
    await mountPage();
    await tick(1);
    api.historyBatch.mockRejectedValue(new Error("一次最多 200 条"));
    q("mh-batch-retry")!.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("一次最多 200 条"));
  });

  it("手机行卡上也能勾选", async () => {
    await mountPage(true);
    q("mh-check-2")!.querySelector<HTMLInputElement>("input")!.click();
    await flush();
    expect(q("mh-band")!.textContent).toContain("已选 1 条");
    q("mh-check-2")!.querySelector<HTMLInputElement>("input")!.click();
    await flush();
    expect(q("mh-band")).toBeNull();
    vi.restoreAllMocks();
  });
});

describe("整理历史：按条目", () => {
  const groups = [
    {
      media_type: "tv",
      tmdb_id: 100088,
      season: 1,
      title: "最后生还者",
      year: 2023,
      total: 3,
      done: 2,
      failed: 1,
      skipped: 0,
      removed: 0,
      latest_at: "2026-11-03T12:00:00Z",
    },
    {
      torrent: "Unknown.2020",
      total: 1,
      done: 0,
      failed: 1,
      skipped: 0,
      removed: 0,
      latest_at: "2026-11-03T11:00:00Z",
    },
  ];

  it("列分组；点一组看组里的记录（带组的条件），返回分组", async () => {
    await mountPage();
    api.historyGroups.mockResolvedValue({ groups, total: 2 });
    radio("mh-view", "按条目");
    await vi.waitFor(() => expect(api.historyGroups).toHaveBeenCalledTimes(1));
    expect(api.historyGroups.mock.calls[0]![0]).toEqual({
      status: "",
      q: "",
      limit: 50,
      offset: 0,
    });
    await vi.waitFor(() => expect(q("mh-groups")).not.toBeNull());
    const text = q("mh-groups")!.textContent ?? "";
    expect(text).toContain("最后生还者 (2023) S01");
    expect(text).toContain("3 条：已整理 2 · 失败 1");
    expect(text).toContain("Unknown.2020");
    expect(text).toContain("没识别出来的种子");
    expect(q("mh-band")).toBeNull();

    q("mh-group-0")!.click();
    await vi.waitFor(() => expect(api.history).toHaveBeenCalledTimes(2));
    expect(api.history.mock.calls[1]![0]).toEqual({
      status: "",
      q: "",
      limit: 50,
      offset: 0,
      media_type: "tv",
      tmdb_id: 100088,
      season: 1,
    });
    expect(q("mh-group-bar")!.textContent).toContain("最后生还者 (2023) S01");
    await vi.waitFor(() => expect(q("mh-table")).not.toBeNull());

    q("mh-group-back")!.click();
    await vi.waitFor(() => expect(api.historyGroups).toHaveBeenCalledTimes(2));
    expect(q("mh-group-bar")).toBeNull();
    await vi.waitFor(() => expect(q("mh-group-1")).not.toBeNull());

    q("mh-group-1")!.click();
    await vi.waitFor(() => expect(api.history).toHaveBeenCalledTimes(3));
    expect(api.history.mock.calls[2]![0]).toMatchObject({ torrent: "Unknown.2020" });

    radio("mh-view", "按记录");
    await vi.waitFor(() => expect(api.history).toHaveBeenCalledTimes(4));
    expect(api.history.mock.calls[3]![0]).toEqual({ status: "", q: "", limit: 50, offset: 0 });
  });

  it("分组为空、读不到时写明", async () => {
    await mountPage();
    api.historyGroups.mockResolvedValue({ groups: [], total: 0 });
    radio("mh-view", "按条目");
    await vi.waitFor(() => expect(q("mh-groups-state")).not.toBeNull());
    expect(q("mh-groups-state")!.textContent).toContain("还没有整理记录");
  });
});

describe("整理历史：让 AI 认", () => {
  it("没识别出来的记录有「让 AI 认」：打开整理弹窗，预览以后认一次", async () => {
    await mountPage();
    expect(q("mh-ai-1")).toBeNull();
    expect(q("mh-ai-4"), "别的分类没有").toBeNull();
    api.preview.mockResolvedValue({
      downloader_id: 1,
      downloader_name: "qb",
      hash: "abc",
      task_id: "abc",
      name: "Unknown.2020",
      save_path: "/downloads",
      local_path: "/data/downloads",
      mapped: true,
      items: [],
      problem: "没有识别出来：TMDB 上没有可靠的匹配",
      fail_kind: "recognize",
    });
    api.libraries.mockResolvedValue([]);
    api.aiRecognize.mockResolvedValue({
      input: { title: "Unknown.2020" },
      candidates: [],
      chosen: -1,
      reason: "TMDB 上没有搜到候选",
    });
    q("mh-ai-2")!.click();
    await vi.waitFor(() =>
      expect(api.aiRecognize).toHaveBeenCalledWith({ downloader_id: 1, hash: "abc" }),
    );
    expect(api.preview).toHaveBeenCalledWith({ downloader_id: 1, hash: "abc" });
    await vi.waitFor(() => expect(q("organize-ai-result")).not.toBeNull());
  });
});

describe("整理历史：导入的已有文件", () => {
  const imported = row({
    id: 7,
    mode: "existing",
    trigger: "import",
    downloader_id: 0,
    downloader_name: "导入",
    info_hash: "import:0123456789abcdef",
    source_path: "/media/tv/最后生还者 (2023)/Season 1/S01E01.mkv",
    target_path: "/media/tv/最后生还者 (2023)/Season 1/S01E01.mkv",
  });

  it("写明方式是已有文件、触发方式是导入", async () => {
    await mountPage(false, [imported]);
    expect(document.body.textContent).toContain("已有文件");
    expect(document.body.textContent).toContain("导入已有文件");
    expect(document.body.textContent, "就在库里：写位置，不写箭头").toContain(
      "位置：/media/tv/最后生还者 (2023)/Season 1/S01E01.mkv",
    );
    expect(document.body.textContent).not.toContain("→ /media/tv");
  });

  it("批量连文件删除：选中的里有导入的已有文件时再确认一次，确认以后才删它们的文件", async () => {
    await mountPage(false, [imported, row({})]);
    await tick(0);
    await tick(1);
    ui.confirm.mockResolvedValueOnce("confirm").mockRejectedValueOnce("cancel");
    q("mh-batch-delete-files")!.click();
    await vi.waitFor(() => expect(ui.confirm).toHaveBeenCalledTimes(2));
    expect(ui.confirm.mock.calls[1]![0]).toContain("其中 1 条是导入时就在库里的文件");
    await flush();
    expect(api.historyBatch, "第二次取消：不删").not.toHaveBeenCalled();

    ui.confirm.mockResolvedValue("confirm");
    api.historyBatch.mockResolvedValue({
      items: [
        { id: 7, ok: true },
        { id: 1, ok: true },
      ],
      ok: 2,
      failed: 0,
    });
    q("mh-batch-delete-files")!.click();
    await vi.waitFor(() =>
      expect(api.historyBatch).toHaveBeenCalledWith([7, 1], "delete_files", true),
    );
  });

  it("单条删除：写明是导入的已有文件，勾上连文件删时再确认一次", async () => {
    await mountPage(false, [imported]);
    q("mh-del-7")!.click();
    await flush();
    expect(q("mh-del-existing")!.textContent).toContain("导入时就在库里的文件");
    expect(q("mh-del-text")!.textContent, "导入的文件就在库里：不写下载目录").not.toContain(
      "下载目录",
    );
    q("mh-del-files")!.querySelector("input")!.click();
    await flush();
    ui.confirm.mockRejectedValueOnce("cancel");
    q("mh-del-confirm")!.click();
    await flush();
    expect(api.deleteHistory).not.toHaveBeenCalled();

    ui.confirm.mockResolvedValueOnce("confirm");
    api.deleteHistory.mockResolvedValue({ ok: true });
    q("mh-del-confirm")!.click();
    await vi.waitFor(() => expect(api.deleteHistory).toHaveBeenCalledWith(7, true, true));
  });
});
