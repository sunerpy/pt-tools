// @vitest-environment happy-dom
/*
 * 钉子：刷流任务页。列表按真实 DTO 画；新建时优惠类型合成逗号串、默认关闭；开关只翻 enabled；
 * 「立即运行」把结果说清楚；种子抽屉按标签取对应状态的种子，晚到的旧请求不覆盖新标签；
 * 运行记录抽屉列出每一轮（最新一条展开、可以接着往下读），「上次运行」那格点了也打开它。
 * 收益统计按日、周、月、季、年切换（2026-10-28 用户要的），KPI、任务表的「本期」列与收益图一起换。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BrushRun, BrushTask } from "@/api";
import { type MountedView, mountView } from "@/test-mount";
import { defaultBrushConfig } from "@/utils/brush";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  run: vi.fn(),
  runs: vi.fn(),
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
          mteam: { enabled: true, is_builtin: true },
          ourbits: { enabled: false, is_builtin: true },
        }),
      ),
    },
    downloaderDirectoriesApi: {
      ...real.downloaderDirectoriesApi,
      listAll: vi.fn(() =>
        Promise.resolve({
          2: [
            { id: 1, downloader_id: 2, path: "/downloads", alias: "", is_default: true },
            { id: 2, downloader_id: 2, path: "/downloads/brush", alias: "刷流", is_default: false },
          ],
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

function brushRun(over: Partial<BrushRun> = {}): BrushRun {
  return {
    id: 1,
    task_id: 1,
    trigger: "schedule",
    started_at: "2026-10-06T10:00:00Z",
    finished_at: "2026-10-06T10:00:08Z",
    status: "ok",
    summary: "列表 40 个、符合条件 3 个、加入 2 个",
    error: "",
    listed: 40,
    eligible: 3,
    added: 2,
    removed: 0,
    gone: 0,
    steps: [],
    steps_truncated: false,
    ...over,
  };
}

const GB = 1024 ** 3;

/** 按日 30 段：只有最后一天（今天）有 5 GB 上传 */
const STATS = {
  granularity: "day",
  periods: 30,
  days: 30,
  from: "2026-09-07",
  to: "2026-10-06",
  dates: [],
  tasks: [],
  totals: Array.from({ length: 30 }, (_, i) => {
    const d = new Date(2026, 8, 7 + i);
    const day = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
    return {
      date: day,
      end: day,
      uploaded: i === 29 ? 5 * GB : 0,
      downloaded: 0,
      added: i === 29 ? 2 : 0,
      removed: 0,
    };
  }),
};

/** 按周 12 段：上周 20 GB、本周 30 GB / 4 GB；任务 1 本周 7 GB / 2 GB */
function weekStats() {
  const mondays = Array.from({ length: 12 }, (_, i) => new Date(2026, 7, 17 + i * 7));
  const ymd = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  const point = (d: Date, i: number, up: number, down: number) => ({
    date: ymd(d),
    end: ymd(new Date(d.getFullYear(), d.getMonth(), d.getDate() + 6)),
    uploaded: up,
    downloaded: down,
    added: i,
    removed: 0,
  });
  return {
    granularity: "week",
    periods: 12,
    days: 80,
    from: ymd(mondays[0]!),
    to: "2026-11-04",
    dates: mondays.map(ymd),
    tasks: [
      {
        task_id: 1,
        name: "HDSky 刷流",
        series: mondays.map((d, i) => point(d, 0, i === 11 ? 7 * GB : 0, i === 11 ? 2 * GB : 0)),
      },
    ],
    totals: mondays.map((d, i) =>
      point(d, i, i === 11 ? 30 * GB : i === 10 ? 20 * GB : 0, i === 11 ? 4 * GB : 0),
    ),
  };
}

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
  localStorage.clear();
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

/** 打开刷流任务对话框里的站点下拉框，给出它自己的选项文字 */
async function siteOptions(): Promise<string[]> {
  const sel = document.querySelector<HTMLElement>("[data-testid=brush-site]")!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const listId = sel.querySelector("input")?.getAttribute("aria-controls");
  const read = () => [
    ...(document.getElementById(listId ?? "")?.querySelectorAll(".el-select-dropdown__item") ?? []),
  ];
  await vi.waitFor(() => expect(read().length).toBeGreaterThan(0));
  return read().map((o) => (o.textContent ?? "").trim());
}

