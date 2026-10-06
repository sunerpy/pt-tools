/**
 * @vitest-environment happy-dom
 *
 * 升级进度轮询：原来是 setInterval(async …, 1000)。一次请求超过一秒就会和下一次重叠，
 * 接口持续失败（401、500）时也不退避、不停止，每秒一直发；取消升级也不清理定时器。
 * 现在一次请求回来才排下一次，失败时退避，连续失败到上限就停下并告知，取消和终态都停止轮询。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UpgradeProgress } from "../api";

const api = vi.hoisted(() => ({
  getUpgradeProgress: vi.fn(),
  startUpgrade: vi.fn(),
  cancelUpgrade: vi.fn(),
}));

const notify = vi.hoisted(() => vi.fn());

vi.mock("../api", async (orig) => {
  const real = await orig<typeof import("../api")>();
  return {
    ...real,
    versionApi: { ...real.versionApi, ...api },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return { ...real, ElNotification: notify };
});

function progress(status: UpgradeProgress["status"]): UpgradeProgress {
  return {
    status,
    target_version: "v1.0.0",
    progress: 50,
    bytes_downloaded: 1,
    total_bytes: 2,
  } as UpgradeProgress;
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

async function startedStore() {
  const { useVersionStore } = await import("./version");
  const store = useVersionStore();
  api.startUpgrade.mockResolvedValue({ success: true, message: "" });
  await store.startUpgrade("v1.0.0");
  return store;
}

beforeEach(() => {
  setActivePinia(createPinia());
  vi.useFakeTimers();
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

describe("升级进度轮询", () => {
  it("上一次请求没回来就不发下一次", async () => {
    const pending = deferred<UpgradeProgress>();
    api.getUpgradeProgress.mockReturnValueOnce(pending.promise);
    await startedStore();

    await vi.advanceTimersByTimeAsync(5_000);
    expect(api.getUpgradeProgress).toHaveBeenCalledTimes(1);

    api.getUpgradeProgress.mockResolvedValue(progress("downloading"));
    pending.resolve(progress("downloading"));
    await vi.advanceTimersByTimeAsync(1_000);
    expect(api.getUpgradeProgress).toHaveBeenCalledTimes(2);
  });

  it("接口持续失败时退避，到上限后停止并告知，升级状态复位", async () => {
    api.getUpgradeProgress.mockRejectedValue(new Error("401 未登录"));
    const store = await startedStore();

    await vi.advanceTimersByTimeAsync(10_000);
    // 1s 后第一次失败，之后按 2s、4s 退避：10 秒内只发 3 次，而不是 10 次
    expect(api.getUpgradeProgress).toHaveBeenCalledTimes(3);

    await vi.advanceTimersByTimeAsync(30 * 60_000);
    const calls = api.getUpgradeProgress.mock.calls.length;
    await vi.advanceTimersByTimeAsync(30 * 60_000);
    expect(api.getUpgradeProgress).toHaveBeenCalledTimes(calls);
    expect(store.upgrading).toBe(false);
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ title: "无法获取升级进度" }));
  });

  it("失败之后恢复成功：退避清零，回到每秒一次", async () => {
    api.getUpgradeProgress
      .mockRejectedValueOnce(new Error("502"))
      .mockResolvedValue(progress("downloading"));
    await startedStore();

    await vi.advanceTimersByTimeAsync(1_000); // 第一次，失败
    await vi.advanceTimersByTimeAsync(2_000); // 退避 2s 后第二次，成功
    expect(api.getUpgradeProgress).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(api.getUpgradeProgress).toHaveBeenCalledTimes(3);
  });

  it("升级完成后停止轮询", async () => {
    api.getUpgradeProgress.mockResolvedValue(progress("completed"));
    const store = await startedStore();

    await vi.advanceTimersByTimeAsync(1_000);
    expect(store.upgrading).toBe(false);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(api.getUpgradeProgress).toHaveBeenCalledTimes(1);
  });

  it("取消升级后停止轮询，在途请求回来也不再写进度", async () => {
    const pending = deferred<UpgradeProgress>();
    api.getUpgradeProgress.mockReturnValueOnce(pending.promise);
    api.cancelUpgrade.mockResolvedValue({ success: true, message: "" });
    const store = await startedStore();

    await vi.advanceTimersByTimeAsync(1_000);
    expect(api.getUpgradeProgress).toHaveBeenCalledTimes(1);
    await store.cancelUpgrade();
    pending.resolve(progress("downloading"));
    await vi.advanceTimersByTimeAsync(10_000);

    expect(api.getUpgradeProgress).toHaveBeenCalledTimes(1);
    expect(store.upgradeProgress).toBeNull();
    expect(store.upgrading).toBe(false);
  });
});
