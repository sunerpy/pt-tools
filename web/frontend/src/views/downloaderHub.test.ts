// @vitest-environment happy-dom
/*
 * 钉子：下载器 Web UI 的「删除」与「删除+文件」必须先确认，取消就一个请求都不发。
 *
 * 删除直接落到下载器上，没有回收站；「删除+文件」连磁盘上已下载的数据一起删，不可恢复。
 * 原来批量条与右键菜单（两种表格都走 handleContextAction）点下去就直接发 batch-action，
 * 一次误点就是整批数据没了。确认框要写清条数，删数据那一档用 error 类型和明确的按钮文案。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  batchAction: vi.fn(),
}));

const ui = vi.hoisted(() => ({
  confirm: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));

function row(id: string, title: string) {
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
    tags: "",
    eta: 0,
  };
}

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  const cap = {
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
    categories: [],
    tags: [],
  };
  return {
    ...real,
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderTorrentsApi: {
      list: api.list,
      batchAction: api.batchAction,
      capabilities: vi.fn(() => Promise.resolve({ items: [cap] })),
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
    ElMessageBox: { confirm: ui.confirm },
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      warning: vi.fn(),
      info: vi.fn(),
    }),
  };
});

let view: MountedView | null = null;

async function mountHub() {
  localStorage.clear();
  api.list.mockResolvedValue({
    items: [row("a1", "任务甲"), row("b2", "任务乙")],
    total: 2,
    failures: [],
  });
  api.batchAction.mockResolvedValue({ success_count: 2, failed_count: 0 });
  const DownloaderHub = (await import("./DownloaderHub.vue")).default;
  view = mountView(DownloaderHub);
  await vi.waitFor(() => expect(document.querySelectorAll("tr.torrent-row")).toHaveLength(2));
}

/** 表头那枚全选勾选框：两行一起选上，批量条才会出现 */
async function selectAll() {
  const all = document.querySelector<HTMLInputElement>(".el-table__header .el-checkbox__original")!;
  all.click();
  await vi.waitFor(() => expect(document.body.textContent).toContain("已选 2 项"));
}

/** 右键第一行，点菜单里的某一项 */
async function contextMenu(label: string) {
  const tr = document.querySelector("tr.torrent-row")!;
  tr.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, clientX: 20, clientY: 20 }));
  await vi.waitFor(() => expect(document.querySelector(".table-context-menu")).not.toBeNull());
  buttonByText(label, document.querySelector(".table-context-menu")!).click();
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("下载器 Web UI：删除先确认", () => {
  it("批量「删除+文件」：先弹 error 类型的确认框写明条数，取消就不发请求", async () => {
    await mountHub();
    await selectAll();
    ui.confirm.mockRejectedValue("cancel");

    buttonByText("删除+文件").click();
    await vi.waitFor(() => expect(ui.confirm).toHaveBeenCalledTimes(1));
    const [message, title, opts] = ui.confirm.mock.calls[0]!;
    expect(String(message)).toContain("2 个任务");
    expect(String(message)).toContain("文件");
    expect(title).toBeTruthy();
    expect(opts.type).toBe("error");
    expect(opts.confirmButtonText).toContain("删除");
    expect(opts.confirmButtonText).not.toBe("确定");

    await new Promise((r) => setTimeout(r, 20));
    expect(api.batchAction).not.toHaveBeenCalled();
  });

  it("批量「删除」：确认之后才发 delete，目标是选中的两条", async () => {
    await mountHub();
    await selectAll();
    ui.confirm.mockResolvedValue("confirm");

    buttonByText("删除").click();
    await vi.waitFor(() => expect(api.batchAction).toHaveBeenCalledTimes(1));
    expect(ui.confirm).toHaveBeenCalledTimes(1);
    expect(String(ui.confirm.mock.calls[0]![0])).toContain("2 个任务");
    const payload = api.batchAction.mock.calls[0]![0];
    expect(payload.action).toBe("delete");
    expect(payload.targets).toHaveLength(2);
  });

  it("右键「删除任务和文件」：同样先确认，关掉对话框就不发请求", async () => {
    await mountHub();
    ui.confirm.mockRejectedValue("close");

    await contextMenu("删除任务和文件");
    await vi.waitFor(() => expect(ui.confirm).toHaveBeenCalledTimes(1));
    const [message, , opts] = ui.confirm.mock.calls[0]!;
    expect(String(message)).toContain("任务甲");
    expect(opts.type).toBe("error");
    await new Promise((r) => setTimeout(r, 20));
    expect(api.batchAction).not.toHaveBeenCalled();
  });

  it("右键「删除任务」：确认之后只删这一条", async () => {
    await mountHub();
    ui.confirm.mockResolvedValue("confirm");

    await contextMenu("删除任务");
    await vi.waitFor(() => expect(api.batchAction).toHaveBeenCalledTimes(1));
    expect(ui.confirm).toHaveBeenCalledTimes(1);
    expect(String(ui.confirm.mock.calls[0]![0])).toContain("1 个任务");
    const payload = api.batchAction.mock.calls[0]![0];
    expect(payload.action).toBe("delete");
    expect(payload.targets).toEqual([{ downloader_id: 1, task_id: "a1" }]);
  });

  it("暂停、复检这类非删除动作不弹确认", async () => {
    await mountHub();
    await contextMenu("暂停");
    await vi.waitFor(() => expect(api.batchAction).toHaveBeenCalledTimes(1));
    expect(ui.confirm).not.toHaveBeenCalled();
    expect(api.batchAction.mock.calls[0]![0].action).toBe("pause");
  });
});

/*
 * qB 对未完成的任务回 completion_on = -1（这里的假数据就是这样铺的），表格按秒乘 1000 传给
 * formatShortDateTime。只把 0 当零值时，「完成日期」一格印的是「1970-01-01 07:59」。
 */
describe("下载器 Web UI：未完成任务的完成日期", () => {
  it("completed_at = -1 显示「-」，不显示 1970；添加日期照常显示", async () => {
    await mountHub();
    const body = document.querySelector(".el-table__body")!;
    expect(body.textContent).not.toContain("1970");
    expect(body.textContent).not.toContain("1969");
    /* 按表头文字找列：列顺序可以由用户拖动，不能写死第几格 */
    const heads = [...document.querySelectorAll<HTMLElement>(".el-table__header th")].map(
      (th) => th.textContent?.trim() ?? "",
    );
    const cell = (label: string) => {
      const tds = document.querySelectorAll<HTMLElement>("tr.torrent-row")[0]!.children;
      return (tds[heads.indexOf(label)] as HTMLElement | undefined)?.textContent?.trim();
    };
    expect(cell("添加日期")).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/);
    expect(cell("完成日期")).toBe("-");
  });
});