describe("刷流任务页", () => {
  it("列表按 DTO 画出站点、在做数、今日收益与上次运行结果；KPI 与 30 天收益来自统计接口", async () => {
    await mountPage([task()]);
    await vi.waitFor(() => expect(document.body.textContent).toContain("HDSky 刷流"));
    expect(api.stats).toHaveBeenCalledWith("day", 30);
    const text = document.body.textContent ?? "";
    expect(text).toContain("1 / 3");
    expect(text).toContain("5 GB / 1 GB");
    expect(text).toContain("列表 40 个、符合条件 3 个、加入 2 个");
    expect(text).toContain("今日上传");
    expect(text).toContain("最近 30 天收益");
    // 读数默认是当前这一段（今天）
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-trend-readout]")?.textContent).toContain(
        "上传 5 GB",
      ),
    );
    expect(document.querySelector("[data-testid=brush-trend-readout]")?.textContent).toContain(
      "10月6日（今日）",
    );
    expect(document.querySelectorAll("[data-testid=brush-trend] .btc__col")).toHaveLength(30);
  });

  /** 点统计周期里的一档 */
  async function pickPeriod(label: string) {
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-period]")).not.toBeNull(),
    );
    const item = [
      ...document.querySelectorAll<HTMLElement>("[data-testid=brush-period] .el-segmented__item"),
    ].find((e) => (e.textContent ?? "").trim() === label);
    if (!item) throw new Error(`找不到周期「${label}」`);
    item.click();
  }

  it("换成按周：重新读统计，KPI、任务表的本期列与收益图一起换，记住选的周期", async () => {
    await mountPage([task()]);
    await vi.waitFor(() => expect(document.body.textContent).toContain("最近 30 天收益"));
    api.stats.mockResolvedValue(weekStats());
    await pickPeriod("周");
    await vi.waitFor(() => expect(api.stats).toHaveBeenLastCalledWith("week", 12));
    await vi.waitFor(() => expect(document.body.textContent).toContain("最近 12 周收益"));
    const text = document.body.textContent ?? "";
    expect(text).toContain("本周上传");
    expect(text).toContain("30 GB");
    expect(text).toContain("上周 20 GB");
    expect(text).toContain("本周 上传 / 下载");
    // 任务这一列是它本周的收益，不再是今天的
    expect(text).toContain("7 GB / 2 GB");
    expect(text).not.toContain("5 GB / 1 GB");
    expect(text).toContain("周从周一开始");
    expect(document.querySelector("[data-testid=brush-trend-readout]")?.textContent).toContain(
      "11月2日–11月8日（本周）",
    );
    expect(localStorage.getItem("pt-brush-period")).toBe("week");

    // 重新打开这一页还是按周
    view?.unmount();
    api.stats.mockClear();
    api.stats.mockResolvedValue(weekStats());
    const Page = (await import("./BrushTasks.vue")).default;
    view = mountView(Page);
    await vi.waitFor(() => expect(api.stats).toHaveBeenCalledWith("week", 12));
  });

  it("新周期的统计还在路上：数字与叫法都还是上一份的，统计图标在转", async () => {
    await mountPage([task()]);
    await vi.waitFor(() => expect(document.body.textContent).toContain("最近 30 天收益"));
    let release!: (v: unknown) => void;
    api.stats.mockImplementation(
      () =>
        new Promise((resolve) => {
          release = resolve;
        }),
    );
    await pickPeriod("周");
    await vi.waitFor(() => expect(api.stats).toHaveBeenLastCalledWith("week", 12));
    await vi.waitFor(() => expect(document.querySelector(".brush-spin")).not.toBeNull());
    const text = document.body.textContent ?? "";
    expect(text).toContain("今日上传");
    expect(text).not.toContain("本周上传");
    expect(text).toContain("最近 30 天收益");
    release(weekStats());
    await vi.waitFor(() => expect(document.body.textContent).toContain("本周上传"));
    expect(document.querySelector(".brush-spin")).toBeNull();
  });

  it("换了周期统计没读到：本期数字写「—」，不拿今天的冒充本周", async () => {
    await mountPage([task()]);
    await vi.waitFor(() => expect(document.body.textContent).toContain("最近 30 天收益"));
    api.stats.mockRejectedValue(new Error("boom"));
    await pickPeriod("月");
    await vi.waitFor(() => expect(api.stats).toHaveBeenLastCalledWith("month", 12));
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("收益统计没有读到，点刷新可重试"),
    );
    const text = document.body.textContent ?? "";
    expect(text).toContain("本月 上传 / 下载");
    expect(text).not.toContain("5 GB / 1 GB");
    expect(text).toContain("站点、下载器或收益统计有没读到的");
  });

  it("快速连点两个周期：先发的请求晚到不覆盖后选的", async () => {
    await mountPage([task()]);
    await vi.waitFor(() => expect(document.body.textContent).toContain("最近 30 天收益"));
    let releaseWeek!: (v: unknown) => void;
    api.stats.mockImplementation((gran: string) =>
      gran === "week"
        ? new Promise((resolve) => {
            releaseWeek = resolve;
          })
        : Promise.resolve({
            ...weekStats(),
            granularity: "year",
            periods: 5,
            totals: weekStats()
              .totals.slice(-5)
              .map((p, i) => ({
                ...p,
                date: `${2022 + i}-01-01`,
                end: `${2022 + i}-12-31`,
              })),
          }),
    );
    await pickPeriod("周");
    await vi.waitFor(() => expect(api.stats).toHaveBeenLastCalledWith("week", 12));
    await pickPeriod("年");
    await vi.waitFor(() => expect(document.body.textContent).toContain("最近 5 年收益"));
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-trend-readout]")?.textContent).toContain(
        "2026年（今年）",
      ),
    );
    releaseWeek(weekStats());
    await flush();
    expect(document.body.textContent).toContain("最近 5 年收益");
    expect(document.querySelector("[data-testid=brush-trend-readout]")?.textContent).toContain(
      "2026年（今年）",
    );
    expect(document.querySelectorAll("[data-testid=brush-trend] .btc__col")).toHaveLength(5);
  });

  it("收益明细：表里每一段的数字都有，新的在前", async () => {
    await mountPage([task()]);
    api.stats.mockResolvedValue(weekStats());
    await pickPeriod("周");
    await vi.waitFor(() => expect(document.body.textContent).toContain("最近 12 周收益"));
    document.querySelector<HTMLButtonElement>("[data-testid=brush-trend-detail-toggle]")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-trend-table]")).not.toBeNull(),
    );
    const rows = [...document.querySelectorAll("[data-testid=brush-trend-table] tbody tr")].map(
      (r) => [...r.children].map((c) => (c.textContent ?? "").trim()),
    );
    expect(rows).toHaveLength(12);
    expect(rows[0]).toEqual(["11月2日–11月8日（本周）", "30 GB", "4 GB", "11", "0"]);
    expect(rows[1]).toEqual(["10月26日–11月1日", "20 GB", "0 B", "10", "0"]);
  });

  it("没有任务时给出新建入口", async () => {
    await mountPage([]);
    await vi.waitFor(() => expect(document.body.textContent).toContain("任务默认关闭"));
  });

  // 2026-10-26 用户：「刷流任务的站点下拉框中，不应展示未启用的站点」
  it("站点下拉框只列启用的站点", async () => {
    await mountPage([]);
    await vi.waitFor(() => expect(document.body.textContent).toContain("任务默认关闭"));
    await flush();
    button("新建任务").click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-site]")).not.toBeNull(),
    );
    expect(await siteOptions()).toEqual(["hdsky", "mteam"]);
  });

  it("编辑的任务的站点后来停用了：仍列出它并标未启用，别的未启用站点不列", async () => {
    await mountPage([task({ id: 4, name: "OB 刷流", site_name: "ourbits", site_enabled: false })]);
    await vi.waitFor(() =>
      expect(document.querySelector("[aria-label='编辑 OB 刷流']")).not.toBeNull(),
    );
    await flush();
    document.querySelector<HTMLButtonElement>("[aria-label='编辑 OB 刷流']")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-site]")).not.toBeNull(),
    );
    expect(await siteOptions()).toEqual(["hdsky", "mteam", "ourbits（未启用）"]);
  });

  // 2026-10-26 用户：「下载器设置了下载目录，就可以直接选择而不是必须要输入」
  it("保存路径：列出所选下载器预设的目录，也能直接输入", async () => {
    await mountPage([]);
    api.create.mockResolvedValue(task({ id: 9, enabled: false }));
    await vi.waitFor(() => expect(document.body.textContent).toContain("任务默认关闭"));
    await flush();
    button("新建任务").click();
    const field = () => {
      const el = document.querySelector<HTMLElement>("[data-testid=brush-save-path]")!;
      return (el instanceof HTMLInputElement ? el : el.querySelector("input")) as HTMLInputElement;
    };
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-save-path]")).not.toBeNull(),
    );
    field().focus();
    field().dispatchEvent(new Event("focus"));
    let items: HTMLElement[] = [];
    await vi.waitFor(() => {
      items = [...document.querySelectorAll<HTMLElement>(".pt-path-input__popper li")];
      expect(items).toHaveLength(2);
    });
    items.find((li) => (li.textContent ?? "").includes("刷流"))!.click();
    document.querySelector<HTMLButtonElement>("[data-testid=brush-save]")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0][0].save_path).toBe("/downloads/brush");
  });

  it("保存路径：列表里没有的路径直接输入", async () => {
    await mountPage([]);
    api.create.mockResolvedValue(task({ id: 9, enabled: false }));
    await vi.waitFor(() => expect(document.body.textContent).toContain("任务默认关闭"));
    await flush();
    button("新建任务").click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-save-path]")).not.toBeNull(),
    );
    const el = document.querySelector<HTMLElement>("[data-testid=brush-save-path]")!;
    const input = (
      el instanceof HTMLInputElement ? el : el.querySelector("input")
    ) as HTMLInputElement;
    input.value = "/mnt/brush-other";
    input.dispatchEvent(new Event("input"));
    await flush();
    document.querySelector<HTMLButtonElement>("[data-testid=brush-save]")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0][0].save_path).toBe("/mnt/brush-other");
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

  it("分区只对 M-Team：别的站点不显示、保存为空；M-Team 老任务的空值按综合区显示与保存", async () => {
    await mountPage([
      task(),
      task({ id: 2, name: "MT 成人区", site_name: "mteam", section: "adult" }),
      task({ id: 3, name: "MT 老任务", site_name: "mteam", section: "" }),
    ]);
    api.update.mockImplementation((_id: number, body: unknown) =>
      Promise.resolve({ ...task(), ...(body as object) }),
    );
    const save = async (name: string) => {
      await vi.waitFor(() =>
        expect(document.querySelector(`[aria-label='编辑 ${name}']`)).not.toBeNull(),
      );
      document.querySelector<HTMLButtonElement>(`[aria-label='编辑 ${name}']`)!.click();
      await vi.waitFor(() =>
        expect(document.querySelector("[data-testid=brush-save]")).not.toBeNull(),
      );
      await flush();
      const shown = document.querySelector("[data-testid=brush-section]") !== null;
      const text = document.querySelector("[data-testid=brush-section]")?.textContent ?? "";
      document.querySelector<HTMLButtonElement>("[data-testid=brush-save]")!.click();
      await vi.waitFor(() => expect(api.update).toHaveBeenCalled());
      const body = api.update.mock.calls.at(-1)![1];
      api.update.mockClear();
      // 对话框关上时内容还留在页面里：等保存走完（提示「已保存」）再开下一个
      await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("已保存"));
      ui.success.mockClear();
      return { shown, text, section: body.section };
    };

    expect(await save("HDSky 刷流")).toEqual({ shown: false, text: "", section: "" });
    const adult = await save("MT 成人区");
    expect(adult.shown).toBe(true);
    expect(adult.section).toBe("adult");
    const old = await save("MT 老任务");
    expect(old.text).toContain("综合区");
    expect(old.section).toBe("normal");
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

  it("运行记录：按钮打开抽屉，最新一条展开、错误与步骤都在；再看 N 条接着往下读", async () => {
    await mountPage([task()]);
    api.runs
      .mockResolvedValueOnce({
        items: [
          brushRun({
            id: 3,
            trigger: "manual",
            status: "failed",
            summary: "列表 0 个、符合条件 0 个、加入 0 个",
            error: "读取 hdsky 的种子列表失败: authentication failed",
            steps: [
              {
                kind: "info",
                text: "任务名下在做 1 个种子（下载中 0 个，共 4.00 GB），今天已加入 0.00 GB",
              },
              { kind: "error", text: "读取 hdsky 的种子列表失败: authentication failed" },
            ],
          }),
          brushRun({
            id: 2,
            status: "partial",
            steps: [{ kind: "add", text: "Demo.One（免费，4.00 GB）" }],
            steps_truncated: true,
          }),
        ],
        total: 3,
      })
      .mockResolvedValueOnce({ items: [brushRun({ id: 1 })], total: 3 });
    await vi.waitFor(() =>
      expect(document.querySelector("[aria-label='HDSky 刷流 的运行记录']")).not.toBeNull(),
    );
    document.querySelector<HTMLButtonElement>("[aria-label='HDSky 刷流 的运行记录']")!.click();
    await vi.waitFor(() => expect(api.runs).toHaveBeenCalledWith(1, { limit: 20, offset: 0 }));
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-runs]")).not.toBeNull(),
    );

    const drawer = () => document.querySelector("[data-testid=brush-runs]")!.textContent ?? "";
    expect(drawer()).toContain("手动");
    expect(drawer()).toContain("失败");
    expect(drawer()).toContain("部分失败");
    expect(drawer()).toContain("用时 8 秒");
    expect(drawer()).toContain("authentication failed");
    // 最新一条展开，第二条收着
    expect(document.querySelector("[data-testid=brush-run-steps-3]")?.textContent).toContain(
      "今天已加入 0.00 GB",
    );
    expect(document.querySelector("[data-testid=brush-run-steps-2]")).toBeNull();

    document.querySelector<HTMLButtonElement>("[data-testid=brush-run-2]")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-run-steps-2]")?.textContent).toContain(
        "Demo.One（免费，4.00 GB）",
      ),
    );
    expect(document.querySelector("[data-testid=brush-run-steps-2]")?.textContent).toContain(
      "步骤太多",
    );

    const more = document.querySelector<HTMLButtonElement>("[data-testid=brush-runs-more]")!;
    expect(more.textContent).toContain("再看 1 条");
    more.click();
    await vi.waitFor(() => expect(api.runs).toHaveBeenLastCalledWith(1, { limit: 20, offset: 2 }));
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-run-1]")).not.toBeNull(),
    );
    expect(document.querySelector("[data-testid=brush-runs-more]")).toBeNull();
  });

  it("「上次运行」的摘要点了也打开运行记录；立即运行之后开着的记录重新读", async () => {
    await mountPage([task()]);
    api.runs.mockResolvedValue({ items: [brushRun()], total: 1 });
    await vi.waitFor(() => expect(document.querySelector(".brush-lastrun")).not.toBeNull());
    document.querySelector<HTMLButtonElement>(".brush-lastrun")!.click();
    await vi.waitFor(() => expect(api.runs).toHaveBeenCalledTimes(1));
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=brush-run-1]")).not.toBeNull(),
    );

    api.run.mockResolvedValueOnce({
      result: { task_id: 1, sampled: 0, removed: 0, gone: 0, listed: 10, eligible: 0, added: 0 },
    });
    document.querySelector<HTMLButtonElement>("[aria-label='立即运行 HDSky 刷流']")!.click();
    await vi.waitFor(() => expect(api.runs).toHaveBeenCalledTimes(2));
  });

  it("运行记录：换了任务时旧请求晚到不覆盖", async () => {
    await mountPage([task(), task({ id: 2, name: "MT 刷流", site_name: "mteam" })]);
    let releaseFirst!: (v: unknown) => void;
    api.runs.mockImplementation((id: number) =>
      id === 1
        ? new Promise((resolve) => {
            releaseFirst = resolve;
          })
        : Promise.resolve({
            items: [brushRun({ id: 20, task_id: 2, summary: "MT 的一轮" })],
            total: 1,
          }),
    );
    await vi.waitFor(() =>
      expect(document.querySelector("[aria-label='MT 刷流 的运行记录']")).not.toBeNull(),
    );
    document.querySelector<HTMLButtonElement>("[aria-label='HDSky 刷流 的运行记录']")!.click();
    await vi.waitFor(() => expect(api.runs).toHaveBeenCalledWith(1, { limit: 20, offset: 0 }));
    document.querySelector<HTMLButtonElement>("[aria-label='MT 刷流 的运行记录']")!.click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("MT 的一轮"));

    releaseFirst({ items: [brushRun({ id: 10, summary: "HDSky 的旧一轮" })], total: 1 });
    await flush();
    expect(document.body.textContent).not.toContain("HDSky 的旧一轮");
    expect(document.body.textContent).toContain("MT 的一轮");
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
