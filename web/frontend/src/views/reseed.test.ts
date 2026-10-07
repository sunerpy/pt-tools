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
  run: vi.fn(),
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
    max_per_site_per_day: 20,
    last_result: "",
    running: false,
    ...extra,
  };
}

async function mountPage(s = settings()) {
  api.settings.mockResolvedValue(s);
  api.records.mockResolvedValue({ items: [] });
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
});
