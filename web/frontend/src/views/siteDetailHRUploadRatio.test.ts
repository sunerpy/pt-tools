// @vitest-environment happy-dom
/*
 * 钉子：站点详情的「H&R 上传量达标」（#542）。读出来的值显示在数字框里，改了以后随站点配置一起保存；
 * 说明写清楚只在站点规则写明可以这样达标时填、只算下完的种子。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ getSite: vi.fn(), saveSite: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    sitesApi: {
      get: api.getSite,
      save: api.saveSite,
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

async function mountDetail(ratio: number) {
  api.getSite.mockResolvedValue({
    enabled: true,
    auth_method: "cookie",
    cookie: "",
    has_cookie: true,
    api_key: "",
    api_url: "https://hdsky.example",
    passkey: "",
    upload_limit_kbs: 0,
    download_limit_kbs: 0,
    seeding_capacity_gb: 0,
    hr_seed_time_h: 72,
    hr_upload_ratio: ratio,
    rss: [],
  });
  api.saveSite.mockResolvedValue(undefined);
  const SiteDetail = (await import("./SiteDetail.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/sites/:name", name: "site-detail", component: SiteDetail },
      { path: "/sites", component: { render: () => null } },
    ],
  });
  await router.push("/sites/hdsky");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
  await vi.waitFor(() => expect(buttonByText("保存配置").disabled).toBe(false));
}

function ratioInput(): HTMLInputElement {
  const input = document.querySelector<HTMLInputElement>(
    "[data-testid=site-hr-upload-ratio] input",
  );
  if (!input) throw new Error("找不到「H&R 上传量达标」");
  return input;
}

afterEach(() => {
  view?.unmount();
  view = null;
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

describe("站点详情：H&R 上传量达标", () => {
  it("显示读出来的值，改了以后随站点配置一起保存", async () => {
    await mountDetail(0.9);
    await vi.waitFor(() => expect(ratioInput().value).toBe("0.9"));
    const item = ratioInput().closest(".el-form-item") as HTMLElement;
    expect(item.querySelector(".el-form-item__label")?.textContent).toContain("H&R 上传量达标");
    expect(item.textContent).toContain("只在站点规则写明可以这样达标时填");
    expect(item.textContent).toContain("只算下完的种子");

    const input = ratioInput();
    input.value = "1.2";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
    buttonByText("保存配置").click();
    await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalled());
    const body = api.saveSite.mock.calls.at(-1)!.at(-1) as {
      hr_upload_ratio?: number;
      hr_seed_time_h?: number;
    };
    expect(body.hr_upload_ratio).toBe(1.2);
    expect(body.hr_seed_time_h).toBe(72);
  });
});
