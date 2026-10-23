// @vitest-environment happy-dom
/*
 * 站点列表的「状态」列与状态 chip：还没有探测结果的站点是灰色的「未探测」，不是绿色的「正常」。
 *
 * 以前从未探测过的站点（新装、或设成不探测、或动态站点暂不支持探测）一律画成绿色「正常」，
 * 「正常 N」的计数也把它们算进去 —— 把「不知道」说成了「健康」。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const site = (enabled: boolean) => ({
  enabled,
  auth_method: "cookie",
  cookie: "",
  api_key: "",
  api_url: "",
  passkey: "",
  rss: [],
});

const state = (site_name: string, extra: Record<string, unknown>) => ({
  site_name,
  enabled: true,
  consecutive_probe_failures: 0,
  ban_threshold_days: 0,
  remind_before_days: 0,
  ...extra,
});

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    sitesApi: {
      ...real.sitesApi,
      list: vi.fn(() =>
        Promise.resolve({
          hdsky: site(true),
          mteam: site(true),
          ourbits: site(true),
          pter: site(false),
        }),
      ),
      listLoginStates: vi.fn(() =>
        Promise.resolve([
          state("mteam", { last_probe_status: "OK" }),
          state("ourbits", { last_probe_status: "SESSION_EXPIRED" }),
        ]),
      ),
    },
    attendanceApi: { ...real.attendanceApi, list: vi.fn(() => Promise.resolve([])) },
    chatopsApi: { ...real.chatopsApi, notifications: { list: vi.fn(() => Promise.resolve([])) } },
    userInfoApi: { ...real.userInfoApi, getTrends: vi.fn(() => Promise.reject(new Error("n/a"))) },
  };
});

let view: MountedView | null = null;

beforeEach(() => {
  setActivePinia(createPinia());
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      }) as MediaQueryList,
  );
});

afterEach(() => {
  view?.unmount();
  view = null;
  vi.restoreAllMocks();
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
  await vi.waitFor(() =>
    expect(document.querySelector("[data-testid=site-status-ourbits]")).toBeTruthy(),
  );
}

const pill = (name: string) =>
  document.querySelector(`[data-testid=site-status-${name}] .pt-pill`) as HTMLElement;
const chip = (key: string) =>
  document.querySelector(`[data-testid=site-status-chip-${key}]`)?.textContent?.trim() ?? null;

describe("站点列表：状态列分出「未探测」", () => {
  it("没探测过的站点显示灰色「未探测」，探测成功的是绿色「正常」，失败的是「异常」", async () => {
    await mountList();
    expect(pill("hdsky").textContent?.trim()).toBe("未探测");
    expect(pill("hdsky").classList.contains("pt-pill--neutral")).toBe(true);
    expect(pill("mteam").textContent?.trim()).toBe("正常");
    expect(pill("mteam").classList.contains("pt-pill--ok")).toBe(true);
    expect(pill("ourbits").textContent?.trim()).toBe("异常");
  });

  it("状态 chip 的计数按四档分开：「正常」不再把没探测过的算进去", async () => {
    await mountList();
    expect(chip("ok")).toBe("正常 1");
    expect(chip("bad")).toBe("异常 1");
    expect(chip("none")).toBe("未探测 1");
    expect(chip("off")).toBe("已禁用 1");

    (document.querySelector("[data-testid=site-status-chip-none]") as HTMLElement).click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=site-status-mteam]")).toBeNull(),
    );
    expect(document.querySelector("[data-testid=site-status-hdsky]")).toBeTruthy();
  });
});
