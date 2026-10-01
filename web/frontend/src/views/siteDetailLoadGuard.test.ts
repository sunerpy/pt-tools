// @vitest-environment happy-dom
/*
 * 钉子：站点详情没加载成功之前，三处写操作（保存配置、添加 RSS、编辑 RSS）一律不放行。
 *
 * 这三处都是把**整份** form 交给后端，而后端 UpsertSiteWithRSS 会先删光该站点的 RSS 再按提交内容重建。
 * 详情还没回来时 form 是默认空表单 —— 原来的锁只看 error / perm，首次加载中、失败后重试加载中
 * 按钮都能点，点一下就用空配置把真实站点覆盖掉、把订阅整批删掉。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  getSite: vi.fn(),
  saveSite: vi.fn(),
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
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    filterRulesApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    chatopsApi: { notifications: { list: vi.fn(() => Promise.resolve([])) } },
    userInfoApi: { getSite: vi.fn(() => Promise.resolve(null)) },
    tasksApi: { list: vi.fn(() => Promise.resolve({ items: [], total: 0 })) },
  };
});

const SITE = {
  enabled: true,
  auth_method: "cookie",
  cookie: "",
  api_key: "",
  api_url: "",
  passkey: "",
  upload_limit_kbs: 0,
  download_limit_kbs: 0,
  seeding_capacity_gb: 0,
  rss: [{ id: 7, name: "真实订阅", url: "https://example.com/rss", interval_minutes: 10 }],
};

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
  await router.push("/sites/hdsky");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
}

/** 页头里那颗「保存配置」与 RSS 卡上那颗「添加 RSS」 */
const saveBtn = () => buttonByText("保存配置");
const addRssBtn = () => buttonByText("添加 RSS");

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("站点详情：没加载成功时不许写", () => {
  it("首次加载还在飞：保存配置与添加 RSS 都禁用，点了也不发保存", async () => {
    api.getSite.mockReturnValue(new Promise(() => {}));
    await mountDetail();

    await vi.waitFor(() => expect(saveBtn().disabled).toBe(true));
    expect(addRssBtn().disabled).toBe(true);
    saveBtn().click();
    expect(api.saveSite).not.toHaveBeenCalled();
  });

  it("加载失败后点「重试」、重试还在飞：仍然禁用（form 里还是默认空表单）", async () => {
    api.getSite.mockRejectedValueOnce(new Error("连接被拒绝"));
    await mountDetail();
    await vi.waitFor(() => buttonByText("重试"));
    expect(saveBtn().disabled).toBe(true);

    api.getSite.mockReturnValue(new Promise(() => {}));
    buttonByText("重试").click();
    await vi.waitFor(() => expect(api.getSite).toHaveBeenCalledTimes(2));
    expect(saveBtn().disabled).toBe(true);
    expect(addRssBtn().disabled).toBe(true);
    saveBtn().click();
    expect(api.saveSite).not.toHaveBeenCalled();
  });

  it("加载成功之后照常可以保存，提交的是读回来的真实配置", async () => {
    api.getSite.mockResolvedValue(structuredClone(SITE));
    api.saveSite.mockResolvedValue(undefined);
    await mountDetail();

    await vi.waitFor(() => expect(saveBtn().disabled).toBe(false));
    expect(addRssBtn().disabled).toBe(false);
    saveBtn().click();
    await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalledTimes(1));
    const [name, payload] = api.saveSite.mock.calls[0]!;
    expect(name).toBe("hdsky");
    expect(payload.rss).toHaveLength(1);
    expect(payload.rss[0].name).toBe("真实订阅");
  });
});
