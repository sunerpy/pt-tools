// @vitest-environment happy-dom
/*
 * 钉子：画板 30 的站点行卡里那条 120×18 的 8 根柱 = 该站最近 8 天每天的上传增量（每日快照）。
 * 拿不到走势就不画，不造假数据。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ getTrends: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    sitesApi: {
      ...real.sitesApi,
      list: vi.fn(() =>
        Promise.resolve({
          hdsky: {
            enabled: true,
            auth_method: "cookie",
            cookie: "",
            api_key: "",
            api_url: "",
            passkey: "",
            rss: [],
          },
        }),
      ),
      listLoginStates: vi.fn(() => Promise.resolve([])),
    },
    attendanceApi: { ...real.attendanceApi, list: vi.fn(() => Promise.resolve([])) },
    chatopsApi: { ...real.chatopsApi, notifications: { list: vi.fn(() => Promise.resolve([])) } },
    userInfoApi: { ...real.userInfoApi, getTrends: api.getTrends },
  };
});

function mobileViewport() {
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches: query.includes("max-width: 768px"),
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      }) as MediaQueryList,
  );
}

let view: MountedView | null = null;

beforeEach(() => {
  setActivePinia(createPinia());
  mobileViewport();
});

afterEach(() => {
  view?.unmount();
  view = null;
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

async function mountList() {
  const SiteList = (await import("./SiteList.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/sites", component: SiteList },
      { path: "/sites/:name", component: { render: () => null } },
    ],
  });
  await router.push("/sites");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
}

describe("站点列表行卡的走势柱", () => {
  it("有走势时每张行卡画 8 根柱", async () => {
    api.getTrends.mockResolvedValue({
      days: 8,
      from: "2026-09-29",
      to: "2026-10-06",
      dates: [],
      totals: [],
      sites: {
        hdsky: Array.from({ length: 8 }, (_, i) => ({
          date: `d${i}`,
          uploaded: i * 100,
          downloaded: 0,
          bonus: 0,
        })),
      },
    });
    await mountList();

    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=site-card-hdsky]")).toBeTruthy(),
    );
    await vi.waitFor(() =>
      expect(document.querySelectorAll("[data-testid=site-card-hdsky] .pt-bars i").length).toBe(8),
    );
    expect(api.getTrends).toHaveBeenCalledWith(8);
  });

  it("走势拿不到：不画柱", async () => {
    api.getTrends.mockRejectedValue(new Error("503"));
    await mountList();

    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=site-card-hdsky]")).toBeTruthy(),
    );
    await new Promise((r) => setTimeout(r, 0));
    expect(document.querySelector("[data-testid=site-card-hdsky] .pt-bars")).toBeNull();
  });
});
