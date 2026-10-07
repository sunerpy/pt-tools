// @vitest-environment happy-dom
/*
 * 钉子：转移做种页。任务列表写明状态与进度，只有还没加入目标的任务能取消；
 * 路径映射按一对下载器整体保存；定时规则新建时默认关闭，开关、立即运行、删除都走接口。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  preview: vi.fn(),
  create: vi.fn(),
  jobs: vi.fn(),
  cancel: vi.fn(),
  clearFinished: vi.fn(),
  pathMaps: vi.fn(),
  savePathMaps: vi.fn(),
  rules: vi.fn(),
  createRule: vi.fn(),
  updateRule: vi.fn(),
  deleteRule: vi.fn(),
  runRule: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  message: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    transferApi: api,
    downloadersApi: {
      ...real.downloadersApi,
      list: vi.fn(() =>
        Promise.resolve([
          {
            id: 1,
            name: "qb-src",
            type: "qbittorrent",
            url: "",
            username: "",
            is_default: true,
            enabled: true,
          },
          {
            id: 2,
            name: "tr-nas",
            type: "transmission",
            url: "",
            username: "",
            is_default: false,
            enabled: true,
          },
          {
            id: 3,
            name: "off",
            type: "qbittorrent",
            url: "",
            username: "",
            is_default: false,
            enabled: false,
          },
        ]),
      ),
    },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(ui.message, {
      success: ui.success,
      error: ui.error,
      warning: ui.warning,
      info: vi.fn(),
    }),
    ElMessageBox: { ...real.ElMessageBox, confirm: ui.confirm },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));

function job(id: number, state: string, extra: Record<string, unknown> = {}) {
  return {
    id,
    source_downloader_id: 1,
    target_downloader_id: 2,
    source_name: "qb-src",
    target_name: "tr-nas",
    info_hash: `h${id}`,
    name: `Movie.${id}`,
    total_size: 1 << 30,
    site_name: "hdsky",
    source_save_path: "/downloads",
    target_save_path: "/data",
    state,
    message: "",
    progress: 0,
    final: ["source_removed", "rolled_back", "failed", "canceled"].includes(state),
    created_at: "2026-10-07T10:00:00Z",
    updated_at: "2026-10-07T10:00:00Z",
    ...extra,
  };
}

async function mountPage(
  jobs = [
    job(1, "pending"),
    job(2, "checking", { progress: 0.5 }),
    job(3, "rolled_back", { message: "校验只到 40.0%" }),
  ],
  waitText = "Movie.1",
) {
  api.jobs.mockResolvedValue({ items: jobs });
  api.pathMaps.mockResolvedValue({ items: [] });
  api.rules.mockResolvedValue({ items: [] });
  const Page = (await import("./TorrentTransfer.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(document.body.textContent).toContain(waitText));
  await flush();
}

function testid(id: string): HTMLElement {
  const el = document.querySelector<HTMLElement>(`[data-testid=${id}]`);
  if (!el) throw new Error(`找不到 ${id}`);
  return el;
}

async function chooseTab(label: string) {
  const item = [
    ...document.querySelectorAll<HTMLElement>("[data-testid=tf-tabs] .el-segmented__item"),
  ].find((el) => (el.textContent ?? "").trim() === label);
  if (!item) throw new Error(`找不到标签「${label}」`);
  item.click();
  await flush();
}

describe("转移做种", () => {
  it("任务列表：写明状态与校验进度，只有还没加入目标的能取消", async () => {
    await mountPage();
    const text = document.body.textContent ?? "";
    expect(text).toContain("等待导出");
    expect(text).toContain("校验中");
    expect(text).toContain("50.0%");
    expect(text).toContain("已回滚");
    expect(text).toContain("校验只到 40.0%");
    expect(text).toContain("qb-src → tr-nas");
    const cancels = [...document.querySelectorAll("button")].filter(
      (b) => b.textContent?.trim() === "取消",
    );
    expect(cancels).toHaveLength(1);
    api.cancel.mockResolvedValue({ success: true });
    cancels[0].click();
    await vi.waitFor(() => expect(api.cancel).toHaveBeenCalledWith(1));
  });

  it("清除已结束的任务要先确认", async () => {
    await mountPage();
    ui.confirm.mockResolvedValue("confirm");
    api.clearFinished.mockResolvedValue({ deleted: 1 });
    testid("tf-clear").click();
    await vi.waitFor(() => expect(api.clearFinished).toHaveBeenCalled());
    expect(ui.confirm.mock.calls[0][0]).toContain("1 个已结束");
  });

  it("没有任务时提示从哪里建", async () => {
    await mountPage([], "还没有转移任务");
    expect(document.body.textContent).toContain("「转移到…」");
  });

  it("路径映射：按选中的一对下载器读取，加一条后整体保存", async () => {
    await mountPage();
    await chooseTab("路径映射");
    await vi.waitFor(() => expect(api.pathMaps).toHaveBeenCalledWith(1, 2));
    expect(document.body.textContent).not.toContain("off");
    testid("tf-map-add").click();
    await flush();
    const inputs = [...document.querySelectorAll<HTMLInputElement>(".tf-map input")];
    expect(inputs).toHaveLength(2);
    inputs[0].value = "/downloads";
    inputs[0].dispatchEvent(new Event("input"));
    inputs[1].value = "/data";
    inputs[1].dispatchEvent(new Event("input"));
    await flush();
    api.savePathMaps.mockResolvedValue({
      items: [{ source_prefix: "/downloads", target_prefix: "/data" }],
    });
    testid("tf-map-save").click();
    await vi.waitFor(() =>
      expect(api.savePathMaps).toHaveBeenCalledWith(1, 2, [
        { source_prefix: "/downloads", target_prefix: "/data" },
      ]),
    );
  });

  it("定时规则：新建默认关闭，开关、立即运行、删除走接口", async () => {
    await mountPage();
    api.rules.mockResolvedValue({
      items: [
        {
          id: 7,
          name: "归档",
          enabled: false,
          source_downloader_id: 1,
          target_downloader_id: 2,
          source_name: "qb-src",
          target_name: "tr-nas",
          category: "",
          tag: "done",
          site_name: "",
          min_seeding_hours: 720,
          max_per_run: 10,
          interval_min: 60,
          last_result: "",
        },
      ],
    });
    await chooseTab("定时规则");
    await vi.waitFor(() => expect(document.body.textContent).toContain("归档"));
    expect(document.body.textContent).toContain("标签含 done、做种满 720 小时");

    testid("tf-rule-new").click();
    await flush();
    const name = testid("tf-rule-name");
    const input = (
      name instanceof HTMLInputElement ? name : name.querySelector("input")
    ) as HTMLInputElement;
    input.value = "新规则";
    input.dispatchEvent(new Event("input"));
    await flush();
    api.createRule.mockResolvedValue({});
    testid("tf-rule-save").click();
    await vi.waitFor(() => expect(api.createRule).toHaveBeenCalled());
    const created = api.createRule.mock.calls[0][0];
    expect(created).toMatchObject({
      name: "新规则",
      enabled: false,
      source_downloader_id: 1,
      target_downloader_id: 2,
      max_per_run: 10,
      interval_min: 60,
    });

    api.updateRule.mockResolvedValue({});
    document.querySelector<HTMLElement>("[data-testid=tf-rules] .el-switch")!.click();
    await vi.waitFor(() =>
      expect(api.updateRule).toHaveBeenCalledWith(7, expect.objectContaining({ enabled: true })),
    );

    api.runRule.mockResolvedValue({
      result: { matched: 2, created: 1 },
      summary: "符合条件 2 个，建了 1 个任务",
    });
    buttonByText("立即运行").click();
    await vi.waitFor(() => expect(api.runRule).toHaveBeenCalledWith(7));
    await vi.waitFor(() =>
      expect(ui.message).toHaveBeenCalledWith(
        expect.objectContaining({ message: "符合条件 2 个，建了 1 个任务" }),
      ),
    );

    ui.confirm.mockResolvedValue("confirm");
    api.deleteRule.mockResolvedValue({ success: true });
    buttonByText("删除").click();
    await vi.waitFor(() => expect(api.deleteRule).toHaveBeenCalledWith(7));
  });
});
