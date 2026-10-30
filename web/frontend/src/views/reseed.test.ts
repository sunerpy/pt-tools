// @vitest-environment happy-dom
/*
 * 钉子：IYUU 辅种页。没有 token 时不能运行；保存时只在填了新 token 时才带上它，清除要确认并一并关闭；
 * 站点对照与记录写明状态；立即运行在后台跑。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  settings: vi.fn(),
  saveSettings: vi.fn(),
  sites: vi.fn(),
  setSite: vi.fn(),
  setAllSites: vi.fn(),
  run: vi.fn(),
  runs: vi.fn(),
  runDetail: vi.fn(),
  runAttempts: vi.fn(),
  pauseRun: vi.fn(),
  resumeRun: vi.fn(),
  stopRun: vi.fn(),
  continueRun: vi.fn(),
  records: vi.fn(),
  jobs: vi.fn(),
  clearFinished: vi.fn(),
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
    reseedApi: api,
    downloadersApi: {
      ...real.downloadersApi,
      list: vi.fn(() =>
        Promise.resolve([
          {
            id: 1,
            name: "qb",
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
const testid = (id: string) => {
  const el = document.querySelector<HTMLElement>(`[data-testid=${id}]`);
  if (!el) throw new Error(`找不到 ${id}`);
  return el;
};

function settings(extra: Record<string, unknown> = {}) {
  return {
    enabled: false,
    has_token: false,
    interval_hours: 12,
    downloader_ids: [],
    site_names: [],
    site_mode: "all",
    max_per_site_per_day: 20,
    last_result: "",
    running: false,
    ...extra,
  };
}

async function mountPage(s = settings()) {
  api.settings.mockResolvedValue(s);
  api.records.mockResolvedValue({ items: [] });
  api.runs.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 });
  api.sites.mockResolvedValue({
    items: [
      {
        sid: 1,
        iyuu_site: "hdsky",
        nickname: "天空",
        host: "hdsky.me",
        site_name: "hdsky",
        configured: true,
        selected: true,
      },
      {
        sid: 2,
        iyuu_site: "ourbits",
        nickname: "我堡",
        host: "ourbits.club",
        site_name: "ourbits",
        configured: false,
        selected: false,
      },
      {
        sid: 3,
        iyuu_site: "x",
        nickname: "无名",
        host: "x.example",
        configured: false,
        selected: false,
      },
    ],
  });
  const Page = (await import("./Reseed.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.settings).toHaveBeenCalled());
  await flush();
  await flush();
}

async function chooseTab(label: string) {
  const item = [
    ...document.querySelectorAll<HTMLElement>("[data-testid=rs-tabs] .el-segmented__item"),
  ].find((el) => (el.textContent ?? "").trim() === label);
  if (!item) throw new Error(`找不到标签「${label}」`);
  item.click();
  await flush();
}

function setToken(v: string) {
  const el = testid("rs-token");
  const input = (
    el instanceof HTMLInputElement ? el : el.querySelector("input")
  ) as HTMLInputElement;
  input.value = v;
  input.dispatchEvent(new Event("input"));
}

describe("IYUU 辅种", () => {
  it("刷新失败时留着上次的记录和站点，并写明是旧的", async () => {
    await mountPage(settings({ has_token: true }));
    api.records.mockResolvedValue({
      items: [
        {
          id: 1,
          info_hash: "a",
          site_name: "hdsky",
          torrent_id: "11",
          source_hash: "s",
          downloader_id: 1,
          name: "Movie",
          state: "done",
          message: "",
          created_at: "2026-10-07T10:00:00Z",
          updated_at: "2026-10-07T10:00:00Z",
        },
      ],
    });
    await chooseTab("记录");
    await vi.waitFor(() => expect(document.body.textContent).toContain("已辅种"));
    expect(document.querySelector("[data-testid=rs-records-stale]")).toBeNull();
    api.records.mockRejectedValue(new Error("网络断了"));
    testid("rs-refresh").click();
    await vi.waitFor(() => expect(testid("rs-records-stale").textContent).toContain("网络断了"));
    expect(document.body.textContent).toContain("已辅种");

    await chooseTab("站点");
    await vi.waitFor(() => expect(document.body.textContent).toContain("hdsky.me"));
    api.sites.mockRejectedValue(new Error("IYUU 超时"));
    testid("rs-refresh").click();
    await vi.waitFor(() => expect(testid("rs-sites-stale").textContent).toContain("IYUU 超时"));
    expect(document.body.textContent).toContain("hdsky.me");
  });

  it("没有 token 时提示并且不能运行；保存时带上新填的 token", async () => {
    await mountPage();
    expect(document.body.textContent).toContain("填写 IYUU token 后才能辅种");
    expect(document.body.textContent).toContain("info hash 发给 IYUU");
    expect((testid("rs-run") as HTMLButtonElement).disabled).toBe(true);
    expect(api.sites).not.toHaveBeenCalled();

    setToken("  tok  ");
    await flush();
    api.saveSettings.mockResolvedValue(settings({ has_token: true }));
    testid("rs-save").click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalled());
    expect(api.saveSettings.mock.calls[0][0]).toMatchObject({
      token: "tok",
      enabled: false,
      interval_hours: 12,
    });
  });

  it("已有 token：不填就不带 token；清除要确认并关闭辅种", async () => {
    await mountPage(settings({ has_token: true, enabled: true }));
    expect((testid("rs-run") as HTMLButtonElement).disabled).toBe(false);
    api.saveSettings.mockResolvedValue(settings({ has_token: true, enabled: true }));
    testid("rs-save").click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalled());
    expect(api.saveSettings.mock.calls[0][0]).not.toHaveProperty("token");

    ui.confirm.mockResolvedValue("confirm");
    api.saveSettings.mockResolvedValue(settings());
    testid("rs-clear-token").click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(2));
    expect(api.saveSettings.mock.calls[1][0]).toMatchObject({ token: "", enabled: false });
  });

  it("站点对照写明每个 IYUU 站点在 pt-tools 里的情况", async () => {
    await mountPage(settings({ has_token: true }));
    await chooseTab("站点");
    await vi.waitFor(() => expect(document.body.textContent).toContain("hdsky.me"));
    const text = document.body.textContent ?? "";
    expect(text).toContain("参与辅种");
    expect(text).toContain("没有配置");
    expect(text).toContain("pt-tools 不支持");
  });

  it("站点用开关打开或关掉；全部关掉以后写明新站点默认不参与", async () => {
    await mountPage(settings({ has_token: true }));
    await chooseTab("站点");
    await vi.waitFor(() => expect(document.body.textContent).toContain("hdsky.me"));
    expect(document.querySelector("[data-testid=rs-site-switch-ourbits]")).toBeNull();
    expect(document.querySelector("[data-testid=rs-sites-mode-note]")).toBeNull();

    api.setSite.mockResolvedValue({ site_mode: "selected", site_names: [] });
    testid("rs-site-switch-hdsky").click();
    await vi.waitFor(() => expect(api.setSite).toHaveBeenCalledWith("hdsky", false));
    await vi.waitFor(() =>
      expect(testid("rs-sites-mode-note").textContent).toContain("默认不参与"),
    );
    expect(testid("rs-site-switch-hdsky").classList.contains("is-checked")).toBe(false);

    api.setAllSites.mockResolvedValue({ site_mode: "all", site_names: [] });
    testid("rs-sites-all-on").click();
    await vi.waitFor(() => expect(api.setAllSites).toHaveBeenCalledWith(true));
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=rs-sites-mode-note]")).toBeNull(),
    );
    expect(testid("rs-site-switch-hdsky").classList.contains("is-checked")).toBe(true);
  });

  it("设置里不再保存站点名单（站点在「站点」页用开关改）", async () => {
    await mountPage(settings({ has_token: true, site_mode: "selected", site_names: ["hdsky"] }));
    expect(document.querySelector("[data-testid=rs-sites]")).toBeNull();
    api.saveSettings.mockResolvedValue(settings({ has_token: true }));
    testid("rs-save").click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalled());
    expect(api.saveSettings.mock.calls[0][0]).not.toHaveProperty("site_names");
  });

  it("记录写明结果与原因；立即运行在后台跑", async () => {
    await mountPage(
      settings({
        has_token: true,
        last_run_at: "2026-10-07T10:00:00Z",
        last_result: "查询 3 个种子，找到 1 个可辅种，加入 1 个",
      }),
    );
    expect(document.body.textContent).toContain("查询 3 个种子");
    api.records.mockResolvedValue({
      items: [
        {
          id: 2,
          info_hash: "a",
          site_name: "hdsky",
          torrent_id: "11",
          source_hash: "s",
          downloader_id: 1,
          name: "Movie",
          state: "queued",
          message: "",
          job_id: 5,
          job_state: "source_removed",
          created_at: "2026-10-07T10:00:00Z",
        },
        {
          id: 1,
          info_hash: "b",
          site_name: "hdsky",
          torrent_id: "12",
          source_hash: "s",
          downloader_id: 1,
          name: "Movie",
          state: "failed",
          message: "文件列表与原种子不一致：文件数不同（1 与 2）",
          created_at: "2026-10-07T10:00:00Z",
        },
      ],
    });
    await chooseTab("记录");
    await vi.waitFor(() => expect(document.body.textContent).toContain("已辅种"));
    expect(document.body.textContent).toContain("文件列表与原种子不一致");

    api.run.mockResolvedValue({ started: true });
    api.settings.mockResolvedValue(settings({ has_token: true, running: true }));
    testid("rs-run").click();
    await vi.waitFor(() => expect(api.run).toHaveBeenCalled());
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalled());
  });

  it("写明 token 去哪里拿、上次运行与下次运行", async () => {
    await mountPage(
      settings({
        has_token: true,
        enabled: true,
        last_run_at: "2026-10-15T10:00:00Z",
        last_result: "辅种 3 个",
        next_run_at: "2026-10-15T22:00:00Z",
      }),
    );
    expect(testid("rs-token-tip").querySelector("a")!.getAttribute("href")).toBe(
      "https://iyuu.cn/",
    );
    const st = testid("rs-status").textContent ?? "";
    expect(st).toContain("辅种 3 个");
    expect(st).toContain("大约");
  });

  it("有 token、没开启：写明只能立即运行", async () => {
    await mountPage(settings({ has_token: true }));
    expect(testid("rs-status").textContent).toContain("没开启：只能「立即运行」");
    expect(testid("rs-status").textContent).toContain("还没运行过");
  });
});

function currentRun(extra: Record<string, unknown> = {}) {
  return {
    id: 7,
    trigger: "manual",
    status: "running",
    started_at: "2026-10-30T10:00:00Z",
    hashes: 450,
    batches: 3,
    batches_done: 1,
    candidates: 4,
    created: 1,
    failed: 0,
    gone: 0,
    skipped: 2,
    ...extra,
  };
}

function run(extra: Record<string, unknown> = {}) {
  return {
    id: 7,
    trigger: "schedule",
    started_at: "2026-10-30T10:00:00Z",
    finished_at: "2026-10-30T10:05:00Z",
    status: "stopped",
    summary: "查询 450 个种子，找到 4 个可辅种，加入 1 个，这一轮被停止",
    error: "这一轮被停止",
    downloaders: 1,
    hashes: 450,
    batches: 3,
    batches_done: 1,
    candidates: 4,
    created: 1,
    failed: 0,
    gone: 0,
    interrupted: 1,
    skipped: 2,
    skipped_detail: { 下载器里已经有: 2 },
    iyuu_calls: 3,
    paused_seconds: 60,
    steps_truncated: false,
    continuable: true,
    ...extra,
  };
}

describe("辅种轮次", () => {
  it("正在跑的一轮：页头可以暂停、继续、停止，停止前确认能一起取消等待中的任务", async () => {
    await mountPage(settings({ has_token: true, running: true, current_run: currentRun() }));
    expect(document.querySelector("[data-testid=rs-run]")).toBeNull();
    expect(testid("rs-progress").textContent).toContain("第 2/3 批");

    api.pauseRun.mockResolvedValue({ ok: true });
    api.settings.mockResolvedValue(
      settings({ has_token: true, running: true, current_run: currentRun({ status: "paused" }) }),
    );
    testid("rs-pause").click();
    await vi.waitFor(() => expect(api.pauseRun).toHaveBeenCalledWith(7));
    await vi.waitFor(() => expect(testid("rs-progress").textContent).toContain("已暂停"));

    api.resumeRun.mockResolvedValue({ ok: true });
    api.settings.mockResolvedValue(
      settings({ has_token: true, running: true, current_run: currentRun() }),
    );
    testid("rs-resume").click();
    await vi.waitFor(() => expect(api.resumeRun).toHaveBeenCalledWith(7));
    await vi.waitFor(() => expect(document.querySelector("[data-testid=rs-pause]")).not.toBeNull());

    testid("rs-stop").click();
    await vi.waitFor(() => expect(testid("rs-stop-confirm")).toBeTruthy());
    testid("rs-stop-cancel-pending").querySelector("input")!.click();
    api.stopRun.mockResolvedValue({ ok: true });
    api.settings.mockResolvedValue(settings({ has_token: true }));
    testid("rs-stop-confirm").click();
    await vi.waitFor(() => expect(api.stopRun).toHaveBeenCalledWith(7, true));
    await vi.waitFor(() => expect(document.querySelector("[data-testid=rs-run]")).not.toBeNull());
  });

  it("轮次列表：提前停下的最近一轮可以继续；详情里有计数、步骤与尝试", async () => {
    await mountPage(settings({ has_token: true }));
    api.runs.mockResolvedValue({ items: [run()], total: 1, page: 1, page_size: 20 });
    await chooseTab("轮次");
    await vi.waitFor(() => expect(document.body.textContent).toContain("这一轮被停止"));
    expect(document.body.textContent).toContain("已停止");

    api.runDetail.mockResolvedValue({
      ...run(),
      steps: [
        { kind: "query", text: "查询第 1/3 批（200 个种子）：2 个种子在别的站点上有可辅种的" },
        { kind: "stop", text: "这一轮被停止" },
      ],
    });
    api.runAttempts.mockResolvedValue({
      items: [
        {
          id: 1,
          run_id: 7,
          started_at: "2026-10-30T10:01:00Z",
          site_name: "hdsky",
          info_hash: "a",
          torrent_id: "11",
          name: "Movie",
          downloader_id: 1,
          outcome: "created",
          message: "",
          job_id: 3,
          job_state: "source_removed",
          backfilled: false,
        },
        {
          id: 2,
          run_id: 7,
          started_at: "2026-10-30T10:02:00Z",
          site_name: "hdsky",
          info_hash: "b",
          torrent_id: "12",
          name: "Show",
          downloader_id: 1,
          outcome: "interrupted",
          message: "这一轮被打断，没有试完",
          backfilled: false,
        },
      ],
      total: 2,
      page: 1,
      page_size: 50,
    });
    testid("rs-run-open-7").click();
    await vi.waitFor(() => expect(testid("rs-run-steps").textContent).toContain("查询第 1/3 批"));
    expect(testid("rs-run-counts").textContent).toContain("1/3");
    expect(testid("rs-run-attempts").textContent).toContain("已辅种");
    expect(testid("rs-run-attempts").textContent).toContain("被打断");

    api.continueRun.mockResolvedValue({ started: true, run_id: 8 });
    testid("rs-run-continue").click();
    await vi.waitFor(() => expect(api.continueRun).toHaveBeenCalledWith(7));
  });
});
