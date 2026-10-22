// @vitest-environment happy-dom
/*
 * 钉子：添加、编辑 RSS 时链接的检查。示例里的占位地址（hdsky.xxx 这类）取不到任何种子，保存时拦下并说清楚；
 * 域名不是这个站点的只提醒；「试一下」现拉一次，说取到几条或哪一步出了问题。列表里占位地址的那条标出来。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  getSite: vi.fn(),
  saveSite: vi.fn(),
  testRssLink: vi.fn(),
}));

const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    sitesApi: {
      get: api.getSite,
      save: api.saveSite,
      testRssLink: api.testRssLink,
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
      getSite: vi.fn(() => Promise.resolve(null)),
      getHistory: vi.fn(() => Promise.resolve({ points: [] })),
    },
    tasksApi: { list: vi.fn(() => Promise.resolve({ items: [], total: 0 })) },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      warning: ui.warning,
      info: vi.fn(),
    }),
  };
});

const SECRET = "passkey=SECRET0123";

function site(name: string) {
  return {
    enabled: true,
    auth_method: "cookie",
    cookie: "",
    api_key: "",
    api_url: `https://${name}.example`,
    urls: [`https://${name}.example`],
    passkey: "",
    upload_limit_kbs: 0,
    download_limit_kbs: 0,
    seeding_capacity_gb: 0,
    rss: [
      {
        id: 7,
        name: `${name} 订阅`,
        url: `https://${name}.example/torrentrss.php?${SECRET}`,
        interval_minutes: 10,
      },
    ],
  };
}

let view: MountedView | null = null;

async function mountDetail(path: string): Promise<Router> {
  const SiteDetail = (await import("./SiteDetail.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/sites/:name", name: "site-detail", component: SiteDetail },
      { path: "/sites", component: { render: () => null } },
    ],
  });
  await router.push(path);
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
  return router;
}

const saveBtn = () => buttonByText("保存配置");

function fill(selector: string, value: string) {
  const el = document.querySelector<HTMLElement>(selector);
  // el-input 把 data-testid 放在里面的 input 上；按 placeholder 找到的本身就是 input
  const input = (
    el?.tagName === "INPUT" ? el : el?.querySelector("input")
  ) as HTMLInputElement | null;
  if (!input) throw new Error(`找不到输入框 ${selector}`);
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

afterEach(() => {
  view?.unmount();
  view = null;
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);

describe("站点详情：RSS 链接检查", () => {
  it("占位地址拦下，域名对不上只提醒，试一下给结果", async () => {
    api.getSite.mockImplementation(() => Promise.resolve(site("hdsky")));
    api.saveSite.mockResolvedValue(undefined);
    await mountDetail("/sites/hdsky");
    await vi.waitFor(() => expect(saveBtn().disabled).toBe(false));

    buttonByText("添加 RSS").click();
    await vi.waitFor(() => buttonByText("添加"));
    fill('input[placeholder="如：CMCT 电视剧"]', "HDSky");
    fill('[data-testid="rss-new-url"]', "https://hdsky.xxx/torrentrss.php?xxx");
    await vi.waitFor(() =>
      expect(q("rss-new-problem")?.textContent).toContain("占位地址（hdsky.xxx）"),
    );
    // 占位地址取不到任何种子：「试一下」点不了（下面已经写明为什么）
    expect((q("rss-new-test") as HTMLButtonElement).disabled).toBe(true);
    buttonByText("添加").click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalled());
    expect(String(ui.error.mock.calls[0]![0])).toContain("占位地址");
    expect(api.saveSite).not.toHaveBeenCalled();

    // 别的站点的链接：只提醒，照样能加
    fill('[data-testid="rss-new-url"]', "https://springsunday.net/torrentrss.php?passkey=k");
    await vi.waitFor(() => expect(q("rss-new-problem")?.textContent).toContain("不像是这个站点的"));
    expect(q("rss-new-problem")!.className).toContain("field-tip--warn");

    // 试一下：能用时说取到几条
    api.testRssLink.mockResolvedValueOnce({ title: "SSD", count: 30, titles: ["最新的一部"] });
    q("rss-new-test")!.click();
    await vi.waitFor(() =>
      expect(q("rss-new-test-result")?.textContent).toContain(
        "能用：取到 30 条，最新一条「最新的一部」",
      ),
    );
    // 链接改了：旧结果不显示
    fill('[data-testid="rss-new-url"]', "https://hdsky.example/torrentrss.php?passkey=k");
    await vi.waitFor(() => expect(q("rss-new-test-result")).toBeNull());
    expect(q("rss-new-problem")).toBeNull();
    // 失败时显示说明
    const { ApiError } = await import("@/api");
    api.testRssLink.mockRejectedValueOnce(
      new ApiError(502, "hdsky.example 回了 HTTP 403 Forbidden：passkey 可能失效了"),
    );
    q("rss-new-test")!.click();
    await vi.waitFor(() => expect(q("rss-new-test-result")?.textContent).toContain("HTTP 403"));
    expect(q("rss-new-test-result")!.className).toContain("field-tip--dang");

    buttonByText("添加").click();
    await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalledTimes(1));
  });

  it("库里已有的占位地址：列表里标出来；编辑时不改地址照样能存", async () => {
    const s = site("hdsky");
    s.rss[0]!.url = "https://hdsky.xxx/torrentrss.php?xxx";
    api.getSite.mockImplementation(() => Promise.resolve(structuredClone(s)));
    api.saveSite.mockResolvedValue(undefined);
    await mountDetail("/sites/hdsky");
    await vi.waitFor(() => expect(q("rss-placeholder-7")).not.toBeNull());
    expect(q("rss-placeholder-7")!.textContent).toContain("占位地址，不会去拉");

    buttonByText("编辑").click();
    await vi.waitFor(() => buttonByText("保存"));
    expect(q("rss-edit-problem")?.textContent).toContain("占位地址");
    expect((q("rss-edit-test") as HTMLButtonElement).disabled).toBe(true);
    buttonByText("保存").click();
    await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalledTimes(1));
  });
});
