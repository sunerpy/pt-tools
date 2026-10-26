// @vitest-environment happy-dom
/*
 * 钉子：添加 RSS 时的下载路径。点开列出所选下载器（没选时是默认下载器）预设的目录，选一个就行；
 * 列表里没有的路径直接输入（以前要先点「自定义」）。2026-10-26 用户：「下载器设置了下载目录，就可以直接选择」。
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
    downloadersApi: {
      list: vi.fn(() =>
        Promise.resolve([
          {
            id: 1,
            name: "qb",
            type: "qbittorrent",
            url: "",
            username: "",
            is_default: true,
            enabled: true,
          },
        ]),
      ),
    },
    filterRulesApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderDirectoriesApi: {
      listAll: vi.fn(() =>
        Promise.resolve({
          1: [
            { id: 3, downloader_id: 1, path: "/downloads", alias: "", is_default: true },
            { id: 4, downloader_id: 1, path: "/downloads/tv", alias: "剧集", is_default: false },
          ],
        }),
      ),
    },
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

function pathInput(): HTMLInputElement {
  const el = q("rss-new-path")!;
  return (el instanceof HTMLInputElement ? el : el.querySelector("input")) as HTMLInputElement;
}

async function addRss(pick: (input: HTMLInputElement) => Promise<void>) {
  api.getSite.mockImplementation(() => Promise.resolve(site("hdsky")));
  api.saveSite.mockResolvedValue(undefined);
  await mountDetail("/sites/hdsky");
  await vi.waitFor(() => expect(saveBtn().disabled).toBe(false));
  buttonByText("添加 RSS").click();
  await vi.waitFor(() => buttonByText("添加"));
  fill('input[placeholder="如：CMCT 电视剧"]', "HDSky 剧集");
  fill('[data-testid="rss-new-url"]', "https://hdsky.example/torrentrss.php?passkey=k");
  await vi.waitFor(() => expect(q("rss-new-path")).not.toBeNull());
  await pick(pathInput());
  buttonByText("添加").click();
  await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalledTimes(1));
  const body = api.saveSite.mock.calls[0]!.at(-1) as {
    rss: { name: string; download_path: string }[];
  };
  return body.rss.find((r) => r.name === "HDSky 剧集")!;
}

describe("站点详情：RSS 的下载路径", () => {
  it("点开列出默认下载器的预设目录，选一个", async () => {
    const rss = await addRss(async (input) => {
      input.focus();
      input.dispatchEvent(new Event("focus"));
      let items: HTMLElement[] = [];
      await vi.waitFor(() => {
        items = [...document.querySelectorAll<HTMLElement>(".pt-path-input__popper li")];
        expect(items).toHaveLength(2);
      });
      items.find((li) => (li.textContent ?? "").includes("剧集"))!.click();
      await vi.waitFor(() => expect(input.value).toBe("/downloads/tv"));
    });
    expect(rss.download_path).toBe("/downloads/tv");
  });

  it("列表里没有的路径直接输入", async () => {
    const rss = await addRss(async (input) => {
      input.value = "/mnt/rss-other";
      input.dispatchEvent(new Event("input", { bubbles: true }));
      await new Promise((r) => setTimeout(r, 0));
    });
    expect(rss.download_path).toBe("/mnt/rss-other");
  });
});
