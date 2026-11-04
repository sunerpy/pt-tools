/*
 * 钉子：外壳的 runtime store 每拍从 transfer-stats 记下每台下载器的速度与服务端记的最近几拍总速度（首页的 NAS 状态跟着它更新）。
 * 这一拍没带速度历史时留着上一拍的；取不到时标成过期、留着上一拍的值。
 */
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";

const stats = vi.hoisted(() => ({ transferStats: vi.fn() }));

vi.mock("../api", async (orig) => {
  const real = await orig<typeof import("../api")>();
  return {
    ...real,
    downloaderTorrentsApi: { ...real.downloaderTorrentsApi, transferStats: stats.transferStats },
    sitesApi: { ...real.sitesApi, list: vi.fn(async () => ({})) },
    tasksApi: { ...real.tasksApi, list: vi.fn(async () => ({ items: [], total: 0 })) },
    pausedTorrentsApi: {
      ...real.pausedTorrentsApi,
      list: vi.fn(async () => ({ items: [], total: 0 })),
    },
    controlApi: { ...real.controlApi, status: vi.fn(async () => null) },
  };
});

import { useRuntimeStore } from "./runtime";

const item = {
  downloader_id: 1,
  downloader_name: "qb",
  downloader_type: "qbittorrent",
  upload_speed: 100,
  download_speed: 200,
  uploaded: 0,
  downloaded: 0,
  session_uploaded: 0,
  session_downloaded: 0,
  free_space: 0,
  reachable: true,
};
const sample = { at: "2026-11-04T12:00:00Z", down: 200, up: 100, answered: 1, total: 1 };

beforeEach(() => {
  setActivePinia(createPinia());
  stats.transferStats.mockReset();
});

describe("runtime store 的速度", () => {
  it("记下每台下载器这一拍的速度与最近几拍的总速度", async () => {
    stats.transferStats.mockResolvedValue({
      total_download_speed: 200,
      total_upload_speed: 100,
      total_free_space: 0,
      downloaders: [item],
      speed_history: [sample],
    });
    const rt = useRuntimeStore();
    await rt.refresh();
    expect(rt.stale).toBe(false);
    expect(rt.downloaderStats).toEqual([item]);
    expect(rt.speedHistory).toEqual([sample]);
  });

  it("这一拍没带速度历史时留着上一拍的；取不到时标成过期、留着上一拍的值", async () => {
    const rt = useRuntimeStore();
    stats.transferStats.mockResolvedValueOnce({
      total_download_speed: 200,
      total_upload_speed: 100,
      total_free_space: 0,
      downloaders: [item],
      speed_history: [sample],
    });
    await rt.refresh();
    stats.transferStats.mockResolvedValueOnce({
      total_download_speed: 300,
      total_upload_speed: 100,
      total_free_space: 0,
    });
    await rt.refresh();
    expect(rt.speedHistory).toEqual([sample]);
    expect(rt.downloaderStats, "没带下载器时清空").toEqual([]);
    expect(rt.downloadSpeed).toBe(300);

    stats.transferStats.mockRejectedValueOnce(new Error("下载器离线"));
    await rt.refresh();
    expect(rt.stale).toBe(true);
    expect(rt.downloadSpeed).toBe(300);
    expect(rt.speedHistory).toEqual([sample]);
  });
});
