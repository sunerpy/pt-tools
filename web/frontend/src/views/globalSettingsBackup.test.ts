// @vitest-environment happy-dom
/*
 * 钉子：系统设置里的「备份」一节。和每日战报、站点代理一样走自己的接口：改了才保存（全局配置之后的一步），
 * 成功换上服务端整理过的值，失败留着改动、一条提示说清没保存；读不回来时整段禁用、不参与保存，
 * 下面的「备份与恢复」面板写明读取失败而不是「还没有备份」。带上密钥时写明风险。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BackupsView } from "@/api";
import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  saveGlobal: vi.fn(),
  getBackups: vi.fn(),
  saveBackups: vi.fn(),
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
          free_end_advance_minutes: 5,
          default_filter_mode: "auto_free",
        }),
      ),
      save: api.saveGlobal,
    },
    chatopsApi: { notifications: { list: vi.fn(() => Promise.resolve([])) } },
    userInfoApi: {
      ...real.userInfoApi,
      getDailyReport: vi.fn(() =>
        Promise.resolve({ enabled: false, time: "22:00", channel_ids: [] }),
      ),
      saveDailyReport: vi.fn(),
    },
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
    backupApi: { ...real.backupApi, get: api.getBackups, saveSettings: api.saveBackups },
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

function backups(
  over: Partial<BackupsView["settings"]> = {},
  rest: Partial<BackupsView> = {},
): BackupsView {
  return {
    settings: {
      enabled: true,
      time: "04:30",
      keep: 7,
      dir: "",
      include_secret: true,
      default_dir: "/root/.pt-tools/backups",
      min_keep: 1,
      max_keep: 60,
      ...over,
    },
    dir: over.dir || "/root/.pt-tools/backups",
    env_key: false,
    next_at: "2026-11-03T04:30:00Z",
    last: null,
    restore: null,
    pending: null,
    backups: [],
    ...rest,
  };
}

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const q = <T extends HTMLElement = HTMLElement>(id: string) =>
  document.querySelector<T>(`[data-testid=${id}]`);
const tick = () => new Promise((r) => setTimeout(r, 0));

function saveButton(): HTMLButtonElement {
  const btn = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => (b.textContent ?? "").trim() === "保存设置" && !b.disabled,
  );
  if (!btn) throw new Error("找不到可点的「保存设置」");
  return btn;
}

function disabled(id: string): boolean {
  const el = q(id)!;
  return (
    el.classList.contains("is-disabled") ||
    el.querySelector(".is-disabled") !== null ||
    (el as HTMLInputElement).disabled === true
  );
}

async function mountPage(got: BackupsView | Error = backups()) {
  if (got instanceof Error) api.getBackups.mockRejectedValue(got);
  else api.getBackups.mockResolvedValue(got);
  const Page = (await import("./GlobalSettings.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.getBackups).toHaveBeenCalled());
  await vi.waitFor(() => saveButton());
  await tick();
}

async function fillDir(value: string) {
  const input = q<HTMLInputElement>("backup-dir")!;
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await tick();
}

describe("系统设置：备份", () => {
  it("读回设置：开关、时刻、份数、目录；默认目录写在提示里；带上密钥时写明风险", async () => {
    await mountPage();
    expect(q("backup-enabled")!.classList.contains("is-checked")).toBe(true);
    expect(q("backup-time")!.textContent).toContain("04:30");
    expect(q("backup-keep")!.querySelector<HTMLInputElement>("input")!.value).toBe("7");
    expect(q<HTMLInputElement>("backup-dir")!.placeholder).toBe("/root/.pt-tools/backups");
    expect(document.body.textContent).toContain("留空用默认目录");
    expect(q("backup-secret-risk")).not.toBeNull();
    expect(q("backup-env-key")).toBeNull();
    expect(q("backup-load-failed")).toBeNull();
    // 下面的面板用同一份备份页
    expect(q("backup-empty")?.textContent).toContain("还没有备份");
  });

  it("关掉带上密钥：风险提示收起；密钥来自环境变量时写明", async () => {
    await mountPage(backups({ include_secret: false }, { env_key: true }));
    expect(q("backup-secret-risk")).toBeNull();
    expect(q("backup-env-key")?.textContent).toContain("PT_TOOLS_SECRET_KEY");
  });

  it("没改备份设置：保存时不写", async () => {
    await mountPage();
    api.saveGlobal.mockResolvedValue(undefined);
    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("保存成功"));
    expect(api.saveBackups).not.toHaveBeenCalled();
  });

  it("改了目录与开关：全局配置之后保存备份设置，换上服务端整理过的值", async () => {
    await mountPage();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveBackups.mockResolvedValue(backups({ enabled: false, dir: "/mnt/bk" }));
    await fillDir("  /mnt/x/../bk  ");
    q("backup-enabled")!.click();
    await tick();

    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("保存成功"));
    expect(api.saveGlobal.mock.invocationCallOrder[0]).toBeLessThan(
      api.saveBackups.mock.invocationCallOrder[0]!,
    );
    expect(api.saveBackups).toHaveBeenCalledWith({
      enabled: false,
      time: "04:30",
      keep: 7,
      dir: "/mnt/x/../bk",
      include_secret: true,
    });
    await tick();
    expect(q<HTMLInputElement>("backup-dir")!.value).toBe("/mnt/bk");
    expect(document.body.textContent).toContain("定时备份已关闭");

    // 再点一次：已经是保存过的值，不重复写
    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledTimes(2));
    expect(api.saveBackups).toHaveBeenCalledTimes(1);
  });

  it("备份设置保存失败：说明全局配置已保存，改动留在表单里", async () => {
    await mountPage();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveBackups.mockRejectedValue(new Error("备份目录要写绝对路径"));
    await fillDir("backups");
    saveButton().click();
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalled());
    const msg = String(ui.warning.mock.calls[0]![0]);
    expect(msg).toContain("全局配置已保存");
    expect(msg).toContain("备份设置没有保存：备份目录要写绝对路径");
    expect(q<HTMLInputElement>("backup-dir")!.value).toBe("backups");
  });

  it("读不回来：整段禁用、不参与保存；面板写明读取失败", async () => {
    await mountPage(new Error("session 无效或已过期"));
    expect(q("backup-load-failed")).not.toBeNull();
    expect(disabled("backup-enabled")).toBe(true);
    expect(disabled("backup-secret")).toBe(true);
    expect(q("backup-state")?.textContent).toContain("session 无效或已过期");
    expect(q("backup-empty")).toBeNull();

    api.saveGlobal.mockResolvedValue(undefined);
    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("保存成功"));
    expect(api.saveBackups).not.toHaveBeenCalled();
  });
});
