// @vitest-environment happy-dom
/*
 * 站点列表（桌面表格）点行进站点详情；点到行里的启用开关、操作按钮时照旧做它们自己的事。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { type MountedView, mountView } from "@/test-mount";

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
    userInfoApi: { ...real.userInfoApi, getTrends: vi.fn(() => Promise.reject(new Error("n/a"))) },
  };
});

let view: MountedView | null = null;
let router: Router;

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
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/sites", component: SiteList },
      { path: "/sites/:name", component: { render: () => null } },
    ],
  });
  await router.push("/sites");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
  await vi.waitFor(() => expect(document.querySelector(".el-table__row .site__name")).toBeTruthy());
}

describe("站点列表点行进详情", () => {
  it("点站点名所在的行：进站点详情", async () => {
    await mountList();
    const row = document.querySelector(".el-table__row") as HTMLElement;
    expect(row.classList.contains("pt-row-link")).toBe(true);
    (document.querySelector(".el-table__row .site__name") as HTMLElement).click();
    await vi.waitFor(() => expect(router.currentRoute.value.fullPath).toBe("/sites/hdsky"));
  });

  it("点行里的操作按钮：不跳转，按钮照旧做自己的事", async () => {
    await mountList();
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    const btn = document.querySelector("[data-testid=open-site-btn-hdsky]") as HTMLElement | null;
    expect(btn).toBeTruthy();
    btn!.click();
    await new Promise((r) => setTimeout(r, 0));
    expect(router.currentRoute.value.fullPath).toBe("/sites");
    open.mockRestore();
  });
});
