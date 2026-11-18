// @vitest-environment happy-dom
/*
 * 下载器页的「保留」：批量条与右键菜单加、去 pt-tools-keep；带它的行在标题前标「保留」。
 * 下载器能力里没有 can_keep（老后端）时按钮不可用；部分失败要把原因写出来。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  batchAction: vi.fn(),
  canKeep: true,
}));

const ui = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
}));

function row(id: string, title: string, tags = "") {
  return {
    downloader_id: 1,
    downloader_name: "qb-家里",
    downloader_type: "qbittorrent",
    task_id: id,
    title,
    info_hash: id,
    state: "seeding",
    progress: 100,
    size: 1024 ** 3,
    upload_speed: 0,
    download_speed: 0,
    seeds: 3,
    connections: 1,
    ratio: 1.2,
    added_at: 1_700_000_000,
    completed_at: -1,
    save_path: "/downloads",
    category: "",
    tags,
    eta: 0,
  };
}

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderTorrentsApi: {
      list: api.list,
      batchAction: api.batchAction,
      capabilities: vi.fn(() =>
        Promise.resolve({
          items: [
            {
              downloader_id: 1,
              downloader_name: "qb-家里",
              supports_categories: true,
              supports_tags: true,
              can_pause: true,
              can_resume: true,
              can_delete: true,
              can_delete_with_data: true,
              can_set_location: true,
              can_recheck: true,
              can_add_torrent: true,
              ...(api.canKeep ? { can_keep: true } : {}),
              categories: [],
              tags: [],
            },
          ],
        }),
      ),
      meta: vi.fn(() => Promise.resolve({ downloaders: [], categories: [], tags: [] })),
      transferStats: vi.fn(() =>
        Promise.resolve({
          total_upload_speed: 0,
          total_download_speed: 0,
          total_uploaded: 0,
          total_downloaded: 0,
          total_session_uploaded: 0,
          total_session_downloaded: 0,
          total_free_space: 0,
          downloaders: [],
        }),
      ),
      detail: vi.fn(),
      add: vi.fn(),
    },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessageBox: { confirm: vi.fn() },
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: vi.fn(),
      warning: ui.warning,
      info: vi.fn(),
    }),
  };
});

let view: MountedView | null = null;

async function mountHub() {
  localStorage.clear();
  api.list.mockResolvedValue({
    items: [row("a1", "任务甲"), row("b2", "任务乙", "hdsky,pt-tools-keep")],
    total: 2,
    failures: [],
  });
  api.batchAction.mockResolvedValue({ success_count: 2, failed_count: 0, results: [] });
  const DownloaderHub = (await import("./DownloaderHub.vue")).default;
  view = mountView(DownloaderHub);
  await vi.waitFor(() => expect(document.querySelectorAll("tr.torrent-row")).toHaveLength(2));
}

async function selectAll() {
  document.querySelector<HTMLInputElement>(".el-table__header .el-checkbox__original")!.click();
  await vi.waitFor(() => expect(document.body.textContent).toContain("已选 2 项"));
}

async function openMenu(index: number) {
  const tr = document.querySelectorAll("tr.torrent-row")[index]!;
  tr.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, clientX: 20, clientY: 20 }));
  await vi.waitFor(() => expect(document.querySelector(".table-context-menu")).not.toBeNull());
  return document.querySelector(".table-context-menu")!;
}

afterEach(() => {
  view?.unmount();
  view = null;
  api.canKeep = true;
  vi.clearAllMocks();
});

describe("下载器 Web UI：保留", () => {
  it("带 pt-tools-keep 的行在标题前标「保留」，悬停写明哪些自动功能跳过它", async () => {
    await mountHub();
    const marks = document.querySelectorAll('[data-testid="row-keep-mark"]');
    expect(marks).toHaveLength(1);
    expect(marks[0]!.closest("tr")!.textContent).toContain("任务乙");
    expect(marks[0]!.getAttribute("title")).toContain("做种竞争度");
    expect(marks[0]!.getAttribute("title")).toContain("刷流");
  });

  it("选中的不全是保留着的：按钮是「保留」，发 keep，目标是选中的两条；成功时报个数", async () => {
    await mountHub();
    await selectAll();
    buttonByText("保留").click();
    await vi.waitFor(() => expect(api.batchAction).toHaveBeenCalledTimes(1));
    const payload = api.batchAction.mock.calls[0]![0];
    expect(payload.action).toBe("keep");
    expect(payload.targets).toHaveLength(2);
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("保留：成功 2 个"));
  });

  it("选中的都保留着时按钮是「取消保留」，发 unkeep；部分失败时把第一条原因写出来", async () => {
    await mountHub();
    const kept = [...document.querySelectorAll("tr.torrent-row")].find((r) =>
      r.textContent?.includes("任务乙"),
    )!;
    kept.querySelector<HTMLInputElement>(".el-checkbox__original")!.click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("已选 1 项"));
    api.batchAction.mockResolvedValue({
      success_count: 0,
      failed_count: 1,
      results: [
        {
          downloader_id: 1,
          downloader_name: "qb-家里",
          task_id: "b2",
          success: false,
          message: "种子不在下载器里了",
        },
      ],
    });
    buttonByText("取消保留").click();
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalledTimes(1));
    const payload = api.batchAction.mock.calls[0]![0];
    expect(payload).toEqual({ action: "unkeep", targets: [{ downloader_id: 1, task_id: "b2" }] });
    expect(ui.warning.mock.calls[0]![0]).toBe("取消保留：成功 0，失败 1（种子不在下载器里了）");
  });

  it("右键菜单：没保留的行给「保留」，保留着的给「取消保留」", async () => {
    await mountHub();
    let menu = await openMenu(0);
    expect(menu.textContent).toContain("保留");
    expect(menu.textContent).not.toContain("取消保留");
    buttonByText("保留", menu).click();
    await vi.waitFor(() => expect(api.batchAction).toHaveBeenCalledTimes(1));
    expect(api.batchAction.mock.calls[0]![0]).toEqual({
      action: "keep",
      targets: [{ downloader_id: 1, task_id: "a1" }],
    });

    menu = await openMenu(1);
    buttonByText("取消保留", menu).click();
    await vi.waitFor(() => expect(api.batchAction).toHaveBeenCalledTimes(2));
    expect(api.batchAction.mock.calls[1]![0].action).toBe("unkeep");
  });

  it("下载器能力里没有 can_keep（老后端）时，批量条的保留按钮不可用", async () => {
    api.canKeep = false;
    await mountHub();
    await selectAll();
    expect(buttonByText("保留").disabled).toBe(true);
  });
});
