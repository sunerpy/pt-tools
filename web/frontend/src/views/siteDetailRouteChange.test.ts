// @vitest-environment happy-dom
/*
 * 钉子：站点详情在三种情况下不能把错的东西写出去或留在页面上。
 *
 * 1. 只改路由参数（/sites/A → /sites/B）时 Vue Router 复用组件。原来只在 onMounted 加载，
 *    页面还是 A 的表单、loaded 仍为真，点「保存配置」就把 A 的整份配置 PUT 到 B
 *    （后端 UpsertSiteWithRSS 先删光 B 的 RSS 再按提交内容重建）。
 * 2. 编辑 RSS 保存后要重新读一次详情。原来这次读取写在 finally 里、排在复位 loading 之前，
 *    读取失败时 finally 自己抛错，保存键一直转圈，还多出一个未处理的 rejection。
 * 3. RSS 链接通常带 passkey，添加、编辑时原来会把完整链接打到控制台。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  getSite: vi.fn(),
  saveSite: vi.fn(),
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
      listLoginStates: vi.fn(() => Promise.resolve([])),
      deleteRss: vi.fn(),
      delete: vi.fn(),
    },
    attendanceApi: { list: vi.fn(() => Promise.resolve([])), signNow: vi.fn() },
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    filterRulesApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    chatopsApi: { notifications: { list: vi.fn(() => Promise.resolve([])) } },
    userInfoApi: { getSite: vi.fn(() => Promise.resolve(null)) },
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
    api_url: "",
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

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
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
  const input = document.querySelector<HTMLInputElement>(selector);
  if (!input) throw new Error(`找不到输入框 ${selector}`);
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

describe("站点详情：切换站点", () => {
  it("从 A 站直接切到 B 站：重新加载 B，保存提交的是 B 的配置", async () => {
    api.getSite.mockImplementation((name: string) => Promise.resolve(site(name)));
    api.saveSite.mockResolvedValue(undefined);
    const router = await mountDetail("/sites/hdsky");
    await vi.waitFor(() => expect(saveBtn().disabled).toBe(false));

    await router.push("/sites/pter");
    await vi.waitFor(() => expect(api.getSite).toHaveBeenCalledWith("pter"));
    await vi.waitFor(() => expect(saveBtn().disabled).toBe(false));
    saveBtn().click();

    await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalledTimes(1));
    const [name, payload] = api.saveSite.mock.calls[0]!;
    expect(name).toBe("pter");
    expect(payload.rss[0].name).toBe("pter 订阅");
  });

  it("B 还在加载：保存禁用，点了也不会把 A 的表单写到 B", async () => {
    api.getSite.mockImplementation((name: string) =>
      name === "hdsky" ? Promise.resolve(site(name)) : new Promise(() => {}),
    );
    const router = await mountDetail("/sites/hdsky");
    await vi.waitFor(() => expect(saveBtn().disabled).toBe(false));

    await router.push("/sites/pter");
    await vi.waitFor(() => expect(api.getSite).toHaveBeenCalledWith("pter"));
    await vi.waitFor(() => expect(saveBtn().disabled).toBe(true));
    saveBtn().click();
    expect(api.saveSite).not.toHaveBeenCalled();
  });

  it("A 的响应比 B 晚回来：不会落到 B 的页面上", async () => {
    const late = deferred<ReturnType<typeof site>>();
    api.getSite.mockImplementation((name: string) =>
      name === "hdsky" ? late.promise : Promise.resolve(site(name)),
    );
    api.saveSite.mockResolvedValue(undefined);
    const router = await mountDetail("/sites/hdsky");
    await vi.waitFor(() => expect(api.getSite).toHaveBeenCalledWith("hdsky"));

    await router.push("/sites/pter");
    await vi.waitFor(() => expect(saveBtn().disabled).toBe(false));
    late.resolve(site("hdsky"));
    // 迟到的响应只经过微任务链：过一个宏任务边界，它该落地的都已经落地
    await new Promise((r) => setTimeout(r, 0));
    saveBtn().click();

    await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalledTimes(1));
    const [name, payload] = api.saveSite.mock.calls[0]!;
    expect(name).toBe("pter");
    expect(payload.rss[0].name).toBe("pter 订阅");
  });
});

describe("站点详情：RSS 写操作", () => {
  it("编辑 RSS 保存成功、重新读取失败：保存键复位，提示刷新而不是报保存失败", async () => {
    api.getSite.mockResolvedValueOnce(site("hdsky")).mockRejectedValueOnce(new Error("网络断开"));
    api.saveSite.mockResolvedValue(undefined);
    await mountDetail("/sites/hdsky");
    await vi.waitFor(() => expect(saveBtn().disabled).toBe(false));

    buttonByText("编辑").click();
    await vi.waitFor(() => buttonByText("保存"));
    buttonByText("保存").click();

    await vi.waitFor(() => expect(api.getSite).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalled());
    expect(ui.error).not.toHaveBeenCalled();
    const btn = buttonByText("保存");
    expect(btn.classList.contains("is-loading")).toBe(false);
    expect(btn.disabled).toBe(false);
  });

  it("添加、编辑 RSS 不把订阅链接（含 passkey）打到控制台", async () => {
    const calls: unknown[][] = [];
    for (const level of ["log", "info", "warn", "error", "debug"] as const) {
      vi.spyOn(console, level).mockImplementation((...args: unknown[]) => {
        calls.push(args);
      });
    }
    api.getSite.mockImplementation(() => Promise.resolve(site("hdsky")));
    api.saveSite.mockResolvedValue(undefined);
    await mountDetail("/sites/hdsky");
    await vi.waitFor(() => expect(saveBtn().disabled).toBe(false));

    buttonByText("编辑").click();
    await vi.waitFor(() => buttonByText("保存"));
    buttonByText("保存").click();
    await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalledTimes(1));

    buttonByText("添加 RSS").click();
    await vi.waitFor(() => buttonByText("添加"));
    fill('input[placeholder="如：CMCT 电视剧"]', "新订阅");
    fill(
      'input[placeholder="https://..."]',
      `https://hdsky.example/torrentrss.php?cat=1&${SECRET}`,
    );
    buttonByText("添加").click();
    await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalledTimes(2));

    expect(
      calls.map((args) => args.map(String).join(" ")).filter((s) => s.includes(SECRET)),
    ).toEqual([]);
  });
});
