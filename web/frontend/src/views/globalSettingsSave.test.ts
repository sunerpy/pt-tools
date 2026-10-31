// @vitest-environment happy-dom
/*
 * 钉子：系统设置的「保存设置」先写全局配置、再写每日战报，两步分开报告结果。
 *
 * 原来两步包在同一个 try 里：全局配置已经落库、后一步没写上时，页面只弹一句「保存失败」，
 * 用户以为什么都没改，其实全局配置已经变了。（每日签到的设置挪到了「自动签到」页，第二步现在是每日战报。）
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  getGlobal: vi.fn(),
  saveGlobal: vi.fn(),
  getReport: vi.fn(),
  saveReport: vi.fn(),
}));

const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    globalApi: { get: api.getGlobal, save: api.saveGlobal },
    userInfoApi: {
      ...real.userInfoApi,
      getDailyReport: api.getReport,
      saveDailyReport: api.saveReport,
    },
    chatopsApi: { ...real.chatopsApi, notifications: { list: vi.fn(() => Promise.resolve([])) } },
    // 站点代理：读到默认值（没有设置），没改就不写
    siteProxyApi: {
      get: vi.fn(() =>
        Promise.resolve({
          enabled: false,
          proxy_url: "",
          mode: "all",
          sites: [],
          updated_at: null,
          site_options: [],
        }),
      ),
      save: vi.fn(),
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

const SETTINGS = {
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
};

let view: MountedView | null = null;

async function mountPage() {
  const GlobalSettings = (await import("./GlobalSettings.vue")).default;
  view = mountView(GlobalSettings);
}

function saveButton(): HTMLButtonElement {
  const btn = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => (b.textContent ?? "").trim() === "保存设置" && !b.disabled,
  );
  if (!btn) throw new Error("找不到可点的「保存设置」");
  return btn;
}

/** 把每日战报的发送时刻改成 00:00 */
async function changeReportTime() {
  const field = document.querySelector("[data-testid=daily-report-time]")!;
  (field.querySelector<HTMLElement>(".el-select__wrapper") ?? (field as HTMLElement)).click();
  await vi.waitFor(() => {
    const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
      (o) => (o.textContent ?? "").trim() === "00:00",
    );
    expect(opt).toBeTruthy();
    opt!.click();
  });
}

async function mountLoaded() {
  api.getGlobal.mockResolvedValue(structuredClone(SETTINGS));
  // 战报得是开着的：关着的时候「发送时刻」不可改（跟着开关走）
  api.getReport.mockResolvedValue({ enabled: true, time: "22:00", channel_ids: [1] });
  await mountPage();
  await vi.waitFor(() => expect(api.getReport).toHaveBeenCalled());
  await vi.waitFor(() => saveButton());
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("系统设置：两步保存分开报告", () => {
  it("全局配置已保存、每日战报失败：说明前一步已生效，不报整体失败", async () => {
    await mountLoaded();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveReport.mockRejectedValue(new Error("战报设置写入失败"));
    await changeReportTime();

    saveButton().click();

    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalled());
    const msg = String(ui.warning.mock.calls[0]![0]);
    expect(msg).toContain("全局配置已保存");
    expect(msg).toContain("战报设置写入失败");
    expect(ui.error).not.toHaveBeenCalled();
    expect(ui.success).not.toHaveBeenCalled();
  });

  it("全局配置保存失败：报失败，不再写每日战报", async () => {
    await mountLoaded();
    api.saveGlobal.mockRejectedValue(new Error("磁盘只读"));
    await changeReportTime();

    saveButton().click();

    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("磁盘只读"));
    expect(api.saveReport).not.toHaveBeenCalled();
    expect(ui.success).not.toHaveBeenCalled();
  });

  it("两步都成功：提示保存成功，战报记成已保存，再点一次不重复写", async () => {
    await mountLoaded();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveReport.mockImplementation((v: Record<string, unknown>) => Promise.resolve({ ...v }));
    await changeReportTime();

    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledTimes(1));
    expect(api.saveReport).toHaveBeenCalledWith({
      enabled: true,
      time: "00:00",
      channel_ids: [1],
      scope: "previous_day",
    });

    await vi.waitFor(() => saveButton());
    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledTimes(2));
    expect(api.saveReport).toHaveBeenCalledTimes(1);
  });

  it("每日签到的设置挪到了「自动签到」页：这里只留一句指路", async () => {
    await mountLoaded();
    const moved = document.querySelector("[data-testid=attendance-moved]");
    expect(moved?.textContent).toContain("自动签到");
    expect(document.querySelector("[data-testid=attendance-window-start]")).toBeNull();
  });
});

describe("系统设置：免费结束提前量", () => {
  it("提前量是 0 时提示可能多出计费下载量，改成 5 以后不再提示", async () => {
    await mountLoaded();
    expect(document.querySelector("[data-testid=gs-advance-zero]")).toBeTruthy();
    view!.unmount();
    api.getGlobal.mockResolvedValue({ ...structuredClone(SETTINGS), free_end_advance_minutes: 5 });
    await mountPage();
    await vi.waitFor(() => expect(api.getGlobal).toHaveBeenCalledTimes(2));
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=gs-advance-zero]")).toBeNull(),
    );
  });
});

describe("系统设置：页头摘要与每日战报的从属字段", () => {
  it("页头摘要写的是已生效的配置：改了下载模式没保存时不变，保存成功后才更新", async () => {
    await mountLoaded();
    const head = () => document.querySelector("#pt-head-sub")?.textContent ?? "";
    await vi.waitFor(() => expect(head()).toContain("智能模式"));

    const radio = [...document.querySelectorAll<HTMLInputElement>("input[type=radio]")].find(
      (r) => r.value === "free_only",
    )!;
    radio.click();
    await vi.waitFor(() => expect(radio.checked).toBe(true));
    expect(head()).toContain("智能模式");
    expect(head()).not.toContain("仅免费");

    api.saveGlobal.mockResolvedValue(undefined);
    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalled());
    await vi.waitFor(() => expect(head()).toContain("仅免费"));
    expect(api.saveGlobal.mock.calls[0]![0]).toMatchObject({ default_filter_mode: "free_only" });
  });

  it("战报没开时发送时刻与接收通道不可改，打开开关后可改", async () => {
    api.getGlobal.mockResolvedValue(structuredClone(SETTINGS));
    api.getReport.mockResolvedValue({ enabled: false, time: "22:00", channel_ids: [] });
    await mountPage();
    await vi.waitFor(() => saveButton());
    const disabled = (id: string) => {
      const el = document.querySelector(`[data-testid=${id}]`)!;
      return el.classList.contains("is-disabled") || el.querySelector(".is-disabled") !== null;
    };
    await vi.waitFor(() => expect(disabled("daily-report-time")).toBe(true));
    expect(disabled("daily-report-channels")).toBe(true);

    document.querySelector<HTMLElement>("[data-testid=daily-report-enabled]")!.click();
    await vi.waitFor(() => expect(disabled("daily-report-time")).toBe(false));
    expect(disabled("daily-report-channels")).toBe(false);
  });
});
