// @vitest-environment happy-dom
/*
 * 钉子：自动清理页的全局配置没读回来时，两颗「保存设置」都不能点。
 *
 * 读失败（或还在读）时表单停在一整套前端默认值上；save() 会先 GET 一次现有配置，
 * 再用表单里的删种策略整段覆盖上去 —— 等于把服务端真实的删种条件换成默认值。
 * 与系统设置页（GlobalSettings）一致：加载中或加载失败时禁用保存。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  getGlobal: vi.fn(),
  saveGlobal: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    globalApi: { get: api.getGlobal, save: api.saveGlobal },
    maintenanceApi: { preview: vi.fn(), clean: vi.fn() },
  };
});

const SETTINGS = {
  cleanup_enabled: true,
  cleanup_interval_min: 30,
  cleanup_scope: "database",
  cleanup_scope_tags: "",
  cleanup_protect_tags: "keep",
  cleanup_condition_mode: "and",
  cleanup_max_seed_time_h: 240,
  cleanup_min_ratio: 3,
  cleanup_max_inactive_h: 0,
};

let view: MountedView | null = null;

async function mountPage() {
  const AutoCleanup = (await import("./AutoCleanup.vue")).default;
  view = mountView(AutoCleanup);
}

/** 自动删种卡与做种竞争度卡各有一颗，共用同一份 save() */
function saveButtons(): HTMLButtonElement[] {
  return [...document.querySelectorAll<HTMLButtonElement>("button")].filter(
    (b) => (b.textContent ?? "").trim() === "保存设置",
  );
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("自动清理：配置没读回来时不许保存", () => {
  it("还在加载：两颗保存键都禁用", async () => {
    api.getGlobal.mockReturnValue(new Promise(() => {}));
    await mountPage();

    await vi.waitFor(() => expect(saveButtons()).toHaveLength(2));
    for (const b of saveButtons()) expect(b.disabled).toBe(true);
  });

  it("加载失败：两颗保存键都禁用，点了也不会把默认值写回服务端", async () => {
    api.getGlobal.mockRejectedValue(new Error("数据库连接失败"));
    await mountPage();

    await vi.waitFor(() => expect(document.body.textContent).toContain("配置加载失败"));
    for (const b of saveButtons()) {
      expect(b.disabled).toBe(true);
      b.click();
    }
    await new Promise((r) => setTimeout(r, 20));
    expect(api.saveGlobal).not.toHaveBeenCalled();
  });

  it("加载成功后照常保存，提交的删种条件是读回来的那一套", async () => {
    api.getGlobal.mockResolvedValue(structuredClone(SETTINGS));
    api.saveGlobal.mockResolvedValue(undefined);
    await mountPage();

    await vi.waitFor(() => {
      expect(saveButtons()).toHaveLength(2);
      for (const b of saveButtons()) expect(b.disabled).toBe(false);
    });
    saveButtons()[0]!.click();
    await vi.waitFor(() => expect(api.saveGlobal).toHaveBeenCalledTimes(1));
    const payload = api.saveGlobal.mock.calls[0]![0];
    expect(payload.cleanup_max_seed_time_h).toBe(240);
    expect(payload.cleanup_min_ratio).toBe(3);
    expect(payload.cleanup_protect_tags).toBe("keep");
  });
});
