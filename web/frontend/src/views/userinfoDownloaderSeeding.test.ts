// @vitest-environment happy-dom
/*
 * 钉子：用户统计页的「下载器做种」卡。每台启用的下载器一行：正在做种的个数与体积；
 * 下载完了没在做种的写在小字里；连不上的那台写「没连上」与原因。整个请求失败时卡里只写一句、能重试。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const GiB = 1024 ** 3;

const api = vi.hoisted(() => ({ seedingStats: vi.fn() }));

vi.mock("@/components/SiteAvatar.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return { default: defineComponent({ render: () => h("span") }) };
});

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    userInfoApi: {
      ...real.userInfoApi,
      getAggregated: vi.fn(() =>
        Promise.resolve({
          totalUploaded: 10 * GiB,
          totalDownloaded: 1 * GiB,
          averageRatio: 10,
          totalSeeding: 3,
          totalLeeching: 0,
          totalBonus: 100,
          totalSeederSize: 3 * GiB,
          siteCount: 1,
          lastUpdate: 1758000000,
          perSiteStats: [
            {
              site: "hdsky",
              username: "u",
              uploaded: 10 * GiB,
              downloaded: 1 * GiB,
              ratio: 10,
              bonus: 100,
              seeding: 3,
              leeching: 0,
              rank: "",
              lastUpdate: 1758000000,
            },
          ],
        }),
      ),
      getSummary: vi.fn(() => Promise.resolve(null)),
      getTrends: vi.fn(() => Promise.resolve(null)),
    },
    downloaderTorrentsApi: { ...real.downloaderTorrentsApi, seedingStats: api.seedingStats },
    sitesApi: {
      ...real.sitesApi,
      list: vi.fn(() => Promise.resolve({})),
      listLoginStates: vi.fn(() => Promise.resolve([])),
    },
    tasksApi: { ...real.tasksApi, stats: vi.fn(() => Promise.resolve(null)) },
    siteLevelsApi: { ...real.siteLevelsApi, getAll: vi.fn(() => Promise.resolve({ sites: {} })) },
  };
});

let view: MountedView | null = null;

beforeEach(() => {
  setActivePinia(createPinia());
});

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

async function mountDashboard() {
  const Dash = (await import("./UserInfoDashboard.vue")).default;
  view = mountView(Dash);
}

function card(): HTMLElement | null {
  return document.querySelector<HTMLElement>("[data-testid=userinfo-downloader-seeding]");
}

describe("用户统计 · 下载器做种", () => {
  it("每台一行：做种个数与体积、没在做种的小字、连不上的原因；卡头是合计", async () => {
    api.seedingStats.mockResolvedValue({
      total_seeding_count: 7,
      total_seeding_size: 6 * GiB,
      downloaders: [
        {
          downloader_id: 1,
          downloader_name: "qb-nas",
          downloader_type: "qbittorrent",
          reachable: true,
          seeding_count: 5,
          seeding_size: 4 * GiB,
          idle_count: 2,
          idle_size: 1 * GiB,
        },
        {
          downloader_id: 2,
          downloader_name: "tr-box",
          downloader_type: "transmission",
          reachable: true,
          seeding_count: 2,
          seeding_size: 2 * GiB,
          idle_count: 0,
          idle_size: 0,
        },
        {
          downloader_id: 3,
          downloader_name: "qb-old",
          downloader_type: "qbittorrent",
          reachable: false,
          error: "dial tcp: connection refused",
          seeding_count: 0,
          seeding_size: 0,
          idle_count: 0,
          idle_size: 0,
        },
      ],
    });
    await mountDashboard();
    await vi.waitFor(() => expect(card()).toBeTruthy());

    const text = card()!.textContent ?? "";
    expect(text).toContain("下载器做种");
    expect(text).toContain("7 个 · 6 GB");
    expect(text).toContain("qb-nas");
    expect(text).toContain("5 个 · 4 GB");
    expect(text).toContain("另有 2 个下载完了没在做种（1 GB）");
    expect(text).toContain("2 个 · 2 GB");
    expect(text).toContain("没连上");
    expect(text).toContain("connection refused");
    // 没在做种的只写有的那台
    expect(text.match(/另有/g)?.length).toBe(1);
    // 连不上的原因在小字里只留两行，完整的放在这一行的悬停提示里；连得上的没有悬停提示
    const rows = [...card()!.querySelectorAll<HTMLElement>(".bd__row")];
    const row = (name: string) => rows.find((r) => (r.textContent ?? "").includes(name));
    expect(row("qb-old")?.getAttribute("title")).toBe("dial tcp: connection refused");
    expect(row("qb-nas")?.hasAttribute("title")).toBe(false);
  });

  it("没有启用的下载器时不显示这张卡", async () => {
    api.seedingStats.mockResolvedValue({
      total_seeding_count: 0,
      total_seeding_size: 0,
      downloaders: [],
    });
    await mountDashboard();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=userinfo-totals]")).toBeTruthy(),
    );
    await vi.waitFor(() => expect(api.seedingStats).toHaveBeenCalled());
    expect(card()).toBeNull();
  });

  it("请求失败时卡里写一句，点「重试」再取一次", async () => {
    api.seedingStats.mockRejectedValueOnce(new Error("boom"));
    api.seedingStats.mockResolvedValueOnce({
      total_seeding_count: 1,
      total_seeding_size: 1 * GiB,
      downloaders: [
        {
          downloader_id: 1,
          downloader_name: "qb-nas",
          downloader_type: "qbittorrent",
          reachable: true,
          seeding_count: 1,
          seeding_size: 1 * GiB,
          idle_count: 0,
          idle_size: 0,
        },
      ],
    });
    await mountDashboard();
    await vi.waitFor(() => expect(card()?.textContent ?? "").toContain("没取到下载器的做种情况"));

    const retry = [...card()!.querySelectorAll<HTMLButtonElement>("button")].find((b) =>
      (b.textContent ?? "").includes("重试"),
    );
    expect(retry).toBeTruthy();
    retry!.click();
    await vi.waitFor(() => expect(card()?.textContent ?? "").toContain("1 个 · 1 GB"));
    expect(api.seedingStats).toHaveBeenCalledTimes(2);
  });
});
