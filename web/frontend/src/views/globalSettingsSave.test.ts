// @vitest-environment happy-dom
/*
 * 钉子：系统设置的「保存设置」先写全局配置、再写签到时间窗，两步分开报告结果。
 *
 * 原来两步包在同一个 try 里：全局配置已经落库、签到时间窗没写上时，页面只弹一句「保存失败」，
 * 用户以为什么都没改，其实全局配置已经变了。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  getGlobal: vi.fn(),
  saveGlobal: vi.fn(),
  getAttendance: vi.fn(),
  saveAttendance: vi.fn(),
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
    attendanceApi: { getSettings: api.getAttendance, saveSettings: api.saveAttendance },
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

/** 把签到时间窗开始改成 00:00（只有开始那一列有这个选项） */
async function changeWindowStart() {
  const field = document.querySelector("[data-testid=attendance-window-start]")!;
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
  api.getAttendance.mockResolvedValue({ window_start: "08:00", window_end: "10:00" });
  await mountPage();
  await vi.waitFor(() => expect(api.getAttendance).toHaveBeenCalled());
  await vi.waitFor(() => saveButton());
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("系统设置：两步保存分开报告", () => {
  it("全局配置已保存、签到时间窗失败：说明前一步已生效，不报整体失败", async () => {
    await mountLoaded();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveAttendance.mockRejectedValue(new Error("签到设置写入失败"));
    await changeWindowStart();

    saveButton().click();

    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalled());
    const msg = String(ui.warning.mock.calls[0]![0]);
    expect(msg).toContain("全局配置已保存");
    expect(msg).toContain("签到设置写入失败");
    expect(ui.error).not.toHaveBeenCalled();
    expect(ui.success).not.toHaveBeenCalled();
  });

  it("全局配置保存失败：报失败，不再写签到时间窗", async () => {
    await mountLoaded();
    api.saveGlobal.mockRejectedValue(new Error("磁盘只读"));
    await changeWindowStart();

    saveButton().click();

    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("磁盘只读"));
    expect(api.saveAttendance).not.toHaveBeenCalled();
    expect(ui.success).not.toHaveBeenCalled();
  });

  it("两步都成功：提示保存成功，签到时间窗记成已保存，再点一次不重复写", async () => {
    await mountLoaded();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveAttendance.mockImplementation((v: { window_start: string; window_end: string }) =>
      Promise.resolve({ ...v }),
    );
    await changeWindowStart();

    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledTimes(1));
    expect(api.saveAttendance).toHaveBeenCalledWith({ window_start: "00:00", window_end: "10:00" });

    await vi.waitFor(() => saveButton());
    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledTimes(2));
    expect(api.saveAttendance).toHaveBeenCalledTimes(1);
  });
});
