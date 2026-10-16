// @vitest-environment happy-dom
/*
 * 钉子：站点详情「站点统计」卡里账号的下载量写明口径（设计 free-expiry-safety B.2.17）。
 * 「上传 / 下载（计费）」是站点记账的数；站点另外报了实际传输量时多一行「实际上传 / 下载」，
 * 不报的站点没有这一行。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const TiB = 1024 ** 4;

const api = vi.hoisted(() => ({ getUserInfo: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    sitesApi: {
      get: vi.fn(() =>
        Promise.resolve({
          enabled: true,
          auth_method: "api_key",
          cookie: "",
          api_key: "",
          api_url: "",
          passkey: "",
          upload_limit_kbs: 0,
          download_limit_kbs: 0,
          seeding_capacity_gb: 0,
          rss: [],
        }),
      ),
      save: vi.fn(),
      listLoginStates: vi.fn(() => Promise.resolve([])),
      deleteRss: vi.fn(),
      delete: vi.fn(),
    },
    attendanceApi: { list: vi.fn(() => Promise.resolve([])), signNow: vi.fn() },
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    filterRulesApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    chatopsApi: { notifications: { list: vi.fn(() => Promise.resolve([])) } },
    userInfoApi: {
      getSite: api.getUserInfo,
      getHistory: vi.fn(() => Promise.resolve({ points: [] })),
    },
    tasksApi: { list: vi.fn(() => Promise.resolve({ items: [], total: 0 })) },
  };
});

function stats(extra: Record<string, number> = {}) {
  return {
    site: "mteam",
    username: "u",
    userId: "1",
    uploaded: 3 * TiB,
    downloaded: 1 * TiB,
    ratio: 3,
    bonus: 0,
    seeding: 5,
    leeching: 0,
    rank: "",
    lastUpdate: 1758000000,
    ...extra,
  };
}

let view: MountedView | null = null;

async function mountDetail() {
  const SiteDetail = (await import("./SiteDetail.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/sites/:name", name: "site-detail", component: SiteDetail },
      { path: "/sites", component: { render: () => null } },
    ],
  });
  await router.push("/sites/mteam");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
}

/** 「站点统计」卡的「键 → 值」 */
function statRows(): Record<string, string> {
  const out: Record<string, string> = {};
  for (const li of document.querySelectorAll(".sd-kv__row")) {
    const k = (li.querySelector(".sd-kv__k")?.textContent ?? "").trim();
    out[k] = (li.querySelector(".sd-kv__v")?.textContent ?? "").replace(/\s+/g, " ").trim();
  }
  return out;
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("站点详情：站点统计卡的下载量口径", () => {
  it("站点报了实际传输：计费与实际各一行", async () => {
    api.getUserInfo.mockResolvedValue(stats({ trueUploaded: 3 * TiB, trueDownloaded: 2.5 * TiB }));
    await mountDetail();

    await vi.waitFor(() => expect(statRows()["上传 / 下载（计费）"]).toBe("3.00 TB / 1.00 TB"));
    expect(statRows()["实际上传 / 下载"]).toBe("3.00 TB / 2.50 TB");
  });

  it("站点不报实际传输：只有计费那一行", async () => {
    api.getUserInfo.mockResolvedValue(stats());
    await mountDetail();

    await vi.waitFor(() => expect(statRows()["上传 / 下载（计费）"]).toBe("3.00 TB / 1.00 TB"));
    expect(document.querySelector("[data-testid=site-true-transfer]")).toBeNull();
  });
});
