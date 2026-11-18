// @vitest-environment happy-dom
/*
 * 钉子：Rousi Pro 的 Passkey 一栏填的是个人 API Key。Rousi 的「账户 → API Key」页写明外部工具填这把 Key、
 * 不要填 Tracker Passkey，并按权限授予（签到要「执行签到」）；表单照这个说法写，别的 Passkey 站点不变。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { type MountedView, mountView } from "@/test-mount";

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    sitesApi: {
      get: vi.fn(() =>
        Promise.resolve({
          enabled: true,
          auth_method: "passkey",
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
      getSite: vi.fn(() => Promise.reject(new Error("no stats"))),
      getHistory: vi.fn(() => Promise.resolve({ points: [] })),
    },
    tasksApi: { list: vi.fn(() => Promise.resolve({ items: [], total: 0 })) },
  };
});

let view: MountedView | null = null;

async function mountDetail(name: string) {
  const SiteDetail = (await import("./SiteDetail.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/sites/:name", name: "site-detail", component: SiteDetail },
      { path: "/sites", component: { render: () => null } },
    ],
  });
  await router.push(`/sites/${name}`);
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
  await vi.waitFor(() => expect(document.querySelector("[data-testid=sd-passkey]")).not.toBeNull());
}

function passkeyItem(): HTMLElement {
  return document
    .querySelector("[data-testid=sd-passkey]")!
    .closest(".el-form-item") as HTMLElement;
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("站点详情：Rousi Pro 的 Passkey 一栏", () => {
  it("写明填个人 API Key、不是 Tracker Passkey，并列出签到要的权限", async () => {
    await mountDetail("rousipro");
    const item = passkeyItem();
    expect(item.querySelector(".el-form-item__label")?.textContent).toContain("个人 API Key");
    const tip = document.querySelector("[data-testid=sd-passkey-tip]");
    expect(tip?.textContent).toContain("不是 Tracker Passkey");
    expect(tip?.textContent).toContain("执行签到");
    expect(tip?.querySelector("a")?.getAttribute("href")).toBe("https://rousi.pro/account/api-key");
  });

  it("别的 Passkey 站点照旧", async () => {
    await mountDetail("someptsite");
    expect(passkeyItem().querySelector(".el-form-item__label")?.textContent?.trim()).toBe(
      "Passkey",
    );
    expect(document.querySelector("[data-testid=sd-passkey-tip]")).toBeNull();
  });
});
