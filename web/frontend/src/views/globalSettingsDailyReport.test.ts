// @vitest-environment happy-dom
/*
 * 钉子：系统设置里的每日战报是保存的第三步。开启却没选通道时前端就拦下、什么都不写；
 * 第三步失败时说清前两步已保存。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  saveGlobal: vi.fn(),
  getReport: vi.fn(),
  saveReport: vi.fn(),
}));
const ui = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    globalApi: {
      get: vi.fn(() =>
        Promise.resolve({
          default_interval_minutes: 10,
          download_dir: "/downloads",
          download_limit_enabled: false,
          download_speed_limit: 20,
          torrent_size_gb: 200,
          torrent_min_size_gb: 0,
          min_free_minutes: 30,
          auto_start: false,
          retain_hours: 24,
          max_retry: 3,
          default_concurrency: 3,
          default_enabled: false,
          auto_delete_on_free_end: false,
          free_end_advance_minutes: 0,
          default_filter_mode: "auto_free",
        }),
      ),
      save: api.saveGlobal,
    },
    attendanceApi: {
      getSettings: vi.fn(() => Promise.resolve({ window_start: "08:00", window_end: "10:00" })),
      saveSettings: vi.fn(),
    },
    chatopsApi: {
      notifications: {
        list: vi.fn(() =>
          Promise.resolve([{ id: 7, name: "TG", channel_type: "telegram", enabled: true }]),
        ),
      },
    },
    userInfoApi: {
      ...real.userInfoApi,
      getDailyReport: api.getReport,
      saveDailyReport: api.saveReport,
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

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

function saveButton(): HTMLButtonElement {
  const btn = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => (b.textContent ?? "").trim() === "保存设置" && !b.disabled,
  );
  if (!btn) throw new Error("找不到可点的「保存设置」");
  return btn;
}

async function mountPage() {
  const Page = (await import("./GlobalSettings.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.getReport).toHaveBeenCalled());
  await vi.waitFor(() => saveButton());
}

async function toggleReport() {
  const sw = document.querySelector<HTMLElement>("[data-testid=daily-report-enabled]")!;
  sw.click();
  await new Promise((r) => setTimeout(r, 0));
}

describe("系统设置：每日战报", () => {
  it("开启却没选通道：直接拦下，什么都不保存", async () => {
    api.getReport.mockResolvedValue({ enabled: false, time: "22:00", channel_ids: [] });
    await mountPage();
    await toggleReport();

    saveButton().click();
    await vi.waitFor(() =>
      expect(ui.error).toHaveBeenCalledWith("开启每日战报至少要选一个通知通道"),
    );
    expect(api.saveGlobal).not.toHaveBeenCalled();
    expect(api.saveReport).not.toHaveBeenCalled();
  });

  it("改了战报设置：第三步保存；它失败时说清前两步已保存", async () => {
    api.getReport.mockResolvedValue({ enabled: true, time: "22:00", channel_ids: [7] });
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveReport.mockRejectedValue(new Error("磁盘只读"));
    await mountPage();
    await toggleReport(); // 关掉

    saveButton().click();
    await vi.waitFor(() =>
      expect(api.saveReport).toHaveBeenCalledWith({
        enabled: false,
        time: "22:00",
        channel_ids: [7],
      }),
    );
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalled());
    expect(String(ui.warning.mock.calls[0]![0])).toContain("每日战报设置没有保存：磁盘只读");
    expect(ui.success).not.toHaveBeenCalled();
  });

  it("没改战报设置：不发第三步请求", async () => {
    api.getReport.mockResolvedValue({ enabled: true, time: "22:00", channel_ids: [7] });
    api.saveGlobal.mockResolvedValue(undefined);
    await mountPage();

    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalled());
    expect(api.saveReport).not.toHaveBeenCalled();
  });
});
