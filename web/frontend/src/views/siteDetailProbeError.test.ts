// @vitest-environment happy-dom
/*
 * 钉子：站点详情「保号规则」卡在探测没通过时写明原因。
 * 用户反馈：只看到「解析失败 (PARSE_ERROR)」，看不出是 Cookie 失效、站点改版还是别的原因。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ listLoginStates: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    sitesApi: {
      get: vi.fn(() =>
        Promise.resolve({
          enabled: true,
          auth_method: "cookie",
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
      listLoginStates: api.listLoginStates,
      deleteRss: vi.fn(),
      delete: vi.fn(),
    },
    attendanceApi: { list: vi.fn(() => Promise.resolve([])), signNow: vi.fn() },
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    filterRulesApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    chatopsApi: { notifications: { list: vi.fn(() => Promise.resolve([])) } },
    userInfoApi: {
      getSite: vi.fn(() => Promise.resolve(null)),
      getHistory: vi.fn(() => Promise.resolve({ points: [] })),
    },
    tasksApi: { list: vi.fn(() => Promise.resolve({ items: [], total: 0 })) },
  };
});

function state(status: string, error = "") {
  return {
    site_name: "audiences",
    enabled: true,
    last_probe_at: 1760850000,
    last_probe_status: status,
    last_probe_error: error,
    consecutive_probe_failures: status === "OK" ? 0 : 1,
    ban_threshold_days: 30,
    remind_before_days: 10,
    reminder_cron: "",
    notification_channel_ids: [],
    last_reminder_tier: "",
    days_remaining: 29,
    tier: "none",
    probe_mode: "auto",
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
  await router.push("/sites/audiences");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("站点详情：探测没通过时写明原因", () => {
  it("解析失败：在「探测结果」下面写出后端给的原因", async () => {
    api.listLoginStates.mockResolvedValue([
      state("PARSE_ERROR", "取回的页面上没有用户名，多半不是登录后的页面"),
    ]);
    await mountDetail();

    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=sd-probe-status]")?.textContent).toContain(
        "解析失败",
      ),
    );
    expect(document.querySelector("[data-testid=sd-probe-error]")?.textContent?.trim()).toBe(
      "取回的页面上没有用户名，多半不是登录后的页面",
    );
  });

  it("探测正常时不写原因（成功时后端也可能留一句说明）", async () => {
    api.listLoginStates.mockResolvedValue([
      state("OK", "页面上没有找到「最近动向」，按这次探测访问的时间记"),
    ]);
    await mountDetail();

    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=sd-probe-status]")?.textContent).toContain(
        "正常",
      ),
    );
    expect(document.querySelector("[data-testid=sd-probe-error]")).toBeNull();
  });
});
