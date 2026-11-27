/**
 * @vitest-environment happy-dom
 *
 * 飞牛应用中心安装的 pt-tools（运行环境 managed_by = "fnos"）不在网页里升级：
 * 升级按钮按 canSelfUpgrade 隐藏，版本面板按 isFnOSApp 显示到 Release 下载新版 fpk 的提示。
 */
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { RuntimeEnvironment } from "../api";

const api = vi.hoisted(() => ({ getRuntime: vi.fn() }));

vi.mock("../api", async (orig) => {
  const real = await orig<typeof import("../api")>();
  return { ...real, versionApi: { ...real.versionApi, ...api } };
});

function runtimeOf(extra: Partial<RuntimeEnvironment>): RuntimeEnvironment {
  return {
    is_docker: false,
    os: "linux",
    arch: "amd64",
    executable: "/vol1/@appcenter/pt-tools/bin/pt-tools",
    can_self_upgrade: true,
    ...extra,
  };
}

async function storeWith(runtime: RuntimeEnvironment) {
  api.getRuntime.mockResolvedValue({ runtime, upgrade_progress: { status: "idle" } });
  const { useVersionStore } = await import("./version");
  const store = useVersionStore();
  await store.fetchRuntime();
  return store;
}

beforeEach(() => {
  setActivePinia(createPinia());
  vi.clearAllMocks();
});

describe("运行环境：飞牛应用", () => {
  it("managed_by 为 fnos 时是飞牛应用，不能在网页里升级", async () => {
    const store = await storeWith(runtimeOf({ managed_by: "fnos", can_self_upgrade: false }));
    expect(store.isFnOSApp).toBe(true);
    expect(store.canSelfUpgrade).toBe(false);
    expect(store.isDocker).toBe(false);
  });

  it("没有 managed_by 的二进制部署照旧可以升级", async () => {
    const store = await storeWith(runtimeOf({}));
    expect(store.isFnOSApp).toBe(false);
    expect(store.canSelfUpgrade).toBe(true);
  });

  it("不认识的 managed_by 不当作飞牛应用", async () => {
    const store = await storeWith(runtimeOf({ managed_by: "other", can_self_upgrade: false }));
    expect(store.isFnOSApp).toBe(false);
  });
});
