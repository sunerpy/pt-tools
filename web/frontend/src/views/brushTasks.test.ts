// @vitest-environment happy-dom
/*
 * 钉子：刷流任务页。列表按真实 DTO 画；新建时优惠类型合成逗号串、默认关闭；开关只翻 enabled；
 * 「立即运行」把结果说清楚；种子抽屉按标签取对应状态的种子，晚到的旧请求不覆盖新标签。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BrushTask } from "@/api";
import { type MountedView, mountView } from "@/test-mount";
import { defaultBrushConfig } from "@/utils/brush";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  run: vi.fn(),
  torrents: vi.fn(),
  stats: vi.fn(),
}));
const ui = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    brushApi: api,
    sitesApi: {
      ...real.sitesApi,
      list: vi.fn(() =>
        Promise.resolve({
          hdsky: { enabled: true, is_builtin: true },
          ourbits: { enabled: false, is_builtin: true },
        }),
      ),
    },
    downloadersApi: {
      ...real.downloadersApi,
      list: vi.fn(() =>
        Promise.resolve([
          {
            id: 2,
            name: "qb-main",
            type: "qbittorrent",
            url: "",
            username: "",
            is_default: true,
            enabled: true,
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
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      warning: ui.warning,
      info: vi.fn(),
    }),
  };
});

function task(over: Partial<BrushTask> = {}): BrushTask {
  return {
    ...defaultBrushConfig(),
    id: 1,
    name: "HDSky 刷流",
    site_name: "hdsky",
    downloader_id: 2,
    enabled: true,
    last_error: "",
    last_result: "列表 40 个、符合条件 3 个、加入 2 个",
    last_run_at: "2026-10-06T10:00:00Z",
    created_at: "2026-10-06T09:00:00Z",
    updated_at: "2026-10-06T09:00:00Z",
    downloader_name: "qb-main",
    site_enabled: true,
    active_count: 3,
    downloading_count: 1,
    active_size_bytes: 3 * 1024 ** 3,
    today: { uploaded: 5 * 1024 ** 3, downloaded: 1024 ** 3, added: 2, removed: 1 },
    total: { uploaded: 50 * 1024 ** 3, downloaded: 10 * 1024 ** 3, added: 9, removed: 6 },
    ...over,
  };
}

const STATS = {
  days: 30,
  from: "2026-09-07",
  to: "2026-10-06",
  dates: [],
  tasks: [],
  totals: Array.from({ length: 30 }, (_, i) => ({
    date: `d${i}`,
    uploaded: i === 29 ? 5 * 1024 ** 3 : 0,
    downloaded: 0,
    added: i === 29 ? 2 : 0,
    removed: 0,
  })),
};

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

async function mountPage(tasks: BrushTask[]) {
  api.list.mockResolvedValue(tasks);
  api.stats.mockResolvedValue(STATS);
  const Page = (await import("./BrushTasks.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.list).toHaveBeenCalled());
}

function button(text: string): HTMLButtonElement {
  const hit = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => (b.textContent ?? "").trim() === text && !b.disabled,
  );
  if (!hit) throw new Error(`找不到按钮「${text}」`);
  return hit;
}

const flush = () => new Promise((r) => setTimeout(r, 0));

describe("刷流任务页", () => {
  it("列表按 DTO 画出站点、在做数、今日收益与上次运行结果；KPI 与 30 天收益来自统计接口", async () => {
    await mountPage([task()]);
    await vi.waitFor(() => expect(document.body.textContent).toContain("HDSky 刷流"));
    const text = document.body.textContent ?? "";
    expect(text).toContain("1 / 3");
    expect(text).toContain("5 GB / 1 GB");
    expect(text).toContain("列表 40 个、符合条件 3 个、加入 2 个");
    expect(text).toContain("今日上传");
    expect(document.querySelector("[data-testid=brush-trend] .pt-bars")).not.toBeNull();
  });

  it("没有任务时给出新建入口", async () => {
    await mountPage([]);
    await vi.waitFor(() => expect(document.body.textContent).toContain("任务默认关闭"));
  });

  it("新建：默认关闭，优惠类型合成逗号串，站点与下载器预选", async () => {
    await mountPage([]);
    api.create.mockResolvedValue(task({ id: 9, enabled: false }));
    // 站点与下载器和任务表一起取，等页面画完再点，预选才有值
    await vi.waitFor(() => expect(document.body.textContent).toContain("任务默认关闭"));
    await flush();
    button("新建任务").click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-save]")).not.toBeNull(),
    );
    document.querySelector<HTMLButtonElement>("[data-testid=brush-save]")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    const body = api.create.mock.calls[0][0];
    expect(body.enabled).toBe(false);
    expect(body.site_name).toBe("hdsky");
    expect(body.downloader_id).toBe(2);
    expect(body.discounts).toBe("FREE,2XFREE");
    expect(body.name).toBe("hdsky 刷流");
    expect(body).not.toHaveProperty("id");
  });

  it("开关只翻 enabled，其余配置原样带回", async () => {
    const t = task({ remove_ratio: 3 });
    await mountPage([t]);
    api.update.mockImplementation((_id: number, body: unknown) =>
      Promise.resolve({ ...t, ...(body as object) }),
    );
    await vi.waitFor(() => expect(document.querySelector(".el-switch")).not.toBeNull());
    document.querySelector<HTMLElement>(".el-switch")!.click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    const [id, body] = api.update.mock.calls[0];
    expect(id).toBe(1);
    expect(body.enabled).toBe(false);
    expect(body.remove_ratio).toBe(3);
    await vi.waitFor(() =>
      expect(ui.success).toHaveBeenCalledWith(
        expect.stringContaining("已加入的种子继续按规则删除"),
      ),
    );
  });

  it("立即运行：成功、带错误、失败三种提示", async () => {
    await mountPage([task()]);
    await vi.waitFor(() =>
      expect(document.querySelector("[aria-label='立即运行 HDSky 刷流']")).not.toBeNull(),
    );
    const runBtn = () =>
      document.querySelector<HTMLButtonElement>("[aria-label='立即运行 HDSky 刷流']")!;

    api.run.mockResolvedValueOnce({
      result: { task_id: 1, sampled: 3, removed: 0, gone: 0, listed: 10, eligible: 2, added: 1 },
    });
    runBtn().click();
    await vi.waitFor(() =>
      expect(ui.success).toHaveBeenCalledWith(expect.stringContaining("加入 1 个")),
    );

    api.run.mockRejectedValueOnce(new Error("读取 hdsky 的种子列表失败: 站点 502"));
    await flush();
    runBtn().click();
    await vi.waitFor(() =>
      expect(ui.error).toHaveBeenCalledWith(expect.stringContaining("站点 502")),
    );
  });

  it("种子抽屉按标签取数据；切标签时旧请求晚到不覆盖", async () => {
    await mountPage([task()]);
    let releaseActive!: (v: unknown) => void;
    api.torrents.mockImplementation((_id: number, state: string) =>
      state === "active"
        ? new Promise((resolve) => {
            releaseActive = resolve;
          })
        : Promise.resolve({
            items: [
              {
                id: 5,
                title: "Removed.One",
                state: "removed",
                remove_reason: "分享率 3.10，达到 3.00",
                size_bytes: 1,
                discount: "FREE",
                progress: 1,
                ratio: 3.1,
                uploaded: 1,
                added_at: "2026-10-06T08:00:00Z",
                removed_at: "2026-10-06T09:00:00Z",
                has_hr: false,
              },
            ],
            total: 1,
          }),
    );
    await vi.waitFor(() =>
      expect(document.querySelector("[aria-label='HDSky 刷流 的种子']")).not.toBeNull(),
    );
    document.querySelector<HTMLButtonElement>("[aria-label='HDSky 刷流 的种子']")!.click();
    await vi.waitFor(() => expect(api.torrents).toHaveBeenCalledWith(1, "active", 1, 100));

    const tab = [...document.querySelectorAll<HTMLElement>(".el-segmented__item")].find((e) =>
      e.textContent?.includes("已删除"),
    );
    tab!.click();
    await vi.waitFor(() => expect(api.torrents).toHaveBeenCalledWith(1, "removed", 1, 100));
    await vi.waitFor(() => expect(document.body.textContent).toContain("分享率 3.10，达到 3.00"));

    releaseActive({
      items: [
        {
          id: 6,
          title: "Stale.Active",
          state: "active",
          size_bytes: 1,
          discount: "FREE",
          progress: 0.5,
          ratio: 0,
          uploaded: 0,
          added_at: "2026-10-06T08:00:00Z",
          remove_reason: "",
          has_hr: false,
        },
      ],
      total: 1,
    });
    await flush();
    expect(document.body.textContent).not.toContain("Stale.Active");
  });

  it("种子抽屉：旧请求晚到的失败也不覆盖新标签的列表与 loading", async () => {
    await mountPage([task()]);
    let failActive!: (e: unknown) => void;
    api.torrents.mockImplementation((_id: number, state: string) =>
      state === "active"
        ? new Promise((_resolve, reject) => {
            failActive = reject;
          })
        : Promise.resolve({
            items: [
              {
                id: 7,
                title: "Gone.One",
                state: "gone",
                remove_reason: "下载器里已经没有这个种子",
                size_bytes: 1,
                discount: "FREE",
                progress: 1,
                ratio: 1,
                uploaded: 1,
                added_at: "2026-10-06T08:00:00Z",
                removed_at: "2026-10-06T09:00:00Z",
                has_hr: false,
              },
            ],
            total: 1,
          }),
    );
    await vi.waitFor(() =>
      expect(document.querySelector("[aria-label='HDSky 刷流 的种子']")).not.toBeNull(),
    );
    document.querySelector<HTMLButtonElement>("[aria-label='HDSky 刷流 的种子']")!.click();
    await vi.waitFor(() => expect(api.torrents).toHaveBeenCalledWith(1, "active", 1, 100));
    const tab = [...document.querySelectorAll<HTMLElement>(".el-segmented__item")].find((e) =>
      e.textContent?.includes("已不在下载器"),
    );
    tab!.click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("Gone.One"));

    failActive(new Error("旧请求失败"));
    await flush();
    expect(document.body.textContent).toContain("Gone.One");
    expect(document.body.textContent).not.toContain("旧请求失败");
  });
});
